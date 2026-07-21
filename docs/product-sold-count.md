# Product sold count

**Status:** Implemented.  
**Related:** [payment-service.md](payment-service.md) (inventory plan B), [product-guest-views-plan.md](product-guest-views-plan.md) (`viewCount` sibling field), [api.md](api.md).

## Semantics

| Field | JSON | Meaning |
|-------|------|---------|
| `products.sold_count` | `soldCount` | Units of this **parent** style that have been **committed** from inventory |

**When it increments:** on inventory reservation **commit** (order `paid` → `in_transit` via `POST /api/v1/orders/{id}/ship`). Quantity is summed per parent via `product_variants.product_id`.

**When it does not change:**

- Reserve at checkout
- Payment / `order.paid`
- Release (cancel)
- Double-commit (idempotent `ErrReservationClosed`)

Same plan-B stock rules as [payment-service.md](payment-service.md): stock leaves the warehouse on ship commit, not on payment.

## Storage

```sql
ALTER TABLE products
    ADD COLUMN IF NOT EXISTS sold_count BIGINT NOT NULL DEFAULT 0;
```

Increment runs in the same Postgres transaction as stock decrement (`FinalizeReservation` when status = `committed`), joining `reservation_items` → `product_variants` → `products`.

## API

Public on `GET /api/v1/products/{id}` (and any parent JSON that selects `sold_count`), next to `viewCount`.

## Checklist

- [x] `sold_count` column + domain `SoldCount` / JSON `soldCount`
- [x] Increment on reservation commit (PG + in-memory)
- [x] Release / double-commit do not inflate the counter
- [x] OpenAPI `Product.soldCount`
