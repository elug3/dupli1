# Plan: product catalog API v2

**Status:** Proposed (2026-11-04). Not implemented. Open decisions in §3 need an answer before Step 3.
**Supersedes:** the API surface of [product-flat-sellable-model-plan.md](product-flat-sellable-model-plan.md) (§3 target struct, §5, and phases 3–4/6 as they reshape v1). That plan's storage migration (§4, §6, phases 1, 2, 5) still applies and is Step 2 here.
**Related:** [product-structure-final-review.md](product-structure-final-review.md) (Phase 0 locks), [product-sku-system.md](product-sku-system.md), [product-price-on-parent.md](product-price-on-parent.md), [product-master-catalog.md](product-master-catalog.md), [frontend-product-variants-migration.md](frontend-product-variants-migration.md).

---

## 1. Why a v2

The flat plan makes the SKU the `Product` and the parent a `Style`, which is right. It gets there by reshaping **v1 in place**: `id` and `skuId` emitted with one value, `productId` beside `styleCode`, `variants` mirrored as `siblings`. Every client carries the duplicate fields for the length of the migration, and v1 ends up more ambiguous than it started.

The structs also have problems that no in-place change can fix without breaking clients:

| Problem today | Where |
|---|---|
| One struct is the stored row, the response, the create body and the patch; response-only fields (`availableQty`, `inStock`, `price`) are accepted on input and ignored | `domain.Variant`, `domain.Product` |
| `MergeUpdate` treats `""` / `0` / empty slice as "unchanged", so some values can never be written (clear `size`, remove every image, price `0`), and the price overrides need a separate "0 clears" rule | `domain/enrich.go` |
| Money is `float64` (columns `NUMERIC(10,2)`), so a fractional won can exist and is caught only at runtime | `Price`, `PriceOverride`, `ValidateVariantPrices` |
| `colorCode` / `sizeCode` are patchable but `sku` is not, so `sku` can stop describing the row | `UpdateVariant` |
| `style` means the occasion (`casual`) on the product and the design family (`CAS001`) in the SKU | `Product.Style` vs `Product.StyleCode` |
| `brand` (name) is copied onto the row and goes stale when the master is renamed | `Product.Brand` |
| Legacy copies of the default variant on the parent | `Product.Color`, `Product.ImageURLs`, `Product.Stock` |
| Stock and counters sit among entity fields | `AvailableQty`, `InStock`, `ViewCount`, `SoldCount`, `WishlistCount` |
| Strings for statuses and timestamps | `Status string`, `CreatedAt string` |
| Errors are `{"error": "<message>"}` with no code; an unknown master code on create is a `404` | `handler/errors.go` |

v2 is a new path, `/api/v2/…`, with clean schemas over the flattened model. v1 stays as it is today, served by a translation layer, until its clients move.

---

## 2. Three steps

| Step | What | Wire change | Who must change |
|---|---|---|---|
| **1. Domain cleanup** | Typed money, enums, time; separate entity / view / input / patch types | None (v1 byte-identical) | Nobody |
| **2. Flatten storage** | Flat plan phases 1, 2, 5: `product_styles`, sellable rows, `id` = existing `sku_id` | None (v1 translated) | Nobody |
| **3. `/api/v2`** | New handlers and schemas for products and styles | Additive (new path) | `dupli1-web`, `dupli1-manage-web`, then `shared/pkg/productclient` |

Each step ships on its own and can be rolled back on its own. Step 1 alone removes most of the type problems in §1.

---

## 3. Decisions

### Inherited (locked in the final review §5)

- Listing **A**: one card per style.
- Shared fields are **denormalized** onto each sellable row and **fanned out** on a style edit, in one transaction.
- `viewCount` / `wishlistCount` at **style** level; `soldCount` per sellable product with a style rollup.
- Keystone: the sellable `products.id` **is** the existing `sku_id`, so cart, order, reservation and stock rows stay valid.

### Open: answer before Step 3

