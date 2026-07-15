# Manager Settings API (sketch)

Design sketch for **mutable** manager-controlled application settings.

**Status:** Sketch only — not implemented.

**Related:** [permissions.md](permissions.md), [endpoints.md](endpoints.md), [current-state.md](current-state.md), [payment-service.md](payment-service.md), [product-guest-views-plan.md](product-guest-views-plan.md).

---

## Distinction from service introspection

| Surface | Paths | Auth | Mutability |
|---------|-------|------|------------|
| **Service introspection** | `GET /settings`, `GET /api/v1/<service>/settings` | Public | Read-only (runtime snapshot, no secrets) |
| **Manager settings** (this doc) | `GET|PATCH /api/v1/settings/...` | Bearer + `settings.*` | Persisted, manager-editable |

Service introspection stays as-is for ops/sidecars. Manager settings is a separate, authenticated control plane for `dupli1-manage-web`.

---

## Hosting

**Recommendation (v1):** own the API in **auth** (already the identity/control plane), persist in `dupli1_db`, publish `settings.updated` on NATS so other services reload overrides.

Alternative later: dedicated `dupli1-settings` service if the document grows large.

Gateway: add `location /api/v1/settings` → auth (or settings service).

---

## Permissions

| Permission | Grants |
|------------|--------|
| `settings.read` | Read all settings sections |
| `settings.update` | Patch any section (non-secret fields) |
| `settings.secrets.update` | Rotate/update secret references only (owner / `*`) — optional v2 |
| `settings.*` | Both read and update |

`admin.*` does **not** imply `settings.*` (settings are store-wide policy, not user-admin). Owner `*` implies all.

---

## Conventions

- All paths under `/api/v1/settings`.
- `GET` returns the full section document (defaults merged with overrides).
- `PATCH` is **JSON Merge Patch** (RFC 7396): omit = leave unchanged; `null` = reset field to platform default.
- Every response includes:
  - `updated_at` — last change time
  - `updated_by` — user id (nullable for seeded defaults)
  - `etag` — opaque version for `If-Match` optimistic concurrency
- Secrets are **never** returned. Writable secret fields accept a vault reference or “set” marker; reads show `configured: true|false` only.
- Validation errors: `400` with `{ "error": "...", "fields": { "path": "reason" } }`.

---

