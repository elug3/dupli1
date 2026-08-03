# Dupli1

Go microservice backend for a fashion bag marketplace. Services behind an nginx proxy, wired with Docker Compose for local dev and deployed to AWS ECS on EC2 (ALB, RDS, S3, CloudWatch Logs) in production.

## Services

| Service | Local port | Description |
|---------|------------|-------------|
| `dupli1-auth` | 18080 | JWT login/refresh, RS256 tokens, JWKS, RBAC user admin |
| `dupli1-product` | 8081 | Bag catalog, coupons, product CRUD, image upload, stock and reservation APIs |
| `dupli1-order` | 8083 | Checkout sessions and order lifecycle (PostgreSQL) |
| `dupli1-cart` | 8086 | Shopping cart (PostgreSQL) |
| `dupli1-payment` | 8087 | Stripe Checkout + manager Bypass (PostgreSQL) |
| `dupli1-notification` | 8084 | NATS → Telegram ops alerts |
| `dupli1-proxy` | 8080 / 80 | nginx reverse proxy (HTTP locally) |
| `postgres-auth` | 5432 | Auth DB |
| `postgres-product` | 5433 | Product DB (also stock/reservations) |
| `postgres-order` | 5435 | Order DB |
| `postgres-cart` | 5436 | Cart DB |
| `postgres-payment` | 5437 | Payment DB |
| `redis` | 6379 | Rate limiter backing store |
| `minio` | 9000 / 9001 | S3-compatible image storage (console on 9001) |

## Running

### Local (Docker Compose)

```bash
cp .env.example .env   # set OWNER_EMAIL, OWNER_PASSWORD, JWT_SECRET
docker compose up --build
```

API gateway: `http://localhost:8080` (also mapped to host port 80).

```bash
curl http://localhost:8080/gateway/health
```

All services share a single root [Dockerfile](Dockerfile) built with a `SERVICE` build arg (e.g. `--build-arg SERVICE=auth`). Docker Compose sets this automatically.

MinIO bucket `product-images` is created automatically on first start.

### Against Amazon RDS (requires VPN)

Production databases live on **Amazon RDS** in a private subnet. To run auth/product locally against RDS:

```bash
# AWS credentials required (Secrets Manager read)
bash infra/scripts/fetch-rds-env.sh
docker compose -f docker-compose.yml -f docker-compose.rds.yml --env-file .env.rds up --build
```

See [docs/deployment-aws.md](docs/deployment-aws.md) for production ECS + RDS setup.

## Project Structure

```
dupli1/
├── auth/                 # Auth service (cmd/ + pkg/)
├── product/              # Product catalog (also stock/reservations)
├── order/                # Order + checkout
├── cart/                 # Shopping cart
├── payment/              # Stripe Checkout + Bypass payments
├── notification/         # NATS → Telegram alerts
├── api/
│   ├── nginx.conf        # Gateway routing
│   └── Dockerfile
├── infra/
│   ├── terraform/        # VPC, ECS/EC2, ALB, RDS, ECR, S3, CloudWatch
│   └── scripts/          # RDS cutover helpers
├── certs/                # TLS material (not wired into local nginx yet)
├── Dockerfile            # Multi-service build (SERVICE build arg)
└── docs/                 # API reference and deployment guides
```

Each service follows hexagonal architecture: `domain/`, `service/`, `ports/`, `infra/`, `handler/`, `bootstrap/`. See [ARCHITECTURE.md](ARCHITECTURE.md) and [docs/service-layout.md](docs/service-layout.md).

## API

Full reference: [docs/api.md](docs/api.md). Route index: [docs/endpoints.md](docs/endpoints.md). Permission matrix: [docs/permissions.md](docs/permissions.md).

**Path convention:** canonical routes are namespaced by service (`/api/v1/products/…`, `/api/v1/orders/…`, `/api/v1/cart/…`, `/api/v1/payments/…`). Legacy top-level aliases (`/api/v1/inventory`, `/api/v1/coupons`, `/api/v1/checkout`, `/api/v1/carts`, …) still work but are deprecated — see [docs/TODO.md](docs/TODO.md).

