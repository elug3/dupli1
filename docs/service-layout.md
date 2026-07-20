# Services Layout

How the Dupli1 Go backend is organized, what each service owns, and where to add new code.

## Directory map

```text
dupli1/
├── auth/                     # Auth service module
│   ├── cmd/                  # CLI entrypoint (cobra)
│   └── pkg/                  # domain, service, ports, infra, handler, bootstrap
├── product/                  # Product catalog module (also owns stock/reservations)
│   ├── cmd/
│   └── pkg/
├── order/
│   ├── cmd/
│   └── pkg/
├── cart/
│   ├── cmd/
│   └── pkg/
├── notification/
│   ├── cmd/
│   └── pkg/
├── shared/                   # Reusable Go modules (permissions, …)
│   ├── go.mod
│   └── pkg/
├── api/
│   ├── nginx.conf            # Gateway routing (dupli1-proxy image)
│   └── Dockerfile
├── infra/
│   ├── terraform/
│   └── scripts/
├── certs/                    # Self-signed TLS material (not wired in local nginx yet)
├── Dockerfile                # Multi-service image (SERVICE build arg)
├── docker-compose.yml
├── go.mod                    # Workspace stub module
└── docs/
```

Each service is an independent Go module (`auth/go.mod`, `product/go.mod`, …). The `shared/` directory is also its own module (`shared/go.mod`). There is no top-level `go.work` file; run and test from each module directory.

### Shared (`shared/pkg`)

**Module:** `github.com/elug3/dupli1/shared`

Cross-service libraries with no service-specific dependencies. Services import via `go get` (local dev: `go mod edit -replace github.com/elug3/dupli1/shared=../shared`).

| Package | Purpose |
|---------|---------|
| `permissions` | Fine-grained permission constants, `Has` / `HasAny`, legacy role expansion, bundles — see [permissions.md](permissions.md) |
| `settings` | Shared non-secret `GET /settings` response helpers used by all services |

```bash
cd shared && go test ./...
```

## Layer responsibilities

Every service package follows hexagonal architecture ([ARCHITECTURE.md](../ARCHITECTURE.md)):

1. **Entrypoints (`<service>/cmd/`)** — parse flags/env and start the HTTP server.
2. **Bootstrap (`<service>/pkg/bootstrap/`)** — wire DB clients, repositories, services, handlers, routes.
3. **Handlers (`handler/`)** — HTTP translation only.
4. **Services (`service/`)** — use cases; depend on `domain/` and `ports/` only.
5. **Ports (`ports/`)** — interfaces for repositories and external clients.
6. **Infra (`infra/`)** — Postgres, Redis, S3, HTTP clients, in-memory fakes.
7. **Domain (`domain/`)** — entities and business rules.

Configuration lives in `bootstrap/config.go` and/or package `options.go`.

## Service packages

### Auth (`auth/pkg`)

**Module:** `github.com/elug3/dupli1/auth`  
**Framework:** Gin  
**Storage:** PostgreSQL (required), Redis (rate limits + session cache in Compose), NATS (optional events)

Owns:

- Login, logout, refresh, RS256 JWT + JWKS
- Fine-grained **permissions** on users and in JWT `permissions` claim
- Account types: `customer`, `admin`, `service` on `User.AccountType` / JSON `account_type`
- User admin at `/api/v1/auth/users` (not `/api/v1/users`); `PATCH …/permissions` canonical
- Login lockout (5 failed attempts) skips **admin** and **owner**; see [permissions.md](permissions.md)
- Owner seeding via `OWNER_EMAIL` / `OWNER_PASSWORD` (`permissions: ["*"]`)
- Service account seeding via `DUPLI1_WEB_SERVICE_*` (`permissions: ["user.create"]`)

### Product (`product/pkg`)

**Module:** `github.com/elug3/dupli1/product`  
**Framework:** stdlib `net/http`  
**Storage:** PostgreSQL (`products` table, plus stock/reservations tables), MinIO/S3 (images)

Owns:

