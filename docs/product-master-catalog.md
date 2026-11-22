# Product master catalog (merchandising taxonomy)

**Status:** Implemented.  
**Related:** [product-rich-search.md](product-rich-search.md), [product-sku-system.md](product-sku-system.md), [api.md](api.md).

## Intent

Storefront bag filters need a fixed **master catalog** independent of SKU segment masters (`brands` / design-family `styles` / `colors`):

| Dimension | Product field | Query param | Codes |
|-----------|---------------|-------------|-------|
| Category | `category` | `category` | `bags`, `clothing` |
| Sub category (per category) | `subCategory` | `subcategory` (alias `subCategory`) | bags: `handbags`, `tote`, `shoulder`, `cross`, `mini` · clothing: `padded`, `jackets` |
| Style (occasion / look) | `style` | `style` | `casual`, `evening`, `business`, `weekend`, `statement` |
| Target (audience) | `target` | `target` | `all`, `men`, `women`, `kids` |

`style` here is **not** SKU `styleCode` (design family under a brand).

Categories are defined in code (`domain.SeedCategories`); adding one there is what makes its products creatable. Their **subcategories**, and the shared **bag styles** and **targets**, are data staff edit (below). A category may also list its **sizes** (size master codes): `clothing` sells letter sizes `XXS`–`XXXL` and `4XL`, and Italian sizes `34`–`60` in even steps, each in a short, regular and long fit (`48S`, `48`, `48R`, `48L`). Every one of them is seeded into the size master on migrate (`domain.CategorySizes`, named by its code; with the default `OS` these are the only rows the service seeds, `domain.CodeSizes`), so each can be used on a SKU. A jacket SKU in `OS` or a bag capacity size is refused with `400`, and so is moving a product into `clothing` while one of its SKUs has such a size. `bags` lists none and takes any size. Style and target are shared by every category. See [product-multi-category-design.md](product-multi-category-design.md).

## Master catalog APIs (public)

| Method | Path | Response |
|--------|------|----------|
| `GET` | `/api/v1/products/catalog/categories` | `[{ code, name, subCategories, sizes? }]` — every category, storefront order |
| `GET` | `/api/v1/products/catalog/master?category=` | `{ subCategories, styles, targets }` each `{ code, name, sortOrder, category? }[]`; `category` defaults to `bags`, unknown → `400` |
| `GET` | `/api/v1/products/catalog/subcategories?category=` | that category's subcategory terms (default `bags`) |
| `GET` | `/api/v1/products/catalog/bag-styles` | bag style terms |
| `GET` | `/api/v1/products/catalog/targets` | target terms |

Legacy aliases: `/api/v1/catalog/master`, `/api/v1/catalog/subcategories`, `/api/v1/catalog/bag-styles`, `/api/v1/catalog/targets`.

Lists are ordered by `sortOrder`, then code. A subcategory term carries its `category`.

### Editing the taxonomy (`product.master.write`)

Subcategories, bag styles and targets live in Postgres (`bag_subcategories` with a `category` column, `bag_styles`, `bag_targets`). Nothing seeds them at startup: a new environment loads them with `scripts/seed-reference-data.py` (the `subcategories`, `bag_styles` and `targets` lists in `scripts/seed/reference-data.json`). Product create/update and the search filters read the same tables, so a term staff add is usable at once and one they delete is refused on the next write.

| Method | Path | Body | Result |
|--------|------|------|--------|
| `POST` | `/api/v1/products/catalog/subcategories` | `{ code, name, category?, sortOrder? }` | `201` term. `category` is `bags` or `clothing` (default `bags`) |
| `POST` | `/api/v1/products/catalog/bag-styles` · `/targets` | `{ code, name, sortOrder? }` | `201` term; a `category` here is `400` |
| `PATCH` | `/api/v1/products/catalog/{subcategories,bag-styles,targets}/{code}` | `{ name?, sortOrder? }` | `200` term; neither given → `400` |
| `DELETE` | `/api/v1/products/catalog/{subcategories,bag-styles,targets}/{code}` | — | `204`; `409` while a product carries the code |

- Codes are lowercase letters, digits, `-` or `_` (at most 32), normalized to lowercase; anything else is `400`. A taken code is `409`; a **subcategory code is unique across categories**.
- An omitted `sortOrder` puts the term after the last one in its list (for a subcategory, its category's list).
- A term's code never changes, and neither does a subcategory's category: products store the code. Rename with `PATCH`, or add a new term and move products over before deleting the old one.
- Product create/update normalize a code or display name (`Crossbody` → `cross`; the target typo `mem` → `men`) and refuse a value the tables do not hold (`400`).
- Upgrading in place: the migrate that adds `bag_subcategories.category` marks the existing rows `bags` and copies `padded` and `jackets` (the clothing subcategories the code used to serve) once, so existing clothing products stay editable before anyone runs the script.

## Product search

Example:

```http
GET /api/v1/products?category=bags&subcategory=tote&style=casual&target=women
```

Filters are exact match on the normalized codes stored on the parent product.

**Note:** `target=all` means products tagged with audience code `all`, not “any target.” To leave audience unfiltered, omit the `target` query param.

## Size chart

A parent may carry `sizeChart`, the garment measurements behind the storefront size guide: `[{ "size": "M", "chestCm": 112, "lengthCm": 70.5, "shoulderCm": 48, "sleeveCm": 63 }]`. One chart per parent, since every color of a style is cut the same. Rules: up to 20 rows, sizes unique and (for clothing) one of the category's sizes, each measurement 0–300 cm rounded to 0.5 cm, at least one per row. Omitted on update keeps the chart; `[]` clears it. Stored in `products.size_chart` (JSONB).

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
- [x] Category sizes (`clothing`: `XXS`–`4XL`, Italian `34`–`60` with S/R/L fits) and a per-parent `sizeChart`