| # | Question | Recommendation |
|---|---|---|
| D1 | JSON casing for v2. Product v1 is camelCase (`officialPrice`); cart, order and payment are snake_case (`unit_price_won`), and the project rule names money `*_won` | **snake_case**, so money reads `price_won` the same way everywhere. Examples below use it; if camelCase wins, they change mechanically |
| D2 | When v1 product routes are removed | One release after both web apps ship on v2, with a date set when v2 ships (the way [product-promotion-rename.md](product-promotion-rename.md) did) |
| D3 | Optimistic locking on writes | `ETag` from `updated_at`, `If-Match` required on `PATCH`. A style edit fans out to every color, so two managers overwriting each other costs more than it does today |
| D4 | Rename the occasion field | `style` → `occasion` in v2 (filter `occasion=casual`). Frees "style" to mean the design family only |

---

## 4. Step 1 — domain cleanup (no wire change)

All inside `product/`; v1 JSON stays byte-identical, which the existing handler tests check.

| Change | Detail |
|---|---|
| Money | `type Won int64` in `shared/pkg/money` (it already owns whole-won rules; `FromProductPrice` becomes unnecessary once product stops emitting floats internally). Price columns go from `NUMERIC(10,2)` to `BIGINT`. Inline migrations can't make breaking changes, so it is additive: add the `BIGINT` columns, backfill, switch reads and writes, drop the old columns a release later. v1 still writes JSON numbers, which serialize the same for whole values |
| Enums | `type Status string` (`draft`, `active`, `archived`) with `Valid()`; same for category codes. Parse at the handler, reject there |
| Time | `time.Time` in the domain; RFC 3339 formatting only in views |
| Entity vs view | `domain.Variant` / `domain.Product` hold stored fields only. Effective price, stock, colour hexes, product name and counters move to `handler` view types built from the entity plus lookups |
| Input vs patch | `CreateVariant` / `CreateProduct` input structs; `VariantPatch` / `ProductPatch` with pointer fields (`nil` = keep), like `domain.ColorPatch`. `MergeUpdate` is deleted |
| JSON tags | Removed from `domain`; they live on handler types only. Keeps the domain free of wire concerns, per the hexagonal rule |
| Immutable codes | **Done early (2026-11-04)**: v1 `PUT` that changes a code answers `400` naming the field and the sku (`domain.CheckVariantCodesUnchanged`). What remains here is the type-level version: `VariantPatch` has no `ColorCode` / `SizeCode` / `EditionCode` |

---

## 5. Step 2 — flatten storage

Unchanged from [product-flat-sellable-model-plan.md](product-flat-sellable-model-plan.md) §4, §6 and phases 1, 2 and 5. Two additions:

- **Prerequisite:** every parent must carry `brand_code` and `style_code` before Phase 1. One parent per `(brand_code, style_code)` is already enforced by `ux_products_brand_style`, but only where both are non-null; legacy rows without codes cannot be grouped and need a backfill or a manual assignment first. Count them before starting.
- **v1 is a translator, not a dual emitter.** v1 handlers build today's parent + `variants` JSON from a style and its sellable products. No `siblings`, no `id`/`skuId` duplication on v1; new shapes go only to v2. Legacy parent ids keep resolving to their style on v1.

---

## 6. Step 3 — the v2 API

### 6.1 Conventions

