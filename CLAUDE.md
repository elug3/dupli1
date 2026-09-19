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
cd support && go test ./...
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
| `shared/pkg/events` | NATS subject constants + payload structs for cross-service events (`order.*`, `payment.succeeded`, `product.*`, `support.inquiry_opened`); one canonical contract per publisher/subscriber pair instead of redeclaring subject strings and payload shapes on each side |
| `shared/pkg/pgsslmode` | Picks `sslmode` for a Postgres connection string (local/docker hosts → `disable`, everything else including RDS → `require`); used by every service's DB bootstrap so the local-hostname list can't drift out of sync per service again |
| `shared/pkg/natspublisher` | JSON-marshaling NATS event publisher (`New`, `Publish`, `Close`), used by `auth`, `order`, `product`, and `payment` |
| `shared/pkg/authmiddleware` | Bearer-token HTTP middleware (`RequireAuth`, `OptionalAuth`) parameterized by `authjwt.AccessTokenValidator` and a per-service error-response callback, so each service keeps its own error body shape; used by `cart`, `order`, `payment`, `notification`, `product` |
| `shared/pkg/telegram` | Telegram Bot API client — `Client` (send/reply with retry, backoff and bot-token redaction in errors), wire types (`Update`, `Message`, `Chat`, `User`, `CallbackQuery`), inline-keyboard menus (`ReplyMenu`, `EditMessageText`, `AnswerCallback`), `SendSilent` for an alert that should queue rather than interrupt, webhook registration (`WithAllowedUpdates` opts a bot into `callback_query`; the default stays `message` only), `GetUpdates` polling (`RunPoller`/`DrainUpdates` over a `Handler`), HTML escaping and 4096-char-safe truncation. The `AccessPolicy` interface is the client's only view of who may be messaged, so each bot keeps its own policy; used by `notification` (ops alerts) |
| `shared/pkg/productclient` | HTTP client for product's variant-lookup endpoint, returning a superset `Variant`; used by `cart` and `order`, each mapping only the display field it needs (`Color` vs `ProductName`) into its own local `ports.VariantInfo` |

### Service ownership

| Service | Framework | Key responsibility |
|---------|-----------|--------------------|
| `auth` | Gin | RS256 JWT + JWKS, fine-grained permissions, user admin. Customer profile + saved addresses code still lives here too (not yet removed — see [docs/auth-profile-extension-plan.md](docs/auth-profile-extension-plan.md) Phase D) but `profile` is now the wired service for that traffic |
| `product` | stdlib `net/http` | Parent-style + variant(SKU) catalog, images (MinIO/S3), stock & reservations (merged from former `inventory` service) |
| `order` | stdlib `net/http` | Checkout sessions, order lifecycle, transactional outbox → NATS |
| `cart` | stdlib `net/http` | Persistent per-customer cart; enriches lines from product |
| `payment` | stdlib `net/http` | NANO card / manager Bypass (also the local/dev testing path); publishes `payment.succeeded` via outbox |
| `notification` | stdlib `net/http` | NATS subscriber → Telegram ops alerts. Chats opt into each alert class separately (`alert_order`, `alert_product`, `alert_support`); customer inquiry handoffs go only to chats that asked for them, with no env fallback |
| `support` | stdlib `net/http` | Customer-facing Telegram consultation bot: menu router, conversation state, handoff to staff. **Separate bot and token from `notification`'s ops bot** (`TELEGRAM_SUPPORT_*`, never `TELEGRAM_*`) — one token owns one update stream. Phases 0–4 of [docs/support-telegram-bot.md](docs/support-telegram-bot.md): the menu tree routes, conversations, inquiries and transcripts persist in PostgreSQL (`support`), canned answers are seeded insert-if-absent so staff edits survive a deploy, and asking for a human opens an inquiry and publishes `support.inquiry_opened` for `notification` to fan out. Service hours are weekdays 10:00–22:00 KST (config-driven; holidays are staff behavior, not code). Phase 5 adds the manager inbox (`support.read|reply|manage`, `support_agent` bundle) and the manage-web `/support` tab, where staff claim, reply and close; an undeliverable reply is stored and shown as 미전송 |
| `profile` | stdlib `net/http` | Customer commerce profile (display name, phone) + saved addresses; subscribes `user.deleted` from auth to cascade-delete. Extracted per [docs/auth-profile-extension-plan.md](docs/auth-profile-extension-plan.md) Phase D |

### Product model

Products have two levels: **parent** (style, e.g. "Prada Galleria") and **variant** (sellable SKU). Search returns parents only; PDP embeds variants. Each variant has:
- `sku`: human string `Brand_Style_Color[_Edition]_Size`
- `skuId`: canonical ULID (preferred in cart, checkout, inventory)

Price lives on the **parent** (not the variant). Variants inherit price for cart JSON. Never place price on variants.

### Order lifecycle & money path

```
POST /orders → pending → paid → confirmed → in_transit → delivered → fulfilled
                  ↓         ↓(2h SLA) ↑         ↑(commit stock)  ↓(14d SLA or dispute)
               canceled ←──────────────────────────────────  disputed
                  ↑ auto-cancel after 5 min unpaid (pending only)
```

