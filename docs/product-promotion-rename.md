# `coupon` → `promotion` rename

**Status:** Planned — **Phase 1** of [product-promo-referral-code-plan.md](product-promo-referral-code-plan.md). Not started (2026-09-16).
**Repos:** `dupli1` (product, order, shared, api), `dupli1-web`, `dupli1-manage-web`.
**Related:** [product-promo-referral-code-plan.md](product-promo-referral-code-plan.md), [api.md](api.md), [permissions.md](permissions.md), [TODO.md](TODO.md), [current-state.md](current-state.md).

## Why

The product term is **"promotional code."** Decided 2026-09-16, reversing the earlier decision to keep a per-surface 쿠폰 / 프로모션 코드 split and to treat wire renames as a non-goal.

The rename is **full** — customer copy, admin UI, docs, HTTP paths, JSON fields, Postgres columns, Go identifiers and permissions. It lands **before** the promotional-code feature work (Phase 2 onward) so that new columns, JSONB documents, ledger tables and endpoints are never created under the old vocabulary and then renamed a second time.

`discount_won` already reads neutrally and **keeps its name**. So do `discount`, `description`, `expires`, `active` as field names within the definition.

## Precedent

This repo has done a rename of exactly this shape before: `*_cents` → `*_krw` → `*_won`. Reuse its mechanics rather than inventing new ones.

- **Column renames run inline at startup** via `renameColumnIfNeeded`, driven by a table of `{table, from, to}` — see `renameMoneyCentsColumns` in `order/pkg/infra/pg/repository.go:304`. It no-ops when the source column is absent or the target already exists, so it is safe on a fresh database and on a re-deploy.
- **Decoders accept the old key for one release** while nothing emits it. The money rename's residue is documented in the repo guide: *"Nothing emits the old names, but a few decoders still accept them."*
- **Old HTTP paths stay registered as aliases**, the same way the canonical `/api/v1/{service}/…` paths were introduced alongside legacy top-level prefixes.

The money rename's lesson is in the repo guide too: a short canonical window (2026-09-09 → 09-14) left stale branches and docs behind. Keep this rename's window short and land the frontends in the same release.

## Name map

### HTTP

| Before | After |
|--------|-------|
| `GET/POST /api/v1/products/coupons` | `GET/POST /api/v1/products/promotions` |
| `PUT/DELETE /api/v1/products/coupons/by-code/{code}` | `PUT/DELETE /api/v1/products/promotions/by-code/{code}` |
| `POST /api/v1/products/coupons/redeem` | `POST /api/v1/products/promotions/redeem` |
| `POST /api/v1/orders/checkout/sessions/{id}/coupon` | `POST /api/v1/orders/checkout/sessions/{id}/promotion` |
| Legacy `/api/v1/coupons`, `/api/v1/coupons/{code}`, `/api/v1/coupons/redeem` | Kept as-is, still aliases; retired with the other legacy prefixes ([TODO.md](TODO.md)) |

### JSON

| Before | After |
|--------|-------|
| `coupon_code` (order, checkout session) | `promotion_code` |
| `discount_won` | unchanged |
| Coupon object `{code, discount, description, expires, active}` | unchanged field names |

### Postgres

| Table | Before | After |
|-------|--------|-------|
| product | `coupons` | `promotions` |
| order | `orders.coupon_code` | `orders.promotion_code` |
| order | `checkout_sessions.coupon_code` | `checkout_sessions.promotion_code` |

Future tables from Phase 2/3 are created under the new names directly: `promotion_redemptions`, `customer_promotions`.

### Permissions

| Before | After |
|--------|-------|
| `coupon.read` | `promotion.read` |
| `coupon.create` | `promotion.create` |
| `coupon.update` | `promotion.update` |
| `coupon.delete` | `promotion.delete` |
| `coupon.*` | `promotion.*` |

Held in `shared/pkg/permissions/catalog.go`; `coupon.*` is a member of the `catalog_editor` and `catalog_admin` bundles (`bundles.go`) and appears in the legacy role expansions (`legacy.go`).

### Go identifiers

| Before | After |
|--------|-------|
| `product/pkg/domain.Coupon` | `domain.Promotion` |
| `product/pkg/ports.CouponStore` | `ports.PromotionStore` |
| `product/pkg/service.CouponService` | `service.PromotionService` |
| `product/pkg/infra/pg.CouponStore` | `pg.PromotionStore` |
| `product/pkg/infra/memory.CouponStore` | `memory.PromotionStore` |
| Handlers `ListCoupons` / `CreateCoupon` / `UpdateCoupon` / `DeleteCoupon` / `RedeemCoupon` | `ListPromotions` / `CreatePromotion` / `UpdatePromotion` / `DeletePromotion` / `RedeemPromotion` |
| Routes `RouteCoupons` / `RouteCouponByCode` / `RouteRedeemCoupon` | `RoutePromotions` / `RoutePromotionByCode` / `RouteRedeemPromotion` |
| `order/pkg/ports.Coupon` / `CouponClient` / `ErrCouponInvalid` / `ErrCouponUnavailable` | `ports.Promotion` / `PromotionClient` / `ErrPromotionInvalid` / `ErrPromotionUnavailable` |
| `order/pkg/infra/httpcoupon` | `order/pkg/infra/httppromotion` |
| `Service.ApplyCheckoutCoupon` | `ApplyCheckoutPromotion` |
| `CheckoutSession.ApplyCoupon` / `ClearCoupon` | `ApplyPromotion` / `ClearPromotion` |
| `CheckoutSession.CouponCode`, `Order.CouponCode` | `PromotionCode` |

