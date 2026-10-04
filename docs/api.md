# Dupli1 API Reference

All traffic is routed through the nginx gateway. Locally use **HTTP** at `http://localhost:8080` or `http://localhost` (port 80). Production terminates TLS at the load balancer or gateway.

**Currency:** the storefront uses **KRW only**. Product `price` values and cart/order/payment `*_won` fields are **whole Korean won** (zero-decimal minor units for `krw` — do not multiply by 100). Settings expose `limits.currency: "krw"`.

**Path convention:** every route is namespaced by its owning service — `/api/v1/products/…` (including inventory, catalog and coupons), `/api/v1/orders/…` (including checkout sessions), `/api/v1/cart/…`, `/api/v1/payments/…`, `/api/v1/auth/…`, `/api/v1/profile/…`. The paths documented here are the canonical ones. Older top-level prefixes (`/api/v1/inventory`, `/api/v1/catalog`, `/api/v1/coupons`, `/api/v1/variants`, `/api/v1/checkout`, `/api/v1/carts`) are still registered as aliases and are called out where they differ; new clients should not use them. Migration table: [TODO.md](TODO.md).

---

## Authentication

Protected routes require an `Authorization` header with a Bearer **access** token:

```
Authorization: Bearer <access_token>
```

**Token flow**

1. `POST /api/v1/auth/login` → `{ "refresh_token": "<jwt>" }`
2. `POST /api/v1/auth/refresh` with that refresh token → `{ "token": "<access_jwt>", "refresh_token": "<new_jwt>" }`
3. Use the access token on protected routes until it expires (default 15 min), then refresh again — using the `refresh_token` the previous refresh returned. Refresh tokens rotate on every use: the one just spent stops working, so the caller must store the new one. Reusing an already-rotated refresh token returns `401`. A `401` from `/refresh` always means the token is finished; a `503 refresh unavailable` means auth could not reach its own session ledger, so keep the token and retry rather than treating the session as over.

**Access token claims**

| Claim | Type | Notes |
|-------|------|-------|
| `sub` | string | User ID |
| `type` | string | `"access"` |
| `permissions` | string[] | Fine-grained authorization strings such as `product.create`, `order.ship`, `*` |
| `email` | string | Login email (omitted when empty). Payment reads this for NANO `compOrderMem`; not used for authorization |
| `exp`, `iat` | number | Standard JWT timestamps |
| `jti` | string | Random per-token ID; makes every issued token unique even if minted in the same second |

Refresh tokens contain `sub`, `type: "refresh"`, and `jti` only. Permissions are loaded from the database on every refresh.

Protected routes check the `permissions` claim. See [permissions.md](permissions.md) for the full catalog and endpoint matrix.

Wildcards: `*` (everything), `admin.*` (user-admin domain), `{resource}.*` (e.g. `product.*`).

### Account types

Every user has an `account_type` field (JSON key `account_type`) separate from **permissions**:

| Value | Meaning | Typical permissions |
|-------|---------|---------------------|
| `customer` | End-user storefront account | `[]` (empty — ABAC self-service only) |
| `manager` | Human operator | job-function permissions (`product.*`, `user.*`, …) or `admin.*` / `*` (owner) |
| `service` | Machine / integration account | `user.create`, `order.ship`, … per job function |

`admin` is **not** an account type — it is a permission/management tier (`admin.*`, auth ABAC `ClassAdmin`). Write APIs reject `account_type: "admin"`; use `manager` for operators. Startup migrate rewrites any leftover DB `account_type=admin` → `manager`.

Seeded accounts: owner (`OWNER_EMAIL`) → `permissions: ["*"]`, `account_type: manager`; `dupli1-web` → `["user.create"]`; `dupli1-order` → `["order.ship", "order.status.update", "inventory.reservation.manage", "payment.cancel"]`. `POST /register` defaults to `customer` when `account_type` is omitted.

---

## Gateway

### `GET /gateway/health`

Nginx liveness check — responds without touching any backend service.

**Response `200`** (plain text)
```
ok
```

---

## Auth Service — `/api/v1/auth`

### `GET /health` or `GET /api/v1/auth/health`

Auth service liveness check.

**Response `200`**
```json
{ "status": "ok" }
```

### `GET /settings` or `GET /api/v1/auth/settings`

Non-secret operational settings (auth mode, feature flags, dependency configured flags). Never includes secrets or DSNs.

### `GET /api/v1/auth/.well-known/jwks.json`

RS256 public key set for verifying access tokens issued by auth.

---

### `POST /api/v1/auth/register`

Create a new user account. Requires `user.create`.

**Headers** — `Authorization: Bearer <access_token>`

**Request body**
```json
{
  "email": "user@example.com",
  "password": "minlen8",
  "account_type": "customer"
}
```

| Field | Type | Constraints |
|-------|------|-------------|
| `email` | string | required, valid email |
| `password` | string | required, min 8 chars — except for `account_type: service`, which must omit it (`422` if sent): service accounts have no password and authenticate with an API key |
| `account_type` | string | optional; one of `customer`, `manager`, `service`; defaults to `customer`. Do not send `admin` (permission tier — use `manager`). Callers with only `user.create` (no `admin.*` or `*`) may register `customer` only |

**Response `201`**
```json
{ "user_id": "03f95d58-4840-46d4-9c92-fe48364d2e75" }
```

**Errors**
| Status | Meaning |
|--------|---------|
| `400` | Validation failed (bad email, password too short) |
| `401` | Missing or invalid access token |
| `403` | Caller lacks `user.create`, or attempted a disallowed `account_type` / management target |
| `409` | Email already registered |
| `422` | Invalid email, weak password, or invalid `account_type` |

---

### Service account: dupli1-web

The `dupli1-web` BFF uses a seeded machine account with `permissions: ["user.create"]` and `account_type: "service"`. It can call `POST /api/v1/auth/register` to create customer accounts, but cannot manage passwords, permissions, or user status.

Configure on `dupli1-auth` startup:

| Variable | Purpose |
|----------|---------|
| `DUPLI1_WEB_SERVICE_EMAIL` | Service account email (skip seeding when empty) |
| `DUPLI1_WEB_SERVICE_API_KEY` | Its API key, required when the email is set — service accounts have no password ([auth-service-api-keys.md](auth-service-api-keys.md)) |

`dupli1-web` should log in with these credentials server-side, cache/refresh the access token, and call register from the backend only — never expose the service password to browsers.

---

### `POST /api/v1/auth/login`

Authenticate and receive a refresh token.

**Request body**
```json
{
  "email": "user@example.com",
  "password": "minlen8",
  "client": "storefront"
}
```

`client` names the front end the login is for, and auth refuses an account type that does not belong there:

| `client` | Account types allowed |
|----------|-----------------------|
| `storefront` (dupli1-web) | `customer`, `manager` |
| `manage` (manage-web) | `manager` |
| `service` (was the machine login) | nobody |

