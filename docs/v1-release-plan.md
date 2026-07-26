# Dupli1 v1.0 release plan

**Status:** In progress (2026-07-26) — repo-side work in section C is done; section A/B are operator + sibling-repo work.  
**Scope:** Backend repo `dupli1` + production ops needed for a KRW fashion-bag marketplace launch.  
**Sibling frontends:** `dupli1-web`, `dupli1-manage-web` (called out where they block launch).

**Related:** [current-state.md](current-state.md), [TODO.md](TODO.md), [v1.1-release-plan.md](v1.1-release-plan.md), [quality-bugs-fix-plan.md](quality-bugs-fix-plan.md), [payment-methods-plan.md](payment-methods-plan.md).

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

1. **Deploy latest product** so `official_price` migrate + drop of `selling_price` has run on RDS.
2. **Product images CDN** — Terraform apply CloudFront + OAC; set `S3_PUBLIC_ENDPOINT`; rewrite existing `imageUrls` if hosts are stale.
3. **Stripe live** — `STRIPE_SECRET_KEY` + webhook secret; confirm Bypass only for managers.
4. **Persistent JWT signing key** — no ephemeral RSA on auth restart. Code + Terraform support landed (`JWT_PRIVATE_KEY`); the secret still has to be created and `jwt_private_key_secret_arn` set.
5. **Telegram** — Secrets Manager wired if ops alerts are required.
6. **Smoke the money path on prod** — add to cart → checkout → pay → `paid` → ship → stock committed / `soldCount` up.
7. **Confirm catalog prices** — spot-check non-zero `price` / `officialPrice` after earlier wipe + migrate.

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
- [ ] Runbook below executed for secrets / JWT / Stripe / images

Then tag **v1.0** and execute **v1.1** from [v1.1-release-plan.md](v1.1-release-plan.md) (themes, slices, exit criteria). The postpone table above remains the inventory of deferred items.

---

## Launch runbook (section A)

Operator steps that cannot be done from the repo. Region `us-east-1`, ECS cluster
`production`, Terraform in `infra/terraform/`. Architecture reference:
[deployment-aws.md](deployment-aws.md).

### 1. Persistent JWT signing key

Without this, auth mints a new RSA key on every task start: all outstanding access and
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

### 2. Product images CDN

Terraform already declares CloudFront + OAC, the bucket policy and the
`S3_PUBLIC_ENDPOINT` task env. Apply it, then repoint any rows still holding raw S3 hosts
([product-images-browser-access.md](product-images-browser-access.md)):

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

### 3. Stripe live keys

Set `STRIPE_SECRET_KEY` and `STRIPE_WEBHOOK_SECRET` from Secrets Manager on
`dupli1-payment`, and register the endpoint
`https://dupli1.com/api/v1/payments/webhooks/stripe` in the Stripe dashboard. Bypass stays
gated on `payment.bypass` ([payment-service.md](payment-service.md),
[payment-methods-plan.md](payment-methods-plan.md)). Verify:

```bash
curl -s https://dupli1.com/api/v1/payments/settings \
  | jq '{stripe_checkout: .features.stripe_checkout,
         stripe_webhook: .features.stripe_webhook,
         dev_simulate_success: .features.dev_simulate_success}'
# want: stripe_checkout true, stripe_webhook true, dev_simulate_success false
```

`dev_simulate_success` is derived from an empty `STRIPE_SECRET_KEY`, so a `true` here means
the live key never reached the task.

### 4. Telegram ops alerts

Populate `dupli1/production/telegram` (`TELEGRAM_BOT_TOKEN`, `TELEGRAM_ORDER_CHAT_ID`,
`TELEGRAM_PRODUCT_CHAT_ID`) and redeploy `dupli1-notification`. A missing token is not an
error — messages are skipped and logged, so check the log line if nothing arrives. Handler
failures are logged as `notification nats handler subject=… error=…`; core NATS does not
redeliver, so a logged failure means that one alert was lost.

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

Against production, with a throwaway customer account: add to cart → create a checkout
session → complete it → pay by card → order reaches `paid` → `POST /orders/{id}/ship`
returns `in_transit` → stock decremented and `soldCount` incremented on the PDP. Confirm
the `order.paid` Telegram alert arrived.

---

## Suggested v1.1 first slice (after launch)

Authoritative plan: [v1.1-release-plan.md](v1.1-release-plan.md).

1. Guest cart + merge (`dupli1_guest`) — **P0**  
2. Refunds on paid cancel — **P0**  
3. Remove legacy API aliases once clients are clean — **P1**  
4. Co-view recommendations — **P1**  
5. H6 context + Redis cache — **P2**  
6. Structured API errors + log messages — **P1**  
7. Manager settings + SKU master admin UI — **P2** / sibling  

---

## Doc maintenance in this cut

- Keep [TODO.md](TODO.md) as the checklist; this file is the **v1.0 release boundary**.
- Post-launch scope: [v1.1-release-plan.md](v1.1-release-plan.md).
- Done in this cut: order status machine and canonical paths in [api.md](api.md) / [permissions.md](permissions.md) / `api/specs/order-v1.yaml`; inventory, catalog and coupon coverage in `api/specs/product-v1.yaml`; [openapi.yaml](openapi.yaml) rebuilt as a full gateway index; [current-state.md](current-state.md) “Known gaps” refreshed (the “client-trusted prices” risk was already fixed and is not listed).
- Still to update at ship time: tick the exit criteria above and record the production values (CDN domain, JWT key secret ARN) in [deployment-aws.md](deployment-aws.md).
