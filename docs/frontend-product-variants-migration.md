# Frontend migration: parent products + variants

**Audience:** `dupli1-web` (storefront) and `dupli1-manage-web` (admin)  
**Backend:** `dupli1-product` (catalog, stock, and reservations), `dupli1-order` (cart/checkout)  
**Gateway base URL (local):** `http://localhost:8080`

This document describes how to migrate clients from the old flat product model (one row = one color) to **parent style + variants (SKUs)**.

Related backend plan: [product-variants-plan.md](product-variants-plan.md)

---

## Why this changed

Customers must:

- See **one** catalog card per style (no duplicate colors in search)
- Pick **color / size** on the PDP
- See **images per color**
- Buy a line item that has **stock per color/size**

Order and inventory already use **SKU**. The catalog now exposes:

| Concept | ID example | Role |
|---------|------------|------|
| **Parent (style)** | `BOT-001` | Search, list, PDP URL |
| **Variant (SKU)** | `BOT-001-GRN` | Color/size, images, price, cart, inventory |

Existing products were **backfilled**: each parent has at least one variant with `sku === product.id` (e.g. `BOT-001`). Single-color products keep working if you still send that id as the cart SKU.

---

## Breaking / behavioral changes

| Area | Before | After |
|------|--------|--------|
| Search results | One row per color (duplicates) | **One row per parent style** |
| List response shape | Sometimes a bare array (old admin list) | Always `{ "total", "results" }` |
| PDP | Flat `color`, `price`, `imageUrls` | Parent + `variants[]`, `availableColors`, `availableSizes` |
| Cart / checkout SKU | Product id | **Variant `sku`** (may equal parent id for legacy single-variant products) |
| Stock | Do not trust `product.stock` | `GET /api/v1/inventory/{sku}` per variant |
| Removed | `GET /api/v1/products/bags` | Use `GET /api/v1/products?category=bags` |
| Removed | `GET /api/v1/products/{id}/manage` | Use PDP + manager Bearer on list, or load parent by id |
| Images | `PUT /products/{id}/image` | `POST /products/{id}/images` (default variant) or `POST /products/{id}/variants/{sku}/images` |

Legacy fields (`price`, `color`, `imageUrls`) may still appear on the parent as a **mirror of the default variant**. Prefer the new fields below; do not write new UI against legacy-only fields.

---

## Auth reminder

Protected product routes need:

1. `POST /api/v1/auth/login` → `refresh_token`
2. `POST /api/v1/auth/refresh` → access token in `token`
3. `Authorization: Bearer <access_token>`

Catalog admin permissions: `product.create`, `product.update`, `product.read`, `product.variant.*`, `product.image.upload`, or wildcards such as `product.*` / `*`. See [permissions.md](permissions.md).

Public catalog reads need **no** auth.

---

## Shared types (TypeScript-oriented)

```ts
type ProductStatus = "active" | "draft" | "archived";

interface Variant {
  sku: string;
  productId: string;
  color: string;
  size?: string;
  price: number;
  status: ProductStatus;
  imageUrls?: string[];
  createdAt?: string;
}

interface Product {
  id: string;
  name: string;
  description: string;
  brand: string;
  material: string;
  category: string;
  status: ProductStatus;
  cost?: number;              // managers only; public PDP omits / zeros cost
  capacity?: string;
  tags?: string[];
  createdAt: string;

  // Derived from variants (prefer these)
  priceFrom?: number;
  defaultImageUrl?: string;
  availableColors?: string[];
  availableSizes?: string[];
  variants?: Variant[];       // present on PDP; omitted on list/search cards

  // Legacy mirrors (optional; do not rely on for new UI)
  price?: number;
  color?: string;
  stock?: number;
  imageUrls?: string[];
}

interface ProductSearchResponse {
  total: number;
  results: Product[];
}
```

---

## `dupli1-web` (storefront)

### Catalog / search

**Request**

```http
GET /api/v1/products?category=bags&brand=Bottega&color=Green&tags=hot
```

Query params:

| Param | Notes |
|-------|--------|
| `category` | e.g. `bags` (replaces `/products/bags`) |
| `brand` | Case-insensitive substring |
| `color` | Parent included if **any active variant** has this color |
| `size` | Parent included if **any active variant** has this size |
| `material` | Exact |
| `tags` | Comma-separated; parent must include all |

