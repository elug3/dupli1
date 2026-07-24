# Product bag master catalog (merchandising taxonomy)

**Status:** Implemented.  
**Related:** [product-rich-search.md](product-rich-search.md), [product-sku-system.md](product-sku-system.md), [api.md](api.md).

## Intent

Storefront bag filters need a fixed **master catalog** independent of SKU segment masters (`brands` / design-family `styles` / `colors`):

| Dimension | Product field | Query param | Codes |
|-----------|---------------|-------------|-------|
| Sub category (bag type) | `subCategory` | `subcategory` (alias `subCategory`) | `handbags`, `tote`, `shoulder`, `cross`, `mini` |
| Style (occasion / look) | `style` | `style` | `casual`, `evening`, `business`, `weekend`, `statement` |
| Target (audience) | `target` | `target` | `men`, `women`, `kids` |

`style` here is **not** SKU `styleCode` (design family under a brand).

## Master catalog APIs (public)

| Method | Path | Response |
|--------|------|----------|
| `GET` | `/api/v1/products/catalog/master` | `{ subCategories, styles, targets }` each `{ code, name }[]` |
| `GET` | `/api/v1/products/catalog/subcategories` | subcategory terms |
| `GET` | `/api/v1/products/catalog/bag-styles` | bag style terms |
| `GET` | `/api/v1/products/catalog/targets` | target terms |

Legacy aliases: `/api/v1/catalog/master`, `/api/v1/catalog/subcategories`, `/api/v1/catalog/bag-styles`, `/api/v1/catalog/targets`.

Seeded into Postgres tables `bag_subcategories`, `bag_styles`, `bag_targets` on migrate. Codes are lowercase; create/update normalize case and reject unknown values (`400`).

## Product search

Example:

```http
GET /api/v1/products?category=bags&subcategory=tote&style=casual&target=women
```

Filters are exact match on the normalized codes stored on the parent product.

## Product create / update

Optional JSON fields on parent:

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
