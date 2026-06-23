# API Endpoints

All requests go through the nginx gateway at `http://localhost:80`.  
The gateway prefix (`/auth/`, `/product/`, `/inventory/`, `/order/`) is stripped before forwarding.

---

## Auth

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/auth/health` | Health check |
| `POST` | `/auth/api/v1/auth/register` | Register a new user |
| `POST` | `/auth/api/v1/auth/login` | Login and receive a refresh token |
| `POST` | `/auth/api/v1/auth/logout` | Logout (invalidate session) |
| `POST` | `/auth/api/v1/auth/refresh` | Exchange refresh token for access token |

### POST /auth/api/v1/auth/register
```json
// Request
{ "email": "user@example.com", "password": "min8chars" }

// 201 Created
{ "user_id": "abc123" }
```

### POST /auth/api/v1/auth/login
```json
// Request
{ "email": "user@example.com", "password": "secret" }

// 200 OK
{ "refresh_token": "<token>" }
```

### POST /auth/api/v1/auth/refresh
```json
// Request
{ "refresh_token": "<token>" }

// 200 OK
{ "token": "<access_token>" }
```

---

## Product

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/product/api/health` | Health check |
| `GET` | `/product/api/categories` | List all categories |
| `GET` | `/product/api/filters?category=<name>` | List filters for a category |
| `GET` | `/product/api/products/search?category=<name>[&<filter>=<value>]` | Search across categories |
| `GET` | `/product/api/products/consultations` | Search consultations |
| `GET` | `/product/api/products/shoes` | Search shoes |
| `GET` | `/product/api/products/outerwear` | Search outerwear |
| `GET` | `/product/api/products/bottoms` | Search bottoms |
| `GET` | `/product/api/products/bags` | Search bags |
| `GET` | `/product/api/products/clocks` | Search clocks |

**Categories**: `consultations`, `shoes`, `outerwear`, `bottoms`, `bags`, `clocks`

**Filters per category**:

| Category | Filters |
|----------|---------|
| `consultations` | `title`, `status` |
| `shoes` | `brand`, `size`, `color`, `gender`, `material` |
| `outerwear` | `brand`, `size`, `color`, `gender`, `material` |
| `bottoms` | `brand`, `size`, `color`, `gender`, `material` |
| `bags` | `brand`, `color`, `material` |
| `clocks` | `brand`, `type`, `material` |

```json
// GET /product/api/categories — 200 OK
{ "categories": ["consultations", "shoes", "outerwear", "bottoms", "bags", "clocks"] }

// GET /product/api/filters?category=shoes — 200 OK
{ "category": "shoes", "filters": ["brand", "size", "color", "gender", "material"] }

// GET /product/api/products/shoes?brand=Nike&size=42 — 200 OK
{ "total": 3, "results": [...] }
```

---

## Inventory

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/inventory/health` | Health check |
| `GET` | `/inventory/api/v1/inventory/{sku}` | Get stock for a SKU |
| `PUT` | `/inventory/api/v1/inventory/{sku}` | Set stock quantity for a SKU |
| `POST` | `/inventory/api/v1/inventory/{sku}/adjust` | Adjust stock by delta |
| `POST` | `/inventory/api/v1/inventory/reservations` | Create a reservation |
| `POST` | `/inventory/api/v1/inventory/reservations/{id}/commit` | Commit a reservation |
| `POST` | `/inventory/api/v1/inventory/reservations/{id}/release` | Release a reservation |

### PUT /inventory/api/v1/inventory/{sku}
```json
// Request
{ "quantity": 100 }

// 200 OK — inventory item
```

### POST /inventory/api/v1/inventory/{sku}/adjust
```json
// Request
{ "delta": -5 }

// 200 OK — inventory item
```

### POST /inventory/api/v1/inventory/reservations
```json
// Request
{
  "order_id": "order-123",
  "items": [
    { "sku": "SHOE-42-BLK", "quantity": 2 }
  ]
}

// 201 Created
{ "reservation_id": "res-456", "reservation": { ... } }
```

---

## Order

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/order/health` | Health check |
| `POST` | `/order/api/v1/orders` | Create an order |
| `GET` | `/order/api/v1/orders?customer_id=<id>` | List orders for a customer |
| `GET` | `/order/api/v1/orders/{id}` | Get an order by ID |
| `PUT` | `/order/api/v1/orders/{id}/status` | Update order status |

### POST /order/api/v1/orders
```json
// Request
{
  "customer_id": "cust-789",
  "items": [
    { "sku": "SHOE-42-BLK", "quantity": 1 }
  ]
}

// 201 Created — order object
```

### PUT /order/api/v1/orders/{id}/status

Valid statuses: `confirmed`, `canceled`, `fulfilled`

```json
// Request
{ "status": "confirmed" }

// 200 OK — order object
```

### GET /order/api/v1/orders?customer_id=cust-789
```json
// 200 OK
{ "total": 2, "orders": [...] }
```