| Topic | Rule |
|---|---|
| Base path | `/api/v2/products/…` and `/api/v2/styles/…`, served by product |
| Casing | snake_case (D1) |
| Money | integers, whole won, `*_won` |
| Time | RFC 3339 UTC strings |
| Unknown or read-only request fields | `400 unknown_field` (decoder uses `DisallowUnknownFields`; request structs contain only writable fields) |
| `PATCH` | JSON Merge Patch ([RFC 7396](https://www.rfc-editor.org/rfc/rfc7396)): missing = keep, `null` = clear, value = set. No `0`/`""` sentinels |
| Immutable fields | `sku`, `brand_code`, `style_code`, `color_code`, `size_code`, `edition_code`, `id`, `created_at`: changing one is `422 immutable_field`. A different color is a new product |
| Pagination | cursor: `?limit=` (max 100) and `?cursor=`, response `next_cursor` (null at the end). No offsets |
| Errors | `{"error": {"code": "unknown_master", "message": "color RDX does not exist", "field": "color_code"}}`. An unknown master code on create is `422`, not `404` |
| Concurrency | `ETag` on reads, `If-Match` on `PATCH` / `DELETE` (D3); mismatch is `412` |
| Permissions | unchanged names: `product.read`, `product.create`, `product.update`, `product.delete`, `product.master.*` |

### 6.2 Resources

**Product** — the sellable unit (one color/size/edition of a style).

```json
{
  "id": "01JAY6Z9K3F8QW1G7H2T5X0ABC",
  "sku": "BOT_CAS001_BLK_V_MED",
  "style": {
    "brand": { "code": "BOT", "name": "Bottega Veneta" },
    "code": "CAS001",
    "name": "Cassette"
  },
  "color":   { "code": "BLK", "name": "Black", "hex": "#0a0a0a", "hex2": null },
  "size":    { "code": "MED", "name": "Medium" },
  "edition": { "code": "V", "name": "Standard" },
  "status": "active",
  "price_won": 3200000,
  "list_price_won": 3900000,
  "price_source": "style",
  "dimensions_mm": { "width": 340, "height": 220, "depth": 80 },
  "images": [
    { "url": "https://…/1.jpg", "listing_url": "https://…/1_600.jpg" }
  ],
  "availability": { "qty": 3, "in_stock": true },
  "stats": { "sold": 12 },
  "created_at": "2026-11-04T09:12:00Z",
  "updated_at": "2026-11-04T09:12:00Z"
}
```

- `price_won` / `list_price_won` are effective (product override, else style). `price_source` is `style` or `product`, so the console can show which one is in force. `list_price_won` replaces `officialPrice`.
- Master names (`brand.name`, `color.name`, `size.name`) are joined on read, never stored on the row.
- `images` pairs each full image with its listing thumbnail instead of two parallel arrays.

**Style** — the design (brand + style code), its shared copy, and the browse/PDP unit.

```json
{
  "brand": { "code": "BOT", "name": "Bottega Veneta" },
  "code": "CAS001",
  "name": "Cassette",
  "description": "…",
  "material": "Intrecciato lambskin",
  "category": "bags",
  "sub_category": "shoulder",
  "occasion": "evening",
  "target": "women",
  "capacity": "…",
  "tags": ["intrecciato"],
  "attributes": { "care": "…" },
  "size_chart": [],
  "status": "active",
  "price_won": 3200000,
  "list_price_won": 3900000,
  "stats": { "views": 1840, "wishlists": 96, "sold": 31 },
  "products": [ { "…": "Product, as above" } ],
  "created_at": "…",
  "updated_at": "…"
}
```

**Style card** — what listings return (listing A): the style without `products`, plus `colors` (code, name, hex), `sizes`, `price_from_won` / `price_to_won`, `image` (representative product's first listing image) and `in_stock`.

### 6.3 Routes

| Method | Path | Body / query | Notes |
|---|---|---|---|
| `GET` | `/api/v2/styles` | `q`, `category`, `sub_category`, `occasion`, `target`, `brand`, `in_stock`, `sort`, `limit`, `cursor` | Style cards; replaces `GET /api/v1/products` |
| `GET` | `/api/v2/styles/{brand_code}/{style_code}` | — | Style with all its products; the PDP call |
| `POST` | `/api/v2/styles` | style fields + `brand_code`, `style_code` | Masters must exist |
| `PATCH` | `/api/v2/styles/{brand_code}/{style_code}` | merge patch | Fans out shared fields to every product of the style in one transaction |
| `DELETE` | `/api/v2/styles/{brand_code}/{style_code}` | — | Refused (`409`) while any product holds reserved stock, as today |
| `GET` | `/api/v2/products/{id}` | — | One sellable product |
| `GET` | `/api/v2/products/by-sku/{sku}` | — | Lookup by human SKU |
| `GET` | `/api/v2/products` | `ids=a,b,…` (max 100) | Batch; replaces `/variants?sku_ids=`. What cart and order need |
| `POST` | `/api/v2/products` | `brand_code`, `style_code`, `color_code`, `size_code`, `edition_code?`, `status?`, `price_override_won?`, `list_price_override_won?`, `dimensions_mm?` | Composes `sku`, creates a stock row at `0` |
| `PATCH` | `/api/v2/products/{id}` | merge patch: `status`, `price_override_won`, `list_price_override_won`, `dimensions_mm` | `null` override = inherit from style |
| `DELETE` | `/api/v2/products/{id}` | — | Same reserved-stock refusal |
| `POST` | `/api/v2/products/{id}/images` | multipart | Appends; listing thumb generated as today |
| `PUT` | `/api/v2/products/{id}/images` | `[{"url": …}]` | Replace order / remove; `[]` removes all |

**Stays on v1 for now** (works with v2 ids, since a v2 product id is a v1 `skuId`): inventory, promotions, catalog masters and taxonomy, wishlist, recommendations, visits, reports. Each can get a v2 later if it has its own reasons to change; none are needed for the catalog cutover.

### 6.4 v1 → v2 mapping

| v1 | v2 |
|---|---|
| `Product` (parent) | `Style` |
| `Variant` | `Product` |
| `skuId` | `id` |
| `productId` on a variant | `style.brand.code` + `style.code` |
| `style` (occasion) | `occasion` |
| `brandCode` / `brand` | `style.brand.code` / `style.brand.name` |
| `price` / `officialPrice` | `price_won` / `list_price_won` |
| `priceOverride` / `officialPriceOverride` | `price_override_won` / `list_price_override_won` (write), `price_source` (read) |
| `imageUrls` + `listingImageUrls` | `images[].url` + `images[].listing_url` |
| `availableQty` / `inStock` | `availability.qty` / `availability.in_stock` |
| `viewCount` / `wishlistCount` / `soldCount` | `stats.views` / `stats.wishlists` / `stats.sold` |
| `colorHex` / `colorHex2` | `color.hex` / `color.hex2` |
| `Product.color`, `Product.imageUrls`, `Product.stock` | removed |
| `limit` / `offset` | `limit` / `cursor` |

---

## 7. Clients and gateway

| Client | Change | When |
|---|---|---|
| `dupli1-web` | Listing from `/styles`, PDP from `/styles/{brand}/{style}`, picker from `products` | After Step 3 |
| `dupli1-manage-web` | Parent/variant screens become style/product screens; send `If-Match` | After Step 3 |
| `shared/pkg/productclient` (cart, order) | Switch to `GET /api/v2/products?ids=` and `/by-sku/`; map `price_won` instead of `price` | Last; v1 lookups keep working until then |
| Storefront URLs | PDP links move from a parent id to `/{brand}/{style}`; old links resolve through v1's legacy-id lookup and redirect | With `dupli1-web` |
| Gateway | `location /api/v2/products` and `location /api/v2/styles` → `$gw_product` in `api/gateway/routes.conf` only; no internal v2 routes, so the `:8081` regex is untouched | Step 3 |
| Docs | `docs/api.md`, `docs/endpoints.md`, `docs/current-state.md` | Step 3 |

---

## 8. Catalog findings deferred to v2

Found while reviewing the catalog master (2026-11-04). Three bugs were fixed in v1 without changing the wire shape: every category size is seeded into the size master, display names resolve against the master tables instead of the compiled-in seeds, and variant codes can't change after create. Everything below changes what clients send or receive, or needs several repos to move together, so it waits for v2.

| Item | Today | v2 |
|---|---|---|
| Categories as data | `domain.SeedCategories` in Go (`taxonomy.go`): categories, subcategories, category sizes; a new category needs a deploy | Categories, subcategories and category sizes in Postgres with an admin API (`product.master.write`) |
| One list, read by every client | dupli1-web (`app/lib/catalog.ts` slug maps and `STOREFRONT_CATEGORIES`, `SIZE_ORDER` in `product-variants.ts`) and manage-web (`app/lib/categories.ts`, `KNOWN_CATEGORIES` in `routes/products.tsx`) keep their own copies | Both read `/catalog/categories`, which carries display order; adding a category is one admin action |
| Bag taxonomy tables | `bag_subcategories` / `bag_styles` / `bag_targets` are seeded and listed from Postgres, but writes validate against the Go seeds and there is no write API, so a row added there shows as a filter and is refused on create | Settled by the category redesign: one source, read and validated the same way |
| Size-chart measurements | Four fixed fields (`chestCm`, `lengthCm`, `shoulderCm`, `sleeveCm`), at most 20 rows | Measurement keys per category (trousers: waist, inseam; shoes: foot length) |
| A SKU's own size | Clients find the `sizeChart` row for the variant's `sizeCode` themselves | `measurements_cm` (clothing) / `dimensions_mm` (bags) on every product (§6.2), still stored once per style |
| Names as input | A create may send `color: "Ivory"` instead of `colorCode`; v1 resolves it against the masters | Requests carry codes only; names are read-only |
| Error and empty-list shapes | An unknown master code is `404`; a brand with no styles lists as `null` | `422 unknown_master` naming the field; empty lists are `[]` |
| Blank category | Validated and stored as bags (`DefaultCategory`) | `category` required; the migration assigns old blank rows |
| Legacy fallbacks | `"mem"` → `"men"`, `PRD` brand prefix, `OptionCode` X-padding, `StyleCodeFromProductID` (`BOT-001` → `S001`) | Removed once the flatten has given every row real codes |
| Measurements before going live | A style can be `active` with no size chart, a bag with no dimensions | A clothing style needs a size chart, and a bag product its dimensions, before `active` |

---

## 9. Trade-offs

| Gain | Cost |
|---|---|
| Clients see one clean shape; no duplicate fields during the migration | Two API versions to keep correct until D2's date |
| Type-level guarantees: no fractional won, no unknown status, no read-only field on input | Step 1 touches every product store and handler |
| Codes are immutable, so `sku` always describes the row | Fixing a wrong color means archiving and recreating, which loses that row's sold count and images |
| Explicit `null` semantics on `PATCH` | Clients must send `null`, not omit, to clear |
| Readable style URLs (`/BOT/CAS001`) | A style can't be re-coded; the codes are already immutable masters, so this just makes it visible |

---

## 10. Checklist

**Decisions**
- [ ] D1 JSON casing
- [ ] D2 v1 removal date
- [ ] D3 optimistic locking
- [ ] D4 `style` → `occasion`

**Step 1 — domain cleanup**
- [ ] `money.Won`; product price columns to `BIGINT`
- [ ] `Status` / category enums; `time.Time`
- [ ] Entity / view / input / patch types; `MergeUpdate` removed; JSON tags out of `domain`
- [x] Variant codes immutable on update (v1 `400`) — landed early with the catalog master fixes
- [ ] v1 handler tests pass unchanged

**Step 2 — flatten storage** (flat plan phases 1, 2, 5)
- [ ] Parents without `brand_code` / `style_code` counted and fixed
- [ ] `product_styles` + sellable rows; `id` = `sku_id`
- [ ] v1 served by translation from styles + products

**Step 3 — v2**
- [ ] Routes in §6.3 with tests; error, cursor and merge-patch helpers
- [ ] Gateway `location`s; `api/gateway/test.sh` passes
- [ ] `dupli1-web`, then `dupli1-manage-web`, then `productclient` on v2
- [ ] v1 product routes removed on the D2 date
- [ ] §8 deferred catalog items
