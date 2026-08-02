# Design: adding wallets (multi-category product types)

**Status:** Design plan (no code change yet).  
**Scope:** What the product service needs so `category=wallets` is a first-class type alongside bags.  
**Related:** [product-multi-category-naming-plan.md](product-multi-category-naming-plan.md) (no `Bag` / `Wallet` structs), [product-master-catalog.md](product-master-catalog.md) (bag taxonomy as built), [product-flat-sellable-model-plan.md](product-flat-sellable-model-plan.md), [product-sku-system.md](product-sku-system.md).

---

## 1. The answer in one line

**Do not add a `Wallet` type.** Keep one `Product` type and make **category a first-class, validated dimension** with a per-category taxonomy registry. Wallets then need seed data and a validation fix — not a new model.

---

## 2. What actually blocks wallets today

A wallet with a **blank** `subCategory` can be created today (empty values are allowed). A wallet with a *wallet* subcategory **fails**, and not for a structural reason. `NormalizeProductTaxonomy` (`product/pkg/domain/taxonomy.go`) validates `subCategory` / `style` / `target` against hardcoded **bag** seeds without ever reading `p.Category`:

```go
sc, ok := NormalizeSubCategory(p.SubCategory) // looks up SeedSubCategories only
if !ok {
	return fmt.Errorf("invalid subcategory %q", p.SubCategory)
}
```

Verified against the current code:

| Input | Result today | Should be |
|-------|--------------|-----------|
| `category=wallets`, no `subCategory` | accepted | accepted |
| `category=wallets`, `subCategory=bifold` | **`400 invalid subcategory "bifold"`** | accepted |
| `category=wallets`, `subCategory=tote` | **accepted** (a bag term on a wallet) | `400` |
| `category=wallets`, `style=business`, `target=women` | accepted | accepted (shared dimensions) |

Both failure modes come from the same cause: the legal set is the bag seeds (`handbags`, `tote`, `shoulder`, `cross`, `mini`) for every product, so wallet terms are rejected and bag terms leak onto wallets.

Three related gaps:

| Gap | Detail | Consequence for wallets |
|-----|--------|-------------------------|
| **Validation is bag-hardcoded** | `NormalizeProductTaxonomy` ignores `p.Category` | Any wallet subcategory is rejected |
| **`category` is free text** | `products.category TEXT DEFAULT ''`, no master, no validation on create | `wallets`, `Wallet`, `wallet` all coexist silently |
| **Taxonomy tables are bag-named and read-only for validation** | `bag_subcategories` / `bag_styles` / `bag_targets` are seeded and listed, but validation uses Go slices | Adding a DB term doesn't make it valid |

So the work is category-scoping the taxonomy, not modeling a new entity.

---

## 3. Which dimensions are actually category-specific

Auditing the three existing dimensions against wallets:

| Dimension | Product field | Bags | Wallets | Verdict |
|-----------|---------------|------|---------|---------|
| **Sub category** | `subCategory` | tote, shoulder, cross, mini, handbags | bifold, trifold, card holder, zip-around, long, coin purse | **Category-scoped** |
| **Style / look** | `style` | casual, evening, business, weekend, statement | same set reads naturally | **Shared** (registry may override) |
| **Target** | `target` | all, men, women, kids | identical | **Shared** |

Only `subCategory` genuinely differs. That keeps the change small: one dimension becomes category-scoped, the other two stay global.

SKU masters need no split either — brands, colors, sizes, and editions are already category-agnostic. Wallets compose the same human SKU (`Brand_Style_Color[_Edition]_Size`) with size `OS` in most cases.

---

## 4. Target design

### 4.1 One type, category-driven behavior

```go
// Unchanged shape. Category selects which taxonomy terms are legal.
type Product struct {
	Category    string // "bags" | "wallets" | …  (validated against master)
	SubCategory string // category-scoped term
	Style       string // shared "look" term
	Target      string // shared audience term
	// …
}
```

No `Wallet` struct, no `BagProduct` / `WalletProduct` interface, no per-category table, no second service. The unused `Bag` / `Shoes` / `Outerwear` stubs in `products.go` are the pattern to avoid — they never earned a field.

### 4.2 Category registry (domain)

```go
type Dimension struct {
	Key      string // "subCategory" | "style" | "target"
	Scope    Scope  // ScopeCategory | ScopeShared
	Required bool
}

type CategoryDef struct {
	Code       string // "wallets"
	Name       string // "Wallets"
	Dimensions []Dimension
}
```

`NormalizeProductTaxonomy(p)` resolves `p.Category` → `CategoryDef`, then validates each dimension against the terms for that (category, dimension) pair. Unknown category → `400`; unknown term for that category → `400` naming the category, e.g. `invalid subcategory "tote" for category "wallets"`.

Go seeds stay the bootstrap source; the DB is the runtime source of truth (see 4.3), so ops can add a wallet subcategory without a deploy.

### 4.3 Storage

