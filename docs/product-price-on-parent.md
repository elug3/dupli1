# Product price on parent

**Status:** Implemented.  
**Related:** [frontend-product-variants-migration.md](frontend-product-variants-migration.md), [product-variants-plan.md](product-variants-plan.md).

## Rule

Pricing lives on the **parent product**, not on each variant/SKU:

| Field | Stored on | JSON | Meaning |
|-------|-----------|------|---------|
| Actual sale price (after discounts) | `products.price` | `price` | Charged / cart / order |
| Reference / list price | `products.official_price` | `officialPrice` | Display only (not charged) |

Variants keep color, size, images, and status. Variant create/update **ignores** any price fields in the body.

Removed legacy fields: `sellingPrice`, `sellingPriceFrom`, `priceFrom`.

## API behavior

- `GET /products` and PDP return parent `price` / `officialPrice`.
- Public variant lookups still include `price` / `officialPrice` **copied from the parent** so cart/order clients that read variant JSON keep working.
- `sort=price` orders by `products.price`.
- `PUT /products/{id}` uses merge-on-update: omitted fields (including `price`) keep their current values.

## Migration

On product-service startup:

1. Ensure `products.official_price` exists.
2. Backfill parent `price` / `official_price` from cheapest active variant when parent `price` is still `0`.
3. Copy legacy `selling_price` → `official_price` when official is still `0`.
4. Zero variant price columns (columns kept for schema compat; app no longer reads/writes them).
