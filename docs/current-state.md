# Current Code State

Authoritative snapshot of what is implemented in the Dupli1 repository today.

## Overview

Dupli1 is a fashion bag marketplace backend: Go microservices behind an nginx gateway. Local dev uses Docker Compose; production uses AWS ECS on EC2, ALB, and Amazon RDS PostgreSQL.

| Area | Status |
|------|--------|
| Auth (login, JWT, fine-grained permissions) | Implemented |
| Product catalog (bags, images, PDP) | Implemented |
| Promotional codes | **Implemented** — percent **and fixed-₩** benefits, enforced `expires_at` (KST end-of-day), conditions JSONB (min spend, category, brand, price band, on-sale exclusion), redemption ledger with reserve → consume → release, once-per-customer, campaign caps, write-time validation, rate-limited public endpoints. single-user entitlements with a per-entitlement expiry, a customer wallet, manager issue/revoke, and automatic issue to new customers on `user.registered`. The sign-up campaign `WELCOME50` is **seeded inactive** — enable it in the admin to go live ([product-promo-referral-code-plan.md](product-promo-referral-code-plan.md)) |
| Currency | **KRW only** — product prices and `*_won` amounts are whole won ([payment-service.md](payment-service.md)) |
| Inventory (stock, reservations) | Implemented (PostgreSQL, owned by product) |
| Orders + checkout sessions | Implemented (PostgreSQL) |
| Shopping cart | Implemented (PostgreSQL) |
| Payments (NANO card + Bypass) | Implemented — see [payment-service.md](payment-service.md) |
| Payment methods | Credit card (NANO) + Bypass implemented; Bitcoin planned — see [payment-methods-plan.md](payment-methods-plan.md) |
| Notifications | Implemented (NATS → Telegram when configured); **subscriptions are not yet persisted in production** — the ECS task has no `DUPLI1_NOTIFICATION_DB`, see [dupli1-notification](#dupli1-notification) |
| Customer Telegram consultation bot | Partial — **`support`** walks the full consultation menu in place and hands off to staff: asking for a human opens an inquiry, records the transcript, and publishes `support.inquiry_opened`, which `notification` fans out to chats holding `alert_support` (silently outside service hours). Service hours are weekdays 10:00–22:00 KST. Staff work the queue from manage-web `/support` (대기 / 내 상담 / 완료): claim, reply, close, with a reply that never reached the shopper shown as 미전송 The storefront's floating button opens `@dupli1_support_bot` carrying the page the shopper came from ([support-telegram-bot.md](support-telegram-bot.md) Phases 0–6). Remaining: deployment — no token in Secrets Manager and no ECS task, so the bot is not reachable in production |
| Customer commerce profile + addresses | Implemented — own **`profile`** service (PostgreSQL), extracted from auth ([profile-service.md](profile-service.md), [auth-profile-extension-plan.md](auth-profile-extension-plan.md)); chat/analytics not started |
| Guest PDP views + recommendations | Implemented — in product |
| Manager settings (mutable store policy) | Sketch — see [manager-settings-api.md](manager-settings-api.md) |

## Repository layout

Services live in **per-service directories**, not `cmd/dupli1-*` / `pkg/*` at the repo root:

```text
auth/, profile/, product/, order/, cart/, payment/, notification/, support/   # each has cmd/ + pkg/
api/nginx.conf                                      # gateway
```

See [service-layout.md](service-layout.md) for details.

## Services

### dupli1-auth

- **Host port (Compose):** 18080 → container 8080
- **Stack:** Gin, PostgreSQL, Redis, optional NATS
- **Persistence:** `dupli1_db` on `postgres-auth`
- **Features:**
  - Login returns a **refresh token**; `POST /refresh` returns a short-lived **access token** (`token` field) plus a **rotated refresh token** (`refresh_token` field) — the token sent in is invalidated immediately
  - RS256 JWT + JWKS at `/api/v1/auth/.well-known/jwks.json`
  - Access tokens include `type: "access"`, `permissions`, and the user's login **`email`** (for downstream consumers such as NANO `compOrderMem`; not an authz claim); refresh tokens include `type: "refresh"`; both include a random `jti` so same-second issuances never collide
  - Fine-grained **permissions** stored on users (`users.permissions TEXT[]`); JWT access tokens include `permissions` claim
  - Permission constants and evaluation in `shared/pkg/permissions` (`github.com/elug3/dupli1/shared`)
  - Wildcards: `*`, `admin.*`, `{resource}.*` (e.g. `product.*`)
  - Account types: `customer`, `manager`, `service` only (`account_type`). `admin` is a permission tier (`admin.*`), not an account type — write APIs reject it.
  - Register: **temporary open customer signup** via `AUTH_OPEN_REGISTER` (default on); anonymous callers create `customer` only. Set `AUTH_OPEN_REGISTER=false` to require `user.create` again. Authenticated `user.create` still follows ABAC for other account types.
  - Auth ABAC hierarchy governs who may manage whom
  - User admin at `/api/v1/auth/users`; update via `PATCH …/permissions`
  - Customer commerce profile/addresses moved to **`profile`** service (`/api/v1/profile/me/…`); one-release gateway aliases keep `/api/v1/auth/me/profile` and `/api/v1/auth/me/addresses` — [auth-profile-extension-plan.md](auth-profile-extension-plan.md)
  - `DELETE /api/v1/auth/users/:id` (`user.delete`) writes `user.deleted` to a transactional **outbox** in the same Postgres transaction as the user row delete; the drain worker publishes to NATS so profile can drop owned PII. In-memory/tests publish first and refuse the delete if the broker rejects.
  - Owner seeded from `OWNER_EMAIL` / `OWNER_PASSWORD` (`permissions: ["*"]`, `account_type` `manager`)
  - Login lockout after 5 failed attempts for customers/managers, auto-expiring after 15 minutes; **admin and owner are never locked**
  - Deactivated/locked accounts are rejected on their very next authenticated request (not just next login/refresh) — `RequireAuth` re-checks account status on every call
  - `dupli1-web` service account: `permissions: ["user.create"]` (`DUPLI1_WEB_SERVICE_*`); seeded/synced on auth boot; ECS injects the shared Secrets Manager secret into auth + web (see [infra/terraform/README.md](../infra/terraform/README.md))
  - `dupli1-order` service account: `order.ship`, `order.status.update`, `inventory.reservation.manage`, `payment.cancel` (`DUPLI1_ORDER_SERVICE_*`); order refreshes a Bearer access token and calls product stock/promotions **and payment cancel** via **`DUPLI1_GATEWAY_URL`** (`httpstock` / `httppayment` / gateway paths)
  - Login/refresh rate-limited per IP via Redis; Gin trusts only RFC1918 proxy hops (`SetTrustedProxies`) so a client-supplied `X-Forwarded-For` can't spoof a fresh IP and bypass the limit
  - Session store falls back to in-memory (with background GC) when no Redis is configured, so `/logout` and refresh-token revocation still work on a single instance instead of silently no-op'ing
  - `user.registered` NATS publish is best-effort: a broker outage is logged and the account still registers
  - Structured **zerolog** logging (`event` field) for session paths, internal errors, and bootstrap — [auth-logging.md](auth-logging.md)
- **Tests:** `cd auth && go test ./...`

### dupli1-profile

- **Host port:** 8088
- **Stack:** stdlib HTTP, PostgreSQL, NATS
- **Persistence:** `profiles` on `postgres-profile`
- **Features:**
  - Customer commerce profile (`display_name`, `phone`) + saved shipping addresses at `/api/v1/profile/me/…`, extracted from `auth` — see [profile-service.md](profile-service.md)
  - One-release nginx aliases keep `/api/v1/auth/me/profile` and `/api/v1/auth/me/addresses` working during cutover
  - Self-service only: owner is the JWT `sub`, no dedicated permission (ABAC, same pattern as cart); foreign-user address access returns `404` not `403`
  - Max **10** addresses per user; at most one `is_default` per user enforced by a Postgres partial unique index
  - Subscribes to auth's `user.deleted` NATS event and cascade-deletes owned profile + addresses (no FK to auth's `users` table — different database)
  - No coupling to order/checkout: order's shipping snapshot is copied client-side from a chosen address, not fetched server-to-server
