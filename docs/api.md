# Schick API Reference

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

Create a new user account. Requires `admin` or `user_manager` role.

**Headers** — `Authorization: Bearer <access_token>`

**Request body**
```json
{
  "email": "user@example.com",
  "password": "minlen8"
}
```

| Field | Type | Constraints |
|-------|------|-------------|
| `email` | string | required, valid email |
| `password` | string | required, min 8 chars |

**Response `201`**
```json
{ "user_id": "03f95d58-4840-46d4-9c92-fe48364d2e75" }
```

**Errors**
| Status | Meaning |
|--------|---------|
| `400` | Validation failed (bad email, password too short) |
| `401` | Missing or invalid access token |
| `403` | Caller does not have `admin` or `user_manager` role |
| `409` | Email already registered |

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

List all users. Requires `admin` role.

**Response `200`**
```json
{
  "users": [
    {
      "user_id": "03f95d58-4840-46d4-9c92-fe48364d2e75",
      "email": "admin@schick.com",
      "roles": ["admin"],
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
| `403` | Caller does not have `admin` role |

---

### `PATCH /api/v1/auth/users/{id}/roles`

Replace the role list for a user. Requires `admin` role.

**Request body**
```json
{ "roles": ["user_manager"] }
```

**Response `200`** — updated user object

**Errors**
| Status | Meaning |
|--------|---------|
| `400` | Missing or malformed body |
| `401` | Missing or invalid access token |
| `403` | Caller does not have `admin` role |
| `404` | User not found |

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

### `GET /api/v1/products/bags`

Search bags. No authentication required.

| Filter | Match type |
|--------|-----------|
| `brand` | case-insensitive substring |
| `color` | exact |
| `material` | exact |

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

Public product detail page (PDP). No authentication required. Returns only `status = active` products and omits `cost`.

**Response `200`** — product object

**Errors**
| Status | Meaning |
|--------|---------|
| `404` | Product not found or not active |

---

### Product CRUD (authenticated)

All routes below require `Authorization: Bearer <access_token>`. Product validates RS256 tokens via JWKS (`AUTH_JWKS_URL`).

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/products` | List all products |
| POST | `/api/v1/products` | Create product |
| GET | `/api/v1/products/{id}/manage` | Get product by ID (includes drafts and cost) |
| PUT | `/api/v1/products/{id}` | Update product |
| DELETE | `/api/v1/products/{id}` | Delete product |
| PUT | `/api/v1/products/{id}/image` | Upload image (multipart field `image`) |
| GET | `/api/v1/coupons` | List coupons |
| POST | `/api/v1/coupons` | Create coupon |
| PUT | `/api/v1/coupons/{code}` | Update coupon |
| DELETE | `/api/v1/coupons/{code}` | Delete coupon |

Product IDs are generated from the brand prefix (e.g. `BOT-001`). Image upload appends to the `imageUrls` array.

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

## Order Service — `/api/v1`

In-memory store. Calls inventory to reserve stock and product to redeem coupons.

When `JWT_SECRET` is set, order and checkout routes require `Authorization: Bearer <token>` (HMAC validator — see [current-state.md](current-state.md) for RS256 alignment). Customers may only access their own `customer_id`.

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
| POST | `/api/v1/auth/register` | `admin`, `user_manager` | auth |
| POST | `/api/v1/auth/login` | — | auth |
| GET | `/api/v1/auth/me` | Bearer | auth |
| POST | `/api/v1/auth/refresh` | — | auth |
| POST | `/api/v1/auth/logout` | — | auth |
| GET | `/api/v1/auth/users` | `admin` | auth |
| PATCH | `/api/v1/auth/users/{id}/roles` | `admin` | auth |
| PATCH | `/api/v1/auth/users/{id}/password` | `admin`, `user_manager` | auth |
| PATCH | `/api/v1/auth/users/{id}/status` | `admin`, `user_manager` | auth |
| GET | `/api/v1/products/health` | — | product |
| GET | `/api/v1/products/bags` | — | product |
| GET | `/api/v1/products/{id}` | — | product |
| POST | `/api/v1/coupons/redeem` | — | product |
| GET/POST | `/api/v1/products` | Bearer | product |
| GET | `/api/v1/products/{id}/manage` | Bearer | product |
| PUT/DELETE | `/api/v1/products/{id}` | Bearer | product |
| PUT | `/api/v1/products/{id}/image` | Bearer | product |
| GET/POST/PUT/DELETE | `/api/v1/coupons` | Bearer | product |
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

\* Bearer required when `JWT_SECRET` is configured on the order service.