- `paid → confirmed`: manager accepts the order for fulfillment (`POST /orders/{id}/confirm`), auto-confirmed after a 2-hour SLA if the manager doesn't act (`EnforceRefundPolicy`).
- `confirmed → in_transit`: manager ships (`POST /orders/{id}/ship`), which commits the inventory reservation.
- `in_transit → delivered`: carrier or manager marks delivered (`POST /orders/{id}/deliver`).
- `delivered → fulfilled`: customer confirms receipt (`POST /orders/{id}/receipt/confirm`), a manager override (`PUT /orders/{id}/status`), or auto-fulfilled 14 days after delivery with no response (`EnforceDeliveryPolicy`).
- `delivered → disputed`: customer reports non-receipt (`POST /orders/{id}/receipt/dispute`). A manager resolves it either way — `POST /orders/{id}/dispute/resolve` closes it fulfilled (proof of delivery), or `PUT /orders/{id}/status` → `canceled` refunds it like any other cancel.
- Cancellation: immediate refund while `pending`/`paid` (not yet confirmed); from `confirmed` through `delivered` a customer cancel opens a manager-approval request (`POST /orders/{id}/cancel`, approved/rejected via `.../cancel/approve` / `.../cancel/reject`, auto-approved after the same 2-hour SLA). Once shipped, canceling refunds but never auto-restocks.

Order calls product stock/promotions via the internal nginx gateway (`DUPLI1_GATEWAY_URL`), not direct service URLs. Pricing is resolved server-side — client `unit_price_won` is ignored.

Event flow: `payment.succeeded` (NATS, published by payment outbox) → order marks `paid`. `POST /orders/{id}/ship` → commits inventory reservation → `in_transit`.

Both order and payment use a **transactional outbox** pattern: event rows are written in the same DB transaction as the state change, then a drain worker publishes to NATS. This makes state changes the source of truth — NATS failures are retried.

### Auth token flow

`POST /login` → `{ "refresh_token": "..." }`. Call `POST /refresh` with that token → `{ "token": "<access_jwt>", "refresh_token": "<new_jwt>" }`. Send as `Authorization: Bearer <token>` on protected routes. Access tokens carry a `permissions` string array claim (no `roles`).

Refresh tokens rotate on every use: `/refresh` invalidates the token it was given and returns a new one, which the caller must store and use next time. Reusing an already-rotated refresh token fails with `401`.

### Authorization

Fine-grained permissions (`{resource}.{action}`, e.g. `product.create`, `order.ship`). Wildcards: `*` (owner), `admin.*`, `{resource}.*`. Storefront customers use ABAC (JWT `sub` must match resource owner) with no explicit permission required.

Key bundles: `catalog_editor`, `catalog_admin`, `fulfillment`, `user_admin`, `support_agent`. See `docs/permissions.md` for the full catalog.

### Schema migrations

Services migrate their own schema inline on startup (no separate migration tool). Order and product use `ALTER TABLE … ADD COLUMN IF NOT EXISTS` for additive changes and silently continue on error for those. Breaking schema changes are not supported this way.

### In-memory fallbacks

Order, cart, payment, and support use PostgreSQL when their `DUPLI1_*_DB` env var is set; otherwise they fall back to an in-memory repository. Tests rely on this — no database needed unless testing Postgres-specific behavior.

## Key constraints

- **Currency: KRW only.** All `*_won` fields are whole Korean won. No fractional amounts.
- **Money fields are `*_won`** — in JSON, Go identifiers, and Postgres columns. Never `*_krw` (canonical only from 2026-09-09 to 09-14) or `*_cents`. Nothing emits the old names, but a few decoders still *accept* them, so writing one fails silently rather than loudly. Branches and docs cut in that window still say `*_krw`; treat them as stale. See `shared/pkg/money`.
- **No `go.work`.** Run and test from each service module directory.
- **nginx resolver:** `api/nginx.conf` must list only Docker's embedded DNS `127.0.0.11` in its `resolver` directive. Adding `10.0.0.2` (AWS VPC) causes ~50% of local requests to fail with `502`.
- **Promotional codes, not coupons.** Canonical everywhere: table `promotions`, `promotion_code`, `promotion.*` permissions, `/api/v1/products/promotions`, `domain.Promotion`. `discount_won` keeps its name. Definitions carry `benefit` and `conditions` JSONB and an enforced `expires_at`; `promotion_redemptions` is the usage ledger (reserve at checkout complete → consume on paid → release on a cancel before shipment, mirroring the stock rule). Order asks product to price a code against the cart rather than computing a discount itself. `single_user` codes need a `customer_promotions` entitlement, issued automatically on `user.registered` (`DUPLI1_WELCOME_PROMOTION_CODE`), by a manager, or by `product/cmd/backfill-welcome-promotion`; the entitlement grants access while the ledger still decides whether it has been spent. The sign-up campaign `WELCOME50` is seeded **inactive** — enabling it is a manager action, not a deploy. Renamed from `coupon` on 2026-09-16; for **one release** the old spellings are still accepted — the `coupon.*` permissions, the `/api/v1/products/coupons` and `/api/v1/coupons` routes, the `…/sessions/{id}/coupon` sub-route, and a `coupon_code` key that order still emits alongside `promotion_code`. Write only the new names; every remaining `coupon` in the tree is deliberate compatibility scaffolding listed in [docs/product-promotion-rename.md](docs/product-promotion-rename.md), which also says how to remove it.
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
| profile | 5439 | `profiles` | `dupli1` | `dupli1_dev` |
| support | 5440 | `support` | `dupli1` | `dupli1_dev` |

Seeded owner: `admin@dupli1.com` / `password`.

## Multi-service Docker image

The single root `Dockerfile` builds any service via a `SERVICE` build arg:
```bash
docker build --build-arg SERVICE=order -t dupli1-order .
```
Docker Compose sets this automatically per service definition.
