# AGENTS.md

Guidance for AI agents working in the Dupli1 repository.

## Repository status

Dupli1 is a Go microservice backend for a fashion bag marketplace. The repo contains:

- Six HTTP services in `auth/`, `product/`, `order/`, `cart/`, `payment/`, `notification/` (each with `cmd/` + `pkg/`). `product` also owns stock/reservations (the former standalone `inventory` service was merged in).
- nginx gateway in `api/` (`dupli1-proxy` in Docker Compose)
- Docker Compose for local development
- Terraform and GitHub Actions for AWS ECS deployment

See [docs/current-state.md](docs/current-state.md) for the authoritative snapshot of what is implemented today. See [docs/service-layout.md](docs/service-layout.md) for directory and module layout.

## Documentation conventions

- AI agents must look up existing documents in `docs/` before writing new ones. You don't have to read everything — skim for relevant overlap first.
- Add a service-name prefix when creating a new document (e.g. `product-*.md`, `cart-*.md`). You can skip the `order` document when working in `product-service`.

## Cursor Cloud specific instructions

### Prerequisites

| Tool | Version / notes |
|------|-----------------|
| Go | 1.26 (auto-downloaded on first use via the `go` toolchain) — modules target Go 1.26 |
| Docker + Compose v2 | Installed in the VM snapshot. **The daemon does NOT auto-start** — run `sudo service docker start` (or `sudo dockerd &`) at the beginning of a session, then use `sudo docker …`. |

> **Docker is the supported local path** and provides Postgres, Redis, NATS and MinIO as containers (see below). Postgres/Redis are **not** installed as bare-metal system services on this image, so `sudo service postgresql start` / `redis-server start` will not work — use Docker Compose instead. Per-module `go test` runs directly on the host and needs no infrastructure (order/cart/payment use in-memory fallbacks when their DB URL is unset).

### Database credentials (dev)

Docker Compose provides two Postgres instances:

| Service | Host port | Database | User | Password |
|---------|-----------|----------|------|----------|
| `postgres-auth` | 5432 | `dupli1_db` | `dupli1` | `dupli1_dev` |
| `postgres-product` | 5433 | `products` | `dupli1` | `dupli1_dev` |
| `postgres-order` | 5435 | `orders` | `dupli1` | `dupli1_dev` |
| `postgres-cart` | 5436 | `cart` | `dupli1` | `dupli1_dev` |
| `postgres-payment` | 5437 | `payments` | `dupli1` | `dupli1_dev` |

Connection strings:

- Auth: `postgres://dupli1:dupli1_dev@localhost:5432/dupli1_db?sslmode=disable`
- Product: `postgres://dupli1:dupli1_dev@localhost:5433/products?sslmode=disable` (also stock/reservations)
- Order: `postgres://dupli1:dupli1_dev@localhost:5435/orders?sslmode=disable`
- Cart: `postgres://dupli1:dupli1_dev@localhost:5436/cart?sslmode=disable`
- Payment: `postgres://dupli1:dupli1_dev@localhost:5437/payments?sslmode=disable`

Production uses **Amazon RDS** — see [docs/deployment-aws.md](docs/deployment-aws.md) and [infra/terraform/README.md](infra/terraform/README.md).

### Running locally

```bash
sudo service docker start        # daemon is not auto-started in the VM
cp .env.example .env             # set JWT_SECRET, OWNER_EMAIL, OWNER_PASSWORD
sudo docker compose up --build   # the ubuntu user is not in the docker group; use sudo
```

Seeded dev owner: `admin@dupli1.com` / `password`.

Gateway (HTTP): `http://localhost:8080` or `http://localhost` (port 80). TLS certs exist in `certs/` but are not wired into local nginx yet.

Direct service ports (bypass gateway):

| Service | Host port |
|---------|-----------|
| `dupli1-auth` | 18080 |
| `dupli1-product` | 8081 |
| `dupli1-order` | 8083 |
| `dupli1-cart` | 8086 |
| `dupli1-payment` | 8087 |
| `dupli1-notification` | 8084 |

### Running a single service (without Docker)

```bash
cd auth && go test ./...
cd product && go test ./...

# Auth server
cd auth/cmd && go run . -help

# Product server
cd product/cmd && go run . -help
```

### Testing

Run tests per service module (root `go test ./...` does not work — the root `go.mod` is a stub):

```bash
cd auth && go test ./...
cd product && go test ./...
cd order && go test ./...
cd cart && go test ./...
cd payment && go test ./...
```

### Gotchas

- **Module paths:** all services use `github.com/elug3/dupli1/<service>` (e.g. `github.com/elug3/dupli1/product`). There is no top-level `go.work`.
- **Auth token flow:** login returns only a `refresh_token`; call `POST /api/v1/auth/refresh` to obtain a short-lived access token in the `token` field.
- **Product JWT:** protected routes validate RS256 via `AUTH_JWKS_URL` (set in Compose to auth's JWKS endpoint).
- **Order JWT:** protected routes validate RS256 via `AUTH_JWKS_URL` (set in Compose to auth's JWKS endpoint), with `JWT_SECRET` HS256 fallback in dev.
- **Order, cart, and payment** use PostgreSQL when `DUPLI1_ORDER_DB` / `DUPLI1_CART_DB` / `DUPLI1_PAYMENT_DB` are set (Docker Compose); in-memory fallback for tests without a DB URL.
- **Payment flow:** `POST /api/v1/payments` → Stripe Checkout (or dev simulate URL). On success, payment publishes **`payment.succeeded`**; order service marks order **`paid`**. Ship via `POST /api/v1/orders/{id}/ship` → **`in_transit`** (commits stock). See [docs/payment-service.md](docs/payment-service.md).
- **Notification** subscribes to NATS (e.g. `order.paid` → Telegram when configured).
- **Redis and NATS** are optional for auth (rate limits, session cache, events); Redis and NATS are wired in Compose. Order and payment use NATS for payment events.
- **SMTP and OAuth providers** are external. **Stripe** is optional locally — without `STRIPE_SECRET_KEY`, payment uses a dev simulate endpoint. Simulate success via `GET /api/v1/payments/{id}/simulate-success` (returns the dev `checkout_url`).
- **Gateway 502s locally:** `api/nginx.conf` lists a production VPC resolver (`10.0.0.2`) alongside Docker DNS (`127.0.0.11`) and uses `proxy_pass $upstream` (runtime DNS). Locally `10.0.0.2` is unreachable, so the gateway (`http://localhost:8080`) returns intermittent `{"error":"bad gateway"}`. For reliable local end-to-end testing, hit the **direct service ports** listed above (auth 18080, product 8081, order 8083, cart 8086, payment 8087) instead of the gateway. Cross-service JWT validation still works because tokens are validated via the internal `AUTH_JWKS_URL`, regardless of which host port you call.
- **Product auto-ID on Postgres:** creating a product without an explicit `id` (`POST /api/v1/products`) currently fails with `generate product id: cannot convert N to Text` — `nextProductID` in `product/pkg/infra/pg` relies on `SUBSTRING(id FROM $1)`, which Postgres treats as a regex/text form so the integer arg fails to encode (the in-memory store used by tests does not exercise this path). Workaround for local flows: pass an explicit `"id"` (and variant `"sku"`) in the request body, which bypasses generation.
- **Order items require `sku_id`:** `POST /api/v1/orders` persists `order_items.sku_id NOT NULL`, so include each line's `sku_id` (the ULID returned by the product/variant and cart APIs), not just `sku`.
