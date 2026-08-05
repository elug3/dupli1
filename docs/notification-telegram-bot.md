# Notification Telegram bot

Design and operations guide for the Dupli1 ops Telegram bot (`dupli1-notification`).

**Status:** Bot token + allowlist + `/start` implemented (env/config). **Target:** chat IDs and routing stored in auth DB and managed via Manager Settings API (not Secrets Manager).

**Related:** [current-state.md](current-state.md), [deployment-aws.md](deployment-aws.md), [manager-settings-api.md](manager-settings-api.md), [payment-service.md](payment-service.md).

---

## Purpose

The Telegram bot gives **operations staff** real-time alerts from the marketplace:

- New and updated orders (especially **paid** orders that need shipping)
- Product catalog changes (create, update, delete, image upload)

It is **not** a customer-facing channel. Shoppers never interact with this bot.

```text
Product / Order / Payment services
        │ publish NATS events
        ▼
   dupli1-nats
        │
        ▼
 dupli1-notification  ──►  Telegram Bot API  ──►  ops group / DM
        ▲
        │ long-poll getUpdates (commands)
        └── /start from allowlisted users
```

---

## Components

| Piece | Role |
|-------|------|
| `dupli1-notification` | Subscribes to NATS; formats HTML messages; sends via Bot API |
| `dupli1-nats` | Event bus (`order.*`, `product.*`, `payment.succeeded` consumed indirectly via order) |
| Telegram Bot API | Outbound `sendMessage`; inbound `getUpdates` for `/start` |
| Secrets Manager `dupli1/production/telegram` | **Bot token only** (target). Transitional: also holds chat IDs via env |
| Auth DB `dupli1_db` | **Target:** chat routing, allowlist, channel/event toggles |

Production bot (2026-08): `@MHYM7_BOT` (`dupli1_notification`).

---

## Security model

### Principle: least privilege

| Data | Sensitivity | Store (target) | Store (today) |
|------|-------------|----------------|---------------|
| `TELEGRAM_BOT_TOKEN` | **Secret** — full send access as the bot | Secrets Manager | Secrets Manager |
| Telegram **user IDs** (allowlist) | ACL — who may use bot commands | Auth DB | Env `TELEGRAM_ALLOWED_USER_IDS` |
| **Chat IDs** (destinations) | Config — where alerts go; not credentials | Auth DB | Env / Secrets Manager (transitional) |
| Channel / event toggles | Policy | Auth DB (`settings.notifications`) | Not implemented |

### Access rules (implemented)

1. **Outbound ops alerts** — sent only to configured `TELEGRAM_ORDER_CHAT_ID` and `TELEGRAM_PRODUCT_CHAT_ID` (allowlist enforces chat IDs on `Send`).
2. **`/start` replies** — only for:
   - Telegram users in `TELEGRAM_ALLOWED_USER_IDS`, or
   - chats that already match a configured order/product chat ID.
3. **Everyone else** — silently ignored (no reply, no error leaked to sender).

### Why chat IDs should not live in Secrets Manager

