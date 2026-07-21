# Fix plan: remaining quality / security bugs (2026-07-20)

Concrete solutions for open findings from [quality-performance-review.md](quality-performance-review.md) and the weekly check. Feature / ops TODOs (Bitcoin, settings API, AWS cost cleanup) are out of scope here.

**Template to copy:** cart already server-sources prices (`cart/pkg/service` + `httpproduct`); product already sanitizes 500s ([product-error-wrapping.md](product-error-wrapping.md)) and fails closed on missing JWT at bootstrap.

---

## Implementation order

| Step | ID | Fix | Complexity |
|------|-----|-----|------------|
| 1 | **C1** | Server-side order/checkout pricing | Med |
| 2 | **H7** | Fail closed without JWT (+ Bypass hole) | Low–med |
| 3 | **H1** | Order create publish failure / orphan reservations | **Done** — idempotency + outbox |
| 4 | **H3** | NATS handler errors / redelivery | Med–high |
| 5 | **H4** | Sanitize auth/order/cart/payment 500s | Low |
| 6 | **H5** | Product migrate `Exec` error checks | Low |
| 7 | **H6** | Plumb request `context` through product stores | Med |
| 8 | **H8+H9** | Shared `authjwt` + JWKS `singleflight` | Med |

Do **C1** and **H7** first — active money/auth risk. **H1** outbox landed; reuse for **H3** / payment publish side. Bundle **H8** with **H9**.

---

## 1. C1 — Server-side pricing (Critical)

### Problem

`POST /orders` and checkout item APIs accept client `unit_price_cents`. Totals become Stripe / Bypass charge amounts. Cart already ignores client prices.

### Solution (copy cart)

