# Dupli1

Go microservice backend for a fashion bag marketplace. Services behind an nginx proxy, wired with Docker Compose for local dev. Production runs the same images under Docker Compose on VENUS, a self-hosted machine behind a Cloudflare Tunnel ([docs/deployment-venus.md](docs/deployment-venus.md)); it moved off AWS ECS / RDS on 2026-09-27.

**Authoritative implementation snapshot:** [docs/current-state.md](docs/current-state.md). Doc index: [docs/README.md](docs/README.md). The tables below are a quick reference; when they diverge, trust `current-state.md` and [docs/api.md](docs/api.md).

## Services

| Service | Local port | Description |
|---------|------------|-------------|
| `dupli1-auth` | 18080 | JWT login/refresh, RS256 tokens, JWKS, permission-based user admin |
| `dupli1-profile` | 8088 | Customer commerce profile + saved shipping addresses (PostgreSQL) |
| `dupli1-product` | 8081 | Bag catalog, promotional codes, product CRUD, image upload, stock and reservation APIs |
| `dupli1-order` | 8083 | Checkout sessions and order lifecycle (PostgreSQL) |
| `dupli1-cart` | 8086 | Shopping cart (PostgreSQL) |
| `dupli1-payment` | 8087 | Payments — NANO card, manager Bypass |
| `dupli1-notification` | 8084 | NATS subscriber → Telegram ops alerts when configured |
| `dupli1-support` | 8089 | Customer consultation: Telegram bot, web chat, product questions, staff inbox |
| `dupli1-proxy` | 8080 / 80 | nginx reverse proxy (HTTP locally) |
| `postgres-auth` | 5432 | Auth DB (`dupli1_db`) |
| `postgres-profile` | 5439 | Profile DB (`profiles`) |
| `postgres-product` | 5433 | Product DB (also stock/reservations) |
| `postgres-order` | 5435 | Order DB |
| `postgres-cart` | 5436 | Cart DB |
| `postgres-payment` | 5437 | Payment DB |
| `postgres-notification` | 5438 | Notification DB (Telegram subscriptions) |
| `postgres-support` | 5440 | Support DB (`support`) |
| `redis` | 6379 | Rate limiter / session cache |
| `nats` | 4222 | Event bus (order/payment/notification) |
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

### Against Amazon RDS (historical)

Production no longer runs on AWS. These helpers ran auth/product locally against the former RDS instance:

```bash
# AWS credentials required (Secrets Manager read)
bash infra/scripts/fetch-rds-env.sh
docker compose -f docker-compose.yml -f docker-compose.rds.yml --env-file .env.rds up --build
```

[docs/deployment-aws.md](docs/deployment-aws.md) describes the former ECS + RDS setup; production is in [docs/deployment-venus.md](docs/deployment-venus.md).

## Project Structure

```
dupli1/
├── auth/                 # Auth service (cmd/ + pkg/)
├── profile/              # Customer commerce profile + saved addresses
├── product/              # Product catalog (also stock/reservations)
├── order/                # Order + checkout
├── cart/                 # Shopping cart
├── payment/              # Payments (NANO card / Bypass)
├── notification/         # NATS → Telegram ops alerts
├── support/              # Customer consultation (Telegram bot, web chat, product questions)
├── shared/               # Cross-service Go libraries
├── api/
│   ├── gateway/routes.conf  # Route table shared by every environment
│   ├── nginx*.conf       # Per-environment gateway wrappers
│   └── Dockerfile*
├── deploy/venus/         # Production Compose stack and deploy.sh
├── infra/
│   ├── terraform/        # Former AWS production (historical)
│   └── scripts/          # RDS cutover helpers (historical)
├── certs/                # TLS material (not wired into local nginx yet)
├── Dockerfile            # Multi-service build (SERVICE build arg)
└── docs/                 # API reference and deployment guides
```

Each service follows hexagonal architecture: `domain/`, `service/`, `ports/`, `infra/`, `handler/`, `bootstrap/`. See [ARCHITECTURE.md](ARCHITECTURE.md) and [docs/service-layout.md](docs/service-layout.md).

## API

Full reference: [docs/api.md](docs/api.md). Route index: [docs/endpoints.md](docs/endpoints.md).

