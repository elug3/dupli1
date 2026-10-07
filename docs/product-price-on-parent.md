# Product price: parent default, optional SKU override

**Status:** Implemented in `product`; order and cart pick it up through the variant `price` they already read ([#345](https://github.com/elug3/dupli1/issues/345)).  
**Related:** [frontend-product-variants-migration.md](frontend-product-variants-migration.md), [product-variants-plan.md](product-variants-plan.md).

The file keeps its name because other docs link to it.

## Rule

Pricing lives on the **parent product**. A variant/SKU **inherits** it unless it carries its own optional override:

| Field | Stored on | JSON | Meaning |
|-------|-----------|------|---------|
| Actual sale price (after discounts) | `products.price` | `price` | Charged / cart / order for every SKU without an override |
| Reference / list price | `products.official_price` | `officialPrice` | Display only (not charged) |
| SKU sale price override | `product_variants.price_won` (nullable) | `priceOverride` | Replaces the parent `price` for that SKU |
| SKU reference price override | `product_variants.official_price_won` (nullable) | `officialPriceOverride` | Replaces the parent `officialPrice` for that SKU |

`NULL` means inherit, so a SKU with no override behaves exactly as before. Overrides are whole won, non-negative; a fractional or negative value is rejected with `400` (`ports.Invalid`).

**Effective price.** On every variant read, `price` / `officialPrice` are the *effective* values — the override when set, otherwise the parent's. They are response-only; `priceOverride` / `officialPriceOverride` are what is stored. Cart, order, promotions and the storefront read `price` and need no knowledge of overrides.

Why: colourways of one style that sell at different prices (e.g. Prada `1BA906` ₩300k / `1BA906MNZV` ₩380k) used to need separate parent products. With an override they can be one product.

## API behavior

- `GET /products` and PDP return the parent `price` / `officialPrice`; the PDP's embedded variants carry their **effective** `price` / `officialPrice` plus any `priceOverride` / `officialPriceOverride`.
- Public variant lookups (`/variants/{sku}`, `/variants?sku_ids=`) return the effective price, so cart/order clients that read variant JSON keep working.
- **`priceFrom`** on a product (search cards, PDP) is the lowest effective price across **active** variants. It is present only when active variants differ in price; absent means one price, shown as `price`. (An older, unrelated `priceFrom` was removed earlier — see below; this is a new field with this meaning.)
- `sort=price` orders by the lowest effective active-SKU price (`priceFrom` when present, else `price`).
- Variant create and `PUT .../variants/{sku}` accept `priceOverride` / `officialPriceOverride`. Update merges: **omit** a field to leave the override unchanged, send **`0`** to clear it back to inheriting, send a positive whole-won amount to set it.
- `PUT /products/{id}` uses merge-on-update: omitted fields (including `price`) keep their current values.

## Downstream services

- **order** — `priceItems` resolves each line from the variant lookup, so each SKU is charged at its own effective price; client `unit_price_won` is still ignored. The order stores the per-line price, and refunds use the order total, so they follow.
- **cart** — line enrichment uses the same variant `price`.
- **promotions** — order sends each line's resolved `unit_price_won`; the evaluator reads sale state from the catalog as the SKU's effective prices (`line.on_sale` is true when the SKU's effective official price exceeds its effective price), not from the parent. Whole-order discount rules are unchanged.

## Partial updates

`PUT /products/{id}` merges non-empty / non-zero fields. Omitted JSON fields keep their current values. Setting the **parent** `price` or `officialPrice` to `0` is ignored (cannot clear a price via zero); send a positive amount to change them. SKU overrides are the exception: `0` clears them.

## Migration

Additive: `ALTER TABLE product_variants ADD COLUMN IF NOT EXISTS price_won / official_price_won NUMERIC(10,2)` on startup. No backfill; existing SKUs inherit.

The older parent-price migration still runs on startup:

1. Ensure `products.official_price` exists.
2. Backfill parent `price` from `MIN(active variant.price)` when parent `price` is still `0`.
3. Backfill parent `official_price` from `MAX(active variant.selling_price)` when official is still `0` (even if sale price was already set).
4. Copy legacy `products.selling_price` → `official_price` when official is still `0`.
5. Drop `products.selling_price` and the **legacy** `product_variants.price` / `selling_price` columns if present. (The new columns are named `price_won` / `official_price_won`, so this step never touches them.)

Removed legacy fields: `sellingPrice`, `sellingPriceFrom`.
