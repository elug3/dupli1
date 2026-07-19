# Dupli1 quality and performance review (2026-07-15)

Audit of the Go microservice backend (`auth/`, `product/`, `order/`, `cart/`, `payment/`, `notification/`, `shared/`, `api/`). Findings are ordered by severity. Items marked **Fixed in this PR** were addressed alongside the review; the rest remain recommended follow-ups.

## Verdict

Architecture (hexagonal DDD per service, JWT/JWKS auth, PostgreSQL, NATS payment events) is coherent for a marketplace MVP. The highest risks are **money-path correctness** (client-trusted prices, payment event loss on retry, checkout ABAC hole) and **catalog/cart latency** (unbounded product search, sequential cart enrichment). Notification remains thin.

---

## Critical

| # | Finding | Status |
|---|---------|--------|
| C1 | **Client-controlled order prices** — `POST /orders` and checkout item APIs accept `unit_price_cents` from the client; totals and Stripe amounts derive from that. | Open — resolve price from product at order/checkout time |
| C2 | **Unauthenticated `simulate-success`** — `GET /api/v1/payments/{id}/simulate-success` completes payment and publishes `payment.succeeded` with no auth. | **Fixed** — route only registered when Stripe is unset (`allowDevSimulate`) |
| C3 | **Checkout delete-by-skuId skips ownership check** — `DELETE …/items/by-sku-id/{id}` omitted `withCheckoutSessionAccess`. | **Fixed** |
| C4 | **Payment succeeded + failed NATS publish = stuck order** — `CompletePayment` saves `succeeded` then publishes; on publish failure, retry returns early without republishing. | **Fixed** — already-succeeded payments republish the event (order `MarkOrderPaid` is idempotent) |

---

## High (quality / reliability)

| # | Finding | Status |
|---|---------|--------|
| H1 | Order create: save succeeds, publish failure can orphan reservations on client retry | Open — outbox or compensate |
| H2 | Order bootstrap fetches inventory service token once; never refreshes after expiry | **Fixed** — `ServiceAccountTokenSource` + `httpstock` client (product stock API); retries once on 401 |
| H3 | NATS subscribers discard handler errors (`_ = handler(...)`) — at-most-once, silent loss | Open — queue group + retry/DLQ |
| H4 | Internal `err.Error()` returned on many 500 responses (auth, product, order/cart/payment) | **Partial** — product sanitizes 500s via error wrapping; see [product-error-wrapping.md](product-error-wrapping.md). Other services still open |
| H5 | Product PG migrations ignore some `Exec` errors during migrate/seed | Open |
| H6 | Product stores use `context.Background()` on request-path queries | Open — plumb request context |
| H7 | `requireAuth` no-ops when JWT validator is nil (order/cart/payment); product fails closed | Open |
| H8 | Duplicated `authjwt` in four services — drift risk | Open — move to `shared/` |
| H9 | JWKS refresh has no `singleflight` (thundering herd on cold start / key rotation) | Open |

---

## High (performance)

| # | Finding | Status |
|---|---------|--------|
| P1 | **Product search unbounded** — no `LIMIT`; full variant enrich for list | **Fixed** — default `limit=50`, max `100`, `offset` supported; response includes `limit`/`offset`; filter indexes added |
| P2 | **Cart enrichment O(N) sequential HTTP** — product + inventory per line; mutations re-`GetCart` | **Fixed (partial)** — parallel enrich with bounded concurrency; batch APIs still recommended |
| P3 | **Order list / expiry N+1** — `loadOrderItems` per order | **Fixed** — batch load by `order_id = ANY($1)` |
| P4 | Missing indexes: product filter/sort columns; pending expiry `(status, payment_due_at)` | **Fixed** — product filter indexes + partial index on pending `payment_due_at` |
| P5 | Untuned pgx/sql connection pools | Open |
| P6 | Local nginx variable `proxy_pass` disables upstream keepalive; missing proxy timeouts | Open |
| P7 | Order/checkout `Save` deletes and reinserts all items on every write | Open |

---

## Medium

- Cart/order HTTP clients fall back to `http.DefaultClient` (no timeout) if bootstrap omits a client
- Auth `ListAll` / `HasOwner` load entire users table (including password hashes for list)
- Brand filter uses leading-wildcard `ILIKE` (hard to index)
- Product ID generation does regex `MAX` scan instead of a sequence
- Stripe webhook body not size-limited (`io.ReadAll`)
- Variant event publish errors ignored inconsistently vs product create/update
- Redis unused outside auth — no catalog/cart cache
- Prod nginx (`api/nginx.prod.conf`) may lag local route coverage (cart/payment/variants) — verify against live

---

## Low

- Product in-memory store has no mutex (test-only risk)
- Auth ephemeral RSA key when PEM missing (tokens invalidate on restart)
- Options timeout type drift (`int` seconds vs `time.Duration`)
- Notification tests cover dispatcher only
- Startup inline migrations slow restarts as data grows

---

## Test coverage gaps

| Area | Gap |
|------|-----|
| Order | Checkout by-sku-id ABAC (now covered); payment consumer; expiry worker; price trust |
| Payment | simulate-success gating (now covered); publish-after-save retry (now covered); Stripe webhook |
| Product | PG store / migrate failure; pagination (now covered at handler level) |
| Cart | Enrichment failure / partial availability behavior |
| Notification | NATS subscriber + Telegram client |

---

## Hot-path summary

| Path | Bottleneck | Mitigation |
|------|------------|------------|
| Product search | Unbounded query + enrich | Pagination (this PR); add filter indexes next |
| Cart GET/mutate | Per-item product + inventory HTTP | Parallel enrich (this PR); batch APIs next |
| Checkout / create order | Client prices + HTTP reserve | Server-side price resolve (open) |
| Order list / expiry | N+1 items + missing index | Batch load + partial index (this PR) |
| Payment → order paid | Publish-after-save + swallowed NATS errors | Republish on retry (this PR); outbox next |

---

## Recommended priority (remaining)

1. **Server-side pricing** at order/checkout create (ignore client `unit_price_cents`)
2. ~~Inventory service **token refresh** in order bootstrap~~ **done**
3. **Transactional outbox** (or JetStream) for `payment.succeeded` / order events; stop swallowing NATS handler errors
4. Product **filter indexes** + request-context plumbing; slim list DTOs
5. Batch cart/product APIs (`?sku_ids=`); Redis cache for public catalog
6. Consolidate `authjwt` + shared HTTP client helpers; fail-closed auth bootstrap
7. Product: sanitize 500 responses (**done** — [product-error-wrapping.md](product-error-wrapping.md)); other services still need the same; check migrate `Exec` errors

See also: [TODO.md](TODO.md), [current-state.md](current-state.md), [aws-cost-optimization.md](aws-cost-optimization.md).
