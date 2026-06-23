# API Endpoints

All services run on port `8080` internally. The nginx gateway (port `80`) proxies by path prefix with no stripping, so gateway paths are identical to service paths.

| Gateway prefix | Upstream service |
|---|---|
| `/api/v1/auth/` | `schick-auth:8080` |
| `/api/v1/products/` | `schick-product:8080` |
| `/api/v1/inventory/` | `schick-inventory:8080` |
| `/api/v1/orders` | `schick-order:8080` |

Each service also registers `/health` directly for internal/sidecar use.

---

## Auth Service

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/auth/health` | Health check |
| `POST` | `/api/v1/auth/register` | Create a new user account |
| `POST` | `/api/v1/auth/login` | Login and receive a refresh token |
| `POST` | `/api/v1/auth/logout` | Invalidate the current session |
| `POST` | `/api/v1/auth/refresh` | Exchange a refresh token for a new access token |
| `GET` | `/api/v1/auth/me` | Return the authenticated user's profile |

### GET /api/v1/auth/health

Response `200`: `ok` (plain text)

### POST /api/v1/auth/register

Request:
```json
{ "email": "user@example.com", "password": "minlen8" }
```

Response `201`:
```json
{ "user_id": "uuid" }
```

Errors: `400` bad request, `409` user already exists, `500` internal error.

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

Response `204` (no body). Token invalidation is a TODO in the current implementation.

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
{ "user_id": "uuid", "email": "user@example.com" }
```

Errors: `401` missing or invalid token, `404` user not found.

---

## Product Service

| Method | Path | Query params | Description |
|---|---|---|---|
| `GET` | `/api/v1/products/health` | — | Health check |
| `GET` | `/api/v1/products/all` | — | Return all products grouped by category |
| `GET` | `/api/v1/products/categories` | — | List all product categories |
| `GET` | `/api/v1/products/filters` | `category` (required) | List filter keys for a category |
| `GET` | `/api/v1/products/search` | `category` (required) + category filters | Search across any category |
| `GET` | `/api/v1/products/consultations` | `title`, `status` | Search consultations |
| `GET` | `/api/v1/products/shoes` | `brand`, `size`, `color`, `gender`, `material` | Search shoes |
| `GET` | `/api/v1/products/outerwear` | `brand`, `size`, `color`, `gender`, `material` | Search outerwear |
| `GET` | `/api/v1/products/bottoms` | `brand`, `size`, `color`, `gender`, `material` | Search bottoms |
| `GET` | `/api/v1/products/bags` | `brand`, `color`, `material` | Search bags |
| `GET` | `/api/v1/products/clocks` | `brand`, `type`, `material` | Search clocks |

Categories: `consultations`, `shoes`, `outerwear`, `bottoms`, `bags`, `clocks`.

### GET /api/v1/products/health

Response `200`:
```json
{ "status": "healthy" }
```

### GET /api/v1/products/all

Response `200`:
```json
{
  "total": 42,
  "results": {
    "consultations": [ /* consultation objects */ ],
    "shoes":         [ /* shoes objects */ ],
    "outerwear":     [ /* outerwear objects */ ],
    "bottoms":       [ /* bottoms objects */ ],
    "bags":          [ /* bag objects */ ],
    "clocks":        [ /* clock objects */ ]
  }
}
```

### GET /api/v1/products/categories

Response `200`:
```json
{ "categories": ["consultations", "shoes", "outerwear", "bottoms", "bags", "clocks"] }
```

### GET /api/v1/products/filters?category=shoes

Response `200`:
```json
{ "category": "shoes", "filters": ["brand", "size", "color", "gender", "material"] }
```

Errors: `400` missing category, `404` unknown category.

### GET /api/v1/products/search?category=shoes&brand=Nike

Response `200`:
```json
{ "total": 1, "results": [ /* category-specific objects */ ] }
```

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

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/orders/health` | Health check |
| `POST` | `/api/v1/orders` | Create a new order |
| `GET` | `/api/v1/orders?customer_id={id}` | List all orders for a customer |
| `GET` | `/api/v1/orders/{id}` | Get a single order |
| `PUT` | `/api/v1/orders/{id}/status` | Transition order status |

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

Request:
```json
{ "status": "confirmed" }
```

Valid status transitions:
- `pending` → `confirmed`
- `pending` → `canceled`
- `confirmed` → `fulfilled`

Response `200`: updated order object. Errors: `400` invalid transition, `404` not found.

Order object shape:
```json
{
  "id": "ord-abc",
  "customer_id": "cust-123",
  "reservation_id": "res-xyz",
  "items": [ { "sku": "SHOE-001", "quantity": 1, "unit_price_cents": 9900 } ],
  "status": "pending",
  "total_cents": 9900,
  "created_at": "...",
  "updated_at": "..."
}
```
