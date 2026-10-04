# Product master catalog (merchandising taxonomy)

**Status:** Implemented.  
**Related:** [product-rich-search.md](product-rich-search.md), [product-sku-system.md](product-sku-system.md), [api.md](api.md).

## Intent

Storefront bag filters need a fixed **master catalog** independent of SKU segment masters (`brands` / design-family `styles` / `colors`):

| Dimension | Product field | Query param | Codes |
|-----------|---------------|-------------|-------|
| Category | `category` | `category` | `bags`, `clothing` |
| Sub category (per category) | `subCategory` | `subcategory` (alias `subCategory`) | bags: `handbags`, `tote`, `shoulder`, `cross`, `mini` · clothing: `padded` |
| Style (occasion / look) | `style` | `style` | `casual`, `evening`, `business`, `weekend`, `statement` |
| Target (audience) | `target` | `target` | `all`, `men`, `women`, `kids` |

`style` here is **not** SKU `styleCode` (design family under a brand).

Categories and their subcategories are Go seeds (`domain.SeedCategories`); adding a category there is what makes its products creatable. Style and target are shared by every category. See [product-multi-category-design.md](product-multi-category-design.md).

## Master catalog APIs (public)

| Method | Path | Response |
|--------|------|----------|
| `GET` | `/api/v1/products/catalog/categories` | `[{ code, name, subCategories }]` — every category, storefront order |
| `GET` | `/api/v1/products/catalog/master?category=` | `{ subCategories, styles, targets }` each `{ code, name }[]`; `category` defaults to `bags`, unknown → `400` |
| `GET` | `/api/v1/products/catalog/subcategories?category=` | that category's subcategory terms (default `bags`) |
| `GET` | `/api/v1/products/catalog/bag-styles` | bag style terms |
| `GET` | `/api/v1/products/catalog/targets` | target terms |

Legacy aliases: `/api/v1/catalog/master`, `/api/v1/catalog/subcategories`, `/api/v1/catalog/bag-styles`, `/api/v1/catalog/targets`.

Bag terms are seeded into Postgres tables `bag_subcategories`, `bag_styles`, `bag_targets` on migrate and served from there, so bag responses are unchanged; other categories are served from their Go seeds. Codes are lowercase; create/update normalize case and reject unknown values (`400`).

## Product search

Example:

```http
GET /api/v1/products?category=bags&subcategory=tote&style=casual&target=women
```

Filters are exact match on the normalized codes stored on the parent product.

**Note:** `target=all` means products tagged with audience code `all`, not “any target.” To leave audience unfiltered, omit the `target` query param.

## Product create / update

Optional JSON fields on parent. `category` must be a known category (code or name, any case); `subCategory` must belong to it, so `tote` on `clothing` or `padded` on `bags` is `400 invalid subcategory "…" for category "…"`. A blank category is validated as `bags` and stored blank. A row that already carries an unknown free-text category (written before categories were checked) keeps it on update, but no product can be given a new unknown one.

```json
{
  "category": "bags",
  "subCategory": "tote",
  "style": "casual",
  "target": "women"
}
```

## Checklist

- [x] Seeded taxonomy masters + public list/master endpoints
- [x] Product fields `subCategory` / `style` / `target`
- [x] `GET /products` query filters
- [x] Validation on create/update
- [x] Docs + OpenAPI notes
- [x] Categories (`bags`, `clothing`) with category-scoped subcategories
