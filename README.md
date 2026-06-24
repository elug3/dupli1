# Schick

Go microservice backend for a fashion bag marketplace. Six services behind an nginx proxy, wired with Docker Compose.

## Services

| Service | Port | Description |
|---------|------|-------------|
| `schick-auth` | 8080 | JWT login/register, refresh tokens, RBAC user admin |
| `schick-product` | 8081 | Bag catalog, coupons, product CRUD, image upload |
| `schick-inventory` | 8082 | Stock and reservation APIs (in-memory) |
| `schick-order` | 8083 | Checkout and order lifecycle APIs (in-memory) |
| `schick-notification` | 8084 | Notification service stub (health only) |
| `schick-proxy` | 80/443 | nginx reverse proxy |
| `postgres-auth` | 5432 | Auth DB |
| `postgres-product` | 5433 | Product DB |
| `minio` | 9000 / 9001 | S3-compatible image storage (console on 9001) |

## Running

### Local (Docker Compose Postgres)

```bash
cp .env.example .env   # set JWT_SECRET, OWNER_EMAIL, OWNER_PASSWORD
docker compose up --build
```

### Against Amazon RDS (requires VPN)

Production databases live on **Amazon RDS** in a private subnet. To run auth/product locally against RDS:

```bash
# AWS credentials required (Secrets Manager read)
bash infra/scripts/fetch-rds-env.sh
docker compose -f docker-compose.yml -f docker-compose.rds.yml --env-file .env.rds up --build
```

See [docs/deployment-aws.md](docs/deployment-aws.md) and [infra/terraform/README.md](infra/terraform/README.md) for production ECS + RDS setup.

API gateway: `https://localhost` (self-signed cert — pass `-k` to curl or trust `certs/server.crt`).

Production uses **Amazon RDS** for PostgreSQL. See [docs/deployment-aws.md](docs/deployment-aws.md) and [infra/terraform/README.md](infra/terraform/README.md).

MinIO bucket `product-images` is created automatically on first start.

## Project Structure

```
schick/
├── cmd/
│   ├── schick-auth/          # Auth server entrypoint
│   ├── schick-product/       # Product server entrypoint
│   ├── schick-inventory/     # Inventory server entrypoint
│   ├── schick-order/         # Order server entrypoint
│   ├── schick-notification/  # Notification server entrypoint
│   └── schick-proxy/         # nginx reverse proxy
├── pkg/
│   ├── auth/                 # Auth service (Gin, Postgres, optional Redis/NATS)
│   ├── product/              # Product service (stdlib HTTP, Postgres, MinIO)
│   ├── inventory/            # Inventory service (in-memory)
│   ├── order/                # Order service (in-memory, calls inventory)
│   └── notification/         # Notification stub
├── docs/
│   ├── api.md                # API reference
│   ├── current-state.md      # Implementation snapshot
│   ├── deployment-aws.md     # AWS/ECS deployment
│   └── service-layout.md     # Service organization guide
└── infra/
    ├── terraform/            # RDS and secrets
    └── scripts/              # RDS cutover helpers
```

Each service package follows hexagonal architecture: `domain/`, `service/`, `ports/`, `infra/`, `handler/`, `bootstrap/`. See [ARCHITECTURE.md](ARCHITECTURE.md).

## API

Full reference: [docs/api.md](docs/api.md).

### Auth (`schick-auth` :8080)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/health` | — | Health check |
| POST | `/api/v1/auth/register` | — | Register user |
| POST | `/api/v1/auth/login` | — | Login, returns access + refresh tokens |
| GET | `/api/v1/auth/me` | Bearer | Current user profile |
| POST | `/api/v1/auth/refresh` | — | Exchange refresh token |
| POST | `/api/v1/auth/logout` | — | Invalidate refresh token |
| GET | `/api/v1/users` | Admin | List users |
| POST | `/api/v1/users` | Admin | Create user |
| GET | `/api/v1/users/{id}` | Admin | Get user |
| PUT | `/api/v1/users/{id}/role` | Admin | Update user role |
| DELETE | `/api/v1/users/{id}` | Admin | Delete user |

### Products (`schick-product` :8081)

**Public**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/health` | Health check |
| GET | `/api/products/bags` | Search bags (`?brand=`, `?color=`, `?material=`) |
| POST | `/api/coupons/redeem` | Redeem a coupon code |

**Requires `Authorization: Bearer <token>`**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/products` | List all products |
| POST | `/api/products` | Create product |
| GET | `/api/products/{id}` | Get product |
| PUT | `/api/products/{id}` | Update product |
| DELETE | `/api/products/{id}` | Delete product |
| PUT | `/api/products/{id}/image` | Upload product image (multipart `image` field) |
| GET | `/api/coupons` | List coupons |
| POST | `/api/coupons` | Create coupon |
| PUT | `/api/coupons/{code}` | Update coupon |
| DELETE | `/api/coupons/{code}` | Delete coupon |

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
curl -k -X PUT https://localhost/api/products/BOT-001/image \
  -H "Authorization: Bearer $TOKEN" \
  -F "image=@photo.jpg"
```

Returns the updated product with `imageUrls` populated.

## Environment Variables

### Auth service

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_URL` | — | Postgres connection string |
| `JWT_SECRET` | `dev-secret-change-in-production` | Signing secret |
| `SCHICK_AUTH_ADDR` | `:8080` | Listen address |
| `OWNER_EMAIL` | — | Seed owner email |
| `OWNER_PASSWORD` | — | Seed owner password |

### Product service

| Variable | Default | Description |
|----------|---------|-------------|
| `SCHICK_PRODUCT_DB` | — | Postgres connection string |
| `JWT_SECRET` | `dev-secret-change-in-production` | Signing secret |
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
cd pkg/auth && go test ./...
cd pkg/product && go test ./...
cd pkg/inventory && go test ./...
cd pkg/order && go test ./...
```

## Dependencies

| Package | Purpose |
|---------|---------|
| `jackc/pgx/v4` | Postgres driver |
| `golang-jwt/jwt/v5` | JWT auth |
| `minio/minio-go/v7` | S3 image storage |
| `gin-gonic/gin` | Auth HTTP framework |
| `google/uuid` | UUID generation |
