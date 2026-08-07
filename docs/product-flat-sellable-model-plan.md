# Plan: Product as the sellable unit (SKU folded into Product)

**Status:** Reflection / migration plan (no code change yet).  
**Intent:** Make **`Product` the unit of sale**. Every current variant (SKU) becomes a `Product` row; the human `sku` and its segment codes become **fields on `Product`**. The old parent keeps only what is genuinely shared across colors, under a new name (**style**).  
**Related:** [product-variants-plan.md](product-variants-plan.md) (the model this revises), [product-sku-system.md](product-sku-system.md), [product-price-on-parent.md](product-price-on-parent.md), [product-multi-category-naming-plan.md](product-multi-category-naming-plan.md), [current-state.md](current-state.md).

---

## 1. Why this is coherent

Today's split exists because one style is sold in several colors. But the observation holds: **all variants of a parent share the same `styleCode`**, so the parent row carries no identity that `brandCode + styleCode` doesn't already carry.

```text
Today                                   Proposed
─────────────────────────────           ────────────────────────────────────
Product (parent, not sellable)          Style (grouping: brand + style code)
  id, name, price, taxonomy               shared marketing copy, taxonomy
  └── Variant (sellable SKU)              └── Product (SELLABLE)
        skuId (ULID), sku                       id  = former skuId (ULID)
        color/size/edition codes                sku = human SKU  ← now a Product field
        images                                  color/size/edition codes, images
             │                                  price, status, counts
             ▼                                       │
        stock / cart / order (by skuId)              ▼
                                              stock / cart / order (by product id)
```

**The migration's keystone:** set the new sellable `products.id` to the **existing `sku_id` ULID**. Cart, order, checkout sessions, reservations, and stock rows already store that value — so historical rows stay valid and no cross-service data rewrite is needed. `skuId` simply becomes an alias for `productId`.

---

## 2. The one decision that must come first

Flattening reintroduces the problem the parent was created to solve: **listing duplicates**. A style in 5 colors is 5 sellable products.

| Option | `GET /products` returns | PDP | Verdict |
|--------|------------------------|-----|---------|
| **A. Style-grouped listing** | One card per `styleCode` (representative product + sibling summary) | Product page with color/size picker across siblings | **Recommended** — same storefront UX as today, flat storage underneath |
| **B. Flat listing** | One card per sellable product (color-level cards) | Product page for exactly that color | Valid merchandising choice (common on luxury sites); higher result density, weaker "one style, one card" |
| C. Client-side dedupe | Everything, client groups | — | Reject: breaks pagination and totals |

Everything below assumes **A**, implemented as `SELECT DISTINCT ON (brand_code, style_code)` with a deterministic representative (lowest-price active, then oldest). Option B is a one-line relaxation of the same query, so the schema work is identical either way. **Pick A or B before Phase 3**; it is a merchandising decision, not a technical one.

---

## 3. Target shape

### `Product` (sellable)

```go
type Product struct {
	ID  string `json:"id"`            // ULID — was Variant.SkuID
	SKU string `json:"sku"`           // human SKU — was Variant.SKU

	// Grouping / design identity
	StyleCode string `json:"styleCode"`
	BrandCode string `json:"brandCode"`
	Brand     string `json:"brand"`

	// Option identity (was Variant)
	Color       string `json:"color"`
	ColorCode   string `json:"colorCode,omitempty"`
	Size        string `json:"size,omitempty"`
	SizeCode    string `json:"sizeCode,omitempty"`
	EditionCode string `json:"editionCode,omitempty"`

	// Merchandising (shared per style, denormalized or joined — see 4.2)
	Name, Description, Material, Category string
	SubCategory, Style, Target, Capacity  string
	Tags                                  []string
	Attributes                            map[string]string

	Price, OfficialPrice float64
	Status               string
	ImageURLs            []string

	ViewCount, SoldCount, WishlistCount int64
	CreatedAt, CreatedBy                string
}
```

