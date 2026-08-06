# Decision: color × size variant matrix (one SkuID per cell)

**Status:** Accepted decision.  
**Related:** [product-sku-system.md](product-sku-system.md), [product-sku-dimensions.md](product-sku-dimensions.md), [product-variants-plan.md](product-variants-plan.md), [frontend-product-variants-migration.md](frontend-product-variants-migration.md).

## Decision

**One SkuID per color × size combination** (plus optional edition). That cell is the unit of sale for inventory, cart, checkout, and shipping.

A style offered in 5 colors and 3 sizes may have up to **15** variants. That count is expected, not a modeling mistake.

```text
Manage as axes                         Persist as cells (SkuID)
─────────────────                      ─────────────────────────
colors: BLK, GRN, RED, …               BLK×S  BLK×M  BLK×L
sizes:  S, M, L          ──generate──► GRN×S  GRN×M  GRN×L
                                       …
Each cell: stock, cart line, reserve, ship
```

## Why not fewer SkuIDs

| Shortcut | Why it fails |
|----------|--------------|
| One SkuID per color (sizes are display-only) | Cannot reserve or ship the right size; size stock mixes |
| One SkuID per size (colors are display-only) | Same for color; images and availability break |
| One SkuID per style | Collapses all options into one stock pool |
| “Too many rows — merge cells” | Admin pain is UX; collapsing cells breaks commerce |

Stock for Black-M and Green-L is independent. Cart and order already key on `skuId` ([product-sku-system.md](product-sku-system.md)). Flattening the matrix would undo that.

## What actually gets hard

**Managing** the matrix, not storing it.

Today each cell is created with a separate `POST /api/v1/products/{id}/variants`. Fields that belong to one axis are repeated on every cell:

| Axis | Typically shared across | Example |
|------|-------------------------|---------|
| Color | All sizes of that color | `imageUrls` |
| Size | All colors of that size | `dimensions` (mm) — [product-sku-dimensions.md](product-sku-dimensions.md) |
| Style / parent | Every cell | name, price, taxonomy |

Bags often stay small (many are `OS` / one size). Clothing is where the grid grows — that is a reason for better admin tools, not fewer SkuIDs.

## How to manage it (product rules)

1. **Sparse matrix** — only create cells you actually sell. 5×3 is a ceiling, not a quota.
2. **Edit by axis, fan out to cells** — set dimensions once per size; set images once per color; apply to matching SkuIDs.
3. **Bulk generate** from a color list × size list (manage-web matrix / future bulk API), instead of hand-creating 15 rows.
4. **Keep the cell as the API identity** — list/PDP may summarize with `availableColors` / `availableSizes`; cart/order still use `skuId`.

## Non-goals

- Do not collapse SkuIDs to “simplify” the catalog.
- Do not move stock onto color-only or size-only keys.
- Do not require a dense matrix (every color × every size) when some combinations are not sold.

## Follow-ups (not blocking this decision)

- [ ] manage-web: color × size matrix UI (create/enable cells, stock overview)
- [ ] Optional bulk variant-create API (axes in, cells out)
- [ ] Axis fan-out helpers (dimensions by size, images by color) — product or manage-web
- [ ] Storefront size chart from per-size `dimensions` ([product-sku-dimensions.md](product-sku-dimensions.md))
