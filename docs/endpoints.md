# API Endpoints

All services listen on port `8080` inside Docker. The nginx gateway proxies by path prefix with no stripping, so gateway paths match service paths.

| Gateway prefix | Upstream service |
|---|---|
| `/gateway/health` | nginx (static) |
| `/api/v1/auth/` | `dupli1-auth:8080` |
| `/api/v1/products` | `dupli1-product:8080` |
| `/api/v1/coupons` | `dupli1-product:8080` |
| `/api/v1/inventory/` | `dupli1-inventory:8080` |
| `/api/v1/orders` | `dupli1-order:8080` |
| `/api/v1/checkout` | `dupli1-order:8080` |
| `/api/v1/cart` | `dupli1-cart:8080` |
| `/api/v1/carts/` | `dupli1-cart:8080` |
| `/api/v1/payments` | `dupli1-payment:8080` |
| `/api/v1/payments/` | `dupli1-payment:8080` |

Local gateway: `http://localhost:8080` (also host port 80).

Each service also registers `/health` directly for internal/sidecar use.

---

## Auth Service

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/api/v1/auth/health` | — | Health check |
| `POST` | `/api/v1/auth/register` | `owner`, `admin`, `user_manager`, `customer_registrar` | Create a new user account |
| `POST` | `/api/v1/auth/login` | — | Login and receive a refresh token |
| `POST` | `/api/v1/auth/logout` | — | Invalidate the current session |
| `POST` | `/api/v1/auth/refresh` | — | Exchange a refresh token for a new access token |
| `GET` | `/api/v1/auth/me` | Bearer | Return the authenticated user's profile |
| `GET` | `/api/v1/auth/users` | `owner`, `admin` | List all users |
| `PATCH` | `/api/v1/auth/users/:id/roles` | `owner`, `admin` | Replace a user's roles (optional `account_type`) |
| `PATCH` | `/api/v1/auth/users/:id/password` | `admin`, `user_manager` | Set a new password for a user |
| `PATCH` | `/api/v1/auth/users/:id/status` | `admin`, `user_manager` | Activate or deactivate a user |

**dupli1-web service account:** set `DUPLI1_WEB_SERVICE_EMAIL` and `DUPLI1_WEB_SERVICE_PASSWORD` on `dupli1-auth` to seed a machine user with `customer_registrar` role and `account_type` `service`. That role may register customers only (`account_type` `customer`).

**Account types:** `customer`, `admin`, `service` — returned on user objects as `account_type`. Distinct from RBAC `roles`.

### GET /api/v1/auth/health

Response `200`: `ok` (plain text)

### POST /api/v1/auth/register

Header: `Authorization: Bearer <access_token>` (must have `owner`, `admin`, `user_manager`, or `customer_registrar` role)

Request:
```json
{
  "email": "user@example.com",
  "password": "minlen8",
  "account_type": "customer"
}
```

`account_type` is optional (`customer`, `admin`, or `service`); defaults to `customer`. `customer_registrar` may only use `customer`.

Response `201`:
```json
{ "user_id": "uuid" }
```

Errors: `400` bad request, `401` missing/invalid token, `403` insufficient role, `409` user already exists, `422` invalid email/password/account_type, `500` internal error.

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
  "roles": ["customer"],
  "is_active": true,
  "locked_at": null,
  "failed_login_attempts": 0
}
```

Errors: `401` missing or invalid token, `404` user not found.

### GET /api/v1/auth/users

Header: `Authorization: Bearer <access_token>` (must have `owner` or `admin` role)

Response `200`:
```json
{
  "users": [
    {
      "user_id": "uuid",
      "email": "user@example.com",
      "account_type": "customer",
      "roles": ["customer"],
      "is_active": true,
      "locked_at": null,
      "failed_login_attempts": 0
    }
  ]
}
```

Errors: `401` missing or invalid token, `403` caller lacks `owner` or `admin` role, `500` internal error.

### PATCH /api/v1/auth/users/:id/roles

Header: `Authorization: Bearer <access_token>` (must have `owner` or `admin` role)

Request:
```json
{
  "roles": ["admin", "user_manager"],
  "account_type": "admin"
}
```

Response `200`: user object (same shape as list item).

Errors: `400` bad request, `401` missing/invalid token, `403` insufficient role, `404` user not found, `422` invalid account_type, `500` internal error.

### PATCH /api/v1/auth/users/:id/password

Header: `Authorization: Bearer <access_token>` (must have `admin` or `user_manager` role)

Request:
```json
{ "password": "newpassword" }
```

Response `204` (no body).

Errors: `400` bad request, `401` missing/invalid token, `403` insufficient role, `404` user not found, `422` password too short, `500` internal error.

### PATCH /api/v1/auth/users/:id/status

Header: `Authorization: Bearer <access_token>` (must have `admin` or `user_manager` role)

Request:
```json
{ "is_active": false }
```

Response `200`: user object (same shape as list item).

Errors: `400` bad request, `401` missing/invalid token, `403` insufficient role, `404` user not found, `500` internal error.

---

