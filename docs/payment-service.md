# Payment Service

**Status:** Implemented (`payment/`). Local dev without Stripe uses `GET /api/v1/payments/{id}/simulate-success`.

The **payment service** (`dupli1-payment`) collects money for **pending** orders via **Stripe Checkout** (hosted redirect). Dupli1 **never** handles card numbers, CVC, or card passwords.

On PG success, payment publishes **`payment.succeeded`**. The **order service** consumes it, verifies amount, and moves the order to **`paid`**. The **notification service** sends a Telegram alert to ops. An **order manager** ships the order (`paid` → **`in_transit`**), which **commits** inventory (plan B).

See also: [cart-service.md](cart-service.md), [checkout-session.md](checkout-session.md).

---

## Order state machine

| Status | Meaning | Stock (plan B) |
|--------|---------|----------------|
| `pending` | Created at checkout, **not paid** | **Reserved** |
| `paid` | PG success; ops queue | Reserved |
| `in_transit` | Order-manager shipped | **Committed** |
| `fulfilled` | Delivered | Committed |
| `canceled` | Unpaid timeout, payment failed, or ops reject | **Released** |

```mermaid
stateDiagram-v2
    [*] --> pending: checkout complete
    pending --> paid: payment.succeeded
    pending --> canceled: 5min TTL / payment failed
    paid --> in_transit: POST /orders/{id}/ship
    paid --> canceled: ops reject (+ refund)
    in_transit --> fulfilled: ops fulfill
    canceled --> [*]
    fulfilled --> [*]
```

**Removed:** `confirmed` — replaced by `paid` (money received) and `in_transit` (approved to ship).

---

## End-to-end flow

```mermaid
sequenceDiagram
    participant Client
    participant Cart as dupli1-cart
    participant Order as dupli1-order
    participant Pay as dupli1-payment
    participant PG as Stripe Checkout
    participant Bus as NATS
    participant Notif as dupli1-notification
    participant Ops as order-manager

    Client->>Cart: shop
    Client->>Order: checkout complete
    Order-->>Client: order_id (pending, stock reserved)

    Client->>Pay: POST /api/v1/payments { order_id }
    Pay->>Order: GET order (verify pending + total)
    Pay->>PG: Create Checkout Session
    Pay-->>Client: checkout_url
    Client->>PG: redirect — pay on Stripe

    PG->>Pay: webhook checkout.session.completed
    Pay->>Bus: payment.succeeded { order_id, payment_id, amount_cents }
    Pay-->>Client: success redirect (poll order status)

    Bus->>Order: consume payment.succeeded
    Order->>Order: pending → paid
    Order->>Bus: order.paid
    Bus->>Notif: Telegram to ops

    Note over Client: "Payment received — we're preparing your order"

    Ops->>Order: POST /api/v1/orders/{id}/ship
    Order->>Order: paid → in_transit (commit stock)
    Ops->>Order: PUT status → fulfilled (later)
```

---

## Design decisions (locked)

| Topic | Choice |
|-------|--------|
| PG UI | **Stripe Checkout redirect** |
| Card data on Dupli1 | **Never** |
| Default currency | **`krw` only** (single currency; other codes rejected) |
| Amount unit | Whole Korean won (`amount_cents` = Stripe minor units for KRW — **not** won×100) |
| Unpaid `pending` TTL | **5 minutes** → auto-cancel + release stock |
| Inventory plan | **B** — reserve on checkout complete; **commit on `in_transit`** |
| Payment → order | **`payment.succeeded` event** (not HTTP confirm from payment) |
| Who sets `paid` | **Order service** (event consumer) |
| Who sets `in_transit` | **Order-manager** via `POST /orders/{id}/ship` |
| Manual `confirmed` | **Removed** |
| Telegram | **Notification service** on `order.paid` |
| Event payload | `order_id`, **`payment_id`**, **`amount_cents`** (must match `order.total_cents`) |
| Audit | `shipped_by`, `shipped_at` on order |

---

## Service boundaries

### Payment service owns

- Payment records and Stripe Checkout Session creation
- Stripe webhooks (signature verification, idempotency)
- Publishing **`payment.succeeded`**
- Dev/simulate endpoints when Stripe is not configured (local only)

### Order service owns

- Order lifecycle and inventory reserve/commit/release
- Consuming **`payment.succeeded`** → `paid`
- **`POST /api/v1/orders/{id}/ship`** → `in_transit` + commit
- 5-minute pending expiry worker

### Notification service owns

- Subscribing to **`order.paid`** (and other order events)
- Telegram messages to `TELEGRAM_ORDER_CHAT_ID`

