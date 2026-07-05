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

Admin routes require a token belonging to a user with the `owner` or `admin` role (or `user_manager` where noted).

### Account types

Every user has an `account_type` field (JSON key `account_type`) separate from RBAC `roles`:

| Value | Meaning | Typical roles |
|-------|---------|----------------|
| `customer` | End-user storefront account | `customer` |
| `admin` | Human operator (owner, admin staff) | `owner`, `admin`, `user_manager`, `product_manager`, … |
| `service` | Machine / integration account | `customer_registrar`, `order_manager`, … |

Seeded accounts: owner (`OWNER_EMAIL`) → `admin`; `dupli1-web` / `dupli1-order` service accounts → `service`. `POST /register` defaults to `customer` when `account_type` is omitted.

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

**Response `200`** (plain text)
```
ok
```

### `GET /api/v1/auth/.well-known/jwks.json`

RS256 public key set for verifying access tokens issued by auth.

---

### `POST /api/v1/auth/register`

Create a new user account. Requires `owner`, `admin`, `user_manager`, or `customer_registrar` role.

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
| `account_type` | string | optional; one of `customer`, `admin`, `service`; defaults to `customer`. `customer_registrar` may only create `customer` accounts |

**Response `201`**
```json
{ "user_id": "03f95d58-4840-46d4-9c92-fe48364d2e75" }
```

**Errors**
| Status | Meaning |
|--------|---------|
| `400` | Validation failed (bad email, password too short) |
| `401` | Missing or invalid access token |
| `403` | Caller lacks register role, or `customer_registrar` requested a non-customer `account_type` |
| `409` | Email already registered |
| `422` | Invalid email, weak password, or invalid `account_type` |

---

### Service account: dupli1-web

The `dupli1-web` BFF uses a seeded machine account with the `customer_registrar` role and `account_type` `service`. It can call `POST /api/v1/auth/register` to create customer accounts, but cannot manage passwords, roles, or user status.

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
  "roles": ["customer"],
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

List all users. Requires `owner` or `admin` role.

**Response `200`**
```json
{
  "users": [
    {
      "user_id": "03f95d58-4840-46d4-9c92-fe48364d2e75",
      "email": "admin@dupli1.com",
      "account_type": "admin",
      "roles": ["owner", "product_manager"],
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
| `403` | Caller does not have `owner` or `admin` role |

---

### `PATCH /api/v1/auth/users/{id}/roles`

Replace the role list for a user. Requires `owner` or `admin` role.

**Request body**
```json
{
  "roles": ["user_manager"],
  "account_type": "admin"
}
```

| Field | Type | Constraints |
|-------|------|-------------|
| `roles` | string[] | required |
| `account_type` | string | optional; one of `customer`, `admin`, `service` |

**Response `200`** — updated user object (includes `account_type`)

**Errors**
| Status | Meaning |
|--------|---------|
| `400` | Missing or malformed body |
| `401` | Missing or invalid access token |
| `403` | Caller does not have `owner` or `admin` role |
| `404` | User not found |
| `422` | Invalid `account_type` |

---

### `PATCH /api/v1/auth/users/{id}/password`

Set a new password for a user. Requires `admin` or `user_manager` role.

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
| `403` | Caller does not have `admin` or `user_manager` role |
| `404` | User not found |
| `422` | Password too short (min 8 chars) |

---

### `PATCH /api/v1/auth/users/{id}/status`

Activate or deactivate a user. Requires `admin` or `user_manager` role.

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
| `403` | Caller does not have `admin` or `user_manager` role |
| `404` | User not found |

---

## Product Service — `/api/v1/products`

### `GET /api/v1/products/health`

Product service liveness check.

**Response `200`**
```json
{ "status": "healthy" }
```

---

### `GET /api/v1/products`

Search **parent styles** (one row per style; colors are not duplicated). No authentication required for the public catalog view (active parents only; cost omitted). With a manager Bearer token, returns all statuses and includes cost.

| Filter | Match type |
|--------|-----------|
| `category` | exact (e.g. `bags`) |
| `brand` | case-insensitive substring |
| `color` | parent has an active variant with this color |
| `size` | parent has an active variant with this size |
| `material` | exact |
| `tags` | parent must include all listed tags (comma-separated or repeated) |
| `status` | exact (managers only) |

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

**Response `200`** — parent product object with variants

**Errors**
| Status | Meaning |
|--------|---------|
| `404` | Product not found or not active |

---

### Product CRUD (authenticated)

All routes below require `Authorization: Bearer <access_token>` from auth with role `product_manager`, `admin`, or `owner`. Product validates RS256 tokens via JWKS (`AUTH_JWKS_URL`).

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/products` | Create parent style |
| PUT | `/api/v1/products/{id}` | Update parent |
| DELETE | `/api/v1/products/{id}` | Delete parent (cascades variants) |
| POST | `/api/v1/products/{id}/images` | Upload image to default variant |
| POST | `/api/v1/products/{id}/variants` | Create variant (SKU) |
| PUT | `/api/v1/products/{id}/variants/{sku}` | Update variant |
| DELETE | `/api/v1/products/{id}/variants/{sku}` | Delete variant |
| POST | `/api/v1/products/{id}/variants/{sku}/images` | Upload image for variant |
| GET | `/api/v1/coupons` | List coupons |
| POST | `/api/v1/coupons` | Create coupon |
| PUT | `/api/v1/coupons/{code}` | Update coupon |
| DELETE | `/api/v1/coupons/{code}` | Delete coupon |