## Product Service

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/api/v1/products/health` | — | Health check |
| `GET` | `/api/v1/products` | optional | Search **parent styles** only (no color duplicates); managers see drafts/cost |
| `GET` | `/api/v1/products/{id}` | — | Parent PDP with `variants[]`, `availableColors`, `availableSizes` |
| `POST` | `/api/v1/coupons/redeem` | — | Redeem a coupon code |
| `POST` | `/api/v1/products` | `product_manager`, `admin`, `owner` | Create parent (optional legacy color/price seeds default variant) |
| `PUT` | `/api/v1/products/{id}` | `product_manager`, `admin`, `owner` | Update parent |
| `DELETE` | `/api/v1/products/{id}` | `product_manager`, `admin`, `owner` | Delete parent (cascades variants) |
| `POST` | `/api/v1/products/{id}/images` | `product_manager`, `admin`, `owner` | Upload image to default variant |
| `POST` | `/api/v1/products/{id}/variants` | `product_manager`, `admin`, `owner` | Create variant (SKU) |
| `PUT` | `/api/v1/products/{id}/variants/{sku}` | `product_manager`, `admin`, `owner` | Update variant |
| `DELETE` | `/api/v1/products/{id}/variants/{sku}` | `product_manager`, `admin`, `owner` | Delete variant |
| `POST` | `/api/v1/products/{id}/variants/{sku}/images` | `product_manager`, `admin`, `owner` | Upload image for variant |
| `GET` | `/api/v1/coupons` | `product_manager`, `admin`, `owner` | List coupons |
| `POST` | `/api/v1/coupons` | `product_manager`, `admin`, `owner` | Create coupon |
| `PUT` | `/api/v1/coupons/{code}` | `product_manager`, `admin`, `owner` | Update coupon |
| `DELETE` | `/api/v1/coupons/{code}` | `product_manager`, `admin`, `owner` | Delete coupon |

Public search defaults to `status = active` on the **parent**. Query filters: `category`, `brand`, `material`, `tags`, `color`, `size` (color/size match any active variant). Managers may also pass `status`. Checkout uses **variant SKU** with inventory. See [product-variants-plan.md](product-variants-plan.md).

### GET /api/v1/products/health

Response `200`:
```json
{ "status": "healthy" }
```

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
| `GET` | `/api/v1/cart` | Bearer | Get current user's cart |
| `DELETE` | `/api/v1/cart` | Bearer | Clear current user's cart |
| `PUT` | `/api/v1/cart/items` | Bearer | Replace all cart items |
| `POST` | `/api/v1/cart/items` | Bearer | Add or update one item |
| `DELETE` | `/api/v1/cart/items/{sku}` | Bearer | Remove one item |
| `GET` | `/api/v1/carts/{customer_id}` | Bearer (admin) | Get a customer's cart |

See [cart-service.md](cart-service.md) for architecture, boundaries with inventory/order, and checkout handoff.

---

## Payment Service

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/api/v1/payments/health` | — | Health check |
| `POST` | `/api/v1/payments` | Bearer | Start Stripe Checkout for a pending order |
| `GET` | `/api/v1/payments/{id}` | Bearer | Payment status |
| `POST` | `/api/v1/payments/webhooks/stripe` | Stripe signature | Webhook handler |
| `GET` | `/api/v1/payments/{id}/simulate-success` | — | Dev only (no Stripe key): mark payment succeeded |

See [payment-service.md](payment-service.md) for Stripe redirect flow, 5-minute auto-cancel, and `payment.succeeded` → `paid`.

---

## Inventory Service

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/inventory/health` | Health check |
| `GET` | `/api/v1/inventory/{sku}` | Get a stock item by SKU |
| `PUT` | `/api/v1/inventory/{sku}` | Create or overwrite stock quantity for a SKU |
| `POST` | `/api/v1/inventory/{sku}/adjust` | Add or subtract stock quantity (delta) |
| `POST` | `/api/v1/inventory/reservations` | Create a reservation |
| `POST` | `/api/v1/inventory/reservations/{id}/commit` | Commit a reservation (deducts stock) |
| `POST` | `/api/v1/inventory/reservations/{id}/release` | Release a reservation (restores stock) |

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

Requires `Authorization: Bearer <access_token>` when `AUTH_JWKS_URL` or `JWT_SECRET` is set on the order service (RS256 via auth JWKS in Compose). Customers may only use their own `customer_id`. See [checkout-session.md](checkout-session.md).

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/api/v1/orders/health` | — | Health check |
| `POST` | `/api/v1/checkout/sessions` | Bearer* | Create checkout session |
| `GET` | `/api/v1/checkout/sessions/{id}` | Bearer* | Get session |
| `PUT` | `/api/v1/checkout/sessions/{id}/items` | Bearer* | Replace all items |
| `POST` | `/api/v1/checkout/sessions/{id}/items` | Bearer* | Add or update one item |
| `DELETE` | `/api/v1/checkout/sessions/{id}/items/{sku}` | Bearer* | Remove item |
| `POST` | `/api/v1/checkout/sessions/{id}/coupon` | Bearer* | Apply coupon |
| `POST` | `/api/v1/checkout/sessions/{id}/complete` | Bearer* | Complete checkout |
| `POST` | `/api/v1/orders` | Bearer* | Create a new order |
| `GET` | `/api/v1/orders?customer_id={id}` | Bearer* | List all orders for a customer |
| `GET` | `/api/v1/orders/{id}` | Bearer* | Get a single order |
| `POST` | `/api/v1/orders/{id}/ship` | Bearer* (`order_manager`, `admin`, `owner`) | Ship order (`paid` → `in_transit`, commit stock) |
| `PUT` | `/api/v1/orders/{id}/status` | Bearer* | Transition order status (`canceled`, `fulfilled`) |

\* Required when `AUTH_JWKS_URL` or `JWT_SECRET` is configured; optional in tests with no validator.

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

`order_manager` / `admin` / `owner` may cancel or fulfill. **`pending` → `paid` is set only by the payment event consumer** (not this endpoint).

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

Moves a **`paid`** order to **`in_transit`** and commits inventory reservations. Requires `order_manager`, `admin`, or `owner`.

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
