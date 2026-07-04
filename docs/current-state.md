# Current Code State

Authoritative snapshot of what is implemented in the Dupli1 repository today.

## Overview

Dupli1 is a fashion bag marketplace backend: Go microservices behind an nginx gateway. Local dev uses Docker Compose; production uses AWS ECS Fargate and Amazon RDS PostgreSQL.

| Area | Status |
|------|--------|
| Auth (login, JWT, RBAC) | Implemented |
| Product catalog (bags, coupons, images, PDP) | Implemented |
| Inventory (stock, reservations) | Implemented (PostgreSQL) |
| Orders + checkout sessions | Implemented (PostgreSQL) |
| Notifications | Stub (health only) |
| User profiles, chat, analytics | Not started |

## Repository layout

Services live in **per-service directories**, not `cmd/dupli1-*` / `pkg/*` at the repo root:

```text
auth/, product/, inventory/, order/, notification/   # each has cmd/ + pkg/
api/nginx.conf                                      # gateway
```

See [service-layout.md](service-layout.md) for details.

## Services

### dupli1-auth

- **Host port (Compose):** 18080 → container 8080
- **Stack:** Gin, PostgreSQL, Redis, optional NATS
- **Persistence:** `dupli1_db` on `postgres-auth`
- **Features:**
  - Login returns a **refresh token**; `POST /refresh` returns a short-lived **access token** (`token` field)
  - RS256 JWT + JWKS at `/api/v1/auth/.well-known/jwks.json`
  - Access tokens include `type: "access"`; refresh tokens include `type: "refresh"`
  - Roles: `owner`, `admin`, `user_manager`, `customer_registrar`, `product_manager`, `order_manager`, `customer`
  - Register requires `admin`, `user_manager`, or `customer_registrar` (not public)
  - User admin at `/api/v1/auth/users`
  - Owner seeded from `OWNER_EMAIL` / `OWNER_PASSWORD`
  - `dupli1-web` service account seeded from `DUPLI1_WEB_SERVICE_EMAIL` / `DUPLI1_WEB_SERVICE_PASSWORD`
  - Login/refresh rate-limited per IP via Redis
- **Tests:** `cd auth && go test ./...`

### dupli1-product

- **Host port:** 8081
- **Stack:** stdlib HTTP, PostgreSQL, MinIO/S3
- **Persistence:** `products` on `postgres-product`
- **Features:**
  - Parent (style) + variant (SKU) model: search returns parents only (no color duplicates)
  - Public: `GET /api/v1/products` (query filters), `GET /api/v1/products/{id}` (parent + variants), coupon redeem
  - Admin: parent CRUD, variant CRUD at `/api/v1/products/{id}/variants`, images on variant or default variant
  - Filters: `category`, `brand`, `color`, `size`, `material`, `tags` (color/size match any active variant)
  - Stock is per variant SKU in inventory (product `stock` is legacy)
  - Protected routes validate RS256 via `AUTH_JWKS_URL` and require `product_manager`, `admin`, or `owner` role
  - Inline schema migration + variant backfill on startup
  - Plan: [product-variants-plan.md](product-variants-plan.md)
- **Tests:** `cd product && go test ./...`

### dupli1-inventory

- **Host port:** 8082
- **Persistence:** PostgreSQL (`inventory` on `postgres-inventory`)
- **Features:** Stock and reservations at `/api/v1/inventory/*`
- **Auth:** None on reads; writes require Bearer JWT (`order_manager`, `admin`, or `owner`) when `AUTH_JWKS_URL` is set
- **Tests:** `cd inventory && go test ./...`

### dupli1-order

- **Host port:** 8083
- **Persistence:** PostgreSQL (`orders` on `postgres-order`)
- **Features:**
  - Checkout sessions at `/api/v1/checkout/sessions` (see [checkout-session.md](checkout-session.md))
  - Order lifecycle at `/api/v1/orders`
  - Calls inventory to reserve stock; calls product to redeem coupons
- **Auth:** Bearer JWT validated via `AUTH_JWKS_URL` (RS256 JWKS from auth; access tokens only), with `JWT_SECRET` HS256 fallback in dev
- **Tests:** `cd order && go test ./...`

### dupli1-notification

- **Host port:** 8084
- **Status:** `GET /health` only

### dupli1-proxy

- **Host ports:** 8080 and 80 (HTTP), 443 exposed but TLS not configured in nginx
- **Config:** [api/nginx.conf](../api/nginx.conf)
- **Health:** `GET /gateway/health` → `ok`

## Data stores

| Store | Used by | Local |
|-------|---------|-------|
| PostgreSQL `dupli1_db` | auth | `postgres-auth:5432` |
| PostgreSQL `products` | product | `postgres-product:5433` |
| PostgreSQL `inventory` | inventory | `postgres-inventory:5434` |
| PostgreSQL `orders` | order | `postgres-order:5435` |
| MinIO `product-images` | product | `minio:9000` |
| Redis | auth | `redis:6379` (in Compose) |
| NATS | auth (optional) | `nats:4222` (in Compose) |

## API surface (summary)

| Service | Public | Authenticated |
|---------|--------|---------------|
| auth | login, refresh, logout | register (admin/user_manager), me, user admin |
| product | health, bag search, PDP, coupon redeem | product/coupon CRUD, image upload |
| inventory | all routes | — |
| order | health only | orders, checkout (when JWT configured) |
| notification | health | — |

Full reference: [api.md](api.md). Route index: [endpoints.md](endpoints.md).

## Go modules

| Module | Path |
|--------|------|
| `github.com/elug3/dupli1` | root stub |
| `github.com/elug3/dupli1/auth` | `auth/` |
| `github.com/elug3/dupli1/product` | `product/` |
| `github.com/elug3/dupli1/inventory` | `inventory/` |
| `github.com/elug3/dupli1/order` | `order/` |
| `github.com/elug3/dupli1/notification` | `notification/` |

## Known gaps

1. **Local TLS** — certs in `certs/` are not wired into nginx; gateway is HTTP only
2. **Notification** — no outbound messaging
3. **No migrations directory** — product migrates inline; auth uses bootstrap DDL
4. **Planned packages not started** — user, chat, analytics, shared lib

## Running and testing

```bash
cp .env.example .env
docker compose up --build

# Gateway (HTTP)
curl http://localhost:8080/gateway/health

# Tests (per service directory)
cd auth && go test ./...
cd product && go test ./...
```

## Deployment

Production: ECS Fargate, RDS PostgreSQL 16, Secrets Manager. See [deployment-aws.md](deployment-aws.md).
