# Product price: parent default, optional SKU override

**Status:** Implemented in `product`; order and cart pick it up through the variant `price` they already read ([#345](https://github.com/elug3/dupli1/issues/345)).  
**Related:** [frontend-product-variants-migration.md](frontend-product-variants-migration.md), [product-variants-plan.md](product-variants-plan.md).

The file keeps its name because other docs link to it.

## Rule

Pricing lives on the **parent product**. A variant/SKU **inherits** it unless it carries its own optional override:

| Field | Stored on | JSON | Meaning |
|-------|-----------|------|---------|
| Actual sale price (after discounts) | `products.price_won` | `price` | Charged / cart / order for every SKU without an override |
| Reference / list price | `products.official_price_won` | `officialPrice` | Display only (not charged) |
| SKU sale price override | `product_variants.price_override_won` (nullable) | `priceOverride` | Replaces the parent `price` for that SKU |
| SKU reference price override | `product_variants.official_price_override_won` (nullable) | `officialPriceOverride` | Replaces the parent `officialPrice` for that SKU |

`NULL` means inherit, so a SKU with no override behaves exactly as before. Every price is whole won (`money.Won`, `BIGINT` columns) and non-negative. A fractional or negative value is rejected with `400 invalid request: <field> must be a non-negative whole won amount`, naming the field. That applies to the parent `price` / `officialPrice` and to overrides on SKUs created inline with a product, both of which went unchecked before 2026-11-04. A whole amount is accepted in any JSON number form (`1000`, `1000.0`, `1e3`).

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

**Whole-won `BIGINT` columns (2026-11-04, step 1 of [product-api-v2-plan.md](product-api-v2-plan.md)).** Prices used to be `NUMERIC(10,2)` read into `float64`: `products.price` / `official_price` and the SKU overrides `product_variants.price_won` / `official_price_won` (despite the name). They now live in `BIGINT` columns: `products.price_won` / `official_price_won` and `product_variants.price_override_won` / `official_price_override_won`. The change is additive (`migrateWonPrices` in `product/pkg/infra/pg/price_migrate.go`):

- The store reads only the `BIGINT` columns. While the `NUMERIC` ones exist it writes both, so a rollback to older code still finds every price current.
- Every start copies `NUMERIC` → `BIGINT` for any row older code wrote in the meantime. It rounds half away from zero, as `money.FromProductPrice` already did for cart and order, so a fractional legacy value reads as the amount that was being charged anyway.
- **Next release:** drop the four `NUMERIC` columns, the copy and the dual write. Also drop `official_price` and the variant `price_won` / `official_price_won` from `migrate()`'s `CREATE TABLE` and `ADD COLUMN` lists, so a fresh database never makes them again. A rollback across that drop still works: legacy mode needs `products.price`, which only `CREATE TABLE` creates, so this release then runs on the `BIGINT` columns alone.

The SKU override columns were first added on startup as `NUMERIC(10,2)` with no backfill; existing SKUs inherit.

The older parent-price migration still runs on startup:

1. Ensure `products.official_price` exists.
2. Backfill parent `price` from `MIN(active variant.price)` when parent `price` is still `0`.
3. Backfill parent `official_price` from `MAX(active variant.selling_price)` when official is still `0` (even if sale price was already set).
4. Copy legacy `products.selling_price` → `official_price` when official is still `0`.
5. Drop `products.selling_price` and the **legacy** `product_variants.price` / `selling_price` columns if present. (The override columns are named `*_won`, so this step never touches them.)

Removed legacy fields: `sellingPrice`, `sellingPriceFrom`.
