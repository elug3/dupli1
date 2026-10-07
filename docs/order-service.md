# Order Service

**Status:** Implemented (`order/`). Checkout sessions, order lifecycle, transactional outbox → NATS, and background policy workers.

The **order service** (`dupli1-order`) owns the short-lived **checkout session** and the long-lived **order**. It reserves stock on checkout `complete`, consumes **`payment.succeeded`** / **`payment.canceled`** from payment, calls product (stock/promotional codes) and payment (refunds) through the **nginx gateway** (`DUPLI1_GATEWAY_URL`), and publishes order events via a transactional **outbox**.

For checkout session field semantics see [checkout-session.md](checkout-session.md). For the money path and refund rules see [payment-service.md](payment-service.md). For the route-by-route API table see [api.md](api.md) and [endpoints.md](endpoints.md).

---

## Role in the purchase flow

```mermaid
flowchart LR
    Cart["dupli1-cart<br/>(intent)"]
    Order["dupli1-order<br/>(checkout + order)"]
    Product["dupli1-product<br/>(price, stock, promotions)"]
    Pay["dupli1-payment"]
    NATS["NATS"]

    Cart -.->|"client copies items"| Order
    Order -->|"reserve / commit stock"| Product
    Order -->|"evaluate / reserve promotion"| Product
    Pay -->|"payment.succeeded"| NATS
    NATS --> Order
    Order -->|"refund on cancel"| Pay
    Order -->|"order.* outbox"| NATS
```

| Phase | Service | Stock impact |
|-------|---------|--------------|
| Checkout session open | `dupli1-order` | None |
| Checkout `complete` | `dupli1-order` | **Reserved** via product |
| `paid` → `confirmed` | `dupli1-order` | Reserved |
| `confirmed` → `in_transit` (`POST …/ship`) | `dupli1-order` | **Committed** |
| Cancel before ship | `dupli1-order` | **Released** |
| Cancel after ship | `dupli1-order` | Refund only — stock is **not** auto-restocked |

---

## Order lifecycle

Statuses (domain constants in `order/pkg/domain/order.go`):

`pending` → `paid` → `confirmed` → `in_transit` → `delivered` → `fulfilled`

Branches: `canceled` (from most non-final states), `disputed` (customer non-receipt on `delivered`).

```text
POST /orders → pending → paid → confirmed → in_transit → delivered → fulfilled
                  ↓         ↓(2h SLA) ↑         ↑(commit stock)  ↓(14d SLA or dispute)
               canceled ←──────────────────────────────────  disputed
                  ↑ auto-cancel after 5 min unpaid (pending only)
```

| Transition | Trigger | Permission / rule |
|------------|---------|-----------------|
| → `paid` | `payment.succeeded` consumer | Payment-driven only |
| `paid` → `confirmed` | `POST /orders/{id}/confirm` or 2h auto-confirm worker | `order.status.update` |
| `confirmed` → `in_transit` | `POST /orders/{id}/ship` (atomic `ShipIfConfirmed`) | `order.ship` |
| `in_transit` → `delivered` | `POST /orders/{id}/deliver` | `order.ship` |
| `delivered` → `fulfilled` | Customer `POST …/receipt/confirm`, manager `PUT /status`, or 14-day sweep | ABAC / `order.status.update` |
| `delivered` → `disputed` | Customer `POST …/receipt/dispute` | ABAC owner |
| `disputed` → `fulfilled` | `POST …/dispute/resolve` | `order.status.update` |
| → `canceled` | Customer/manager cancel, unpaid expiry, full-refund `payment.canceled` | See refund table in [payment-service.md](payment-service.md) |

