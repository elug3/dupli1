# Product SKU Identity & Master Data

Authoritative reference for how Dupli1 identifies sellable units (`skuId` / human `sku`) and the **master data** dictionaries those human SKUs are built from.

Related: [product-sku-master-data-plan.md](product-sku-master-data-plan.md) (phase plan), [product-variants-plan.md](product-variants-plan.md) (parent + variants).

---

## Identifiers

### Parent product `id`

| Field | JSON | Type | Role | Mutable? |
|-------|------|------|------|----------|
| **id** | `id` | ULID string (new) | Opaque parent / PDP key | Never after create |

```text
id  =  01JAY6Z9K3F8QW1G7H2T5X0ABC     ← domain.NewProductID()
```

Human design identity is **not** encoded in `id`. Use `brandCode` + `styleCode` (and master names) for ops. Legacy rows may still use brand-prefixed ids (e.g. `BOT-001`); they remain valid.

### Dual identity (variants)

Every sellable variant (`product_variants` row) has **two** identifiers:

| Field | JSON | Type | Role | Mutable? |
|-------|------|------|------|----------|
| **skuId** | `skuId` | ULID string | Canonical cross-service key (inventory, cart, order, reservations) | Never after create |
| **sku** | `sku` | Human string | ERP / warehouse / ops display key | Never after create |

```text
skuId  =  01JAY6Z9K3F8QW1G7H2T5X0ABC     ← integrations, FKs, stock_items
sku    =  BOT_CAS001_BLK_V_MED            ← composed from master codes
```

**Rule:** Prefer `skuId` for machine-to-machine calls. Accept either on lookup APIs (`/variants/{sku}`, `/variants/by-sku-id/{skuId}`, inventory siblings). Renaming a master **name** never changes `sku` or `skuId`.

### Why both?

- **skuId** is opaque and stable even if human naming conventions evolve; it is the primary key of `product_variants` and is what stock/cart/order store.
- **sku** is deterministic and readable: ops can see brand, style, color, edition, and size without joining tables.

Generation: `domain.NewProductID()` / `domain.NewSkuID()` (ULID) on create; human `sku` from `BuildSKU` / `ComposeVariantSKU` when parent brand/style codes and variant color/size codes are present.

---

## Human SKU format

```text
<BrandCode>_<StyleCode>_<ColorCode>_<VariantCode>_<SizeCode>
```

`VariantCode` (edition) is optional. Without it:

```text
<BrandCode>_<StyleCode>_<ColorCode>_<SizeCode>
```

| Example | Meaning |
|---------|---------|
| `BOT_CAS001_BLK_V_MED` | Bottega, style CAS001, black, edition V, medium |
| `BOT_CAS001_BLK_MED` | Same without edition |
| `PR_1BA457_F0032_V_YO0` | Prada-style numeric color/size codes |

**Do not encode** in the SKU: product marketing name, season, price, stock qty, supplier, warehouse.

Legacy rows may still use `{productId}-{color}` (e.g. `BOT-001-GRN`); they keep working. New variants under coded parents use the underscore format.

---

## Master data (code → name)

**Master data** = shared reference dictionaries. Each SKU segment is a **code**; each code has a display **name**. Codes are immutable; names can be renamed without rewriting SKUs.

```text
brands
  └── styles          (design family under a brand — SKU styleCode)

colors                (global palette)
sizes                 (apparel / capacity)
sku_editions          (optional construction / VariantCode)

# Product attribute masters (not human-SKU segments)
subcategories         (bag type: handbags, tote, …)
occasions             (look/style: casual, evening, …)
targets               (audience: men, women, kids)
```

| Master | Table | Code rules | Example |
|--------|-------|------------|---------|
| Brand | `brands` | 2–3 uppercase letters | `BOT` → Bottega Veneta |
| Style | `styles` | Unique within brand; alphanumeric | `CAS001` → Cassette |
| Color | `colors` | Alphanumeric, reused across products | `BLK` → Black |
| Size | `sizes` | Alphanumeric | `MED` → Medium |
| Edition | `sku_editions` | Optional VariantCode segment | `V` → Standard |
| Subcategory | `subcategories` | Bag type on parent product | `HBG` → Handbags |
| Occasion | `occasions` | Look/style on parent (not SKU `styleCode`) | `CAS` → Casual |
| Target | `targets` | Audience on parent product | `WMN` → Women |

### Code vs name

| | Code | Name |
|---|------|------|
| Used in human `sku` | Yes | No |
| Change after create | **Never** | Allowed (PATCH) |
| Delete | Only when unused (409 + DB `RESTRICT`) | — |

### How masters relate to catalog / SKUs

