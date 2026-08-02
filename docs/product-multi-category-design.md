# Type design for multiple product categories (bags, wallets, clothing)

**Status:** Design plan (no code change yet).  
**Question:** Wallets and clothing are scheduled. How should the product types be designed?  
**Related:** [product-multi-category-naming-plan.md](product-multi-category-naming-plan.md) (why types stay category-agnostic), [product-flat-sellable-model-plan.md](product-flat-sellable-model-plan.md) (Product as the sellable unit), [product-master-catalog.md](product-master-catalog.md), [product-sku-system.md](product-sku-system.md).

---

## 1. Answer in one paragraph

Do **not** add `Wallet` / `Clothing` Go types. Keep one `Product` type and make **category a first-class master** that carries its own taxonomy and its own **facet spec**. Category-specific fields (bag `capacity`, clothing `fit` / `fabric`, wallet `cardSlots`) become **validated, filterable facets** stored in a single `details` JSONB column, described by data rather than by Go structs. Adding a category then means seeding a spec, not writing code.

---

## 2. What actually blocks wallets and clothing today

Three concrete blockers, all in the taxonomy layer — the rest of the service is already category-agnostic.

| Blocker | Where | Effect on a wallet/clothing row |
|---------|-------|-------------------------------|
| `category` is unvalidated free text | no master table; `products.category TEXT` | `category: "clothng"` is accepted silently; no list of valid categories exists |
| Taxonomy validates against **bag seeds only** | `domain.NormalizeProductTaxonomy` → `SeedSubCategories` / `SeedBagStyles` | `subCategory: "cardholder"` or `"tshirt"` is rejected with `400`, regardless of category |
| Category-specific fields have nowhere to go | `Product.Capacity` (bag-ish); `attributes` is free-form and unsearchable by design ([product-attributes.md](product-attributes.md)) | Clothing `fit` / `fabric` can only go into `attributes`, so they cannot be filtered or validated |

Secondary, smaller items: the `sizes` master is one flat global list (`OS`, `MIN`/`MED`/`LRG`, `XS`–`XL`) with no notion of which sizes apply to which category; `handler.searchFilters` is a fixed bag-shaped list; and the public routes are bag-named (`/catalog/bag-styles`).

Already fine, no change needed: SKU composition (`Brand_Style_Color[_Edition]_Size` works for any category), stock/reservations, cart, order, payment, images, coupons, wishlist, views, and same-category recommendations.

---

## 3. Design

### 3.1 Shape

```text
Category (master: bags | wallets | clothing)
  ├── taxonomy terms   subCategory / style(occasion) / target   ← scoped per category
  ├── facet spec       capacity | fit | fabric | cardSlots …    ← typed, validated, filterable
  └── size group       one-size | apparel-alpha | apparel-numeric
        │
        ▼
Style (brandCode + styleCode)   ← shared design copy
        │
        ▼
Product (sellable)              ← one Go type for every category
   shared columns  +  details JSONB (facet values for its category)
```

### 3.2 Types

```go
// Unchanged in spirit: one Product for every category.
type Product struct {
	// … identity, sku, price, status, images, counts (see flat-sellable plan) …

	Category    string `json:"category"`              // FK → categories.code
	SubCategory string `json:"subCategory,omitempty"` // scoped to Category
	Style       string `json:"style,omitempty"`       // occasion, scoped to Category
	Target      string `json:"target,omitempty"`      // audience, shared

	// Category-specific, validated facets (bag capacity, clothing fit, …).
	Details map[string]string `json:"details,omitempty"`

	// Free-form PDP memo — unchanged, still not searched.
	Attributes map[string]string `json:"attributes,omitempty"`
}
```

The category spec is **data**, not code:

```go
type FacetKind string // "enum" | "text" | "int"

type Facet struct {
	Code       string        `json:"code"`            // "fit", "capacity", "cardSlots"
	Name       string        `json:"name"`            // "Fit"
	Kind       FacetKind     `json:"kind"`
	Terms      []CatalogTerm `json:"terms,omitempty"` // enum only
	Filterable bool          `json:"filterable"`      // exposed as a ?query param
	Required   bool          `json:"required"`
	Scope      FacetScope    `json:"scope"`           // "style" | "product"
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

`Scope` matters once products are the sellable unit: `fit` and `fabric` are the same for every color of a style (**style scope**, fan-out on edit), while a per-color facet would be **product scope**. See [product-flat-sellable-model-plan.md](product-flat-sellable-model-plan.md) §4.2.

### 3.3 Why `details` JSONB rather than columns or Go structs

| Approach | Verdict |
|----------|---------|
| `details` JSONB + spec validation | **Recommended.** New category = seed rows. GIN-indexable for equality filters; validated on write, so it is not a free-for-all |
| One nullable column per facet | Rejected initially — `products` grows a column per category quirk. Promote an individual facet to a real column later *if* it needs heavy indexed sorting |
| Per-category Go structs / extension tables | Rejected — see [product-multi-category-naming-plan.md](product-multi-category-naming-plan.md); the unused `Bag`/`Shoes`/`Outerwear` stubs are what this looks like when it fails |
| Dump into existing `attributes` | Rejected — `attributes` is deliberately unvalidated and unsearchable |

---

## 4. Storage

| Table | Purpose |
|-------|---------|
| `categories` | `code` PK, `name`, `size_group`, `position`, `active` |
| `category_terms` | `(category_code, dimension, code)` PK, `name` — `dimension` ∈ `subcategory` / `style` / `target`. Replaces `bag_subcategories` / `bag_styles` / `bag_targets` |
| `category_facets` | `(category_code, code)` PK, `name`, `kind`, `filterable`, `required`, `scope` |
| `category_facet_terms` | `(category_code, facet_code, code)` PK, `name` — enum values |
| `products.category` | FK → `categories(code)` (`RESTRICT`, matching how master deletes already behave) |
| `products.details` | `JSONB NOT NULL DEFAULT '{}'` + GIN index |
| `sizes.group` | Which size group a size code belongs to (`one-size`, `apparel-alpha`, `apparel-numeric`) |

Bag seeds migrate into `categories` + `category_terms` unchanged, so current behavior is preserved exactly.

### Seeds for the new categories

| Category | subCategory | style (occasion) | Facets | Size group |
|----------|-------------|------------------|--------|------------|
| `bags` (existing) | handbags, tote, shoulder, cross, mini | casual, evening, business, weekend, statement | `capacity` (enum, from today's column) | one-size |
| `wallets` | bifold, trifold, cardholder, zip-around, long | casual, business, gift | `cardSlots` (int), `coinPocket` (enum yes/no), `closure` (enum) | one-size |
| `clothing` | tshirt, shirt, knit, outer, pants, skirt, dress | casual, business, evening, sport | `fit` (enum slim/regular/oversized), `fabric` (enum), `sleeve` (enum), `season` (enum) | apparel-alpha |

Clothing is the category that finally exercises per-size stock. Under the flat sellable model each size+color is its own product with its own stock row, which is exactly what apparel needs.

---

## 5. API

| Endpoint | Change |
|----------|--------|
| `GET /products/catalog/categories` | **New** — `[{ code, name, sizeGroup }]` |
| `GET /products/catalog/master?category=clothing` | Returns that category's `subCategories`, `styles`, `targets`, **and** `facets`. Omitting `category` keeps today's bag response |
| `GET /products/catalog/subcategories`, `/bag-styles`, `/targets` | Kept as bag-scoped legacy aliases; add `?category=` support, deprecate the bag-named path |
| `GET /products?category=clothing&fit=slim&fabric=cotton` | Facet filters accepted when `filterable` and the category is given. Unknown facet or term → `400`; facet without `category` → `400` |
| `POST` / `PUT /products` | `category` must exist; `subCategory` / `style` / `target` validated against **that category's** terms; `details` validated against its facet spec (unknown key, bad enum, or missing required → `400`) |
| Facet master CRUD | Under `/products/catalog/categories/{code}/facets`, reusing `product.master.read` / `product.master.write` |

Worked examples:

```jsonc
// clothing
{ "category": "clothing", "subCategory": "tshirt", "style": "casual", "target": "men",
  "size": "L", "color": "Black",
  "details": { "fit": "slim", "fabric": "cotton", "sleeve": "short", "season": "ss" } }

// wallet
{ "category": "wallets", "subCategory": "cardholder", "style": "business", "target": "all",
  "details": { "cardSlots": "6", "coinPocket": "no", "closure": "snap" } }
```

Validation moves from a bag-hardcoded function to a spec-driven one:

```go
// today
func NormalizeProductTaxonomy(p *Product) error

// proposed
func NormalizeProductTaxonomy(spec CategorySpec, p *Product) error
func ValidateDetails(spec CategorySpec, details map[string]string) (map[string]string, error)
```

---

## 6. Phases

Each phase is additive and leaves bag behavior identical until Phase 5.

1. **Category master.** `categories` table + seed `bags`; validate `products.category` on write; `GET /catalog/categories`.
2. **Scoped taxonomy.** `category_terms` (backfilled from the three bag tables); spec-driven `NormalizeProductTaxonomy`; `?category=` on master endpoints. Bag responses byte-identical.
3. **Facets.** `category_facets` / `category_facet_terms`, `products.details` JSONB + GIN, write validation, filterable facets in search. Migrate `capacity` into a bag facet (keep the column until frontends move).
4. **Size groups.** `sizes.group`; validate a product's size code against its category's group.
5. **Seed wallets + clothing.** Spec rows only — no schema or Go changes if 1–4 landed. First non-bag products can be created here.
6. **Cleanup.** Drop bag-named routes and the `capacity` column once `dupli1-manage-web` / `dupli1-web` migrate.

### Sequencing against the flat sellable model

Phases 1–2 are small, additive, and touch a different part of the schema than the flat migration, so they can land first and unblock category work early. **Phase 3 should wait** until the flat model's Phase 0 decision is made, because `Facet.Scope` (style vs product) only has meaning once style and sellable rows are separate. If the flat model is deferred, treat every facet as style-scoped.

---

## 7. What this rules out

| Anti-pattern | Why |
|--------------|-----|
| `Wallet{}` / `Clothing{}` Go types | Same failure as the existing unused `Bag`/`Shoes` stubs; forces polymorphism through every store and handler |
| `/api/v1/wallets`, `/api/v1/clothing` routes | Splits search, recommendations, and permissions per category for no gain |
| A separate service per category | Duplicates stock, coupons, images, and auth wiring |
| Category-specific columns added ad hoc | `products` becomes a union of every category's quirks |
| Category-specific behavior in handlers (`if category == "clothing"`) | Behavior belongs in the spec data, not in branches |

---

## 8. Checklist

- [ ] `categories` master + `category` write validation
- [ ] `category_terms` backfilled from bag tables; spec-driven taxonomy validation
- [ ] `?category=` on catalog master endpoints; `GET /catalog/categories`
- [ ] `category_facets` + `products.details` JSONB with validation and filterable search
- [ ] `capacity` migrated to a bag facet
- [ ] Size groups on the `sizes` master
- [ ] Wallet + clothing specs seeded
- [ ] Recommendations verified to stay within a category and score on shared fields only
- [ ] Facet scope (style vs product) reconciled with [product-flat-sellable-model-plan.md](product-flat-sellable-model-plan.md)
- [ ] Bag-named routes and `capacity` column dropped after frontend migration