### Auth (`dupli1-auth` :18080)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/health` | — | Health check |
| GET | `/settings` | — | Non-secret service settings |
| GET | `/api/v1/auth/.well-known/jwks.json` | — | RS256 public key set |
| POST | `/api/v1/auth/login` | — | Login; returns refresh token |
| POST | `/api/v1/auth/refresh` | — | Exchange refresh token for access token |
| POST | `/api/v1/auth/logout` | — | Revoke refresh token |
| GET | `/api/v1/auth/me` | Bearer | Current user profile |
| POST | `/api/v1/auth/register` | open customer signup (`AUTH_OPEN_REGISTER`, default on) or `user.create` | Create user (`account_type`: `customer` \| `manager` \| `service`) |
| GET | `/api/v1/auth/users` | `user.read` | List users |
| PATCH | `/api/v1/auth/users/{id}/permissions` | `user.permissions.update` | Replace permissions / `account_type` |
| PATCH | `/api/v1/auth/users/{id}/password` | `user.password.update` | Set user password |
| PATCH | `/api/v1/auth/users/{id}/status` | `user.status.update` | Activate / deactivate user |

**Token flow:** `POST /login` returns `{ "refresh_token": "..." }`. Call `POST /refresh` with that token to get `{ "token": "<access_jwt>" }`. Send the access token as `Authorization: Bearer <token>` on protected routes.

Login and refresh are rate-limited per IP via Redis.

Tokens are signed with RS256. In dev, an ephemeral 2048-bit key is generated on startup when neither `JWT_PRIVATE_KEY` nor `JWT_PRIVATE_KEY_FILE` is set. Production injects `JWT_PRIVATE_KEY` from Secrets Manager (see [docs/deployment-aws.md](docs/deployment-aws.md)).

### Products (`dupli1-product` :8081)

**Public**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/products/health` | Health check |
| GET | `/api/v1/products/settings` | Non-secret service settings |
| GET | `/api/v1/products` | Search **parent styles** (`?category=`, `?brand=`, `?color=`, `?size=`, `?tags=`) |
| GET | `/api/v1/products/{id}` | PDP: parent + variants (colors/sizes/images per SKU) |
| POST | `/api/v1/products/coupons/redeem` | Redeem a coupon code (alias: `/api/v1/coupons/redeem`) |

**Requires `Authorization: Bearer <access_token>`** (validated via JWKS)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/products` | Manager search (all statuses) |
| POST | `/api/v1/products` | Create parent style |
| PUT | `/api/v1/products/{id}` | Update parent |
| DELETE | `/api/v1/products/{id}` | Delete parent (cascades variants) |
| POST | `/api/v1/products/{id}/images` | Upload image to default variant |
| POST | `/api/v1/products/{id}/variants` | Create variant (SKU) |
| PUT/DELETE | `/api/v1/products/{id}/variants/{sku}` | Update / delete variant |
| POST | `/api/v1/products/{id}/variants/{sku}/images` | Upload image for a variant |
| GET | `/api/v1/products/coupons` | List coupons |
| POST | `/api/v1/products/coupons` | Create coupon |
| PUT | `/api/v1/products/coupons/by-code/{code}` | Update coupon |
| DELETE | `/api/v1/products/coupons/by-code/{code}` | Delete coupon |

### Inventory (served by `dupli1-product` :8081)

Stock and reservations, merged into the product service. Each variant also has a
canonical ULID `skuId`; every route below has a `by-sku-id/{skuId}` sibling
(e.g. `GET /api/v1/inventory/by-sku-id/{skuId}`) alongside the `sku`-keyed form.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/inventory/health` | Health check |
| GET | `/api/v1/inventory/settings` | Non-secret product-service settings |
| GET | `/api/v1/inventory/{sku}` | Get stock for SKU |
| PUT | `/api/v1/inventory/{sku}` | Set stock quantity |
| POST | `/api/v1/inventory/{sku}/adjust` | Adjust stock by delta |
| POST | `/api/v1/inventory/reservations` | Reserve stock for an order |
| POST | `/api/v1/inventory/reservations/{id}/commit` | Commit reservation |
| POST | `/api/v1/inventory/reservations/{id}/release` | Release reservation |

### Orders (`dupli1-order` :8083)

Requires `Authorization: Bearer <access_token>` when `AUTH_JWKS_URL` or `JWT_SECRET` is set (RS256 via auth JWKS in Compose; `JWT_SECRET` is HS256 fallback in dev).

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/orders/health` | Health check |
| GET | `/api/v1/orders/settings` | Non-secret service settings |
| POST | `/api/v1/orders/checkout/sessions` | Create checkout session |
| GET | `/api/v1/orders/checkout/sessions/{id}` | Get session |
| PUT/POST/DELETE | `/api/v1/orders/checkout/sessions/{id}/items` | Manage cart items |
| POST | `/api/v1/orders/checkout/sessions/{id}/coupon` | Apply coupon |
| POST | `/api/v1/orders/checkout/sessions/{id}/complete` | Complete checkout |
| POST | `/api/v1/orders` | Create order directly |
| GET | `/api/v1/orders?customer_id=` | List customer orders |
| GET | `/api/v1/orders/{id}` | Get order |
| POST | `/api/v1/orders/{id}/ship` | `order.ship` — `paid` → `in_transit`, commit stock |
| PUT | `/api/v1/orders/{id}/status` | `order.status.update` — fulfill or cancel |

