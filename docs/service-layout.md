# Services Layout

How the Dupli1 Go backend is organized, what each service owns, and where to add new code.

## Directory map

```text
dupli1/
├── auth/                     # Auth service module
│   ├── cmd/                  # CLI entrypoint (cobra)
│   └── pkg/                  # domain, service, ports, infra, handler, bootstrap
├── product/                  # Product catalog module
│   ├── cmd/
│   └── pkg/
├── inventory/
│   ├── cmd/
│   └── pkg/
├── order/
│   ├── cmd/
│   └── pkg/
├── notification/
│   ├── cmd/
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

Each service is an independent Go module (`auth/go.mod`, `product/go.mod`, …). There is no top-level `go.work` file; run and test from each service directory.

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
- RBAC roles: `owner`, `admin`, `user_manager`, `customer_registrar`, `product_manager`, `customer`
- Account types: `customer`, `admin`, `service` on `User.AccountType` / JSON `account_type`
- User admin at `/api/v1/auth/users` (not `/api/v1/users`)
- Owner seeding via `OWNER_EMAIL` / `OWNER_PASSWORD` (`account_type` `admin`)
- Service account seeding via `DUPLI1_WEB_SERVICE_EMAIL` / `DUPLI1_WEB_SERVICE_PASSWORD` (`account_type` `service`, role `customer_registrar`)

### Product (`product/pkg`)

**Module:** `github.com/elug3/dupli1/product`  
**Framework:** stdlib `net/http`  
**Storage:** PostgreSQL (`products` table), MinIO/S3 (images)

Owns:

- Parent styles + variants (SKUs): search returns parents only; PDP embeds variants
- Admin product/variant/coupon CRUD; brand-prefixed parent IDs (`BOT-001`); images on variants
- JWT validation via `AUTH_JWKS_URL` (RS256 JWKS from auth; access tokens only)

### Inventory (`inventory/pkg`)

**Module:** `github.com/elug3/dupli1/inventory`  
**Storage:** PostgreSQL (`inventory` table set), in-memory fallback when no DB URL is configured (tests)

Owns stock and reservations at `/api/v1/inventory/*`. Public reads; writes require Bearer JWT with `order_manager`, `admin`, or `owner` when auth is configured.

### Order (`order/pkg`)

**Module:** `github.com/elug3/dupli1/order`  
**Storage:** PostgreSQL (`orders` table set), in-memory fallback when no DB URL is configured (tests)

Owns orders and checkout sessions at `/api/v1/orders` and `/api/v1/checkout/sessions`. Requires Bearer JWT when `AUTH_JWKS_URL` or `JWT_SECRET` is set (RS256 JWKS from auth; access tokens only).

### Notification (`notification/pkg`)

**Module:** `github.com/elug3/dupli1/notification`  
**Status:** Health endpoint only

## Gateway routing

`dupli1-proxy` uses [api/nginx.conf](../api/nginx.conf). Local gateway: **HTTP** on port **8080** (also mapped to host port 80).

| Path prefix | Backend |
|-------------|---------|
| `/gateway/health` | nginx (static `ok`) |
| `/api/v1/auth/` | dupli1-auth |
| `/api/v1/products` | dupli1-product |
| `/api/v1/coupons` | dupli1-product |
| `/api/v1/inventory/` | dupli1-inventory |
| `/api/v1/orders` | dupli1-order |
| `/api/v1/checkout` | dupli1-order |

Checkout sessions are served by order (`/api/v1/checkout/sessions`).

Direct host ports (bypass gateway): auth **18080**, product **8081**, inventory **8082**, order **8083**, notification **8084**.

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
cd inventory && go test ./...
cd order && go test ./...
```

Root `go test ./...` does not work — the root `go.mod` is a stub.