### Frontends

| Repo | Before | After |
|------|--------|-------|
| manage-web | route `coupons` → `routes/coupons.tsx` | `promotions` → `routes/promotions.tsx` |
| manage-web | `getCoupons` / `createCoupon` / `updateCoupon` / `deleteCoupon`, `Coupon` / `CouponInput` / `CouponUpdate` | `getPromotions` / `createPromotion` / `updatePromotion` / `deletePromotion`, `Promotion` / `PromotionInput` / `PromotionUpdate` |
| manage-web | i18n `nav.coupons`, `coupons.*` block | `nav.promotions`, `promotions.*` |
| manage-web | sidebar `CouponsIcon`, label "Coupons" / "쿠폰" | `PromotionsIcon`, "Promotional codes" / "프로모션 코드" |
| web | route `api/coupons/redeem`, `api/v1/checkout/sessions/:id/coupon` | `api/promotions/redeem`, `…/:id/promotion` |
| web | `redeemCoupon`, `RedeemedCoupon`, `applySessionCoupon` | `redeemPromotion`, `RedeemedPromotion`, `applySessionPromotion` |
| web | `profile.coupons` = "Coupons" / "쿠폰", `profile.couponAdded`, `profile.invalidCoupon`, `profile.noActiveCoupons`, `profile.faqApplyCoupon` | `profile.promotions*` keys, all reading "promotional code" / "프로모션 코드" |
| web | `cart.promo`, `cart.promoCode`, `cart.invalidPromo`, `checkout.invalidPromo` | Wording aligned to "promotional code" / "프로모션 코드"; keys may stay |

The customer-facing split disappears: cart/checkout already say 프로모션 코드, the profile wallet says 쿠폰, and the wallet is the side that moves.

**Name collision to watch:** `dupli1-web` already has a `profile.promotions` key meaning marketing opt-in ("Promotions & offers"). Pick a distinct key for the wallet section (e.g. `profile.promotionCodes`) rather than overloading it.

### nginx

`location /api/v1/coupons` exists in all three configs — `api/nginx.conf:104`, `api/nginx.prod.conf:107`, `api/nginx.ecs.conf:81`. Add a matching `/api/v1/promotions` location to each; keep the old one until the legacy prefixes are dropped.

Per the repo guide, the `resolver` directive in `api/nginx.conf` must continue to list only `127.0.0.11`. After editing, rebuild the proxy alone: `sudo docker compose up -d --build dupli1-proxy`.

## Cutover order

One release, in this order, so nothing is ever broken between steps:

1. **shared** — add `promotion.*` permission constants; keep `coupon.*` defined and accepted so tokens minted before the rollout still authorize. Bundles gain the new names.
2. **product** — rename Go identifiers, table (`ALTER TABLE IF EXISTS coupons RENAME TO promotions`, guarded the same way `renameColumnIfNeeded` guards columns), routes. Register `/promotions…` and keep `/coupons…` + the legacy top-level aliases pointing at the same handlers. Accept either permission name on each route during the window.
3. **order** — rename identifiers and the `coupon_code` columns via the rename-table helper; emit `promotion_code` in JSON while the decoder still accepts `coupon_code`; register the `…/promotion` sub-route and keep `…/coupon`.
4. **nginx** — add the `/api/v1/promotions` locations in all three configs.
5. **frontends** — manage-web and dupli1-web switch to the new paths, helpers and copy in the same release. Both currently call the **legacy** `/api/v1/coupons` prefix, so this is also an opportunity to move them to canonical `/api/v1/products/promotions` and shorten the legacy-prefix backlog in [TODO.md](TODO.md).
6. **docs** — [api.md](api.md), [permissions.md](permissions.md), [current-state.md](current-state.md), the repo guide's coupon references.
7. **one release later** — drop the `coupon_code` decoder alias, the `coupon.*` permission acceptance and the `/coupons` routes, together with the other legacy prefixes.

## Risks

| Risk | Mitigation |
|------|------------|
| A token minted with `coupon.read` stops authorizing after deploy | Accept both names on each route for one release; only then remove |
| Frontend deployed before backend (or after) | Old paths stay registered through the window, so either order works |
| The `coupons` table rename runs against a DB that already has `promotions` | Guard with an existence check, as `renameColumnIfNeeded` does |
| A branch cut during the window still writes `coupon_code` | Decoder accepts it for one release; the repo guide gets a dated note like the `*_krw` one |
| Stale docs and branches (the `*_krw` lesson) | Keep the window short; land docs in the same release; note the window's dates in the repo guide |
| Rename churn collides with Phase 2 feature work | Rename **first**, as its own change, with no behavior change in the same commit |

## Definition of done

- [ ] `promotion.*` permissions exist, bundles updated, `coupon.*` still accepted
- [ ] Product serves `/api/v1/products/promotions…`; `coupons` paths still answer
- [ ] `coupons` table renamed to `promotions` with a guarded inline migration
- [ ] `orders.promotion_code` and `checkout_sessions.promotion_code` renamed via the existing helper
- [ ] Order emits `promotion_code`; `coupon_code` still decodes
- [ ] `/api/v1/promotions` nginx locations in `nginx.conf`, `nginx.prod.conf`, `nginx.ecs.conf`
- [ ] Both frontends call the canonical promotion paths and say "promotional code" / "프로모션 코드" on every surface
- [ ] No `Coupon` identifier left in `product/` or `order/` outside compatibility shims
- [ ] Existing tests renamed and passing; no behavior change in the rename commit
- [ ] [api.md](api.md), [permissions.md](permissions.md), [current-state.md](current-state.md) updated
