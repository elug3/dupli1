# Manager Settings API (sketch)

Design sketch for **mutable global action controls** — store-wide gates and policies that enable, disable, or constrain whole classes of actions.

**Status:** Sketch only — not implemented.

**Related:** [permissions.md](permissions.md), [endpoints.md](endpoints.md), [current-state.md](current-state.md).

---

## What settings are (and are not)

Settings control **global actions**, not individual records.

| Settings do | Settings do not |
|-------------|-----------------|
| Block all new logins | Edit one user's password |
| Block creating new users | Create/update one product |
| Set session / token TTL for everyone | Ship one order |
| Freeze checkout or payments store-wide | Change one coupon code |
| Set unpaid-order cancel window | Refund one payment |

Entity CRUD stays on existing domain APIs (`/products`, `/orders`, `/users`, …). Settings are the **kill switches and policy knobs** those APIs consult before acting.

```text
Manager flips setting  →  persisted globally  →  every service enforces on next request
     e.g. block_login=true     auth.settings      POST /login → 403 "login disabled"
```

---

## Distinction from service introspection

| Surface | Paths | Auth | Purpose |
|---------|-------|------|---------|
| **Service introspection** | `GET /api/v1/<service>/settings` | Public | Read-only runtime snapshot |
| **Manager settings** (this doc) | `GET\|PATCH /api/v1/settings/...` | Bearer + `settings.*` | Mutable global action controls |

---

## Hosting & permissions

**Host (v1):** auth service (`dupli1_db`), NATS `settings.updated` so other services reload.

| Permission | Grants |
|------------|--------|
| `settings.read` | Read all sections |
| `settings.update` | Change global controls |
| `settings.*` | Both |

Owner `*` implies all. `admin.*` does **not** imply `settings.*`.

---

## Conventions

- `GET` / `PATCH /api/v1/settings/{section}` — JSON Merge Patch; `If-Match` etag.
- Boolean gates prefer clear names: `allow_login`, `allow_register`, `allow_checkout`, …
- When a gate blocks an action, domain APIs return **`403`** with a stable machine code, e.g.:

```json
{ "error": "login disabled by settings", "code": "settings.auth.login_disabled" }
```

- Owner / callers with `*` may bypass selected gates only where noted (so you cannot lock yourself out of manage-web).
- Secrets never returned.

---

## Endpoint index

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/settings` | `settings.read` | Compact status of all sections + active blocks |
| `GET` | `/api/v1/settings/{section}` | `settings.read` | Full section |
| `PATCH` | `/api/v1/settings/{section}` | `settings.update` | Change global controls |
| `POST` | `/api/v1/settings/{section}/reset` | `settings.update` | Reset section to defaults |
| `GET` | `/api/v1/settings/audit` | `settings.read` | Change history |

**Sections:** `auth`, `store`, `catalog`, `checkout`, `orders`, `cart`, `payments`, `inventory`, `notifications`.

---

## Aggregate

### `GET /api/v1/settings`

Highlights **active global blocks** for the manage UI banner.

```json
{
  "active_blocks": [
    "auth.allow_login",
    "checkout.allow_checkout"
  ],
  "sections": {
    "auth": { "etag": "a1", "updated_at": "…" },
    "checkout": { "etag": "c2", "updated_at": "…" }
  }
}
```

`active_blocks` lists setting keys currently denying a major action (value `false` on an `allow_*` gate, or `true` on a `block_*` / `maintenance_mode` flag).

---

## 1. Auth — primary example

### `GET|PATCH /api/v1/settings/auth`

Global controls for identity and sessions.

```json
{
  "allow_login": true,
  "allow_register": true,
  "allow_refresh": true,
  "allow_password_change": true,
  "session": {
    "access_token_ttl_seconds": 900,
    "refresh_token_ttl_seconds": 86400
  },
  "rate_limits": {
    "login_per_minute_per_ip": 10,
    "refresh_per_minute_per_ip": 30
  },
  "lockout": {
    "max_failed_attempts": 5,
    "lock_minutes": 15
  },
  "etag": "…",
  "updated_at": "…",
  "updated_by": "…"
}
```

| Control | Global action gated |
|---------|---------------------|
| `allow_login` | `POST /api/v1/auth/login` — all new logins |
| `allow_register` | `POST /api/v1/auth/register` — creating new users |
| `allow_refresh` | `POST /api/v1/auth/refresh` — new access tokens |
| `allow_password_change` | `PATCH …/password` (and future self-service) |
| `session.*` | TTL applied to **all** newly issued tokens |
| `rate_limits.*` | Store-wide IP throttles (today hardcoded) |
| `lockout.*` | Failed-login lock policy for all accounts |

**Enforcement examples**

| Setting | Request | Result |
|---------|---------|--------|
| `allow_login: false` | `POST /login` | `403` `settings.auth.login_disabled` |
| `allow_register: false` | `POST /register` | `403` `settings.auth.register_disabled` |
| `allow_refresh: false` | `POST /refresh` | `403` `settings.auth.refresh_disabled` |
| `access_token_ttl_seconds: 300` | successful login/refresh | access JWT `exp` = now+5m |

**Lockout bypass:** owner (`*`) can still log in when `allow_login` is false, so the store cannot be permanently locked. Optional: also allow `settings.update` holders.

**Example PATCH — emergency freeze**

```http
PATCH /api/v1/settings/auth
Authorization: Bearer <access>
If-Match: "a1"
Content-Type: application/merge-patch+json

