# Cart Service

The **cart service** (`dupli1-cart`) stores a persistent shopping cart per authenticated customer. It holds **what the customer intends to buy** (variant SKU/SkuID + quantity). It does **not** reserve stock, apply coupons, or create orders — those remain in **order** and **product** (stock and reservations were merged into the product service; see [product-variants-plan.md](product-variants-plan.md)).

For the short-lived checkout pipeline (coupon, TTL, inventory reservation), see [checkout-session.md](checkout-session.md).

## Role in the purchase flow

```mermaid
flowchart LR
    subgraph shopping["Shopping (persistent)"]
        Cart["dupli1-cart"]
        Product["dupli1-product<br/>(catalog + stock)"]
    end

    subgraph checkout["Checkout (short-lived)"]
        Order["dupli1-order"]
    end

    Cart -->|"validate price"| Product
    Cart -->|"available_qty hint"| Product
    Cart -.->|"client copies items"| Order
    Order -->|"POST .../complete: reserve stock"| Product
    Order -.->|"pending order"| Pay["dupli1-payment"]
```

| Phase | Service | Lifetime | Stock impact |
|-------|---------|----------|--------------|
| Browse / save cart | `dupli1-cart` | Until cleared or checked out | None |
| Checkout session | `dupli1-order` | 30 minutes (default) | Reserved on `complete` |
| Payment | `dupli1-payment` | 5 minutes to pay | Reserved until ship or cancel |
| Order | `dupli1-order` | Permanent | Committed after payment |

---

## Service boundaries

### What cart owns

- One cart per customer (`customer_id` = JWT `sub`)
- Line items: **variant SKU** + **quantity**
- CRUD at `/api/v1/cart` (current user)
- Admin read at `/api/v1/carts/{customer_id}`

### What cart does **not** own

| Concern | Owner | Notes |
|---------|-------|-------|
| Stock levels | `dupli1-product` | Cart may **read** availability; never writes. Merged in from the former standalone inventory service |
| Stock reservations | `dupli1-product` | Triggered by order on checkout `complete` |
| Coupons / discounts | `dupli1-product` + `dupli1-order` | Applied in checkout session |
| Orders | `dupli1-order` | Created when checkout completes |
| Catalog / prices | `dupli1-product` | Cart validates SKU and resolves price on read |

There is **no conflict** with inventory: cart stores **intent**; product stores **truth**. Overselling at checkout time is handled when order calls `POST /api/v1/inventory/reservations` (served by product).

---

## Architecture decision: split vs merge

### Current design: **split** (separate `cart/` service)

```text
cart/       → persistent cart
order/      → checkout sessions + orders
product/    → catalog + stock + reservations (inventory merged in; see product-variants-plan.md)
```

**Why split**

- Different lifecycles (persistent cart vs 30‑min checkout vs committed order)
- Clear bounded contexts (intent vs commitment vs catalog/warehouse)
- Cart can evolve (guest cart, merge on login) without bloating order

**Costs**

- Extra service, Postgres instance, and deploy target
- Client (or a future `POST /api/v1/cart/checkout`) orchestrates cart → checkout handoff

### Alternative: merge cart into **order**

Move cart tables and routes into `dupli1-order` so one “commerce” service owns cart → checkout → order.

**When to consider**

- Small team and ops simplicity matter more than strict boundaries
- You want server-side checkout handoff without a separate cart container

**Not recommended:** merge cart into **product**. Product (including stock) is SKU-centric catalog/inventory truth; cart is customer-centric shopping intent. This mirrors the reasoning that kept inventory itself SKU-centric even after it merged into product.

### Alternative: stay split + checkout handoff API (future)

Keep services separate but add `POST /api/v1/cart/checkout` in cart to create a checkout session in order internally. Reduces client round trips without merging codebases.

---

## Authentication

All cart routes except health and settings require a Bearer **access** token (same flow as order):

