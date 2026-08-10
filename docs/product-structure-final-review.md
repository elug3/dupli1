# Final review: `Product{}` and sellable SKU structure

**Status:** Accepted — authoritative field ownership and naming for the catalog model.  
**Code today:** `domain.Product` (parent) + `domain.Variant` (sellable SKU). There is **no** `Sku{}` / `SKU{}` Go type.  
**Target:** fold the sellable SKU into `Product` per [product-flat-sellable-model-plan.md](product-flat-sellable-model-plan.md).  
**Related:** [product-sku-system.md](product-sku-system.md), [product-sale-unit-reflection.md](product-sale-unit-reflection.md), [product-multi-category-naming-plan.md](product-multi-category-naming-plan.md), [product-multi-category-design.md](product-multi-category-design.md), [product-sku-dimensions.md](product-sku-dimensions.md).

---

## 1. Verdict

| Question | Answer |
|----------|--------|
| Is there a `Sku{}` type? | **No.** Sellable unit is `Variant` today (`SkuID` + human `SKU` fields). |
| Rename `Product` → `Bag` / invent `BagSku`? | **Reject** — keep category-agnostic names. |
| What is the unit of sale? | **SKU** (`skuId`) today; **`Product`** after flatten (`id` = former `skuId`). |
| What groups colors for browse/PDP? | Parent `Product` today; **`Style`** (`brandCode` + `styleCode`) after flatten. |
| Keep empty `Bag`/`Shoes`/… embeddings? | **No** — delete; category is data, not a Go subtype. |

```text
TODAY (implemented)                         TARGET (accepted, not migrated)
─────────────────────────────               ────────────────────────────────
Product (parent / PDP shell)                Style (grouping: brand + styleCode)
  └── Variant (sellable SKU)                  └── Product (SELLABLE)
        skuId, sku, color/size/…                    id (= old skuId), sku, color/size/…
             │                                           │
             ▼                                           ▼
        stock / cart / order (skuId)              stock / cart / order (product id)
```

Do **not** start the flatten migration until the Phase 0 locks in §5 are treated as fixed (they are, below). Implementation stays in [product-flat-sellable-model-plan.md](product-flat-sellable-model-plan.md).

---

## 2. What the code has today

Source: `product/pkg/domain/products.go`.

### 2.1 `Product` — parent catalog style (not the cart line)

| Field | Role | Notes |
|-------|------|-------|
| `ID` | Parent / PDP key (ULID for new rows) | Opaque; not encoded with color |
| `Name`, `Description`, `Brand`, `Material` | Merchandising copy | Shared across variants |
| `BrandCode`, `StyleCode` | SKU segment masters | Required on create; compose human `sku` |
| `Category`, `SubCategory`, `Style`, `Target` | Taxonomy | Bag-shaped today; `Style` ≠ `StyleCode` |
| `OfficialPrice`, `Price` | Reference + sale (KRW) | On parent; variants inherit on read |
| `Status` | `active` \| `draft` \| `archived` | |
| `Capacity`, `Tags`, `Attributes` | Bag capacity + free-form PDP memo | `Attributes` not searched |
| `ViewCount`, `SoldCount`, `WishlistCount` | Denormalized counters | Views/wishlist at parent; sold rolls up from commits |
| `DefaultImageURL`, `AvailableColors`, `AvailableSizes`, `Variants` | Read-time summaries | Not separate storage |
| `Color`, `Stock`, `ImageURLs` | **Legacy mirrors** | Prefer variant fields + inventory |

### 2.2 `Variant` — sellable SKU (the real sale unit)

Docs and ops say “SKU”; the Go type is `Variant`.

| Field | Role | Notes |
|-------|------|-------|
| `SkuID` | Canonical ULID | Inventory, cart, order, reservations |
| `SKU` | Human string | `Brand_Style_Color[_Edition]_Size`; immutable after create |
| `ProductID` | Parent FK | |
| `Color`, `Size` | Display labels | |
| `ColorCode`, `EditionCode`, `SizeCode` | Normalized SKU segments | Masters in catalog tables |
| `Dimensions` | Physical mm (`widthMm`/`heightMm`/`depthMm`) | Distinct from letter size |
| `OfficialPrice`, `Price` | Filled from parent on read | Not stored on variant row |
| `Status`, `ImageURLs`, `CreatedAt` | Option lifecycle + media | Per-color images |

Supporting types (not `Sku{}`):

- `SKUParts` — segment DTO for compose/parse (`sku.go`)
- `SkuRef` — inventory lookup by `SkuID` and/or human `SKU`
- `Dimensions` — physical size on the sellable option

### 2.3 Dead stubs (remove)

`Bag`, `Shoes`, `Outerwear`, `Bottoms`, `Clock` embed `Product` and have **zero** call sites. They are the wrong multi-category pattern ([product-multi-category-naming-plan.md](product-multi-category-naming-plan.md)). Delete from `products.go`; keep `Consultation` only if still referenced.

