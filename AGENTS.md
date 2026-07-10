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

### Prerequisites (pre-installed on the VM)

| Tool | Version / notes |
|------|-----------------|
| Go | 1.22+ (`/usr/bin/go`) — modules target Go 1.26 |
| PostgreSQL | 16 — `localhost:5432` |
| Redis | 7 — `localhost:6379` |

### Starting infrastructure services

For tests or non-Docker workflows, start system services:

```bash
sudo service postgresql start
sudo service redis-server start
pg_isready -h localhost
redis-cli ping
```

Local development normally uses Docker Compose Postgres containers instead (see below).

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
cp .env.example .env   # set JWT_SECRET, OWNER_EMAIL, OWNER_PASSWORD
docker compose up --build
```

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
- **SMTP and OAuth providers** are external. **Stripe** is optional locally — without `STRIPE_SECRET_KEY`, payment uses a dev simulate endpoint.
