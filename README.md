# Dupli1

Go microservice backend for a fashion bag marketplace. Five services behind an nginx proxy, wired with Docker Compose for local dev and deployed to AWS ECS Fargate in production.

## Services

| Service | Local port | Description |
|---------|------------|-------------|
| `dupli1-auth` | 18080 | JWT login/refresh, RS256 tokens, JWKS, RBAC user admin |
| `dupli1-product` | 8081 | Bag catalog, coupons, product CRUD, image upload |
| `dupli1-inventory` | 8082 | Stock and reservation APIs (PostgreSQL) |
| `dupli1-order` | 8083 | Checkout sessions and order lifecycle (PostgreSQL) |
| `dupli1-notification` | 8084 | Notification stub (health only) |
| `dupli1-proxy` | 8080 / 80 | nginx reverse proxy (HTTP locally) |
| `postgres-auth` | 5432 | Auth DB |
| `postgres-product` | 5433 | Product DB |
| `postgres-inventory` | 5434 | Inventory DB |
| `postgres-order` | 5435 | Order DB |
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
├── product/              # Product catalog
├── inventory/            # Inventory service
├── order/                # Order + checkout
├── notification/         # Notification stub
├── api/
│   ├── nginx.conf        # Gateway routing
│   └── Dockerfile
├── infra/
│   ├── terraform/        # RDS and secrets
│   └── scripts/          # RDS cutover helpers
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
| GET | `/api/v1/auth/.well-known/jwks.json` | — | RS256 public key set |
| POST | `/api/v1/auth/login` | — | Login; returns refresh token |
| POST | `/api/v1/auth/refresh` | — | Exchange refresh token for access token |
| POST | `/api/v1/auth/logout` | — | Revoke refresh token |
| GET | `/api/v1/auth/me` | Bearer | Current user profile |
| POST | `/api/v1/auth/register` | `owner` / `admin` / `user_manager` / `customer_registrar` | Create user account (`account_type` optional) |
| GET | `/api/v1/auth/users` | `owner` / `admin` | List users |
| PATCH | `/api/v1/auth/users/{id}/roles` | `owner` / `admin` | Set user roles / `account_type` |
| PATCH | `/api/v1/auth/users/{id}/password` | `admin` / `user_manager` | Set user password |
| PATCH | `/api/v1/auth/users/{id}/status` | `admin` / `user_manager` | Activate / deactivate user |

**Token flow:** `POST /login` returns `{ "refresh_token": "..." }`. Call `POST /refresh` with that token to get `{ "token": "<access_jwt>" }`. Send the access token as `Authorization: Bearer <token>` on protected routes.

Login and refresh are rate-limited per IP via Redis.

Tokens are signed with RS256. In dev, an ephemeral 2048-bit key is generated on startup when `JWT_PRIVATE_KEY_FILE` is not set.

### Products (`dupli1-product` :8081)

**Public**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/products/health` | Health check |
| GET | `/api/v1/products` | Search **parent styles** (`?category=`, `?brand=`, `?color=`, `?size=`, `?tags=`) |
| GET | `/api/v1/products/{id}` | PDP: parent + variants (colors/sizes/images per SKU) |
| POST | `/api/v1/coupons/redeem` | Redeem a coupon code |

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
| GET | `/api/v1/coupons` | List coupons |
| POST | `/api/v1/coupons` | Create coupon |
| PUT | `/api/v1/coupons/{code}` | Update coupon |
| DELETE | `/api/v1/coupons/{code}` | Delete coupon |

### Inventory (`dupli1-inventory` :8082)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/inventory/health` | Health check |
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
| POST | `/api/v1/checkout/sessions` | Create checkout session |
| GET | `/api/v1/checkout/sessions/{id}` | Get session |
| PUT/POST/DELETE | `/api/v1/checkout/sessions/{id}/items` | Manage cart items |
| POST | `/api/v1/checkout/sessions/{id}/coupon` | Apply coupon |
| POST | `/api/v1/checkout/sessions/{id}/complete` | Complete checkout |
| POST | `/api/v1/orders` | Create order directly |
| GET | `/api/v1/orders?customer_id=` | List customer orders |
| GET | `/api/v1/orders/{id}` | Get order |
| PUT | `/api/v1/orders/{id}/status` | Confirm, cancel, or fulfill order |

See [docs/checkout-session.md](docs/checkout-session.md) for the checkout flow.

### Product IDs and variants

Parent style IDs are generated from the brand name: first 3 characters uppercased, followed by a sequential counter.

```
Bottega Veneta → BOT-001, BOT-002, …
```

Variants (sellable SKUs) hang under a parent, e.g. `BOT-001-GRN` / `BOT-001-BLK`. Search returns parents only so colors do not duplicate results. Checkout and inventory use the **variant SKU**.

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
| `JWT_PRIVATE_KEY_FILE` | — | Path to PEM-encoded RSA private key (RS256); ephemeral key used in dev if unset |
| `JWT_KEY_ID` | `default` | `kid` value in the JWKS document |
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
| `S3_ENDPOINT` | — | MinIO/S3 endpoint URL |
| `S3_ACCESS_KEY` | — | S3 access key |
| `S3_SECRET_KEY` | — | S3 secret key |
| `S3_BUCKET` | `product-images` | Bucket name |

### Order service

| Variable | Default | Description |
|----------|---------|-------------|
| `DUPLI1_ORDER_DB` | — | Postgres connection string |
| `AUTH_JWKS_URL` | — | JWKS URL for RS256 token validation (set in Compose) |
| `JWT_SECRET` | — | HS256 fallback when JWKS is unavailable |
| `DUPLI1_INVENTORY_URL` | — | Inventory service base URL |
| `DUPLI1_PRODUCT_URL` | — | Product service base URL (coupon redeem) |

### Inventory service

| Variable | Default | Description |
|----------|---------|-------------|
| `DUPLI1_INVENTORY_DB` | — | Postgres connection string |

### MinIO

| Variable | Default | Description |
|----------|---------|-------------|
| `MINIO_ACCESS_KEY` | `dupli1` | Root user |
| `MINIO_SECRET_KEY` | `dupli1_dev` | Root password |

## Testing

```bash
cd auth && go test ./...
cd product && go test ./...
cd inventory && go test ./...
cd order && go test ./...
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
