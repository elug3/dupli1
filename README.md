# Schick

Go backend for a fashion bag marketplace. Two services — auth and product — behind an nginx proxy, all wired with Docker Compose.

## Services

| Service | Port | Description |
|---------|------|-------------|
| `schick-auth` | 8080 | JWT login/register |
| `schick-product` | 8081 | Product catalog, bags, coupons, image upload |
| `schick-inventory` | 8082 | Stock and reservation APIs |
| `schick-order` | 8083 | Checkout and order lifecycle APIs |
| `schick-notification` | 8084 | Outbound notification service |
| `schick-proxy` | 80/443 | nginx reverse proxy |
| `postgres-auth` | 5432 | Auth DB |
| `postgres-product` | 5433 | Product DB |
| `minio` | 9000 / 9001 | S3-compatible image storage (console on 9001) |

## Running

```bash
cp .env.example .env   # set JWT_SECRET, OWNER_EMAIL, OWNER_PASSWORD
docker compose up --build
```

MinIO bucket `product-images` is created automatically on first start.

## Project Structure

```
schick/
├── cmd/
│   ├── schick-auth/       # Auth server entrypoint
│   └── schick-product/    # Product server entrypoint
└── pkg/
    ├── auth/              # Auth service
    │   ├── domain/        # User model
    │   ├── handler/       # HTTP handlers
    │   ├── infra/postgres/ # User repository
    │   ├── ports/         # Repository interface
    │   └── service/       # Login, register, token logic
    └── product/           # Product service
        ├── domain/        # Product, Bag, Coupon models
        ├── handler/       # HTTP handlers
        ├── infra/
        │   ├── pg/        # Postgres product store
        │   ├── memory/    # In-memory store (tests)
        │   └── s3/        # MinIO image store
        ├── middleware/    # JWT auth middleware
        ├── ports/         # ProductStore, ImageStore interfaces
        └── service/       # ProductSearchService, CouponService
```

## API

### Auth (`schick-auth` :8080)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/register` | — | Register user |
| POST | `/login` | — | Login, returns JWT |
| GET | `/health` | — | Health check |

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

### Product IDs

IDs are generated from the brand name: first 3 characters uppercased, followed by a sequential counter.

```
Bottega Veneta → BOT-001, BOT-002, …
Gucci          → GUC-001, GUC-002, …
```

### Image Upload

```bash
curl -X PUT http://localhost:8081/api/products/BOT-001/image \
  -H "Authorization: Bearer $TOKEN" \
  -F "image=@photo.jpg"
```

Returns the updated product with `imageUrl` set to the public MinIO URL.

## Environment Variables

### Auth service

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_URL` | — | Postgres connection string |
| `JWT_SECRET` | `dev-secret-change-in-production` | Signing secret |
| `SCHICK_AUTH_ADDR` | `:8080` | Listen address |
| `OWNER_EMAIL` | — | Seed admin email |
| `OWNER_PASSWORD` | — | Seed admin password |

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
cd pkg/product
go test ./...

cd pkg/auth
go test ./...
```

## Dependencies

| Package | Purpose |
|---------|---------|
| `jackc/pgx/v4` | Postgres driver |
| `golang-jwt/jwt/v5` | JWT auth |
| `minio/minio-go/v7` | S3 image storage |
| `google/uuid` | UUID fallback IDs |
