# Dupli1 API Reference

All traffic is routed through the nginx gateway. Locally use **HTTP** at `http://localhost:8080` or `http://localhost` (port 80). Production terminates TLS at the load balancer or gateway.

---

## Authentication

Protected routes require an `Authorization` header with a Bearer **access** token:

```
Authorization: Bearer <access_token>
```

**Token flow**

1. `POST /api/v1/auth/login` → `{ "refresh_token": "<jwt>" }`
2. `POST /api/v1/auth/refresh` with that refresh token → `{ "token": "<access_jwt>" }`
3. Use the access token on protected routes until it expires (default 15 min), then refresh again.

**Access token claims**

| Claim | Type | Notes |
|-------|------|-------|
| `sub` | string | User ID |
| `type` | string | `"access"` |
| `permissions` | string[] | Fine-grained authorization strings such as `product.create`, `order.ship`, `*` |
| `exp`, `iat` | number | Standard JWT timestamps |

Refresh tokens contain `sub` and `type: "refresh"` only. Permissions are loaded from the database on every refresh.

Protected routes check the `permissions` claim. See [permissions.md](permissions.md) for the full catalog and endpoint matrix.

Wildcards: `*` (everything), `admin.*` (user-admin domain), `{resource}.*` (e.g. `product.*`).

### Account types

Every user has an `account_type` field (JSON key `account_type`) separate from **permissions**:

| Value | Meaning | Typical permissions |
|-------|---------|---------------------|
| `customer` | End-user storefront account | `[]` (empty — ABAC self-service only) |
| `admin` | Human operator | `admin.*`, `product.*`, … or `*` (owner) |
| `service` | Machine / integration account | `user.create`, `order.ship`, … per job function |

Seeded accounts: owner (`OWNER_EMAIL`) → `permissions: ["*"]`; `dupli1-web` → `["user.create"]`; `dupli1-order` → `["order.ship", "order.status.update", "inventory.reservation.manage"]`. `POST /register` defaults to `customer` when `account_type` is omitted.

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
| `password` | string | required, min 8 chars |
| `account_type` | string | optional; one of `customer`, `admin`, `service`; defaults to `customer`. Callers with only `user.create` (no `admin.*` or `*`) may register `customer` only |

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
| `DUPLI1_WEB_SERVICE_PASSWORD` | Service account password (required when email is set) |

`dupli1-web` should log in with these credentials server-side, cache/refresh the access token, and call register from the backend only — never expose the service password to browsers.

---

### `POST /api/v1/auth/login`

Authenticate and receive a refresh token.

**Request body**
```json
{
  "email": "user@example.com",
  "password": "minlen8"
}
```

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
| `401` | Invalid credentials |
| `403` | Account locked or deactivated |

---

### `GET /api/v1/auth/me`

Return the currently authenticated user's profile.

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
  "failed_login_attempts": 0
}
```

**Errors**
| Status | Meaning |
|--------|---------|
| `401` | Missing, malformed, or expired access token |
| `404` | User no longer exists |

---

### `POST /api/v1/auth/refresh`

Exchange a refresh token for a new access token.

**Request body**
```json
{ "refresh_token": "<jwt>" }
```

**Response `200`**
```json
{
  "token": "<access_jwt>"
}
```

**Errors**
| Status | Meaning |
|--------|---------|
| `400` | Missing or malformed body |
| `401` | Refresh token invalid or expired |

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
      "email": "admin@dupli1.com",
      "account_type": "admin",
      "permissions": ["*"],
      "is_active": true,
      "locked_at": null,
      "failed_login_attempts": 0
    }
  ]
}
```

**Errors**
| Status | Meaning |
|--------|---------|
| `401` | Missing or invalid access token |
| `403` | Caller lacks `user.read` or management hierarchy forbids listing |

---

### `PATCH /api/v1/auth/users/{id}/permissions`

Replace the permission list for a user. Requires `user.permissions.update`. Subject to auth ABAC hierarchy (who may manage whom).

