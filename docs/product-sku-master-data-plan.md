# Plan: SKU Master Data (Code → Name) Runtime CRUD

**Status:** Plan (improves the mapping layer on top of [product-sku-system.md](product-sku-system.md))  
**Branch context:** builds on the luxury SKU format already in product service

## Intent

Operators must maintain **code → name** dictionaries for SKU segments at runtime (create / update name / delete when unused). Dictionaries live in PostgreSQL and are referenced by catalog styles and sellable SKU rows.

Focus entities from the request: **brand**, **style**, **color**. Size and edition follow the same pattern (include them so the model stays complete).

---

## Review of current state (gaps)

| Area | Today | Gap |
|------|-------|-----|
| Brand / color / size / edition tables | Seeded on migrate; `ensure*` upserts on product/variant write | No list/create/update/delete API; auto-create weakens “code must exist” |
| Style | Only `products.style_code` + display `products.name` | No dedicated `styles` master; cannot manage style code↔name independently of a product row |
| FKs | TEXT columns, no FK | Orphans possible; delete cannot be enforced by the DB |
| Codes vs names | Codes treated as immutable in docs | Name updates not exposed; code rename not forbidden by API |
| Permissions | `product.*` only | No `product.master.*` (or similar) for dictionary admin |
| Delete | N/A | Need “block if referenced by products/SKUs” |

**Verdict:** Seeded masters + silent `ensure*` are fine for bootstrap, but the durable model should be: **explicit master CRUD**, **hard FKs**, **Style as its own table under Brand**, and **no silent invent-on-SKU-create** once APIs exist.

---

## Improved target model

```text
brands          (code PK-ish unique, name)
  └── styles    (brand_code + code unique, name)     ← Style master
colors          (code, name)                         ← global palette
sizes           (code, name)
sku_editions    (code, name)                         ← optional VariantCode

products (catalog parent)
  ├── brand_code → brands.code          (FK, RESTRICT)
  ├── style_code → styles.code          (FK with brand, RESTRICT)
  └── name, description, …              (marketing; not in SKU)

product_variants (sellable SKU)
  ├── product_id → products.id
  ├── color_code → colors.code          (FK, RESTRICT)
  ├── size_code  → sizes.code           (FK, RESTRICT)
  ├── edition_code → sku_editions.code  (FK, nullable, RESTRICT)
  ├── human sku  (composed, immutable)
  └── sku_id     (ULID, canonical)
```

### Code vs name rules

| Field | Mutable? | Rule |
|-------|----------|------|
| `code` | **Never** after insert | Assigned on create; used in SKU composition |
| `name` | **Yes** | Display / ops label; changing name must **not** rewrite any human `sku` |
| Delete | Only if unused | `ON DELETE RESTRICT` + API returns 409 when referenced |

### Style is not the product row

- **Style** = design family identity (`BOT` + `CAS001` → “Cassette”).
- **Product** = catalog/PDP shell that *uses* a style (and may carry marketing copy, category, tags).
- One style → one product in v1 (keep `UNIQUE (brand_code, style_code)` on both `styles` and `products`), but the **name mapping** for the style code lives on `styles`, not only on `products.name`.

Migration path: backfill `styles` from existing `products (brand_code, style_code, name)`.

---

## Relational schema (target)

```sql
-- Masters (code immutable; name editable)
CREATE TABLE brands (
  code       TEXT PRIMARY KEY CHECK (code ~ '^[A-Z]{2,3}$'),
  name       TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE styles (
  brand_code TEXT NOT NULL REFERENCES brands(code) ON DELETE RESTRICT,
  code       TEXT NOT NULL CHECK (code ~ '^[A-Z0-9]{1,12}$'),
  name       TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (brand_code, code)
);

CREATE TABLE colors (
  code       TEXT PRIMARY KEY CHECK (code ~ '^[A-Z0-9]{1,12}$'),
  name       TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE sizes (
  code       TEXT PRIMARY KEY CHECK (code ~ '^[A-Z0-9]{1,12}$'),
  name       TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE sku_editions (
  code       TEXT PRIMARY KEY CHECK (code ~ '^[A-Z0-9]{1,12}$'),
  name       TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Catalog / SKU reference masters (after backfill + empty-string cleanup)
ALTER TABLE products
  ADD CONSTRAINT products_brand_fk
    FOREIGN KEY (brand_code) REFERENCES brands(code) ON DELETE RESTRICT,
  ADD CONSTRAINT products_style_fk
    FOREIGN KEY (brand_code, style_code) REFERENCES styles(brand_code, code) ON DELETE RESTRICT;

ALTER TABLE product_variants
  ADD CONSTRAINT variants_color_fk
    FOREIGN KEY (color_code) REFERENCES colors(code) ON DELETE RESTRICT,
  ADD CONSTRAINT variants_size_fk
    FOREIGN KEY (size_code) REFERENCES sizes(code) ON DELETE RESTRICT,
  ADD CONSTRAINT variants_edition_fk
    FOREIGN KEY (edition_code) REFERENCES sku_editions(code) ON DELETE RESTRICT;
-- edition_code '' / NULL: prefer NULL for “no edition” before adding FK
```

