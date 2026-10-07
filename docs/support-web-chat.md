# Support — web consultation chat

A signed-in shopper can open a consultation with staff from the storefront, attach the product or order they are asking about, and get the reply in the same panel. It is a **second channel of `support`**, beside the Telegram bot ([support-telegram-bot.md](support-telegram-bot.md)): web inquiries land in the same manage-web `/support` inbox, where staff claim, reply and close them as they do Telegram ones, with a context panel beside the conversation.

Decided 2026-10-01 / 10-03 (spec: the "Web consultation chat spec" doc linked from project memory):

- Signed-in shoppers only. Signed-out shoppers keep the Telegram button.
- Staff see the shopper's email, the product or order they asked about, and their purchase history (order count, lifetime spend, last orders). Staff do **not** see the profile phone or saved addresses.
- `support_agent` gains `order.read.all`, so an agent can read the orders behind the context panel. Scoping it to the chatting customer would need a new order endpoint.
- If a reply is left unread for 5 minutes, the shopper gets an **email**. It never carries the reply text, only what the consultation is about and a link back to the chat.
- Transport is **SSE plus plain POSTs**, not WebSocket, for the same reasons as the live order feed ([order-live-events.md](order-live-events.md)): the browser `WebSocket` API cannot send `Authorization`, and the BFF session gateways forward with `fetch`, which cannot carry an upgrade.

## Model

| | Telegram | Web |
|---|---|---|
| Conversation key (`chat_id`) | Telegram chat id | `web:<customer id>` — one per account, so the existing unique index holds |
| `channel` | `telegram` (also the default for rows from before this) | `web` |
| Who is asking | `username` | `customer_id`, `customer_email` (refreshed from the token on each send) |
| Read state | — | `customer_last_read_at`; `customer_notified_at` for the reply notice |

An inquiry carries the same `channel` and `customer_id`, plus `product_id` / `sku_id` / `order_id` from the message that opened it. Its `topic` is `ord`, `prd` or `agt` depending on which was attached.

A message has a `kind`: `text`, `product_ref`, `order_ref` or `system` (after-hours note, "상담이 종료되었습니다"). A reference message keeps a short readable body (`상품: Prada Galleria (Black)`) and a `ref_snapshot` JSON of what the card showed when it was sent, so a later price or status change does not rewrite history. The retention purge and account deletion drop the snapshot along with the body.

All columns are additive (`ADD COLUMN IF NOT EXISTS`), so a Telegram-only deployment migrates in place.

## References

Each reference is checked before anything is written, so a refused one leaves no half-sent message.

- **Product** — by `sku_id` (the sellable variant), optionally with `product_id`, which must match. Resolved through product's variant lookup (`shared/pkg/productclient`) via the gateway. `GET /products/{id}` is deliberately not used: it counts a PDP view. A `product_id` without a `sku_id` is refused.
- **Order** — resolved as `GET /api/v1/orders/{id}` through the gateway **with the caller's own token**. For a shopper, order's ABAC allows only their own orders. For staff, `order.read.all` allows any order. A `403` and a `404` both come back as `invalid_reference`, so a shopper cannot probe which order ids exist.

Without `DUPLI1_GATEWAY_URL`, references answer `503 reference_unavailable` and text still works.

## API

Customer routes act on the caller's own conversation only; there is no id in the path. Any signed-in customer or manager account may use them (managers shop too); service accounts get `403`.

| Method | Path | Notes |
|---|---|---|
| GET | `/api/v1/support/web/conversation` | `{ conversation: { inquiry, messages, unread, service_open, service_window } }`. `inquiry` is the open one or `null`. Messages are the last 200. A read creates nothing |
| POST | `/api/v1/support/web/messages` | `{ body?, product_id?, sku_id?, order_id? }` — text, references, or both. Opens an inquiry when none is open (and announces it to staff); otherwise joins the open one. Answers the conversation |
| POST | `/api/v1/support/web/read` | `204`. Moves the read mark to now |
| POST | `/api/v1/support/web/inquiries/current/close` | Shopper ends the consultation. Writing again opens a new one in the same conversation |
| GET | `/api/v1/support/web/events` | SSE, the caller's own changes |

A shopper's message shows no author, no delivery error and no notice status. Which manager answered is staff business.

Errors carry a stable `code` for the storefront's copy:

