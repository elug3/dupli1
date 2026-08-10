# Reflection: Product naming for multi-category catalog

**Status:** Reflection / decision plan (no code change yet).  
**Question:** Other product types may arrive later (wallets, clothing, …). Today the catalog is bag-shaped. Can we rename `Product{}` → `Bag{}` and `SKU{}` → `BagSku{}`?  
**Related:** [product-sale-unit-reflection.md](product-sale-unit-reflection.md), [product-variants-plan.md](product-variants-plan.md), [product-sku-system.md](product-sku-system.md), [product-master-catalog.md](product-master-catalog.md), [current-state.md](current-state.md).

## Short answer

**No — do not rename `Product` → `Bag` (or invent `BagSku`).** Keep `Product` / `Variant` as category-agnostic catalog types. Make bag-ness a *category* (and optional bag-only fields), not the core type name.

When wallets/clothing land, a bag-named core type would force a second rename (or awkward `Bag`-that-means-wallet). The cost of renaming now is high; the modeling benefit is low.

## What the code actually has today

| Name in Go | Role | Bag-specific? |
|------------|------|---------------|
| `domain.Product` | Parent style / PDP shell | Mostly no — but carries bag taxonomy fields (`subCategory`, `style`, `target`) and `capacity` |
| `domain.Variant` | Sellable option (human `sku` + ULID `skuId`) | No — color/size/edition SKU segments are shared fashion patterns |
| `domain.Bag` / `Shoes` / … | Removed (were unused embeddings of `Product`) | — |
| `SKU{}` type | **Does not exist** | Docs call variants “SKUs”; Go type is `Variant` |

API, DB, and permissions already speak “product”, not “bag”:

- Routes: `/api/v1/products/...`
- Tables: `products`, `product_variants`, …
- Permissions: `product.create`, `product.read`, …
- Downstream (cart/order): resolve variants by `sku` / `skuId`; they do not import `domain.Product`

Bag merchandising is already scoped as *data*, not the type name: `category=bags`, seeded `bag_subcategories` / `bag_styles` / `bag_targets`, and public catalog under `/products/catalog/...` ([product-master-catalog.md](product-master-catalog.md)).

## Why renaming `Product` → `Bag` is the wrong direction

```text
Today (correct abstraction, bag-shaped data)
  Product  ──has many──►  Variant (SKU)
     │
     └── category = "bags" (+ bag taxonomy columns)

Proposed rename (inverts the abstraction)
  Bag  ──has many──►  BagSku
     │
     └── when wallets arrive: rename again, or misuse Bag for non-bags
```

1. **`Product` is the stable cross-service noun.** Search, PDP, wishlist, views, sold count, recommendations, cart enrichment, and permissions all mean “catalog parent,” not “bag.”
2. **Variants are already generic.** Color/size/edition + dual SKU identity are not bag-only. `BagSku` would misname clothing sizes and wallet colors.
3. **Unused category stubs prove the old idea failed.** `Bag` / `Shoes` / … as empty embeddings of `Product` never gained fields or call sites. Embedding alone does not solve multi-category modeling.
4. **Rename blast radius is large for no API win.** Hundreds of Go identifiers, tests, docs, and mental model — while JSON and HTTP would still say `product` unless you also break clients.
5. **Bags already filter via `category`.** Storefront and recs assume `category=bags` ([product-recommendations.md](product-recommendations.md)). That is the extension point.

## Recommended model (when other types arrive)

Keep shared core; attach category-specific shape beside it.

```text
Product (shared parent)
  id, name, brand, material, category, status, price, tags, …
  attributes (free-form PDP memo — already category-agnostic)
  │
  ├── category = "bags"     → bag taxonomy: subCategory / style / target
  ├── category = "wallets"  → wallet taxonomy (later)
  └── category = "clothing" → size charts / gender / etc. (later)
         │
         ▼
Variant (shared sellable SKU) ──► inventory / cart / order by skuId
```

### Principles

| Do | Don’t |
|----|--------|
| Keep `Product` + `Variant` as the core types | Rename core types to `Bag` / `BagSku` |
| Discriminate with `category` (and per-category masters) | One Go type per merchandise class at the store/API boundary |
| Put bag-only masters under bag-named tables/APIs (already done) | Put bag-only columns into a type named as if it were universal without documenting them as bag taxonomy |
| Use `attributes` or a future `BagDetails` struct for bag-only PDP fields if the shared struct gets noisy | Prematurely split tables/services per category |
| Delete or ignore unused `Shoes`/`Outerwear`/… stubs until a real design exists | Grow empty category wrappers “just in case” |

**Wallets and clothing are the first concrete cases** — the category-scoped design is worked out in [product-multi-category-design.md](product-multi-category-design.md).

### Optional later shapes (pick when needed, not now)

**A. Soft polymorphism (likely enough for v1.x)**  
Keep one `products` table. Bag taxonomy columns stay nullable; other categories leave them empty and use their own masters + filters. Shared search filters on `category` + shared fields.

**B. Side details struct (if bag fields crowd `Product`)**  
```go
type Product struct { /* shared */ Category string; /* … */ }
type BagDetails struct { SubCategory, Style, Target, Capacity string }
// Load BagDetails only when category == "bags"
```
No rename of `Product` / `Variant`.

**C. Hard split (only if categories diverge heavily)**  
`products` core + `bag_products` / `wallet_products` extension tables joined by `product_id`. Still: core type stays `Product`.

## What about `SKU` → `BagSku`?

There is no `SKU` Go type to rename. Options if naming feels unclear:

| Option | Verdict |
|--------|---------|
| Rename `Variant` → `SKU` | Reasonable *later* alias discussion (docs already say SKU); still category-agnostic |
| Introduce `BagSku` | Reject — same problem as `Bag` for the parent |
| Keep `Variant` + fields `SKU` / `SkuID` | **Status quo — preferred** |

Cart/order already treat SKUs as opaque sellable ids. Category-specific SKU *segment masters* (brand/style/color) can stay shared; merchandising taxonomies stay per category.

## If someone still wants a rename: cost sketch

Only useful as a *local* clarity rename inside bag-only packages — not the shared domain types. A full rename would touch:

- `product/pkg/domain`, services, handlers, ports, infra, tests
- Docs (`product-*.md`, API examples)
- Possibly JSON field names / OpenAPI if clients are updated (breaking)
- Confusion with existing unused `domain.Bag`

**Do not do this** unless the product decision is “this service will only ever sell bags forever.” That contradicts the stated roadmap (wallets, clothing).

## Decision

| Proposal | Decision |
|----------|----------|
| Rename `Product{}` → `Bag{}` | **Reject** |
| Rename / introduce `BagSku{}` | **Reject** |
| Keep `Product` + `Variant`; use `category` (+ bag masters) | **Accept (current direction)** |
| Clean up unused `Bag`/`Shoes`/… stubs | **Done** — removed; see [product-structure-final-review.md](product-structure-final-review.md) |
| Multi-category extension plan | Now scheduled (wallets, clothing) — concrete type design in [product-multi-category-design.md](product-multi-category-design.md) |

## Checklist (first non-bag categories: **wallets, clothing**, scheduled — see [product-multi-category-design.md](product-multi-category-design.md))

- [ ] Confirm shared vs category-only fields for the new type
- [ ] Add category masters + search filters (mirror bag taxonomy pattern)
- [ ] Ensure recommendations / search same-category rules still hold
- [ ] Keep cart/order on `skuId` only (no category coupling)
- [ ] Decide soft columns vs side `*Details` vs extension table
- [ ] Do **not** rename `Product` / `Variant`