### Auth (`dupli1-auth` :18080)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/health` | — | Health check |
| GET | `/settings` | — | Non-secret service settings |
| GET | `/api/v1/auth/.well-known/jwks.json` | — | RS256 public key set |
| POST | `/api/v1/auth/token` | `ApiKey` | Service account API key → access token (internal listener only) |
| POST | `/api/v1/auth/login` | — | Login; returns refresh token |
| POST | `/api/v1/auth/refresh` | — | Exchange refresh token for access token |
| POST | `/api/v1/auth/logout` | — | Revoke refresh token |
| GET | `/api/v1/auth/me` | Bearer | Current user account |
| POST | `/api/v1/auth/register` | `user.create` *or open register* | Create user (`AUTH_OPEN_REGISTER` allows anonymous → `customer`) |
| GET | `/api/v1/auth/users` | `user.read` | List users (auth ABAC) |
| PATCH | `/api/v1/auth/users/{id}/permissions` | `user.permissions.update` | Replace permissions / optional `account_type` |
| PATCH | `/api/v1/auth/users/{id}/password` | `user.password.update` | Set user password |
| PATCH | `/api/v1/auth/users/{id}/status` | `user.status.update` | Activate / deactivate user |
| DELETE | `/api/v1/auth/users/{id}` | `user.delete` | Permanently delete user; enqueues `user.deleted` in the same transaction (consumed by `profile`) |
| GET/POST | `/api/v1/auth/users/{id}/api-keys` | `user.apikey.read` / `user.apikey.manage` | List / mint a service account's API keys |
| DELETE | `/api/v1/auth/api-keys/{keyId}` | `user.apikey.manage` | Revoke an API key |
| GET | `/api/v1/auth/reports/registrations` | `user.read` | Customer sign-ups per KST week or month |

### Profile (`dupli1-profile` :8088)

Customer commerce PII — display name, phone, saved shipping addresses — extracted from `auth`. Self-service only (JWT `sub` ABAC, no dedicated permission). See [docs/profile-service.md](docs/profile-service.md).

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET/PATCH | `/api/v1/profile/me/profile` | Bearer | Customer commerce profile |
| GET/POST/PATCH/DELETE | `/api/v1/profile/me/addresses`… | Bearer | Saved shipping addresses (max 10) |
| POST | `/api/v1/profile/me/addresses/{id}/default` | Bearer | Set sole default address |

One-release nginx aliases keep `/api/v1/auth/me/profile` and `/api/v1/auth/me/addresses` working during storefront cutover.

Authorization uses fine-grained **permissions** in the JWT (`permissions` claim), not legacy roles. See [docs/permissions.md](docs/permissions.md).

**Token flow:** `POST /login` returns `{ "refresh_token": "..." }`. Call `POST /refresh` with that token to get `{ "token": "<access_jwt>", "refresh_token": "<new_jwt>" }`. Send the access token as `Authorization: Bearer <token>` on protected routes.

Refresh tokens rotate on every use — the token you sent stops working and the response's `refresh_token` must be used for the next refresh. Login and refresh are rate-limited per IP via Redis.

Tokens are signed with RS256. In dev, an ephemeral 2048-bit key is generated on startup when `JWT_PRIVATE_KEY_FILE` is not set.

### Products (`dupli1-product` :8081)

**Public**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/products/health` | Health check |
| GET | `/api/v1/products/settings` | Non-secret service settings |
| GET | `/api/v1/products` | Search **parent styles** (`?category=`, `?brand=`, `?color=`, `?size=`, `?tags=`, …) |
| GET | `/api/v1/products/{id}` | PDP: parent + variants (colors/sizes/images per SKU) |
| GET | `/api/v1/products/{id}/recommendations` | Content + popularity recommendations |
| POST | `/api/v1/products/promotions/redeem` | Look up a promotional code |
| POST | `/api/v1/products/promotions/evaluate` | Price a promotional code against a cart |

**Requires `Authorization: Bearer <access_token>`** (validated via JWKS; per-route permissions)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/products` | Manager search (all statuses; needs `product.read`) |
| POST | `/api/v1/products` | Create parent style |
| PUT | `/api/v1/products/{id}` | Update parent |
| DELETE | `/api/v1/products/{id}` | Delete parent (cascades variants) |
| POST | `/api/v1/products/{id}/images` | Upload image to default variant |
| POST | `/api/v1/products/{id}/variants` | Create variant (SKU) |
| PUT/DELETE | `/api/v1/products/{id}/variants/{sku}` | Update / delete variant |
| POST | `/api/v1/products/{id}/variants/{sku}/images` | Upload image for a variant |
| GET/POST | `/api/v1/products/promotions` | List / create promotional codes |
| PUT/DELETE | `/api/v1/products/promotions/by-code/{code}` | Update / delete a promotional code |

