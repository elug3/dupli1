# Dupli1 v1.0 release plan

**Status:** Planning (2026-07-26)  
**Scope:** Backend repo `dupli1` + production ops needed for a KRW fashion-bag marketplace launch.  
**Sibling frontends:** `dupli1-web`, `dupli1-manage-web` (called out where they block launch).

**Related:** [current-state.md](current-state.md), [TODO.md](TODO.md), [quality-bugs-fix-plan.md](quality-bugs-fix-plan.md), [payment-methods-plan.md](payment-methods-plan.md).

---

## Verdict

The **money path is implemented**: cart → checkout/order → Stripe/Bypass → `payment.succeeded` → `paid` → ship → stock commit. Critical money/auth bugs from the Jul review (server-side pricing, JWT fail-closed, outboxes) are done.

**v1.0 is a launch cut**, not feature-complete. Ship a reliable KRW checkout loop with catalog, inventory, and ops alerts. Defer guest commerce, Bitcoin, refunds automation, co-view recs, chat/analytics, and deep cleanup to **v1.1**.

---

## What already works (do not rebuild)

| Area | Notes |
|------|--------|
| Auth | Login → refresh → access JWT; JWKS; permissions; user admin; rate limits |
| Product | Parent + variants; `price` / `officialPrice`; bag taxonomy; SKU masters; search; wishlist; views; soldCount; content recs |
| Inventory | Stock + reserve/commit/release in product service |
| Cart | JWT cart; server-sourced prices |
| Order | Checkout sessions; idempotency; unpaid expiry; ship + stock commit |
| Payment | Stripe Checkout + webhook; Bypass; KRW; outbox/reconcile |
| Notification | Telegram on order/product events |
| AWS | ECS/ALB/RDS/Secrets; cart + payment on ECS; HTTPS on ALB |

---

## Known bugs / residual risks

| Risk | Severity | v1.0 action |
|------|----------|-------------|
| Product image URLs still point at private S3 (browser 403) if CloudFront/`S3_PUBLIC_ENDPOINT` not applied | **Launch-blocker** | Apply CDN + rewrite hosts — [product-images-browser-access.md](product-images-browser-access.md) |
| Frontends still on legacy paths / human `sku` only (not `skuId`) / old price fields | **Launch-blocker** | Migrate storefront + admin before cut |
| Catalog prices wiped to `0` (historical partial-PUT bug) | **Data** | Verify prod prices; restore snapshot/PITR or re-enter if still zero |
| `MergeUpdate` cannot deliberately set `price`/`officialPrice` to `0` | Low | Document; rare admin case |
| Auth `user.registered` NATS publish fails → register rolls back (no consumer) | Med | Soft-success publish for v1.0 if register is used in prod |
| Notification handler errors silently discarded | Med | Log errors before launch |
| Stale docs (`api.md` still mentions `confirmed`; `current-state` still flags client prices) | Med | Fix docs in v1.0 cut |
| `api/nginx.prod.conf` missing cart/payment/variants (Compose “prod” file) | Med | Confirm live uses `nginx.ecs.conf`; do not deploy broken prod conf |
| OpenAPI lag (inventory, catalog, coupons canonical paths) | Low–med | Refresh specs for external integrators |
| Admin/owner never locked out | Accepted | Keep rate limits + strong passwords |

Money-path Criticals **C1 / H1 / H3 / H7** are fixed in code — re-verify on the deployed revision, don’t re-implement.

---

## v1.0 — must ship

### A. Launch ops (production)

1. **Deploy latest product** so `official_price` migrate + drop of `selling_price` has run on RDS.
2. **Product images CDN** — Terraform apply CloudFront + OAC; set `S3_PUBLIC_ENDPOINT`; rewrite existing `imageUrls` if hosts are stale.
3. **Stripe live** — `STRIPE_SECRET_KEY` + webhook secret; confirm Bypass only for managers.
4. **Persistent JWT signing key** — no ephemeral RSA on auth restart.
5. **Telegram** — Secrets Manager wired if ops alerts are required.
6. **Smoke the money path on prod** — add to cart → checkout → pay → `paid` → ship → stock committed / `soldCount` up.
7. **Confirm catalog prices** — spot-check non-zero `price` / `officialPrice` after earlier wipe + migrate.

### B. Client alignment (sibling repos)

1. Canonical API paths (`/api/v1/products/…`, `/api/v1/orders/…`, `/api/v1/cart/…`) — keep legacy aliases until done.
2. Use parent `price` + `officialPrice` (not `sellingPrice` / SKU price).
3. Prefer `skuId` for cart/order/inventory; keep displaying human `sku`.
4. Stock UX: call inventory (or accept oversell risk) — PDP `inStock` enrichment is **not** required if clients poll inventory.