**Note:** Today columns default to `''`. Before FKs, normalize empty edition to `NULL` and reject empty brand/style/color/size on new rows.

---

## Runtime API (product service)

Base path: `/api/v1/catalog/...` (avoids clashing with `/products`). All writes require auth.

| Method | Path | Behavior |
|--------|------|----------|
| `GET` | `/catalog/brands` | List `{ code, name }` |
| `POST` | `/catalog/brands` | Create `{ code, name }` — 409 on duplicate code |
| `PATCH` | `/catalog/brands/{code}` | Update **name only** |
| `DELETE` | `/catalog/brands/{code}` | 409 if any `styles` or `products` reference it |
| `GET` | `/catalog/brands/{code}/styles` | List styles for brand |
| `POST` | `/catalog/brands/{code}/styles` | Create `{ code, name }` |
| `PATCH` | `/catalog/brands/{brandCode}/styles/{code}` | Update name only |
| `DELETE` | `/catalog/brands/{brandCode}/styles/{code}` | 409 if any `products` use it |
| `GET/POST/PATCH/DELETE` | `/catalog/colors` … | Same pattern |
| `GET/POST/PATCH/DELETE` | `/catalog/sizes` … | Same pattern |
| `GET/POST/PATCH/DELETE` | `/catalog/editions` … | Same pattern |

### Permissions

Add (covered by existing `product.*` wildcard):

- `product.master.read`
- `product.master.write` (create / rename / delete)

Reads may be public later for storefront filters; v1: `product.master.read` or `product.read`.

### Product / variant create changes

1. **Require** existing `brandCode` + `styleCode` (or create style first via catalog API).
2. **Require** existing `colorCode` + `sizeCode` (default size `OS` only if that row exists in `sizes`).
3. **Stop** `ensureBrand` / `ensureColor` inventing masters on SKU write (keep only for one-time migrate/backfill).
4. Resolve display names for JSON from master tables (join), so renaming “Black” → “Noir” updates PDP without touching SKUs.

---

## Delete policy (improved)

| Delete target | Allowed when | Reject when |
|---------------|--------------|-------------|
| Brand | No styles and no products | Any style or product references `brand_code` |
| Style | No products with that `(brand_code, style_code)` | Product exists |
| Color / size / edition | No `product_variants` rows with that code | Any SKU references it |

Never cascade-delete masters into SKUs. Never rewrite human `sku` or `skuId` when a **name** changes.

Optional later: soft-delete (`archived_at`) if ops want to hide unused codes without hard delete.

---

## Mapping UX (ops)

1. Create brand `BOT` / “Bottega Veneta”.
2. Create style `CAS001` under `BOT` / “Cassette”.
3. Ensure color `BLK` / “Black”, size `MED` / “Medium”, edition `V` / “Standard”.
4. Create product linked to `BOT` + `CAS001` (catalog shell).
5. Create variant with codes → SKU `BOT_CAS001_BLK_V_MED`.

Code↔name lookups:

- Admin grids: list masters.
- PDP / search: join codes to names for filters and labels.
- Warehouse: print human SKU; names are secondary.

---

## What NOT to do

- Do not put marketing product title into the SKU.
- Do not allow changing `code` after create (that would force SKU rewrites).
- Do not auto-delete masters when the last product is removed (explicit delete only).
- Do not invent brand/color codes from free-text on variant create once catalog APIs ship.
- Do not store season/price/stock in master or SKU segments.

---

## Implementation phases

### Phase A — Styles master + FKs (data integrity)

1. Create `styles` table; backfill from `products`.
2. Normalize `edition_code`: `''` → `NULL`.
3. Add FKs (`RESTRICT`) once backfill has no orphans.
4. Remove runtime `ensure*` from CreateProduct/CreateVariant (migrate-only helpers remain).

### Phase B — Catalog master APIs

1. Handlers + service + pg/memory stores for brand/style/color/size/edition.
2. Permissions `product.master.read` / `product.master.write`.
3. OpenAPI + `docs/endpoints.md` + manage-web hooks later.

### Phase C — Enforce on SKU writes

1. Variant create requires codes that exist (404/400 with clear errors).
2. Product create requires brand+style exist; optional: auto-create style only via explicit flag / separate call (prefer separate).
3. Enrich API responses with master names when display fields blank.

### Phase D — Admin UI / cleanup

1. Manage-web dictionary screens.
2. Drop reliance on in-memory `SeedBrands` for resolution (DB is source of truth; seed stays migrate-only).

---

## Success criteria

- Ops can create and delete brand/style/color (and size/edition) at runtime via API.
- Every SKU segment code resolves to exactly one name row in Postgres.
- Delete of an in-use code returns 409; DB FK would also block.
- Renaming a color updates labels without changing any `sku` / `skuId`.
- New SKUs still compose deterministically from codes only.

## References

- Format & principles: [product-sku-system.md](product-sku-system.md)
- Parent + variants: [product-variants-plan.md](product-variants-plan.md)
- Permissions catalog: [permissions.md](permissions.md)
- Code today: `product/pkg/domain/sku*.go`, `product/pkg/infra/pg/sku_master.go`
