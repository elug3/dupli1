# Current Code State

Authoritative snapshot of what is implemented in the Dupli1 repository today.

## Overview

Dupli1 is a fashion bag marketplace backend: Go microservices behind an nginx gateway. Local dev uses Docker Compose; production uses AWS ECS on EC2, ALB, and Amazon RDS PostgreSQL.

| Area | Status |
|------|--------|
| Auth (login, JWT, fine-grained permissions) | Implemented |
| Product catalog (bags, coupons, images, PDP) | Implemented |
| Inventory (stock, reservations) | Implemented (PostgreSQL, owned by product) |
| Orders + checkout sessions | Implemented (PostgreSQL) |
| Shopping cart | Implemented (PostgreSQL) |
| Payments (Stripe Checkout) | Implemented — see [payment-service.md](payment-service.md) |
| Notifications | Stub (health only) |
| User profiles, chat, analytics | Not started (guest cookie + unique PDP views planned — see [product-guest-views-plan.md](product-guest-views-plan.md)) |
| Manager settings (mutable store policy) | Sketch — see [manager-settings-api.md](manager-settings-api.md) |

## Repository layout

Services live in **per-service directories**, not `cmd/dupli1-*` / `pkg/*` at the repo root:

```text
auth/, product/, order/, cart/, payment/, notification/   # each has cmd/ + pkg/
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
  - Fine-grained **permissions** stored on users (`users.permissions TEXT[]`); JWT access tokens include `permissions` claim only
  - Permission constants and evaluation in `shared/pkg/permissions` (`github.com/elug3/dupli1/shared`)
  - Wildcards: `*`, `admin.*`, `{resource}.*` (e.g. `product.*`)
  - Account types: `customer`, `admin`, `service` (JSON field `account_type`; distinct from permissions)
  - Register requires `user.create` (not public); auth ABAC hierarchy governs who may manage whom
  - User admin at `/api/v1/auth/users`; update via `PATCH …/permissions`
  - Owner seeded from `OWNER_EMAIL` / `OWNER_PASSWORD` (`permissions: ["*"]`, `account_type` `admin`)
  - `dupli1-web` service account: `permissions: ["user.create"]` (`DUPLI1_WEB_SERVICE_*`)
  - `dupli1-order` service account: `order.ship`, `order.status.update`, `inventory.reservation.manage` (`DUPLI1_ORDER_SERVICE_*`)
  - Login/refresh rate-limited per IP via Redis
- **Tests:** `cd auth && go test ./...`

### dupli1-product

- **Host port:** 8081
- **Stack:** stdlib HTTP, PostgreSQL, MinIO/S3
- **Persistence:** `products` on `postgres-product`
- **Features:**
  - Parent (style) + variant (SKU) model: search returns parents only (no color duplicates)
  - Public: `GET /api/v1/products` (optional `product.read` widens view; query filters `category`, `brand`, `color`, `size`, `material`, `tags`), `GET /api/v1/products/{id}` (parent + variants), coupon redeem
  - Admin: per-route permissions (`product.create`, `coupon.read`, …) — see [permissions.md](permissions.md); parent CRUD, variant CRUD at `/api/v1/products/{id}/variants`, images on variant or default variant
  - Stock and reservations at `/api/v1/inventory/*` (merged in from the former standalone `inventory` service), keyed by a canonical ULID `SkuID` with `sku` and `by-sku-id/{skuId}` lookups both supported; reads are public, writes require `inventory.stock.write` or `inventory.reservation.manage`
  - Protected routes validate RS256 via `AUTH_JWKS_URL`; authorization from `permissions` claim
  - Inline schema migration + variant backfill on startup
  - Plan: [product-variants-plan.md](product-variants-plan.md)
- **Tests:** `cd product && go test ./...`

### dupli1-order

- **Host port:** 8083
- **Persistence:** PostgreSQL (`orders` on `postgres-order`)
- **Features:**
  - Checkout sessions at `/api/v1/checkout/sessions` (see [checkout-session.md](checkout-session.md))
  - Order lifecycle at `/api/v1/orders` — statuses: `pending`, `paid`, `in_transit`, `fulfilled`, `canceled`
  - Consumes **`payment.succeeded`** (NATS) → `paid`; 5-minute unpaid `pending` expiry worker
  - `POST /api/v1/orders/{id}/ship` → `in_transit` + commit inventory (plan B)
  - Calls product to reserve stock and redeem coupons
- **Auth:** Bearer JWT via `AUTH_JWKS_URL` (RS256 JWKS; HS256 fallback in dev). Storefront ABAC on `customer_id`; `order.create` / `order.read.all` bypass ABAC. Ship requires `order.ship`; status changes require `order.status.update`
- **Tests:** `cd order && go test ./...`

### dupli1-cart

- **Host port:** 8086
- **Persistence:** PostgreSQL (`cart` on `postgres-cart`)
- **Features:**
  - Persistent per-customer cart at `/api/v1/cart` (see [cart-service.md](cart-service.md))
  - Admin read at `/api/v1/carts/{customer_id}` requires `cart.read`
  - Enriches lines from product (price, images, availability)
- **Auth:** Bearer JWT via `AUTH_JWKS_URL` (RS256 JWKS from auth; access tokens only), with `JWT_SECRET` HS256 fallback in dev
- **Tests:** `cd cart && go test ./...`

### dupli1-payment

- **Host port:** 8087
- **Persistence:** PostgreSQL (`payments` on `postgres-payment`)
- **Features:**
  - Stripe Checkout redirect at `POST /api/v1/payments` (see [payment-service.md](payment-service.md))
  - Dev mode without `STRIPE_SECRET_KEY`: simulate URL `GET /api/v1/payments/{id}/simulate-success`
  - Publishes **`payment.succeeded`** on NATS when payment completes
- **Auth:** Bearer JWT on customer routes; ownership ABAC unless `payment.create` / `payment.read.all`. Stripe signature on webhook
- **Tests:** `cd payment && go test ./...`

### dupli1-notification

- **Host port:** 8084
- **Features:** NATS subscriber; Telegram alerts on `order.paid` (when `TELEGRAM_*` configured)
- **Status:** Health + event dispatch (no outbound email/SMS yet)

### dupli1-proxy

- **Host ports:** 8080 and 80 (HTTP), 443 exposed but TLS not configured in nginx
- **Config:** [api/nginx.conf](../api/nginx.conf)
- **Health:** `GET /gateway/health` → `ok`

## Data stores

| Store | Used by | Local |
|-------|---------|-------|
| PostgreSQL `dupli1_db` | auth | `postgres-auth:5432` |
| PostgreSQL `products` | product (also stock/reservations) | `postgres-product:5433` |
| PostgreSQL `orders` | order | `postgres-order:5435` |
| PostgreSQL `cart` | cart | `postgres-cart:5436` |
| PostgreSQL `payments` | payment | `postgres-payment:5437` |
| MinIO `product-images` | product | `minio:9000` |
| Redis | auth | `redis:6379` (in Compose) |
| NATS | auth, order, payment, notification | `nats:4222` (in Compose) |

## API surface (summary)

| Service | Public | Authenticated |
|---------|--------|---------------|
| auth | login, refresh, logout, JWKS | register (`user.create`), me, user admin (permissions) |
| product | health, product search/PDP, coupon redeem, inventory reads | product/coupon CRUD (per permission), image upload, inventory writes (`inventory.stock.write`, `inventory.reservation.manage`) |
| order | health only | orders, checkout (ABAC + permissions), ship (`order.ship`) |
| cart | health only | own cart; admin read (`cart.read`) |
| payment | health, dev simulate | payments (ABAC + permissions) |
| notification | health | — |

Full reference: [api.md](api.md). Route index: [endpoints.md](endpoints.md). Permission spec: [permissions.md](permissions.md).

## Go modules

| Module | Path |
|--------|------|
| `github.com/elug3/dupli1` | root stub |
| `github.com/elug3/dupli1/auth` | `auth/` |
| `github.com/elug3/dupli1/product` | `product/` |
| `github.com/elug3/dupli1/order` | `order/` |
| `github.com/elug3/dupli1/cart` | `cart/` |
| `github.com/elug3/dupli1/payment` | `payment/` |
| `github.com/elug3/dupli1/notification` | `notification/` |
| `github.com/elug3/dupli1/shared` | `shared/` (permissions library) |

## Known gaps

1. **Local TLS** — certs in `certs/` are not wired into nginx; gateway is HTTP only
2. **Notification** — no outbound messaging
3. **No migrations directory** — product migrates inline; auth uses bootstrap DDL
4. **Planned packages not started** — user, chat, analytics (beyond `shared/pkg/permissions`)

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

Production: ECS on EC2, ALB, RDS PostgreSQL 16, S3, Secrets Manager. See [deployment-aws.md](deployment-aws.md).