{
  "allow_login": false,
  "allow_register": false,
  "allow_refresh": false
}
```

**Example PATCH — shorten sessions**

```json
{ "session": { "access_token_ttl_seconds": 300, "refresh_token_ttl_seconds": 3600 } }
```

---

## 2. Store

### `GET|PATCH /api/v1/settings/store`

```json
{
  "maintenance_mode": false,
  "maintenance_message": "",
  "currency": "krw",
  "store_name": "Dupli1",
  "etag": "…"
}
```

| Control | Global action gated |
|---------|---------------------|
| `maintenance_mode` | Storefront browse/checkout (manage-web stays up) |
| `currency` | All price display / payment currency (default **`krw`**) |

---

## 3. Catalog

### `GET|PATCH /api/v1/settings/catalog`

```json
{
  "allow_product_create": true,
  "allow_product_update": true,
  "allow_product_delete": true,
  "allow_variant_mutations": true,
  "allow_image_upload": true,
  "allow_coupon_mutations": true,
  "allow_coupon_redeem": true,
  "default_product_status": "draft",
  "etag": "…"
}
```

| Control | Global action gated |
|---------|---------------------|
| `allow_product_create` | `POST /api/v1/products` |
| `allow_product_update` / `delete` | `PUT` / `DELETE` products |
| `allow_variant_mutations` | variant create/update/delete |
| `allow_image_upload` | image upload routes |
| `allow_coupon_mutations` | coupon CRUD |
| `allow_coupon_redeem` | `POST /coupons/redeem` + checkout apply |

Reads (public catalog) stay available unless `store.maintenance_mode` is on.

---

## 4. Checkout

### `GET|PATCH /api/v1/settings/checkout`

```json
{
  "allow_checkout": true,
  "allow_guest_checkout": false,
  "session_ttl_minutes": 30,
  "max_line_items": 50,
  "max_quantity_per_line": 10,
  "etag": "…"
}
```

| Control | Global action gated |
|---------|---------------------|
| `allow_checkout` | Create/complete checkout sessions; `POST /orders` |
| `allow_guest_checkout` | Checkout without customer account |
| `session_ttl_minutes` | Lifetime of **all** new checkout sessions |

---

## 5. Orders

### `GET|PATCH /api/v1/settings/orders`

```json
{
  "allow_ship": true,
  "allow_status_update": true,
  "allow_cancel": true,
  "unpaid_cancel_minutes": 5,
  "require_tracking_on_ship": false,
  "etag": "…"
}
```

| Control | Global action gated |
|---------|---------------------|
| `allow_ship` | `POST /orders/{id}/ship` for everyone |
| `allow_status_update` | `PUT …/status` |
| `allow_cancel` | cancel transitions |
| `unpaid_cancel_minutes` | Auto-cancel window for **all** unpaid pending orders |

---

## 6. Cart

### `GET|PATCH /api/v1/settings/cart`

```json
{
  "allow_cart_mutations": true,
  "allow_guest_cart": false,
  "enforce_stock_on_add": false,
  "etag": "…"
}
```

| Control | Global action gated |
|---------|---------------------|
| `allow_cart_mutations` | Add/update/clear cart for all customers |
| `allow_guest_cart` | Guest cart usage (when implemented) |

---

## 7. Payments

### `GET|PATCH /api/v1/settings/payments`

```json
{
  "allow_create_payment": true,
  "allow_refunds": false,
  "provider_mode": "live",
  "etag": "…"
}
```

| Control | Global action gated |
|---------|---------------------|
| `allow_create_payment` | `POST /api/v1/payments` |
| `allow_refunds` | Future refund API |
| `provider_mode` | `live` \| `test` \| `off` (`off` ≡ block creates) |

---

## 8. Inventory

### `GET|PATCH /api/v1/settings/inventory`

```json
{
  "allow_stock_write": true,
  "allow_reservations": true,
  "low_stock_threshold": 3,
  "reservation_ttl_minutes": 30,
  "etag": "…"
}
```

| Control | Global action gated |
|---------|---------------------|
| `allow_stock_write` | Set/adjust stock |
| `allow_reservations` | Create/commit/release reservations (also blocks checkout that needs reserve) |

---

## 9. Notifications

### `GET|PATCH /api/v1/settings/notifications`

```json
{
  "allow_outbound": true,
  "channels": {
    "telegram": true,
    "email": false,
    "sms": false
  },
  "events": {
    "order.paid": true,
    "order.in_transit": true,
    "order.canceled": true,
    "inventory.low_stock": false
  },
  "etag": "…"
}
```

| Control | Global action gated |
|---------|---------------------|
| `allow_outbound` | All notification sends |
| `channels.*` | Channel-wide mute |
| `events.*` | Event-type mute |

---

## Enforcement contract (all services)

1. On startup (and on `settings.updated`), load relevant section(s).
2. Before performing a **mutating** global action, check the gate.
3. If denied → `403` + `code: "settings.<section>.<gate>"`.
4. TTLs / limits apply when **creating** new sessions/tokens/reservations (existing ones keep prior values unless a revoke-all control is added later).

```http
POST /api/v1/auth/login
→ 403 {"error":"login disabled by settings","code":"settings.auth.login_disabled"}
```

---

## Events

| Subject | Payload |
|---------|---------|
| `settings.updated` | `{ "section": "auth", "etag": "…", "updated_at": "…" }` |

---

## Out of scope

- Per-entity CRUD (users, products, orders, …)
- Infra URLs / secrets rotation (env / Secrets Manager)
- Storefront CMS content

---

## Implementation outline

1. Add `settings.read` / `settings.update` to the permissions catalog.
2. Persist sections in `system_settings`.
3. Auth: enforce `allow_login` / `allow_register` / `allow_refresh` + session TTLs from `settings/auth`.
4. Propagate gates to order, cart, payment, product, notification.
5. Manage-web: Settings pages with prominent **global block** toggles (login, register, checkout, payments).

---

## Manage-web mapping

| UI | Endpoint | Typical controls |
|----|----------|------------------|
| Auth & sessions | `…/auth` | Block login / register / refresh; session TTL |
| Store | `…/store` | Maintenance mode |
| Catalog | `…/catalog` | Freeze catalog writes / coupon redeem |
| Checkout | `…/checkout` | Freeze checkout; session TTL |
| Orders | `…/orders` | Freeze ship/cancel; unpaid window |
| Payments | `…/payments` | Freeze payment creates |
| Notifications | `…/notifications` | Mute outbound / channels |
| Audit | `…/audit` | Who flipped which gate |
