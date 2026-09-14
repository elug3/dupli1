# Order Service

**Status:** Implemented (`order/`). Checkout sessions, order lifecycle, transactional outbox → NATS, and background policy workers.

The **order service** (`dupli1-order`) owns the short-lived **checkout session** and the long-lived **order**. It reserves stock on checkout `complete`, consumes **`payment.succeeded`** / **`payment.canceled`** from payment, calls product (stock/coupons) and payment (refunds) through the **nginx gateway** (`DUPLI1_GATEWAY_URL`), and publishes order events via a transactional **outbox**.

For checkout session field semantics see [checkout-session.md](checkout-session.md). For the money path and refund rules see [payment-service.md](payment-service.md). For the route-by-route API table see [api.md](api.md) and [endpoints.md](endpoints.md).

---

## Role in the purchase flow

```mermaid
flowchart LR
    Cart["dupli1-cart<br/>(intent)"]
    Order["dupli1-order<br/>(checkout + order)"]
    Product["dupli1-product<br/>(price, stock, coupons)"]
    Pay["dupli1-payment"]
    NATS["NATS"]

    Cart -.->|"client copies items"| Order
    Order -->|"reserve / commit stock"| Product
    Order -->|"redeem coupon"| Product
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

---

## Background workers

Started from `order/pkg/bootstrap` alongside the HTTP server:

| Worker | Purpose | Interval / trigger |
|--------|---------|-------------------|
| Pending expiry | Auto-`canceled` unpaid `pending` orders after `DefaultPaymentTTL` (5 min) | Periodic sweep |
| `EnforceRefundPolicy` | Auto-`confirmed` paid orders past 2h (`ManagerConfirmationWindow`); auto-approve overdue cancel requests | Runs inside payment consumer loop |
| `EnforceDeliveryPolicy` | Auto-`fulfilled` `delivered` orders with no customer response after 14 days (`DeliveryAutoFulfillWindow`) | Same loop |
| Outbox drain | Publish `order.created` / status events to NATS | Continuous |

Constants: `order/pkg/domain/order.go` (`ManagerConfirmationWindow`, `DeliveryAutoFulfillWindow`).

---

## NATS consumers

| Subject | Handler | Effect |
|---------|---------|--------|
| `payment.succeeded` | `MarkOrderPaid` | `pending` → `paid` (idempotent on `payment_id`); late payment on auto-canceled order re-reserves stock |
| `payment.canceled` | Refund cancel handler | Full refund with matching `payment_id` → cancel still-`paid` order (atomic guard vs concurrent ship) |

Event subject names and payload shapes: `shared/pkg/events`.

---

## Gateway dependencies

Order never calls product or payment by direct service URL in Compose/ECS — always via **`DUPLI1_GATEWAY_URL`**:

| Need | Gateway path | Auth |
|------|--------------|------|
| Reserve / commit / release stock | `/api/v1/products/inventory/reservations/…` | Order service account Bearer |
| Redeem coupon | `/api/v1/products/coupons/…` | Order service account Bearer |
| Refund on cancel | `/api/v1/payments/{id}/cancel` | Operator Bearer forwarded, else order service account (`payment.cancel`) |

The `dupli1-order` service account is seeded with `order.ship`, `order.status.update`, `inventory.reservation.manage`, `payment.cancel` (`DUPLI1_ORDER_SERVICE_*`).

---

## Money fields

All order JSON / DB money uses **whole KRW won** with the `*_won` suffix (`subtotal_won`, `discount_won`, `shipping_fee_won`, `total_won`, `unit_price_won`). Legacy `*_krw` / `*_cents` columns are renamed on migrate.

**Shipping fee:** flat per-order charge from `DUPLI1_ORDER_SHIPPING_FEE_WON` (deprecated aliases `DUPLI1_ORDER_SHIPPING_FEE_KRW`, `DUPLI1_ORDER_SHIPPING_FEE_CENTS`; default **30000**). Snapshotted on the checkout session at open; `complete` charges the quoted fee. Coupons discount goods only — total never drops below shipping unless shipping is also discounted (future promo work).

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

Fulfillment snapshot on checkout complete: `recipient_name`, `recipient_phone`, `shipping_address` (immutable on the order).

---

## Live order feed (admin SSE)

`dupli1-manage-web` subscribes to `GET /api/v1/orders/events` (Server-Sent Events) for operator dashboards. **The route is not registered in `dupli1-order` yet** — manage-web uses a mock gateway for browser tests until the backend hub ships. Planned contract: [order-live-events.md](order-live-events.md).

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
| [order-live-events.md](order-live-events.md) | Admin SSE contract (planned backend) |
