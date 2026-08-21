# Type design for multiple product categories (bags, wallets, clothing)

**Status:** Design plan (no code change yet).  
**Schedule:** **Wallets — September 2026**; padded bag-line products (catalog expansion, not a new category) — **October 2026**. See [v1-release-plan.md § Schedule](v1-release-plan.md#schedule).  
**Question:** Wallets and clothing are scheduled. How should the product types be designed?  
**Related:** [product-multi-category-naming-plan.md](product-multi-category-naming-plan.md) (why types stay category-agnostic), [product-flat-sellable-model-plan.md](product-flat-sellable-model-plan.md) (Product as the sellable unit), [product-master-catalog.md](product-master-catalog.md) (bag taxonomy as built), [product-sku-system.md](product-sku-system.md), [product-attributes.md](product-attributes.md).

---

## 1. Answer in one paragraph

Do **not** add `Wallet` / `Clothing` Go types. Keep one `Product` type and make **category a first-class master** that owns its taxonomy terms and a **facet spec**. Category-specific fields (bag `capacity`, wallet `cardSlots`, clothing `fit`) become validated, filterable facets in a single `details` JSONB column, described by data rather than by Go structs. Adding a category then means seeding spec rows, not writing code.

Read §10 before committing to the facet layer: it trades type safety and index quality for flexibility, and for a three-category catalog the cheaper fallback (Phases 1–2 plus a few nullable columns) may be the better buy.

---

## 2. What actually blocks non-bag products today

`NormalizeProductTaxonomy` (`product/pkg/domain/taxonomy.go`) validates `subCategory` / `style` / `target` against hardcoded **bag** seeds without ever reading `p.Category`:

```go
sc, ok := NormalizeSubCategory(p.SubCategory) // looks up SeedSubCategories only
if !ok {
	return fmt.Errorf("invalid subcategory %q", p.SubCategory)
}
```

That single omission cuts both ways:

| Input | Today | Should be |
|-------|-------|-----------|
| `category=wallets`, no `subCategory` | accepted (blank is allowed) | accepted |
| `category=wallets`, `subCategory=bifold` | **`400 invalid subcategory "bifold"`** | accepted |
| `category=wallets`, `subCategory=tote` | **accepted** — a bag term on a wallet | `400` |
| `category=clothing`, `subCategory=tshirt` | **`400`** | accepted |
| `category=wallets`, `style=business`, `target=women` | accepted | accepted (shared dimensions) |

Three gaps behind it:

| Gap | Detail | Consequence |
|-----|--------|-------------|
| **Validation is bag-hardcoded** | `NormalizeProductTaxonomy` ignores `p.Category` | Non-bag terms rejected; bag terms leak onto non-bags |
| **`category` is free text** | `products.category TEXT DEFAULT ''`, no master, no validation | `wallets`, `Wallet`, `wallet` coexist silently |
| **Taxonomy tables are bag-named and not used for validation** | `bag_subcategories` / `bag_styles` / `bag_targets` are seeded and listed, but validation reads Go slices | Adding a DB term doesn't make it valid |

Plus, category-specific fields have nowhere to go: `Product.Capacity` is bag-flavored, and `attributes` is deliberately free-form and unsearchable ([product-attributes.md](product-attributes.md)), so clothing `fit` could never be filtered.

Secondary: the `sizes` master is one flat global list (`OS`, `MIN`/`MED`/`LRG`, `XS`–`XL`) with no notion of which sizes apply where; `handler.searchFilters` is a fixed bag-shaped list; public routes are bag-named (`/catalog/bag-styles`).

Already fine, no change needed: SKU composition (`Brand_Style_Color[_Edition]_Size` suits any category), stock/reservations, cart, order, payment, images, coupons, wishlist, views, and same-category recommendations.

---

## 3. Which dimensions are genuinely category-specific

| Dimension | Field | Bags | Wallets | Clothing | Verdict |
|-----------|-------|------|---------|----------|---------|
| **Sub category** | `subCategory` | handbags, tote, shoulder, cross, mini | bifold, trifold, cardholder, zip-around, long | tshirt, shirt, knit, outer, pants, skirt, dress | **Category-scoped** |
| **Style / look** | `style` | casual, evening, business, weekend, statement | reads naturally as-is | add `sport` | **Shared**, with per-category additions |
| **Target** | `target` | all, men, women, kids | identical | identical | **Shared** |

Only `subCategory` differs fundamentally, which keeps the taxonomy change small. SKU masters need no split — brands, colors, sizes, and editions are already category-agnostic.

---

## 4. Design

### 4.1 Shape

```text
Category (master: bags | wallets | clothing)
  ├── taxonomy terms   subCategory (scoped) / style / target (shared, extensible)
  ├── facet spec       capacity | cardSlots | fit | sleeve …   ← typed, validated, filterable
  └── size group       one-size | apparel-alpha | apparel-numeric
        │
        ▼
Style (brandCode + styleCode, itself category-tagged)
        │
        ▼
Product (sellable)          ← one Go type for every category
   shared columns  +  details JSONB (facet values for its category)
```

### 4.2 Types

```go
// Unchanged in spirit: one Product for every category.
type Product struct {
	// … identity, sku, price, status, images, counts (see flat-sellable plan) …

	Category    string `json:"category"`              // FK → categories.code
	SubCategory string `json:"subCategory,omitempty"` // scoped to Category
	Style       string `json:"style,omitempty"`       // "look" — see §6 naming hazard
	Target      string `json:"target,omitempty"`      // audience, shared

	// Category-specific, validated facets (capacity, fit, cardSlots, …).
	Details map[string]string `json:"details,omitempty"`

	// Free-form PDP memo — unchanged, still not searched.
	Attributes map[string]string `json:"attributes,omitempty"`
}
```

No `Wallet` struct, no `BagProduct` / `WalletProduct` interface, no per-category table, no second service. The unused `Bag` / `Shoes` / `Outerwear` stubs in `products.go` are the pattern to avoid — they never earned a field.

The category spec is **data**, not code:

```go
type FacetKind string  // "enum" | "text" | "int"
type FacetScope string // "style" | "product"

type Facet struct {
	Code       string        `json:"code"`            // "fit", "capacity", "cardSlots"
	Name       string        `json:"name"`
	Kind       FacetKind     `json:"kind"`
	Terms      []CatalogTerm `json:"terms,omitempty"` // enum only
	Filterable bool          `json:"filterable"`      // exposed as a ?query param
	Required   bool          `json:"required"`
	Scope      FacetScope    `json:"scope"`
}

type CategorySpec struct {
	Code          string        `json:"code"` // bags | wallets | clothing
	Name          string        `json:"name"`
	SubCategories []CatalogTerm `json:"subCategories"`
	Styles        []CatalogTerm `json:"styles"`
	Targets       []CatalogTerm `json:"targets"`
	Facets        []Facet       `json:"facets"`
	SizeGroup     string        `json:"sizeGroup"`
}
```

Validation becomes spec-driven, and error messages name the category (`invalid subcategory "tote" for category "wallets"`):

```go
// today
func NormalizeProductTaxonomy(p *Product) error

// proposed
func NormalizeProductTaxonomy(spec CategorySpec, p *Product) error
func ValidateDetails(spec CategorySpec, details map[string]string) (map[string]string, error)
```

Go seeds remain the bootstrap source; the DB is the runtime source of truth, so ops can add a wallet subcategory without a deploy.

`Scope` matters once products are the sellable unit: `fit` and `sleeve` are identical for every color of a style (**style scope**, fan-out on edit), while a per-color facet would be **product scope**. See [product-flat-sellable-model-plan.md](product-flat-sellable-model-plan.md) §4.2.

### 4.3 Why `details` JSONB rather than columns or Go structs

| Approach | Verdict |
|----------|---------|
| `details` JSONB + spec validation | **Recommended.** New category = seed rows. GIN-indexable for equality filters; validated on write, so not a free-for-all |
| One nullable column per facet | Rejected initially — `products` becomes a union of every category's quirks. Promote a single facet to a real column later *if* it needs heavy indexed sorting |
| Per-category Go structs / extension tables | Rejected — see [product-multi-category-naming-plan.md](product-multi-category-naming-plan.md). Keep the extension table as an escape hatch only if a category diverges deeply (not foreseen) |
| Dump into existing `attributes` | Fine for display-only PDP specs **now**, with zero schema work; not acceptable for anything filterable |

Each row above has a real cost; §10 spells out what this choice gives up and when to prefer plain columns instead.

---

## 5. Storage

| Table | Purpose |
|-------|---------|
| `categories` | `code` PK, `name`, `size_group`, `sort_order`, `active` |
| `category_terms` | `(category_code, dimension, code)` PK, `name`, `sort_order`. `dimension` ∈ `subcategory` / `style` / `target`; `category_code = '*'` means shared. Replaces `bag_subcategories` / `bag_styles` / `bag_targets` |
| `category_facets` | `(category_code, code)` PK, `name`, `kind`, `filterable`, `required`, `scope` |
| `category_facet_terms` | `(category_code, facet_code, code)` PK, `name` — enum values |
| `products.category` | FK → `categories(code)` (`RESTRICT`, matching existing master-delete behavior) |
| `products.details` | `JSONB NOT NULL DEFAULT '{}'` + GIN index |
| `styles.category` | Prevents attaching a bag design family to a wallet; warn first, enforce later. Under the flat model, `product_styles` carries it too |
| `sizes.group` | Which size group a size code belongs to (`one-size`, `apparel-alpha`, `apparel-numeric`) |

Bag rows migrate into `categories` + `category_terms` unchanged, preserving current behavior and any admin edits. Keep the `bag_*` tables until the cleanup phase for rollback safety.

### Seeds for the new categories

| Category | subCategory | style (look) | Facets | Size group |
|----------|-------------|--------------|--------|------------|
| `bags` (existing) | handbags, tote, shoulder, cross, mini | casual, evening, business, weekend, statement | `capacity` (enum, migrated from the column) | one-size |
| `wallets` | bifold, trifold, cardholder, zip-around, long, coin-purse | casual, business, gift | `cardSlots` (int), `coinPocket` (enum), `closure` (enum) | one-size |
| `clothing` | tshirt, shirt, knit, outer, pants, skirt, dress | casual, business, evening, sport | `fit` (enum), `sleeve` (enum), `season` (enum) | apparel-alpha |

Fabric is **not** a facet: `material` is already a first-class column and filter. Facets must never restate an existing column — see §10.

Clothing is the category that finally exercises per-size stock. Under the flat sellable model each size+color is its own product with its own stock row — exactly what apparel needs. Wallets are usually size `OS` and need no SKU format change.

---

## 6. Naming hazard to fix while we're here

Adding categories multiplies an existing collision:

| Name | Meaning |
|------|---------|
| `Product.StyleCode` | SKU design family (`CAS001`) |
| `domain.Style` | Master dictionary entry for the above |
| `Product.Style` | Merchandising look (`casual`) |
| `bag_styles` table | Terms for the look |

Cheap hygiene: rename the Go field `Product.Style` → `Product.Look` (keeping `json:"style"`) and call the dimension `look` internally. Public API unchanged; four-way ambiguity drops to two. This also resolves a route conflict — a canonical `…/catalog/styles` would collide with the SKU-master route `…/catalog/brands/{code}/styles`, so the look endpoint should be `…/catalog/looks` (or keep only the legacy alias).

---

## 7. API

| Endpoint | Change |
|----------|--------|
| `GET /products/catalog/categories` | **New** — `[{ code, name, sizeGroup }]` for storefront nav |
| `GET /products/catalog/master?category=clothing` | Returns that category's `subCategories`, `styles`, `targets`, **and** `facets`. `category` defaults to `bags` for back-compat |
| `GET /products/catalog/subcategories?category=wallets` | Category-scoped list |
| `GET /products/catalog/bag-styles` | Legacy alias → shared look terms; canonical `…/catalog/looks` (see §6) |
| `GET /products/catalog/targets` | Unchanged (shared) |
| `GET /products?category=clothing&material=cotton&fit=slim` | Facet filters accepted when `filterable` and a `category` is given; shared columns like `material` keep working as today |
| `GET /products?subcategory=bifold` (no category) | `400` — a category-scoped filter without a category is ambiguous. Shared filters (`target`, `brand`, `color`, `material`, price) stay cross-category |
| Unknown facet / term | `400` naming the category |
| `POST` / `PUT /products` | `category` required for new products and must exist; taxonomy validated against that category's terms; `details` validated against its facet spec. Existing rows grandfathered |
| Facet + term master CRUD | `/products/catalog/categories/{code}/facets`, reusing `product.master.read` / `product.master.write` |

Worked examples:

```jsonc
// clothing
{ "category": "clothing", "subCategory": "tshirt", "style": "casual", "target": "men",
  "size": "L", "color": "Black", "material": "cotton",
  "details": { "fit": "slim", "sleeve": "short", "season": "ss" } }

// wallet
{ "category": "wallets", "subCategory": "cardholder", "style": "business", "target": "all",
  "details": { "cardSlots": "6", "coinPocket": "no", "closure": "snap" } }
```

Recommendations already filter to the seed's category, so wallets won't leak into bag rails — worth an explicit test rather than new logic.

---

## 8. Phases

Each phase is additive and leaves bag behavior identical until new categories are seeded.

1. **Category master.** `categories` table seeded with `bags`; validate `products.category` on write; `GET /catalog/categories`.
2. **Scoped taxonomy.** `category_terms` backfilled from the three `bag_*` tables (`*` for shared dimensions); spec-driven `NormalizeProductTaxonomy`; `?category=` on catalog reads. Bag responses stay byte-identical. **Wallets become creatable here.**
3. **Facets.** `category_facets` / `category_facet_terms`, `products.details` JSONB + GIN, write validation, filterable facets in search. Migrate `capacity` into a bag facet (keep the column until frontends move). **Optional** — see §10.8 for the nullable-column fallback and the triggers for adopting this phase.
4. **Size groups.** `sizes.group`; validate a product's size code against its category's group. **Clothing needs this.**
5. **Seed wallets + clothing.** Spec rows only — no schema or Go changes if 1–4 landed.
6. **Cleanup.** Drop `bag_*` tables and bag-named routes; drop the `capacity` column; optional `Style` → `Look` rename; enforce `styles.category`.

Wallets can ship after Phase 2 with display-only specs in `attributes`; clothing realistically wants Phases 3–4 first. Target: **wallet category live in September 2026** (phases 1–2). **Padded** products in **October 2026** are a bag-line catalog seed (existing `bags` category / style masters), not a new multi-category phase.

### Sequencing against the flat sellable model

Phases 1–2 are small, additive, and touch a different part of the schema than the flat migration, so they can land first and unblock category work early. **Phase 3 should wait** for the flat model's Phase 0 decision, because `Facet.Scope` (style vs product) only has meaning once style and sellable rows are separate. If the flat model is deferred, treat every facet as style-scoped.

---

## 9. Explicitly rejected

| Idea | Why not |
|------|---------|
| `type Wallet struct { Product }` | Empty embedding solves nothing — the existing `Bag` / `Shoes` stubs prove it |
| `/api/v1/wallets`, `/api/v1/clothing` routes | Splits search, recommendations, and permissions per category for no gain |
| A separate table or service per category | Same lifecycle, SKU rules, stock, and checkout; splitting duplicates all of it |
| Per-category Go interfaces at store/handler boundaries | Turns one query into N; search must stay category-agnostic |
| Nullable per-category columns on `products` | Column sprawl per category; `details` covers it |
| Category branches in handlers (`if category == "clothing"`) | Behavior belongs in spec data, not in code paths |
| A category-specific SKU format | Segment codes are already shared |

---

## 10. Downsides, and when this is the wrong design

This design buys flexibility with three currencies: type safety, index quality, and operational surface. The costs are real and worth stating plainly.

### 10.1 `details` JSONB gives up what columns give for free

| Lost | Detail | Mitigation |
|------|--------|------------|
| **Type safety** | Everything is `map[string]string`. `cardSlots: "6"` is text; the Go compiler cannot help, and a typo in a facet code surfaces as a `400` in production rather than a build failure | Spec validation on write; contract tests per category spec |
| **Range and sort queries** | GIN on JSONB serves containment (equality) only. "6–12 card slots" or `sort=cardSlots` needs a cast plus a per-facet expression index — exactly the per-facet schema work the design claims to avoid | **Promotion path:** when a facet needs range/sort, give it a real column and keep the spec entry as metadata |
| **Referential integrity** | Values in `details` are not FK-checked against `category_facet_terms`. A direct SQL write or a bad migration creates orphan values; renaming or deleting a term does not cascade | App-level validation on every write; a "term in use" check before delete (same pattern as existing master `RESTRICT`); a periodic orphan report |
| **Query planning** | Planner estimates on JSONB containment are poor, so plans can degrade as the catalog grows. Facet **counts** ("Slim (12)") are far more expensive than a btree-indexed column | Fine below roughly 10⁵ products; revisit with promotion or a search index if facet counts are ever required |

### 10.2 Validation-as-data moves failures from compile time to runtime

- **Spec changes can invalidate existing rows.** Turning a facet required, or deleting an enum term, leaves products holding now-illegal values. The rule has to be: **validate on write, stay lenient on read** — a PDP must never 500 because a spec changed. That rule is easy to state and easy to forget.
- **Specs are read on every product write and every faceted search**, so they need an in-process cache with invalidation on master edits. That is a small amount of code with a classic staleness bug waiting in it (edit a term, keep getting `400` from a stale cache on another instance).
- **Error quality degrades** unless deliberately built: generic "invalid details" messages are much less useful than today's typed field errors.

### 10.3 The "no code to add a category" promise is only half true

It holds **after** Phase 3 *and* after someone builds the spec editor in `dupli1-manage-web`. That admin UI does not exist — the equivalent for SKU masters (Phase D) is still open in [TODO.md](TODO.md). Until then, adding a category means writing seeds and deploying, which is what the design set out to avoid. The honest sequencing is: Phases 1–2 pay off immediately, Phase 3 only pays off once the UI exists.

### 10.4 Governance: easy facets invite taxonomy sprawl

Once merchandisers can create facets, they will create near-duplicates. My own first draft of the seed table did exactly this — it proposed a clothing `fabric` facet when `material` is already a first-class column and filter. Rules needed, and someone to enforce them:

- A facet must never restate an existing column (`material`, `color`, `size`, `brand`, `capacity`).
- Term codes are immutable; names are editable (same rule as SKU masters).
- Unused facets get pruned, or the filter sidebar rots.

There is also a live ambiguity for admins: `attributes` (free-form memo) and `details` (validated facets) are two string maps on the same object, and the difference is a convention rather than a shape. Expect misuse in both directions.

### 10.5 Cross-category search gets weaker, not stronger

Facet filters require a `category`, so "all black leather items under ₩500k" can only use shared columns. If the storefront later wants global filters over category-specific concepts, the per-category model actively works against it and the answer becomes promoting facets to shared columns — partially undoing the design. Recommendations have the same limitation: scoring stays on shared fields unless the recommender learns to read specs, so clothing similarity will be cruder than bag similarity is today.

### 10.6 Migration risk on live data, stacked with another migration

- `products.category` becoming a validated FK requires cleaning existing free-text values first; blank categories in production must be assigned or explicitly allowed.
- Moving `capacity` to a facet needs a dual-read window because frontends read `capacity` today.
- This lands on the same table as [product-flat-sellable-model-plan.md](product-flat-sellable-model-plan.md). Two large migrations queued on `products` interleave badly; each one's rollback story gets worse if they overlap. Sequencing is not optional here.

### 10.7 Security note

Dynamic facet filters replace today's fixed `searchFilters` allowlist. Filter keys must be resolved **against the spec** and mapped to parameterized JSONB predicates — never interpolated into SQL from the query string. This is easy to get right and easy to get wrong in a hurry.

### 10.8 The strongest counterargument: this may be over-built

If the catalog ends at three categories with about four stable facets each, then eight nullable columns plus a `switch` in validation would be simpler, fully typed, fully indexable, and a fraction of the code. The spec engine (four tables, a validation layer, a dynamic query builder, an admin UI) is a real investment justified by *expected churn*, not by three categories.

**Cheaper fallback, worth taking seriously:** do Phases 1–2 only — category master plus category-scoped taxonomy — and add category-specific fields as **plain nullable columns** as they come up. That unblocks wallets and clothing, fixes the validation bug, and defers the entire facet machinery. Adopt Phase 3 when one of these is true:

- a fourth category is scheduled, or
- non-engineers need to add filterable fields without a deploy, or
- the nullable-column count on `products` passes roughly a dozen.

Until then, the simpler path is defensible and reversible; the facet spec can be introduced later without redoing Phases 1–2.

---

## 11. Checklist

- [ ] `categories` master seeded; `category` validated and required on create
- [ ] `category_terms` created and backfilled from `bag_*` (shared dimensions as `*`)
- [ ] Spec-driven `NormalizeProductTaxonomy`; errors name the category
- [ ] `?category=` on catalog endpoints; `GET /catalog/categories`; legacy bag routes aliased
- [ ] Category-scoped search validation; scoped filter without `category` → `400`
- [ ] Decide facets vs nullable columns (§10.8) before starting Phase 3
- [ ] `category_facets` + `products.details` JSONB with validation and filterable search
- [ ] Facet governance rules written down (no facet may restate a column; immutable codes)
- [ ] Filter keys resolved from the spec into parameterized predicates, never interpolated (§10.7)
- [ ] `capacity` migrated to a bag facet
- [ ] Size groups on the `sizes` master
- [ ] Wallet + clothing specs seeded
- [ ] Recommendations isolation test (wallet seed returns no bags)
- [ ] Facet scope reconciled with [product-flat-sellable-model-plan.md](product-flat-sellable-model-plan.md)
- [ ] Cleanup: drop `bag_*` and bag-named routes, `capacity` column, optional `Style` → `Look`, enforce `styles.category`
