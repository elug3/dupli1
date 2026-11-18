# Points (적립금)

**Status:** the ledger, earning, VIP tiers, the APIs, spending points at checkout and review rewards are built. The manage-web screens are in review; the storefront screens are being built.

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
| Returned on cancel | Points an order paid with go back on any cancel, to the lots they came from, keeping those lots' expiry. If a lot expired meanwhile, those points expire at once (an `expire` entry follows the `return`), decided 2026-11-18 |

### VIP tiers

The tier multiplies the base rate. It is decided by goods paid on fulfilled orders (the earn base above) over the **12 KST calendar months before the current month**, so a tier holds for a whole month and is rechecked on the 1st. Refunded orders stop counting.

| Tier | Spend | Multiplier | Rate |
|------|-------|-----------|------|
| `member` | under ₩1,000,000 | 1× | 1% |
| `silver` | ₩1,000,000+ | 1.5× | 1.5% |
| `gold` | ₩3,000,000+ | 2× | 2% |
| `diamond` | ₩5,000,000+ | 3× | 3% |

Thresholds and multipliers are code (`domain.Tiers`), not settings. This replaces the old VIP auto-discount (`apply_mode: auto` promotions in product), retired on 2026-11-18: order no longer applies tiers and product refuses new ones.

## Model

- **Lot** (`payment_point_lots`): points that arrived together and expire together, unique per `(source, ref)`: `order` + order id, `review` + `<order id>:<sku id>` (plus `:photo` for the photo bonus), `adjust` + an adjustment id. That uniqueness is what makes every credit idempotent. Status `pending` → `available` → `expired`, or `revoked`.
- **Hold** (`payment_point_holds`, one per order): points set aside at checkout with the lots they came from. `reserved` → `consumed` (paid) or `released` (canceled).
- **Entry** (`payment_point_entries`): the shopper-visible history, signed: `earn`, `review`, `grant`, `spend`, `return`, `clawback`, `adjust`, `expire`.

A change loads one customer's whole account under a per-customer advisory lock, applies a domain method and saves it in one transaction (`PointsStore.MutatePointAccount`). Different customers never wait on each other.

## Earning from order events

Payment subscribes to `order.paid` and `order.status_updated` (queue group `payment-workers`) and acts on `status`: `paid` → pending, `fulfilled` → credit, `canceled` → undo. Every step is idempotent per order, and a `fulfilled` event without an earlier `paid` still credits. Without `NATS_URL` nothing is earned.

These are core NATS subjects, so an event missed while payment is down is not redelivered. A reconcile pass against order is not built yet.

## Review rewards

A review earns points once per order line, decided 2026-11-18:

| Review | Points |
|--------|--------|
| Text of at least 20 characters (trimmed, counted in characters) | 2,000 P |
| The same with at least one photo | 5,000 P in all |

- **Who:** only the buyer of a fulfilled order can write a review at all ([product-reviews-plan.md](product-reviews-plan.md)), so payment does not check that again.
- **How it arrives:** product publishes `product.review_changed` (`events.ProductReview`: action, review/order/sku/customer ids, rating, `body_chars`, `photo_count`, `hidden`; never the text) on create, edit, hide and unhide. Payment subscribes in the same queue group as order's events and calls `ReviewWritten` or `ReviewHidden`. Like order's events this is core NATS, so a review written while payment is down earns nothing until it is edited again.
- **Once per line:** the reward is two lots keyed by the line, `review` + `<order id>:<sku id>` (2,000) and `…:photo` (the 3,000 photo bonus). A photo added in a later edit tops the review up to 5,000. Deleting the review and writing a new one for the same line earns nothing more. Shortening the text or removing the photos later keeps what was earned.
- **History:** one `review` entry per reward, with the order id, the review id as `ref`, and `note` `text` or `photo`.
- **Hidden by staff:** the line's reward is taken back (a `clawback`, shortfall forgiven like an order's) and the line cannot earn again, even if the review is unhidden. A manager can grant points by hand for a mistake.
- **Canceled order:** canceling an order after fulfillment also takes back every review reward on it, and its lines can no longer earn.
- **Expiry:** 12 months, like any lot.

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

## Checkout

The shopper sends `points_won` with checkout complete (`POST /api/v1/orders/checkout/sessions/{id}/complete`). Order:

1. Trims it to the goods after every discount (`subtotal_won − discount_won`). Points never pay delivery, so "use all" can send the whole balance.
2. Prices the order: goods − discounts − points + delivery, then the card surcharge on that (`CardSurchargeWon(total − points)`). It stores `points_won`, already off `total_won`, so payment charges and refunds only the money part.
3. Reserves the points in payment, keyed by the order. Too few (`409 insufficient_points`) rolls the order back, like a code spent in the meantime; the session stays open.
4. Consumes the hold when the order is paid, next to the code's consume.
5. Releases the hold on **every** cancel, shipped or not: points are money, and a shipped cancel refunds the card part too. Payment also releases it on the `canceled` event, so either one is enough.

**Paid in full with points.** With free delivery and points covering the goods the total is ₩0: no surcharge, nothing to charge. The storefront still calls `POST /api/v1/payments`; payment records a succeeded `method: points`, `provider: points` payment of ₩0 (no PG, no `checkout_url`) and publishes `payment.succeeded`, so order moves to paid the usual way. A cancel refunds nothing by card and gives the points back. The payments table allows `amount_won = 0` for this (migrated on startup).

**Earning** still uses the hold: an order earns on goods after discounts less the points it used. `order.*` events also carry `points_won`.

**Sales report:** `points_won` is its own column. It is not money received, so `gross_won` (the sum of `total_won`) already leaves it out.

**Known gaps:** a ₩0 order is refused while the `credit_card` kill switch is engaged, like any card order. An order auto-canceled for non-payment gets its points back; if a card payment then arrives late and reinstates it, the points are not taken again (the consume is logged as failed).

## Decisions

Answered by the owner on 2026-11-18:

- A point returned on cancel after its lot's expiry passed **expires at once**, as built.
- A clawback the shopper cannot cover is **forgiven**: no negative balance, as built.
- There were no VIP members, so nobody was migrated into the tiers. The `apply_mode: auto` promotions were retired on 2026-11-18.