Full transition matrix: [api.md § Orders](api.md#orders).

### Pre-shipment video inspection (operations step)

Every order is filmed before it ships. After an order is `confirmed` and before the operator calls `POST /orders/{id}/ship`, the operator records a video inspecting the item before it is packed. The order ships only after this video is recorded.

This step happens **outside the system** today. The order service stores nothing about it: there is no field on the order, no status between `confirmed` and `in_transit`, no upload in manage-web, and the shopper does not see it. The video is kept outside dupli1, by the team that ships.

What the video is for: it is the shop's evidence of what left the warehouse and in what condition. It is the first thing to check when a shopper opens a `disputed` order (`POST …/receipt/dispute`) or asks to cancel or return an item they say arrived damaged or different, before a manager resolves it with `POST …/dispute/resolve` or a cancel.

If the step is ever brought into the system, it belongs between `confirmed` and `in_transit`: an inspection video reference on the order, required by `POST /orders/{id}/ship`, so shipping an uninspected order is refused rather than relying on the procedure.

---

## Background workers

Started from `order/pkg/bootstrap` alongside the HTTP server:

| Worker | Purpose | Interval / trigger |
|--------|---------|-------------------|
| Pending expiry | Auto-`canceled` unpaid `pending` orders after `DefaultPaymentTTL` (5 min) | Every 30s (`StartPendingExpiryWorker`) |
| `EnforceRefundPolicy` | Auto-`confirmed` paid orders past 2h (`ManagerConfirmationWindow`); auto-approve overdue cancel requests | Every 30s (`StartRefundPolicyWorker`) |
| `EnforceDeliveryPolicy` | Auto-`fulfilled` `delivered` orders with no customer response after 14 days (`DeliveryAutoFulfillWindow`) | Every 30s (`StartFulfillmentPolicyWorker`) |
| Outbox drain | Publish `order.created` / status events to NATS | Every 2s (`StartOutboxWorker`) |

Constants: `order/pkg/domain/order.go` (`ManagerConfirmationWindow`, `DeliveryAutoFulfillWindow`).

---

## NATS consumers

| Subject | Handler | Effect |
|---------|---------|--------|
| `payment.succeeded` | `MarkOrderPaid` | `pending` → `paid` (idempotent on `payment_id`); late payment on auto-canceled order re-reserves stock |
| `payment.canceled` | Refund cancel handler | Full refund with matching `payment_id` → cancel a `paid` or `confirmed` order (atomic guard vs concurrent ship); shipped orders are logged, not canceled |

Event subject names and payload shapes: `shared/pkg/events`.

---

## Gateway dependencies

Order never calls product or payment by direct service URL — always via **`DUPLI1_GATEWAY_URL`** (the gateway's internal `:8081` listener). Its service-account token comes from exchanging `DUPLI1_ORDER_SERVICE_API_KEY` at `POST /api/v1/auth/token` (`DUPLI1_AUTH_URL`, falling back to the gateway):

| Need | Gateway path | Auth |
|------|--------------|------|
| Reserve / commit / release stock | `/api/v1/products/inventory/reservations/…` | Order service account Bearer |
| Variant lookups | `/api/v1/products/variants/…` | — (public) |
| Promotional codes | `/api/v1/products/promotions/evaluate`, `…/reserve`, `…/consume`, `…/release`, `…/tier` | Order service account Bearer (`promotion.redeem`) |
| Refund on cancel | `/api/v1/payments/{id}/cancel` | Always the order service account (`payment.cancel`); the operator is recorded only in the refund reason |

The `dupli1-order` service account is seeded with `order.ship`, `order.status.update`, `inventory.reservation.manage`, `payment.cancel`, `promotion.redeem` (`DUPLI1_ORDER_SERVICE_*`).

---

## Money fields

All order JSON / DB money uses **whole KRW won** with the `*_won` suffix (`subtotal_won`, `discount_won`, `shipping_fee_won`, `total_won`, `unit_price_won`). Legacy `*_krw` / `*_cents` columns are renamed on migrate.

**Shipping fee:** flat per-order charge from `DUPLI1_ORDER_SHIPPING_FEE_WON` (deprecated aliases `DUPLI1_ORDER_SHIPPING_FEE_KRW`, `DUPLI1_ORDER_SHIPPING_FEE_CENTS`; default **0**, free delivery). Snapshotted on the checkout session at open; `complete` charges the quoted fee. Promotional codes discount goods only — total never drops below shipping unless shipping is also discounted (future promo work).

**Card surcharge:** a card order (`payment_method: credit_card`, the default) adds `DUPLI1_ORDER_CARD_SURCHARGE_BPS` (default 1000 = 10%) of `subtotal - discount + shipping`, rounded down, as `card_surcharge_won` inside `total_won`; `bypass` orders have none. See [checkout-session.md](checkout-session.md).

Client-sent `unit_price_won` on create/checkout is **ignored**; prices are resolved server-side from product.

---

## Order response fields (fulfillment)

Beyond `status` and line items, responses carry audit timestamps and tracking:

| Field | Set when |
|-------|----------|
| `paid_at` | Payment succeeds |
| `confirmed_at` | `paid` → `confirmed` |
| `shipped_at`, `shipped_by`, `carrier`, `tracking_number`, `carrier_note` | Ship |
| `delivered_at`, `delivered_by` | Deliver |
| `receipt_confirmed_at` | Customer confirms receipt (nil when auto-fulfilled or manager override) |
| `disputed_at`, `dispute_reason` | Customer disputes delivery |
| `cancel_requested_at`, `cancel_request_reason` | Customer cancel request after confirm |
| `canceled_at` | When the order became canceled; cleared if a late payment reinstates it. With `paid_at` set it marks a refund, which the sales report counts in this period |

Fulfillment snapshot on checkout complete: `recipient_name`, `recipient_phone`, `shipping_address` (immutable on the order).

---

## Live order feed (admin SSE)

`dupli1-manage-web` subscribes to `GET /api/v1/orders/events` (Server-Sent Events, `order.read.all`) for operator dashboards. Implemented since 2026-09-27: `order/pkg/livefeed` holds a broadcast NATS subscription to `order.*` on every replica and relays each change to that replica's streams (`order/pkg/handler/events.go`). Contract: [order-live-events.md](order-live-events.md).

---

## Local development

```bash
cd order && go test ./...
POSTGRES_URL=postgres://dupli1:dupli1_dev@localhost:5435/orders?sslmode=disable go test ./...
```

Compose host port **8083** (direct) or **8080** via gateway. Smoke: `BASE=http://localhost:8080 scripts/smoke-money-path.sh` (stack must be running).

---

## Related docs

| Doc | Topic |
|-----|-------|
| [checkout-session.md](checkout-session.md) | Session TTL, complete, unavailable items |
| [payment-service.md](payment-service.md) | Payment + refund policy, state diagram |
| [cart-service.md](cart-service.md) | Persistent cart vs checkout |
| [permissions.md](permissions.md) | `order.*` permission matrix |
| [order-live-events.md](order-live-events.md) | Admin SSE live order feed |
