# Schick API Reference

All traffic is routed through the nginx gateway. In production use `https://your-domain`. Locally use `https://localhost` (self-signed cert — pass `-k` to curl or add the cert to your trust store).

HTTP requests are automatically redirected to HTTPS.

---

## Authentication

Protected routes require an `Authorization` header with a Bearer access token obtained from the login or refresh endpoints.

```
Authorization: Bearer <access_token>
```

Access tokens are short-lived (default 15 min). Use the refresh endpoint to issue new ones without re-authenticating.

---

## Gateway

### `GET /gateway/health`

Nginx liveness check — responds without touching any backend service.

**Response `200`**
```
ok
```

---

## Auth Service — `/api/v1/auth`

### `GET /health`

Auth service liveness check.

**Response `200`**
```
ok
```

---

### `POST /api/v1/auth/register`

Create a new user account.

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
| `409` | Email already registered |

---

### `POST /api/v1/auth/login`

Authenticate and receive token pair.

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
  "access_token":  "<jwt>",
  "refresh_token": "<jwt>"
}
```

**Errors**
| Status | Meaning |
|--------|---------|
| `400` | Missing or malformed body |
| `401` | Invalid credentials |

---

### `GET /api/v1/auth/me`

Return the currently authenticated user's profile.

**Headers** — `Authorization: Bearer <access_token>`

**Response `200`**
```json
{
  "id":    "03f95d58-4840-46d4-9c92-fe48364d2e75",
  "email": "user@example.com"
}
```

**Errors**
| Status | Meaning |
|--------|---------|
| `401` | Missing, malformed, or expired access token |
| `404` | User no longer exists |

---

### `POST /api/v1/auth/refresh`

Exchange a refresh token for a new token pair. Invalidates the supplied refresh token.

**Request body**
```json
{ "refresh_token": "<jwt>" }
```

**Response `200`**
```json
{
  "access_token":  "<jwt>",
  "refresh_token": "<jwt>"
}
```

**Errors**
| Status | Meaning |
|--------|---------|
| `400` | Missing or malformed body |
| `401` | Refresh token invalid or expired |

---

### `POST /api/v1/auth/logout`

Invalidate the session associated with a refresh token. The access token remains valid until it expires naturally.

**Request body**
```json
{ "refresh_token": "<jwt>" }
```

**Response `204`** — no body

**Errors**
| Status | Meaning |
|--------|---------|
| `400` | Missing or malformed body |
| `401` | Refresh token invalid |

---

## Product Service — `/api`

All product endpoints are read-only (GET). No authentication required.

### `GET /api/health`

Product service liveness check.

**Response `200`**
```json
{ "status": "healthy" }
```

---

### `GET /api/categories`

List all available product categories.

**Response `200`**
```json
{
  "categories": ["consultations", "shoes", "outerwear", "bottoms", "bags", "clocks"]
}
```

---

### `GET /api/filters`

List the filterable fields for a given category.

**Query params**
| Param | Required | Description |
|-------|----------|-------------|
| `category` | yes | One of the values returned by `/api/categories` |

**Response `200`**
```json
{
  "category": "shoes",
  "filters":  ["brand", "size", "color", "gender", "material"]
}
```

**Errors**
| Status | Meaning |
|--------|---------|
| `400` | `category` param missing |
| `404` | Unknown category |

---

### `GET /api/products/search`

Generic search across any category.

**Query params**
| Param | Required | Description |
|-------|----------|-------------|
| `category` | yes | Category to search within |
| *(filter fields)* | no | See per-category filter tables below |

**Response `200`**
```json
{
  "total":   2,
  "results": [ /* category-specific objects */ ]
}
```

**Errors**
| Status | Meaning |
|--------|---------|
| `400` | `category` param missing |
| `500` | Unknown category value |

---

### `GET /api/products/consultations`

| Filter | Match type |
|--------|-----------|
| `title` | case-insensitive substring |
| `status` | exact |

**Result object**
```json
{
  "ID": "1", "Title": "Style Consultation", "Description": "...",
  "Duration": 60, "Price": 75.00, "Status": "available"
}
```

---

### `GET /api/products/shoes`

| Filter | Match type |
|--------|-----------|
| `brand` | case-insensitive substring |
| `size` | exact |
| `color` | exact |
| `gender` | exact |
| `material` | exact |

**Result object**
```json
{
  "ID": "1", "Name": "Air Max 90", "Description": "...",
  "Price": 120.00, "Brand": "Nike", "Color": "White",
  "Material": "Leather", "Stock": 50, "Category": "shoes",
  "Size": "42", "Gender": "Unisex"
}
```

---

### `GET /api/products/outerwear`

| Filter | Match type |
|--------|-----------|
| `brand` | case-insensitive substring |
| `size` | exact |
| `color` | exact |
| `gender` | exact |
| `material` | exact |

**Result object** — same fields as shoes, with `Size` and `Gender`.

---

### `GET /api/products/bottoms`

| Filter | Match type |
|--------|-----------|
| `brand` | case-insensitive substring |
| `size` | exact |
| `color` | exact |
| `gender` | exact |
| `material` | exact |

**Result object** — same fields as shoes, with `Size` and `Gender`.

---

### `GET /api/products/bags`

| Filter | Match type |
|--------|-----------|
| `brand` | case-insensitive substring |
| `color` | exact |
| `material` | exact |

**Result object**
```json
{
  "ID": "1", "Name": "Tote Pro", "Description": "...",
  "Price": 45.00, "Brand": "Generic", "Color": "Beige",
  "Material": "Canvas", "Capacity": "15L", "Stock": 100, "Category": "bags"
}
```

---

### `GET /api/products/clocks`

| Filter | Match type |
|--------|-----------|
| `brand` | case-insensitive substring |
| `type` | exact (`Analog`, `Digital`, `Smart`) |
| `material` | exact |

**Result object**
```json
{
  "ID": "1", "Name": "Seiko 5", "Description": "...",
  "Price": 150.00, "Brand": "Seiko",
  "Type": "Analog", "Material": "Steel",
  "Stock": 20, "Category": "clocks"
}
```

---

## Common error shape

All error responses use the same JSON envelope:

**Auth service** (Gin)
```json
{ "error": "human-readable message" }
```

**Product service** (stdlib)
```json
{ "error": "human-readable message", "code": 400 }
```

---

## Quick reference

| Method | Path | Auth? | Service |
|--------|------|-------|---------|
| GET | `/gateway/health` | — | nginx |
| GET | `/health` | — | auth |
| POST | `/api/v1/auth/register` | — | auth |
| POST | `/api/v1/auth/login` | — | auth |
| GET | `/api/v1/auth/me` | Bearer | auth |
| POST | `/api/v1/auth/refresh` | — | auth |
| POST | `/api/v1/auth/logout` | — | auth |
| GET | `/api/health` | — | product |
| GET | `/api/categories` | — | product |
| GET | `/api/filters` | — | product |
| GET | `/api/products/search` | — | product |
| GET | `/api/products/consultations` | — | product |
| GET | `/api/products/shoes` | — | product |
| GET | `/api/products/outerwear` | — | product |
| GET | `/api/products/bottoms` | — | product |
| GET | `/api/products/bags` | — | product |
| GET | `/api/products/clocks` | — | product |
