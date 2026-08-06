# SKU physical dimensions

**Status:** Implemented.  
**Related:** [product-sku-system.md](product-sku-system.md), [product-variants-plan.md](product-variants-plan.md), [product-attributes.md](product-attributes.md).

## Purpose

Letter size labels (`Size` / `SizeCode`: S, M, L, XL, MED, OS, …) identify the option in the human SKU and the size picker. Customers also need the **actual product measurements**.

`dimensions` on each **variant (SKU)** stores width × height × depth in **millimeters**.

| Field | Role | Example |
|-------|------|---------|
| `size` / `sizeCode` | Option label / SKU segment | `"M"` / `"M"` |
| `dimensions.widthMm` | Physical width | `340` |
| `dimensions.heightMm` | Physical height | `220` |
| `dimensions.depthMm` | Physical depth | `80` |

## Why on the SKU (variant), not elsewhere

| Place | Verdict |
|-------|---------|
| **Variant / SKU** | **Chosen.** Different letter sizes of the same style have different physical sizes (M ≠ L). |
| Size master (`sizes` table) | Rejected — `MED` is not a fixed mm size across brands/styles. |
| Parent product | Rejected — would force one measurement for every size option. |
| Parent `attributes` / `capacity` | Rejected — free-form, untyped, parent-only; not structured for PDP size charts. |

Same letter size across colors of one style usually shares measurements; store them per SKU (simple) and let managers copy values. No automatic fan-out yet — axis fan-out is the intended manage path for the color × size matrix ([product-variant-matrix.md](product-variant-matrix.md)).

## API

JSON on variant create / update / PDP / public variant lookups:

```json
{
  "skuId": "01JAY…",
  "sku": "BOT_CAS001_BLK_M",
  "size": "M",
  "sizeCode": "M",
  "dimensions": {
    "widthMm": 340,
    "heightMm": 220,
    "depthMm": 80
  }
}
```

- Unit is always **mm**. Omit an axis that does not apply (e.g. depth for a flat wallet).
- Optional — existing SKUs without measurements omit `dimensions`.
- **Merge-on-update:** omitting `dimensions` keeps the current value; `"dimensions": {}` clears; a non-empty object replaces all axes.
- Validation: each axis ≥ 0 and ≤ 10000 mm (`domain.MaxDimensionMm`). Invalid → `400`.

## Storage

`product_variants` columns (nullable):

| Column | Type |
|--------|------|
| `width_mm` | `INTEGER` |
| `height_mm` | `INTEGER` |
| `depth_mm` | `INTEGER` |

Added on product-service migrate (`ADD COLUMN IF NOT EXISTS`). Not used for search/filter in v1.

## Checklist

- [x] `domain.Dimensions` + `Variant.Dimensions`
- [x] Normalize / validate on create and update
- [x] Merge-on-update (omit / replace / clear)
- [x] PG columns + memory store round-trip
- [x] PDP and public variant responses include dimensions when set
- [ ] Storefront / manage-web size chart UI (frontend repos)
- [ ] Axis fan-out (dimensions by size across colors) — see [product-variant-matrix.md](product-variant-matrix.md)