**Response:** `{ total, results }` — **one card per style**.

**UI mapping**

| UI element | Field |
|------------|--------|
| Title | `name` |
| Brand | `brand` |
| Price | `priceFrom` (fallback: `price`) |
| Thumbnail | `defaultImageUrl` (fallback: `imageUrls[0]`) |
| Color chips | `availableColors` |
| Size chips | `availableSizes` |
| Link | `/products/{id}` using **parent** `id` |

Do **not** render one card per variant.

### Product detail page (PDP)

**Request**

```http
GET /api/v1/products/{id}
```

Public PDP returns only `status === "active"` parents. Draft/archived → `404`. `cost` is not shown.

**UI flow**

1. Load parent by `id`.
2. Build color picker from `availableColors` (or unique `variants[].color`).
3. Build size picker from `availableSizes` when non-empty.
4. On selection, resolve the matching **active** variant:

```ts
function findVariant(
  product: Product,
  color: string,
  size: string = "",
): Variant | undefined {
  return product.variants?.find(
    (v) =>
      v.status === "active" &&
      v.color === color &&
      (v.size || "") === (size || ""),
  );
}
```

5. Show `variant.imageUrls` and `variant.price` for the selection.
6. Add to cart with **`variant.sku`**, not `product.id` (unless they are equal for a legacy single-SKU product).

### Stock (optional but recommended)

```http
GET /api/v1/inventory/{sku}
```

Use the selected variant’s `sku`. Disable “Add to cart” when quantity is insufficient. Do not use `product.stock`.

### Checkout / cart

Order and checkout line items already use `sku`:

```json
{
  "sku": "BOT-001-GRN",
  "quantity": 1,
  "unit_price_cents": 250000
}
```

Rules:

- `sku` = **variant.sku**
- `unit_price_cents` = `Math.round(variant.price * 100)` (or your pricing source of truth)

### Storefront checklist

- [ ] Replace `/api/v1/products/bags` with `/api/v1/products?category=bags`
- [ ] Parse list as `{ total, results }`
- [ ] List cards use parent fields (`priceFrom`, `defaultImageUrl`, `availableColors`)
- [ ] PDP loads `variants` and selects color/size → variant
- [ ] Gallery uses `variant.imageUrls`
- [ ] Cart/checkout send **variant SKU**
- [ ] Stock checks use inventory by variant SKU
- [ ] Stop depending on `/products/{id}/manage`

---

## `dupli1-manage-web` (admin)

All write routes require Bearer access token + role (`product_manager` | `admin` | `owner`).

### List / search parents

```http
GET /api/v1/products
Authorization: Bearer <access_token>
```

With a manager token you see **all statuses** and **cost**. Same `{ total, results }` shape. Optional `?status=draft`.

There is **no** `/manage` endpoint. Open a parent with:

```http
GET /api/v1/products/{id}
```

(Public GET hides drafts; for draft parents, use manager list or add auth-aware PDP later—see [TODO.md](TODO.md). Today, create/list as manager, then edit by id if the parent is active, or keep draft ids from the create response.)

### Create a parent style

```http
POST /api/v1/products
Authorization: Bearer <access_token>
Content-Type: application/json

{
  "name": "Cassette Bag",
  "brand": "Bottega Veneta",
  "category": "bags",
  "material": "Leather",
  "description": "...",
  "status": "active",
  "tags": ["new"]
}
```

Parent id is auto-generated (`BOT-001`) if omitted.

**Legacy shortcut:** if you also send `color`, `price`, and/or `imageUrls`, the API seeds **one default variant** with `sku === parent.id`. Prefer explicit variant APIs for multi-color products.

### Add a variant to an existing parent

```http
POST /api/v1/products/BOT-001/variants
Authorization: Bearer <access_token>
Content-Type: application/json

{
  "color": "Black",
  "size": "",
  "price": 2500,
  "status": "active"
}
```

- `sku` optional → auto (e.g. `BOT-001-BLA`). You may set `"sku": "BOT-001-BLK"`.
- `(color, size)` must be unique per parent.

### Update / delete a variant

