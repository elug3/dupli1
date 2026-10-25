# Payment kill switches

An operator can switch a payment method off at runtime — for example card
payments while the PG is having an incident — without a deploy, and switch it
back on the same way. Owned by **payment**; order and the storefront honour it.

## Switches

Switches are declared in code (`payment/pkg/domain/killswitch.go`,
`KillSwitchDefs`), not created at runtime: a switch is only worth having once
something enforces it. Each has a `key`, a `kind` (what it turns off, so which
code path enforces it) and a `target`.

| Key | Kind | Blocks |
|-----|------|--------|
| `payment_method.credit_card` | `payment_method` | New NANO card payments |
| `payment_method.bypass` | `payment_method` | New manager Bypass payments |

A switch nobody has flipped is released. State lives in `payment_kill_switches`
(one row per flipped key; in-memory when `DUPLI1_PAYMENT_DB` is unset, which is
right for one replica only).

## What an engaged `payment_method` switch does

| Where | Effect |
|-------|--------|
| payment `POST /api/v1/payments` | `409` `reason: payment_method_disabled` with `method` and the operator's `message`. Checked after the `Idempotency-Key` replay (a retry of a payment that already went through still returns it) and **before** an open checkout is reused, so a card window issued before the switch is not handed out again |
| payment `GET …/{id}/nano/checkout` | Sends the shopper back with the usual retryable `checkout_failed`; the retry is then refused with the message |
| order `POST /orders`, checkout complete | Same `409` body, before any stock is reserved, so a shopper is not left with an unpayable order. Order asks payment's public methods endpoint; if payment cannot be reached it **lets the order through** (payment still refuses the payment — failing closed would turn a payment blip into a checkout outage) |
| storefront checkout | Does not offer the method; shows the operator's message, or its generic "unavailable" copy |

Never blocked: payments already captured, refunds (`…/cancel`), NANO
`return`/webhook callbacks for a checkout already under way, and the reconcile
worker. The switch stops new money coming in by a method; it never strands
money that already moved.

A store that cannot be read fails `POST /payments` (`500`) rather than letting
it through.

## API

| Method | Path | Auth | |
|--------|------|------|---|
| `GET` | `/api/v1/payments/methods` | — | `{ "methods": [{ "method", "available", "configured", "disabled", "message"?, "requires_permission"? }] }`. `available` = `configured && !disabled`; `configured` is false for card without NANO credentials. `Cache-Control: no-store` |
| `GET` | `/api/v1/payments/kill-switches` | `payment.killswitch.read` or `.manage` | `{ "kill_switches": [{ "key", "kind", "target", "engaged", "message"?, "reason"?, "updated_by"?, "updated_at"? }] }`, every declared switch in order |
| `PUT` | `/api/v1/payments/kill-switches/{key}` | `payment.killswitch.manage` | `{ "engaged": bool, "message"?, "reason"? }` → the switch. `engaged` is required (a body that forgot it must not release a switch). `message` (shown to shoppers) and `reason` (staff only) are ≤200 characters. Unknown key `404` |

`updated_by` is the operator's email (user id when the token has none). Every
flip is also logged by payment.

Both permissions are held only through `*` / `admin.*` unless granted
explicitly; no bundle carries them.

## Adding a switch

1. Add a `KillSwitchDef` (and, for a new kind, a `KillSwitchKind…` constant).
2. Add the check where the thing happens, and say in this doc what it blocks
   and what it must never block.
3. manage-web lists switches from `GET /kill-switches`, so a new key shows up
   there without a change; give it a label in the console's copy.
