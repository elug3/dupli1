# API Endpoints

All services listen on port `8080` inside Docker. The nginx gateway proxies by path prefix with no stripping, so gateway paths match service paths.

| Gateway prefix | Upstream service |
|---|---|
| `/gateway/health` | nginx (static) |
| `/api/v1/auth/` | `dupli1-auth:8080` |
| `/api/v1/products` | `dupli1-product:8080` |
| `/api/v1/coupons` | `dupli1-product:8080` |
| `/api/v1/inventory/` | `dupli1-product:8080` |
| `/api/v1/orders` | `dupli1-order:8080` |
| `/api/v1/checkout` | `dupli1-order:8080` |
| `/api/v1/cart` | `dupli1-cart:8080` |
| `/api/v1/carts/` | `dupli1-cart:8080` |
| `/api/v1/payments` | `dupli1-payment:8080` |
| `/api/v1/payments/` | `dupli1-payment:8080` |

Local gateway: `http://localhost:8080` (also host port 80).

Each service also registers `/health` and `/settings` directly for internal/sidecar use (product uses versioned paths only: `/api/v1/products/health` and `/api/v1/products/settings`).

`GET /settings` (and `GET /api/v1/<service>/settings`) returns non-secret operational configuration: service name, auth mode, storage backend, feature flags, and dependency hostnames. Secrets, DSNs, and API keys are never included.

---

## Auth Service

| Method | Path | Permission | Description |
|---|---|---|---|
| `GET` | `/api/v1/auth/health` | — | Health check |
| `GET` | `/api/v1/auth/settings` | — | Non-secret service settings |
| `POST` | `/api/v1/auth/register` | `user.create` | Create a new user account |
| `POST` | `/api/v1/auth/login` | — | Login and receive a refresh token |
| `POST` | `/api/v1/auth/logout` | — | Invalidate the current session |
| `POST` | `/api/v1/auth/refresh` | — | Exchange a refresh token for a new access token |
| `GET` | `/api/v1/auth/me` | Bearer | Return the authenticated user's profile |
| `GET` | `/api/v1/auth/users` | `user.read` | List users (filtered by auth ABAC hierarchy) |
| `PATCH` | `/api/v1/auth/users/:id/permissions` | `user.permissions.update` | Replace a user's permissions (optional `account_type`) |
| `PATCH` | `/api/v1/auth/users/:id/password` | `user.password.update` | Set a new password for a user |
| `PATCH` | `/api/v1/auth/users/:id/status` | `user.status.update` | Activate or deactivate a user |

**dupli1-web service account:** set `DUPLI1_WEB_SERVICE_EMAIL` and `DUPLI1_WEB_SERVICE_PASSWORD` on `dupli1-auth` to seed a machine user with `permissions: ["user.create"]` and `account_type` `service`. That account may register customers only (`account_type` `customer`).

**Account types:** `customer`, `admin`, `service` — returned on user objects as `account_type`. Distinct from **permissions** (fine-grained authorization strings).

See [permissions.md](permissions.md) for the full catalog, JWT claim shape, and auth ABAC hierarchy.

### GET /api/v1/auth/health

Response `200`: `{"status":"ok"}`

### GET /api/v1/auth/settings

Response `200` JSON with non-secret operational settings (`service`, `api_version`, `auth`, `storage`, `features`, `limits`, `dependencies`).

### POST /api/v1/auth/register

Header: `Authorization: Bearer <access_token>` (requires `user.create`)

Request:
```json
{
  "email": "user@example.com",
  "password": "minlen8",
  "account_type": "customer"
}
```

`account_type` is optional (`customer`, `admin`, or `service`); defaults to `customer`. Callers with only `user.create` may register `customer` accounts only.

Response `201`:
```json
{ "user_id": "uuid" }
```

Errors: `400` bad request, `401` missing/invalid token, `403` insufficient permission, `409` user already exists, `422` invalid email/password/account_type, `500` internal error.

### POST /api/v1/auth/login

Request:
```json
{ "email": "user@example.com", "password": "secret" }
```