1. `POST /api/v1/auth/login` → `refresh_token`
2. `POST /api/v1/auth/refresh` → `token` (access JWT)
3. Send `Authorization: Bearer <token>` on cart requests

The cart owner is always **`sub` from the JWT** — clients do not send `customer_id` on `/api/v1/cart` mutations.

| Route | Identity |
|-------|----------|
| `/api/v1/cart` | `claims.UserID` from token |
| `/api/v1/carts/{customer_id}` | Path id; requires `cart.read` |

Guest / anonymous carts are **not implemented** yet (planned phase 2). Identity should reuse the shared `dupli1_guest` browser cookie defined in [product-guest-views-plan.md](product-guest-views-plan.md) (minted on PDP in phase 1; cart mints if absent later).

---

## Data model

### Persisted (PostgreSQL)

**SKU/SkuID + quantity** are stored. **Not** `product_id`, price, or images.

```sql
CREATE TABLE carts (
    customer_id TEXT PRIMARY KEY,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE cart_items (
    customer_id TEXT NOT NULL REFERENCES carts(customer_id) ON DELETE CASCADE,
    sku         TEXT NOT NULL,
    sku_id      TEXT NOT NULL,
    quantity    INTEGER NOT NULL CHECK (quantity > 0),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (customer_id, sku_id)
);
```

Sellable unit is the **variant SKU**, identified by a human-readable `sku` (e.g. `BOT-001-BLK`) and a canonical ULID `sku_id`, aligned with [product variants](product-variants-plan.md), inventory, and checkout. Cart resolves whichever identifier the caller sends against product and always persists both — writes self-heal toward a full pair over time.

### Enriched on read (not stored)

On `GET /api/v1/cart` and after mutations, cart calls product for both variant info and stock:

| Source | Endpoint | Fields added |
|--------|----------|--------------|
| Product | `GET /api/v1/products/variants?sku_ids=` (preferred) or `/api/v1/products/variants/by-sku/{sku}` / `by-sku-id/{skuId}` | `product_id`, `unit_price_cents`, `color`, `image_url` |
| Product (inventory) | `GET /api/v1/products/inventory/items/{sku}` or `.../items/by-sku-id/{skuId}` | `available_qty` (optional) |

Prices are **server-sourced** from product — clients send only `{ "sku" or "sku_id", "quantity" }` on add. Catalog `price` is **KRW won**; cart maps it 1:1 into `unit_price_cents` (whole won — do not ×100).

---

## API

Base path: `/api/v1/cart` on `dupli1-cart` (port **8086** locally, gateway **8080**).

### `GET /api/v1/cart`

Return the authenticated user's cart.

**Response `200`**
```json
{
  "customer_id": "03f95d58-4840-46d4-9c92-fe48364d2e75",
  "items": [
    {
      "sku": "BOT-001-BLK",
      "sku_id": "01JAY6Z9K3F8QW1G7H2T5X0ABC",
      "product_id": "BOT-001",
      "quantity": 1,
      "unit_price_cents": 125000,
      "color": "Black",
      "image_url": "https://...",
      "available_qty": 3
    }
  ],
  "subtotal_cents": 125000,
  "updated_at": "2026-07-05T12:00:00Z"
}
```

---

### `POST /api/v1/cart/items`

Add or update one line (upsert by `sku` or `sku_id`; either identifier resolves to the same variant).

**Request**
```json
{ "sku": "BOT-001-BLK", "quantity": 2 }
```
or
```json
{ "sku_id": "01JAY6Z9K3F8QW1G7H2T5X0ABC", "quantity": 2 }
```

**Response `200`** — updated cart (same shape as `GET`).

---

### `PUT /api/v1/cart/items`

Replace all line items.

**Request**
```json
{
  "items": [
    { "sku": "BOT-001-BLK", "quantity": 2 },
    { "sku": "BOT-001-GRN", "quantity": 1 }
  ]
}
```

---

### `DELETE /api/v1/cart/items/{sku}`

