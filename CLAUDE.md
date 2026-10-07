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

# Postgres-backed tests — skipped when POSTGRES_URL is unset, so run them
# explicitly. CI supplies one for auth, product and order.
cd order && POSTGRES_URL=postgres://dupli1:dupli1_dev@localhost:5435/orders?sslmode=disable go test ./...
cd product && POSTGRES_URL=postgres://dupli1:dupli1_dev@localhost:5433/products?sslmode=disable go test ./...

# Build a service binary
cd auth && go build ./cmd/

# Run locally (full stack)
sudo dockerd >/tmp/dockerd.log 2>&1 &   # daemon does not autostart on this VM
sudo docker compose up --build

# End-to-end money path smoke test (stack must be running)
BASE=http://localhost:8080 scripts/smoke-money-path.sh

# Sign-up campaign dry run — auto-issue, min spend, consume, release
# (stack must be running; enables the campaign and restores it on the way out)
BASE=http://localhost:8080 scripts/smoke-promotion-campaign.sh
```

**Docker note:** all `docker`/`docker compose` commands need `sudo` on this VM. The `fuse-overlayfs` storage driver is configured; standard overlayfs does not work here.

After editing `api/nginx.conf` or `api/gateway/*.conf`, rebuild the proxy only:
```bash
sudo docker compose up -d --build dupli1-proxy
```

Gateway route test (every nginx config through real nginx, needs Docker; CI job `gateway`):
```bash
DOCKER="sudo docker" api/gateway/test.sh
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
| `shared/pkg/events` | NATS subject constants + payload structs for cross-service events (`order.*`, `payment.succeeded`, `product.*`, `support.inquiry_opened`, `support.message_created|inquiry_updated`); one canonical contract per publisher/subscriber pair instead of redeclaring subject strings and payload shapes on each side |
| `shared/pkg/pgsslmode` | Picks `sslmode` for a Postgres connection string (local/docker hosts → `disable`, everything else including RDS → `require`); used by every service's DB bootstrap so the local-hostname list can't drift out of sync per service again |
| `shared/pkg/natspublisher` | JSON-marshaling NATS event publisher (`New`, `Publish`, `Close`), used by `auth`, `order`, `product`, and `payment` |
| `shared/pkg/authmiddleware` | Bearer-token HTTP middleware (`RequireAuth`, `OptionalAuth`) parameterized by `authjwt.AccessTokenValidator` and a per-service error-response callback, so each service keeps its own error body shape; used by `cart`, `order`, `payment`, `notification`, `product` |
| `shared/pkg/telegram` | Telegram Bot API client — `Client` (send/reply with retry, backoff and bot-token redaction in errors), wire types (`Update`, `Message`, `Chat`, `User`, `CallbackQuery`), inline-keyboard menus (`ReplyMenu`, `EditMessageText`, `AnswerCallback`), `SendSilent` for an alert that should queue rather than interrupt, webhook registration (`WithAllowedUpdates` opts a bot into `callback_query`; the default stays `message` only), `GetUpdates` polling (`RunPoller`/`DrainUpdates` over a `Handler`), HTML escaping and 4096-char-safe truncation. The `AccessPolicy` interface is the client's only view of who may be messaged, so each bot keeps its own policy; used by `notification` (ops alerts) |
| `shared/pkg/serviceaccount` | Service account names (`dupli1-order`, `dupli1-web`) auth stamps into the `service_name` claim and internal routes allowlist via `authjwt.Claims.CalledBy`. No dependencies, so auth imports it without the JWKS validator |
| `shared/pkg/productclient` | HTTP client for product's variant-lookup endpoint, returning a superset `Variant`; used by `cart` and `order`, each mapping only the display field it needs (`Color` vs `ProductName`) into its own local `ports.VariantInfo` |
| `shared/pkg/reportperiod` | KST report periods (Monday weeks, calendar months): parses `granularity`/`from`/`to`, lists periods and buckets a time into one; used by `order` (sales report) and `auth` (sign-up report) so their periods line up |
| `shared/pkg/sentrymon` | Sentry reporting, off unless `SENTRY_DSN` is set: `Init(service)` from each `cmd/main.go` (also mirrors the stdlib logger to Sentry Logs), `Handler` around each service's HTTP handler (panics and `5xx` responses become issues; SSE still flushes). See [docs/sentry-monitoring.md](docs/sentry-monitoring.md) |

### Service ownership

| Service | Framework | Key responsibility |
|---------|-----------|--------------------|
| `auth` | Gin | RS256 JWT + JWKS, fine-grained permissions, user admin. Customer profile + saved addresses code still lives here too (not yet removed — see [docs/auth-profile-extension-plan.md](docs/auth-profile-extension-plan.md) Phase D) but `profile` is now the wired service for that traffic |
| `product` | stdlib `net/http` | Parent-style + variant(SKU) catalog, images (MinIO/S3), stock & reservations (merged from former `inventory` service) |
| `order` | stdlib `net/http` | Checkout sessions, order lifecycle, transactional outbox → NATS |
| `cart` | stdlib `net/http` | Persistent per-customer cart; enriches lines from product |
| `payment` | stdlib `net/http` | NANO card / manager Bypass (also the local/dev testing path); publishes `payment.succeeded` via outbox |
| `notification` | stdlib `net/http` | NATS subscriber → Telegram ops alerts. Chats opt into each alert class separately (`alert_order`, `alert_product`, `alert_support`) and can mute single order/product messages inside a class (`muted_events`, e.g. keep `order.paid`, drop `order.created`); customer inquiry handoffs go only to chats that asked for them, with no env fallback |
| `support` | stdlib `net/http` | Customer-facing Telegram consultation bot: menu router, conversation state, handoff to staff. **Separate bot and token from `notification`'s ops bot** (`TELEGRAM_SUPPORT_*`, never `TELEGRAM_*`) — one token owns one update stream. Phases 0–4 of [docs/support-telegram-bot.md](docs/support-telegram-bot.md): the menu tree routes, conversations, inquiries and transcripts persist in PostgreSQL (`support`), canned answers are seeded insert-if-absent so staff edits survive a deploy, and asking for a human opens an inquiry and publishes `support.inquiry_opened` for `notification` to fan out. Service hours are weekdays 10:00–22:00 KST (config-driven; holidays are staff behavior, not code). Phase 5 adds the manager inbox (`support.read|reply|manage`, `support_agent` bundle) and the manage-web `/support` tab, where staff claim, reply and close; an undeliverable reply is stored and shown as 미전송. Phase 6 points the storefront button at `@dupli1_support_bot`. Phase 7 (production) is written but **not applied** — Terraform, the `/api/v1/support/` gateway route, the CI rows and a 24h purge that replaces message bodies older than `DUPLI1_SUPPORT_MESSAGE_RETENTION_DAYS` (default 180) with a placeholder, keeping the record and dropping the words. **Web consultation chat** ([docs/support-web-chat.md](docs/support-web-chat.md)) is a second channel in the same service and inbox: signed-in shoppers only, one conversation per account (`chat_id` `web:<customer id>`), product (by `sku_id`) and order references checked through `DUPLI1_GATEWAY_URL` with the caller's own token, SSE frames that carry ids only (fanned out across replicas by a broadcast NATS subscription), a reply-notice email after 5 minutes unread that never holds the reply text (`DUPLI1_SUPPORT_SMTP_*`), and `user.deleted` purging the shopper's words. `support_agent` includes `order.read.all` for the context panel. **Product questions** (상품 문의, [docs/support-product-questions.md](docs/support-product-questions.md)) are a third surface: **private** per-product questions (only the asker and staff see them, no public Q&A), asked by signed-in shoppers about a variant (`sku_id`), answered and hidden from the console (`support.reply`), alerting staff on `support.inquiry_opened` with `channel: "product_question"`, and purged with transcripts |
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

Card payments carry a surcharge (`DUPLI1_ORDER_CARD_SURCHARGE_BPS`, default 1000 = 10%) on goods after discounts plus delivery. Order prices it at checkout complete from the order's `payment_method` (`credit_card` by default; `bypass` needs `payment.bypass` and has none) and stores it as `card_surcharge_won` inside `total_won`, so payment and refunds just use the total; payment refuses a card checkout on a bypass-priced order.

Order calls product stock/promotions via the internal nginx gateway (`DUPLI1_GATEWAY_URL`), not direct service URLs. Pricing is resolved server-side — client `unit_price_won` is ignored.

Event flow: `payment.succeeded` (NATS, published by payment outbox) → order marks `paid`. `POST /orders/{id}/ship` → commits inventory reservation → `in_transit`.

Both order and payment use a **transactional outbox** pattern: event rows are written in the same DB transaction as the state change, then a drain worker publishes to NATS. This makes state changes the source of truth — NATS failures are retried.

The admin console's **live order stream** (`GET /api/v1/orders/events`, SSE, `order.read.all`) is fed from that outbox: every replica holds a *broadcast* (non-queue-group) NATS subscription to `order.*` and relays each change, as `GET /orders/{id}` presents it, to the streams it holds (`order/pkg/livefeed`). A stream ends when its token expires; the client reconnects with `Last-Event-ID` and replays. A new order subject only reaches the console once it is added to `livefeed`'s relayed set. See [docs/order-live-events.md](docs/order-live-events.md).

### Auth token flow

`POST /login` → `{ "refresh_token": "..." }`. Call `POST /refresh` with that token → `{ "token": "<access_jwt>", "refresh_token": "<new_jwt>" }`. Send as `Authorization: Bearer <token>` on protected routes. Access tokens carry a `permissions` string array claim (no `roles`), plus `account_type` and, for service accounts, `service_name`.

**Service accounts have no password** and cannot sign in to the storefront or manage-web: `/login` refuses them through every client, `/refresh` refuses their old tokens, and setting a password on one is a `422`. They authenticate with an **API key**: `POST /api/v1/auth/token` with `Authorization: ApiKey dk_live_…` → `{ "token", "token_type", "expires_in" }`, no refresh token (exchange again near expiry). Internal listener only. Keys come from `DUPLI1_{ORDER,WEB}_SERVICE_API_KEY` — required whenever the matching `*_SERVICE_EMAIL` is set; changing one rotates the key, unsetting revokes it — or are minted by the owner via `/users/{id}/api-keys`; only a SHA-256 is stored. See [docs/auth-service-api-keys.md](docs/auth-service-api-keys.md).

`POST /login` takes a `client` — `storefront` (dupli1-web: customer, manager) or `manage` (manage-web: manager only); `service` is still accepted but admits nobody — and answers `403 account_type_not_allowed` with a message to show when the account type does not belong there. The web apps only display it; the rule lives in auth. Optional for one release while callers roll over.

Refresh tokens rotate on every use: `/refresh` invalidates the token it was given and returns a new one, which the caller must store and use next time. Reusing an already-rotated refresh token fails with `401`.

`/refresh` answers `401` **only** when the token itself is bad (invalid, expired, revoked, or the account is gone/locked). If the refresh-token ledger or the user lookup is unreachable it answers `503 refresh unavailable`, because a client cannot tell the two apart and every BFF discards the token on a `401` — flattening a Redis blip into `401` signed out every customer and operator on a routine deploy. Clients must keep the token and retry on `503`.

### Authorization

Fine-grained permissions (`{resource}.{action}`, e.g. `product.create`, `order.ship`). Wildcards: `*` (owner), `admin.*`, `{resource}.*`. Storefront customers use ABAC (JWT `sub` must match resource owner) with no explicit permission required.

Internal APIs (product's promotion `reserve|consume|release` and inventory reservations) also require the caller to be the named service account (`dupli1-order`), so no wildcard or owner token reaches them. They are also off the public path: the gateway answers `404` for them on `:80` (what the edge/ALB reaches) and serves them only on its internal `:8081` listener, where order's `DUPLI1_GATEWAY_URL` points. A new internal route goes in that regex too, in `api/gateway/routes.conf`. See `docs/permissions.md` → Internal APIs.

Key bundles: `catalog_editor`, `catalog_admin`, `fulfillment`, `user_admin`, `support_agent` (`support.read|reply` + `order.read.all`). See `docs/permissions.md` for the full catalog.

### Schema migrations

Services migrate their own schema inline on startup (no separate migration tool). Order and product use `ALTER TABLE … ADD COLUMN IF NOT EXISTS` for additive changes and silently continue on error for those. Breaking schema changes are not supported this way.

### In-memory fallbacks

Order, cart, payment, and support use PostgreSQL when their `DUPLI1_*_DB` env var is set; otherwise they fall back to an in-memory repository. Tests rely on this — no database needed unless testing Postgres-specific behavior.

## Key constraints

- **Currency: KRW only.** All `*_won` fields are whole Korean won. No fractional amounts.
- **Money fields are `*_won`** — in JSON, Go identifiers, and Postgres columns. Never `*_krw` (canonical only from 2026-09-09 to 09-14) or `*_cents`. Nothing emits the old names, but a few decoders still *accept* them, so writing one fails silently rather than loudly. Branches and docs cut in that window still say `*_krw`; treat them as stale. See `shared/pkg/money`.
- **No `go.work`.** Run and test from each service module directory.
- **Gateway routes live in one file.** `api/gateway/routes.conf` is the route table every environment shares; `api/nginx.conf` (local), `api/nginx.ecs.conf` (the proxy image), `deploy/venus/nginx-gateway.conf` (VENUS) and `api/nginx.prod.conf` (single-EC2) are thin wrappers holding only the resolver, listeners, extra locations and each service's upstream as a `$gw_<service>` variable (ECS and VENUS share `api/gateway/hosts.dupli1.local.conf`). Add or change a route in `routes.conf`, never in a wrapper; a new service also needs its `$gw_` variable in each wrapper. `api/gateway/test.sh` checks them all.
- **nginx resolver:** `api/nginx.conf` must list only Docker's embedded DNS `127.0.0.11` in its `resolver` directive. Adding `10.0.0.2` (AWS VPC) causes ~50% of local requests to fail with `502`.
- **Promotional codes, not coupons.** Canonical everywhere: table `promotions`, `promotion_code`, `promotion.*` permissions, `/api/v1/products/promotions`, `domain.Promotion`. `discount_won` keeps its name. Definitions carry `benefit` and `conditions` JSONB and an enforced `expires_at`; `promotion_redemptions` is the usage ledger (reserve at checkout complete → consume on paid → release on a cancel before shipment, mirroring the stock rule). Every promotion discounts the **whole order** (the goods subtotal, never shipping): conditions decide only whether a code applies, and `apply_to: eligible_lines` is refused since 2026-09-30. Order asks product to price a code against the cart rather than computing a discount itself, and sends only each line's `{sku_id, sku, quantity, unit_price_won}` — conditions on a line's category, brand, parent or sale state are resolved by the evaluator from product's own catalog, so they cannot be claimed by a caller. `single_user` codes need a `customer_promotions` entitlement, issued automatically on `user.registered` (to every definition a manager set `auto_issue: "user_registered"` — no code is compiled in, seeded or configured by env var; campaigns are registered at runtime through the admin API), by a manager, or by `product/cmd/backfill-welcome-promotion -code <CODE>`; the entitlement grants access while the ledger still decides whether it has been spent. A `single_user` definition with `apply_mode: "auto"` is a **customer tier** (VIP, a private tier): the entitlement is membership, and order applies the member's best tier to every order without a code, stacked under the one code (`tier_promotion_code` / `tier_discount_won`, inside `discount_won`); a tier is never typed, never in the wallet, and has no ledger or cap. `auto_issue` decides who is handed a code, `active` whether it can be spent — the sign-up campaign `WELCOME50` is **inactive** and enabling it is a manager action, not a deploy. Renamed from `coupon` on 2026-09-16; for **one release** the old spellings are still accepted — the `coupon.*` permissions, the `/api/v1/products/coupons` and `/api/v1/coupons` routes, the `…/sessions/{id}/coupon` sub-route, and a `coupon_code` key that order still emits alongside `promotion_code`. Write only the new names; every remaining `coupon` in the tree is deliberate compatibility scaffolding listed in [docs/product-promotion-rename.md](docs/product-promotion-rename.md), which also says how to remove it.
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
