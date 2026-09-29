# Promotional codes: order-level vs product-level application

Status: **proposal — awaiting a decision** (2026-09-29). Nothing here is implemented yet beyond what "Today" describes.

This document explains how a promotional code turns into a discount today, then lays out three options for where the discount applies — the whole order, specific products, or either (chosen per code) — with worked examples, trade-offs and the work each needs. A recommendation is at the end, but the choice is a business decision.

Related: [product-promo-referral-code-plan.md](product-promo-referral-code-plan.md) (conditions/benefit model), [product-promotion-rename.md](product-promotion-rename.md).

## Today

One code per checkout. Order sends the cart lines (`sku_id`, `sku`, `quantity`, `unit_price_won`) to product, product's evaluator (`product/pkg/domain/promotion_evaluate.go`) returns **one** `discount_won`, and order stores it once on the checkout session / order:

```
total_won = subtotal_won − discount_won + shipping_fee_won
```

A definition has two parts:

| Part | Field | What it controls |
|------|-------|------------------|
| Conditions | `conditions.all` (cart rules: `subtotal_won`, `item_count`; line rules: `line.category`, `line.brandCode`, `line.productId`, `line.skuId`, `line.unit_price_won`, `line.on_sale`), `conditions.exclude`, `conditions.line_match` (`any` / `all`) | Whether the code is accepted, and which lines are **eligible** |
| Benefit | `discount_type` (`percent` / `fixed`), `discount_fraction`, `discount_fixed_won`, `max_discount_won`, **`apply_to`** | How much comes off, and from what base |

`apply_to` already has two values:

- `entire_subtotal` (default) — the discount is computed on the whole cart subtotal.
- `eligible_lines` — computed on the sum of the eligible lines only.

So the **calculation** can already be order-level or product-level. What is missing is the **result**: in both cases the outcome is a single number on the order. No line records how much of the discount it received, so:

- the storefront/receipt cannot show "−10% on this item";
- a partial cancel/refund of one item has no principled amount to take back (today only whole-order cancel exists, which refunds `total_won`);
- reporting cannot say which products a campaign actually discounted.

## Worked example used below

Cart:

| Line | Brand | Qty × unit | Line total |
|------|-------|-----------|-----------:|
| A — bag | X | 1 × 300,000 | 300,000 |
| B — wallet | X | 1 × 100,000 | 100,000 |
| C — scarf | Y | 1 × 100,000 | 100,000 |
| | | **Subtotal** | **500,000** |

Code: 10% off, capped at 40,000원.

## Option 1 — Order-level only

Every code discounts the whole order. Conditions may still gate acceptance ("only if the cart contains brand X", "subtotal ≥ 200,000"), but the base is always the full subtotal. `apply_to` is fixed to `entire_subtotal`; `eligible_lines` is removed.

Example: 10% of 500,000 = 50,000 → capped → **−40,000**. Customer pays 460,000 + shipping.

For display/refunds the discount is split across lines in proportion to line totals (A −24,000, B −8,000, C −8,000), rounding remainder to the largest line so the parts sum exactly.

| Pros | Cons |
|------|------|
| Simplest rule for customers and managers ("10% off your order") | Cannot run "10% off brand X only" — a brand campaign discounts other brands too |
| Smallest change (mostly removing an option) | The discount spills onto items the campaign was not meant to fund |
| Matches most existing codes (`WELCOME50`) | |

## Option 2 — Product-level only

Every code discounts only eligible lines. With no line conditions every line is eligible, so "10% off everything" is still expressible — it is just the same as eligible = all. `apply_to` is fixed to `eligible_lines`.

Example with condition `line.brandCode = X`: eligible = A + B = 400,000 → 10% = 40,000 → **−40,000**, split A −30,000, B −10,000, C 0.

| Pros | Cons |
|------|------|
| Discount lands exactly on the products the campaign targets | A fixed-amount code ("5,000원 off") becomes awkward: which line pays for it? It must be split anyway |
| Per-line amounts are natural, so display and partial refunds are clean | Cart-level rules (`subtotal_won ≥ …`) still gate, which can confuse ("spend 200,000 on anything, get 10% off only brand X") |
| | Changes the meaning of existing `entire_subtotal` codes (for codes with no line conditions the result is identical; for codes with line conditions it shrinks) |

## Option 3 — Both, chosen per code (manager picks)

Keep both `apply_to` values and expose the choice in manage-web when a code is created:

- **주문 전체 (Whole order)** → `entire_subtotal`
- **대상 상품만 (Eligible products only)** → `eligible_lines`

Example, same code with condition `line.brandCode = X`:

| Choice | Base | Discount | Allocation |
|--------|-----:|---------:|------------|
| Whole order | 500,000 | −40,000 (capped) | A −24,000 · B −8,000 · C −8,000 |
| Eligible products only | 400,000 | −40,000 | A −30,000 · B −10,000 · C 0 |

Without the cap: whole order −50,000, eligible only −40,000.

| Pros | Cons |
|------|------|
| Covers both campaign types ("10% off your order when you buy brand X" and "10% off brand X") | Managers must understand the difference — needs a clear label and a preview in the form |
| The evaluator already implements both; no change to how the amount is computed | Slightly more to test |
| Existing codes keep their meaning (default stays `entire_subtotal`) | |

## Work common to all options: per-line allocation

Whichever option is chosen, the gap is the same — record where the discount went.

1. **product** — the evaluate response gains `line_discounts: [{sku_id, discount_won}]`. Allocation: proportional to each base line's total, `floor`, remainder to the largest line, sum == `discount_won` exactly. Lines outside the base get 0. Pure function in `promotion_benefit.go`, table-tested.
2. **order** — `OrderItem` gains `discount_won` (Postgres `ADD COLUMN IF NOT EXISTS … DEFAULT 0`, additive, per the inline-migration rule). The order-level `discount_won` stays and must equal the sum of the lines; `NewOrder` checks this.
3. **Old orders** — rows with no line split read 0 per line; nothing recomputes history.
4. **Frontends** — cart/checkout/receipt show each line's discount; manage-web order detail shows it too.
5. **Refunds** — a future partial cancel refunds `line_total − line.discount_won`. Whole-order cancel is unchanged.
6. **Ledger** — `promotion_redemptions.applied_benefit` already snapshots the benefit; also snapshot `line_discounts` so the split can't be rewritten by a later edit.

Option-specific work on top:

- **Option 1:** reject `apply_to = eligible_lines` in `Benefit.Validate`; migrate any stored `eligible_lines` definitions (expected none — confirm with a query before the change).
- **Option 2:** reject `entire_subtotal`, migrate existing definitions to `eligible_lines`, and review each code that has line conditions, because its discount will shrink.
- **Option 3:** manage-web radio for `apply_to` with a live preview against a sample cart; no data migration.

## Decision questions

1. Do we plan campaigns aimed at one brand/category ("10% off brand X")? If **no** → Option 1 is enough.
2. Do we need "spend on the target, get a discount on everything"? If **yes** together with (1) → Option 3.
3. Do we need partial (per-item) refunds soon? Any option needs the per-line allocation above; it is the part to prioritise.

## Recommendation

**Option 3**, with per-line allocation built first. The evaluator already supports both bases, existing codes keep their behaviour, and it avoids redoing the model when the first brand-specific campaign arrives. The only real cost is a clear choice in the manager form. If the business confirms it will never target products, Option 1 is the simpler fallback — the per-line allocation work is the same either way.