| Status | `code` | When |
|---|---|---|
| 400 | `invalid_message` | No text and no reference, or more than 2000 characters |
| 422 | `invalid_reference` | Unknown SKU, mismatched product, an order that is not yours |
| 503 | `reference_unavailable` | Product or order could not be reached |
| 429 | `rate_limited` | More than 20 messages a minute or 500 a day (per replica), `Retry-After: 60` |
| 403 | `customer_required` | Service account |
| 503 | `chat_unavailable` | Web chat not wired |

Staff routes are the existing inbox, extended:

- `GET /api/v1/support/inquiries?channel=web|telegram` filters by channel. Each inquiry now has `channel`, and for web ones `customer_id`, `customer_email`, `customer_last_read_at` (the console's 읽음), `product_id`, `sku_id` and `order_id`.
- Transcript messages carry `kind`, and for cards `ref_id` and `ref`. Web replies also carry `notice_status` (`pending` → `sent` / `failed` / `skipped`).
- `POST …/inquiries/{id}/reply` also takes `sku_id` / `order_id` to send a card. The order is read with the manager's token. On a Telegram inquiry a card is `422 reference_on_telegram`, since the bot can only send text. A web reply never fails delivery: it is stored, pushed to the shopper's stream, and left for the notice job. `delivered` is always `true`.
- `GET /api/v1/support/inquiries/events` is SSE for every change, gated on `support.read`.

The context panel's purchase history is not served by support: manage-web reads `GET /api/v1/orders?customer_id=` with the operator's token (`order.read.all`).

## Live updates

Frames carry **ids only, never message text**. A shopper's address typed into the chat never reaches NATS, the gateway's logs or a proxy buffer through this path.

```
event: ready     data: {}                                        (first frame; load or reload through REST)
event: message   data: {"type":"message","inquiry_id":"…","message_id":"…"}
event: inquiry   data: {"type":"inquiry","inquiry_id":"…","status":"answered"}
```

The inbox stream adds `conversation_id` and `channel`. Clients refetch over REST on every frame. Because the REST answer already holds everything that changed while a stream was down, a reconnect needs no replay buffer and no `Last-Event-ID`.

Each replica holds a `livefeed.Hub`. With NATS configured, every change is published to `support.message_created` / `support.inquiry_updated`, and every replica holds a **broadcast** (non-queue) subscription that feeds its hub. That way a reply saved on one replica reaches a shopper streaming from another. Without NATS, events go straight into the local hub.

A stream sends `retry: 3000`, heartbeats every 20s (inside nginx's 60s read timeout), bounds each write to 10s, and ends when the token that opened it expires (at most 15 minutes). The BFF then reconnects with a fresh token. The gateway serves both stream paths unbuffered (`api/gateway/routes.conf`).

## Reply notice

A web reply is stored with `notice_status = pending`. Every minute, `SendDueNotices` picks pending replies older than 5 minutes and groups them by conversation:

- If the shopper has read past the newest one, or an email already covers this unread batch (`customer_notified_at` after `customer_last_read_at`), they are marked `skipped`.
- Otherwise, one email goes out for the batch, and the replies are marked `sent`, or `failed` when SMTP refused.

The next notice can only follow a read. The subject is the product name or `주문 <id>`, never the reply text, and the link is `DUPLI1_STOREFRONT_URL/profile/support`. The email is written in Korean, then English, because support does not know which language the shopper uses. This is the backend's first customer email.

Without SMTP or a storefront URL, every reply ends `skipped` and chat works normally.

## Account deletion

support joins `user.deleted` (queue group `support-workers`, so one replica handles each deletion). It closes the open web inquiry, replaces every message body in the conversation with the purge placeholder and drops the snapshots, and clears the email. The rows stay, as with the 180-day retention purge.

## Configuration

| Variable | Purpose |
|---|---|
| `DUPLI1_GATEWAY_URL` | Internal gateway for product/order references (local compose: `http://dupli1-proxy:8081`) |
| `DUPLI1_STOREFRONT_URL` | Storefront origin for the notice link |
| `DUPLI1_SUPPORT_SMTP_ADDR` | `host:port`; unset disables notices |
| `DUPLI1_SUPPORT_SMTP_USERNAME` / `_PASSWORD` | Optional; STARTTLS is used when offered, and PLAIN auth is refused on cleartext to a non-local host |
| `DUPLI1_SUPPORT_SMTP_FROM` | Sender address |

`NATS_URL` (live fan-out, `user.deleted`), `AUTH_JWKS_URL` and `DUPLI1_SUPPORT_DB` are as before.

support is not yet in production (see [support-telegram-bot.md](support-telegram-bot.md#deploying-phase-7)). Web chat ships with its first deployment, which needs these variables plus SMTP credentials.