`domain.Variant` disappears as a concept; `SkuID` survives only as a deprecated JSON alias of `id`.

### `Style` (grouping)

Today's `products` table becomes `product_styles`, keyed by `(brand_code, style_code)`, holding whatever is edited once per design: name, description, material, category, bag taxonomy, tags, attributes, and (optionally) baseline price.

**Keep the existing `styles` master table separate.** It is a code→name dictionary with `RESTRICT` FKs from products/variants; merging it with merchandising copy in the same migration multiplies risk for no benefit. Revisit once flattening has landed.

---

## 4. Field mapping

### 4.1 Direct moves

| Today | After |
|-------|-------|
| `product_variants.sku_id` (PK) | `products.id` (PK) |
| `product_variants.sku` | `products.sku` (UNIQUE) |
| `product_variants.color/size/*_code`, `image_urls`, `status` | same columns on `products` |
| `products.brand_code/style_code` | on `products` (denormalized) + `product_styles` |
| `stock_items.sku_id`, `reservation_items.sku_id` | unchanged values; now FK → `products(id)` |

### 4.2 Shared fields: denormalize or join

| Approach | Pros | Cons |
|----------|------|------|
| **Denormalize onto each sellable row** (recommended) | Single-table search stays fast; no join in the hot path; per-color overrides become possible (e.g. color-specific price) | Style edits must fan out to sibling rows in one transaction |
| Join `product_styles` on read | One source of truth | Every search/PDP query joins; grouped listing gets heavier |

Recommended: **denormalize + fan-out write**. `PUT /products/styles/{styleCode}` updates the style row and all siblings in one transaction; `PUT /products/{id}` edits only that sellable row.

### 4.3 Counts

| Counter | Today | After |
|---------|-------|-------|
| `viewCount` | Parent (unique guest per parent) | Keep at **style** level (`product_views` repoints to style) — PDP is still a style page under option A |
| `soldCount` | Parent, on reservation commit | Move to **sellable product** (which color sold), plus style rollup `SUM` for cards |
| `wishlistCount` | Parent | Keep at **style** level; wishlisting a color is a separate future feature |

Under option B, all three move to the sellable product.

---

## 5. API surface

| Endpoint | Change |
|----------|--------|
| `GET /api/v1/products` | Returns style-grouped cards (option A). Each card gains `styleCode`, representative `id`/`sku`, `availableColors`, `availableSizes`, price range |
| `GET /api/v1/products/{id}` | Accepts a sellable id **or** a legacy parent id (mapped to its style). Returns the sellable product plus `siblings` (same `styleCode`) |
| `variants` array in PDP JSON | Deprecated alias of `siblings` — keep one release for `dupli1-web` / `dupli1-manage-web` |
| `POST /api/v1/products` | Creates a sellable product; requires `brandCode` + `styleCode` + `colorCode`/`sizeCode` (composes `sku` exactly as today) |
| `POST /products/{id}/variants` | Deprecated → rewrite as "create sibling product under the same style" |
| `GET /products/variants/by-sku/{sku}` | Keep; now a product lookup by `sku` |
| `GET /products/variants/by-sku-id/{skuId}` | Keep; `skuId == id` |
| `GET /products/variants?sku_ids=` | Keep (batch by id); add `?ids=` as canonical name |
| Inventory `/products/inventory/*` | Unchanged paths. Internally the `sku` ⇄ `skuId` resolution collapses to a single product id lookup |
| Coupons, catalog masters, taxonomy | Unaffected |

**JSON compatibility:** emit both `id` and `skuId` (same value), and both `productId` (→ style) and `styleCode`, until frontends migrate. That keeps cart/order clients working untouched through the whole migration.

---

## 6. Cross-service impact