Remove one line by human SKU. `DELETE /api/v1/cart/items/by-sku-id/{skuId}` removes by canonical `sku_id` instead.

---

### `DELETE /api/v1/cart`

Clear the cart. **Response `204`**.

---

### `GET /api/v1/carts/{customer_id}`

Admin read-only view of a customer's cart. Requires elevated role.

---

## Checkout handoff (client-driven)

Cart does not create orders. The storefront copies cart lines into a checkout session:

```mermaid
sequenceDiagram
    participant Client
    participant Cart as dupli1-cart
    participant Order as dupli1-order
    participant Pay as dupli1-payment
    participant Product as dupli1-product

    Client->>Cart: GET /api/v1/cart
    Cart-->>Client: items + unit_price_cents

    Client->>Order: POST /api/v1/checkout/sessions
    Client->>Order: PUT /api/v1/checkout/sessions/{id}/items
    Note right of Client: use sku, quantity, unit_price_cents from cart

    Client->>Order: POST .../coupon (optional)
    Client->>Order: POST .../complete
    Order->>Product: reserve stock (/api/v1/inventory/reservations)
    Order-->>Client: pending order

    Client->>Pay: POST /api/v1/payments → redirect Stripe
    Note over Client,Pay: see payment-service.md

    Client->>Cart: DELETE /api/v1/cart
```

See [checkout-session.md](checkout-session.md) for checkout session details. See [payment-service.md](payment-service.md) for payment and order confirmation.

---

## Stock semantics

- **Add to cart** does not decrement inventory.
- **`available_qty`** on the cart response is informational (from inventory read).
- If stock drops before checkout, **`complete`** may fail when order reserves — same as checkout without a persistent cart.
- Optional future: reject add when `quantity > available_qty`, or flag stale lines on `GET`.

---

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `DUPLI1_CART_ADDR` | `:8086` | Listen address |
| `DUPLI1_CART_DB` | — | Postgres URL (`cart` database); omit for in-memory (tests) |
| `DUPLI1_PRODUCT_URL` | `http://localhost:8081` | Variant lookup |
| `DUPLI1_INVENTORY_URL` | `http://localhost:8081` | Stock hints (optional; product service — stock/reservations were merged in) |
| `AUTH_JWKS_URL` | — | RS256 JWT validation (Compose: auth JWKS) |
| `JWT_SECRET` | — | HS256 dev fallback |

Local Postgres: `postgres://dupli1:dupli1_dev@localhost:5436/cart?sslmode=disable`

---

## Package layout

| Path | Role |
|------|------|
| `cart/pkg/domain/cart.go` | Cart entity, validation |
| `cart/pkg/service/service.go` | Use cases, enrichment |
| `cart/pkg/ports/` | Repository, product, inventory interfaces |
| `cart/pkg/infra/pg/` | Postgres persistence |
| `cart/pkg/infra/httpproduct/` | Product variant client |
| `cart/pkg/infra/httpinventory/` | Inventory read client |
| `cart/pkg/handler/http.go` | HTTP routes |
| `shared/pkg/authjwt` | JWT validation (shared with order/cart/payment/product) |

---

## Errors

| Status | Condition |
|--------|-----------|
| `400` | Invalid SKU, quantity, or empty customer |
| `401` | Missing or invalid token |
| `403` | Customer calling admin route |
| `404` | Unknown or inactive variant |
| `503` | Product service unavailable |

---

## Related documentation

- [checkout-session.md](checkout-session.md) — checkout and order creation
- [product-variants-plan.md](product-variants-plan.md) — SKU model
- [endpoints.md](endpoints.md) — route index
- [service-layout.md](service-layout.md) — repo layout

## Planned enhancements

| Item | Status |
|------|--------|
| Guest cart + merge on login | Not started — reuse `dupli1_guest` from [product-guest-views-plan.md](product-guest-views-plan.md) |
| `POST /api/v1/cart/checkout` | Not started |
| Stock enforcement on add | Not started |
| CI / ECS deploy for cart | Partial (Compose only) |
