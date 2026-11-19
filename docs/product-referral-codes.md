# Referral codes (secret)

**Status:** built 2026-11-19 (Linear SER-105). Product holds the codes, order
keeps who referred each order and serves the monthly commission report,
manage-web creates the codes and shows the report. The storefront has no
change and must never get one: this is a **secret** feature for dupli1's
acquaintances.

## What a referral code is

A promotional code with `kind: "referral"` and a **referrer name**. The
referrer is a person written on the code, **not a linked account** (decided
2026-11-19): nobody has to sign up to refer, and the referrer never sees the
code's numbers in the shop.

| Rule | How it is built |
|------|-----------------|
| Typed in the normal promotional code box, never named on the site | It is an ordinary `global` code to every public route. `POST …/promotions/redeem` returns `PublicView()` (no `kind`, `referrer_name`, `commission_bps`); `…/evaluate` never carried them; the wallet only lists `single_user` codes. Order never serializes the referral fields of an order (`json:"-"`). |
| Discount set per code, goods only, no ₩ cap | The ordinary `benefit` (percent or fixed, `target: goods`). SER-105's code for 허경호 is 15%. Leave `max_discount_won` unset. |
| One code per order | The order's one code slot, as for any code. |
| The same customer may use it on any number of orders | `Promotion.CustomerLimit()` is unlimited for a referral code; `max_per_customer` is ignored. `max_redemptions` still caps the code overall if set. |
| Points can pay part of a referral order; VIP multipliers still apply | Nothing special: points and VIP earning treat it as any code order. |
| Commission per code, default 5% | `commission_bps` (500 = 5%), defaulted on create when omitted, editable later. |
| Paid monthly by hand, fulfilled orders only, 3.3% withheld | Order's `GET /api/v1/orders/reports/referrals?month=YYYY-MM`. |

A referral code must be `global`, cannot be auto-issued, and needs a
`referrer_name` (≤80 characters). `kind` is fixed when the code is created
(`PUT` with `kind` answers `400`). An ordinary code may not carry a referrer or
a commission.

## Snapshot on the order

Order's checkout complete reserves the code through product's internal
`POST /api/v1/products/promotions/reserve` (service account `dupli1-order`,
internal listener only). For a referral code, that response carries
`referral: {referrer_name, commission_bps}` beside `result`, and order stores
both on the order (`orders.referrer_name`, `orders.referral_commission_bps`).
Renaming the referrer or changing the rate later never changes what a placed
order earns.

Order also records `fulfilled_at` on every route to `fulfilled` (customer
confirmation, the 14-day auto-fulfill, a manager override, a dispute resolved
as delivered). Orders fulfilled before the column existed were backfilled from
`receipt_confirmed_at`, else `updated_at`.

## The commission report

`GET /api/v1/orders/reports/referrals?month=2026-11` (`order.read.all`; month
in KST, default the current one):

- **Earning orders:** referred orders **fulfilled** in the month. Fulfilled is
  final in order (a fulfilled order cannot be canceled), so a refund always
  happens before fulfillment and the order simply never earns. There is no
  clawback from a month already paid. Should partial refunds or refunds after
  fulfillment ever exist, they would be deducted in the month they happen,
  like the sales report.
- **Base:** goods after the code's discount, `subtotal_won - discount_won`.
  Shipping and the card surcharge earn nothing. Points the shopper paid with
  are part of the sale and count.
- **Commission:** base × the order's rate, rounded down to the won, per order.
- **Withholding (3.3%)** on each referrer's monthly total: income tax 3% and
  local income tax (a tenth of the income tax), each rounded down to ten won.
  `net_won = commission_won - withholding_won` is what to transfer.
  The small-amount exemption (소액부징수) is not applied; staff decide that.
- **Open orders:** referred orders paid but not yet fulfilled, whenever they
  were placed, with the commission they will earn if fulfilled. They are not
  payable yet.

Grouped by referrer name (the name snapshotted on the order), sorted by name.

```json
{
  "month": "2026-11", "from": "2026-11-01", "to": "2026-11-30",
  "timezone": "Asia/Seoul", "withholding_bps": 330,
  "referrers": [{
    "referrer_name": "허경호", "codes": ["HUH15"],
    "orders": 1, "base_won": 850000, "commission_won": 42500,
    "income_tax_won": 1270, "local_tax_won": 120, "withholding_won": 1390,
    "net_won": 41110,
    "lines": [{ "order_id": "…", "customer_id": "…", "code": "HUH15",
                "status": "fulfilled", "paid_at": "…", "fulfilled_at": "…",
                "subtotal_won": 1000000, "discount_won": 150000, "points_won": 0,
                "base_won": 850000, "commission_bps": 500, "commission_won": 42500 }],
    "open_orders": 0, "open_commission_won": 0, "open": []
  }],
  "totals": { "orders": 1, "base_won": 850000, "commission_won": 42500,
              "withholding_won": 1390, "net_won": 41110,
              "open_orders": 0, "open_commission_won": 0 }
}
```

The report does not record that a month was paid; staff keep that record.

## Not built

The richer referral tracking discussed on 2026-11-18 (time from sign-up to
first code order, repeat rates, cost and return per code, spike alerts) is
not part of this. The report is the money view only.
