# Current Code State

Authoritative snapshot of what is implemented in the Dupli1 repository today.

## Overview

Dupli1 is a fashion bag marketplace backend: Go microservices behind an nginx gateway. Local dev uses Docker Compose; production uses AWS ECS Fargate and Amazon RDS PostgreSQL.

| Area | Status |
|------|--------|
| Auth (login, JWT, RBAC) | Implemented |
| Product catalog (bags, coupons, images, PDP) | Implemented |
| Inventory (stock, reservations) | Implemented (in-memory) |
| Orders + checkout sessions | Implemented (in-memory) |
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
  - Roles: `owner`, `admin`, `user_manager`, `customer_registrar`, `product_manager`, `customer`
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
  - Public: `GET /api/v1/products/bags`, `GET /api/v1/products/{id}`, coupon redeem
  - Admin CRUD at `/api/v1/products`, `GET /api/v1/products/{id}/manage`
  - Bag search reads `products` where `category = 'bags'` and `status = 'active'`
  - Protected routes validate RS256 via `AUTH_JWKS_URL` and require `product_manager`, `admin`, or `owner` role
  - Inline schema migration on startup
- **Tests:** `cd product && go test ./...`

### dupli1-inventory

- **Host port:** 8082
- **Persistence:** In-memory
- **Features:** Stock and reservations at `/api/v1/inventory/*`
- **Auth:** None
- **Tests:** `cd inventory && go test ./...`

### dupli1-order

- **Host port:** 8083
- **Persistence:** In-memory
- **Features:**
  - Checkout sessions at `/api/v1/checkout/sessions` (see [checkout-session.md](checkout-session.md))
  - Order lifecycle at `/api/v1/orders`
  - Calls inventory to reserve stock; calls product to redeem coupons
- **Auth:** Bearer JWT when `JWT_SECRET` is set (HMAC validator — **not aligned with auth RS256 yet**)
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
| MinIO `product-images` | product | `minio:9000` |
| In-memory | inventory, order | process-local |
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

1. **Order JWT** — HMAC `JWT_SECRET` validator; does not consume auth JWKS/RS256 tokens
2. **Local TLS** — certs in `certs/` are not wired into nginx; gateway is HTTP only
3. **Inventory/order persistence** — in-memory; data lost on restart
4. **Notification** — no outbound messaging
5. **No migrations directory** — product migrates inline; auth uses bootstrap DDL
6. **Planned packages not started** — user, chat, analytics, shared lib

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