Chat IDs are **routing configuration**, not secrets. Keeping them in Secrets Manager forces AWS console edits and ECS redeploys for every group change. The target design persists them in the database and exposes them through the Manager Settings API (see [Configuration tiers](#configuration-tiers) below).

---

## Configuration tiers

### Today (v1 — env / Secrets Manager)

Injected into `dupli1-notification` as environment variables (Compose or ECS secrets):

| Variable | Required | Purpose |
|----------|----------|---------|
| `TELEGRAM_BOT_TOKEN` | Yes (for Telegram) | Bot API token from [@BotFather](https://t.me/BotFather) |
| `TELEGRAM_ALLOWED_USER_IDS` | Recommended | Comma-separated Telegram **user** IDs allowed to use `/start` |
| `TELEGRAM_ORDER_CHAT_ID` | For order alerts | Group/supergroup/channel or DM chat ID |
| `TELEGRAM_PRODUCT_CHAT_ID` | For product alerts | Same; may equal order chat for a single ops channel |
| `NATS_URL` | Yes (for dispatch) | e.g. `nats://nats.dupli1.local:4222` |

If order/product chat IDs are empty, events are **logged and skipped** (no Telegram send). Core NATS does not redeliver — a missed alert is only visible in CloudWatch (`/ecs/dupli1-notification`).

### Target (v1.1+ — database + Manager Settings)

Host mutable notification config in **auth** (`dupli1_db`), consistent with [manager-settings-api.md](manager-settings-api.md):

```json
{
  "allow_outbound": true,
  "channels": {
    "telegram": true
  },
  "telegram": {
    "order_chat_id": "-1001234567890",
    "product_chat_id": "-1001234567890",
    "allowed_user_ids": [123456789, 987654321]
  },
  "events": {
    "order.created": true,
    "order.status_updated": true,
    "order.paid": true,
    "product.created": true,
    "product.updated": true,
    "product.deleted": true,
    "product.image_uploaded": true
  },
  "etag": "…"
}
```

**Enforcement:**

- `dupli1-notification` loads the `notifications` section on startup and reloads on NATS `settings.updated`.
- Only `TELEGRAM_BOT_TOKEN` remains in Secrets Manager.
- Manage-web edits `PATCH /api/v1/settings/notifications` (requires `settings.update`).

**Optional table** (if settings JSON is too coarse):

```sql
-- dupli1_db (auth), illustrative
CREATE TABLE notification_telegram_destinations (
  purpose     TEXT NOT NULL CHECK (purpose IN ('order', 'product')),
  chat_id     TEXT NOT NULL,
  chat_label  TEXT,
  enabled     BOOLEAN NOT NULL DEFAULT TRUE,
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (purpose)
);

CREATE TABLE notification_telegram_allowed_users (
  telegram_user_id  BIGINT PRIMARY KEY,
  display_name      TEXT,
  registered_chat_id TEXT,  -- filled when user sends /start
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

`/start` (target behaviour): allowlisted user sends `/start` → service upserts `registered_chat_id` → manager assigns that chat to `order` or `product` purpose in the UI (or auto-bind for single-channel setups).

---

## NATS events → Telegram messages

| NATS subject | Destination chat | Message summary |
|--------------|------------------|-----------------|
| `order.created` | Order | New order, status, customer, items, total (KRW) |
| `order.status_updated` | Order | Status change |
| `order.paid` | Order | **Paid — action required** (ship when ready) |
| `product.created` | Product | New product, brand, category, price |
| `product.updated` | Product | Updated product |
| `product.deleted` | Product | Deleted product |
| `product.image_uploaded` | Product | Image URL |

Messages use **HTML** (`parse_mode: HTML`). Amounts use KRW formatting via `shared/pkg/money`.

Publishers: `order` and `product` services (payment success flows through order → `order.paid`).

---

## Bot commands

| Command | Who | Behaviour |
|---------|-----|-----------|
| `/start` | Allowlisted users only | Welcome text + **chat ID** for ops setup (today: paste into config; target: auto-register in DB) |

No other commands are implemented. Unknown commands are ignored.

**Inbound transport:** long-polling `getUpdates` (no public webhook). On startup the service calls `deleteWebhook` so polling works.

---

## Setup runbook

### 1. Create the bot

1. Open [@BotFather](https://t.me/BotFather) → `/newbot` → save the **token**.
2. Store token in Secrets Manager:

```bash
aws secretsmanager put-secret-value --region us-east-1 \
  --secret-id dupli1/production/telegram \
  --secret-string '{"TELEGRAM_BOT_TOKEN":"<token>"}'
```

### 2. Allowlist ops users

Get each ops person's Telegram **user ID** (e.g. [@userinfobot](https://t.me/userinfobot)).

**Today (env):**

```bash
# Add to secret or .env
TELEGRAM_ALLOWED_USER_IDS=123456789,987654321
```

**Target:** add IDs via `PATCH /api/v1/settings/notifications` or manage-web.

### 3. Configure alert destinations

**Option A — ops group (recommended)**

1. Create a private Telegram supergroup for ops.
2. Add `@MHYM7_BOT` to the group.
3. An allowlisted user sends `/start` in the group (or DM) to learn the **chat ID** (negative for groups).
4. Set `TELEGRAM_ORDER_CHAT_ID` and `TELEGRAM_PRODUCT_CHAT_ID` to that ID.

**Option B — separate channels**

Use one chat ID for orders and another for catalog alerts.

**Today:** update secret + redeploy notification service:

```bash
aws ecs update-service --region us-east-1 \
  --cluster production --service dupli1-notification \
  --force-new-deployment
```

**Target:** set chat IDs in Manager Settings — no redeploy.

### 4. Local development

In `.env` (see `.env.example`):

```env
TELEGRAM_BOT_TOKEN=
TELEGRAM_ALLOWED_USER_IDS=
TELEGRAM_ORDER_CHAT_ID=
TELEGRAM_PRODUCT_CHAT_ID=
```

Run the stack: `sudo docker compose up --build`. Trigger a test order or product change; check `dupli1-notification` logs.

---

## Operations

### Health

- `GET /api/v1/notification/health` → `{"status":"ok"}`
- `GET /api/v1/notification/settings` → runtime flags (`telegram_enabled`, `order_chat_configured`, …)

### Logs

CloudWatch: `/ecs/dupli1-notification`

| Log line | Meaning |
|----------|---------|
| `notification dispatcher subscribed to order and product events` | NATS wiring OK |
| `telegram command poller started` | Inbound `/start` active |
| `… skipped: TELEGRAM_ORDER_CHAT_ID not set` | Event received; no destination configured |
| `notification nats handler subject=… error=…` | Handler failed; alert **not** retried |

### Failure behaviour

- NATS: at-most-once delivery; failed Telegram sends are logged and dropped.
- Missing token: Telegram disabled; NATS handler may still run.
- Missing chat ID: per-event skip with log line.
- Non-allowlisted `/start`: no reply.

### Rotating the bot token

1. Revoke/regenerate in BotFather.
2. Update `TELEGRAM_BOT_TOKEN` in Secrets Manager.
3. Redeploy `dupli1-notification`.
4. Chat IDs and allowlist unchanged.

---

## Implementation map

| Area | Path |
|------|------|
| NATS dispatcher | `notification/pkg/service/dispatcher.go` |
| Telegram client | `notification/pkg/infra/telegram/client.go` |
| Allowlist | `notification/pkg/infra/telegram/allowlist.go` |
| `/start` handler | `notification/pkg/infra/telegram/commands.go` |
| Update poller | `notification/pkg/infra/telegram/poller.go` |
| Bootstrap | `notification/pkg/bootstrap/bootstrap.go` |
| ECS secrets (transitional) | `infra/terraform/ecs_services.tf` |

---

## Roadmap

| Step | Description |
|------|-------------|
| **Done** | NATS → Telegram dispatch; allowlist; `/start` with chat ID |
| **Next** | Move chat IDs + allowlist to auth DB; `settings.notifications` section |
| **Then** | Manage-web UI for destinations and event toggles |
| **Later** | `/start` auto-registers chat in DB; optional per-user DM routing |

---

## Open questions

1. **Single ops channel vs split** — default both order and product to one group, or require two?
2. **Auto-bind on `/start`** — should the first allowlisted user's group become `order_chat_id` without manual PATCH?
3. **Audit** — log Telegram sends to `settings/audit` or a dedicated `notification_deliveries` table?