Response `200`:
```json
{ "refresh_token": "<token>" }
```

Errors: `400` bad request, `401` invalid credentials.

### POST /api/v1/auth/logout

Request:
```json
{ "refresh_token": "<token>" }
```

Response `204` (no body).

### POST /api/v1/auth/refresh

Request:
```json
{ "refresh_token": "<token>" }
```

Response `200`:
```json
{ "token": "<new_access_token>" }
```

Errors: `400` bad request, `401` invalid or expired token.

### GET /api/v1/auth/me

Header: `Authorization: Bearer <access_token>`

Response `200`:
```json
{
  "user_id": "uuid",
  "email": "user@example.com",
  "account_type": "customer",
  "permissions": [],
  "is_active": true,
  "locked_at": null,
  "failed_login_attempts": 0
}
```

Errors: `401` missing or invalid token, `404` user not found.

### GET /api/v1/auth/users

Header: `Authorization: Bearer <access_token>` (requires `user.read`)

Response `200`:
```json
{
  "users": [
    {
      "user_id": "uuid",
      "email": "user@example.com",
      "account_type": "customer",
      "permissions": [],
      "is_active": true,
      "locked_at": null,
      "failed_login_attempts": 0
    }
  ]
}
```

Errors: `401` missing or invalid token, `403` caller lacks `user.read`, `500` internal error.

### PATCH /api/v1/auth/users/:id/permissions

Header: `Authorization: Bearer <access_token>` (requires `user.permissions.update`)

Request:
```json
{
  "permissions": ["user.password.update", "user.status.update"],
  "account_type": "admin"
}
```

Response `200`: user object (same shape as list item).

Errors: `400` bad request, `401` missing/invalid token, `403` insufficient permission, `404` user not found, `422` invalid account_type/permission, `500` internal error.

### PATCH /api/v1/auth/users/:id/password

Header: `Authorization: Bearer <access_token>` (requires `user.password.update`)

Request:
```json
{ "password": "newpassword" }
```

Response `204` (no body).

Errors: `400` bad request, `401` missing/invalid token, `403` insufficient permission, `404` user not found, `422` password too short, `500` internal error.

### PATCH /api/v1/auth/users/:id/status

Header: `Authorization: Bearer <access_token>` (requires `user.status.update`)

Request:
```json
{ "is_active": false }
```

Response `200`: user object (same shape as list item).

Errors: `400` bad request, `401` missing/invalid token, `403` insufficient permission, `404` user not found, `500` internal error.

---

## Product Service

