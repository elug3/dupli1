# Dupli1 v1.0 release plan

**Status:** **Postponed** (2026-07-27) — v1.0 will not ship until every open item in [v1.0-release-spec.md](v1.0-release-spec.md) sections A–F is resolved (ops, smoke, frontends, sign-off — not only backend hardening).  
**Closeout checklist:** [v1.0-release-spec.md](v1.0-release-spec.md) — authoritative spec + ship checklist.  
**Scope:** Backend repo `dupli1` + production ops needed for a KRW fashion-bag marketplace launch.  
**Sibling frontends:** `dupli1-web`, `dupli1-manage-web` (called out where they block launch).

**Related:** [current-state.md](current-state.md), [TODO.md](TODO.md), [v1.0-release-spec.md](v1.0-release-spec.md), [v1.1-release-plan.md](v1.1-release-plan.md), [quality-bugs-fix-plan.md](quality-bugs-fix-plan.md), [payment-methods-plan.md](payment-methods-plan.md).

---

## Verdict

The **money path is implemented**: cart → checkout/order → Stripe/Bypass → `payment.succeeded` → `paid` → ship → stock commit. Critical money/auth bugs from the Jul review (server-side pricing, JWT fail-closed, outboxes) are done. Backend hardening (section C) is **done in the repo**.

**v1.0 is postponed** until all launch-blockers and checklist items in [v1.0-release-spec.md](v1.0-release-spec.md) are closed — product images CDN, persistent JWT, Telegram wiring, and gateway ECS conf are done; **Stripe/card PG is waived** (PG TBD). Remaining: disable prod simulate (A5), catalog prices (A9), prod smoke (Bypass path), and `dupli1-web` / `dupli1-manage-web` alignment. **v1.0 is a launch cut**, not feature-complete: when unblocked, ship a reliable KRW checkout loop with catalog, inventory, Bypass pay, and ops alerts. Defer guest commerce, refunds, co-view recs, and deep product cleanup to **v1.2**. **v1.1** (logging, deployment, automation) starts only after v1.0 tags — see [v1.1-release-plan.md](v1.1-release-plan.md).

---

## What already works (do not rebuild)

| Area | Notes |
|------|--------|
| Auth | Login → refresh → access JWT; JWKS; permissions; user admin; rate limits |
| Product | Parent + variants; `price` / `officialPrice`; bag taxonomy; SKU masters; search; wishlist; views; soldCount; content recs |
| Inventory | Stock + reserve/commit/release in product service |
| Cart | JWT cart; server-sourced prices |
| Order | Checkout sessions; idempotency; unpaid expiry; ship + stock commit |
| Payment | Bypass + local simulate; KRW; outbox/reconcile (no Stripe) |
| Notification | Telegram on order/product events |
| AWS | ECS/ALB/RDS/Secrets; cart + payment on ECS; HTTPS on ALB |

---

## Known bugs / residual risks

| Risk | Severity | v1.0 action |
|------|----------|-------------|
| Product image URLs still point at private S3 (browser 403) if CloudFront/`S3_PUBLIC_ENDPOINT` not applied | Fixed | Prod `imageUrls` use CloudFront — [product-images-browser-access.md](product-images-browser-access.md) |
| Frontends still on legacy paths / human `sku` only (not `skuId`) / old price fields | **Launch-blocker** | Migrate storefront + admin before cut |
| Catalog prices wiped to `0` (historical partial-PUT bug) | **Data** | Verify prod prices; restore snapshot/PITR or re-enter if still zero |
| `MergeUpdate` cannot deliberately set `price`/`officialPrice` to `0` | Low | **Documented** in [product-price-on-parent.md](product-price-on-parent.md) + [api.md](api.md); rare admin case |
| Auth `user.registered` NATS publish fails → register rolls back (no consumer) | Med | **Fixed** — publish is best-effort; failure is logged and the account survives |
| Notification handler errors silently discarded | Med | **Fixed** — NATS dispatch logs subject + error |
| Stale docs (`api.md` still mentions `confirmed`) | Med | **Fixed** — status machine, persistence and canonical paths corrected in `api.md`, `permissions.md`, `api/specs/order-v1.yaml` |
| `api/nginx.prod.conf` missing cart/payment/variants (Compose “prod” file) | Med | **Fixed** — cart/payment/notification/variants locations added, body limit raised to 20m. Live ECS still uses `nginx.ecs.conf`, which was already complete |
| OpenAPI lag (inventory, catalog, coupons canonical paths) | Low–med | **Fixed** — `api/specs/product-v1.yaml` covers all 41 canonical product routes; `docs/openapi.yaml` is a complete 67-path gateway index |
| Admin/owner never locked out | Accepted | Keep rate limits + strong passwords |