**Request body**
```json
{
  "permissions": ["user.password.update", "user.status.update"],
  "account_type": "admin"
}
```

| Field | Type | Constraints |
|-------|------|-------------|
| `permissions` | string[] | required |
| `account_type` | string | optional; one of `customer`, `admin`, `service` |

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
| `422` | Password too short (min 8 chars) |

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

## Product Service — `/api/v1/products`

### `GET /api/v1/products/health`

Product service liveness check.

**Response `200`**
```json
{ "status": "ok" }
```

---

### `GET /api/v1/products`

Search **parent styles** (one row per style; colors are not duplicated). No authentication required for the public catalog view (active parents only). With a valid Bearer token that includes `product.read` (or `product.*` / `*`), returns all statuses and includes cost.

| Filter | Match type |
|--------|-----------|
| `category` | exact (e.g. `bags`) |
| `brand` | case-insensitive substring |
| `color` | parent has an active variant with this color |
| `size` | parent has an active variant with this size |
| `material` | exact |
| `tags` | parent must include all listed tags (comma-separated or repeated) |
| `status` | exact (`product.read` or wildcard required) |

Example: `GET /api/v1/products?category=bags&color=Green`

**Response `200`**
```json
{
  "total": 1,
  "results": [
    {
      "id": "BOT-001",
      "name": "Cassette Bag",
      "description": "...",
      "price": 2500.00,
      "brand": "Bottega Veneta",
      "color": "Green",
      "material": "Leather",
      "stock": 5,
      "category": "bags",
      "capacity": "Medium",
      "tags": ["hot"],
      "imageUrls": ["https://cdn.example/bot-001.jpg"]
    }
  ]
}
```

---

### `POST /api/v1/coupons/redeem`

Redeem a coupon code. No authentication required.

**Request body**
```json
{ "code": "SUMMER20" }
```

**Response `200`** — coupon object

**Errors**
| Status | Meaning |
|--------|---------|
| `404` | Invalid coupon code |

---

### `GET /api/v1/products/{id}`

Public PDP. No authentication required. Returns an active **parent** with `variants[]`, `availableColors`, and `availableSizes`. Omits `cost`. Cart lines use each variant's `sku` (inventory key).

**Planned:** unique view counting via `dupli1_guest` cookie and public `viewCount` — see [product-guest-views-plan.md](product-guest-views-plan.md).

**Response `200`** — parent product object with variants

**Errors**
| Status | Meaning |
|--------|---------|
| `404` | Product not found or not active |

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
| GET | `/api/v1/coupons` | `coupon.read` |
| POST | `/api/v1/coupons` | `coupon.create` |
| PUT | `/api/v1/coupons/{code}` | `coupon.update` |
| DELETE | `/api/v1/coupons/{code}` | `coupon.delete` |

Parent IDs use the brand prefix (e.g. `BOT-001`). Dual identity and master dictionaries: [product-sku-system.md](product-sku-system.md) — ULID `skuId` (canonical) + human `sku` (`Brand_Style_Color[_Edition]_Size`). Catalog CRUD at `/api/v1/catalog/…`. See also [product-variants-plan.md](product-variants-plan.md).

---

## Inventory — `/api/v1/inventory` (served by the product service)

Merged into the product service; same routes as the former standalone
inventory service. Each route also has a `by-sku-id/{skuId}` sibling keyed by
the variant's canonical ULID `skuId`. **Reads are public.** Writes require
Bearer JWT when `AUTH_JWKS_URL` is configured.

| Method | Path | Permission |
|--------|------|------------|
| GET | `/api/v1/inventory/{sku}` | — (public) |
| PUT | `/api/v1/inventory/{sku}` | `inventory.stock.write` |
| POST | `/api/v1/inventory/{sku}/adjust` | `inventory.stock.write` |
| POST | `/api/v1/inventory/reservations` | `inventory.reservation.manage` |
| POST | `/api/v1/inventory/reservations/{id}/commit` | `inventory.reservation.manage` |
| POST | `/api/v1/inventory/reservations/{id}/release` | `inventory.reservation.manage` |