| Method | Path | Permission | Description |
|---|---|---|---|
| `GET` | `/api/v1/products/health` | — | Health check |
| `GET` | `/api/v1/products/settings` | — | Non-secret service settings |
| `GET` | `/api/v1/products` | optional `product.read` | Search **parent styles**; public active-only; `product.read` adds drafts/cost |
| `GET` | `/api/v1/products/{id}` | — | Parent PDP with `variants[]`, `availableColors`, `availableSizes` |
| `POST` | `/api/v1/coupons/redeem` | — | Redeem a coupon code |
| `POST` | `/api/v1/products` | `product.create` | Create parent |
| `PUT` | `/api/v1/products/{id}` | `product.update` | Update parent |
| `DELETE` | `/api/v1/products/{id}` | `product.delete` | Delete parent (cascades variants) |
| `POST` | `/api/v1/products/{id}/images` | `product.image.upload` | Upload image to default variant |
| `POST` | `/api/v1/products/{id}/variants` | `product.variant.create` | Create variant (SKU) |
| `PUT` | `/api/v1/products/{id}/variants/{sku}` | `product.variant.update` | Update variant |
| `DELETE` | `/api/v1/products/{id}/variants/{sku}` | `product.variant.delete` | Delete variant |
| `POST` | `/api/v1/products/{id}/variants/{sku}/images` | `product.image.upload` | Upload image for variant |
| `GET` | `/api/v1/catalog/brands` | `product.master.read` | List brand codes |
| `POST` | `/api/v1/catalog/brands` | `product.master.write` | Create brand `{code,name}` |
| `PATCH` | `/api/v1/catalog/brands/{code}` | `product.master.write` | Rename brand |
| `DELETE` | `/api/v1/catalog/brands/{code}` | `product.master.write` | Delete brand (409 if in use) |
| `GET` | `/api/v1/catalog/brands/{code}/styles` | `product.master.read` | List styles for brand |
| `POST` | `/api/v1/catalog/brands/{code}/styles` | `product.master.write` | Create style |
| `PATCH` | `/api/v1/catalog/brands/{code}/styles/{styleCode}` | `product.master.write` | Rename style |
| `DELETE` | `/api/v1/catalog/brands/{code}/styles/{styleCode}` | `product.master.write` | Delete style (409 if in use) |
| `GET` | `/api/v1/catalog/colors` | `product.master.read` | List colors |
| `POST` | `/api/v1/catalog/colors` | `product.master.write` | Create color |
| `PATCH` | `/api/v1/catalog/colors/{code}` | `product.master.write` | Rename color |
| `DELETE` | `/api/v1/catalog/colors/{code}` | `product.master.write` | Delete color (409 if in use) |
| `GET` | `/api/v1/catalog/sizes` | `product.master.read` | List sizes |
| `POST` | `/api/v1/catalog/sizes` | `product.master.write` | Create size |
| `PATCH` | `/api/v1/catalog/sizes/{code}` | `product.master.write` | Rename size |
| `DELETE` | `/api/v1/catalog/sizes/{code}` | `product.master.write` | Delete size (409 if in use) |
| `GET` | `/api/v1/catalog/editions` | `product.master.read` | List editions (VariantCode) |
| `POST` | `/api/v1/catalog/editions` | `product.master.write` | Create edition |
| `PATCH` | `/api/v1/catalog/editions/{code}` | `product.master.write` | Rename edition |
| `DELETE` | `/api/v1/catalog/editions/{code}` | `product.master.write` | Delete edition (409 if in use) |
| `GET` | `/api/v1/coupons` | `coupon.read` | List coupons |
| `POST` | `/api/v1/coupons` | `coupon.create` | Create coupon |
| `PUT` | `/api/v1/coupons/{code}` | `coupon.update` | Update coupon |
| `DELETE` | `/api/v1/coupons/{code}` | `coupon.delete` | Delete coupon |

Public search defaults to `status = active` on the **parent**. Query filters: `category`, `brand`, `material`, `tags`, `color`, `size` (color/size match any active variant). Managers may also pass `status`. Checkout uses **variant SKU** (human `sku` or canonical `skuId`) with inventory. Identity + masters: [product-sku-system.md](product-sku-system.md). See also [product-variants-plan.md](product-variants-plan.md).

### Catalog master data

Code → name dictionaries for SKU segments. See [product-sku-system.md](product-sku-system.md). Permissions: `product.master.read` / `product.master.write`.

### GET /api/v1/products/health

Response `200`:
```json
{ "status": "ok" }
```

### GET /api/v1/products/settings

Response `200` JSON with non-secret operational settings. Also available at `/api/v1/inventory/settings`.

### GET /api/v1/products

Returns **one row per parent style** (not per color). Query params: `category`, `brand` (partial), `material`, `tags`, `color`, `size`, and `status` (managers only).

Example: `GET /api/v1/products?category=bags&color=Green`

Response `200`:
```json
{
  "total": 1,
  "results": [
    {
      "id": "BOT-001",
      "name": "Cassette Bag",
      "brand": "Bottega Veneta",
      "category": "bags",
      "status": "active",
      "priceFrom": 2500,
      "defaultImageUrl": "https://cdn.example/green-1.jpg",
      "availableColors": ["Green", "Black"],
      "availableSizes": []
    }
  ]
}
```

### GET /api/v1/products/{id}

Public PDP: parent plus `variants[]` (active only), `availableColors`, `availableSizes`. Returns `404` for draft/archived parents. `cost` is omitted. Cart/checkout use each variant's `sku`.