Parent IDs use the brand prefix (e.g. `BOT-001`). Variants are sellable SKUs (e.g. `BOT-001-GRN`). See [product-variants-plan.md](product-variants-plan.md).

---

## Inventory Service — `/api/v1/inventory`

In-memory store. No authentication today.

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
| GET | `/api/v1/carts/{customer_id}` | Get a customer's cart (`order_manager`, `admin`, or `owner`) |

### Product variant lookup (used by cart)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/variants/{sku}` | Public active variant by SKU |

---

## Order Service — `/api/v1`

In-memory store. Calls inventory to reserve stock and product to redeem coupons.

When `AUTH_JWKS_URL` or `JWT_SECRET` is set, order and checkout routes require `Authorization: Bearer <access_token>` (RS256 via auth JWKS when configured; HS256 fallback in dev). Customers may only access their own `customer_id`.

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
| PUT | `/api/v1/orders/{id}/status` | Confirm, cancel, or fulfill |

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

## Payment Service — `/api/v1/payments` (planned)

Stripe Checkout **redirect** — Dupli1 never handles card numbers, CVC, or card passwords.

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/payments` | Start Checkout for a pending order → `checkout_url` |
| GET | `/api/v1/payments/{id}` | Payment status |
| POST | `/api/v1/payments/webhooks/stripe` | Stripe webhooks |

Unpaid `pending` orders auto-cancel after **5 minutes**. Full design: [payment-service.md](payment-service.md).

---

## Notification Service

Health check only (`GET /health`). Outbound messaging is not implemented.

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

| Method | Path | Auth? | Service |
|--------|------|-------|---------|
| GET | `/gateway/health` | — | nginx |
| GET | `/api/v1/auth/health` | — | auth |
| GET | `/api/v1/auth/.well-known/jwks.json` | — | auth |
| POST | `/api/v1/auth/register` | `owner`, `admin`, `user_manager`, `customer_registrar` | auth |
| POST | `/api/v1/auth/login` | — | auth |
| GET | `/api/v1/auth/me` | Bearer | auth |
| POST | `/api/v1/auth/refresh` | — | auth |
| POST | `/api/v1/auth/logout` | — | auth |
| GET | `/api/v1/auth/users` | `admin` | auth |
| PATCH | `/api/v1/auth/users/{id}/roles` | `admin` | auth |
| PATCH | `/api/v1/auth/users/{id}/password` | `admin`, `user_manager` | auth |
| PATCH | `/api/v1/auth/users/{id}/status` | `admin`, `user_manager` | auth |
| GET | `/api/v1/products/health` | — | product |
| GET | `/api/v1/products` | optional manager Bearer | product |
| GET | `/api/v1/products/{id}` | — | product |
| POST | `/api/v1/coupons/redeem` | — | product |
| POST | `/api/v1/products` | `product_manager`, `admin`, `owner` | product |
| PUT/DELETE | `/api/v1/products/{id}` | `product_manager`, `admin`, `owner` | product |
| POST | `/api/v1/products/{id}/images` | `product_manager`, `admin`, `owner` | product |
| POST | `/api/v1/products/{id}/variants` | `product_manager`, `admin`, `owner` | product |
| PUT/DELETE | `/api/v1/products/{id}/variants/{sku}` | `product_manager`, `admin`, `owner` | product |
| POST | `/api/v1/products/{id}/variants/{sku}/images` | `product_manager`, `admin`, `owner` | product |
| GET/POST/PUT/DELETE | `/api/v1/coupons` | `product_manager`, `admin`, `owner` | product |
| GET | `/api/v1/inventory/health` | — | inventory |
| GET/PUT | `/api/v1/inventory/{sku}` | — | inventory |
| POST | `/api/v1/inventory/{sku}/adjust` | — | inventory |
| POST | `/api/v1/inventory/reservations` | — | inventory |
| POST | `/api/v1/inventory/reservations/{id}/commit` | — | inventory |
| POST | `/api/v1/inventory/reservations/{id}/release` | — | inventory |
| POST/GET | `/api/v1/checkout/sessions` | Bearer* | order |
| GET/PUT/POST/DELETE | `/api/v1/checkout/sessions/{id}/...` | Bearer* | order |
| POST/GET | `/api/v1/orders` | Bearer* | order |
| GET | `/api/v1/orders/{id}` | Bearer* | order |
| PUT | `/api/v1/orders/{id}/status` | Bearer* | order |
| GET | `/health` | — | notification |

\* Bearer required when `AUTH_JWKS_URL` or `JWT_SECRET` is configured on the order service.
