# Schick

Go microservice backend for a fashion bag marketplace. Six services behind an nginx proxy, wired with Docker Compose for local dev and deployed to AWS ECS Fargate in production.

## Services

| Service | Local port | Description |
|---------|------------|-------------|
| `schick-auth` | 18080 | JWT login/register, RS256 tokens, JWKS endpoint, RBAC user admin |
| `schick-product` | 8081 | Bag catalog, coupons, product CRUD, image upload |
| `schick-inventory` | 8082 | Stock and reservation APIs (in-memory) |
| `schick-order` | 8083 | Checkout and order lifecycle APIs (in-memory) |
| `schick-notification` | 8084 | Notification service stub (health only) |
| `schick-proxy` | 80 / 443 | nginx reverse proxy |
| `postgres-auth` | 5432 | Auth DB |
| `postgres-product` | 5433 | Product DB |
| `redis` | 6379 | Rate limiter backing store |
| `minio` | 9000 / 9001 | S3-compatible image storage (console on 9001) |

## Running

### Local (Docker Compose)

```bash
cp .env.example .env   # set OWNER_EMAIL, OWNER_PASSWORD
docker compose up --build
```

API gateway: `https://localhost` (self-signed cert — pass `-k` to curl or trust `certs/server.crt`).

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
schick/
├── auth/                 # Auth service
│   ├── cmd/              # CLI entrypoint (cobra, applyEnv)
│   └── pkg/              # bootstrap/, handler/, service/, infra/, domain/, ports/
├── product/              # Product service
├── inventory/            # Inventory service
├── order/                # Order service
├── notification/         # Notification stub
├── docker/nginx/         # nginx config
├── infra/
│   ├── terraform/        # RDS and secrets
│   └── scripts/          # RDS cutover helpers
├── certs/                # Self-signed TLS cert for local dev
├── Dockerfile            # Multi-service build (SERVICE build arg)
└── docs/                 # API reference and deployment guides
```

Each service follows hexagonal architecture: `domain/`, `service/`, `ports/`, `infra/`, `handler/`, `bootstrap/`. See [ARCHITECTURE.md](ARCHITECTURE.md).

## API

Full reference: [docs/api.md](docs/api.md).

### Auth (`schick-auth` :18080)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/health` | — | Health check |
| GET | `/.well-known/jwks.json` | — | RS256 public key set |
| POST | `/api/v1/auth/login` | — | Login, returns access + refresh tokens |
| POST | `/api/v1/auth/refresh` | — | Exchange refresh token |
| POST | `/api/v1/auth/logout` | — | Invalidate refresh token |
| GET | `/api/v1/auth/me` | Bearer | Current user profile |
| POST | `/api/v1/auth/register` | `admin` / `user_manager` | Create user account |
| GET | `/api/v1/auth/users` | `admin` | List users |
| PATCH | `/api/v1/auth/users/{id}/roles` | `admin` | Set user roles |
| PATCH | `/api/v1/auth/users/{id}/password` | `admin` / `user_manager` | Set user password |
| PATCH | `/api/v1/auth/users/{id}/status` | `admin` / `user_manager` | Activate / deactivate user |

Login and refresh are rate-limited per IP (10 req/min and 30 req/min respectively) via Redis.

Tokens are signed with RS256. In dev, an ephemeral 2048-bit key is generated on startup when `JWT_PRIVATE_KEY_FILE` is not set. In production, mount a stable PEM key.

### Products (`schick-product` :8081)

**Public**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/products/health` | Health check |
| GET | `/api/v1/products/bags` | Search bags (`?brand=`, `?color=`, `?material=`) |
| GET | `/api/v1/products/{id}` | Public product detail (active products only) |
| POST | `/api/v1/coupons/redeem` | Redeem a coupon code |

**Requires `Authorization: Bearer <token>`**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/products` | List all products |
| POST | `/api/v1/products` | Create product |
| GET | `/api/v1/products/{id}/manage` | Get product (admin, includes drafts/cost) |
| PUT | `/api/v1/products/{id}` | Update product |
| DELETE | `/api/v1/products/{id}` | Delete product |
| PUT | `/api/v1/products/{id}/image` | Upload product image (multipart `image` field) |
| GET | `/api/v1/coupons` | List coupons |
| POST | `/api/v1/coupons` | Create coupon |
| PUT | `/api/v1/coupons/{code}` | Update coupon |
| DELETE | `/api/v1/coupons/{code}` | Delete coupon |

### Inventory (`schick-inventory` :8082)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| GET | `/api/v1/inventory/{sku}` | Get stock for SKU |
| PUT | `/api/v1/inventory/{sku}` | Set stock quantity |
| POST | `/api/v1/inventory/{sku}/adjust` | Adjust stock by delta |
| POST | `/api/v1/inventory/reservations` | Reserve stock for an order |
| POST | `/api/v1/inventory/reservations/{id}/commit` | Commit reservation |
| POST | `/api/v1/inventory/reservations/{id}/release` | Release reservation |

### Orders (`schick-order` :8083)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| POST | `/api/v1/orders` | Create order |
| GET | `/api/v1/orders?customer_id=` | List customer orders |
| GET | `/api/v1/orders/{id}` | Get order |
| PUT | `/api/v1/orders/{id}/status` | Confirm, cancel, or fulfill order |

### Product IDs

IDs are generated from the brand name: first 3 characters uppercased, followed by a sequential counter.

```
Bottega Veneta → BOT-001, BOT-002, …
Gucci          → GUC-001, GUC-002, …
```

### Image Upload

```bash
curl -k -X PUT https://localhost/api/v1/products/BOT-001/image \
  -H "Authorization: Bearer $TOKEN" \
  -F "image=@photo.jpg"
```

Returns the updated product with `imageUrls` populated.

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
| `SCHICK_AUTH_ADDR` | `:8080` | Listen address |
| `OWNER_EMAIL` | — | Seed owner email (skips seeding if empty) |
| `OWNER_PASSWORD` | — | Seed owner password |

### Product service

| Variable | Default | Description |
|----------|---------|-------------|
| `SCHICK_PRODUCT_DB` | — | Postgres connection string |
| `JWT_SECRET` | `dev-jwt-secret-do-not-use-in-production` | Signing secret (HS256 fallback) |
| `SERVER_HOST` | `localhost` | Listen host |
| `SERVER_PORT` | `8080` | Listen port |
| `S3_ENDPOINT` | — | MinIO/S3 endpoint URL |
| `S3_ACCESS_KEY` | — | S3 access key |
| `S3_SECRET_KEY` | — | S3 secret key |
| `S3_BUCKET` | `product-images` | Bucket name |

### MinIO

| Variable | Default | Description |
|----------|---------|-------------|
| `MINIO_ACCESS_KEY` | `schick` | Root user |
| `MINIO_SECRET_KEY` | `schick_dev` | Root password |

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