Money-path Criticals **C1 / H1 / H3 / H7** are fixed in code — re-verify on the deployed revision, don’t re-implement.

---

## v1.0 — must ship

### A. Launch ops (production)

1. [x] **Deploy latest product** so `official_price` migrate + drop of `selling_price` has run on RDS (API serves `officialPrice`).
2. [x] **Product images CDN** — CloudFront + OAC live; prod `imageUrls` use CloudFront hosts.
3. [x] **Card PG** — **waived**: not contracting Stripe for v1.0; PG company TBD. Launch pay path = manager **Bypass**.
4. [x] **Persistent JWT signing key** — secret `dupli1/production/jwt-private-key` wired; prod `ephemeral_jwt_key` is `false`.
5. [x] **Telegram** — Secrets Manager wired into `dupli1-notification`.
6. **Smoke the money path on prod** — add to cart → checkout → **Bypass pay** → `paid` → ship → stock committed / `soldCount` up. Confirm `dev_simulate_success` is `false`.
7. **Confirm catalog prices** — spot-check non-zero `price` / `officialPrice` after earlier wipe + migrate (live catalog currently has `price: 0` on the sample bag — A9).
8. **A5** — after deploying payment with `PAYMENT_ALLOW_DEV_SIMULATE` unset, confirm simulate is off.