### GET /api/v1/variants/{sku}

Public variant lookup by SKU. Returns `404` when the variant or parent product is not active. Used by the cart service for price validation.

---

## Cart Service

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/api/v1/cart/health` | — | Health check |
| `GET` | `/api/v1/cart/settings` | — | Non-secret service settings |
| `GET` | `/api/v1/cart` | Bearer | Get current user's cart |
| `DELETE` | `/api/v1/cart` | Bearer | Clear current user's cart |
| `PUT` | `/api/v1/cart/items` | Bearer | Replace all cart items |
| `POST` | `/api/v1/cart/items` | Bearer | Add or update one item |
| `DELETE` | `/api/v1/cart/items/{sku}` | Bearer | Remove one item |
| `GET` | `/api/v1/carts/{customer_id}` | `cart.read` | Get a customer's cart |

See [cart-service.md](cart-service.md) for architecture, boundaries with inventory/order, and checkout handoff.

---

## Payment Service

| Method | Path | Permission / rule | Description |
|---|---|---|---|
| `GET` | `/api/v1/payments/health` | — | Health check |
| `GET` | `/api/v1/payments/settings` | — | Non-secret service settings |
| `POST` | `/api/v1/payments` | ABAC / `payment.create` | Start Stripe Checkout for a pending order |
| `GET` | `/api/v1/payments/{id}` | ABAC / `payment.read.all` | Payment status |
| `POST` | `/api/v1/payments/webhooks/stripe` | Stripe signature | Webhook handler |
| `GET` | `/api/v1/payments/{id}/simulate-success` | — | Dev only (no Stripe key): mark payment succeeded |

See [payment-service.md](payment-service.md) for Stripe redirect flow, 5-minute auto-cancel, and `payment.succeeded` → `paid`.

---

## Inventory (served by the product service)

Merged into `dupli1-product` — same routes as the former standalone inventory
service. Each route also has a `by-sku-id/{skuId}` sibling keyed by the
variant's canonical ULID `skuId` (e.g. `GET /api/v1/inventory/by-sku-id/{skuId}`).

| Method | Path | Permission | Description |
|---|---|---|---|
| `GET` | `/api/v1/inventory/health` | — | Health check |
| `GET` | `/api/v1/inventory/settings` | — | Non-secret product-service settings |
| `GET` | `/api/v1/inventory/{sku}` | — | Get a stock item by SKU |
| `PUT` | `/api/v1/inventory/{sku}` | `inventory.stock.write` | Create or overwrite stock quantity |
| `POST` | `/api/v1/inventory/{sku}/adjust` | `inventory.stock.write` | Add or subtract stock (delta) |
| `POST` | `/api/v1/inventory/reservations` | `inventory.reservation.manage` | Create a reservation |
| `POST` | `/api/v1/inventory/reservations/{id}/commit` | `inventory.reservation.manage` | Commit a reservation |
| `POST` | `/api/v1/inventory/reservations/{id}/release` | `inventory.reservation.manage` | Release a reservation |

### GET /api/v1/inventory/health

Response `200`:
```json
{ "status": "ok" }
```

### GET /api/v1/inventory/{sku}

Response `200`:
```json
{
  "sku": "SHOE-001",
  "quantity": 100,
  "reserved": 10,
  "updated_at": "2026-06-23T12:00:00Z"
}
```

### PUT /api/v1/inventory/{sku}

Request:
```json
{ "quantity": 50 }
```

Response `200`: stock item object (same shape as GET).

### POST /api/v1/inventory/{sku}/adjust

Request:
```json
{ "delta": -5 }
```

Response `200`: updated stock item. Errors: `400` insufficient stock.

### POST /api/v1/inventory/reservations

Request:
```json
{
  "order_id": "ord-abc",
  "items": [
    { "sku": "SHOE-001", "quantity": 2 }
  ]
}
```

Response `201`:
```json
{
  "reservation_id": "res-xyz",
  "reservation": {
    "id": "res-xyz",
    "order_id": "ord-abc",
    "items": [ { "sku": "SHOE-001", "quantity": 2 } ],
    "status": "active",
    "created_at": "...",
    "updated_at": "..."
  }
}
```

### POST /api/v1/inventory/reservations/{id}/commit

### POST /api/v1/inventory/reservations/{id}/release

Both return `200` with the updated reservation object. Errors: `404` not found, `400` reservation already closed.

---

## Order Service

Requires `Authorization: Bearer <access_token>` when `AUTH_JWKS_URL` or `JWT_SECRET` is set (RS256 via auth JWKS in Compose). Storefront callers use ABAC on `customer_id` / session owner; `order.create` and `order.read.all` bypass ABAC. See [checkout-session.md](checkout-session.md) and [permissions.md](permissions.md).

| Method | Path | Permission / rule | Description |
|---|---|---|---|
| `GET` | `/api/v1/orders/health` | — | Health check |
| `GET` | `/api/v1/orders/settings` | — | Non-secret service settings |
| `POST` | `/api/v1/checkout/sessions` | ABAC / `order.create` | Create checkout session |
| `GET` | `/api/v1/checkout/sessions/{id}` | ABAC / `order.read.all` | Get session |
| `PUT` | `/api/v1/checkout/sessions/{id}/items` | ABAC / `order.create` | Replace all items |
| `POST` | `/api/v1/checkout/sessions/{id}/items` | ABAC / `order.create` | Add or update one item |
| `DELETE` | `/api/v1/checkout/sessions/{id}/items/{sku}` | ABAC / `order.create` | Remove item |
| `POST` | `/api/v1/checkout/sessions/{id}/coupon` | ABAC / `order.create` | Apply coupon |
| `POST` | `/api/v1/checkout/sessions/{id}/complete` | ABAC / `order.create` | Complete checkout |
| `POST` | `/api/v1/orders` | ABAC / `order.create` | Create a new order |
| `GET` | `/api/v1/orders?customer_id={id}` | ABAC / `order.read.all` | List orders for a customer |
| `GET` | `/api/v1/orders/{id}` | ABAC / `order.read.all` | Get a single order |
| `POST` | `/api/v1/orders/{id}/ship` | `order.ship` | Ship order (`paid` → `in_transit`, commit stock) |
| `PUT` | `/api/v1/orders/{id}/status` | `order.status.update` | Cancel or fulfill |

### GET /api/v1/orders/health

Response `200`:
```json
{ "status": "ok" }
```

### POST /api/v1/orders

Request:
```json
{
  "customer_id": "cust-123",
  "items": [
    { "sku": "SHOE-001", "quantity": 1, "unit_price_cents": 9900 }
  ]
}
```

Response `201`: order object.

### GET /api/v1/orders?customer_id=cust-123

Response `200`:
```json
{ "total": 2, "orders": [ /* order objects */ ] }
```

### GET /api/v1/orders/{id}

Response `200`: order object.

### PUT /api/v1/orders/{id}/status

Requires `order.status.update`. **`pending` → `paid` is set only by the payment event consumer** (not this endpoint).

Request:
```json
{ "status": "canceled" }
```

Valid status transitions via this endpoint:
- `pending` → `canceled`
- `paid` → `canceled`
- `in_transit` → `fulfilled`

Response `200`: updated order object. Errors: `400` invalid transition, `404` not found.

### POST /api/v1/orders/{id}/ship

Moves a **`paid`** order to **`in_transit`** and commits inventory reservations. Requires `order.ship`.

Response `200`: updated order object with `shipped_by`, `shipped_at`. Errors: `400` invalid state, `404` not found.

Order object shape:
```json
{
  "id": "ord-abc",
  "customer_id": "cust-123",
  "reservation_id": "res-xyz",
  "payment_id": "pay_000001",
  "items": [ { "sku": "SHOE-001", "quantity": 1, "unit_price_cents": 9900 } ],
  "status": "paid",
  "total_cents": 9900,
  "payment_due_at": "...",
  "paid_at": "...",
  "created_at": "...",
  "updated_at": "..."
}
```