See [docs/checkout-session.md](docs/checkout-session.md) for the checkout flow. See [docs/cart-service.md](docs/cart-service.md) for the persistent cart. See [docs/payment-service.md](docs/payment-service.md) for payments (Stripe Checkout, manager Bypass, dev simulate).

### Cart (`dupli1-cart` :8086)

Requires `Authorization: Bearer <access_token>` when `AUTH_JWKS_URL` or `JWT_SECRET` is set.

Full design (boundaries vs inventory/order, data model, checkout handoff): [docs/cart-service.md](docs/cart-service.md).

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/cart/health` | Health check |
| GET | `/api/v1/cart/settings` | Non-secret service settings |
| GET | `/api/v1/cart` | Get my cart |
| DELETE | `/api/v1/cart` | Clear my cart |
| PUT | `/api/v1/cart/items` | Replace all items |
| POST | `/api/v1/cart/items` | Add or update one item |
| DELETE | `/api/v1/cart/items/{sku}` | Remove line |
| GET | `/api/v1/cart/customers/{customer_id}` | `cart.read` — admin read (alias: `/api/v1/carts/{customer_id}`) |

### Payment (`dupli1-payment` :8087)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/api/v1/payments` | Bearer | Start Checkout or Bypass (`method`: `credit_card` \| `bypass`) |
| GET | `/api/v1/payments/{id}` | Bearer | Payment status |
| GET | `/api/v1/payments/{id}/simulate-success` | Bearer | Dev only when `PAYMENT_ALLOW_DEV_SIMULATE=true` |
| POST | `/api/v1/payments/webhooks/stripe` | Stripe signature | Webhook handler |

Stripe Checkout **redirect** — card data is entered only on Stripe's hosted page. v1.0 launches **without Stripe** in production; managers mark orders paid via **Bypass** (`payment.bypass`). Unpaid `pending` orders auto-cancel after **5 minutes**. Full flow: [docs/payment-service.md](docs/payment-service.md).

### Product IDs and variants

Parent products have a **ULID** `id` (API primary key). Human-readable variant SKUs follow the master-code system (`Brand_Style_Color[_Edition]_Size`) — see [docs/product-sku-system.md](docs/product-sku-system.md). Each variant also exposes a canonical ULID `skuId` for cart, order, and inventory. Search returns parent styles only (no color duplicates). Checkout and inventory accept either human `sku` or `skuId`.

### Image Upload

```bash
# Preferred: image for a specific color/size variant
curl -X POST http://localhost:8080/api/v1/products/BOT-001/variants/BOT-001-GRN/images \
  -H "Authorization: Bearer $TOKEN" \
  -F "image=@photo.jpg"

# Legacy: appends to the default variant
curl -X POST http://localhost:8080/api/v1/products/BOT-001/images \
  -H "Authorization: Bearer $TOKEN" \
  -F "image=@photo.jpg"
```

## Environment Variables

