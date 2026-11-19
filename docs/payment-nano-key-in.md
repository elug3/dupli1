# NANO key-in card payment (수기결제)

Linear SER-106. NANO asked us to move card approval from 인증결제 (v2.7, NANO's own card window) to 수기결제 (key-in): the shopper types the card on the storefront's checkout page and payment sends it to NANO server to server. Source: **[NANO] 수기결제 연동 API 안내 v2.5** §1 (공통) and §2 (결제요청). Refunds already used that guide's §3 (`cancel.io`) and are unchanged ([payment-service.md](payment-service.md)).

NANO has **no hosted card form** for this API, so the card number does pass through Dupli1. That reverses the old rule in [payment-methods-plan.md](payment-methods-plan.md) ("do not integrate 수기결제"); it was decided on 2026-11-19. Everything below is about passing the card through once without keeping any of it.

## Flow

```
storefront /checkout/pay/:id  (card form, SSR action)
   └─ BFF → POST /api/v1/payments/{id}/card  (shopper's token)
         └─ payment → NANO POST /api/payment/approval.io  (API_KEY header, encData)
              ← resultCode 0000 → payment succeeded → payment.succeeded → order paid
```

1. `POST /api/v1/payments` (`method: credit_card`) is unchanged, except that with key-in on the payment's `checkout_url` is **empty**: there is no window to send the shopper to. That is how the storefront knows to show its form. A payment created before key-in was switched on keeps its 인증결제 `checkout_url` and still goes through the old bridge.
2. The storefront's `/checkout/pay/:id` shows the form and posts it from its server (an SSR `action`), never from browser JavaScript to a gateway route.
3. `POST /api/v1/payments/{id}/card` checks the card, checks the order is still `pending` at the payment's amount, claims the payment, and calls NANO once.

## `POST /api/v1/payments/{id}/card`

Bearer, and only the shopper who owns the payment (no staff path). Body, all strings:

```json
{ "card_number": "4111111111111111", "exp_month": "07", "exp_year": "30",
  "card_password": "12", "holder_id": "900101", "installment": "00" }
```

- `card_number`: 14–19 digits, Luhn-valid; spaces and dashes are dropped.
- `exp_month` / `exp_year`: `MM` / `YY`; a card is good through the end of its month (KST).
- `card_password`: the first two digits of the card PIN.
- `holder_id`: birth date `YYMMDD`, or the 10-digit business number for a corporate card.
- `installment`: `00` (lump sum, the default) or `02`–`12`, only for ₩50,000 and up.
- Unknown fields are refused (`400`), so a client cannot add CVC or anything else by accident.

| Status | `reason` | Meaning | Storefront |
|---|---|---|---|
| `200` | | The payment, `status: succeeded`. A retried submit after success answers the same. | Go to the confirmation page |
| `422` | `invalid_card` | `field` names what failed. NANO was not called; no attempt used. | Point at the field |
| `402` | `card_declined` | NANO refused. `message` is NANO's own text (비밀번호 오류, 한도 초과, …), `attempts_remaining`. | Show it, let them try again |
| `409` | `card_attempts_exhausted` | Five declines; the payment is `failed`. | Start a new payment from the order |
| `409` | `card_approval_in_progress` | A double submit while the first is still at NANO. | Wait, then reload |
| `409` | `card_approval_unconfirmed` | NANO never answered. The card **may** have been charged. | Say so; never offer another try |
| `409` | `payment_method_disabled` | The card kill switch is on. | The operator's message |
| `400` / `403` / `404` | | Order no longer pending, not your payment, no such payment. | |

The response never contains the card. `Cache-Control: no-store`.

## Never paying twice

The dangerous case is a charge we did not record, followed by the shopper paying again.

- **One call at a time.** The payment row is locked to claim it (`card_attempts`, `card_approval_started_at`) before NANO is called, so a double click cannot send two approvals.
- **No retries.** The approval POST is sent once with a 30-second timeout. A retry after a lost response could charge twice.
- **An unanswered call holds the payment.** Timeout, a dropped connection, a non-2xx or a body with no `resultCode` all set `card_approval_unconfirmed`, raise `payment.callback_rejected` with `source: key_in` and `reason: approval_unconfirmed` (Telegram: "카드 수기결제 응답 없음"), and refuse every later attempt. `POST /api/v1/payments` for the same order hands back the same held payment, so the shopper cannot start over either. A process that dies mid-call leaves `card_approval_started_at` set, which counts as held after 45 seconds.
- **An approval that does not match is held too.** Another `shopcode` or `reqPayAmt` than ours raises the same alert (`shop_mismatch` / `amount_mismatch`).
- **Declines are bounded.** Five per payment (`domain.MaxCardAttempts`), then the payment fails. It leaves room for a mistyped PIN or birth date and slows down card testing.

**Ops, on a "응답 없음" alert:** look the payment up in the NANO console by `compOrderNo` (our payment id). If NANO approved it, refund it there (we have no `tranNo` to refund with). Either way the order auto-cancels when unpaid; the shopper places it again.

## Card data

- Never stored: no column, no log line, no error message, no event carries any part of it. `CardFieldError` names a field, never a value.
- Encrypted for NANO as §1 says: the card JSON (`cardNo`, `cardExpYy`, `cardExpMm`, `cardPw`, `cardHolderYmd`) → AES-256-CBC with PKCS7 → base64, in `encData`. `EncryptNanoCardData` is checked against `openssl enc -aes-256-cbc`.
- Kept from NANO's answer: `tranNo` (as `provider_ref`, for refunds). The masked number and card name are logged on approval; an unmasked `cardNo` in a response is dropped.
- Sentry never sends request bodies (`shared/pkg/sentrymon`); the gateway logs no bodies.
- Key-in puts Dupli1 in PCI DSS scope for card data in transit (storefront SSR → payment → NANO).

## Configuration

| Env | |
|---|---|
| `NANO_API_KEY`, `NANO_SHOPCODE`, `NANO_LOGIN_ID`, `NANO_VER`, `NANO_BASE_URL` | As before |
| `NANO_ENC_KEY` | encData key, exactly 32 bytes |
| `NANO_ENC_IV` | encData IV, exactly 16 bytes |

Both `NANO_ENC_*` set: key-in. Either missing: card checkout stays on 인증결제. A key or IV of the wrong length is logged at startup and leaves 인증결제 on. `GET /api/v1/payments/settings` reports `features.nano_key_in`.

The test merchant (`dev3.nanopay.co.kr`, shop `240000005`, login `shoptest`) and its key and IV are in §1 of the guide; the test server approves everything and charges nothing. Production values come with the contract and go in `/opt/dupli1/.env` on VENUS.

## Rollout

1. Deploy payment and the storefront. Neither depends on the other: an old storefront still opens the 인증결제 bridge, and the new one shows the form only for a payment with no `checkout_url`.
2. Set `NANO_ENC_KEY` / `NANO_ENC_IV` and restart payment. New card payments are key-in from then on.
3. Remove the 인증결제 bridge (`/nano/checkout`, `/nano/return`, `/webhooks/nano`, `BuildRequest`, the return MAC) one release later, once no open payment can still be using it.