### `GET /api/v1/inventory/health`

**Response `200`**
```json
{ "status": "ok" }
```

### `GET /api/v1/inventory/{sku}`

Get stock for a SKU.

### `PUT /api/v1/inventory/{sku}`

Set stock quantity.

**Request body**
```json
{ "quantity": 100 }
```

### `POST /api/v1/inventory/{sku}/adjust`

Adjust stock by delta.

**Request body**
```json
{ "delta": -5 }
```

### `POST /api/v1/inventory/reservations`

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

### `POST /api/v1/inventory/reservations/{id}/commit`

Commit a reservation (deduct stock).

### `POST /api/v1/inventory/reservations/{id}/release`

Release a reservation (return stock).

---

## Cart Service — `/api/v1/cart`

PostgreSQL-backed persistent cart. Enriches lines from product (price, images) and inventory (availability). Does **not** reserve stock or create orders.

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
| DELETE | `/api/v1/cart/items/{sku}` | Remove line |

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
      "unit_price_cents": 125000,
      "color": "Black",
      "available_qty": 3
    }
  ],
  "subtotal_cents": 125000,
  "updated_at": "2026-07-05T12:00:00Z"
}
```

### Admin

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/carts/{customer_id}` | Get a customer's cart (`cart.read`) |

### Product variant lookup (used by cart)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/variants/{sku}` | Public active variant by SKU |

---

## Order Service — `/api/v1`

In-memory store. Calls inventory to reserve stock and product to redeem coupons.

When `AUTH_JWKS_URL` or `JWT_SECRET` is set, order and checkout routes require `Authorization: Bearer <access_token>` (RS256 via auth JWKS when configured; HS256 fallback in dev).

**Storefront ABAC:** callers with empty `permissions` may only access their own `customer_id` / checkout session (`sub` must match). `order.create` bypasses create ABAC; `order.read.all` bypasses read/list ABAC. See [permissions.md](permissions.md).

See [checkout-session.md](checkout-session.md) for the full checkout flow.

### `GET /api/v1/orders/health`

**Response `200`**
```json
{ "status": "ok" }
```

### Checkout sessions

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/checkout/sessions` | Create checkout session |
| GET | `/api/v1/checkout/sessions/{id}` | Get session |
| PUT | `/api/v1/checkout/sessions/{id}/items` | Replace all items |
| POST | `/api/v1/checkout/sessions/{id}/items` | Add or update one item |
| DELETE | `/api/v1/checkout/sessions/{id}/items/{sku}` | Remove item |
| POST | `/api/v1/checkout/sessions/{id}/coupon` | Apply coupon |
| POST | `/api/v1/checkout/sessions/{id}/complete` | Complete checkout → order |

### Orders

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/orders` | Create order directly |
| GET | `/api/v1/orders?customer_id=` | List customer orders |
| GET | `/api/v1/orders/{id}` | Get order |
| POST | `/api/v1/orders/{id}/ship` | `order.ship` — ship order (`paid` → `in_transit`) |
| PUT | `/api/v1/orders/{id}/status` | `order.status.update` — cancel or fulfill |

**Create order request**
```json
{
  "customer_id": "cust-1",
  "items": [{ "sku": "BOT-001", "quantity": 1, "unit_price_cents": 250000 }]
}
```

Supported status transitions: `pending` → `confirmed` | `canceled`; `confirmed` → `fulfilled`.

**Planned:** `pending` → `confirmed` will be **payment-service only** after Stripe Checkout succeeds. See [payment-service.md](payment-service.md).

---

## Payment Service — `/api/v1/payments`

Stripe Checkout **redirect** — Dupli1 never handles card numbers, CVC, or card passwords.

When JWT is configured, `POST` and `GET` require Bearer tokens. Storefront callers may only pay for / read their own orders unless they hold `payment.create` or `payment.read.all`.