```text
brands.code
  └── styles (brand_code, code, name)
        └── products.brand_code + products.style_code   (catalog parent / PDP)
              ├── subcategory_code → subcategories.code  (nullable)
              ├── occasion_code    → occasions.code      (nullable)
              ├── target_code      → targets.code        (nullable)
              └── product_variants
                    ├── sku_id  (ULID, PK)
                    ├── sku     (human, UNIQUE)
                    ├── color_code   → colors.code
                    ├── size_code    → sizes.code
                    └── edition_code → sku_editions.code  (nullable)
```

- **Style** = design identity (master). **Product** = catalog shell that uses a style (marketing copy, category, tags).
- Inventory / cart / order key off **`sku_id`**, not master codes.

---

## Catalog APIs (runtime CRUD)

Base: `/api/v1/catalog/...` (gateway → product). Auth required.

| Resource | Paths | Permissions |
|----------|-------|-------------|
| Brands | `GET/POST /catalog/brands`, `PATCH/DELETE /catalog/brands/{code}` | `product.master.read` / `product.master.write` |
| Styles | `…/brands/{code}/styles`, `…/styles/{styleCode}` | same |
| Colors | `/catalog/colors` | same |
| Sizes | `/catalog/sizes` | same |
| Editions | `/catalog/editions` | same |
| Subcategories | `/catalog/subcategories` (bag type) | same |
| Occasions | `/catalog/occasions` (look/style: casual, evening, …) | same |
| Targets | `/catalog/targets` (men / women / kids) | same |

- **POST** body: `{ "code", "name" }`
- **PATCH** body: `{ "name" }` only (code immutable)
- **DELETE**: `204` if unused; `409` if referenced by styles, products, or variants

Covered by `product.*` and the `catalog_editor` bundle. See [endpoints.md](endpoints.md), [permissions.md](permissions.md).

### Typical ops flow

1. `POST /catalog/brands` → `BOT` / Bottega Veneta  
2. `POST /catalog/brands/BOT/styles` → `CAS001` / Cassette  
3. Ensure color/size/edition (seeded or create)  
4. `POST /products` with `brandCode` + `styleCode` (required; masters must already exist) → ULID `id`  
5. Create variant with color/size/(edition) codes that exist → human `sku` composed; new `skuId` assigned  

### Phase C — enforce on writes

| Create | Required | Missing codes | Unknown code |
|--------|----------|---------------|--------------|
| Product | Existing `brandCode` + `styleCode` | `400` (`ErrMissingSKUCodes`) | `404` (`ErrMasterNotFound`) |
| Variant | Existing `colorCode` + `sizeCode` (+ `editionCode` if set); parent must have brand/style | `400` | `404` |

No silent invent-on-create. Reads enrich blank `brand` / `color` / `size` display names from master tables (`enrichMasterNames`).

---

## API fields (product / variant JSON)

**Product (parent):** `id` (ULID), `brand`, `brandCode`, `styleCode`, …  
**Variant:** `skuId`, `sku`, `color`, `size`, `colorCode`, `editionCode`, `sizeCode`, …

Lookup:

- `GET /api/v1/products/variants?sku_ids=` (batch by canonical ULID; max 50)
- `GET /api/v1/products/variants/by-sku/{sku}` (legacy: `/api/v1/variants/{sku}`)
- `GET /api/v1/products/variants/by-sku-id/{skuId}` (legacy: `/api/v1/variants/by-sku-id/{skuId}`)
- Inventory: `/api/v1/products/inventory/items/{sku}` (legacy: `/api/v1/inventory/{sku}`)

---

## Seeded defaults

On product DB migrate, common rows are seeded (idempotent): brands (`PR`, `BOT`, `LV`, `CD`, `CH`, `GUC`, …), colors (`BLK`, `WHT`, `GRN`, …), sizes (`XS`–`XL`, `MIN`/`SML`/`MED`/`LRG`, `OS`), editions (`V`, `A`, `R`), bag subcategories (`HBG`, `TOT`, `SHD`, `CRS`, `MNI`), occasions (`CAS`, `EVE`, `BUS`, `WKD`, `STM`), targets (`MEN`, `WMN`, `KID`). Runtime catalog APIs are the source of truth afterward.

---

## Implementation map

| Concern | Location |
|---------|----------|
| ULID `id` / `skuId` | `product/pkg/domain/skuid.go` (`NewProductID`, `NewSkuID`) |
| Build / parse / require codes | `product/pkg/domain/sku.go`, `sku_compose.go` |
| Master entity types + seeds | `product/pkg/domain/sku_master.go` |
| Migrate masters, styles, FKs | `product/pkg/infra/pg/sku_master.go` |
| Require masters + enrich names | `product/pkg/infra/pg/master_require.go` |
| Catalog store / HTTP | `product/pkg/infra/pg/catalog_store.go`, `handler/catalog_handler.go` |
| Inventory keyed by `sku_id` | `product/pkg/infra/pg/inventory_store.go`, `service/inventory_service.go` |
