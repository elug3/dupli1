# Checkout Session

Checkout sessions provide a multi-step purchase flow inside the **order service** (`dupli1-order`). A client builds a cart-like session, optionally applies a coupon, then completes checkout to create a pending order with inventory reserved.

For a **persistent** shopping cart (saved across sessions), use the **cart service** first — see [cart-service.md](cart-service.md).

For **payment** after checkout, see [payment-service.md](payment-service.md) (Stripe Checkout redirect; 5-minute unpaid window).

Direct order creation (`POST /api/v1/orders`) remains available for callers that already have a finalized cart.

## Flow

```mermaid
sequenceDiagram
    participant Client
    participant Order as dupli1-order
    participant Product as dupli1-product

    Client->>Order: POST /api/v1/checkout/sessions
    Order-->>Client: open session (expires in 30 min)

    Client->>Order: POST /api/v1/checkout/sessions/{id}/items
    Order-->>Client: session with subtotal

    Client->>Order: POST /api/v1/checkout/sessions/{id}/coupon
    Order->>Product: POST /api/v1/coupons/redeem
    Product-->>Order: discount fraction
    Order-->>Client: session with discount + total

    Client->>Order: POST /api/v1/checkout/sessions/{id}/complete
    Order->>Product: reserve stock (/api/v1/inventory/reservations)
    Order-->>Client: completed session + pending order
```

Stock and reservations are owned by the product service (merged in from the
former standalone inventory service). Order calls them through the **internal
API gateway** (`DUPLI1_GATEWAY_URL`) so paths stay `/api/v1/inventory/...`
and `/api/v1/coupons/...`. Deprecated: `DUPLI1_PRODUCT_URL` / `DUPLI1_INVENTORY_URL`
as direct product overrides.

## Session states

| Status | Meaning |
|--------|---------|
| `open` | Session accepts item and coupon changes |
| `completed` | Checkout finished; `order_id` is set |
| `expired` | `expires_at` passed; session is read-only |

Default TTL is **30 minutes** (`domain.DefaultCheckoutTTL`).

## API

Base path: `/api/v1/checkout/sessions` on `dupli1-order` (port **8083** locally).

### `POST /api/v1/checkout/sessions`

Create an empty checkout session.

**Request**
```json
{ "customer_id": "03f95d58-4840-46d4-9c92-fe48364d2e75" }
```

**Response `201`**
```json
{
  "id": "cs_000001",
  "customer_id": "03f95d58-4840-46d4-9c92-fe48364d2e75",
  "items": [],
  "status": "open",
  "subtotal_cents": 0,
  "discount_cents": 0,
  "total_cents": 0,
  "expires_at": "2026-06-21T12:30:00Z",
  "created_at": "2026-06-21T12:00:00Z",
  "updated_at": "2026-06-21T12:00:00Z"
}
```

---

### `GET /api/v1/checkout/sessions/{id}`

Fetch the current session. Expired open sessions are marked `expired` on read.

**Response `200`** — session object (same shape as above, with items when present).

---

### `PUT /api/v1/checkout/sessions/{id}/items`

Replace all line items. Server resolves prices from product (client `unit_price_cents` is ignored).

**Request**
```json
{
  "items": [
    { "sku": "bag-1", "quantity": 2 }
  ]
}
```

**Response `200`** — updated session with recalculated `subtotal_cents` and `total_cents` (catalog prices).

---

### `POST /api/v1/checkout/sessions/{id}/items`

Add or update a single line item (matched by `sku`). Server resolves price from product.

**Request**
```json
{ "sku": "bag-1", "quantity": 1 }
```

**Response `200`** — updated session.

---

### `DELETE /api/v1/checkout/sessions/{id}/items/{sku}`

Remove a line item.

**Response `200`** — updated session.

---

### `POST /api/v1/checkout/sessions/{id}/coupon`

Apply a coupon by redeeming it from the product service.

**Request**
```json
{ "code": "SUMMER30" }
```

**Response `200`** — session with `coupon_code`, `discount_cents`, and `total_cents` updated.

Requires `DUPLI1_PRODUCT_URL` to be configured. Returns `503` when the coupon client is unavailable.

---

### `POST /api/v1/checkout/sessions/{id}/complete`

Finalize checkout: reserve inventory, create a `pending` order, and mark the session `completed`.

**Response `200`**
```json
{
  "session": {
    "id": "cs_000001",
    "status": "completed",
    "order_id": "ord_000001",
    "coupon_code": "SUMMER30",
    "subtotal_cents": 10000,
    "discount_cents": 3000,
    "total_cents": 7000
  },
  "order": {
    "id": "ord_000001",
    "customer_id": "03f95d58-4840-46d4-9c92-fe48364d2e75",
    "status": "pending",
    "coupon_code": "SUMMER30",
    "subtotal_cents": 10000,
    "discount_cents": 3000,
    "total_cents": 7000,
    "items": [
      { "sku": "BAG-1", "quantity": 2, "unit_price_cents": 5000 }
    ]
  }
}
```

After completion, the order is **`pending`** with inventory reserved. The customer must pay within **5 minutes** via the payment service (Stripe Checkout redirect); see [payment-service.md](payment-service.md). Only `dupli1-payment` confirms the order after a successful webhook — not manual status updates.

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `DUPLI1_ORDER_ADDR` | `:8083` | Listen address |
| `DUPLI1_GATEWAY_URL` | `http://localhost:8080` | Internal nginx gateway (stock + coupons) |
| `DUPLI1_AUTH_URL` | — | Auth base for service-account login (prefer direct; gateway OK after proxy is up) |
| `DUPLI1_PRODUCT_URL` | — | **Deprecated** direct product override |
| `DUPLI1_INVENTORY_URL` | — | **Deprecated** alias for product override |

## Errors

| Status | Condition |
|--------|-----------|
| `400` | Invalid input, empty checkout, expired session, invalid coupon |
| `404` | Session not found |
| `503` | Coupon service not configured |
| `500` | Inventory or persistence failure |

## Package layout

| Path | Role |
|------|------|
| `order/pkg/domain/checkout_session.go` | Session entity and totals logic |
| `order/pkg/service/checkout.go` | Use cases |
| `order/pkg/ports/repository.go` | Order and checkout session persistence |
| `order/pkg/ports/coupon.go` | Coupon redemption port |
| `order/pkg/infra/httpcoupon/` | Product service HTTP adapter |
| `order/pkg/handler/checkout.go` | HTTP routes |

Order and checkout routes require `Authorization: Bearer <access_token>` when `AUTH_JWKS_URL` or `JWT_SECRET` is configured (RS256 via auth JWKS when set).