- **Auth:** Bearer JWT via `AUTH_JWKS_URL` (RS256 JWKS from auth), with `JWT_SECRET` HS256 fallback in dev
- **Known gap:** one-time data copy from auth's pre-extraction `customer_profiles`/`customer_addresses` tables has not run — addresses saved before cutover aren't visible through `profile` yet (see [auth-profile-extension-plan.md](auth-profile-extension-plan.md) Phase D)
- **Tests:** `cd profile && go test ./...`

### dupli1-product

- **Host port:** 8081
- **Stack:** stdlib HTTP, PostgreSQL, MinIO/S3
- **Persistence:** `products` on `postgres-product`
- **Features:**
  - Parent (style) + variant (SKU) model: search returns parents only (no color duplicates)
  - Bag merchandising taxonomy (`subCategory`, `style`, `target`) with public master catalog + product search filters
  - Price stored on parent product (`price` / `officialPrice`); variants inherit for cart JSON — [product-price-on-parent.md](product-price-on-parent.md)
  - Parent `attributes` string map (PDP memo; not searched) — [product-attributes.md](product-attributes.md)
  - Dual SKU identity + master dictionaries: [product-sku-system.md](product-sku-system.md) (ULID product `id` + `skuId`; human `sku`; `/api/v1/products/catalog/…`; Phase C enforces existing master codes on create)
  - Variant physical dimensions (`dimensions.widthMm` / `heightMm` / `depthMm`) distinct from letter `size`/`sizeCode` — [product-sku-dimensions.md](product-sku-dimensions.md)
  - Error wrapping: store-boundary sentinels + sanitized 500s — [product-error-wrapping.md](product-error-wrapping.md)
  - Public: `GET /api/v1/products` (optional `product.read` widens view; filters `q`, `category`, `subcategory`, `style`, `target`, `brand`, `color`, `size`, `material`, `tags`; `sort`/`order` — [product-rich-search.md](product-rich-search.md), [product-master-catalog.md](product-master-catalog.md)), `GET /api/v1/products/{id}` (parent + variants with per-variant `availableQty`/`inStock`; unique guest `viewCount` via `dupli1_guest` cookie; `soldCount` on reservation commit — [product-sold-count.md](product-sold-count.md); `wishlistCount`), wishlist add/remove/list, `GET /api/v1/products/{id}/recommendations` (content + popularity — [product-recommendations.md](product-recommendations.md)), `GET /api/v1/products/variants?sku_ids=` (batch public variant lookup), promotional code redeem
  - Admin: per-route permissions (`product.create`, `promotion.read`, …) — see [permissions.md](permissions.md); parent CRUD, variant CRUD at `/api/v1/products/{id}/variants`, images on variant or default variant
  - **Promotional codes** (`/api/v1/products/promotions`, table `promotions`, `promotion.*` permissions — renamed from `coupon` per [product-promotion-rename.md](product-promotion-rename.md); pre-rename paths and permissions still accepted for one release): `code`, `scope`, `benefit` and `conditions` JSONB, `expires_at` (**enforced**; authored as end-of-day KST), `max_redemptions`, `max_per_customer`, `terms`, `redemption_count`, `active`; PG + in-memory stores, seeds `SUMMER30`. The legacy `discount` fraction and free-text `expires` are still read so pre-Phase-2 rows price correctly, but the free-text column is display-only and never enforced. `POST …/promotions/redeem` is a public lookup and `POST …/promotions/evaluate` prices a code against a cart; both are rate-limited per IP and per customer, failing open. `reserve` / `consume` / `release` move the ledger and need `promotion.redeem` (order's service account). Definitions are validated on write, so a benefit outside `(0,1)` is refused rather than failing later at checkout. Remaining: single-user wallet and auto-issue — [product-promo-referral-code-plan.md](product-promo-referral-code-plan.md)
  - Stock and reservations at `/api/v1/products/inventory/*` (merged in from the former standalone `inventory` service; legacy `/api/v1/inventory/*` still aliased), keyed by a canonical ULID `SkuID` with `sku` and `by-sku-id/{skuId}` lookups both supported; reads are public, writes require `inventory.stock.write` or `inventory.reservation.manage`. Every variant gets a `stock_items` row on create (qty 0); orphans backfilled on inventory migrate — [product-stock-tracking-plan.md](product-stock-tracking-plan.md)
  - Protected routes validate RS256 via `AUTH_JWKS_URL`; authorization from `permissions` claim
  - Inline schema migration + variant backfill on startup; brand/color/size/edition master tables seeded on migrate
  - Structure review (Product vs sellable SKU, flatten Phase 0 locks): [product-structure-final-review.md](product-structure-final-review.md)
  - Plan: [product-variants-plan.md](product-variants-plan.md), [product-sku-system.md](product-sku-system.md), [product-flat-sellable-model-plan.md](product-flat-sellable-model-plan.md)
- **Tests:** `cd product && go test ./...`

### dupli1-order

- **Host port:** 8083
- **Persistence:** PostgreSQL (`orders` on `postgres-order`)
- **Features:**
  - Checkout sessions at `/api/v1/orders/checkout/sessions` (legacy `/api/v1/checkout/sessions` still aliased; see [checkout-session.md](checkout-session.md))
  - Order lifecycle at `/api/v1/orders` — statuses: `pending`, `paid`, `confirmed`, `in_transit`, `delivered`, `fulfilled`, `disputed`, `canceled`
  - List: `GET /api/v1/orders` (all — requires `order.read.all`); `GET /api/v1/orders?customer_id=` (ABAC). There is no `/orders/all` or `/orders/me`.
  - Consumes **`payment.succeeded`** (NATS) → `paid` (idempotent on `payment_id`; replays after ship/fulfill are no-ops); late payment on auto-`canceled` orders **re-reserves stock** and reopens the payment window before marking `paid`
  - Consumes **`payment.canceled`** (NATS) → cancels a still-`paid` order on a full refund (`remaining_won == 0`) only when `payment_id` matches; omitted `remaining_won`, partial refunds, pending, and already-shipped orders are skipped. Atomic `paid`+`payment_id` guard so a concurrent ship is not last-write-wins canceled.
  - **Paid cancel / refund:** `PUT /orders/{id}/status` `{ "status": "canceled" }` calls payment `POST /payments/{payment_id}/cancel` (forwards the operator Bearer, falls back to the order service account) **before** flipping the order; a PG rejection leaves the order unchanged. Pending cancel is local only. Manager cancel is also allowed from `confirmed`, `in_transit`, `delivered`, and `disputed` (refunds; does not restock once stock is committed at ship).
  - **Confirmation / customer cancel:** `POST /orders/{id}/confirm` (`order.status.update`) moves `paid` → `confirmed` (a real status, not just a timestamp). Managers must confirm within 2 hours of payment; a worker auto-confirms after that. Customer `POST /orders/{id}/cancel` (ABAC owner) refunds immediately before confirmation; from `confirmed` through `delivered` it records a cancel request instead. Manager `…/cancel/approve` refunds; `…/cancel/reject` keeps the order. Unanswered cancel requests auto-approve after 2 hours.
  - **Delivery / receipt / dispute:** `POST /orders/{id}/deliver` (`order.ship`) moves `in_transit` → `delivered`. Customer `POST /orders/{id}/receipt/confirm` (ABAC owner) moves `delivered` → `fulfilled`; `POST /orders/{id}/receipt/dispute` moves it to `disputed` instead. A manager resolves a dispute either way: `POST /orders/{id}/dispute/resolve` (`order.status.update`) closes it `fulfilled` with no refund, or `PUT /status` → `canceled` refunds it. A delivered order with no customer response is auto-fulfilled 14 days after delivery.
  - 5-minute unpaid `pending` expiry worker (skips when payment wins the race)
  - **Shipping fee:** flat per-order delivery charge via `DUPLI1_ORDER_SHIPPING_FEE_WON` (deprecated alias `DUPLI1_ORDER_SHIPPING_FEE_CENTS`; whole KRW, default 30000 = 30,000 KRW; set 0 for free). JSON / DB / Go field is `shipping_fee_won`. `total = subtotal - discount + shipping`; no free-shipping threshold; promotional codes discount goods only (shipping benefits are Phase 4 of [product-promo-referral-code-plan.md](product-promo-referral-code-plan.md)). Snapshotted on the checkout session when it opens; `complete` charges that quoted fee even if the configured amount changed mid-checkout. Direct `POST /orders` uses the current configured fee.
  - Publishes order events via transactional **outbox** (`order.created` / status updates); outbox drain worker
  - Live admin SSE at `GET /api/v1/orders/events` is **planned** — manage-web client + mock gateway exist; route not registered in order handler yet ([order-live-events.md](order-live-events.md))
  - Optional `Idempotency-Key` on `POST /api/v1/orders` (replay-safe create)
  - Checkout `complete` snapshots recipient + shipping address (optional prefill from auth profile)
  - Checkout `complete` uses atomic session claim — concurrent completes cannot create duplicate orders
  - `POST /api/v1/orders/{id}/ship` validates `confirmed` → `in_transit` **before** committing inventory (plan B); persists via atomic **`ShipIfConfirmed`** (`UPDATE … WHERE status = confirmed`) so a concurrent full refund cancel cannot last-write-wins ship a canceled order; **requires** `carrier` + `tracking_number` (fixed KR set: `cj`/`hanjin`/`lotte`/`logen`/`epost`/`other`; `carrier_note` required when `other`)
  - Calls product to reserve stock and redeem promotional codes
  - Order responses include optional `carrier`, `tracking_number`, `carrier_note` after ship
- **Auth:** Bearer JWT via `AUTH_JWKS_URL` (RS256 JWKS; HS256 fallback in dev). Storefront ABAC on `customer_id`; `order.create` / `order.read.all` bypass ABAC. Ship and deliver require `order.ship`; status changes, confirm, cancel-request approve/reject, and dispute resolve require `order.status.update`. Customer cancel, receipt confirm, and receipt dispute are owner ABAC.
- **Tests:** `cd order && go test ./...`

### dupli1-cart

- **Host port:** 8086
- **Persistence:** PostgreSQL (`cart` on `postgres-cart`)
- **Features:**
  - Persistent per-customer cart at `/api/v1/cart` (see [cart-service.md](cart-service.md))
  - Admin read at `/api/v1/cart/customers/{customer_id}` requires `cart.read` (legacy `/api/v1/carts/{customer_id}` still aliased)
  - Enriches lines from product (price, images, availability)
  - Upsert/replace reject when quantity exceeds available (`400` `insufficient_stock`)
- **Auth:** Bearer JWT via `AUTH_JWKS_URL` (RS256 JWKS from auth; access tokens only), with `JWT_SECRET` HS256 fallback in dev
- **Tests:** `cd cart && go test ./...`

### dupli1-payment

- **Host port:** 8087
- **Persistence:** PostgreSQL (`payments` on `postgres-payment`)
- **Features:**
  - **NANO** certified card PG when `NANO_*` credentials set; else `credit_card` is unavailable (501) and manager **Bypass** is used, including for local testing (see [payment-service.md](payment-service.md))
  - Default payment currency: **`krw` only** (whole won; `*_won` fields are KRW minor units = won)
  - Publishes **`payment.succeeded`** via transactional **outbox** (soft-success complete; drain + reconcile workers)
  - **Cancel / refund:** `POST /api/v1/payments/{id}/cancel` (`payment.cancel`, staff-only) calls NANO `/api/payment/cancel.io`; full or partial (`amount_won`), `Idempotency-Key` honored. Concurrent cancels serialize on a row lock (`SELECT … FOR UPDATE` / in-memory mutex) so two in-flight requests cannot both call NANO. Publishes **`payment.canceled`** via outbox; order cancels a still-`paid` matching order on a full refund, notification alerts ops.
  - **NANO checkout bridge** (`GET /nano/checkout`) POSTs JSON + `API_KEY` server-side and retries empty/5xx PG blips. Empty/unusable PG 2xx after retries is `checkout_failed` (retryable); 3xx is forwarded as an absolute Location. `receiveUrl` is percent-encoded and carries a payment-scoped `nano_mac` so v2.7 unsigned returns can succeed. `compOrderMem` is the customer email (access-token `email` claim) capped at 30 bytes. The browser never POSTs to `request.io` (see [payment-service.md](payment-service.md))
  - Publishes **`payment.callback_rejected`** (direct, not via outbox) when the PG reported approval and dupli1 refused the callback — notification alerts ops that a card may be charged with no paid order
  - **Methods:** `method` on create — `credit_card` (NANO; 501 when unconfigured), `bypass` (requires `payment.bypass`; succeeds immediately), `bitcoin` (501). See [payment-methods-plan.md](payment-methods-plan.md)
- **Auth:** Bearer JWT on customer routes; ownership ABAC unless `payment.create` / `payment.read.all`. Bypass requires `payment.bypass`
- **Tests:** `cd payment && go test ./...`

### dupli1-notification

- **Host port:** 8084
- **Features:** NATS subscriber (`order.*`, `product.*`, `payment.canceled`, `payment.callback_rejected`); Telegram ops alerts in Korean; webhook or `getUpdates` stores `chat_id` in PostgreSQL; manager API to accept users/chats. See [notification-telegram-bot.md](notification-telegram-bot.md)
- **Database:** PostgreSQL `notifications` (`DUPLI1_NOTIFICATION_DB`; local port 5438). Without it the service falls back to an in-memory subscription repository that does not survive a restart — which is what production runs today, see **Production** below
- **Alert routing:** every destination, not one — the union of the env chat IDs (`TELEGRAM_ORDER_CHAT_ID` / `TELEGRAM_PRODUCT_CHAT_ID`) and every accepted subscription carrying `alert_order` / `alert_product`, each chat once. Env no longer overrides the database, so accepting a chat in manage-web takes effect even where the env fallback is set
- **Registration:** only an explicit `/start` registers a chat, and only if it is not already known, so the pending-registration ack is sent once. An env-allowlisted user is welcomed without a pending row; a `/start` from a known chat refreshes its stored label and username. Outbound alerts and `/start` replies are **Korean**.
- **Delivery:** sends retry 3× with a doubling backoff on a timeout, 5xx or 429 (never on a 4xx); messages are truncated to Telegram's 4096-character limit at a tag/entity boundary. A send that still fails is logged — core NATS does not redeliver, so the alert is then dropped
- **Webhook:** `TELEGRAM_WEBHOOK_SECRET` is required at startup whenever `TELEGRAM_WEBHOOK_URL` is set, and the secret is compared in constant time. An authenticated update is acknowledged **before** it is processed (work continues under the service context, 30s budget), so Telegram does not redeliver an update already in flight; the backlog drain runs before `setWebhook`, since Telegram answers `getUpdates` with `409` while a webhook is active
- **More than one task:** NATS subscriptions use the queue group `dupli1-notification`, so an event is delivered once rather than once per task; the cached allowlist is rebuilt from the database every 30s so an accept served by one task reaches the others. Polling is **not** leader-elected — a second poller on the same bot token gets `409` and backs off 30s, which covers a deploy overlap but not permanent multi-task polling
- **`/health`** reports each wired dependency (`postgres` ping, `nats` connection) and `status: degraded` when one fails, **always with HTTP 200** — nothing probes it today, so a 503 would signal nothing while risking a restart loop for whoever wires a probe to it later. Results cached 5s; probe errors are logged, not returned, since the route is unauthenticated
- **Production:** bot token from Secrets Manager. **Subscriptions are not persisted:** the ECS task definition carries no `DUPLI1_NOTIFICATION_DB` (confirmed on `dupli1-notification:5`), so the service runs on the in-memory repository and every manager accept/reject is lost on deploy. Terraform is wired for the switch — `var.notification_db_url_secret_arn` defaults to `""` and the secret is omitted while it is empty — and needs the `notifications` database created, its URL stored as `dupli1/production/notification-db-url`, and the variable set. Production also runs **polling**, not webhook mode: neither `TELEGRAM_WEBHOOK_URL` nor `TELEGRAM_WEBHOOK_SECRET` is set on the task
- **Status:** Health + event dispatch + Telegram manager API (no outbound email/SMS yet)

### dupli1-proxy

- **Host ports:** 8080 and 80 (HTTP), 443 exposed but TLS not configured in nginx
- **Config:** [api/nginx.conf](../api/nginx.conf) locally, [api/nginx.prod.conf](../api/nginx.prod.conf) for the single-EC2 Compose overlay, [api/nginx.ecs.conf](../api/nginx.ecs.conf) for production ECS (baked into `api/Dockerfile.ecs`). All three route the same API prefixes
- **Health:** `GET /gateway/health` → `ok`

## Data stores

| Store | Used by | Local |
|-------|---------|-------|
| PostgreSQL `dupli1_db` | auth | `postgres-auth:5432` |
| PostgreSQL `products` | product (also stock/reservations) | `postgres-product:5433` |
| PostgreSQL `orders` | order | `postgres-order:5435` |
| PostgreSQL `cart` | cart | `postgres-cart:5436` |
| PostgreSQL `payments` | payment | `postgres-payment:5437` |
| PostgreSQL `notifications` | notification | `postgres-notification:5438` |
| PostgreSQL `profiles` | profile | `postgres-profile:5439` |
| MinIO `product-images` | product (local) | `minio:9000` via gateway `/product-images/` |
| S3 + CloudFront OAC | product (AWS) | `images.dupli1.com` — see [product-images-browser-access.md](product-images-browser-access.md) |
| Redis | auth | `redis:6379` (in Compose) |
| NATS | auth, product, order, payment, notification, profile | `127.0.0.1:4222` in Compose (`--auth` / `NATS_TOKEN`); Cloud Map `nats.dupli1.local` in ECS |

## API surface (summary)

| Service | Public | Authenticated |
|---------|--------|---------------|
| auth | login, refresh, logout, JWKS | register (`user.create` or open register), me, user admin (permissions, delete) |
| profile | health only | commerce profile + saved addresses (ABAC self-service) |
| product | health, product search/PDP, promotional code redeem, inventory reads | product/promotional code CRUD (per permission), image upload, inventory writes (`inventory.stock.write`, `inventory.reservation.manage`) |
| order | health only | orders (list all / by customer), checkout (ABAC + permissions), ship (`order.ship`) |
| cart | health only | own cart; admin read (`cart.read`) |
| payment | health only | payments (ABAC + permissions); Bypass (`payment.bypass`); cancel/refund (`payment.cancel`) |
| notification | health, Telegram webhook | Telegram subscriptions (`notification.telegram.read` / `notification.telegram.manage`) |

Full reference: [api.md](api.md). Route index: [endpoints.md](endpoints.md). Permission spec: [permissions.md](permissions.md).

## Go modules

| Module | Path |
|--------|------|
| `github.com/elug3/dupli1` | root stub |
| `github.com/elug3/dupli1/auth` | `auth/` |
| `github.com/elug3/dupli1/profile` | `profile/` |
| `github.com/elug3/dupli1/product` | `product/` |
| `github.com/elug3/dupli1/order` | `order/` |
| `github.com/elug3/dupli1/cart` | `cart/` |
| `github.com/elug3/dupli1/payment` | `payment/` |
| `github.com/elug3/dupli1/notification` | `notification/` |
| `github.com/elug3/dupli1/shared` | `shared/` (permissions library) |

## Known gaps

1. **Local TLS** — certs in `certs/` are not wired into nginx; gateway is HTTP only
2. **Notification** — Telegram ops alerts only; no email/SMS. In production the service still runs on the in-memory subscription repository (no `DUPLI1_NOTIFICATION_DB` on the ECS task), so manager accept/reject decisions are lost on every deploy; terraform is ready and waiting on the Secrets Manager entry
3. **No migrations directory** — product migrates inline; auth uses bootstrap DDL
4. **Planned packages not started** — user, chat, analytics (beyond `shared/pkg/permissions`)
5. **Quality/performance** — see [quality-performance-review.md](quality-performance-review.md); money-path Criticals (C1 pricing, H7 JWT) are fixed — remaining items in [TODO.md](TODO.md)
6. **v1.0 vs v1.1 vs v1.2** — **v1.0 postponed** (2026-07-27) until all checklist items in [v1.0-release-spec.md](v1.0-release-spec.md) are done; narrative: [v1-release-plan.md](v1-release-plan.md); v1.1 starts after v1.0 tags: [v1.1-release-plan.md](v1.1-release-plan.md)
7. **Production JWT signing key** — **done**. Secret `dupli1/production/jwt-private-key` is injected as `JWT_PRIVATE_KEY` on ECS auth; `GET /api/v1/auth/settings` reports `features.ephemeral_jwt_key: false` (checklist A6).
8. **Legacy API path aliases** — canonical `/api/v1/{service}/…` paths are documented; the old top-level prefixes stay registered until the frontends migrate ([TODO.md](TODO.md))

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