```http
PUT /api/v1/products/BOT-001/variants/BOT-001-BLK
Authorization: Bearer <access_token>
Content-Type: application/json

{
  "color": "Black",
  "size": "",
  "price": 2600,
  "status": "active",
  "imageUrls": ["https://..."]
}
```

```http
DELETE /api/v1/products/BOT-001/variants/BOT-001-BLK
Authorization: Bearer <access_token>
```

### Upload images

**Preferred (per color):**

```http
POST /api/v1/products/BOT-001/variants/BOT-001-BLK/images
Authorization: Bearer <access_token>
Content-Type: multipart/form-data

image=@black.jpg
```

Response `201` → updated **variant**.

**Legacy (default variant only):**

```http
POST /api/v1/products/BOT-001/images
Authorization: Bearer <access_token>
Content-Type: multipart/form-data

image=@photo.jpg
```

Response `201` → updated **parent** (images applied to default variant: `sku === id` if present, else first variant).

Old `PUT .../image` is **removed**.

### Update parent (shared fields only)

```http
PUT /api/v1/products/BOT-001
Authorization: Bearer <access_token>
Content-Type: application/json

{
  "name": "Cassette Bag",
  "description": "...",
  "brand": "Bottega Veneta",
  "material": "Leather",
  "category": "bags",
  "status": "active",
  "cost": 900,
  "tags": ["hot"],
  "capacity": "Medium"
}
```

Do not send color/price/images on the parent for multi-variant styles; manage those on variants.

### Delete parent

```http
DELETE /api/v1/products/BOT-001
Authorization: Bearer <access_token>
```

Cascades variants. Inventory rows for those SKUs are **not** auto-deleted—clean up inventory separately if needed.

### Stock in admin

Set quantity per **variant SKU**:

```http
PUT /api/v1/inventory/BOT-001-BLK
Authorization: Bearer <access_token>
Content-Type: application/json

{ "quantity": 10 }
```

Admin product screens should list variants and link each row to inventory by `sku`.

### Admin checklist

- [ ] Product list uses `{ total, results }` and shows parent styles
- [ ] Product detail shows variant table (sku, color, size, price, status, images)
- [ ] “Add color/size” → `POST .../variants`
- [ ] Image upload targets `.../variants/{sku}/images`
- [ ] Inventory UI keys off variant `sku`
- [ ] Remove calls to `/products/bags`, `/products/{id}/manage`, `PUT .../image`
- [ ] Create flow: parent first, then variants (or legacy single-variant create)

---

## End-to-end example: add Black to existing `BOT-001`

1. **Manage:** open parent `BOT-001` (already has Green variant, often `sku: "BOT-001"` after backfill).
2. `POST /api/v1/products/BOT-001/variants` with `{ "color": "Black", "price": 2500, "sku": "BOT-001-BLK" }`.
3. `POST /api/v1/products/BOT-001/variants/BOT-001-BLK/images` with file `image`.
4. `PUT /api/v1/inventory/BOT-001-BLK` with `{ "quantity": 10 }`.
5. **Web:** `GET /api/v1/products/BOT-001` → `availableColors: ["Green","Black"]`.
6. Customer selects Black → cart line `sku: "BOT-001-BLK"`.

Search still returns a **single** `BOT-001` card.

---

## Compatibility matrix

| Client behavior | Safe now? | Notes |
|-----------------|-----------|--------|
| List `{ total, results }` | Yes | Required |
| Use parent `id` in PDP URL | Yes | |
| Cart uses parent `id` only | Only if single legacy variant with `sku === id` | Breaks for multi-color |
| Read `availableColors` / `variants` | Yes | Preferred |
| Rely on parent `color` / `imageUrls` only | Transitional | Default variant mirror only |
| `GET /products/bags` | No | Use `?category=bags` |
| `GET /products/{id}/manage` | No | Removed |
| `PUT /products/{id}/image` | No | Use `POST .../images` or variant images |

---

## OpenAPI

Machine-readable contract: [api/specs/product-v1.yaml](../api/specs/product-v1.yaml)

---

## Support contacts / follow-ups

Backend remaining work (not required for initial client migration):

- PDP `inStock` enrichment from inventory (clients can call inventory today)
- Auth-aware admin GET for draft parents by id
- Dropping legacy parent columns after all clients migrate

Track in [TODO.md](TODO.md) and [product-variants-plan.md](product-variants-plan.md).
