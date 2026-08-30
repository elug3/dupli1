# Product stock tracking plan

**Status:** Planned (not implemented).  
**Repos:** `dupli1` (product, cart), `dupli1-web`, `dupli1-manage-web`.  
**Related:** [product-sold-count.md](product-sold-count.md), [payment-service.md](payment-service.md) (plan B commit-on-ship), [product-variants-plan.md](product-variants-plan.md) (historical stock section), [cart-service.md](cart-service.md), [v1.1-release-plan.md](v1.1-release-plan.md) (Commerce UX deferred items).

## Goal

Every sellable SKU has an authoritative stock row. Availability is visible on PDP and cart without a separate inventory round-trip where possible, and cart mutations respect available quantity. Checkout reservation remains the hard enforcement point (plan B: stock leaves the warehouse on ship commit).

## What already works

| Layer | Behavior |
|-------|----------|
| Product `stock_items` | `quantity` / `reserved` keyed by variant `skuId`; public GET, manager PUT/adjust |
| Reservations | Order creates → commit on ship → release on cancel; increments parent `soldCount` on commit |
| manage-web | SKU detail + variant table stock editors; optional initial stock on variant create |
| Storefront PDP | Client polls `/api/v1/inventory/…`; missing row ⇒ treat as in stock |
| Cart | Enriches `available_qty` when inventory URL is set; does **not** reject add when qty exceeds available |

## Problem

Stock is **optional** today. Variants without a `stock_items` row are treated as sellable everywhere except reservation time (where missing stock fails). That causes:

1. False "In stock" on PDP for untracked SKUs.
2. Cart can hold quantities that will fail at checkout reserve.
3. Merchants must remember a separate inventory write after every variant create (easy to skip).
4. Legacy parent `Product.stock` still exists in the domain/JSON and confuses clients.

## Decisions

| Decision | Choice |
|----------|--------|
| Unit of stock | Variant `skuId` (unchanged) — not parent product |
| Missing stock row | **Out of stock** after backfill (available = 0), not "untracked = infinite" |
| Auto-create on variant create | Yes — insert `stock_items` with `quantity = 0`, `reserved = 0` in the same transaction when possible |
| Initial quantity | Optional on variant create API / manage-web (already partially in UI); default 0 |
| PDP enrichment | Embed `availableQty` + `inStock` on each variant in `GET /api/v1/products/{id}` (and optionally list cards) |
| Cart add enforcement | Reject upsert/replace when requested qty > available (or available is 0 / missing) |
| Hard enforcement | Still reservation at order create; cart check is soft UX guard against obvious oversell |
| Stock ledger / low-stock alerts | **Out of scope** for this plan (v1.2+ ops) |
| Flat sellable model | Orthogonal — stock stays on the sellable SKU identity even if product/variant fold later |

## Semantics

| Field | Where | Meaning |
|-------|-------|---------|
| `quantity` | `stock_items` | On-hand units (includes reserved) |
| `reserved` | `stock_items` | Units held by active reservations |
| `available` | derived | `max(0, quantity - reserved)` |
| `inStock` | PDP JSON | `available > 0` |
| `availableQty` | PDP / cart JSON | Same as `available` |

Untracked (no row) after Phase 1 backfill must not occur for active variants. If a row is somehow missing, treat as `available = 0` / `inStock = false` in enrichment paths (do not fall back to "assume available").

## Phases

### Phase 0 — Document + contracts (this PR)

- [x] This plan
- [ ] Index in [README.md](README.md) / checklist in [TODO.md](TODO.md)
- [ ] OpenAPI sketch: variant fields `availableQty`, `inStock`; note auto stock row on variant create

### Phase 1 — Always-tracked SKUs (product)

**Backend**

1. On `CreateVariant` (PG + memory): ensure `stock_items` row exists (`quantity` from optional request field or 0).
2. Startup / one-shot migrate: `INSERT … SELECT` for every `product_variants.sku_id` missing from `stock_items` with `quantity = 0`.
3. `GetItem` / enrichment helpers: missing row ⇒ available 0 (or auto-heal insert 0) — prefer explicit backfill so reads stay simple.
4. Tests: create variant creates stock; backfill covers orphans; reserve still works when qty set.

**manage-web**

5. Keep initial-stock on variant create; show "0" instead of empty when row exists.
6. Prefer `skuId` inventory paths everywhere (align with frontend SkuID migration notes).

**Docs**

7. Update [api.md](api.md), [current-state.md](current-state.md): untracked SKUs are no longer a supported mode.

### Phase 2 — PDP enrichment (product + storefront)

**Backend**

1. When building parent PDP (and optionally search hits), batch-read stock for embedded variant `skuId`s.
2. Set variant `availableQty` / `inStock` (new JSON fields; do not revive parent `stock`).
3. Public list: optional parent-level `inStock` = any active variant available (for PLP badges) — only if cheap; otherwise skip and leave PLP without stock until PDP.

**dupli1-web**

4. Prefer PDP-embedded `inStock` / `availableQty`; keep inventory poll as fallback only if fields absent.
5. Stop treating HTTP miss / null as in-stock once Phase 1 ships.

### Phase 3 — Cart stock on add (cart + storefront)

**Backend (cart)**

1. On `UpsertItem` / `ReplaceItems`, after variant resolve, look up available qty.
2. If `quantity > available` → `400` with a clear error (and optionally `available_qty` in body). Missing/zero stock → same as unavailable for purchase.
3. Keep enrichment `available_qty` on GET cart for UI clamp/display.

**dupli1-web**

4. Surface cart errors on add-to-cart / qty change; disable increment past `available_qty`.

### Phase 4 — Cleanup

1. Stop accepting/returning parent `Product.stock` (ignore on write; omit on read once clients migrated).
2. Drop or leave unused `products.stock` column (additive migrate only; dropping is optional follow-up).
3. Mark Commerce UX items in [v1.1-release-plan.md](v1.1-release-plan.md) / [TODO.md](TODO.md) done for `stock on add` + PDP `inStock`.

## Out of scope

- Stock movement audit ledger (who adjusted, when)
- Low-stock Telegram / ops alerts
- Multi-warehouse / location stock
- Soft-hold on "add to cart" (reservation stays order-scoped)
- Guest cart + merge (separate Commerce UX item)
- Changing commit-on-ship (plan B) semantics

## Testing

| Phase | Proof |
|-------|--------|
| 1 | `go test` in `product` — create variant ⇒ stock row; backfill; adjust/reserve |
| 2 | Handler/PDP JSON includes `inStock`/`availableQty`; storefront shows OOS without extra inventory call |
| 3 | Cart upsert over available → 400; smoke path: set stock 1 → add 2 fails → add 1 → checkout reserves |
| 4 | Parent JSON has no meaningful `stock`; OpenAPI + docs match |

## Exit criteria

- [ ] Every active variant has a `stock_items` row
- [ ] New variants always get a row (qty default 0)
- [ ] PDP returns per-variant availability; storefront does not treat missing stock as in-stock
- [ ] Cart rejects quantity above available
- [ ] Docs (`api.md`, `current-state.md`, `TODO.md`) updated; OpenAPI fields present
