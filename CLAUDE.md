# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

Each service is an independent Go module. The root `go.mod` is a stub — `go test ./...` from the repo root does **not** work.

```bash
# Tests (run from each service directory)
cd auth && go test ./...
cd product && go test ./...
cd order && go test ./...
cd cart && go test ./...
cd payment && go test ./...
cd notification && go test ./...
cd shared && go test ./...

# Single package
cd order && go test ./pkg/service/...

# Single test
cd order && go test ./pkg/service/... -run TestCreateOrder

# Postgres-backed tests (order example)
cd order && POSTGRES_URL=postgres://dupli1:dupli1_dev@localhost:5435/orders?sslmode=disable go test ./...

# Build a service binary
cd auth && go build ./cmd/

# Run locally (full stack)
sudo dockerd >/tmp/dockerd.log 2>&1 &   # daemon does not autostart on this VM
sudo docker compose up --build

# End-to-end money path smoke test (stack must be running)
BASE=http://localhost:8080 scripts/smoke-money-path.sh
```

**Docker note:** all `docker`/`docker compose` commands need `sudo` on this VM. The `fuse-overlayfs` storage driver is configured; standard overlayfs does not work here.

After editing `api/nginx.conf`, rebuild the proxy only:
```bash
sudo docker compose up -d --build dupli1-proxy
```

## Architecture

Hexagonal architecture enforced across all services. Dependency flow: `handler → service → ports ← infra`, with `domain` at the center depending on nothing. Business logic belongs only in `service/` and `domain/`. Infra implements ports; handlers translate HTTP.

```
<service>/
├── cmd/           # Process entrypoint (flags, env, starts server)
└── pkg/
    ├── domain/    # Entities and business rules — no external imports
    ├── service/   # Use cases; imports domain + ports only
    ├── ports/     # Interfaces for repos and external clients
    ├── infra/     # Postgres, Redis, S3, HTTP clients, in-memory fakes
    ├── handler/   # HTTP only — validate input, call service, write response
    └── bootstrap/ # Wiring: create DB → repo → service → handler → start server
```

Configuration lives in `<service>/pkg/bootstrap/config.go` and/or `<service>/pkg/options.go`.

### Shared module

`shared/` (`github.com/elug3/dupli1/shared`) holds cross-service libraries with no service-specific dependencies. Local dev: each service `go.mod` has `replace github.com/elug3/dupli1/shared => ../shared`.

| Package | Purpose |
|---------|---------|
| `shared/pkg/permissions` | Permission constants, `Has`/`HasAny`, wildcard evaluation, legacy role expansion, named bundles |
| `shared/pkg/authjwt` | JWKS/JWT validation helpers (RS256 via `AUTH_JWKS_URL`; HS256 fallback) |
| `shared/pkg/settings` | `GET /settings` response helpers used by all services |
| `shared/pkg/outbox` | Transactional outbox drain/retry loop (`Drainer`), used by `order` and `payment`; each service keeps its own outbox table/SQL behind the `Store` interface |
| `shared/pkg/events` | NATS subject constants + payload structs for cross-service events (`order.*`, `payment.succeeded`, `product.*`); one canonical contract per publisher/subscriber pair instead of redeclaring subject strings and payload shapes on each side |
| `shared/pkg/pgsslmode` | Picks `sslmode` for a Postgres connection string (local/docker hosts → `disable`, everything else including RDS → `require`); used by every service's DB bootstrap so the local-hostname list can't drift out of sync per service again |
| `shared/pkg/natspublisher` | JSON-marshaling NATS event publisher (`New`, `Publish`, `Close`), used by `auth`, `order`, `product`, and `payment` |
| `shared/pkg/authmiddleware` | Bearer-token HTTP middleware (`RequireAuth`, `OptionalAuth`) parameterized by `authjwt.AccessTokenValidator` and a per-service error-response callback, so each service keeps its own error body shape; used by `cart`, `order`, `payment`, `notification`, `product` |

### Service ownership

| Service | Framework | Key responsibility |
|---------|-----------|--------------------|
| `auth` | Gin | RS256 JWT + JWKS, fine-grained permissions, user admin, customer profile + saved addresses |
| `product` | stdlib `net/http` | Parent-style + variant(SKU) catalog, images (MinIO/S3), stock & reservations (merged from former `inventory` service) |
| `order` | stdlib `net/http` | Checkout sessions, order lifecycle, transactional outbox → NATS |
| `cart` | stdlib `net/http` | Persistent per-customer cart; enriches lines from product |
| `payment` | stdlib `net/http` | NANO card / manager Bypass (also the local/dev testing path); publishes `payment.succeeded` via outbox |
| `notification` | stdlib `net/http` | NATS subscriber → Telegram ops alerts |

