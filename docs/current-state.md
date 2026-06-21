# Current Code State

Snapshot of the Schick repository as of the latest implementation. Use this document to understand what exists today versus what is planned.

## Overview

Schick is a fashion bag marketplace backend implemented as Go microservices behind an nginx reverse proxy. Local development uses Docker Compose; production deploys to AWS ECS Fargate with Amazon RDS PostgreSQL.

| Area | Status |
|------|--------|
| Auth (login, JWT, RBAC) | Implemented |
| Product catalog (bags, coupons, images) | Implemented |
| Inventory (stock, reservations) | Implemented (in-memory) |
| Orders (checkout, lifecycle) | Implemented (in-memory) |
| Notifications | Stub (health only) |
| User profiles, chat, analytics | Not started |

## Services

### schick-auth (port 8080)

- **Stack:** Gin, PostgreSQL, optional Redis/NATS
- **Persistence:** `schick_db` on `postgres-auth` (port 5432)
- **Features:**
  - Register, login, logout, refresh tokens
  - Access tokens (15 min) + refresh tokens (24 h)
  - Roles: `owner`, `admin`, `user` (stored in DB, not in JWT)
  - Admin user CRUD at `/api/v1/users`
  - Owner seeded on first startup from `OWNER_EMAIL` / `OWNER_PASSWORD`
- **Tests:** `pkg/auth` — bootstrap, handler, service, postgres

### schick-product (port 8081)

- **Stack:** stdlib HTTP, PostgreSQL, MinIO/S3
- **Persistence:** `products` on `postgres-product` (port 5433)
- **Features:**
  - Public bag search with brand/color/material filters
  - Product CRUD (auth required) with brand-prefixed IDs
  - Coupon CRUD and public redemption
  - Image upload to MinIO (auth required)
  - Schema auto-migration on startup
- **Tests:** `pkg/product` — handler, service (root package)

### schick-inventory (port 8082)

- **Stack:** stdlib HTTP
- **Persistence:** In-memory only
- **Features:**
  - Get/set/adjust stock by SKU
  - Reservation create, commit, release
- **Tests:** `pkg/inventory` — service

### schick-order (port 8083)

- **Stack:** stdlib HTTP, HTTP client to inventory
- **Persistence:** In-memory only
- **Features:**
  - Create order (reserves inventory)
  - Confirm, cancel, fulfill status transitions
  - List orders by customer ID
- **Tests:** `pkg/order` — service

### schick-notification (port 8084)

- **Stack:** stdlib HTTP
- **Status:** Health endpoint only; no outbound messaging

### schick-proxy (ports 80/443)

- **Stack:** nginx
- **Routes:** auth, product, inventory, order (see [service-layout.md](service-layout.md))
- **TLS:** Self-signed cert in `certs/` for local dev; ALB terminates TLS in production

## Data Stores

| Store | Used by | Local | Production |
|-------|---------|-------|------------|
| PostgreSQL `schick_db` | auth | `postgres-auth:5432` | RDS |
| PostgreSQL `products` | product | `postgres-product:5433` | RDS |
| MinIO `product-images` | product | `minio:9000` | S3 (planned) |
| In-memory | inventory, order | process-local | process-local |
| Redis | auth (optional) | not in compose | optional |
| NATS | auth, product (optional) | not in compose | optional |

## Go Workspace

The repo uses `go.work` to link independent modules:

```
cmd/schick-auth, cmd/schick-inventory, cmd/schick-notification,
cmd/schick-order, cmd/schick-product,
pkg/auth, pkg/inventory, pkg/notification, pkg/order, pkg/product
```

Module paths are not unified:

- `github.com/elug3/schick/pkg/auth` (and inventory, order, notification cmd modules)
- `github.com/schick/pkg/product`

Root `go.mod` (`github.com/elug3/schick`) is a workspace stub. Run tests per package directory.

## API Surface

See [api.md](api.md) for the full reference. Summary:

| Service | Public endpoints | Authenticated endpoints |
|---------|-----------------|------------------------|
| auth | register, login, refresh, logout | me, user admin |
| product | health, bag search, coupon redeem | product CRUD, coupon CRUD, image upload |
| inventory | all endpoints | none |
| order | all endpoints | none |
| notification | health | none |

## Architecture Compliance

Services follow hexagonal/DDD layout per [ARCHITECTURE.md](../ARCHITECTURE.md):

- `domain/`, `service/`, `ports/`, `infra/`, `handler/`, `bootstrap/` present in auth, product, inventory, order
- Config in `bootstrap/config.go` or root `options.go` (not top-level `config.go`)
- Errors in `autherrors/` (auth only); other services inline errors
- No `pkg/shared/` yet

## Known Gaps

1. **Notification** — no email/push/SMS implementation
2. **Inventory/order persistence** — in-memory only; data lost on restart
3. **Module path inconsistency** — `elug3/schick` vs `schick/pkg/product`
4. **No auth on inventory/order** — endpoints are open
5. **No migrations directory** — product schema migrates inline; auth uses bootstrap DDL
6. **Planned packages not started** — user, chat, analytics, config

## Running and Testing

```bash
# Full stack
cp .env.example .env
docker compose up --build

# Tests (per module)
cd pkg/auth && go test ./...
cd pkg/product && go test ./...
cd pkg/inventory && go test ./...
cd pkg/order && go test ./...
```

## Deployment

Production: ECS Fargate in `us-east-1`, RDS PostgreSQL 16, Secrets Manager for credentials. See [deployment-aws.md](deployment-aws.md).