---

## 3. Field ownership rules (stable across flatten)

These rules do not change when Variant folds into Product — only which Go type holds the column.

| Concern | Lives on | Never on |
|---------|----------|----------|
| Design identity (`brandCode` + `styleCode`) | Grouping layer (parent today / Style later) | Per-color overrides of codes after create |
| Human `sku` segments (color/size/edition) | Sellable row | Parent-only (would lose per-option identity) |
| Sale / official price | Shared today (parent); after flatten denormalized on sellable with style fan-out | Inventing a third price column |
| Stock quantity / reservations | Inventory keyed by `skuId` (→ product `id` later) | Parent aggregate as source of truth |
| Images | Sellable (per color) | Solely on parent (parent may keep `defaultImageUrl`) |
| Physical dimensions (mm) | Sellable | Letter `size` / `sizeCode` |
| Bag taxonomy (`subCategory`, look `style`, `target`) | Grouping / denormalized merchandising | SKU string |
| Free-form `attributes` | Merchandising (PDP memo) | Search, pricing, checkout |
| Cart / order line identity | `skuId` (today) / product `id` (after) | Parent id alone when multi-option |

---

## 4. Target `Product{}` (sellable) after flatten

Accepted shape from the flat plan, plus dimensions added since that plan was written:

```go
type Product struct {
	ID  string `json:"id"`  // ULID — was Variant.SkuID
	SKU string `json:"sku"` // human — was Variant.SKU

	BrandCode string `json:"brandCode"`
	StyleCode string `json:"styleCode"`
	Brand     string `json:"brand"`

	Color       string `json:"color"`
	ColorCode   string `json:"colorCode,omitempty"`
	Size        string `json:"size,omitempty"`
	SizeCode    string `json:"sizeCode,omitempty"`
	EditionCode string `json:"editionCode,omitempty"`
	Dimensions  *Dimensions `json:"dimensions,omitempty"`

	Name, Description, Material, Category string
	SubCategory, Style, Target, Capacity  string
	Tags       []string
	Attributes map[string]string

	Price, OfficialPrice float64
	Status               string
	ImageURLs            []string

	ViewCount, SoldCount, WishlistCount int64 // placement per §5
	CreatedAt, CreatedBy                string
}
```

- No `Sku{}` type. `sku` / optional deprecated JSON `skuId` (== `id`) are fields on `Product`.
- Grouping type is `Style` / `product_styles`, **not** a second sellable entity.
- Category-specific facets (`details`) stay orthogonal — see [product-multi-category-design.md](product-multi-category-design.md).

---

## 5. Phase 0 locks (unblocks flatten)

Recorded here so [product-flat-sellable-model-plan.md](product-flat-sellable-model-plan.md) is no longer blocked on open questions:

| Decision | Lock | Rationale |
|----------|------|-----------|
| Listing / PDP | **A — style-grouped** | One card per `styleCode`; PDP picker from `siblings` (mirror `variants` one release) |
| Shared fields | **Denormalize + fan-out write** | Hot-path search stays single-table; style edit updates siblings in one TX |
| `viewCount` / `wishlistCount` | **Style level** | PDP is still a style page under listing A |
| `soldCount` | **Sellable product** + style `SUM` rollup for cards | Which color sold matters for ops |
| Migration keystone | **`products.id` = existing `sku_id`** | Cart/order/stock history stays valid |

---

## 6. Naming decisions (final)

| Proposal | Decision |
|----------|----------|
| Introduce `type Sku struct` | **Reject** — use `Variant` until flatten; then SKU is fields on `Product` |
| Rename `Variant` → `Sku` before flatten | **Reject** — churn with no API win; flatten removes the type anyway |
| Rename `Product` → `Bag` / `BagSku` | **Reject** |
| Keep `Product` + `Variant` names until flatten lands | **Accept** |
| After flatten: `Product` = sellable, `Style` = grouping | **Accept** |
| Downstream keeps storing `sku_id` values through migration | **Accept** (values unchanged; rename columns later) |

---

## 7. Hygiene checklist

- [x] Final field ownership and naming reviewed (this doc)
- [x] Phase 0 locks recorded (§5)
- [x] Delete unused `Bag` / `Shoes` / `Outerwear` / `Bottoms` / `Clock` / `Consultation` stubs from `products.go`
- [x] Point [product-flat-sellable-model-plan.md](product-flat-sellable-model-plan.md) Phase 0 at this doc
- [ ] Mark [product-variants-plan.md](product-variants-plan.md) superseded when flatten Phase 1 starts (not before)
- [ ] Execute flatten phases 1–6 in the flat plan
- [ ] Multi-category taxonomy fix remains separate ([product-multi-category-design.md](product-multi-category-design.md) phases 1–2)
