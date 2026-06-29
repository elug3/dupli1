# Services Layout

This document describes how Schick's microservice Go backend is organized, what each service owns, and how new service code should be placed.

## Directory Map

```text
schick/
├── cmd/
│   ├── schick-auth/          # Auth server entrypoint + Dockerfile
│   ├── schick-product/       # Product server entrypoint + Dockerfile
│   ├── schick-inventory/     # Inventory server entrypoint + Dockerfile
│   ├── schick-order/         # Order server entrypoint + Dockerfile
│   ├── schick-notification/  # Notification server entrypoint + Dockerfile
│   └── schick-proxy/         # nginx reverse proxy
├── pkg/
│   ├── auth/                 # Identity, tokens, RBAC
│   ├── product/              # Bag catalog, coupons, images
│   ├── inventory/            # Stock and reservations
│   ├── order/                # Order lifecycle
│   └── notification/         # Outbound messaging (stub)
├── docs/                     # API, deployment, and state documentation
├── infra/
│   ├── terraform/            # RDS, secrets, IAM
│   └── scripts/              # RDS cutover helpers
├── docker-compose.yml        # Local development stack
├── go.work                   # Go workspace linking all modules
└── .env.example              # Local environment template
```

There is no monolithic `cmd/server` or shared `internal/` package. Each service is an independent Go module wired through `go.work`.

## Layer Responsibilities

Every service package follows hexagonal architecture (see [ARCHITECTURE.md](../ARCHITECTURE.md)):

1. **Entrypoints (`cmd/schick-*`)** parse flags/env, construct `ServerOptions`, and call `pkg/<service>.NewServer()`.
2. **Bootstrap (`pkg/<service>/bootstrap/`)** wires database clients, repositories, services, handlers, and routes.
3. **Handlers (`handler/`)** translate HTTP requests into service calls. No business logic.
4. **Services (`service/`)** contain use cases and domain rules. Depend only on `domain/` and `ports/`.
5. **Ports (`ports/`)** define interfaces for repositories, external clients, and event publishers.
6. **Infra (`infra/`)** implements ports (Postgres, Redis, S3, in-memory, HTTP clients).
7. **Domain (`domain/`)** holds entities and value objects with no external dependencies.

Configuration lives in `bootstrap/config.go` (or `options.go` at the package root), not in a separate top-level `config.go` as the architecture guide originally specified.

## Service Packages

### Auth (`pkg/auth`)

**Module:** `github.com/elug3/schick/pkg/auth`  
**Framework:** Gin  
**Storage:** PostgreSQL (required), Redis (optional session cache), NATS (optional events)

Owns:

- Registration, login, logout, token refresh
- JWT access/refresh token pair
- Role-based access control (`owner`, `admin`, `user`)
- Admin user management at `/api/v1/users`
- Owner seeding on first startup via `OWNER_EMAIL` / `OWNER_PASSWORD`

### Product (`pkg/product`)

**Module:** `github.com/schick/pkg/product`  
**Framework:** stdlib `net/http`  
**Storage:** PostgreSQL (products), MinIO/S3 (images)

Owns:

- Bag search (`GET /api/v1/products/bags`)
- Product CRUD with brand-prefixed IDs (`BOT-001`)
- Coupon management and redemption
- Image upload (multipart, appends to `imageUrls`)
- JWT middleware for protected routes (validates access tokens only)

### Inventory (`pkg/inventory`)

**Module:** `github.com/elug3/schick/pkg/inventory`  
**Framework:** stdlib `net/http`  
**Storage:** In-memory (no persistence yet)

Owns:

- Per-SKU stock get/set/adjust
- Reservation create/commit/release

### Order (`pkg/order`)

**Module:** `github.com/elug3/schick/pkg/order`  
**Framework:** stdlib `net/http`  
**Storage:** In-memory (no persistence yet)

Owns:

- Order creation (calls inventory to reserve stock)
- Order status transitions (confirmed, canceled, fulfilled)
- Customer order listing

### Notification (`pkg/notification`)

**Module:** `github.com/elug3/schick/pkg/notification`  
**Status:** Stub — health endpoint only

Planned: email, push, SMS, and event-triggered notifications.

## Gateway Routing

`schick-proxy` (nginx) routes external traffic:

| Path prefix | Backend |
|-------------|---------|
| `/gateway/health` | nginx (local response) |
| `/health` | auth |
| `/api/v1/auth/` | auth |
| `/api/v1/users` | auth |
| `/api/v1/products/` | product |
| `/api/v1/coupons/` | product |
| `/api/v1/inventory/` | inventory |
| `/api/v1/orders` | order |

Inventory, order, and notification are also reachable on their direct ports (8082–8084) when running via Docker Compose.

## Dependency Direction

```text
cmd/schick-<service>
  -> pkg/<service>/server.go
      -> bootstrap/
          -> handler/ -> service/ -> ports/
                              -> domain/
          -> infra/ (implements ports)
```

Rules:

- Handlers may depend on services; services must not depend on handlers.
- Services may depend on ports and domain only.
- Infra implements ports; it must not contain business logic.
- Cross-service calls go through HTTP client adapters in `infra/` (e.g. order → inventory).

## Adding a New Service

1. Create `pkg/<service>/` with `domain/`, `service/`, `ports/`, `infra/`, `handler/`, `bootstrap/`, and `server.go`.
2. Add `go.mod` for the package and include it in `go.work`.
3. Create `cmd/schick-<service>/main.go` and `Dockerfile`.
4. Add the service to `docker-compose.yml`.
5. Add nginx location blocks in `cmd/schick-proxy/nginx.conf` and `nginx-alb.conf`.
6. Update `docs/api.md`, `docs/current-state.md`, and `README.md`.

## Testing Expectations

- Unit test service business rules with fake repositories.
- Test handlers with `httptest` and in-memory stores.
- Run tests per module: `cd pkg/<service> && go test ./...`
- Root `go test ./...` does not work because the root `go.mod` is a workspace stub.

## Not Yet Implemented

The following packages described in early design docs do not exist:

- `pkg/user`, `pkg/chat`, `pkg/analytics`, `pkg/config`
- `cmd/server`, `cmd/migrate`, `internal/`, `migrations/`
- `pkg/shared/`
