# Product SKU Naming System

Luxury-fashion-inspired SKU format for Dupli1. Human SKUs are deterministic, stable after creation, and map each segment to a normalized master entity. The canonical cross-service identifier remains the ULID `skuId` on `product_variants`; the human `sku` is the ERP-friendly display / ops key.

## Format

```text
<BrandCode>_<StyleCode>_<ColorCode>_<VariantCode>_<SizeCode>
```

Segments are separated by `_`. **VariantCode** (construction / edition) is optional; when omitted the SKU has four segments:

```text
<BrandCode>_<StyleCode>_<ColorCode>_<SizeCode>
```

### Examples

| Product | SKU |
|---------|-----|
| Prada style `1BA457`, color `F0032`, edition `V`, size `YO0` | `PR_1BA457_F0032_V_YO0` |
| Bottega Cassette, black, edition `V`, medium | `BOT_CAS001_BLK_V_MED` |
| Same style, black, no edition, medium | `BOT_CAS001_BLK_MED` |

## Segments

| # | Segment | Master table | Rules |
|---|---------|--------------|-------|
| 1 | BrandCode | `brands` | 2–3 uppercase letters; unique; assigned once; never changes |
| 2 | StyleCode | `products.style_code` (scoped by brand) | Unique within brand; identifies the design family; never changes after creation |
| 3 | ColorCode | `colors` | One code per official color; reused across products; stable |
| 4 | VariantCode | `sku_editions` | Optional; construction / edition only (not size or color) |
| 5 | SizeCode | `sizes` | Physical size or capacity only |

### What NOT to encode

Product name, marketing name, season/collection, price, inventory quantity, supplier, warehouse location — these live in the database, not the SKU.

## Entity model

```text
Brand
 └── Product (Style)     brand_code + style_code

product_variants (sellable SKU row)
 ├── brand_code   → brands.code
 ├── style_code   → products.style_code (via product_id)
 ├── color_code   → colors.code
 ├── edition_code → sku_editions.code (optional)
 └── size_code   → sizes.code
```

| Table | Columns |
|-------|---------|
| `brands` | `id`, `code` (UNIQUE), `name` |
| `colors` | `id`, `code` (UNIQUE), `name` |
| `sizes` | `id`, `code` (UNIQUE), `name` |
| `sku_editions` | `id`, `code` (UNIQUE), `name` — construction/edition (SKU “VariantCode”); named to avoid clash with `product_variants` |
| `products` | existing fields + `brand_code`, `style_code`; `UNIQUE (brand_code, style_code)` |
| `product_variants` | existing fields + `color_code`, `edition_code`, `size_code` |

`sku_editions` is the master for the optional **VariantCode** segment. `product_variants` remains the sellable row (inventory/order key via `skuId` / human `sku`).

## Design principles

- **Stable** — human `sku` and codes never change after creation
- **Deterministic** — same attribute combination always yields the same SKU
- **Human-readable** — ops can parse brand / style / color / edition / size at a glance
- **Normalized** — each segment maps to exactly one master entity
- **Scalable** — new brands, colors, sizes = new master rows; format unchanged
- **Dual identity** — `skuId` (ULID) for integrations; human `sku` for ERP / warehouse / reporting

## Validation

- Brand code must exist in `brands`
- Style must belong to that brand (`products.brand_code` + `style_code`)
- Color code must exist in `colors`
- Size code must exist in `sizes`
- Edition / VariantCode is optional; when set must exist in `sku_editions`
- Duplicate human SKUs are prohibited (`UNIQUE` on `product_variants.sku`)
- Same codes always compose to the same SKU string

## Generation (product service)

On variant create, when the parent has `brand_code` + `style_code` and the variant resolves `color_code` + `size_code`:

1. Resolve / require color and size codes (by explicit code or name lookup against master tables).
2. Optionally resolve edition code.
3. Compose `BuildSKU(brand, style, color, edition, size)`.
4. Persist codes on the variant row; set human `sku`; assign new `skuId` if blank.

Legacy parents without `brand_code`/`style_code` keep the previous `{productId}-{color}-{size}` helper until backfilled. Existing human SKUs are **not** rewritten (stability).

## Seeded master data

Common luxury brand codes (`PR`, `BOT`, `LV`, `CD`, `CH`, `GUC`, …), standard color codes (`BLK`, `WHT`, `CRM`, `BRN`, `GRN`, …), size codes (`XS`–`XL`, `MIN`/`SML`/`MED`/`LRG`), and a default edition `V` (standard construction).

## API surface

- Product JSON includes `brandCode`, `styleCode` (in addition to display `brand` name).
- Variant JSON includes `colorCode`, `editionCode`, `sizeCode` (in addition to display `color` / `size` names).
- Explicit `sku` on create still allowed (admin override); when omitted, the composed SKU is used.
- Master-data seed runs on product DB migrate.
- **Runtime code→name CRUD** (create / rename / delete-when-unused), dedicated `styles` master, and FKs: see [product-sku-master-data-plan.md](product-sku-master-data-plan.md).

## References

- Implementation: `product/pkg/domain/sku.go`, `product/pkg/domain/sku_master.go`, migrate in `product/pkg/infra/pg/product_store.go`
- Master-data CRUD plan: [product-sku-master-data-plan.md](product-sku-master-data-plan.md)
- Parent + variants plan: [product-variants-plan.md](product-variants-plan.md)
- Canonical `skuId`: `product/pkg/domain/skuid.go`