- Parent styles + variants (SKUs, each with a canonical ULID `SkuID`): search returns parents only; PDP embeds variants
- Admin product/variant/coupon CRUD; brand-prefixed parent IDs (`BOT-001`); images on variants
- Stock and reservations at `/api/v1/inventory/*` (merged in from the former standalone `inventory` service — same routes, keyed by `SkuID` internally, `sku` and `by-sku-id/{skuId}` lookups both supported). Public reads; writes require `inventory.stock.write` or `inventory.reservation.manage`
- JWT validation via `AUTH_JWKS_URL` (RS256 JWKS); per-route permission checks (`product.create`, `coupon.read`, …)

### Order (`order/pkg`)

**Module:** `github.com/elug3/dupli1/order`  
**Storage:** PostgreSQL (`orders` table set), in-memory fallback when no DB URL is configured (tests)

Owns orders and checkout sessions at `/api/v1/orders` and `/api/v1/checkout/sessions`. Requires Bearer JWT when `AUTH_JWKS_URL` or `JWT_SECRET` is set (RS256 JWKS from auth; access tokens only).

### Cart (`cart/pkg`)

**Module:** `github.com/elug3/dupli1/cart`  
**Storage:** PostgreSQL (`cart` table set), in-memory fallback when no DB URL is configured (tests)

Owns shopping carts at `/api/v1/cart` (current user) and `/api/v1/carts/{customer_id}` (admin read). Requires Bearer JWT when `AUTH_JWKS_URL` or `JWT_SECRET` is set. See [cart-service.md](cart-service.md).

### Payment (`payment/pkg`)

**Module:** `github.com/elug3/dupli1/payment`  
**Storage:** PostgreSQL (`payments` table set), in-memory fallback when no DB URL is configured (tests)

Stripe Checkout redirect; publishes `payment.succeeded` on NATS. See [payment-service.md](payment-service.md).

### Notification (`notification/pkg`)

**Module:** `github.com/elug3/dupli1/notification`  
**Status:** Health endpoint only

## Gateway routing

`dupli1-proxy` uses [api/nginx.conf](../api/nginx.conf). Local gateway: **HTTP** on port **8080** (also mapped to host port 80).

| Path prefix | Backend |
|-------------|---------|
| `/gateway/health` | nginx (static `ok`) |
| `/api/v1/auth/` | dupli1-auth |
| `/api/v1/products` | dupli1-product (canonical; also covers `/products/variants`, `/products/coupons`, `/products/catalog`, `/products/inventory`) |
| `/api/v1/coupons` | dupli1-product (legacy alias) |
| `/api/v1/inventory/` | dupli1-product (legacy alias) |
| `/api/v1/orders` | dupli1-order (canonical; also covers `/orders/checkout`) |
| `/api/v1/checkout` | dupli1-order (legacy alias) |
| `/api/v1/cart` | dupli1-cart (canonical; also covers `/cart/customers`) |
| `/api/v1/carts/` | dupli1-cart (legacy alias) |
| `/api/v1/variants` | dupli1-product (legacy alias) |

Checkout sessions: canonical `/api/v1/orders/checkout/sessions` (legacy `/api/v1/checkout/sessions`). Cart admin: canonical `/api/v1/cart/customers/{id}` (legacy `/api/v1/carts/{id}`). Path migration checklist: [TODO.md](TODO.md).

Direct host ports (bypass gateway): auth **18080**, product **8081**, order **8083**, cart **8086**, notification **8084**.

## Adding a new service

1. Create `<service>/pkg/` with `domain/`, `service/`, `ports/`, `infra/`, `handler/`, `bootstrap/`, and `server.go`.
2. Add `<service>/go.mod` and ensure the root build includes `SERVICE=<service>` in [Dockerfile](../Dockerfile).
3. Add `<service>/cmd/main.go`.
4. Add the service to `docker-compose.yml`.
5. Add nginx `location` blocks in [api/nginx.conf](../api/nginx.conf).
6. Update [docs/api.md](api.md), [docs/current-state.md](current-state.md), and [README.md](../README.md).

## Testing

```bash
cd auth && go test ./...
cd product && go test ./...
cd order && go test ./...
cd cart && go test ./...
```

Root `go test ./...` does not work — the root `go.mod` is a stub.