### Auth service

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_URL` | — | Postgres connection string |
| `REDIS_URL` | — | Redis URL for rate limiting |
| `NATS_URL` | — | NATS URL (optional, for pub/sub) |
| `JWT_PRIVATE_KEY` | — | PEM-encoded RSA private key (RS256); preferred on ECS via Secrets Manager |
| `JWT_PRIVATE_KEY_FILE` | — | Path to PEM file (alternative to inline `JWT_PRIVATE_KEY`) |
| `JWT_KEY_ID` | `default` | `kid` value in the JWKS document |
| `AUTH_OPEN_REGISTER` | `true` | Temporary open customer signup; set `false` to require `user.create` again |
| `JWT_EXPIRATION` | `15m` | Access token lifetime |
| `DUPLI1_AUTH_ADDR` | `:8080` | Listen address |
| `OWNER_EMAIL` | — | Seed owner email (skips seeding if empty) |
| `OWNER_PASSWORD` | — | Seed owner password |
| `DUPLI1_WEB_SERVICE_EMAIL` | — | Seed dupli1-web service account email |
| `DUPLI1_WEB_SERVICE_PASSWORD` | — | Seed dupli1-web service account password |

### Product service

| Variable | Default | Description |
|----------|---------|-------------|
| `DUPLI1_PRODUCT_DB` | — | Postgres connection string |
| `AUTH_JWKS_URL` | — | JWKS URL for RS256 token validation (set in Compose) |
| `JWT_SECRET` | — | HS256 fallback when JWKS is unavailable |
| `SERVER_HOST` | `localhost` | Listen host |
| `SERVER_PORT` | `8080` | Listen port |
| `S3_ENDPOINT` | — | MinIO/S3 endpoint URL (uploads) |
| `S3_PUBLIC_ENDPOINT` | — | Browser base for `imageUrls` (Compose: `http://localhost:8080/product-images`; AWS: CloudFront / `images.dupli1.com`) |
| `S3_ACCESS_KEY` | — | S3 access key |
| `S3_SECRET_KEY` | — | S3 secret key |
| `S3_BUCKET` | `product-images` | Bucket name |

### Order service

| Variable | Default | Description |
|----------|---------|-------------|
| `DUPLI1_ORDER_DB` | — | Postgres connection string |
| `AUTH_JWKS_URL` | — | JWKS URL for RS256 token validation (set in Compose) |
| `JWT_SECRET` | — | HS256 fallback when JWKS is unavailable |
| `DUPLI1_GATEWAY_URL` | — | Internal gateway base URL for product stock/coupon calls (order service account) |
| `DUPLI1_ORDER_SERVICE_EMAIL` | — | Seed order service account email |
| `DUPLI1_ORDER_SERVICE_PASSWORD` | — | Seed order service account password |

### Payment service

| Variable | Default | Description |
|----------|---------|-------------|
| `DUPLI1_PAYMENT_DB` | — | Postgres connection string |
| `AUTH_JWKS_URL` | — | JWKS URL for RS256 token validation |
| `STRIPE_SECRET_KEY` | — | Stripe API key (optional locally) |
| `STRIPE_WEBHOOK_SECRET` | — | Stripe webhook signing secret |
| `PAYMENT_ALLOW_DEV_SIMULATE` | unset (prod) / `true` (Compose) | Enables `GET …/simulate-success` without Stripe |

### MinIO

| Variable | Default | Description |
|----------|---------|-------------|
| `MINIO_ACCESS_KEY` | `dupli1` | Root user |
| `MINIO_SECRET_KEY` | `dupli1_dev` | Root password |

## Testing

```bash
cd auth && go test ./...
cd product && go test ./...
cd order && go test ./...
cd cart && go test ./...
cd payment && go test ./...
cd notification && go test ./...
```

The order and auth modules have Postgres-backed tests that skip unless `POSTGRES_URL`
points at a reachable database:

```bash
cd order && POSTGRES_URL=postgres://dupli1:dupli1_dev@localhost:5435/orders?sslmode=disable go test ./...
```

With the stack running, `scripts/smoke-money-path.sh` exercises the full money path
(catalog → cart → checkout → payment → `paid` → ship → `fulfilled`) through the gateway:

```bash
BASE=http://localhost:8080 scripts/smoke-money-path.sh
```

## Dependencies

| Package | Purpose |
|---------|---------|
| `jackc/pgx/v4` | Postgres driver |
| `golang-jwt/jwt/v5` | JWT auth (RS256) |
| `minio/minio-go/v7` | S3 image storage |
| `gin-gonic/gin` | Auth HTTP framework |
| `redis/go-redis/v9` | Redis client (rate limiting) |
| `google/uuid` | UUID generation |
| `spf13/cobra` | Auth CLI |