### C. Backend hardening (small, launch-relevant)

1. Soft-success auth `user.registered` publish (don’t delete user if NATS flaps).
2. Log notification NATS handler failures.
3. Fix stale API docs status machine (`paid` / `in_transit`, not `confirmed`).
4. Refresh OpenAPI enough for storefront/admin (inventory + catalog + coupons).

### D. Explicitly **out of** v1.0 (still OK to leave as 501 / unfinished)

- Bitcoin payment method  
- Automated Stripe refunds on paid cancel  
- Guest cart / merge / `POST /cart/checkout`  
- Co-view recommendations  
- Manager settings API  
- Email/SMS notifications  
- Chat, user profiles, analytics packages  
- Dropping legacy nginx aliases (after clients migrate — can start in v1.0, finish in v1.1)

---

## v1.1 — postpone

Grouped by theme. None of these should delay a KRW card-checkout launch.

### Commerce UX

| Item | Why wait |
|------|----------|
| Guest cart + merge on login | Needs cookie identity end-to-end; cart already works for logged-in |
| `POST /api/v1/cart/checkout` | Clients can copy lines today |
| Stock enforcement on add-to-cart | Reserve-at-order is enough for v1.0 |
| PDP `inStock` enrichment | Clients can hit inventory API |
| Auth-aware draft PDP for managers | Use manage list/admin APIs |
| Paid cancel → Stripe refund | Manual refund in Stripe Dashboard for early ops |

### Payments & messaging

| Item | Why wait |
|------|----------|
| Bitcoin (`bitcoin` → 501 today) | Explicitly planned; do not implement yet |
| Chargebacks / refund APIs | Payment phase 2 |
| Email / SMS notifications | Telegram covers ops |
| Notification queue group + retries | Log first; JetStream/DLQ later |

### Catalog / discovery

| Item | Why wait |
|------|----------|
| Co-view / also-bought recommendations | Content + popularity is enough |
| Drop legacy parent `color`/`stock`/`imageUrls` columns | Transitional mirrors still useful |
| Remove legacy API path aliases + nginx locations | After frontend migration completes |
| SKU master Phase D (manage-web UI) | API exists; UI can follow |
| Redis catalog cache + cart batch client | Latency polish |
| Product PG request `context` plumbing (**H6**) | Correctness polish, not launch |

### Platform / product surface

| Item | Why wait |
|------|----------|
| Manager settings API | Sketch only |
| User / chat / analytics services | Not started |
| Password reset / OAuth / email verify | Ops create accounts for now |
| Formal SQL migrations directory | Inline migrate works |
| Local TLS in Compose | Prod has ALB HTTPS |
| AWS cost orphan cleanup / CI OIDC for backend | Cost & hygiene |
| HTTP→HTTPS ALB `:80` redirect align | Confirm live vs Terraform |
| Align frontend CI task defs with live EC2 bridge | Ops cleanup |

---

## Suggested v1.0 exit criteria

Ship when all are true:

- [ ] Storefront can browse bags, see images, add to cart, checkout, pay with card, see order `paid`
- [ ] Ops can ship → `in_transit` and stock commits
- [ ] Bypass works for managers only; simulate-success off in prod
- [ ] Telegram (or accepted alternative) fires on `order.paid`
- [ ] No known zeroed catalog prices on live products
- [ ] Frontends use canonical paths + parent pricing (+ `skuId` preferred)
- [ ] Runbook: [deployment-aws.md](deployment-aws.md) followed for secrets/JWT/Stripe

Then tag **v1.0** and execute **v1.1** from [v1.1-release-plan.md](v1.1-release-plan.md) (themes, slices, exit criteria). The postpone table above remains the inventory of deferred items.

---

## Suggested v1.1 first slice (after launch)

Authoritative plan: [v1.1-release-plan.md](v1.1-release-plan.md).

1. Guest cart + merge (`dupli1_guest`) — **P0**  
2. Refunds on paid cancel — **P0**  
3. Remove legacy API aliases once clients are clean — **P1**  
4. Co-view recommendations — **P1**  
5. H6 context + Redis cache — **P2**  
6. Manager settings + SKU master admin UI — **P2** / sibling  

---

## Doc maintenance in this cut

- Keep [TODO.md](TODO.md) as the checklist; this file is the **v1.0 release boundary**.
- Post-launch scope: [v1.1-release-plan.md](v1.1-release-plan.md).
- Update [current-state.md](current-state.md) “Known gaps” when v1.0 ships (remove stale “client-trusted prices” risk — fixed).
