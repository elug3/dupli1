# Coupon / sales-trackable referral code plan

**Status:** Planning (2026-08-31) — not started. Target **v1.2+** commerce (after v1.0 / v1.1 platform slices).  
**Repos:** `dupli1` (product, order), `dupli1-web`, `dupli1-manage-web`.  
**Related:** [checkout-session.md](checkout-session.md), [api.md](api.md) (coupons + checkout), [payment-service.md](payment-service.md), [permissions.md](permissions.md), [v1.1-release-plan.md](v1.1-release-plan.md) (commerce deferred to v1.2), [TODO.md](TODO.md).

## Goal

One code system that can **discount**, **attribute sales**, or **both** — so marketing coupons and partner/influencer referral codes share the same checkout path managers already use (`coupon_code` on checkout → order).

## Verdict

**Do not build a separate referral service.** Extend the existing product coupon + order `coupon_code` / `discount_cents` path into a **promo code** model with:

1. Hardened coupon rules (real expiry, usage limits, redemption ledger).
2. Optional **partner / campaign attribution** for sales reporting (GMV + order count by code).
3. Optional **zero-discount** codes (track-only referral) and **discount + track** hybrid codes.

Customer-facing UX keeps calling them “promo / coupon”; admin reports distinguish discount vs referral campaigns.

## What already works

| Layer | Behavior |
|-------|----------|
| Product `coupons` | `code`, `discount` (fraction), `description`, `expires` (free-text), `active` — PG + memory; seed `SUMMER30` |
| Redeem | `POST /api/v1/products/coupons/redeem` (and legacy `/api/v1/coupons/redeem`) — **lookup only**; does not consume uses |
| Order checkout | `POST …/checkout/sessions/{id}/coupon` → product redeem → `coupon_code` + `%` of subtotal as `discount_cents` |
| Order row | Immutable `coupon_code` / `discount_cents` / `total_cents` on create; pricing server-side |
| manage-web | `/coupons` CRUD (code, %, description, expires, active toggle) |
| Storefront | Cart/checkout promo field; validates via redeem; applies via `applySessionCoupon` before complete |
| Permissions | `coupon.read|create|update|delete` / `coupon.*` (catalog_editor bundle) |

## Problem

Today’s coupons are **catalog labels with a percentage**, not a commerce instrument:

1. **No usage accounting** — redeem never increments a counter; unlimited global use.
2. **Expiry not enforced** — `expires` is display text (`"Aug 31, 2026"`); `GetActive` only checks `active`.
3. **Discount shape is narrow** — fraction only (`0 < f < 1`); no fixed KRW off, min spend, max discount, or SKU/brand scope.
4. **No sales reporting by code** — orders store `coupon_code`, but there is no aggregate API or admin report (orders × GMV × paid status).
5. **No partner / referrer identity** — cannot answer “how much did code `YUNA10` drive?” beyond grepping orders.
6. **Profile coupon wallet is a stub** — `dupli1-web` profile “Coupons” is client-local (`COUPONS = []`); not a server wallet.
7. **Referral is missing** — zero-discount attribution codes and partner payouts are out of scope of the current model.

Building a standalone “referral” microservice would duplicate the checkout apply path and leave coupon gaps unfixed.

## Product question (coupon vs referral)

| Need | Coupon-only | Referral-only | **Unified promo (recommended)** |
|------|-------------|---------------|----------------------------------|
| % or ₩ off at checkout | Yes | Optional | Yes (`discount_*`) |
| Track who drove the order | Weak (code string only) | Yes | Yes (`partner_id` / campaign) |
| Track-only (no discount) | Awkward (`discount=0` rejected today) | Yes | Yes (`kind=referral` or `discount=0` allowed) |
| Manager CRUD in one place | Already `/coupons` | New UI | Extend `/coupons` + reports |
| Checkout / payment path | Already wired | New wire | Reuse existing apply + order fields |

**Recommendation:** unified promo codes. “Referral” is a **kind / campaign mode**, not a second code namespace.

## Decisions