## Endpoint index

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/settings` | `settings.read` | Aggregate summary (all sections, compact) |
| `GET` | `/api/v1/settings/{section}` | `settings.read` | Full section document |
| `PATCH` | `/api/v1/settings/{section}` | `settings.update` | Merge-patch section |
| `GET` | `/api/v1/settings/features` | `settings.read` | Feature flags only (alias of `features` section) |
| `PATCH` | `/api/v1/settings/features` | `settings.update` | Toggle feature flags |
| `POST` | `/api/v1/settings/{section}/reset` | `settings.update` | Reset section to platform defaults |
| `GET` | `/api/v1/settings/audit` | `settings.read` | Recent settings change events |

**Sections:** `store`, `catalog`, `pricing`, `inventory`, `checkout`, `orders`, `cart`, `payments`, `notifications`, `security`, `features`.

---

## 1. Aggregate

### `GET /api/v1/settings`

Compact overview for the manage-web Settings home.

```json
{
  "sections": {
    "store": { "etag": "s1", "updated_at": "2026-07-14T12:00:00Z" },
    "features": { "etag": "f3", "updated_at": "2026-07-14T11:00:00Z" }
  },
  "integrations": {
    "stripe": { "configured": true, "mode": "live" },
    "s3": { "configured": true },
    "telegram": { "configured": false },
    "nats": { "configured": true }
  }
}
```

`integrations.*.configured` is derived from env/secrets — not writable here.

---

## 2. Store

### `GET|PATCH /api/v1/settings/store`

```json
{
  "store_name": "Dupli1",
  "currency": "usd",
  "locale_default": "en",
  "maintenance_mode": false,
  "maintenance_message": "",
  "public_base_url": "https://dupli1.com",
  "manage_base_url": "https://manage.dupli1.com",
  "etag": "…",
  "updated_at": "…",
  "updated_by": "…"
}
```

| Field | Notes |
|-------|-------|
| `currency` | ISO 4217 lowercase; today code hardcodes `usd` |
| `maintenance_mode` | Storefront shows closed page when true |

---

## 3. Catalog

### `GET|PATCH /api/v1/settings/catalog`

```json
{
  "default_product_status": "draft",
  "require_image_on_publish": true,
  "max_images_per_variant": 8,
  "max_image_bytes": 10485760,
  "allow_public_draft_preview": false,
  "etag": "…"
}
```

Entity CRUD (products, variants, images) stays on existing product APIs. This section is **policy only**.

---

## 4. Pricing & promotions policy

### `GET|PATCH /api/v1/settings/pricing`

```json
{
  "allow_selling_below_cost": false,
  "coupon_stacking": false,
  "coupon_defaults": {
    "min_subtotal_cents": 0,
    "max_redemptions": null,
    "require_active_product": true
  },
  "etag": "…"
}
```

Coupon entity CRUD remains ` /api/v1/coupons`. These fields gate create/update validation and checkout redeem rules.

---

## 5. Inventory

### `GET|PATCH /api/v1/settings/inventory`

```json
{
  "low_stock_threshold": 3,
  "reservation_ttl_minutes": 30,
  "auto_release_expired_reservations": true,
  "public_stock_reads": true,
  "etag": "…"
}
```

| Field | Notes |
|-------|-------|
| `reservation_ttl_minutes` | Align with checkout session TTL when possible |
| `public_stock_reads` | Today inventory GET is public; flip to require auth later |

---

## 6. Checkout

### `GET|PATCH /api/v1/settings/checkout`

```json
{
  "session_ttl_minutes": 30,
  "guest_checkout_enabled": false,
  "max_line_items": 50,
  "max_quantity_per_line": 10,
  "require_coupon_active": true,
  "etag": "…"
}
```

Replaces hardcoded `DefaultCheckoutTTL` (30m) in order service.

---

## 7. Orders

### `GET|PATCH /api/v1/settings/orders`

```json
{
  "unpaid_cancel_minutes": 5,
  "allow_cancel_when_paid": false,
  "require_tracking_on_ship": false,
  "default_list_page_size": 50,
  "etag": "…"
}
```

| Field | Notes |
|-------|-------|
| `unpaid_cancel_minutes` | Replaces hardcoded 5m payment window |
| `allow_cancel_when_paid` | Policy only; refunds still need payment API |

**Companion domain API (not settings, but required for manage UI):**

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/orders` | `order.read.all` | Global queue — drop required `customer_id` when caller has `order.read.all`; support `?status=` |

---

## 8. Cart

### `GET|PATCH /api/v1/settings/cart`

```json
{
  "guest_cart_enabled": false,
  "enforce_stock_on_add": false,
  "merge_guest_cart_on_login": true,
  "etag": "…"
}
```

Aligns with planned guest-cart work (`docs/cart-service.md`, guest-views plan).

---

## 9. Payments

### `GET|PATCH /api/v1/settings/payments`

```json
{
  "provider": "stripe",
  "mode": "live",
  "success_url": "https://dupli1.com/checkout/success",
  "cancel_url": "https://dupli1.com/checkout/cancel",
  "public_base_url": "https://api.dupli1.com",
  "stripe": {
    "secret_key_configured": true,
    "webhook_secret_configured": true
  },
  "refunds_enabled": false,
  "etag": "…"
}
```

**PATCH body (manager-writable):**

```json
{
  "mode": "test",
  "success_url": "https://staging.dupli1.com/checkout/success",
  "cancel_url": "https://staging.dupli1.com/checkout/cancel",
  "refunds_enabled": true
}
```

Secret rotation stays in AWS Secrets Manager / env for v1. Optional v2:

```http
PUT /api/v1/settings/payments/secrets
Permission: settings.secrets.update
{ "stripe_secret_key": "sk_…", "stripe_webhook_secret": "whsec_…" }
```

Response never echoes secret values — only `configured: true`.

---

## 10. Notifications

### `GET|PATCH /api/v1/settings/notifications`

```json
{
  "channels": {
    "telegram": { "enabled": true, "configured": true },
    "email": { "enabled": false, "configured": false },
    "sms": { "enabled": false, "configured": false }
  },
  "events": {
    "order.paid": { "telegram": true, "email": false },
    "order.in_transit": { "telegram": true, "email": false },
    "order.canceled": { "telegram": true, "email": false },
    "inventory.low_stock": { "telegram": false, "email": false },
    "product.created": { "telegram": false, "email": false }
  },
  "etag": "…"
}
```