| Method | Path | Permission / rule |
|--------|------|-------------------|
| POST | `/api/v1/payments` | ABAC or `payment.create` |
| GET | `/api/v1/payments/{id}` | ABAC or `payment.read.all` |
| POST | `/api/v1/payments/webhooks/stripe` | — (Stripe signature) |
| GET | `/api/v1/payments/{id}/simulate-success` | — (dev only) |

Unpaid `pending` orders auto-cancel after **5 minutes**. Full design: [payment-service.md](payment-service.md).

---

## Notification Service

Health and settings (`GET /health`, `GET /api/v1/notification/health`, `GET /settings`, `GET /api/v1/notification/settings`). Outbound messaging is driven by NATS subscriptions (Telegram when configured).

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
| PATCH | `/api/v1/auth/users/{id}/permissions` | `user.permissions.update` | auth |
| PATCH | `/api/v1/auth/users/{id}/password` | `user.password.update` | auth |
| PATCH | `/api/v1/auth/users/{id}/status` | `user.status.update` | auth |
| GET | `/api/v1/products/health` | — | product |
| GET | `/api/v1/products/settings` | — | product |
| GET | `/api/v1/products` | optional `product.read` | product |
| GET | `/api/v1/products/{id}` | — | product |
| POST | `/api/v1/coupons/redeem` | — | product |
| POST | `/api/v1/products` | `product.create` | product |
| PUT/DELETE | `/api/v1/products/{id}` | `product.update` / `product.delete` | product |
| POST | `/api/v1/products/{id}/images` | `product.image.upload` | product |
| POST | `/api/v1/products/{id}/variants` | `product.variant.create` | product |
| PUT/DELETE | `/api/v1/products/{id}/variants/{sku}` | `product.variant.update` / `product.variant.delete` | product |
| POST | `/api/v1/products/{id}/variants/{sku}/images` | `product.image.upload` | product |
| GET/POST/PUT/DELETE | `/api/v1/coupons` | `coupon.read` / `coupon.create` / `coupon.update` / `coupon.delete` | product |
| GET | `/api/v1/inventory/health` | — | product |
| GET | `/api/v1/inventory/settings` | — | product |
| GET | `/api/v1/inventory/{sku}` | — | product |
| PUT | `/api/v1/inventory/{sku}` | `inventory.stock.write` | product |
| POST | `/api/v1/inventory/{sku}/adjust` | `inventory.stock.write` | product |
| POST | `/api/v1/inventory/reservations` | `inventory.reservation.manage` | product |
| POST | `/api/v1/inventory/reservations/{id}/commit` | `inventory.reservation.manage` | product |
| POST | `/api/v1/inventory/reservations/{id}/release` | `inventory.reservation.manage` | product |
| POST/GET | `/api/v1/checkout/sessions` | ABAC / `order.create` / `order.read.all` | order |
| GET | `/api/v1/orders/health` | — | order |
| GET | `/api/v1/orders/settings` | — | order |
| GET/PUT/POST/DELETE | `/api/v1/checkout/sessions/{id}/...` | ABAC (same as orders) | order |
| POST/GET | `/api/v1/orders` | ABAC / `order.create` / `order.read.all` | order |
| GET | `/api/v1/orders/{id}` | ABAC / `order.read.all` | order |
| POST | `/api/v1/orders/{id}/ship` | `order.ship` | order |
| PUT | `/api/v1/orders/{id}/status` | `order.status.update` | order |
| GET | `/api/v1/cart/health` | — | cart |
| GET | `/api/v1/cart/settings` | — | cart |
| GET/POST/PUT/DELETE | `/api/v1/cart/*` | Bearer (own `sub`) | cart |
| GET | `/api/v1/carts/{customer_id}` | `cart.read` | cart |
| GET | `/api/v1/payments/health` | — | payment |
| GET | `/api/v1/payments/settings` | — | payment |
| POST/GET | `/api/v1/payments` | ABAC / `payment.create` / `payment.read.all` | payment |
| GET | `/api/v1/notification/health` | — | notification |
| GET | `/api/v1/notification/settings` | — | notification |
