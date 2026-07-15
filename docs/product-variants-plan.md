# Plan: Parent Product + Variants

**Status:** Phases 1–3 implemented in product service (schema, backfill, read/write APIs). Phases 4–5 below (stock integration, cleanup) were superseded by a larger follow-up: every variant now has a canonical ULID `SkuID` (in addition to the human `sku`), and the standalone inventory service was merged into product outright (stock/reservations now live in product's own database, not read via an external HTTP client). See `product/pkg/domain/skuid.go`, `product/pkg/service/inventory_service.go`, and [current-state.md](current-state.md) for the implemented shape.

**Frontend clients:** see [frontend-product-variants-migration.md](frontend-product-variants-migration.md) (`dupli1-web`, `dupli1-manage-web`).

Goal: customers see **one catalog entry per style** (no duplicates), while color, size, images, and stock live on **sellable variants (SKUs)**.

## Problem

Today each product row is a flat sellable unit (`color`, `stock`, `imageUrls` on the same record). Offering the same bag in Green and Black either:

- duplicates search results (two products), or
- cannot support per-color stock, images, and checkout.

Customers need:

- One result / PDP per style
- Available colors and sizes
- Images by color
- Stock by color and size (options)

Order and inventory already key on **SKU**. Product must introduce a **parent (style) + variant (SKU)** split.

## Target model

```text
Product (parent / style)          Variant (SKU)
─────────────────────────         ─────────────────────────────
id: BOT-001                       sku: BOT-001-GRN
name, description, brand          product_id: BOT-001
material, category, tags          color: Green
status                            size: "" (optional)
                                  price, image_urls, status
                                         │
                                         ▼
                                  Inventory item (sku = variant.sku)
                                  Order / checkout line (sku)
```

| Layer | Role | Customer-facing? |
|-------|------|------------------|
| **Parent** | Catalog card and PDP shell | Search, list, PDP URL |
| **Variant** | Color / size option, images, price | Picker on PDP; cart uses `sku` |
| **Inventory** | Quantity and reservations | Via SKU (unchanged API) |
| **Order** | Line items | Via SKU (unchanged API) |

**ID rules**

- Keep parent ids as today: brand prefix + sequence (`BOT-001`).
- Parent also stores immutable `brandCode` + `styleCode` used to compose human SKUs.
- Variant human SKUs follow [product-sku-system.md](product-sku-system.md): `Brand_Style_Color[_Edition]_Size` (e.g. `BOT_CAS001_BLK_V_MED`). Legacy `{parentId}-{color}` SKUs remain for rows created before codes existed.
- Canonical cross-service id is ULID `skuId`; do not encode options only in the parent id.

## Data model (product DB)

### `products` (parent) — slim down over time

Keep shared catalog fields:

- `id`, `name`, `description`, `brand`, `material`, `category`, `status`, `tags`, `capacity`, `created_at`
- `cost` stays on parent or moves to variant later (start: parent-level cost for admin)

Deprecate as source of truth (migrate off):

- `color` — move to variant
- `stock` — inventory only
- `image_urls` / `image_url` — move to variant (parent may keep a denormalized `default_image_url` for list cards)

### `product_variants` (new)

| Column | Type | Notes |
|--------|------|--------|
| `sku` | `TEXT PRIMARY KEY` | Sellable id; inventory/order key |
| `product_id` | `TEXT NOT NULL` | FK → `products.id` |
| `color` | `TEXT NOT NULL DEFAULT ''` | |
| `size` | `TEXT NOT NULL DEFAULT ''` | Empty for bags without size |
| `price` | `NUMERIC(10,2) NOT NULL` | |
| `status` | `TEXT NOT NULL DEFAULT 'active'` | `active` \| `draft` \| `archived` |
| `image_urls` | `TEXT[] NOT NULL DEFAULT '{}'` | Images for this option |
| `created_at` | `TIMESTAMPTZ` | |

Indexes: `(product_id)`, `(product_id, color, size)` unique if we want one row per option combo.

Optional later: generic `options JSONB` if more than color/size is needed. Start with explicit `color` + `size`.

## API design

### Public

| Method | Path | Behavior |
|--------|------|----------|
| `GET` | `/api/v1/products` | **Parents only.** No duplicate styles. Query: `category`, `brand`, `material`, `tags`, and filters that match *any* variant (`color`, `size`). Response includes summary: `availableColors`, `availableSizes`, `defaultImageUrl`, `priceFrom`. |
| `GET` | `/api/v1/products/{id}` | Parent + embedded `variants[]`. Derive `availableColors` / `availableSizes` from active variants. Optionally attach `inStock` per variant (product → inventory, or client fetches inventory). |

Example PDP:

```json
{
  "id": "BOT-001",
  "name": "Cassette Bag",
  "brand": "Bottega Veneta",
  "category": "bags",
  "availableColors": ["Green", "Black"],
  "availableSizes": [],
  "variants": [
    {
      "sku": "BOT-001-GRN",
      "color": "Green",
      "size": "",
      "price": 2500,
      "imageUrls": ["https://…/green-1.jpg"],
      "status": "active",
      "inStock": true
    },
    {
      "sku": "BOT-001-BLK",
      "color": "Black",
      "size": "",
      "price": 2500,
      "imageUrls": ["https://…/black-1.jpg"],
      "status": "active",
      "inStock": false
    }
  ]
}
```

### Admin (permissions: `product.create`, `product.update`, `product.variant.*`, `product.image.upload`, or wildcards `product.*` / `*`)

| Method | Path | Behavior |
|--------|------|----------|
| `POST` | `/api/v1/products` | Create parent (no color/stock required). |
| `PUT` / `DELETE` | `/api/v1/products/{id}` | Update / delete parent (delete policy: reject if variants exist, or cascade). |
| `GET` | `/api/v1/products` | Managers: include drafts; optional `status` filter (existing optional auth). |
| `POST` | `/api/v1/products/{id}/variants` | Create variant (`sku` optional → auto from parent + color/size codes). |
| `PUT` / `DELETE` | `/api/v1/products/{id}/variants/{sku}` | Update / delete variant. |
| `POST` | `/api/v1/products/{id}/variants/{sku}/images` | Upload image; append to that variant’s `imageUrls`. |

Deprecate:

- `POST /api/v1/products/{id}/images` on the parent (or redirect to default variant during transition).

### Unchanged

- Inventory: `GET/PUT /api/v1/inventory/{sku}`, reservations by SKU.
- Order / checkout: line items use `sku` = variant SKU.
- Coupons: unchanged.

## Search behavior (no duplicates)

1. Query parents (`products`), not variants as top-level rows.
2. If `color` or `size` filter is present, restrict to parents that have at least one matching **active** variant.
3. List card fields:
   - `defaultImageUrl` — first image of first active variant (or preferred color).
   - `priceFrom` — min active variant price.
   - `availableColors` / `availableSizes` — distinct values from active variants.

## Stock

_(Historical — see the status note at the top of this doc. As implemented, product is the source of truth directly; there is no separate inventory service to call.)_

- **Source of truth:** inventory, by variant `sku` (originally planned as a separate service; now merged into product).
- Remove reliance on `products.stock` for availability (stop writing it; ignore or drop later).
- PDP `inStock`: either
  - **A (preferred for UX):** product service batch-reads inventory for variant SKUs when building PDP, or
  - **B:** storefront calls inventory per SKU after loading PDP.

Start with **B** if inventory client is not wired into product yet; add **A** when PDP should be one request.

## Migration

1. Create `product_variants` table (inline migrate on product startup, same pattern as today).
2. Backfill: for each existing product row, insert one variant:
   - `sku` = current `products.id` (keeps inventory/order working),
   - `product_id` = current `products.id`,
   - `color` = current `color`,
   - `price` = current `price`,
   - `image_urls` = current `image_urls`,
   - `status` = current `status`.
3. Parents with a single variant behave as today for checkout (SKU still `BOT-001`).
4. New multi-color styles: one parent, multiple variants with new SKUs (`BOT-001-GRN`, …); create inventory rows for each new SKU.
5. After clients use the new PDP shape, stop accepting `color` / `imageUrls` / `stock` on parent create/update (or map them to a default variant for one release).

## Implementation phases

### Phase 1 — Schema + backfill (product)

- Add `product_variants` and repository methods.
- Backfill one variant per existing product.
- Keep existing public APIs working (parent still exposes legacy fields mirrored from the single/default variant if needed).

### Phase 2 — Read APIs (product)

- `GET /products` returns parents only + option summaries (no duplicate styles).
- `GET /products/{id}` returns parent + `variants[]` and `availableColors` / `availableSizes`.
- Update OpenAPI, `docs/endpoints.md`, `docs/api.md`, `docs/current-state.md`.
- Close / update `docs/TODO.md` item about admin single-product read if variants change manage semantics.

### Phase 3 — Write APIs (product)

- Variant CRUD + image upload on variant.
- Parent create/update without requiring color.
- Admin scripts (`upload_product_images.py`, seed/import) target variant SKUs.

### Phase 4 — Stock integration

- Document that merchants must set inventory per variant SKU.
- Optional: product PDP enriches `inStock` via inventory HTTP client.
- Remove or ignore `products.stock`.

### Phase 5 — Cleanup

- Drop legacy parent columns (`color`, `stock`, `image_urls`) or leave unused.
- Remove transitional mirroring of legacy fields on parent JSON.

## Out of scope (this plan)

- Generic option matrix UI beyond color/size.
- Changing inventory or order schemas (SKU already sufficient).
- Remodeling parent id generation (`BOT-001` stays).
- Merging existing duplicate color products into one parent (manual or one-off script later).

## Risks and decisions

| Topic | Decision |
|-------|----------|
| Delete parent with variants | Reject delete, or cascade variants + warn about inventory SKUs |
| Auto SKU | `{parentId}-{colorCode}` with optional `-{sizeCode}`; allow explicit `sku` on create |
| Filter by color in search | Parent included if any active variant matches |
| Bags without size | `size = ''`; only color variants |
| Duplicate option combo | Unique `(product_id, color, size)` |

## Success criteria

- Search never returns two rows for the same style solely because of color.
- PDP shows available colors/sizes and images per variant.
- Checkout reserves stock for the chosen variant SKU.
- Existing single-SKU products keep working after backfill without inventory migration.

## References

- Current product service: `product/pkg/`
- Inventory SKU model (merged into product): `product/pkg/domain/inventory.go`, `product/pkg/service/inventory_service.go`
- Order line items: `order/pkg/domain/order.go`
- Prior note on removed `/manage`: [TODO.md](TODO.md)