### Product model

Products have two levels: **parent** (style, e.g. "Prada Galleria") and **variant** (sellable SKU). Search returns parents only; PDP embeds variants. Each variant has:
- `sku`: human string `Brand_Style_Color[_Edition]_Size`
- `skuId`: canonical ULID (preferred in cart, checkout, inventory)

Price lives on the **parent** (not the variant). Variants inherit price for cart JSON. Never place price on variants.

### Order lifecycle & money path

```
POST /orders  →  pending  →  paid  →  in_transit  →  fulfilled
                    ↓                      ↑ (commit stock)
                 canceled ←──── auto-cancel after 5 min unpaid
```

Order calls product stock/coupons via the internal nginx gateway (`DUPLI1_GATEWAY_URL`), not direct service URLs. Pricing is resolved server-side — client `unit_price_cents` is ignored.

Event flow: `payment.succeeded` (NATS, published by payment outbox) → order marks `paid`. `POST /orders/{id}/ship` → commits inventory reservation → `in_transit`.

Both order and payment use a **transactional outbox** pattern: event rows are written in the same DB transaction as the state change, then a drain worker publishes to NATS. This makes state changes the source of truth — NATS failures are retried.

### Auth token flow

`POST /login` → `{ "refresh_token": "..." }`. Call `POST /refresh` with that token → `{ "token": "<access_jwt>" }`. Send as `Authorization: Bearer <token>` on protected routes. Access tokens carry a `permissions` string array claim (no `roles`).

### Authorization

Fine-grained permissions (`{resource}.{action}`, e.g. `product.create`, `order.ship`). Wildcards: `*` (owner), `admin.*`, `{resource}.*`. Storefront customers use ABAC (JWT `sub` must match resource owner) with no explicit permission required.

Key bundles: `catalog_editor`, `catalog_admin`, `fulfillment`, `user_admin`. See `docs/permissions.md` for the full catalog.

### Schema migrations

Services migrate their own schema inline on startup (no separate migration tool). Order and product use `ALTER TABLE … ADD COLUMN IF NOT EXISTS` for additive changes and silently continue on error for those. Breaking schema changes are not supported this way.

### In-memory fallbacks

Order, cart, and payment use PostgreSQL when their `DUPLI1_*_DB` env var is set; otherwise they fall back to an in-memory repository. Tests rely on this — no database needed unless testing Postgres-specific behavior.

## Key constraints

- **Currency: KRW only.** All `*_cents` fields are whole Korean won. No fractional amounts.
- **No `go.work`.** Run and test from each service module directory.
- **nginx resolver:** `api/nginx.conf` must list only Docker's embedded DNS `127.0.0.11` in its `resolver` directive. Adding `10.0.0.2` (AWS VPC) causes ~50% of local requests to fail with `502`.
- **Legacy API aliases.** Canonical paths are `/api/v1/{service}/…`; legacy top-level prefixes (`/api/v1/inventory/`, `/api/v1/checkout/`, `/api/v1/carts/`, etc.) are still registered. New code uses canonical paths only.
- **Docs.** Before adding a new `docs/*.md`, check [docs/README.md](docs/README.md) and existing overlap. Use the service-name prefix (`order-*.md`, `product-*.md`). Update `docs/current-state.md` and `docs/api.md` when the API surface changes.

## Dev database credentials

| Service | Port | DB | User | Password |
|---------|------|----|------|----------|
| auth | 5432 | `dupli1_db` | `dupli1` | `dupli1_dev` |
| product | 5433 | `products` | `dupli1` | `dupli1_dev` |
| order | 5435 | `orders` | `dupli1` | `dupli1_dev` |
| cart | 5436 | `cart` | `dupli1` | `dupli1_dev` |
| payment | 5437 | `payments` | `dupli1` | `dupli1_dev` |
| notification | 5438 | `notifications` | `dupli1` | `dupli1_dev` |

Seeded owner: `admin@dupli1.com` / `password`.

## Multi-service Docker image

The single root `Dockerfile` builds any service via a `SERVICE` build arg:
```bash
docker build --build-arg SERVICE=order -t dupli1-order .
```
Docker Compose sets this automatically per service definition.