Step-by-step commands and verification for all of these: [launch runbook](#launch-runbook-section-a) below.

### B. Client alignment (sibling repos)

1. Canonical API paths (`/api/v1/products/…`, `/api/v1/orders/…`, `/api/v1/cart/…`) — keep legacy aliases until done.
2. Use parent `price` + `officialPrice` (not `sellingPrice` / SKU price).
3. Prefer `skuId` for cart/order/inventory; keep displaying human `sku`.
4. Stock UX: call inventory (or accept oversell risk) — PDP `inStock` enrichment is **not** required if clients poll inventory.

### C. Backend hardening (small, launch-relevant) — **done**

1. [x] Soft-success auth `user.registered` publish (don’t delete user if NATS flaps).
2. [x] Log notification NATS handler failures.
3. [x] Fix stale API docs status machine (`paid` / `in_transit`, not `confirmed`).
4. [x] Refresh OpenAPI enough for storefront/admin (inventory + catalog + coupons).
5. [x] Complete `api/nginx.prod.conf` (cart / payment / notification / variants + 20m body limit).

### D. Explicitly **out of** v1.0 (still OK to leave as 501 / unfinished)

- Bitcoin payment method  
- Automated Stripe refunds on paid cancel  
- Guest cart / merge / `POST /cart/checkout`  
- Co-view recommendations  
- Manager settings API  
- Email/SMS notifications  
- Chat, user profiles, analytics packages  
- Dropping legacy nginx aliases (after clients migrate — can start in v1.0, finish in v1.2)

---

## v1.1 — platform (post-launch)

Authoritative plan: [v1.1-release-plan.md](v1.1-release-plan.md). **Logging, sessions, access control, deployment, automation** — not commerce features.

| Item | Theme |
|------|--------|
| Structured API errors + **zerolog** logging | Logging |
| Consistent refresh sessions (auth + BFF cookie contract) | Sessions |
| Verify ABAC + permissions on money-path routes | Access control |
| BFF logout revokes refresh session (`dupli1-web`) | Sessions |
| AWS cost orphan cleanup, ALB redirect, nginx/ECS alignment | Deployment |
| Backend CI OIDC, frontend task-def alignment, deploy smoke | Automation |
| Notification handler logging, auth register soft-success | Logging / hygiene |

Commerce items below move to **v1.2**.

## v1.2 — commerce & product (deferred from v1.0)

Grouped by theme. None of these should delay v1.0 launch or v1.1 platform work.

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

---

## Suggested v1.0 exit criteria

**Authoritative checklist:** [v1.0-release-spec.md](v1.0-release-spec.md) (sections A–F with owners and Required/Recommended).

Summary — ship when all are true:

- [ ] Storefront can browse bags, see images, add to cart, checkout; pay via **Bypass** (or future PG) → order `paid`
- [ ] Ops can ship → `in_transit` and stock commits
- [ ] Bypass works for managers only; simulate-success off in prod (`PAYMENT_ALLOW_DEV_SIMULATE` unset)
- [ ] Telegram (or accepted alternative) fires on `order.paid`
- [ ] No known zeroed catalog prices on live products
- [ ] Frontends use canonical paths + parent pricing (+ `skuId` preferred)
- [ ] Runbook below executed for secrets / JWT / images (Stripe N/A — PG TBD)

Then tag **v1.0** and execute **v1.1** from [v1.1-release-plan.md](v1.1-release-plan.md) (themes, slices, exit criteria). The postpone table above remains the inventory of deferred items.

---

## Launch runbook (section A)

Operator steps that cannot be done from the repo. Region `us-east-1`, ECS cluster
`production`, Terraform in `infra/terraform/`. Architecture reference:
[deployment-aws.md](deployment-aws.md).

### 1. Persistent JWT signing key — **done**

Prod auth injects `JWT_PRIVATE_KEY` from `dupli1/production/jwt-private-key` and reports
`features.ephemeral_jwt_key: false`. Historical create/apply steps (kept for ops replay):

Without a persistent key, auth mints a new RSA key on every task start: all outstanding access and
refresh tokens break and the other services see a changed JWKS. Auth reads the PEM from
`JWT_PRIVATE_KEY` (see [deployment-aws.md](deployment-aws.md#jwt-signing-key)).

```bash
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out jwt-private-key.pem
aws secretsmanager create-secret --name dupli1/production/jwt-private-key \
  --secret-string "file://jwt-private-key.pem"
# set jwt_private_key_secret_arn to the returned ARN, then:
terraform -chdir=infra/terraform apply
aws ecs update-service --cluster production --service dupli1-auth --force-new-deployment
shred -u jwt-private-key.pem
```

Verify — `ephemeral_jwt_key` must be `false`, and the `kid`/`n` must survive a restart:

```bash
curl -s https://dupli1.com/api/v1/auth/settings | jq .features.ephemeral_jwt_key
curl -s https://dupli1.com/api/v1/auth/.well-known/jwks.json | jq -r '.keys[0].n' | sha256sum
```

### 2. Product images CDN — **done**

Prod `imageUrls` already use CloudFront. No further apply/rewrite required unless
hosts regress to raw S3. Reference: [product-images-browser-access.md](product-images-browser-access.md).

Historical apply / rewrite steps (kept for ops replay):

Terraform already declares CloudFront + OAC, the bucket policy and the
`S3_PUBLIC_ENDPOINT` task env. Apply it, then repoint any rows still holding raw S3 hosts:

```bash
terraform -chdir=infra/terraform apply
terraform -chdir=infra/terraform output product_images_cdn_url
aws ecs update-service --cluster production --service dupli1-product --force-new-deployment
```

```sql
-- image_urls is TEXT[] on product_variants and on the legacy parent products column.
-- Check first, and match whatever host the rows actually hold:
SELECT sku, image_urls FROM product_variants
WHERE EXISTS (SELECT 1 FROM unnest(image_urls) AS u WHERE u LIKE '%s3.amazonaws.com%');

UPDATE product_variants
SET image_urls = ARRAY(
  SELECT replace(u, 'https://dupli1-production-product-images.s3.amazonaws.com',
                    'https://images.dupli1.com')
  FROM unnest(image_urls) AS u
)
WHERE EXISTS (SELECT 1 FROM unnest(image_urls) AS u WHERE u LIKE '%s3.amazonaws.com%');
-- repeat for products.image_urls if legacy parent rows still carry absolute URLs
```

Verify a PDP image URL returns `200` from a plain browser fetch (no signature), not `403`.

### 3. Card PG — **waived for v1.0** (Stripe adapter removed)

Dupli1 has **no card PG adapter** in-tree; the company has not chosen a PG yet.
Credit-card checkout stays unavailable in production until a provider is wired.
Ops marks orders paid with **Bypass** (`payment.bypass`).

**Important:** `simulate-success` requires an explicit `PAYMENT_ALLOW_DEV_SIMULATE=true`
(Compose sets this for local). On ECS leave it **unset** so prod cannot simulate.
Verify after deploying that payment build:

```bash
curl -s https://dupli1.com/api/v1/payments/settings \
  | jq '{dev_simulate_success: .features.dev_simulate_success,
         checkout_provider: .limits.checkout_provider}'
# want: dev_simulate_success false, checkout_provider "none"
```

Bypass stays gated on `payment.bypass` ([payment-service.md](payment-service.md),
[payment-methods-plan.md](payment-methods-plan.md)).

### 4. Telegram ops alerts — **wired**

Secret `dupli1/production/telegram` is injected into `dupli1-notification`. A missing token is not an
error — messages are skipped and logged, so check the log line if nothing arrives. Handler
failures are logged as `notification nats handler subject=… error=…`; core NATS does not
redeliver, so a logged failure means that one alert was lost. Confirm delivery on the next paid-order smoke (B7).

### 5. Catalog price check

An earlier partial-PUT bug wrote `price = 0`. Confirm nothing live is still zeroed:

```sql
SELECT id, name, price, official_price FROM products
WHERE status = 'active' AND (price IS NULL OR price = 0);
```

Restore from snapshot/PITR or re-enter the values. Note that sending `price: 0` through
the API is ignored by design ([product-price-on-parent.md](product-price-on-parent.md)), so
zeroed rows must be fixed with a real amount.

### 6. Money-path smoke test

[`scripts/smoke-money-path.sh`](../scripts/smoke-money-path.sh) walks the whole path —
catalog → cart → checkout session → payment → `paid` → ship → `fulfilled` — and asserts
the launch-critical invariants: the cart and session prices come from the catalog and not
from the client, stock is reserved at checkout and committed on ship, `soldCount` follows
the commit, `method=bypass` is refused for a plain customer, and `confirmed` is rejected.

```bash
# against a real product, so nothing test-shaped is created in the catalog
BASE=https://dupli1.com SKU_ID=<existing skuId> scripts/smoke-money-path.sh
```

It registers a throwaway customer and creates a real order. With live Stripe keys the
script prints the Checkout URL and waits while you complete the card payment; without them
it drives the dev simulate endpoint itself. Omitting `SKU_ID` makes it seed its own
`SMK`-brand product, which is what you want locally but not in the live catalog.

Finally confirm the `order.paid` Telegram alert arrived.

---

## Suggested v1.1 first slice (after launch)

Authoritative plan: [v1.1-release-plan.md](v1.1-release-plan.md).

1. Structured API errors + **zerolog** logging — **P0**  
2. Consistent sessions + BFF revocable logout — **P0**  
3. Verify access control (ABAC + permissions) — **P0**  
4. AWS deployment alignment — **P0**  
5. CI/CD automation (OIDC, smoke) — **P1**  

Commerce backlog (guest cart, refunds, co-view, …) → **v1.2**.

---

## Doc maintenance in this cut

- Checkboxes live in [v1.0-release-spec.md](v1.0-release-spec.md); this file is the **narrative boundary** plus the operator runbook, and [TODO.md](TODO.md) links to the open items rather than repeating them.
- Post-launch scope: [v1.1-release-plan.md](v1.1-release-plan.md).
- Done in this cut: order status machine and canonical paths in [api.md](api.md) / [permissions.md](permissions.md) / `api/specs/order-v1.yaml`; inventory, catalog and coupon coverage in `api/specs/product-v1.yaml`; [openapi.yaml](openapi.yaml) rebuilt as a full gateway index; [current-state.md](current-state.md) “Known gaps” refreshed (the “client-trusted prices” risk was already fixed and is not listed).
- Still to update at ship time: tick the exit criteria above and record the production values (CDN domain, JWT key secret ARN) in [deployment-aws.md](deployment-aws.md).
