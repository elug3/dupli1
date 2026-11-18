# Points (적립금)

**Status:** the ledger, earning, VIP tiers and the APIs are built in the payment service. Not built yet: order spending points at checkout (it will call the internal hold routes below), review rewards, and the storefront and manage-web screens.

**Related:** [payment-service.md](payment-service.md), [order-service.md](order-service.md) (statuses), [permissions.md](permissions.md) (Internal APIs), [product-reviews-plan.md](product-reviews-plan.md).

Points are dupli1's store credit. A shopper earns them on fulfilled orders, can spend them like cash on later orders, and loses them 12 months after they arrive. The payment service owns them, because spending points is a way of paying and refunds already live there (decided 2026-11-18). Order will only record how many points an order used.

## Rules

| Rule | Value |
|------|-------|
| Value | 1 P = ₩1 |
| Base rate | 1% of goods paid (`BaseEarnBps = 100`), rounded down |
| Earn base | `subtotal_won − discount_won` from order's event, minus the points the order paid with. No shipping, no card surcharge, and points do not earn points |
| When | Shown as pending when the order is paid; credited when it is fulfilled (receipt confirmed, or 14 days after delivery) at the tier in force on that day |
| Refund | Canceled before fulfillment: the pending points are dropped. Canceled after: the credited points are taken back, from that order's lot first and then the soonest-expiring others. What the shopper already spent elsewhere is not collected (logged as a shortfall, not carried as debt) |
| Expiry | 12 months after crediting, per lot. Spending takes the soonest-expiring points first. An hourly sweep records what expired |
| Spending | Up to 100% of the goods, no minimum balance, together with a promo or referral code |
| Returned on cancel | Points an order paid with go back to the lots they came from, keeping those lots' expiry. If a lot expired meanwhile, those points expire at once (an `expire` entry follows the `return`), decided 2026-11-18 |

### VIP tiers

The tier multiplies the base rate. It is decided by goods paid on fulfilled orders (the earn base above) over the **12 KST calendar months before the current month**, so a tier holds for a whole month and is rechecked on the 1st. Refunded orders stop counting.

| Tier | Spend | Multiplier | Rate |
|------|-------|-----------|------|
| `member` | under ₩1,000,000 | 1× | 1% |
| `silver` | ₩1,000,000+ | 1.5× | 1.5% |
| `gold` | ₩3,000,000+ | 2× | 2% |
| `diamond` | ₩5,000,000+ | 3× | 3% |

Thresholds and multipliers are code (`domain.Tiers`), not settings. This replaces the old VIP auto-discount (`apply_mode: auto` promotions in product); retiring those is a separate step.

## Model

- **Lot** (`payment_point_lots`): points that arrived together and expire together, unique per `(source, ref)`: `order` + order id, `review` + review id, `adjust` + an adjustment id. That uniqueness is what makes every credit idempotent. Status `pending` → `available` → `expired`, or `revoked`.
- **Hold** (`payment_point_holds`, one per order): points set aside at checkout with the lots they came from. `reserved` → `consumed` (paid) or `released` (canceled).
- **Entry** (`payment_point_entries`): the shopper-visible history, signed: `earn`, `grant`, `spend`, `return`, `clawback`, `adjust`, `expire`.

A change loads one customer's whole account under a per-customer advisory lock, applies a domain method and saves it in one transaction (`PointsStore.MutatePointAccount`). Different customers never wait on each other.

## Earning from order events

Payment subscribes to `order.paid` and `order.status_updated` (queue group `payment-workers`) and acts on `status`: `paid` → pending, `fulfilled` → credit, `canceled` → undo. Every step is idempotent per order, and a `fulfilled` event without an earlier `paid` still credits. Without `NATS_URL` nothing is earned.

These are core NATS subjects, so an event missed while payment is down is not redelivered. A reconcile pass against order is not built yet.

## API

| Method | Path | Who | |
|--------|------|-----|---|
| GET | `/api/v1/payments/points/me` | the signed-in customer | `{ customer_id, balance, pending, expiring, expiring_by, tier, entries, next_cursor? }`. `expiring` = spendable points expiring in 30 days. `tier` = `{ code, multiplier, min_spend_won, spend_won, window_from, window_to, next? }` (`multiplier` in tenths: 15 = 1.5×). `?limit=` entries, default 20, max 100 |
| GET | `/api/v1/payments/points/me/entries?cursor=` | the signed-in customer | `{ entries, next_cursor? }`, newest first |
| GET | `/api/v1/payments/points/customers/{id}` and `…/entries` | `payment.points.read` | The same for any customer |
| POST | `/api/v1/payments/points/customers/{id}/adjust` | `payment.points.manage` | `{ points, reason }`: positive grants (a new lot, 12 months), negative takes away (never below zero: `409 insufficient_points`). `reason` 1–200 characters, required. `Idempotency-Key` makes a grant safe to retry |
| POST | `/api/v1/payments/points/reserve` | `dupli1-order` + `payment.points.redeem`, internal listener | `{ order_id, customer_id, points }` → the hold. Same order and amount again returns it; another amount is `409 points_hold_conflict`; too few points `409 insufficient_points` |
| POST | `/api/v1/payments/points/consume` | same | `{ order_id }` once the order is paid |
| POST | `/api/v1/payments/points/release` | same | `{ order_id }` when it is canceled; returns spent points too |

Errors carry `reason`: `invalid_points` (400), `insufficient_points`, `points_hold_conflict`, `points_hold_closed` (409), `points_hold_not_found` (404).

The hold routes are internal APIs ([permissions.md](permissions.md#internal-apis)): the gateway answers `404` for them on `:80`, and payment refuses any caller that is not the `dupli1-order` service account, owners included.

## Checkout (next step, in order)

Order will reserve points at checkout complete, the way it reserves a promo code, and charge the card surcharge only on what is left: `CardSurchargeWon(total − points)`. It consumes the hold on `payment.succeeded` and releases it on cancel. An order paid fully in points needs a payment path with no card part.

## Decisions

Answered by the owner on 2026-11-18:

- A point returned on cancel after its lot's expiry passed **expires at once**, as built.
- A clawback the shopper cannot cover is **forgiven**: no negative balance, as built.
- There are no current VIP members, so nobody is migrated into the tiers. Retiring the `apply_mode: auto` promotions is still a separate step.