| Service | Impact |
|---------|--------|
| **cart** | None functionally — stores `sku_id` + `sku`; values unchanged. Later rename to `product_id` |
| **order** / checkout sessions | None — `OrderItem.SkuID` values unchanged. `httpproduct` client keeps reading `skuId` from variant JSON |
| **product/inventory** | Simplifies: `SkuRef` dual-resolution can eventually collapse to one id |
| **notification** | Event payloads gain `styleCode`; existing fields retained |
| **frontends** (`dupli1-web`, `dupli1-manage-web`) | Real work: PDP builds its picker from `siblings` instead of `variants`; manage-web's parent/variant CRUD screens become style/product screens. Coordinated with the pending `skuId` migration already tracked in [frontend-product-variants-migration.md](frontend-product-variants-migration.md) |

---

## 7. Phases

**Phase 0 — decide.** Listing semantics (A vs B); counts placement; denormalize vs join. Nothing else starts until these are fixed.

**Phase 1 — schema, additive.** Create `product_styles` from current `products`. Add SKU/merchandising columns to `product_variants`. Backfill: copy parent fields down to every variant row; set `sku_id` as the row's identity. No reads change yet.

**Phase 2 — domain + stores.** New `domain.Product` (sellable) and `domain.Style`; `ports.ProductStore` gains style-grouped search and sibling lookup. Keep `Variant` as a deprecated type alias so handlers compile during the transition.

**Phase 3 — read path.** Grouped search, PDP with `siblings`, `variants` emitted as a mirror. Legacy parent ids resolve to their style. Extensive route/response tests (the existing handler test suite is the safety net here).

**Phase 4 — write path.** Sellable create/update; style-level fan-out update; deprecate `POST /products/{id}/variants` while keeping it functional.

**Phase 5 — rename tables and cut over.** `product_variants` → `products`, old `products` → `product_styles` (already created in Phase 1; this step drops the shim views). Repoint `product_views` / `product_wishlists` FKs per the Phase 0 counts decision.

**Phase 6 — cleanup.** After frontends migrate: drop `variants` mirror, drop `skuId` alias, collapse inventory's dual resolution, rename cart/order columns to `product_id`.

Each phase is independently deployable and reversible; phases 1–4 leave the public API behaviorally unchanged apart from additive fields.

---

## 8. Trade-offs to accept

| Gain | Cost |
|------|------|
| One sellable entity; no parent/variant impedance in cart, order, stock | Reverses [product-variants-plan.md](product-variants-plan.md); that doc must be marked superseded |
| `sku` and its codes live on the thing that is actually sold | Listing must group by `styleCode`, or duplicates return |
| Per-color price / status / imagery becomes natural | Contradicts [product-price-on-parent.md](product-price-on-parent.md); "edit price once" needs style-level fan-out |
| Historical order/cart ids stay valid (id = old `skuId`) | Wishlist/view rows keyed on parent ids need repointing |
| Inventory drops a resolution layer | Recommendations must dedupe by style, or one design floods the rail |
| Multi-category future unaffected | Manage-web CRUD screens need rework |

Naming stays category-agnostic — `Product` remains the right noun, per [product-multi-category-naming-plan.md](product-multi-category-naming-plan.md). This plan changes **granularity**, not category modeling.

---

## 9. Checklist

- [ ] Phase 0 decisions recorded (listing A/B, counts placement, denormalize vs join)
- [ ] `product_styles` created + backfilled; sellable columns added
- [ ] `domain.Product` (sellable) + `domain.Style`; `Variant` deprecated alias
- [ ] Grouped search + PDP `siblings` (+ `variants` mirror) with tests
- [ ] Sellable write path + style fan-out update
- [ ] `product_views` / `product_wishlists` repointed; `soldCount` per product with style rollup
- [ ] Recommendations dedupe by `styleCode`
- [ ] Frontend migration coordinated with the pending `skuId` work
- [ ] Superseded notes added to [product-variants-plan.md](product-variants-plan.md) and [product-price-on-parent.md](product-price-on-parent.md)
- [ ] Legacy aliases dropped (Phase 6)