Service accounts are refused through every client, including none: they have no password and authenticate with an API key ([`POST /api/v1/auth/token`](#post-apiv1authtoken)).

The check runs only after the password is verified, so a wrong password is still `401` whatever the account type. `client` is optional for one release while callers roll over; omitted, no account-type rule applies. An unknown value is `400`.

**Response `200`**
```json
{
  "refresh_token": "<jwt>"
}
```

**Errors**
| Status | Meaning |
|--------|---------|
| `400` | Missing or malformed body |
| `400` | Unknown `client` |
| `401` | Invalid credentials |
| `403` | Account locked (customers/managers after 5 failed attempts) or deactivated |
| `403` | `{"error": "<message to show>", "code": "account_type_not_allowed"}` — account type not allowed for `client`; web apps show `error` as is |

**Lockout:** after **5** consecutive failed logins, `customer` and manager-tier accounts set `locked_at` and further logins return `403` for **15 minutes**, after which the lock auto-expires and failed-attempt counting starts fresh. **Admin** and **owner** accounts are never locked (failed attempts do not set `locked_at`; a stale lock is cleared on the next login attempt). See [permissions.md](permissions.md).

---

### `GET /api/v1/auth/me`

Return the currently authenticated user's **account** (credentials tier — not commerce profile).

**Headers** — `Authorization: Bearer <access_token>`

**Response `200`**
```json
{
  "user_id": "03f95d58-4840-46d4-9c92-fe48364d2e75",
  "email": "user@example.com",
  "account_type": "customer",
  "permissions": [],
  "is_active": true,
  "locked_at": null,
  "failed_login_attempts": 0,
  "has_password": true
}
```

**Errors**
| Status | Meaning |
|--------|---------|
| `401` | Missing, malformed, or expired access token |
| `403` | Account deactivated or locked since the access token was issued |
| `404` | User no longer exists |

---

See the **Profile Service** section below for the commerce profile and saved-addresses API — split out of `auth` into its own `profile` service.

---

### `POST /api/v1/auth/refresh`

Exchange a refresh token for a new access token. The refresh token **rotates**: the one sent in the request is invalidated immediately, and the response carries its replacement. Callers must store the returned `refresh_token` and use it next time — resending the token just spent (or any earlier one) fails with `401`.

**Request body**
```json
{ "refresh_token": "<jwt>" }
```

**Response `200`**
```json
{
  "token": "<access_jwt>",
  "refresh_token": "<new_jwt>"
}
```

**Errors**
| Status | Meaning |
|--------|---------|
| `400` | Missing or malformed body |
| `401` | Refresh token invalid, expired, already rotated/revoked, or the account is deactivated/locked |

---

### `POST /api/v1/auth/logout`

Revoke a refresh token. The access token remains valid until it expires.

**Request body**
```json
{ "refresh_token": "<jwt>" }
```

**Response `204`** — no body

**Errors**
| Status | Meaning |
|--------|---------|
| `400` | Missing or malformed body |
| `500` | Internal error |

---

### `POST /api/v1/auth/token`

Exchange a service-account API key for an access token ([auth-service-api-keys.md](auth-service-api-keys.md)). **Internal only:** the gateway answers `404` on its public listener and serves it on `:8081`; service callers can also reach auth directly.

**Request** — no body:
```
Authorization: ApiKey dk_live_<43 chars>
```

**Response `200`** — no refresh token; exchange again when this one nears expiry:
```json
{ "token": "<access_jwt>", "token_type": "Bearer", "expires_in": 900 }
```

The token carries `sub` (the service account), `account_type: "service"`, `service_name`, `token_use: "api_key"`, `akid` (the key id) and `permissions` — the key's scope intersected with the account's current permissions.

**Errors**
| Status | Meaning |
|--------|---------|
| `401` | `{"error":"invalid_api_key"}` for every refusal — unknown, revoked or expired key, inactive or non-service account. The reason is only logged |
| `429` | Rate limited (60/min per IP) |
| `503` | Key store unreachable — keep the key and retry |

---

## Auth Admin — `/api/v1/auth/users`

Requires `Authorization: Bearer <access_token>`.

### `GET /api/v1/auth/users`

List all users. Requires `user.read`. Results are filtered by auth ABAC hierarchy (callers only see accounts they may manage).

**Response `200`**
```json
{
  "users": [
    {
      "user_id": "03f95d58-4840-46d4-9c92-fe48364d2e75",
      "email": "owner@dupli1.com",
      "account_type": "manager",
      "permissions": ["*"],
      "is_active": true,
      "locked_at": null,
      "failed_login_attempts": 0,
      "created_at": "2026-10-02T03:00:00Z",
      "has_password": true
    }
  ]
}
```

`has_password` is `false` for service accounts, which authenticate with API keys only. `created_at` is when the account registered; it is absent for accounts created before auth recorded it.

**Errors**
| Status | Meaning |
|--------|---------|
| `401` | Missing or invalid access token |
| `403` | Caller lacks `user.read` or management hierarchy forbids listing |

---

### `GET /api/v1/auth/reports/registrations`

Customer sign-ups per week or month. Requires `user.read`. Query: `granularity=week|month` (default `week`), `from` and `to` as inclusive `YYYY-MM-DD` dates in KST, widened to whole periods (default: the last 12 weeks or 12 months up to the current one; at most 104 weeks or 36 months). Weeks run Monday to Sunday; the periods match `GET /api/v1/orders/reports/sales`. Managers and service accounts are not counted.

**Response `200`**
```json
{
  "granularity": "week",
  "timezone": "Asia/Seoul",
  "from": "2026-07-13",
  "to": "2026-10-04",
  "periods": [{ "period_start": "2026-09-28", "period_end": "2026-10-04", "new_customers": 5 }],
  "total_new_customers": 41,
  "undated_customers": 120
}
```

`undated_customers` signed up before auth recorded sign-up times and fall in no period.

**Errors**
| Status | Meaning |
|--------|---------|
| `400` | Bad `granularity`, date, or range |
| `401` | Missing or invalid access token |
| `403` | Caller lacks `user.read` |
| `503` | No user database configured |

---

### `PATCH /api/v1/auth/users/{id}/permissions`

Replace the permission list for a user. Requires `user.permissions.update`. Subject to auth ABAC hierarchy (who may manage whom).

**Request body**
```json
{
  "permissions": ["user.password.update", "user.status.update"],
  "account_type": "manager"
}
```

| Field | Type | Constraints |
|-------|------|-------------|
| `permissions` | string[] | required |
| `account_type` | string | optional; one of `customer`, `manager`, `service`. Do not send `admin` (permission tier — use `manager`) |

**Response `200`** — updated user object (includes `account_type`, `permissions`)

**Errors**
| Status | Meaning |
|--------|---------|
| `400` | Missing or malformed body |
| `401` | Missing or invalid access token |
| `403` | Caller lacks `user.permissions.update` or may not manage this user |
| `404` | User not found |
| `422` | Invalid `account_type` or permission string |

---

### `PATCH /api/v1/auth/users/{id}/password`

Set a new password for a user. Requires `user.password.update`.

**Request body**
```json
{ "password": "newpassword" }
```

**Response `204`** — no body

**Errors**
| Status | Meaning |
|--------|---------|
| `400` | Missing or malformed body |
| `401` | Missing or invalid access token |
| `403` | Caller lacks `user.password.update` or may not manage this user |
| `404` | User not found |
| `422` | Password too short (min 8 chars), or the user is a service account (they have no password) |

---

### `PATCH /api/v1/auth/users/{id}/status`

Activate or deactivate a user. Requires `user.status.update`.

**Request body**
```json
{ "is_active": false }
```

**Response `200`** — updated user object

**Errors**
| Status | Meaning |
|--------|---------|
| `400` | Missing or malformed body |
| `401` | Missing or invalid access token |
| `403` | Caller lacks `user.status.update` or may not manage this user |
| `404` | User not found |

---

### Service-account API keys

Keys attach to `account_type: service` only, and the account hierarchy applies — since only the owner manages service accounts, only the owner reaches these routes in practice. Keys seeded from `DUPLI1_*_SERVICE_API_KEY` show `source: "env"`.

| Method | Path | Permission | Response |
|--------|------|------------|----------|
| `GET` | `/api/v1/auth/users/{id}/api-keys` | `user.apikey.read` | `200 {"api_keys": [...]}` — metadata only, never the key |
| `POST` | `/api/v1/auth/users/{id}/api-keys` | `user.apikey.manage` | `201` — the key object **plus `api_key`, the plaintext, shown this once** |
| `DELETE` | `/api/v1/auth/api-keys/{keyId}` | `user.apikey.manage` | `204`; revoking twice is a no-op |

**Create body:** `{ "name": "claude-ops", "permissions": ["order.read.all"], "expires_in_days": 90 }` — `permissions` omitted or `[]` inherits the account's; `expires_in_days` omitted never expires.

**Key object:** `id`, `user_id`, `name`, `prefix` (`dk_live_A1b2`, for display), `permissions`, `source` (`api` \| `env`), `created_at`, `created_by`, `expires_at`, `last_used_at` (updated at most once a minute), `revoked_at`.

**Errors:** `400` name missing, unknown permission, a scope naming a permission the account lacks, or the account is not a service account (`invalid_account_type`); `403` caller lacks the permission or may not manage the account; `404` account or key not found; `409 env_managed_key` revoking an env-seeded key — change or unset its env var and restart auth instead.

Revocation stops new exchanges at once; tokens already minted live out their 15 minutes, since downstream services validate offline.

---

## Profile Service — `/api/v1/profile`

PostgreSQL-backed customer commerce profile (display name, phone) and saved shipping addresses — separated from `auth`'s identity/credentials data so it can evolve and scale independently. Requires `Authorization: Bearer <access_token>`; the owner is always the JWT `sub` claim — self-service only, no dedicated permission (same ABAC pattern as cart). Subscribes to `auth`'s `user.deleted` NATS event to cascade-delete owned PII.

For one release, nginx also aliases the legacy `/api/v1/auth/me/profile` and `/api/v1/auth/me/addresses` paths to `dupli1-profile`, so clients still calling the pre-extraction paths keep working. See [profile-service.md](profile-service.md) for architecture, data model, and the extraction/cutover plan; [auth-profile-extension-plan.md](auth-profile-extension-plan.md) for phase history.

### `GET /api/v1/profile/health`

**Response `200`**
```json
{ "status": "ok" }
```

### `GET /api/v1/profile/me/profile`

**Response `200`**
```json
{
  "user_id": "03f95d58-4840-46d4-9c92-fe48364d2e75",
  "display_name": "윤라희",
  "phone": "01041125167",
  "default_address_id": "addr_000001",
  "addresses": [
    {
      "id": "addr_000001",
      "label": "home",
      "recipient_name": "윤라희",
      "recipient_phone": "01041125167",
      "postal_code": "06194",
      "address_line1": "테헤란로 78길 14-12",
      "address_line2": "9층",
      "city": "강남구",
      "province": "서울특별시",
      "pccc": "P123456789012",
      "is_default": true
    }
  ]
}
```
`pccc` (Korea Personal Customs Clearance Code, `P` + 12 digits) is omitted per-address when unset — it only applies to overseas-sourced shipments.

### `PATCH /api/v1/profile/me/profile`

Merge-patch: only sent fields change.

**Request**
```json
{ "display_name": "윤라희", "phone": "010-4112-5167" }
```

**Response `200`** — updated `ProfileView` (same shape as `GET`). Phone is normalized to digits-only.

### Saved addresses

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/profile/me/addresses` | `{ "addresses": [ … ] }` |
| `POST` | `/api/v1/profile/me/addresses` | Create (max **10** per user; first address created is default) |
| `GET` | `/api/v1/profile/me/addresses/{id}` | One address |
| `PATCH` | `/api/v1/profile/me/addresses/{id}` | Partial update (merge patch) |
| `DELETE` | `/api/v1/profile/me/addresses/{id}` | Remove |
| `POST` | `/api/v1/profile/me/addresses/{id}/default` | Set sole default |

**Create/update request** — `recipient_name`, `recipient_phone`, `postal_code` (5 digits), `address_line1`, `city`, `province` required on create; optional `label`, `address_line2`, `pccc`, `is_default`:
```json
{
  "recipient_name": "윤라희",
  "recipient_phone": "01041125167",
  "postal_code": "06194",
  "address_line1": "테헤란로 78길 14-12",
  "address_line2": "9층",
  "city": "강남구",
  "province": "서울특별시",
  "pccc": "P123456789012",
  "is_default": true
}
```

`is_default: true` clears the default flag on every other address first (at most one default per user, enforced by a unique partial index). `is_default: false` on `PATCH` un-defaults that address without promoting another — same as what happens when the default address is deleted. Omitting `is_default` on `PATCH` leaves it unchanged.

**Errors**
| Status | Meaning |
|--------|---------|
| `400` | Invalid field (phone, postal code, PCCC format, name/line length) or address limit (10) reached |
| `401` | Missing or invalid access token |
| `404` | Address not found, or not owned by the caller — same code whether it doesn't exist or belongs to someone else, to avoid id enumeration |

---

## Product Service — `/api/v1/products`

### `GET /api/v1/products/health`

Product service liveness check.

**Response `200`**
```json
{ "status": "ok" }
```

---

### `GET /api/v1/products`

Search **parent styles** (one row per style; colors are not duplicated). No authentication required for the public catalog view (active parents only). With a valid Bearer token that includes `product.read` (or `product.*` / `*`), returns all statuses.

| Filter / param | Match type |
|----------------|-----------|
| `q` | case-insensitive substring on name, brand, or description |
| `category` | exact (e.g. `bags`) |
| `subcategory` | exact bag type (`handbags`, `tote`, `shoulder`, `cross`, `mini`; alias `subCategory`) |
| `style` | exact bag occasion (`casual`, `evening`, `business`, `weekend`, `statement`) — not SKU `styleCode` |
| `target` | exact audience (`all`, `men`, `women`, `kids`) |
| `brand` | case-insensitive substring |
| `color` | parent has an active variant with this color |
| `size` | parent has an active variant with this size |
| `material` | exact |
| `tags` | parent must include all listed tags (comma-separated or repeated) |
| `status` | exact (`product.read` or wildcard required) |
| `sort` | `newest` (default), `views` (`popular`), `sold`, `wishlist`, `price`, `name` |
| `order` | `asc` \| `desc` (default `desc`; `name` defaults to `asc`) |
| `period` | `day` \| `week` \| `month` — created within that window (`past_week` / `7d` aliases) |
| `limit` | page size (default `50`, max `100`) |
| `offset` | rows to skip (default `0`) |

Example: `GET /api/v1/products?category=bags&subcategory=tote&style=casual&target=women&sort=views&order=desc&limit=20`

See [product-rich-search.md](product-rich-search.md) and [product-master-catalog.md](product-master-catalog.md).

**Response `200`**
```json
{
  "total": 1,
  "limit": 50,
  "offset": 0,
  "sort": "newest",
  "order": "desc",
  "period": "week",
  "results": [
    {
      "id": "BOT-001",
      "name": "Cassette Bag",
      "description": "...",
      "price": 2500.00,
      "officialPrice": 3200.00,
      "brand": "Bottega Veneta",
      "color": "Green",
      "material": "Leather",
      "stock": 5,
      "category": "bags",
      "subCategory": "handbags",
      "style": "casual",
      "target": "women",
      "capacity": "Medium",
      "tags": ["hot"],
      "viewCount": 12,
      "soldCount": 3,
      "wishlistCount": 1,
      "imageUrls": ["https://cdn.example/bot-001.jpg"],
      "defaultImageUrl": "https://cdn.example/bot-001.jpg",
      "defaultListingImageUrl": "https://cdn.example/bot-001.jpg.w600.jpg"
    }
  ]
}
```

`total` is the full match count before pagination; `results` is the current page.

List/search/home clients should prefer `defaultListingImageUrl` (≈600px JPEG sibling of the original) when present, and fall back to `defaultImageUrl` / `imageUrls`. PDP keeps full-size `imageUrls` / variant `imageUrls`. Variants may also expose parallel `listingImageUrls`. See [product-images-browser-access.md](./product-images-browser-access.md).

### Wishlist

| Method | Path | Notes |
|--------|------|-------|
| `PUT` / `POST` | `/api/v1/products/{id}/wishlist` | Add; JWT `sub` or guest cookie |
| `DELETE` | `/api/v1/products/{id}/wishlist` | Remove |
| `GET` | `/api/v1/products/wishlist` | List current owner's items |

---

### `POST /api/v1/products/promotions/redeem`

> **Renamed.** The product term is **promotional code**. These are the canonical paths; `coupon_code` is now `promotion_code`. The pre-rename spellings `/api/v1/products/coupons…` and the older top-level `/api/v1/coupons…` stay registered as aliases for one release, and the `coupon.*` permission set is still accepted alongside `promotion.*`. See [product-promotion-rename.md](product-promotion-rename.md).

Look up a promotional code. No authentication required.

**Request body**
```json
{ "code": "SUMMER30" }
```

**Response `200`** — the definition object (`code`, `discount`, `description`, `expires`, `active`)

**Errors**
| Status | Meaning |
|--------|---------|
| `404` | Invalid code, or the code exists but is inactive |

This is a **lookup**: it answers whether a code is live, honouring `active`, `expires_at` and the campaign cap, and is rate-limited per IP and per customer. It is **not** cart-aware and returns no discount amount — use `evaluate` for that. Usage limits are enforced by the ledger at `reserve`, not here.

#### Target surface (planned)

Being replaced by a cart-aware evaluation call as part of [product-promo-referral-code-plan.md](product-promo-referral-code-plan.md). Planned, **not implemented**:

| Method | Path | Permission | Purpose | Status |
|--------|------|------------|---------|--------|
| `POST` | `/api/v1/products/promotions/evaluate` | — (public, rate-limited) | Price a code against a checkout context → `{ ok, discount_won, eligible_sku_ids, eligible_subtotal_won, reason, sub_reason }`. Order calls it at apply and again at complete; the storefront uses it to preview. Lines need only `{sku_id, sku, quantity, unit_price_won}` — a line's category, brand, parent and sale state are read from the catalog here, and a caller that sends them has them overwritten | **live** |
| `POST` | `/api/v1/products/promotions/reserve` | `promotion.redeem` | Re-evaluate and record a pending use against an order. Idempotent per order | **live** |
| `POST` | `/api/v1/products/promotions/consume` | `promotion.redeem` | Mark an order's reservation paid. Idempotent | **live** |
| `POST` | `/api/v1/products/promotions/release` | `promotion.redeem` | Hand a use back, for a cancel before shipment | **live** |
| `POST` | `/api/v1/products/promotions/tier` | `promotion.redeem` | The customer's automatic tier discount on a cart (`apply_mode: auto`), the best one if they hold several → `{ ok, code, discount_won, … }`. Internal (the customer id is in the body); order calls it on every session read and at complete | **live** |
| `POST` | `/api/v1/products/promotions/me/tier` | Bearer (ABAC) | The same answer for the signed-in customer (id from the token, never the body), so the storefront can show a member's tier before checkout | **live** |
| `GET`/`POST` | `/api/v1/products/promotions/me` | Bearer (ABAC) | Current customer's wallet. POST a cart to have each entitlement judged against it; the customer id comes from the token, never the body | **live** |
| `POST` | `/api/v1/products/promotions/by-code/{code}/issue` | `promotion.issue` | Issue a single-user entitlement, idempotent on `trigger_key` | **live** |
| `DELETE` | `/api/v1/products/promotions/entitlements/{id}` | `promotion.issue` | Revoke an entitlement; never rewrites an order that used it | **live** |
| `GET` | `/api/v1/products/promotions/{code}/stats` | `promotion.read` | Campaign stats from the paid ledger | Phase 4 |

A rejection is a `200` with `ok: false` — the request succeeded, the cart just
did not earn the discount. `reason` is one of `invalid_code`, `expired`,
`already_used`, `not_eligible`, `campaign_exhausted`, `login_required`;
`not_eligible` adds a `sub_reason` (`min_spend`, `category`, `brand`,
`on_sale_excluded`, `no_line_match`, `condition`). An unknown code and an
inactive one both return `invalid_code`, so probing cannot enumerate live
campaigns.

**Single-user codes.** A `single_user` definition is unusable without an
entitlement in `customer_promotions`. Entitlements are issued automatically to
new customer accounts when auth publishes `user.registered` — for every
definition whose `auto_issue` is `user_registered` — by a manager, or in bulk
by `product/cmd/backfill-welcome-promotion -code <CODE>`. No code is built in
or seeded: a sign-up campaign is a definition a manager creates with
`scope: "single_user"` and `auto_issue: "user_registered"`, and ends by
clearing `auto_issue` (to `""`). Inactive definitions are still issued, so a
campaign can collect sign-ups before it goes live; expired ones are not. Each carries its own
`expires_at`, computed from the definition's `entitlement_ttl_days` at issue
time, so an account issued late in a campaign gets the same window as one
issued at launch. An account that holds no entitlement is refused with
`invalid_code` — the same answer as an unknown code, so guessing a campaign's
code reveals nothing. Whether the code has been *spent* is the redemption
ledger's answer, not the entitlement's.

**Definition fields.** `scope` (`global` | `single_user`), `benefit`
(`{target, discount_type, discount_fraction | discount_fixed_won,
max_discount_won, apply_to}`), `conditions` (versioned predicate document over
an allowlist of attributes), `expires_at`, `max_redemptions`,
`max_per_customer`, `entitlement_ttl_days`, `auto_issue` (`""` | `user_registered`;
`single_user` only), `apply_mode` (`code` default | `auto`), `terms`, `redemption_count`.

**Every promotion discounts the whole order.** The discount base is always the
goods subtotal (selling price, before shipping). `conditions` on brand,
category or SKU decide only whether a code applies; once one line matches, the
discount covers every line. `apply_to` accepts only `entire_subtotal` (or
empty); `eligible_lines` answers `400` since 2026-09-30, and stored definitions
still carrying it are rewritten to `entire_subtotal` on startup. Shipping is
never discounted.

**Customer tiers (`apply_mode: auto`).** A VIP or private tier is a
`single_user` definition with `apply_mode: auto`: issuing its code to an
account makes that account a member, revoking the entitlement takes them out.
Members get the discount on every order without entering anything, and it
stacks under whatever code the order carries (code and tier together are
capped at the goods subtotal). A member of several tiers gets the best one.
A tier has no ledger row and no cap, so `max_redemptions` is refused; typing
its code answers `invalid_code`, and it is left out of the wallet. On create/update, send
`expires_on` as a date (`2026-08-31`) to mean the end of that day in Seoul.
The legacy `discount` fraction and free-text `expires` are still accepted and
read, but are not enforced — a definition needs a real `expires_at` to expire.

Failures carry machine-readable reason codes (`invalid_code`, `expired`, `already_used`, `not_eligible` + sub-reason, `campaign_exhausted`, `login_required`) so customer copy stays in the frontends.

---

### `GET /api/v1/products/{id}`

Public PDP. No authentication required. Returns an active **parent** with `variants[]`, `availableColors`, and `availableSizes`. Cart lines use each variant's `sku` / `skuId` (inventory key). Parent `price` is the charged amount; `officialPrice` is display-only. Each variant may include `dimensions` (`widthMm` / `heightMm` / `depthMm` in millimeters) — distinct from letter `size`/`sizeCode`; see [product-sku-dimensions.md](product-sku-dimensions.md).

Each embedded variant includes stock enrichment: `availableQty` (`max(0, quantity − reserved)`) and `inStock` (`availableQty > 0`). Every sellable SKU has a `stock_items` row (created with qty 0 on variant create; see [product-stock-tracking-plan.md](product-stock-tracking-plan.md)). Legacy parent `stock` is omitted from responses.

On success, the handler ensures a `dupli1_guest` cookie and records a unique view (one count per guest × product). Response includes public `viewCount` and `soldCount` (units committed on ship — [product-sold-count.md](product-sold-count.md)). View-store failures are logged and do not fail the PDP — see [product-guest-views-plan.md](product-guest-views-plan.md).

**Response `200`** — parent product object with variants (includes `viewCount`, `soldCount`)

**Errors**
| Status | Meaning |
|--------|---------|
| `404` | Product not found or not active |

---

### `POST /api/v1/products/visits`

Storefront visit beacon. No authentication, no body. The storefront sends it once per page load from the browser. It ensures the same `dupli1_guest` cookie as the PDP (minting it when absent) and records the browser as a visitor for the current KST day; repeat calls the same day are no-ops. Requests with a crawler/headless `User-Agent`, no `User-Agent`, or a `Sec-Purpose`/`Purpose: prefetch` header are not counted. Disabled with the PDP view count (`PRODUCT_VIEWS_ENABLED=false`).

**Response `204`** — including when the visit was not counted or the store failed (logged). `429` past 120 requests per minute from one IP (a request without the cookie mints a new visitor, so the beacon is capped like promotion redeem; Redis-backed when `REDIS_URL` is set).

---

### `GET /api/v1/products/reports/visitors`

Unique storefront visitors per week or month. Requires `product.read`. Same query and periods as `GET /api/v1/orders/reports/sales`: `granularity=week|month` (default `week`), `from`/`to` inclusive `YYYY-MM-DD` KST dates widened to whole periods (default the last 12 weeks or 12 months; at most 104 weeks or 36 months, else `400`). A visitor is a browser (`dupli1_guest` cookie), so one person on two devices counts twice. Counting started when this endpoint shipped; there is no earlier history.

**Response `200`**
```json
{
  "granularity": "week",
  "timezone": "Asia/Seoul",
  "from": "2026-09-28",
  "to": "2026-10-11",
  "periods": [
    { "period_start": "2026-09-28", "period_end": "2026-10-04", "unique_visitors": 2, "visitor_days": 3 },
    { "period_start": "2026-10-05", "period_end": "2026-10-11", "unique_visitors": 1, "visitor_days": 1 }
  ],
  "total_unique_visitors": 3,
  "today": { "date": "2026-10-06", "unique_visitors": 1 }
}
```

`unique_visitors` counts each browser once per period; `visitor_days` counts it once per day it came (the sum of the period's daily uniques). `total_unique_visitors` counts each browser once across the whole range, so it is not the sum of the periods. `today` is always the current KST day.

---

### `GET /api/v1/products/{id}/recommendations`

Public related-product list for PDP. No authentication required. Returns ordered active **parent** cards (seed excluded). Algorithm: same-category content similarity + soft `view_count` boost — see [product-recommendations.md](product-recommendations.md).

**Query**
| Param | Default | Notes |
|-------|---------|-------|
| `limit` | `8` | Clamped 1–24 |

**Response `200`**

```json
{
  "seedId": "BOT-001",
  "items": [ /* parent list cards */ ]
}
```

**Errors**
| Status | Meaning |
|--------|---------|
| `404` | Seed product not found or not active |
| `400` | Invalid `limit` |

---

### Product CRUD (authenticated)

Routes below require `Authorization: Bearer <access_token>`. Product validates RS256 tokens via JWKS (`AUTH_JWKS_URL`). Each route requires a specific permission (wildcards such as `product.*` also grant access).

| Method | Path | Permission |
|--------|------|------------|
| POST | `/api/v1/products` | `product.create` |
| PUT | `/api/v1/products/{id}` | `product.update` |
| DELETE | `/api/v1/products/{id}` | `product.delete` |
| POST | `/api/v1/products/{id}/images` | `product.image.upload` |
| POST | `/api/v1/products/{id}/variants` | `product.variant.create` |
| PUT | `/api/v1/products/{id}/variants/{sku}` | `product.variant.update` |
| DELETE | `/api/v1/products/{id}/variants/{sku}` | `product.variant.delete` |
| POST | `/api/v1/products/{id}/variants/{sku}/images` | `product.image.upload` |
| GET | `/api/v1/products/promotions` | `promotion.read` |
| POST | `/api/v1/products/promotions` | `promotion.create` |
| PUT | `/api/v1/products/promotions/by-code/{code}` | `promotion.update` |
| DELETE | `/api/v1/products/promotions/by-code/{code}` | `promotion.delete` |

Each route also accepts the pre-rename `coupon.*` permission and answers on `/api/v1/products/coupons…` and `/api/v1/coupons…`, for one release ([product-promotion-rename.md](product-promotion-rename.md)).

`PUT /api/v1/products/{id}` and variant updates **merge**: omitted JSON fields keep their current value, so a partial body cannot blank out data. The trade-off is that a zero value is indistinguishable from an omitted one — sending `price: 0` or `officialPrice: 0` is ignored rather than clearing the price. See [product-price-on-parent.md](product-price-on-parent.md).

**Deletes are permanent and refuse while stock is still in play** (`409`):

- `DELETE /api/v1/products/{id}` removes the parent, every variant and their stock rows. It is refused while any of those SKUs has stock **reserved** for an open order, since that order could then neither ship nor release its hold. Unreserved stock on hand is deleted with the product.
- `DELETE /api/v1/products/{id}/variants/{sku}` needs the SKU's stock row empty: set its quantity to `0` first, and wait for reserved stock to ship or be released. The empty row goes with the variant.
- Product delete publishes `product.deleted` (a Telegram ops alert for chats with `alert_product`) and cascades the product's wishlist entries and view counts; variant delete publishes `product.variant_deleted`. Past orders keep their own snapshot (`product_name`, image), uploaded images stay in object storage, and cart lines naming a removed SKU come back as unavailable.

New parent `id`s are ULIDs (`domain.NewProductID()`); legacy brand-prefixed ids (e.g. `BOT-001`) remain valid. Human identity is `brandCode` + `styleCode`. Dual variant identity and master dictionaries: [product-sku-system.md](product-sku-system.md) — ULID `skuId` (canonical) + human `sku` (`Brand_Style_Color[_Edition]_Size`). Catalog CRUD at `/api/v1/products/catalog/…` (legacy alias `/api/v1/catalog/…`). Product/variant create requires existing master codes (Phase C). See also [product-variants-plan.md](product-variants-plan.md).

---

## Inventory — `/api/v1/products/inventory` (served by the product service)

Merged into the product service. Each item route has a `by-sku-id/{skuId}` sibling
keyed by the variant's canonical ULID `skuId`. **Reads are public.** Writes require
Bearer JWT when `AUTH_JWKS_URL` is configured.

| Method | Path | Permission |
|--------|------|------------|
| GET | `/api/v1/products/inventory/items/{sku}` | — (public) |
| PUT | `/api/v1/products/inventory/items/{sku}` | `inventory.stock.write` |
| POST | `/api/v1/products/inventory/items/{sku}/adjust` | `inventory.stock.write` |
| GET | `/api/v1/products/inventory/items/by-sku-id/{skuId}` | — (public) |
| PUT | `/api/v1/products/inventory/items/by-sku-id/{skuId}` | `inventory.stock.write` |
| POST | `/api/v1/products/inventory/items/by-sku-id/{skuId}/adjust` | `inventory.stock.write` |
| POST | `/api/v1/products/inventory/reservations` | `inventory.reservation.manage` |
| POST | `/api/v1/products/inventory/reservations/{id}/commit` | `inventory.reservation.manage` |
| POST | `/api/v1/products/inventory/reservations/{id}/release` | `inventory.reservation.manage` |

The legacy prefix `/api/v1/inventory/…` remains registered as an alias (item routes there are `/api/v1/inventory/{sku}`, without the `items` segment) and will be removed once clients migrate.

### `GET /api/v1/products/inventory/health`

**Response `200`**
```json
{ "status": "ok" }
```

### `GET /api/v1/products/inventory/items/{sku}`

Get stock for a SKU.

### `PUT /api/v1/products/inventory/items/{sku}`

Set stock quantity.

**Request body**
```json
{ "quantity": 100 }
```

### `POST /api/v1/products/inventory/items/{sku}/adjust`

Adjust stock by delta.

**Request body**
```json
{ "delta": -5 }
```

### `POST /api/v1/products/inventory/reservations`

Reserve stock for an order.

**Request body**
```json
{
  "order_id": "ord-123",
  "items": [{ "sku": "BOT-001", "quantity": 1 }]
}
```

**Response `201`**
```json
{
  "reservation_id": "...",
  "reservation": { }
}
```

### `POST /api/v1/products/inventory/reservations/{id}/commit`

Commit a reservation (deduct stock).

### `POST /api/v1/products/inventory/reservations/{id}/release`

Release a reservation (return stock).

---

## Cart Service — `/api/v1/cart`

PostgreSQL-backed persistent cart. Enriches lines from product (price, images) and inventory (availability). Does **not** reserve stock or create orders. `UpsertItem` / `ReplaceItems` reject when requested quantity exceeds available (including missing stock ⇒ available 0) with `400` and `reason: insufficient_stock`.

When `AUTH_JWKS_URL` or `JWT_SECRET` is set, cart routes require `Authorization: Bearer <access_token>`. The cart owner is the JWT `sub` claim — do not send `customer_id` on `/api/v1/cart` mutations.

See [cart-service.md](cart-service.md) for architecture, service boundaries, and checkout handoff.

### `GET /api/v1/cart/health`

**Response `200`**
```json
{ "status": "ok" }
```

### Cart (current user)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/cart` | Get my cart |
| DELETE | `/api/v1/cart` | Clear my cart |
| PUT | `/api/v1/cart/items` | Replace all items |
| POST | `/api/v1/cart/items` | Add or update one item |
| DELETE | `/api/v1/cart/items/{sku}` | Remove line by human `sku` |
| DELETE | `/api/v1/cart/items/by-sku-id/{skuId}` | Remove line by canonical `skuId` |

**Add item request**
```json
{ "sku": "BOT-001-BLK", "quantity": 1 }
```

**Cart response** (enriched)
```json
{
  "customer_id": "uuid",
  "items": [
    {
      "sku": "BOT-001-BLK",
      "product_id": "BOT-001",
      "quantity": 1,
      "unit_price_won": 125000,
      "color": "Black",
      "available_qty": 3
    }
  ],
  "unavailable_items": [],
  "subtotal_won": 125000,
  "updated_at": "2026-07-05T12:00:00Z"
}
```

Lines that fail variant enrichment stay in `items` with `available: false` and are listed in `unavailable_items` (`sku_id`, `sku`, `reason`). Item mutations that cannot resolve variants return **`422`** with the same `unavailable_items` array (and `error: "variant not found"`). See [cart-service.md](cart-service.md).
### Admin

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/cart/customers/{customer_id}` | Get a customer's cart (`cart.read`); legacy alias `/api/v1/carts/{customer_id}` |

### Product variant lookup (used by cart)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/products/variants?sku_ids=` | Batch public active variants by canonical `skuId` (comma-separated, max 50). Response `{items, missing}`. |
| GET | `/api/v1/products/variants/by-sku/{sku}` | Public active variant by human SKU (legacy alias: `/api/v1/variants/{sku}`) |
| GET | `/api/v1/products/variants/by-sku-id/{skuId}` | Public active variant by canonical ULID (legacy alias: `/api/v1/variants/by-sku-id/{skuId}`) |

---

## Order Service — `/api/v1/orders`

PostgreSQL-backed (`DUPLI1_ORDER_DB`; in-memory fallback only when no DB URL is set, for tests). Calls inventory to reserve stock and product to redeem coupons.

When `AUTH_JWKS_URL` or `JWT_SECRET` is set, order and checkout routes require `Authorization: Bearer <access_token>` (RS256 via auth JWKS when configured; HS256 fallback in dev).

**Storefront ABAC:** callers with empty `permissions` may only access their own `customer_id` / checkout session (`sub` must match). `order.create` bypasses create ABAC; `order.read.all` bypasses read/list ABAC. See [permissions.md](permissions.md).

**Pricing.** Orders and checkout sessions price as:

```
total_won = subtotal_won - discount_won + shipping_fee_won
```

`discount_won` is the whole goods discount. When the customer belongs to an
automatic tier, `tier_promotion_code` names it and `tier_discount_won` is its
share of `discount_won`; the entered code's share is the rest. On a checkout
session the tier is worked out each time it is read, and complete asks again
(failing with `503` rather than charging a member full price if product
cannot answer).

`shipping_fee_won` is a flat per-order delivery charge in whole KRW, set by `DUPLI1_ORDER_SHIPPING_FEE_WON` on the order service (deprecated aliases: `DUPLI1_ORDER_SHIPPING_FEE_KRW`, `DUPLI1_ORDER_SHIPPING_FEE_CENTS`). It defaults to **0** (free delivery since 2026-10-04); set the variable to a positive amount to charge. JSON, Go identifiers, and Postgres columns for money use `*_won` (`shipping_fee_won`, `subtotal_won`, `discount_won`, `total_won`, `unit_price_won`, `amount_won`, …) — not `*_krw` or `*_cents`. Existing databases rename leftover `*_krw` / `*_cents` columns on migrate.

The charge applies to every order regardless of size — there is no free-shipping threshold. A coupon discounts **goods only** and is capped at `subtotal_won`, so the total can never drop below the shipping fee: a 100%-off coupon still pays delivery. An empty checkout session quotes `total_won: 0` rather than a bare delivery charge; the fee appears once the session has at least one item.

The fee is **snapshotted** on the checkout session when it opens; `complete` charges that quoted fee even if the configured amount changed. Direct `POST /orders` uses the current configured fee. Orders created before this feature carry `shipping_fee_won: 0` and keep their original totals.

**Card surcharge.** An order paid by card costs 10% more. `complete` (and direct `POST /api/v1/orders`) takes an optional `payment_method`: `credit_card`, the default when it is left out, or `bypass`, which needs `payment.bypass` (`403` otherwise) because it is how staff record an offline payment. A card order gets `card_surcharge_won` = `DUPLI1_ORDER_CARD_SURCHARGE_BPS` basis points (default `1000`, 10%; `0` turns it off) of `subtotal_won - discount_won + shipping_fee_won`, rounded down to the won, and `total_won` includes it. The order records `payment_method`, and payment refuses a card checkout on an order priced for `bypass` (`409`), so the surcharge cannot be skipped. The session itself never shows the surcharge, since it does not know the method yet; storefronts quote it from `limits.card_surcharge_bps` on `GET /settings`. The rate is read at `complete`, not snapshotted on the session. Orders placed before this have `payment_method: ""` and `card_surcharge_won: 0`.

Because `total_won` is what the payment service charges and what the order requires to mark itself paid, the fee flows through the money path automatically.

See [checkout-session.md](checkout-session.md) for the full checkout flow.

### `GET /api/v1/orders/health`

**Response `200`**
```json
{ "status": "ok" }
```

### Checkout sessions

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/orders/checkout/sessions` | Create checkout session |
| GET | `/api/v1/orders/checkout/sessions/{id}` | Get session |
| PUT | `/api/v1/orders/checkout/sessions/{id}/items` | Replace all items |
| POST | `/api/v1/orders/checkout/sessions/{id}/items` | Add or update one item |
| DELETE | `/api/v1/orders/checkout/sessions/{id}/items/{sku}` | Remove item by human `sku` |
| DELETE | `/api/v1/orders/checkout/sessions/{id}/items/by-sku-id/{skuId}` | Remove item by canonical `skuId` |
| POST | `/api/v1/orders/checkout/sessions/{id}/promotion` | Apply promotional code; `422` with `reason` / `sub_reason` when the cart does not earn it (pre-rename alias `…/coupon` still answers) |
| DELETE | `/api/v1/orders/checkout/sessions/{id}/promotion` | Remove the applied code |
| POST | `/api/v1/orders/checkout/sessions/{id}/complete` | Complete checkout → order |

The legacy prefix `/api/v1/checkout/sessions…` is still registered as an alias for every route above and will be removed once the storefront and admin clients migrate.

### Orders

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/orders` | Create order directly |
| GET | `/api/v1/orders` | List all orders (`order.read.all`) |
| GET | `/api/v1/orders/events` | Live order stream, Server-Sent Events (`order.read.all`) — see [order-live-events.md](order-live-events.md) |
| GET | `/api/v1/orders/reports/sales?granularity=week\|month&from=YYYY-MM-DD&to=YYYY-MM-DD` | Sales report (`order.read.all`). KST periods: weeks run Monday–Sunday, months are calendar months; `from`/`to` are inclusive and widened to whole periods (default: the last 12 weeks or 12 months up to the current one; at most 104 weeks or 36 months, else `400`). Returns `{granularity, timezone, from, to, periods[], totals}`; each period has `period_start`, `period_end`, `orders`, `gross_won`, `discount_won`, `shipping_fee_won`, `refunds`, `refunded_won`, `net_won`, `average_order_won`. A sale counts in the period of its `paid_at`; a refund (a paid order canceled) counts in the period of its `canceled_at`, so `net_won = gross_won - refunded_won` is what moved in that period. Empty periods are listed |
| GET | `/api/v1/orders?customer_id=` | List customer orders |
| GET | `/api/v1/orders/{id}` | Get order |
| POST | `/api/v1/orders/{id}/confirm` | `order.status.update` — manager accepts a paid order (`paid` → `confirmed`; 2-hour SLA from `paid_at`, auto-confirmed by the sweep otherwise) |
| POST | `/api/v1/orders/{id}/ship` | `order.ship` — ship a confirmed order (`confirmed` → `in_transit`); body requires `carrier` + `tracking_number` (`carrier_note` when `carrier=other`) |
| POST | `/api/v1/orders/{id}/deliver` | `order.ship` — carrier/manager marks delivered (`in_transit` → `delivered`) |
| POST | `/api/v1/orders/{id}/receipt/confirm` | Customer ABAC — confirms receipt (`delivered` → `fulfilled`) |
| POST | `/api/v1/orders/{id}/receipt/dispute` | Customer ABAC — reports non-receipt (`delivered` → `disputed`); optional `{ "reason": "…" }` |
| POST | `/api/v1/orders/{id}/dispute/resolve` | `order.status.update` — closes a dispute in the delivery's favor, no refund (`disputed` → `fulfilled`); a manager siding with the customer instead uses `PUT /status` → `canceled` |
| POST | `/api/v1/orders/{id}/cancel` | Customer ABAC — immediate refund before confirm; cancel request from `confirmed` through `delivered` |
| POST | `/api/v1/orders/{id}/cancel/approve` | `order.status.update` — refund + cancel a customer request |
| POST | `/api/v1/orders/{id}/cancel/reject` | `order.status.update` — keep the order, clear the request |
| PUT | `/api/v1/orders/{id}/status` | `order.status.update` — cancel (from any non-final status) or fulfill (manager override / auto-fulfill sweep, from `delivered` or `disputed`) |

**Create order request**
```json
{
  "customer_id": "cust-1",
  "items": [{ "sku_id": "01J9Z…", "quantity": 1 }]
}
```

Identify each line by canonical `sku_id` (preferred) or human `sku`. Unit prices are **resolved server-side** from the catalog; `unit_price_won` is not part of the request body and is ignored if sent.

**Status machine**

| From | To | Trigger |
|------|----|---------|
| — | `pending` | Order created |
| `pending` | `paid` | `payment.succeeded` consumer or bypass payment — **payment-driven only**, no client route |
| `paid` | `confirmed` | `POST /api/v1/orders/{id}/confirm`, or the refund-policy sweep after the 2-hour SLA |
| `confirmed` | `in_transit` | `POST /api/v1/orders/{id}/ship` (commits reserved stock) |
| `in_transit` | `delivered` | `POST /api/v1/orders/{id}/deliver` |
| `delivered` | `fulfilled` | Customer `POST /receipt/confirm`, manager `PUT /status` `{ "status": "fulfilled" }`, or the delivery-policy sweep 14 days after delivery with no response |
| `delivered` | `disputed` | Customer `POST /receipt/dispute` (customer says the parcel never arrived) |
| `disputed` | `fulfilled` | `POST /api/v1/orders/{id}/dispute/resolve` (manager finds the delivery was fine; no refund) |
| `pending`, `paid`, `confirmed`, `in_transit`, `delivered`, `disputed` | `canceled` | Customer `POST /cancel` (immediate before confirm; a manager-approved request from `confirmed` through `delivered`), manager `PUT /status` `{ "status": "canceled" }` (also how a manager resolves a dispute in the customer's favor), unpaid-expiry worker, or auto-approve of an overdue cancel request. **Paid and later** cancels refund the captured payment (`POST /payments/{payment_id}/cancel`) first; a PG rejection leaves the order unchanged. Once shipped (`in_transit` or later), canceling refunds but never auto-restocks. Unpaid expiry never calls payment. |

`PUT /status` accepts only `canceled` and `fulfilled`; use `POST /ship` to reach `in_transit` and `POST /deliver` to reach `delivered`. See [payment-service.md](payment-service.md).

---

## Payment Service — `/api/v1/payments`

Credit card uses **NANO Solution** certified payment when `NANO_*` credentials are set; otherwise `credit_card` is unavailable (501) and payments — including local testing — go through manager **Bypass**. Dupli1 never handles card numbers, CVC, or card passwords.

**Methods:** create body accepts `method`: `credit_card` (NANO; 501 when unconfigured), `bypass` (order manager / `payment.bypass`), `bitcoin` (501 until implemented). See [payment-methods-plan.md](payment-methods-plan.md).

When JWT is configured, `POST` and `GET` require Bearer tokens. Storefront callers may only pay for / read their own orders unless they hold `payment.create` or `payment.read.all`.

| Method | Path | Permission / rule |
|--------|------|-------------------|
| POST | `/api/v1/payments` | ABAC or `payment.create`; `method=bypass` requires `payment.bypass` |
| GET | `/api/v1/payments/{id}` | ABAC or `payment.read.all` |
| POST | `/api/v1/payments/{id}/cancel` | `payment.cancel` — **staff only, no ABAC** |

**Create payment**
```json
{ "order_id": "ord_000001", "method": "credit_card" }
```

**Bypass (manage-web / order manager)**
```json
{ "order_id": "ord_000001", "method": "bypass", "note": "Cash received" }
```
Returns `status: "succeeded"` immediately and publishes `payment.succeeded` (no `checkout_url`).

**Cancel / refund payment**

`POST /api/v1/payments/{id}/cancel` refunds a `succeeded` payment at the PG (NANO `/api/payment/cancel.io`). Requires `payment.cancel`; there is no ABAC path, so a customer can never refund their own payment.

```json
{ "amount_won": 20000, "reason": "ops reject" }
```

Both fields are optional and an empty body is valid: omitting `amount_won` (or sending `0`) cancels the **full remaining balance**. Send an `Idempotency-Key` header to make a retry of the same cancel a no-op — strongly recommended for partial cancels, which local state alone cannot distinguish from a deliberate second refund.

A full cancel moves the payment to `canceled`. A **partial** cancel leaves it `succeeded` with a reduced remaining balance, matching NANO's `remainAmt` semantics; repeat partials until the balance reaches zero, at which point the payment becomes `canceled`. The response is the updated payment, including `canceled_amount_won` (cumulative), `canceled_at`, `cancel_reason`, and `canceled_by`.

| Status | Meaning |
|--------|---------|
| `200` | Cancel accepted by the PG and recorded |
| `400` | `amount_won` negative or above the remaining balance |
| `403` | Caller lacks `payment.cancel` |
| `404` | No such payment |
| `409` | Payment is not cancelable (not `succeeded`, or already fully canceled) |
| `501` | Provider has no cancel API / PG not configured |
| `502` | PG was reached and refused the cancel — payment left unchanged |

`bypass` payments never touched a PG, so they are canceled locally only and the matching refund is made out of band.

The cancel publishes **`payment.canceled`** (NATS, via the payment outbox). Order cancels a still-`paid` order on a full refund when `remaining_won` is present and `0` and `payment_id` matches; notification alerts ops. Concurrent cancels of the same payment serialize on a row lock so NANO is not called twice.

Unpaid `pending` orders auto-cancel after **5 minutes**. Full design: [payment-service.md](payment-service.md).

---

## Notification Service

Health and settings (`GET /health`, `GET /api/v1/notification/health`, `GET /settings`, `GET /api/v1/notification/settings`). Outbound ops alerts are driven by NATS subscriptions (Telegram when configured). Inbound Telegram uses `POST /api/v1/notification/telegram/webhook` (production, requires `TELEGRAM_WEBHOOK_SECRET`) or `getUpdates` polling (local). Managers manage subscriptions at `/api/v1/notification/telegram/subscriptions` (`notification.telegram.read` / `notification.telegram.manage`). Full runbook: [notification-telegram-bot.md](notification-telegram-bot.md).

`GET /health` returns `{"status":"ok"}` plus a `dependencies` map where one is wired (`postgres`, `nats`), each `{"ok": bool}`, and `status` becomes `"degraded"` when a probe fails. **The code is always `200`** — nothing probes this endpoint, so a caller that wants to act reads `status`. Probe results are cached for 5s and probe errors are logged rather than returned, since the route is unauthenticated.

`POST /api/v1/notification/telegram/subscriptions` returns `400` only when neither `telegram_user_id` nor `chat_id` is given, `409` when the chat ID or Telegram user ID already belongs to another subscription, and `500` for a store failure — it no longer returns the driver's error text.

`GET /api/v1/notification/telegram/subscriptions/{id}` returns one subscription. `PATCH` on the same path changes which alerts a pending or accepted chat receives — `{ "alert_order"?, "alert_product"?, "alert_support"? }`, any flag omitted is left as it is — and takes effect on the next alert. It answers `400` when no flag is sent, `404` for an unknown id and `409` for a rejected subscription.

`POST /api/v1/notification/telegram/webhook` answers `200` once the update is authenticated and parsed, **before** it is processed: processing continues in the background, so the `200` means accepted, not handled. A malformed body is `400`, a bad or missing `X-Telegram-Bot-Api-Secret-Token` is `403`, and an unconfigured secret is `503`.

---

## Support Service — `/api/v1/support`

The customer consultation bot and the manager inbox behind it. The bot is a
**different bot from notification's ops bot**, with its own token and its own
update stream. Full design: [support-telegram-bot.md](support-telegram-bot.md).

Health and settings (`GET /health`, `GET /api/v1/support/health`, `GET /settings`, `GET /api/v1/support/settings`) follow the same shape as every other stdlib service.

### Inbound from Telegram

`POST /api/v1/support/telegram/webhook` — authenticated by `X-Telegram-Bot-Api-Secret-Token` against `TELEGRAM_SUPPORT_WEBHOOK_SECRET`. Mismatched or missing is `403`; unconfigured is `503`; a malformed body is `200` with no side effect, because Telegram retries anything else. Locally the service falls back to `getUpdates` polling and this route is unused.

### Manager inbox

| Method | Path | Permission | Purpose |
|---|---|---|---|
| GET | `/api/v1/support/inquiries` | `support.read` | List. `?queue=waiting` is the 대기 list (open, claimed by nobody), `?assigned_to=me` resolves to the caller, `?status=closed` is the 완료 list |
| GET | `/api/v1/support/inquiries/{id}` | `support.read` | One inquiry **with its transcript** |
| POST | `/api/v1/support/inquiries/{id}/assign` | `support.reply` | Claim it |
| POST | `/api/v1/support/inquiries/{id}/reply` | `support.reply` | `{ "body": "…" }` — send to the shopper |
| POST | `/api/v1/support/inquiries/{id}/close` | `support.reply` | Finish it |
| GET / PUT | `/api/v1/support/answers` | `support.manage` | The bot's canned copy; `PUT` takes `{ "node", "body" }` |
| GET | `/api/v1/support/inquiries/events` | `support.read` | SSE of every inquiry change (ids only) |

Since web chat, `GET /api/v1/support/inquiries` also takes `?channel=web|telegram`, and every inquiry carries `channel`. Web inquiries add `customer_id`, `customer_email`, `customer_last_read_at`, `product_id`, `sku_id` and `order_id`. Transcript lines carry `kind` (`text`, `product_ref`, `order_ref`, `system`), cards add `ref_id` and `ref`, and web replies add `notice_status`. A reply may carry `sku_id` / `order_id` to send a card; on a Telegram inquiry that is `422 reference_on_telegram`.

### Web consultation chat

Signed-in shoppers' side of the same inbox. Full design: [support-web-chat.md](support-web-chat.md).

| Method | Path | Auth | Purpose |
|---|---|---|---|
| GET | `/api/v1/support/web/conversation` | Bearer (customer/manager) | The caller's conversation: open inquiry, last 200 messages, `unread`, service hours |
| POST | `/api/v1/support/web/messages` | Bearer | `{ body?, product_id?, sku_id?, order_id? }`; opens an inquiry when none is open |
| POST | `/api/v1/support/web/read` | Bearer | `204`, marks everything read |
| POST | `/api/v1/support/web/inquiries/current/close` | Bearer | The shopper ends the consultation |
| GET | `/api/v1/support/web/events` | Bearer | SSE of the caller's own changes (ids only) |

Errors carry a `code`: `invalid_message` 400, `invalid_reference` 422 (an order that is not yours reads the same as a missing one), `reference_unavailable` 503, `rate_limited` 429, `customer_required` 403, `chat_unavailable` 503.

List responses carry `{"inquiries": […]}` without transcripts; single-inquiry
responses carry `{"inquiry": {…, "transcript": […]}}`.

**Inquiry JSON carries no Telegram chat id.** The shopper's identity on Telegram
stays inside the service; the console works from the inquiry id alone.

**A reply that could not be delivered is still a `200`**, with
`{"delivered": false, "error": "undeliverable"}` and the message stored in the
transcript with `delivery: "failed"`. The manager did their part and the record
has to say the shopper never got it, so this is not an error response. A
successful reply returns `{"delivered": true}`.

`PUT /api/v1/support/answers` rejects an unknown `node` and an empty `body` with
`400` — Telegram refuses to send an empty message, so an empty answer would kill
that menu node rather than clear it.

**Message bodies are purged after 180 days** (`DUPLI1_SUPPORT_MESSAGE_RETENTION_DAYS`). The purge replaces the body with a placeholder and keeps the row, so a purged transcript still shows who spoke and when.

---

## Common error shape

All error responses use a JSON envelope:

**Auth service** (Gin)
```json
{ "error": "human-readable message" }
```

**Other services** (stdlib)
```json
{ "error": "human-readable message", "code": 400 }
```

---

## Quick reference

Permission strings are authoritative; see [permissions.md](permissions.md). `—` = no auth. `Bearer` = valid access token. `Bearer*` = required when JWT is configured on the service.

| Method | Path | Permission / auth | Service |
|--------|------|-------------------|---------|
| GET | `/gateway/health` | — | nginx |
| GET | `/api/v1/auth/health` | — | auth |
| GET | `/api/v1/auth/settings` | — | auth |
| GET | `/api/v1/auth/.well-known/jwks.json` | — | auth |
| POST | `/api/v1/auth/register` | `user.create` | auth |
| POST | `/api/v1/auth/login` | — | auth |
| GET | `/api/v1/auth/me` | Bearer | auth |
| POST | `/api/v1/auth/refresh` | — | auth |
| POST | `/api/v1/auth/logout` | — | auth |
| GET | `/api/v1/auth/users` | `user.read` | auth |
| GET | `/api/v1/auth/reports/registrations` | `user.read` | auth |
| PATCH | `/api/v1/auth/users/{id}/permissions` | `user.permissions.update` | auth |
| PATCH | `/api/v1/auth/users/{id}/password` | `user.password.update` | auth |
| PATCH | `/api/v1/auth/users/{id}/status` | `user.status.update` | auth |
| GET | `/api/v1/profile/health` | — | profile |
| GET | `/api/v1/profile/settings` | — | profile |
| GET/PATCH | `/api/v1/profile/me/profile` | Bearer | profile |
| GET/POST | `/api/v1/profile/me/addresses` | Bearer | profile |
| GET/PATCH/DELETE | `/api/v1/profile/me/addresses/{id}` | Bearer | profile |
| POST | `/api/v1/profile/me/addresses/{id}/default` | Bearer | profile |
| GET | `/api/v1/products/health` | — | product |
| GET | `/api/v1/products/settings` | — | product |
| GET | `/api/v1/products` | optional `product.read` | product |
| GET | `/api/v1/products/{id}` | — | product |
| POST | `/api/v1/products/promotions/redeem` | — | product |
| POST | `/api/v1/products` | `product.create` | product |
| PUT/DELETE | `/api/v1/products/{id}` | `product.update` / `product.delete` | product |
| POST | `/api/v1/products/{id}/images` | `product.image.upload` | product |
| POST | `/api/v1/products/{id}/variants` | `product.variant.create` | product |
| PUT/DELETE | `/api/v1/products/{id}/variants/{sku}` | `product.variant.update` / `product.variant.delete` | product |
| POST | `/api/v1/products/{id}/variants/{sku}/images` | `product.image.upload` | product |
| GET/POST | `/api/v1/products/promotions` | `promotion.read` / `promotion.create` | product |
| PUT/DELETE | `/api/v1/products/promotions/by-code/{code}` | `promotion.update` / `promotion.delete` | product |
| GET | `/api/v1/products/inventory/health` | — | product |
| GET | `/api/v1/products/inventory/settings` | — | product |
| GET | `/api/v1/products/inventory/items/{sku}` | — | product |
| PUT | `/api/v1/products/inventory/items/{sku}` | `inventory.stock.write` | product |
| POST | `/api/v1/products/inventory/items/{sku}/adjust` | `inventory.stock.write` | product |
| POST | `/api/v1/products/inventory/reservations` | `inventory.reservation.manage` | product |
| POST | `/api/v1/products/inventory/reservations/{id}/commit` | `inventory.reservation.manage` | product |
| POST | `/api/v1/products/inventory/reservations/{id}/release` | `inventory.reservation.manage` | product |
| POST/GET | `/api/v1/orders/checkout/sessions` | ABAC / `order.create` / `order.read.all` | order |
| GET | `/api/v1/orders/health` | — | order |
| GET | `/api/v1/orders/settings` | — | order |
| GET/PUT/POST/DELETE | `/api/v1/orders/checkout/sessions/{id}/...` | ABAC (same as orders) | order |
| POST/GET | `/api/v1/orders` | ABAC / `order.create` / `order.read.all` | order |
| GET | `/api/v1/orders/{id}` | ABAC / `order.read.all` | order |
| POST | `/api/v1/orders/{id}/confirm` | `order.status.update` | order |
| POST | `/api/v1/orders/{id}/ship` | `order.ship` | order |
| POST | `/api/v1/orders/{id}/deliver` | `order.ship` | order |
| POST | `/api/v1/orders/{id}/receipt/confirm` | ABAC | order |
| POST | `/api/v1/orders/{id}/receipt/dispute` | ABAC | order |
| POST | `/api/v1/orders/{id}/dispute/resolve` | `order.status.update` | order |
| POST | `/api/v1/orders/{id}/cancel` | ABAC | order |
| POST | `/api/v1/orders/{id}/cancel/approve` | `order.status.update` | order |
| POST | `/api/v1/orders/{id}/cancel/reject` | `order.status.update` | order |
| PUT | `/api/v1/orders/{id}/status` | `order.status.update` | order |
| GET | `/api/v1/cart/health` | — | cart |
| GET | `/api/v1/cart/settings` | — | cart |
| GET/POST/PUT/DELETE | `/api/v1/cart/*` | Bearer (own `sub`) | cart |
| GET | `/api/v1/cart/customers/{customer_id}` | `cart.read` | cart |
| GET | `/api/v1/payments/health` | — | payment |
| GET | `/api/v1/payments/settings` | — | payment |
| POST/GET | `/api/v1/payments` | ABAC / `payment.create` / `payment.read.all` | payment |
| POST | `/api/v1/payments/{id}/cancel` | `payment.cancel` | payment |
| GET | `/api/v1/notification/health` | — | notification |
| GET | `/api/v1/notification/settings` | — | notification |
| POST | `/api/v1/notification/telegram/webhook` | webhook secret header | notification |
| GET | `/api/v1/notification/telegram/subscriptions` | `notification.telegram.read` | notification |
| POST | `/api/v1/notification/telegram/subscriptions` | `notification.telegram.manage` | notification |
| GET | `/api/v1/notification/telegram/subscriptions/{id}` | `notification.telegram.read` | notification |
| PATCH | `/api/v1/notification/telegram/subscriptions/{id}` | `notification.telegram.manage` | notification |
| POST | `/api/v1/notification/telegram/subscriptions/{id}/accept` | `notification.telegram.manage` | notification |
| POST | `/api/v1/notification/telegram/subscriptions/{id}/reject` | `notification.telegram.manage` | notification |
| DELETE | `/api/v1/notification/telegram/subscriptions/{id}` | `notification.telegram.manage` | notification |
| GET | `/api/v1/support/health` | — | support |
| GET | `/api/v1/support/settings` | — | support |
| POST | `/api/v1/support/telegram/webhook` | webhook secret header | support |
| GET | `/api/v1/support/inquiries` | `support.read` | support |
| GET | `/api/v1/support/inquiries/{id}` | `support.read` | support |
| POST | `/api/v1/support/inquiries/{id}/assign` | `support.reply` | support |
| POST | `/api/v1/support/inquiries/{id}/reply` | `support.reply` | support |
| POST | `/api/v1/support/inquiries/{id}/close` | `support.reply` | support |
| GET/PUT | `/api/v1/support/answers` | `support.manage` | support |
| GET | `/api/v1/support/inquiries/events` | `support.read` | support |
| GET | `/api/v1/support/web/conversation` | Bearer (own) | support |
| POST | `/api/v1/support/web/messages` | Bearer (own) | support |
| POST | `/api/v1/support/web/read` | Bearer (own) | support |
| POST | `/api/v1/support/web/inquiries/current/close` | Bearer (own) | support |
| GET | `/api/v1/support/web/events` | Bearer (own) | support |
