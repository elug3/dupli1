# Product price on parent (not SKU)

**Status:** Implemented.  
**Related:** [frontend-product-variants-migration.md](frontend-product-variants-migration.md), [product-variants-plan.md](product-variants-plan.md).

## Rule

Sale price lives on the **parent product**, not on each variant/SKU:

| Field | Stored on | JSON |
|-------|-----------|------|
| Sale price | `products.price` | `price` |
| Display / “was” price | `products.selling_price` | `sellingPrice` |
| List-card aliases | derived | `priceFrom` / `sellingPriceFrom` (= parent price) |

Variants keep color, size, images, and status. Variant create/update **ignores** any `price` / `sellingPrice` in the body.

## API behavior

- `GET /products` and PDP return parent `price` / `sellingPrice`.
- Public variant lookups still include `price` / `sellingPrice` **copied from the parent** so cart/order clients that read variant JSON keep working without a separate product fetch.
- `sort=price` orders by `products.price`.

## Migration

On product-service startup, if a parent still has `price = 0` and an active variant has a non-zero price, the parent is backfilled from the cheapest active variant, then variant price columns are zeroed (columns remain for schema compat; the app no longer reads/writes them).
