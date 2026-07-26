# Product attributes (memo)

**Status:** Implemented.  
**Related:** [product-price-on-parent.md](product-price-on-parent.md), [product-master-catalog.md](product-master-catalog.md), [frontend-product-variants-migration.md](frontend-product-variants-migration.md).

## Purpose

`attributes` is a free-form **string key → string value** map on the **parent product**. It is a display memo for the PDP and admin UI — not a search facet, not pricing, and not used by cart/order/payment.

Typical keys (examples only; not enforced):

| Key | Example value |
|-----|----------------|
| `condition` | `excellent` |
| `authenticity` | `verified` |
| `care` | `wipe with dry cloth` |
| `origin` | `Italy` |

## Rule

| Concern | Where it lives |
|---------|----------------|
| Filterable merchandising | Typed columns / masters (`brand`, `subCategory`, `style`, `target`, `material`, `tags`, …) |
| Long prose | `description` |
| Display-only memo pairs | **`attributes`** |

Do **not** put sale price, SKU identity, or stock in attributes. Promote a key to a real column if you need filter/sort/index later.

## API

JSON field on parent product create/get/update/list/PDP:

```json
{
  "id": "01HZX…",
  "name": "Cassette",
  "price": 2500000,
  "officialPrice": 3200000,
  "attributes": {
    "condition": "excellent",
    "care": "wipe with dry cloth"
  }
}
```

- **Read:** public and authenticated product responses may include `attributes` (omitted when empty).
- **Write:** managers via `POST /api/v1/products` and `PUT /api/v1/products/{id}` (`product.create` / `product.update`).
- **Storefront:** treat as **read-only** (do not send on customer flows).
- **Variants:** no `attributes` field — memo is parent-only.

### Merge-on-update

Same pattern as `tags` / price:

| Request body | Result |
|--------------|--------|
| `attributes` omitted | Keep existing map |
| `"attributes": { … }` | Replace entire map (after normalize) |
| `"attributes": {}` | Clear all attributes |

Partial key patches are not supported — send the full map you want to keep.

### Normalization / limits

On create/update the service:

1. Trims keys and values  
2. Drops empty keys  
3. Rejects maps with more than **32** entries  
4. Rejects key length **> 64** or value length **> 512** (Unicode runes)

Invalid input → `400`.

## Storage

| Table | Column | Type |
|-------|--------|------|
| `products` | `attributes` | `JSONB NOT NULL DEFAULT '{}'` |

Added on product-service startup migrate (`ADD COLUMN IF NOT EXISTS`). Empty maps are stored as `{}` and omitted from JSON responses when empty.

## Frontend notes

- PDP: render as a definition list / table of label → value.  
- Admin: key-value editor; on save send the full `attributes` object.  
- Do not use attributes for faceted search UI — use taxonomy + `tags`.

## Out of scope

- Filtering / sorting by attribute key  
- Nested objects or non-string values  
- Per-variant attributes  
- i18n of keys (use stable English keys; localize labels in the client)