| Decision | Choice |
|----------|--------|
| Ownership of code definitions | **Product** (existing `coupons` table / CRUD) — rename conceptually to *promo codes*; keep HTTP paths `/products/coupons` for compatibility |
| Ownership of applied code on purchase | **Order** — keep `coupon_code` + `discount_cents` on session/order (stable wire names) |
| Sales attribution moment | Count toward **paid** GMV when order becomes `paid` (payment.succeeded). Pending/canceled do not count. Ship/fulfill do not change attribution |
| Redemption / usage consume | On **checkout complete** (order create), after server re-validates redeem — not on preview redeem, not on cart |
| Track-only codes | Allowed: `discount_fraction = 0` and/or `discount_cents = 0`; session still stores `coupon_code` |
| Partner entity | Lightweight: optional `partner_id` (string ULID or slug) + display name on the code row. No payout ledger in v1 of this plan |
| Customer wallet | **Out of scope** for first slices — keep public redeem + checkout apply; profile wallet stays cosmetic until a later phase |
| Currency | KRW only; fixed discounts are whole won (`*_cents` = whole KRW) |
| Permissions | Reuse `coupon.*`; add report read under `coupon.read` or new `coupon.report` if managers need separation later |
| Commission / influencer payout | **Out of scope** — report GMV by code/partner; finance settles offline |

## Domain model (target)

### Promo code (product)

Extend `coupons` (additive columns; keep `code` PK):

| Field | Type | Notes |
|-------|------|-------|
| `code` | text PK | Uppercased; existing |
| `kind` | text | `discount` \| `referral` \| `hybrid` (default `discount`) |
| `discount_type` | text | `percent` \| `fixed` \| `none` |
| `discount` / `discount_fraction` | float | Keep existing column for percent; `0` allowed when type `none` |
| `discount_fixed_cents` | bigint | Whole KRW off when type `fixed` |
| `description` | text | Existing |
| `expires_at` | timestamptz nullable | Enforce on redeem; migrate away from free-text `expires` (keep `expires` as display until clients migrate) |
| `active` | bool | Existing |
| `max_redemptions` | int nullable | Global cap; null = unlimited |
| `max_per_customer` | int nullable | Per `customer_id`; null = unlimited |
| `min_subtotal_cents` | bigint | Default 0 |
| `partner_id` | text nullable | Referrer / campaign owner |
| `partner_label` | text | Admin display |
| `redemption_count` | int | Denormalized counter (ledger is source of truth) |

Rules on redeem (validate) and again on checkout complete:

- `active` and (`expires_at` is null or `now < expires_at`)
- global and per-customer caps not exceeded (count **consumed** redemptions only)
- subtotal ≥ `min_subtotal_cents`
- discount math: percent → `floor(subtotal * fraction)` or existing int cast; fixed → `min(fixed, subtotal)`; none → `0`

### Redemption ledger (product or order)

Prefer **order as source of applied code** + a **product redemption ledger** written when order is created (or when order becomes `paid` — see open questions):

```text
coupon_redemptions (
  id, code, order_id, customer_id,
  discount_cents, order_subtotal_cents,
  status: reserved | consumed | released,
  created_at, paid_at nullable
)
```

| Event | Ledger |
|-------|--------|
| Checkout complete | Insert `reserved` (or `consumed` if we attribute at create); bump counters if counting at create |
| Order → `paid` | Mark `consumed` / set `paid_at`; sales report uses this |
| Order → `canceled` from pending | `released`; free the cap slot |

Exact reserve-vs-paid timing is an open question below; **reports always filter paid**.

### Order (unchanged wire + optional enrichment)

Keep `coupon_code`, `discount_cents`. Optional later: `partner_id` snapshot on the order row for reporting without joining product (denormalize at complete).

## Attribution & reporting

**Sales by code (manager):**

- Inputs: `code` or `partner_id`, date range, status filter (default `paid`+)
- Metrics: order count, GMV (`sum(total_cents)`), discount given (`sum(discount_cents)`), AOV
- API sketch: `GET /api/v1/products/coupons/{code}/stats` and/or `GET /api/v1/products/coupons/stats?partner_id=`
- manage-web: Coupons table columns + detail drawer with stats; Orders filter by coupon code (already show code on order detail)

**Storefront:** no change to “one code per order”; track-only codes still apply like coupons (no visible discount line if 0).

## Flow (target)