### Payment does **not**

- Change order status directly over HTTP (except dev aids)
- Touch inventory
- Send Telegram

---

## Events

### `payment.succeeded` (payment → order)

```json
{
  "event_type": "payment.succeeded",
  "order_id": "ord_000001",
  "payment_id": "pay_000001",
  "amount_cents": 70000,
  "occurred_at": "2026-07-05T12:03:00Z"
}
```

Order consumer: idempotent on `payment_id`; reject if `amount_cents != order.total_cents`.

### `order.paid` (order → notification)

Published when order transitions `pending` → `paid`. Notification formats ops queue message.

---

## API

### Payment (`dupli1-payment`, port **8087**)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/v1/payments/health` | — | Health |
| `GET` | `/api/v1/payments/settings` | — | Non-secret service settings |
| `POST` | `/api/v1/payments` | Bearer | Start Checkout → `checkout_url` |
| `GET` | `/api/v1/payments/{id}` | Bearer | Payment status (poll after redirect) |
| `POST` | `/api/v1/payments/webhooks/stripe` | Stripe signature | Webhook handler |

**Create payment**
```json
{ "order_id": "ord_000001" }
```

**Response**
```json
{
  "id": "pay_000001",
  "order_id": "ord_000001",
  "amount_cents": 70000,
  "currency": "krw",
  "status": "requires_payment",
  "checkout_url": "https://checkout.stripe.com/...",
  "expires_at": "2026-07-05T12:05:00Z"
}
```

**Customer UX after redirect:** show *"Payment received — we're preparing your order"* while status is `paid`; poll `GET /api/v1/orders/{id}` or `GET /api/v1/payments/{id}`.

### Order (changes)

| Method | Path | Who | Description |
|--------|------|-----|-------------|
| `POST` | `/api/v1/orders/{id}/ship` | `order.ship` | `paid` → `in_transit`, commit stock, audit |
| `PUT` | `/api/v1/orders/{id}/status` | RBAC | `fulfilled` from `in_transit`; `canceled` from `pending`/`paid` |

**Ship response** includes `shipped_by`, `shipped_at`.

---

## Inventory (plan B)

| When | Action |
|------|--------|
| Checkout `complete` | `Reserve` |
| `pending` → `canceled` (timeout/fail) | `Release` |
| `paid` → `in_transit` (ship) | `Commit` |
| `paid` → `canceled` (reject) | `Release` |

---

## Security

1. Webhook is source of truth — not the browser redirect alone
2. Verify `amount_cents` on `payment.succeeded`
3. Idempotent webhook handling (`event_id`, `payment_id`)
4. Customer may only pay own orders
5. Ship endpoint requires elevated role; writes `shipped_by` from JWT `sub`

---

## Configuration

| Variable | Service | Description |
|----------|---------|-------------|
| `DUPLI1_PAYMENT_ADDR` | payment | Listen `:8087` |
| `DUPLI1_PAYMENT_DB` | payment | Postgres `payments` |
| `DUPLI1_ORDER_URL` | payment | Fetch order for validation |
| `STRIPE_SECRET_KEY` | payment | Stripe API key |
| `STRIPE_WEBHOOK_SECRET` | payment | Webhook signing secret |
| `STRIPE_SUCCESS_URL` | payment | Checkout success redirect |
| `STRIPE_CANCEL_URL` | payment | Checkout cancel redirect |
| `DUPLI1_PAYMENT_ORDER_TTL` | order | `5m` pending payment window |
| `NATS_URL` | all | Event bus |
| `TELEGRAM_BOT_TOKEN` | notification | Bot token |
| `TELEGRAM_ORDER_CHAT_ID` | notification | Ops chat |

Local Postgres (payment): `postgres://dupli1:dupli1_dev@localhost:5437/payments?sslmode=disable`

---

## Failure paths

| Case | Result |
|------|--------|
| Unpaid > 5 min | `canceled`, release stock |
| Stripe failed / abandoned | stay `pending` until TTL, then cancel |
| Paid, ops rejects | `canceled` + refund (payment phase 2) |
| Duplicate webhook | idempotent — order stays `paid` |

---

## Package layout

```text
payment/
├── cmd/
└── pkg/
    ├── domain/
    ├── service/
    ├── ports/
    ├── infra/pg/, stripe/, httporder/, nats/
    ├── handler/
    └── bootstrap/
```

---

## Related documentation

- [cart-service.md](cart-service.md)
- [checkout-session.md](checkout-session.md)
- [endpoints.md](endpoints.md)
- [api.md](api.md)