The pre-rename `…/coupons` paths are still accepted for one release. The rest of the promotion surface (wallet, tiers, issue, ledger) is in [docs/endpoints.md](docs/endpoints.md#product-service).

### Inventory (served by `dupli1-product` :8081)

Stock and reservations, merged into the product service. **Canonical paths:**
`/api/v1/products/inventory/*` (legacy `/api/v1/inventory/*` still aliased on the gateway).
Each variant has a canonical ULID `skuId`; item routes also have
`by-sku-id/{skuId}` siblings.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/products/inventory/health` | Health check |
| GET | `/api/v1/products/inventory/settings` | Non-secret product-service settings |
| GET | `/api/v1/products/inventory/items/{sku}` | Get stock for SKU |
| PUT | `/api/v1/products/inventory/items/{sku}` | Set stock quantity |
| POST | `/api/v1/products/inventory/items/{sku}/adjust` | Adjust stock by delta |
| POST | `/api/v1/products/inventory/reservations` | Reserve stock for an order |
| POST | `/api/v1/products/inventory/reservations/{id}/commit` | Commit reservation |
| POST | `/api/v1/products/inventory/reservations/{id}/release` | Release reservation |

### Orders (`dupli1-order` :8083)

Requires `Authorization: Bearer <access_token>` when `AUTH_JWKS_URL` or `JWT_SECRET` is set (RS256 via auth JWKS in Compose; `JWT_SECRET` is HS256 fallback in dev).

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/orders/health` | Health check |
| GET | `/api/v1/orders/settings` | Non-secret service settings |
| POST | `/api/v1/orders/checkout/sessions` | Create checkout session (legacy `/api/v1/checkout/sessions`) |
| GET | `/api/v1/orders/checkout/sessions/{id}` | Get session |
| PUT/POST/DELETE | `/api/v1/orders/checkout/sessions/{id}/items` | Manage session items |
| POST/DELETE | `/api/v1/orders/checkout/sessions/{id}/promotion` | Apply / remove a promotional code |
| POST | `/api/v1/orders/checkout/sessions/{id}/complete` | Complete checkout |
| POST | `/api/v1/orders` | Create order directly |
| GET | `/api/v1/orders?customer_id=` | List customer orders |
| GET | `/api/v1/orders/{id}` | Get order |
| POST | `/api/v1/orders/{id}/confirm` | Accept a paid order (`paid` → `confirmed`) |
| POST | `/api/v1/orders/{id}/ship` | Ship order (`confirmed` → `in_transit`, commit stock) |
| POST | `/api/v1/orders/{id}/deliver` | Mark delivered |
| PUT | `/api/v1/orders/{id}/status` | Fulfill or cancel order |

Cancel requests, receipt confirmation and disputes: [docs/endpoints.md](docs/endpoints.md#order-service).

See [docs/checkout-session.md](docs/checkout-session.md) for the checkout flow. See [docs/cart-service.md](docs/cart-service.md) for the persistent cart. See [docs/payment-service.md](docs/payment-service.md) for payment (NANO card, Bypass).

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
| GET | `/api/v1/cart/customers/{customer_id}` | Admin: get user cart (`cart.read`; legacy `/api/v1/carts/{id}`) |

### Payment (`dupli1-payment` :8087)

Credit card uses **NANO Solution** certified payment when `NANO_*` is configured; otherwise `credit_card` is unavailable and manager **Bypass** (`payment.bypass`) is used, including for local testing. Unpaid `pending` orders auto-cancel after **5 minutes**. [docs/payment-service.md](docs/payment-service.md).

### Product IDs and variants

New parent style `id`s are **ULIDs**. Human design identity is `brandCode` + `styleCode` (master catalog). Legacy brand-prefixed ids (e.g. `BOT-001`) remain readable.

Variants (sellable SKUs) hang under a parent. Each variant has a human `sku` (`Brand_Style_Color[_Edition]_Size`) and a canonical ULID `skuId`. Search returns parents only so colors do not duplicate results. Cart, checkout, and inventory prefer **`skuId`** (human `sku` still accepted). See [docs/product-sku-system.md](docs/product-sku-system.md).

### Image Upload

```bash
# Preferred: image for a specific color/size variant
curl -X POST "http://localhost:8080/api/v1/products/{parentId}/variants/{sku}/images" \
  -H "Authorization: Bearer $TOKEN" \
  -F "image=@photo.jpg"

# Appends to the default variant
curl -X POST "http://localhost:8080/api/v1/products/{parentId}/images" \
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
| `NATS_TOKEN` | — | Token matching nats-server `--auth` (Compose default `dupli1_nats_dev`) |
| `JWT_PRIVATE_KEY_FILE` | — | Path to PEM-encoded RSA private key (RS256); ephemeral key used in dev if unset |
| `JWT_KEY_ID` | `default` | `kid` value in the JWKS document |
| `JWT_EXPIRATION` | `15m` | Access token lifetime |
| `DUPLI1_AUTH_ADDR` | `:8080` | Listen address |
| `OWNER_EMAIL` | — | Seed owner email (skips seeding if empty) |
| `OWNER_PASSWORD` | — | Seed owner password |
| `DUPLI1_WEB_SERVICE_EMAIL` | — | Seed dupli1-web service account email |
| `DUPLI1_WEB_SERVICE_API_KEY` | — | Its API key; required with the email (service accounts have no password — [docs/auth-service-api-keys.md](docs/auth-service-api-keys.md)) |
| `DUPLI1_ORDER_SERVICE_EMAIL` / `DUPLI1_ORDER_SERVICE_API_KEY` | — | Same for dupli1-order |

### Product service

| Variable | Default | Description |
|----------|---------|-------------|
| `DUPLI1_PRODUCT_DB` | — | Postgres connection string |
| `AUTH_JWKS_URL` | — | JWKS URL for RS256 token validation (set in Compose) |
| `JWT_SECRET` | — | HS256 fallback when JWKS is unavailable |
| `SERVER_HOST` | `localhost` | Listen host |
| `SERVER_PORT` | `8080` | Listen port |
| `S3_ENDPOINT` | — | MinIO/S3 endpoint URL (uploads) |
| `S3_PUBLIC_ENDPOINT` | — | Browser base for `imageUrls` (Compose: `http://localhost:8080/product-images`; VENUS: `https://dupli1.com/product-images`) |
| `S3_ACCESS_KEY` | — | S3 access key |
| `S3_SECRET_KEY` | — | S3 secret key |
| `S3_BUCKET` | `product-images` | Bucket name |

### Order service

| Variable | Default | Description |
|----------|---------|-------------|
| `DUPLI1_ORDER_DB` | — | Postgres connection string |
| `AUTH_JWKS_URL` | — | JWKS URL for RS256 token validation (set in Compose) |
| `JWT_SECRET` | — | HS256 fallback when JWKS is unavailable |
| `DUPLI1_GATEWAY_URL` | — | Preferred: internal gateway base for stock, promotions and refunds (Compose: `http://dupli1-proxy:8081`) |
| `DUPLI1_ORDER_SERVICE_API_KEY` | — | The `dupli1-order` service account's API key |
| `DUPLI1_INVENTORY_URL` | — | **Deprecated** direct product override for stock |
| `DUPLI1_PRODUCT_URL` | `http://localhost:8081` | **Deprecated** direct product override, used when the gateway URL is unset |

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
cd profile && go test ./...
cd support && go test ./...
cd shared && go test ./...
```

The order, product and auth modules have Postgres-backed tests that skip unless `POSTGRES_URL`
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