1. Add order `ports.ProductClient` (`GetVariant` / `GetVariantBySkuID`, optional batch `?sku_ids=`).
2. Wire HTTP client in `order/pkg/bootstrap` via gateway / product URL (prefer `DUPLI1_GATEWAY_URL` once #111 lands).
3. In `CreateOrder`, `SetCheckoutItems`, `UpsertCheckoutItem`: resolve each line from product; set `UnitPriceCents = money.FromProductPrice(variant.Price)`; reject missing/inactive variants; **ignore** any client price.
4. Handler request DTO becomes `{sku|sku_id, quantity}` only (like cart `ItemInput`). Keep `unit_price_cents` on **responses** and DB as a snapshot.
5. Optional: re-resolve prices on `CompleteCheckout` so stale sessions cannot lock old prices.
6. Update `api/specs/order-v1.yaml`, [checkout-session.md](checkout-session.md), [endpoints.md](endpoints.md).
7. Test: client sends `unit_price_cents: 1` → stored/charged amount is catalog price.

### Done when

Order and checkout totals always equal sum of product catalog prices × qty (minus coupon). Payment/Stripe cannot charge a client-supplied amount.

---

## 2. H7 — Fail closed without JWT

### Problem

Order/cart/payment: if `jwtValidator == nil`, `requireAuth` calls `next` (unauthenticated). Payment also sets `AllowMethodBypass: jwtValidator == nil || CanBypassPayment(...)` — anyone can mark orders paid when JWKS is unset. Product refuses to start without JWT config.

### Solution

1. **Bootstrap:** order/cart/payment require `AUTH_JWKS_URL` or `JWT_SECRET` like product; refuse to start otherwise.
2. **Handler:** if validator is nil → `503 auth not configured` (never call `next`). Same for checkout `mayAccessCheckoutSession`.
3. **Payment:** `AllowMethodBypass` **only** when `jwtValidator != nil && permissions.CanBypassPayment(...)`.
4. **Tests:** use a fake validator; do not rely on nil for “open” protected routes. Optional `NewForTest` that is unreachable from production bootstrap.

### Done when

Misconfigured deploy cannot serve unauthenticated order/cart/payment or Bypass; tests still cover auth with fakes.

---

## 3. H1 — Order create publish failure orphans reservations

### Problem

`CreateOrder`: Reserve → Save → `publish(order.created)`. Publish failure returns error **without** releasing stock. Client retry reserves again; stock held until pending expiry (~5 min).

### Solution (implemented)

**C (outbox) + A (soft-success) + Idempotency-Key:**

- `SaveWithOutbox` persists order + optional `order_idempotency_keys` row + `order_outbox` events in one transaction.
- Create returns **201** after save; publish is best-effort via immediate drain + background outbox worker.
- `Idempotency-Key` on `POST /api/v1/orders`: same key + body returns the existing order (no second reserve); different body → **409**.

### Done when

Publish blip does not leak a second reservation; clients can safely retry create. **Done.**

---

## 4. H3 — NATS handler errors discarded

### Problem

Core NATS `Subscribe` does `_ = handler(...)`. Failures (e.g. `MarkOrderPaid`) are silent; no redelivery.

### Solution

1. **Immediate:** log subject + order/payment id on handler error.
2. Use `QueueSubscribe` when running multiple replicas.
3. Prefer **JetStream** (ack/nak, limited redelivery, DLQ) **or** transactional **outbox** on the publish side (align with H1 / payment `payment.succeeded`).
4. Keep `MarkOrderPaid` idempotent (already is for same payment id).

### Done when

Transient consumer failures are visible and retried; poison messages land in a DLQ / dead letter path.

---

## 5. H4 — Sanitize 500 responses

### Problem

Auth/order/cart/payment return raw `err.Error()` on many 500s (DB/driver leakage). Product maps unknown errors to `"internal error"` + log.

### Solution

Copy [product-error-wrapping.md](product-error-wrapping.md) / `product/pkg/handler/errors.go`:

- Known domain/ports sentinels → stable client messages + correct status.
- Default → log full error; respond `"internal error"`.
- Auth: only sanitize **500** branches; keep intentional 400/401/403 messages.

### Done when

No internal error strings in 500 JSON from auth/order/cart/payment under forced DB failures.

---

## 6. H5 — Product migrate ignores `Exec` errors

### Problem

`product/pkg/infra/pg/product_store.go` `migrate()` discards some `Exec` results (`ADD COLUMN`, `UPDATE`, indexes). Half-applied schema can still start.

### Solution

`_, err := pool.Exec(...); if err != nil { return fmt.Errorf("migrate …: %w", err) }`. `IF NOT EXISTS` no-ops remain success. Smoke-test migrate is idempotent and fails on bad SQL.

### Done when

Any migrate SQL failure aborts startup with a wrapped error.

---

## 7. H6 — Request context through product stores

### Problem

Product/variant/catalog/coupon/view PG stores use `context.Background()`; request cancel/deadline does not abort DB work. Inventory store already takes `ctx`.

### Solution

Add `ctx context.Context` as first arg on store interfaces + memory impls + service call sites; pass `r.Context()` from handlers. Keep `Background` only for migrate/seed/CLI.

### Done when

Cancelled request contexts cancel in-flight product store queries.

---

## 8. H8 + H9 — Shared `authjwt` + JWKS singleflight

### Problem

Four identical `*/pkg/authjwt` packages. On unknown `kid` / cold cache, every request can hit JWKS in parallel (no `singleflight`).

### Solution

1. Move to `shared/pkg/authjwt` (Claims, JWKS/HMAC validators, `NewAccessTokenValidator`, context helpers).
2. Add `golang.org/x/sync/singleflight.Group` around `refreshKeys`.
3. Point order/cart/payment/product at shared; delete local copies.
4. Optional: background TTL refresh.

### Done when

One shared package; concurrent unknown-`kid` requests share a single JWKS fetch.

---

## Suggested PR slicing

| PR | Contents |
|----|----------|
| A | **H7** fail-closed JWT + Bypass fix (small, urgent) |
| B | **C1** server-side pricing (order + checkout + OpenAPI + tests) |
| C | **H4** + **H5** (quick isolated cleanups) |
| D | **H1** soft-success or outbox + idempotency |
| E | **H3** logging → JetStream/outbox |
| F | **H6** context plumbing |
| G | **H8+H9** shared authjwt + singleflight |

---

## Related

- [quality-performance-review.md](quality-performance-review.md) — original findings
- [TODO.md](TODO.md) — checklist
- [product-error-wrapping.md](product-error-wrapping.md) — H4 template
- [cart-service.md](cart-service.md) — C1 pricing template
- [payment-methods-plan.md](payment-methods-plan.md) — Bypass permission model (H7)