| Change | Why |
|--------|-----|
| New `product_categories (code, name, sort_order)`, seeded `bags`, `wallets` | Stops free-text drift; enables a category filter UI |
| New `product_taxonomy_terms (category, dimension, code, name, sort_order)`, PK `(category, dimension, code)` | One table for every category/dimension pair instead of a table per bag dimension |
| Backfill from `bag_subcategories` → `(bags, subCategory, …)`; `bag_styles` / `bag_targets` → `(*, style/target, …)` with `*` meaning shared | Preserves existing rows and admin edits |
| Keep `bag_*` tables until Phase 4, then drop | Rollback safety |
| Product columns unchanged (`sub_category`, `bag_style`, `target`) | They are generic slots already; renaming `bag_style` → `style_term` is optional hygiene |
| Add `category` to the `styles` master (SKU design family) | Prevents attaching a bag design family to a wallet product; warn first, enforce later |

Under the flat sellable model, `product_styles` carries `category` too — the grouping entity is per category by construction.

### 4.4 Wallet-specific product details

Wallets have specs bags don't (card slots, coin pocket, closure). Do **not** add nullable wallet columns to `products`.

| Need | Mechanism | When |
|------|-----------|------|
| Display-only PDP specs | Existing `attributes` string map ([product-attributes.md](product-attributes.md)) | **Now** — zero schema work |
| Filterable / sortable spec | Typed `specs JSONB` + per-category schema + GIN index | Only when merchandising asks to filter on it |
| Deeply divergent category | Extension table `wallet_products(product_id, …)` | Not foreseen; keep as escape hatch |

`capacity` stays a bag-flavored optional column; wallets leave it empty rather than overloading it.

### 4.5 API

| Endpoint | Change |
|----------|--------|
| `GET /products/catalog/master?category=wallets` | Returns that category's dimensions; `category` defaults to `bags` for back-compat |
| `GET /products/catalog/subcategories?category=wallets` | Category-scoped list |
| `GET /products/catalog/bag-styles` | Legacy alias → shared style terms; new canonical `…/catalog/styles` conflicts with the SKU-master route `…/catalog/brands/{code}/styles`, so use `…/catalog/looks` (or keep the alias only) |
| `GET /products/catalog/targets` | Unchanged (shared) |
| `GET /products/catalog/categories` | New — list categories for the storefront nav |
| `GET /products?category=wallets&subcategory=bifold` | Works once terms are category-scoped |
| `GET /products?subcategory=bifold` (no category) | `400` — a category-scoped filter without a category is ambiguous. Shared dimensions (`target`, `brand`, `color`, `material`, price) stay usable cross-category |
| `POST /products` | `category` becomes **required** for new products and must exist in the master; existing rows are grandfathered |

Recommendations already filter to the seed's category, so wallets won't leak into bag rails — worth an explicit test rather than new logic.

---

## 5. Naming hazard to fix while we're here

Adding a category multiplies an existing collision:

| Name | Meaning |
|------|---------|
| `Product.StyleCode` | SKU design family (`CAS001`) |
| `domain.Style` | Master dictionary entry for the above |
| `Product.Style` | Merchandising look (`casual`) |
| `bag_styles` table | Terms for the look |

Recommended hygiene (optional, cheap): rename the Go field `Product.Style` → `Product.Look` keeping `json:"style"`, and name the new dimension `look` internally. Public API unchanged; the four-way ambiguity drops to two.

---

## 6. Phases

**Phase 1 — unblock.** Category master + registry + category-aware `NormalizeProductTaxonomy`; `product_taxonomy_terms` with backfill; seed wallet subcategories. Wallet creation starts working.

**Phase 2 — API.** `?category=` on catalog reads, `/catalog/categories`, category required on create, category-scoped search validation. Legacy bag endpoints keep answering as `category=bags`.

**Phase 3 — merchandising.** Wallet PDP specs via `attributes`; storefront nav and filter UI per category; recommendation isolation test.

**Phase 4 — cleanup.** Drop `bag_*` tables, optionally rename `bag_style` column and the `Style` → `Look` field, add `category` to the `styles` master.

Phases 1–2 are the launch path for wallets; 3–4 can trail.

---

## 7. Explicitly rejected

| Idea | Why not |
|------|---------|
| `type Wallet struct { Product }` | Empty embedding solves nothing — the existing `Bag` / `Shoes` stubs prove it |
| Separate `wallets` table or wallet service | Same lifecycle, same SKU rules, same stock and checkout; splitting duplicates all of it |
| Per-category Go interfaces at store/handler boundaries | Turns one query into N; search must stay category-agnostic |
| Nullable wallet columns on `products` | Column sprawl per category; `attributes` / `specs` already cover it |
| A wallet-specific SKU format | Segment codes are shared; wallets are usually size `OS` |

---

## 8. Checklist

- [ ] `product_categories` master seeded (`bags`, `wallets`); `category` validated and required on create
- [ ] `product_taxonomy_terms` created and backfilled from `bag_*`
- [ ] Category registry in `domain`; `NormalizeProductTaxonomy` scoped by category
- [ ] Wallet subcategory seeds (bifold, trifold, card holder, zip-around, long, coin purse)
- [ ] `?category=` on catalog endpoints; `/catalog/categories`; legacy bag routes aliased
- [ ] Category-scoped search validation; shared dimensions still cross-category
- [ ] Recommendations isolation test (wallet seed returns no bags)
- [ ] Wallet PDP specs via `attributes`; decide later on typed `specs`
- [ ] Optional: `Product.Style` → `Look`, `category` on the `styles` master, drop `bag_*`