```mermaid
sequenceDiagram
  participant Web as dupli1-web
  participant Order as order
  participant Product as product

  Web->>Product: POST /coupons/redeem (preview)
  Product-->>Web: code + discount shape if valid
  Web->>Order: POST checkout/.../coupon
  Order->>Product: Redeem (validate)
  Order-->>Web: session totals
  Web->>Order: POST checkout/.../complete
  Order->>Product: Redeem + reserve redemption
  Order->>Order: Create order with coupon_code
  Note over Order: payment.succeeded → paid
  Order->>Product: Confirm redemption consumed (paid attribution)
```

## Phases

### Phase 0 — Plan + contracts (this doc)

- [x] This plan
- [ ] Index in [README.md](README.md) / checklist in [TODO.md](TODO.md)
- [ ] Resolve open questions (attribution consume timing, fixed discount, partner shape)
- [ ] OpenAPI / api.md stubs for new coupon fields + stats (when implementation starts)

### Phase 1 — Harden coupons (product + order)

**Backend**

1. Add `expires_at`, `max_redemptions`, `max_per_customer`, `min_subtotal_cents`, `redemption_count` (additive migrate).
2. Enforce expiry + caps on redeem and on checkout complete re-validate.
3. Redemption ledger + release on unpaid cancel.
4. Allow `discount = 0` for track-only (adjust domain `ApplyCoupon` which today rejects `<= 0` / `>= 1`).
5. Tests: expired, capped, per-customer, cancel releases, complete recomputes discount.

**Frontends**

6. manage-web: edit new fields; show redemption count.
7. Storefront: clearer errors (expired / limit reached) if API returns distinct codes.

### Phase 2 — Referral / partner attribution

1. Add `kind`, `partner_id`, `partner_label` on coupons.
2. Snapshot partner on order at complete (optional column).
3. Stats API: GMV / count by code and by partner (paid only).
4. manage-web: partner fields + simple report view on coupon detail.
5. Docs: living as-built note (or fold into api.md / current-state).

### Phase 3 — Richer discount shapes (optional)

1. `discount_type` + `discount_fixed_cents`.
2. Order checkout math supports fixed KRW off.
3. manage-web create/edit UI for fixed vs percent vs none.
4. (Defer) brand/SKU scope, stacking, first-order-only.

### Phase 4 — Customer coupon wallet (optional, later)

1. Server-side “saved codes” per customer (auth or product).
2. Replace storefront profile stub with real list + used/expired.
3. Still one applied code per checkout session.

## Non-goals (this plan)

- Separate `dupli1-referral` service or NATS `referral.*` events
- Multi-code stacking on one order
- Automatic partner commission / payouts / tax
- Guest checkout attribution without customer_id (guest cart still open elsewhere)
- Changing JSON field names away from `coupon_code` / `discount_cents`
- Formal SQL migration tooling (continue additive startup migrate)

## Open questions

1. **Consume on create vs paid?** Caps that free on unpaid cancel need a reserved state; simpler v1 may consume only on `paid` and accept oversell of caps under unpaid holds (5 min auto-cancel helps).
2. **Self-referral?** Block `partner_id` == purchaser, or ignore for v1?
3. **Rename admin nav** from “Coupons” to “Promo codes”? (i18n only; paths stay.)
4. **Fixed KRW in Phase 1 or 3?** Phase 1 can stay percent + none if schedule is tight.
5. **Stats from order DB vs product ledger?** Order already has `coupon_code` — product stats can query via gateway or denormalized ledger. Prefer ledger + order filter consistency.

## Exit criteria (feature complete for Phases 1–2)

- [ ] Expired / capped codes rejected at redeem and checkout complete
- [ ] Track-only and hybrid codes store `coupon_code` on paid orders with correct `discount_cents`
- [ ] Manager can see redemption count and paid GMV for a code / partner
- [ ] Unpaid cancel does not permanently burn a capped redemption (if reserve model chosen)
- [ ] Tests cover product + order paths; [api.md](api.md) / [current-state.md](current-state.md) updated

## Suggested sequencing vs releases

```text
v1.0 / v1.1  — no dependency; do not block launch
v1.2+        — Phase 0–2 with other commerce UX
later        — Phase 3–4 as needed
```
