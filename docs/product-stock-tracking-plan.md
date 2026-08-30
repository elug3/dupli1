# Product stock tracking plan

**Status:** Implemented (Phases 1–4).  
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
- [x] Index in [README.md](README.md) / checklist in [TODO.md](TODO.md)
- [x] OpenAPI / api.md: variant fields `availableQty`, `inStock`; auto stock row on variant create

### Phase 1 — Always-tracked SKUs (product)

**Backend**

1. [x] On `CreateVariant` (PG + memory): ensure `stock_items` row exists (`quantity` 0 by default).
2. [x] Startup migrate: `INSERT … SELECT` for every `product_variants.sku_id` missing from `stock_items`.
3. [x] Enrichment: missing row ⇒ available 0 / `inStock` false.
4. [x] Tests: create variant creates stock; PDP enrichment; reserve still works when qty set.

**manage-web**

5. [x] Keep initial-stock on variant create; missing stock shows as 0.
6. [x] Prefer `skuId` inventory paths.

**Docs**

7. [x] Update [api.md](api.md), [current-state.md](current-state.md).

### Phase 2 — PDP enrichment (product + storefront)

**Backend**

1. [x] Batch-read stock for embedded variant `skuId`s on PDP / public variant reads.
2. [x] Set variant `availableQty` / `inStock`.
3. [x] Skip PLP parent-level `inStock` (PDP only for now).

**dupli1-web**

4. [x] Prefer PDP-embedded `inStock` / `availableQty`; inventory poll as fallback.
5. [x] Stop treating HTTP miss / null as in-stock (404 ⇒ 0).

### Phase 3 — Cart stock on add (cart + storefront)

**Backend (cart)**

1. [x] On `UpsertItem` / `ReplaceItems`, look up available qty after variant resolve.
2. [x] If `quantity > available` → `400` with `reason: insufficient_stock` + `available_qty`.
3. [x] Keep enrichment `available_qty` on GET cart.

**dupli1-web**

4. [x] PDP disables add when OOS; cart mutation errors already surfaced via `useCartMutation`.

### Phase 4 — Cleanup

1. [x] Parent `Product.stock` omitted from JSON (`json:"-"`).
2. [x] Column left in place (additive migrate only; drop optional follow-up).
3. [x] [TODO.md](TODO.md) / plan status updated for stock on add + PDP `inStock`.

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

- [x] Every active variant has a `stock_items` row (create + migrate backfill)
- [x] New variants always get a row (qty default 0)
- [x] PDP returns per-variant availability; storefront does not treat missing stock as in-stock
- [x] Cart rejects quantity above available
- [x] Docs (`api.md`, `current-state.md`, `TODO.md`) updated; API narrative documents fields