Bot tokens / chat IDs remain env-backed in v1; PATCH only toggles channel/event matrix.

---

## 11. Security

### `GET|PATCH /api/v1/settings/security`

```json
{
  "access_token_ttl_seconds": 900,
  "refresh_token_ttl_seconds": 86400,
  "login_rate_limit_per_minute": 10,
  "refresh_rate_limit_per_minute": 30,
  "cors_origins": ["https://dupli1.com", "https://manage.dupli1.com"],
  "cookie_secure": true,
  "cookie_name": "dupli1_session",
  "etag": "…"
}
```

Owner-sensitive: require `settings.update` **and** (`*` or `admin.*`) for this section in v1 if desired.

---

## 12. Feature flags

### `GET|PATCH /api/v1/settings/features`

```json
{
  "flags": {
    "guest_pdp_views": false,
    "guest_cart": false,
    "storefront_chat": false,
    "product_cost_visible_to_managers": false,
    "checkout_sessions": true,
    "stripe_checkout": true,
    "dev_simulate_payment": false
  },
  "etag": "…"
}
```

Boolean runtime toggles for manage-web and storefront. Services subscribe to `settings.updated` (subject hint: `settings.features`) or fetch on interval.

---

## 13. Reset & audit

### `POST /api/v1/settings/{section}/reset`

```json
{ "confirm": true }
```

Resets one section to platform defaults. Returns the new section document.

### `GET /api/v1/settings/audit?section=checkout&limit=50`

```json
{
  "events": [
    {
      "id": "…",
      "section": "checkout",
      "actor_user_id": "…",
      "at": "2026-07-14T12:00:00Z",
      "patch": { "session_ttl_minutes": 45 },
      "etag_before": "…",
      "etag_after": "…"
    }
  ]
}
```

---

## Error & concurrency

| Status | When |
|--------|------|
| `401` | Missing/invalid Bearer |
| `403` | Missing `settings.read` / `settings.update` |
| `404` | Unknown section |
| `409` | `If-Match` etag mismatch |
| `422` | Semantically invalid combo (e.g. `guest_checkout_enabled` without `guest_cart_enabled`) |

Clients should send:

```http
PATCH /api/v1/settings/checkout
Authorization: Bearer <access>
If-Match: "<etag>"
Content-Type: application/merge-patch+json
```

---

## Events

| Subject | Payload |
|---------|---------|
| `settings.updated` | `{ "section": "checkout", "etag": "…", "updated_at": "…" }` |

Consumers (order, payment, cart, product, notification) invalidate local cache and re-fetch their section (or the aggregate).

---

## Out of scope for this API

- Product / coupon / order / user **entity** CRUD (existing domain APIs)
- Infrastructure (Postgres, Redis, NATS URLs)
- Terraform / ECS / RDS
- Storefront CMS blocks (can add `settings/content` later or live in `dupli1-web`)
- Writing raw Stripe/Telegram secrets in v1 (vault/env only)

---

## Implementation outline (not scheduled)

1. Permissions `settings.read` / `settings.update` in `shared/pkg/permissions` + `admin` ABAC note in docs.
2. Table `system_settings (section TEXT PK, document JSONB, etag TEXT, updated_at, updated_by)`.
3. Auth routes under `/api/v1/settings`.
4. Wire order/payment hardcoded TTLs to read checkout/orders sections (env = bootstrap default).
5. NATS `settings.updated` + manage-web Settings screens per section.
6. Global `GET /api/v1/orders` without required `customer_id` for `order.read.all` (companion, not settings storage).

---

## Example manage-web mapping

| UI page | Endpoints |
|---------|-----------|
| Settings home | `GET /api/v1/settings` |
| Store | `GET|PATCH …/store` |
| Feature flags | `GET|PATCH …/features` |
| Checkout & orders | `GET|PATCH …/checkout`, `…/orders` |
| Payments | `GET|PATCH …/payments` |
| Notifications | `GET|PATCH …/notifications` |
| Security | `GET|PATCH …/security` |
| Change history | `GET …/audit` |
