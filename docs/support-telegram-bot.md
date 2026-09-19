# Support Telegram bot (customer inquiries)

Design spec for the **customer-facing** Telegram inquiry bot: menu-driven consultation, conversation state, and handoff to a human operator.

**Status:** Phases 0–6 complete, and Phase 7 prepared but not applied — the consultation works end to end, the storefront button opens the bot carrying where the shopper came from, and the Terraform, CI rows and retention job for production are all in the tree. **Nothing runs in production yet:** the bot stays unreachable until an operator creates the two secrets and runs `terraform apply` ([Deploying](#deploying-phase-7)). Supersedes nothing; the ops bot in [notification-telegram-bot.md](notification-telegram-bot.md) stays exactly as it is.

**Scope (Tier 2):** inline-keyboard consultation menus, canned answers, and human handoff. **Out of scope (Tier 3):** authenticated order lookups ("where is my order?"), which need a Telegram↔customer identity binding — see [Deferred: authenticated lookups](#deferred-authenticated-lookups).

**Related:** [notification-telegram-bot.md](notification-telegram-bot.md), [permissions.md](permissions.md), [service-layout.md](service-layout.md), [current-state.md](current-state.md), [deployment-aws.md](deployment-aws.md).

---

## Purpose

The storefront's floating Telegram button (`dupli1-web`, `app/components/telegram-float.tsx`) sends shoppers to a chat. Today that is a human account. This spec replaces it with a bot that answers the common questions immediately and escalates the rest to staff — the affordance Korean storefronts carry as a KakaoTalk consultation menu.

```text
                shopper taps floating button on / or /category/*
                                │  t.me/<support_bot>?start=<context>
                                ▼
                      Telegram Bot API
                                │  webhook POST (message + callback_query)
                                ▼
                    ┌───────────────────────┐
                    │   dupli1-support      │  menu router + conversation state
                    │   (new service)       │  PostgreSQL `support`
                    └───────────┬───────────┘
                                │ publishes support.inquiry_opened (NATS)
                                ▼
                          dupli1-nats
                                │
                                ▼
                    dupli1-notification  ──►  ops Telegram chat
                    (existing fan-out, unchanged routing)

     manager replies from manage-web /support ──► dupli1-support ──► shopper's chat
```

---

## Why a second bot, not the ops bot

Decided. Three independent reasons:

1. **One token owns one update stream.** The existing code already names this failure: `ErrUpdatesConflict` (`shared/pkg/telegram/updates.go`) is Telegram's `409` when a second consumer polls the same bot, or when a webhook is registered while something polls. Sharing a token means one webhook and one router serving two unrelated audiences.
2. **Opposite trust models.** The ops bot is closed by construction: an unknown chat is parked as `pending` and hears nothing until a manager accepts it (`notification/pkg/infra/telegram/processor.go`), and `TELEGRAM_ALLOWED_USER_IDS` gates inbound commands. A customer bot must answer a stranger on the first message and must never allowlist anyone.
3. **Blast radius.** A flood, a spam wave, or a formatting bug on the customer side must not be able to disturb the channel where paid-order alerts land.

Each bot therefore gets its own token, its own webhook URL and secret, its own database, and its own service.

---

## Service placement

New service `support/` (`github.com/elug3/dupli1/support`), following the repo's one-module-per-service convention and the `profile` extraction as the closest template.

The Bot API client is **extracted to `shared/pkg/telegram`** rather than copied. This follows the precedent already set by `shared/pkg/natspublisher`, `shared/pkg/authmiddleware`, and `shared/pkg/productclient` — each extracted when a second consumer appeared. The extraction is Phase 0 below and must be behavior-preserving for `notification`.

**Alternative considered:** a second adapter inside `notification`. Rejected — but on the strength of the cost check below, not on taste.

### Is separating worth it?

**Decided: yes.** The cost was measured against this repo rather than assumed, and it is smaller than the usual "one more microservice" instinct suggests.

What a new service actually costs here:

| Cost | Measured | Notes |
|---|---|---|
| Boilerplate | **~420 lines** | `profile`'s `cmd/main.go` + `cmd/options.go` + `pkg/server.go` + `pkg/bootstrap` + `pkg/options.go`, mostly copy-adapt |
| CI | 3 entries | one job in `test.yml`, a build-matrix row and a deploy row in `aws.yml`, plus an ECR repo |
| Database | **no new instance** | services hold their own database on the shared `dupli1-production` RDS, injected per service from Secrets Manager (`DUPLI1_*_DB`). [aws-cost-reduction-plan.md](aws-cost-reduction-plan.md) lists RDS at $2.40 |
| ALB / NAT | **none** | it sits behind the existing nginx gateway. Those are the expensive line items — the $50–70 idle mode is ALB + NAT |
| Compute | **no new billing unit** | ECS on **EC2**, not Fargate: 2 × `t3.large` (16 GB) carrying ~4 GB of task reservations across 12 services. An extra 256 MB task uses headroom already paid for |

The one constraint worth checking before Phase 2 is **ENI slots, not memory** — `infra/terraform/variables.tf:181` names it directly ("2×t3.large packs all services" with `awsvpcTrunking`, "without trunking, raise to ~5"). Trunking is enabled (`ecs_ec2.tf:141`), and 11 of ~20 branch-ENI slots are in use, so there is room. If that ever changes, a third instance is real money and this table needs re-reading.

What separating buys, at the service level specifically (the bot-level arguments above are already satisfied by either option — the adapter would also have its own token, webhook and update stream):

1. **The ops alert path is the money path's last mile.** It is how staff learn an order was paid. In one process, a customer-traffic flood, a goroutine leak, or a panic in menu rendering shares that task's CPU, DB pool and lifecycle. The single strongest argument for separation is that the failure to avoid is "nobody found out the order was paid."
2. **Deploy independence.** Menu copy and conversation logic will change far more often than ops alerting. Each deploy of the former should not risk the latter.
3. **Different shape of work.** `notification` is a NATS subscriber that formats outbound messages. This is a stateful, conversational, public-facing HTTP service with a manager inbox. Putting it inside would mean `notification` no longer has a one-sentence job.

Honest counterweight: `notification` already exposes a public webhook, so the public surface is not new *in kind*; and one more service is one more thing to deploy, monitor, roll back and keep current. That is the real recurring cost — human, not dollar.

**What is actually expensive to reverse is the module and data boundary, not the deployment topology.** A separate module can be co-located later far more easily than a tangled one can be split. That asymmetry is what settles it: separate the module now, and treat the ECS task count as the cheap, revisitable decision it is.

```
support/
├── cmd/                    # main.go, options.go (env → ServerOptions)
└── pkg/
    ├── domain/             # Conversation, Inquiry, MenuNode, state transitions
    ├── service/            # Router (menu walk), Handoff, Reply
    ├── ports/              # ConversationRepository, InquiryRepository, Publisher, Bot
    ├── infra/
    │   ├── telegram/       # adapter over shared/pkg/telegram: update → domain intent
    │   ├── postgres/       # conversations, inquiries, messages
    │   ├── memory/         # in-memory fallback (tests, local dev without DB)
    │   └── nats/           # publish support.inquiry_opened / support.inquiry_closed
    ├── handler/            # POST webhook (public) + manager inbox API (authed)
    └── bootstrap/          # wiring, config, settings
```

---

## What must be built in the Bot API client

All four gaps below are closed as of Phase 1; the table is kept as the record of what was built and why.

| Gap | Where it is today | What is needed |
|---|---|---|
| No `reply_markup` | `sendMessage` marshals a `map[string]string` of exactly `chat_id`, `text`, `parse_mode` | Typed payload struct with optional `reply_markup` (inline keyboard) |
| Button taps never arrive | `Update` decodes only `message`; `SetWebhook` registers `allowed_updates: ["message"]` | `CallbackQuery` type + `"callback_query"` in `allowed_updates` |
| No `answerCallbackQuery` | absent | Required — until it is called, the button shows a spinner on the shopper's device |
| No message edit | absent | `editMessageText` so a menu step replaces itself instead of stacking new messages |

Keep from the existing client as-is: token redaction in errors (`redactedError`), the retry/backoff budget, HTML parse mode, and the 4096-char truncation.

### Bot API constraints that shape the design

Verify each against the Bot API docs at implementation time; these are the documented limits the design assumes:

- **`callback_data` ≤ 64 bytes.** Menu routing cannot put prose in the payload — hence the compact scheme below.
- **`?start=` deep-link payload ≤ 64 chars**, restricted charset (`A-Z a-z 0-9 _ -`). Storefront context must be encoded, not passed as a URL.
- **A bot cannot message a user first.** Manager replies only work because the shopper opened the chat; if they block the bot, `sendMessage` fails with `403` and the inbox must show that, not retry forever.
- **Rate limits** (~1 message/second per chat, ~30/second overall). The reply path needs a per-chat queue, not a tight loop.

---

## Menu tree

Root menu, sent on `/start` and reachable from every leaf via `⬅️ 처음으로`:

| Button | Node | Behavior |
|---|---|---|
| 📦 주문·배송 문의 | `ord` | Sub-menu: 배송 기간 / 배송 조회 / 주소 변경 → canned answers; 그 외 → handoff |
| 🛍 상품·재고 문의 | `prd` | Canned sizing/material/stock copy; deep-link context names the product when present → handoff |
| 🔁 교환·반품 | `ret` | Policy copy (14-day window, condition rules), then handoff with the order number asked for as free text |
| 💳 결제 문의 | `pay` | Canned payment-method copy → handoff |
| 🙋 상담원 연결 | `agt` | Immediate handoff |

Canned answers live in the database (`support_answers`), not in Go constants, so staff can edit copy from the manage-web inbox without a deploy. Seed them from a migration.

```sql
CREATE TABLE support_answers (
  node       TEXT NOT NULL,
  language   TEXT NOT NULL DEFAULT 'ko',
  body       TEXT NOT NULL,              -- Telegram HTML
  updated_by TEXT,
  updated_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (node, language)
);
```

**Language is in the key from day one even though only `ko` rows ship.** This repo migrates schema inline on startup and supports additive `ADD COLUMN IF NOT EXISTS` only — "breaking schema changes are not supported this way" (root `CLAUDE.md`). Widening a primary key later is exactly such a change. One unused column now costs nothing; retrofitting the key costs a hand-written migration on a live table.

### Language policy

**Decided: Korean only at launch.** Menus, canned answers, and staff replies are Korean, matching the ops bot's existing message style (`notification/pkg/infra/telegram/commands.go`).

Two things still happen for non-Korean shoppers, because "Korean only" should not mean "silently Korean":

1. **The entry language is recorded, not acted on.** The deep-link payload already carries the storefront's active language, and `support_conversations.language` stores it. Every reply is Korean regardless. Recording it is nearly free now and impossible to backfill later, and the count of `en` / `zh` inquiries is exactly the evidence that decides whether a second language is worth staffing.
2. **A non-Korean shopper is told, once, in their own language.** On first contact with an `en` or `zh` entry language, the bot prepends one static sentence — *"Support is currently available in Korean only."* — before the Korean root menu. Two hardcoded strings, not a translation surface: a sign on the door, not a second shop. Arriving at a wall of Korean with no explanation is the one outcome worth avoiding.

The storefront button stays visible in all three languages. It still works — a shopper who reads Korean is served regardless of which UI language they picked, and the one-line notice handles the rest.

### Callback data scheme

`v1:<node>:<arg>` — version prefix, 3-char node id, optional short arg. Fits 64 bytes with room to spare, and the version prefix means a menu redesign can ignore taps from a stale message rather than misroute them (Telegram keeps old messages tappable forever).

Unknown or stale callback data → answer the callback, then re-send the root menu. Never error at the shopper.

---

## Conversation state

One row per Telegram chat, plus an inquiry row per escalation.

```sql
CREATE TABLE support_conversations (
  id              TEXT PRIMARY KEY,           -- ULID
  chat_id         TEXT NOT NULL UNIQUE,
  telegram_user_id BIGINT,
  username        TEXT,
  language        TEXT NOT NULL DEFAULT 'ko',
  node            TEXT NOT NULL DEFAULT 'root', -- current menu node
  entry_context   JSONB,                       -- decoded ?start= payload
  last_seen_at    TIMESTAMPTZ NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL
);

CREATE TABLE support_inquiries (
  id              TEXT PRIMARY KEY,
  conversation_id TEXT NOT NULL REFERENCES support_conversations(id),
  topic           TEXT NOT NULL,              -- ord | prd | ret | pay | agt
  status          TEXT NOT NULL,              -- open | assigned | answered | closed
  assigned_to     TEXT,                       -- auth user id
  opened_at       TIMESTAMPTZ NOT NULL,
  closed_at       TIMESTAMPTZ
);

CREATE TABLE support_messages (
  id              TEXT PRIMARY KEY,
  conversation_id TEXT NOT NULL REFERENCES support_conversations(id),
  inquiry_id      TEXT,
  direction       TEXT NOT NULL,              -- inbound | outbound
  author          TEXT,                       -- auth user id for manager replies
  body            TEXT NOT NULL,
  telegram_message_id BIGINT,
  created_at      TIMESTAMPTZ NOT NULL
);
```

Follows the repo's inline-migration convention (services migrate their own schema on startup; additive changes via `ADD COLUMN IF NOT EXISTS`). An in-memory repository backs local dev and tests when `DUPLI1_SUPPORT_DB` is unset, as order/cart/payment already do.

**State is a menu position, not a wizard.** The shopper can type free text at any node; that text is stored and, if an inquiry is open, forwarded to staff. Never trap someone in a flow — a bot that ignores typed Korean because it wanted a button press is worse than no bot.

---

## Deep links from the storefront

`telegramContactUrl()` (`dupli1-web`, `app/lib/contact.ts`) gains an optional context argument and emits `https://t.me/<bot>?start=<payload>`. The payload is a compact encoding (≤64 chars, `A-Za-z0-9_-`) of:

| Field | Example | Why |
|---|---|---|
| surface | `h` (home) / `c` (category) / `p` (product) | Where they were |
| ref | `b-louis-vuitton`, `t-shoulder-bags` — a one-letter facet code, then the slug | What they were looking at |
| lang | `ko` / `en` / `zh` | Recorded; the bot still answers Korean only |

Fields join with `_` and slugs use `-`, so the two never collide, and the facet code is split off at the *first* hyphen — otherwise `t-shoulder-bags` would read as facet `t-shoulder`. An over-long reference is **dropped rather than truncated**: a cut-off reference points at the wrong product, which is worse for staff than no reference at all.

So a shopper on `/category/brand/louis-vuitton` arrives with the bot already knowing the brand, and a future product-page button arrives with the SKU. A payload that does not decode is ignored — it is a hint, never trusted input, and never a permission.

This is the only change to `dupli1-web`; the button, its placement, and its route gating stay as shipped.

---

## Business hours

**Decided:** weekdays 10:00–22:00 **KST**, closed weekends and public holidays.

### Timezone

Store and compare in `Asia/Seoul` via `time.LoadLocation`, never in the container's clock — ECS tasks run UTC. Korea observes no DST, so the window is a fixed `01:00–13:00 UTC` and never crosses midnight in either zone, which means the weekday is the same in both. That is a convenience, not a licence to hardcode the UTC form: a future hours change (an evening extension, a Saturday shift) would break the equivalence silently.

### Holidays are not tracked

**Decided: the bot has no holiday calendar.** Managers simply do not answer on public holidays, and the bot does not know which days those are.

This drops a table, an admin screen, a yearly seeding chore, and the single likeliest way this feature rots — a calendar nobody refills is worse than no calendar, because it is silently wrong. The Korean calendar makes that chore unavoidable otherwise: Seollal (설날) and Chuseok (추석) are lunar, and substitute holidays (대체공휴일) are declared per year, so no rule in code derives them.

The cost is paid in the copy, not the code: **the bot must never name a specific day it will reply.** A promise of "내일 10시부터" made on the eve of Chuseok is wrong by four days. So the after-hours message states the *window* instead of a date, which stays true on a holiday, during a holiday week, and on an ordinary Tuesday alike. Weekday and clock checks still run — weekends and nights are computed exactly as before; only holidays are invisible.

If long holidays later prove painful, the cheap remedy is **one manual 휴무 toggle** in the inbox that a manager flips when leaving and clears on return — a single boolean with no yearly upkeep, not a calendar. Not specced here; noted so the option is not rebuilt from scratch.

### What changes after hours

Only the **handoff** — never the self-serve path. Menus, canned answers, and the whole tree stay available 24/7; a shopper who wanted the return policy at 2am gets it at 2am.

| Node | Open | Closed |
|---|---|---|
| Canned answers (배송 기간, 반품 정책, …) | Answer immediately | Answer immediately — unchanged |
| Any escalation, incl. 🙋 상담원 연결 | Create inquiry, alert ops chat, promise a reply shortly | Create inquiry, state the service window, alert ops **quietly** |

After-hours copy names the service window, never a date: *"지금은 상담 시간이 아닙니다. 남겨주신 문의는 접수되었으며, 영업 시간(평일 10:00–22:00)에 순차적으로 답변드립니다. 공휴일은 휴무입니다."* Never "잠시만 기다려 주세요" outside hours, and never a specific day — a promise the shop cannot keep is worse than a closed sign, and without a holiday calendar any named day is a guess. Naming the window costs the shopper nothing in precision they could have relied on anyway.

Two edges worth handling explicitly:

- **Near closing.** An inquiry opened at 21:55 should not promise same-day handling. Treat the last 15 minutes as after-hours for the *promise* only — still alert ops loudly, since someone may well still be there.
- **Queued overnight.** The inquiry row is created either way. `support.inquiry_opened` carries `after_hours: true` so `notification` can deliver it without a ping, and the morning shift opens the `/support` inbox to a queue sorted oldest-first rather than a wall of overnight alerts.

Hours live in config, not in Go constants (`DUPLI1_SUPPORT_HOURS_OPEN=10:00`, `..._CLOSE=22:00`, `..._DAYS=mon-fri`, `..._TZ=Asia/Seoul`), so a seasonal change is a deploy variable rather than a code change. A later move to manager-editable hours belongs with [manager-settings-api.md](manager-settings-api.md) if that sketch is ever built.

---

## Handoff to staff

When a node escalates, `support` writes the inquiry row and publishes `support.inquiry_opened` to NATS. `notification` subscribes and fans it out to the ops chats it already resolves (`notification/pkg/service/telegram_routing.go`).

**Why via NATS rather than sending to the ops chat directly:** `notification` owns ops-chat routing — the accepted-subscription rows, the `alert_order` / `alert_product` flags, and the env fallbacks. Two services writing to the same ops chat means two places to change when routing changes, and it would hand the customer-facing service credentials for the ops bot. This also reuses the delivery path that already exists rather than building a second one.

New subjects in `shared/pkg/events` (one canonical contract per publisher/subscriber pair, per that package's stated purpose):

| Subject | Payload |
|---|---|
| `support.inquiry_opened` | inquiry id, topic, language, entry context, first message excerpt, manage-web deep link, `after_hours` |
| `support.inquiry_closed` | inquiry id, resolution, handling duration |

Add an `alert_support` flag to `telegram_subscriptions` so ops chats can opt into inquiry alerts independently of order and product alerts.

---

## Manager inbox

Each inquiry row shows its entry language, so staff see who they are answering and so the `en`/`zh` counts that decide a second language are visible without a query. New tab in `dupli1-manage-web` at `/support`, using the SSR `loader`/`action` pattern already used by `/telegram` (`app/lib/server/notification.server.ts`) so the browser never calls the support service directly.

| Method | Route | Permission |
|---|---|---|
| `POST` | `/api/v1/support/telegram/webhook` | none — secret header, see below |
| `GET` | `/api/v1/support/inquiries` | `support.read` |
| `GET` | `/api/v1/support/inquiries/{id}` | `support.read` |
| `POST` | `/api/v1/support/inquiries/{id}/assign` | `support.reply` |
| `POST` | `/api/v1/support/inquiries/{id}/reply` | `support.reply` |
| `POST` | `/api/v1/support/inquiries/{id}/close` | `support.reply` |
| `GET`/`PUT` | `/api/v1/support/answers` | `support.manage` |

### Where managers work

Two surfaces with different jobs: Telegram **tells** them, manage-web **is where they work**.

1. **The ops Telegram chat notifies.** The `support.inquiry_opened` fan-out lands there with topic, entry language, the first message excerpt, and a deep link straight to that inquiry in manage-web. It is a doorbell, not a workbench: a reply typed in the ops chat reaches other staff, never the shopper.
2. **`/support` in manage-web is the workbench.** Three lists — **대기** (unclaimed, oldest first), **내 상담** (claimed by me), **완료** — and a detail view holding the full transcript, the entry context decoded from the deep link (which page they came from), the entry language, the canned answers for one-click insertion, and the reply box.

**Accepting is claiming.** `POST .../assign` is the accept: it stamps the inquiry with the manager's auth user id so two people do not answer the same shopper. The semantics that matter:

- A claim is **visible, not exclusive.** Anyone with `support.reply` can take over a claimed inquiry; the takeover is recorded rather than blocked. A hard lock strands inquiries when someone's shift ends mid-conversation.
- **Replying auto-claims.** A manager who opens an unclaimed inquiry and just answers should not have to press claim first — the claim is a side effect of the reply.
- Claiming sends the shopper nothing. They are not told a name or that someone picked it up; the next thing they see is an actual answer.

**Replying.** `POST .../reply` writes the message and sends it through the bot to the shopper's chat, storing it in `support_messages` with `author` set to the auth user id — so the transcript records which manager said what, which Telegram alone could never tell you. Delivery is not assumed: if the shopper has blocked the bot, Telegram answers `403` and the inbox shows the reply as **미전송** against that inquiry. A reply that silently never arrived is the worst outcome here, worse than an error.

**Closing** is explicit (`POST .../close`), with an auto-close after 7 days of silence so the queue reflects live work rather than history.

#### Replying from Telegram itself: not at launch

Evening hours run to 22:00, so staff will not always be at a desk, and "just reply in the ops chat" is the obvious wish. It is deliberately not in scope, because the convenient version is unsafe:

- Telegram group membership would become the authorization check. Anyone in the ops chat could speak **as the brand**, with no `support.reply` permission involved.
- Internal chatter in an alert thread would be relayed to a customer by accident. That failure is unrecoverable — the message is already on their phone.
- The audit trail would record a chat id, not an auth user.

If it is wanted later it needs an explicit relay command rather than bare replies, a Telegram-user-id → auth-user mapping, and its own permission — which is the same identity-binding machinery [Tier 3](#deferred-authenticated-lookups) needs, just pointed at staff instead of shoppers. Worth building once, for both, rather than twice.

Until then the mobile path is the deep link: manage-web is already published at `manage.dupli1.com` and works in a phone browser, so the doorbell leads to the workbench in two taps.

Gateway: one `location /api/v1/support/ { set $upstream http://dupli1-support:8080; }` block in `api/nginx.conf`, alongside the existing `/api/v1/notification/` block. The `resolver` directive stays `127.0.0.11` only.

New permissions for [permissions.md](permissions.md), plus a `support_agent` bundle granting `support.read` + `support.reply`. `support.manage` (editing canned answers) stays with `admin.*`.

---

## Security and privacy

| Concern | Measure |
|---|---|
| Bot token | Secrets Manager `dupli1/production/telegram-support` — **separate secret from the ops bot**. Never in env files or CI secrets |
| Token in logs | Reuse the existing `redactedError` wrapper — every Bot API URL carries the token in its path |
| Webhook authenticity | Own `TELEGRAM_SUPPORT_WEBHOOK_SECRET`, constant-time compare of `X-Telegram-Bot-Api-Secret-Token`, mirroring `notification/pkg/handler/http.go:187` |
| Abuse / flood | Per-chat rate limit and a daily message cap per chat; a chat over the cap is answered once with "잠시 후 다시 시도해 주세요" and then ignored until the window resets |
| Customer PII | Shoppers will paste names, phone numbers and addresses into chat. Message bodies are business records: store them, but keep them out of logs entirely. **Retention: 180 days**, after which a scheduled job purges `support_messages.body` and keeps the inquiry metadata (topic, timings, who handled it) so the volume history survives the purge. The job ships in Phase 7 and is written — a retention policy with no job that enforces it is not a policy. It replaces the body with a placeholder rather than deleting the row, so the transcript keeps its shape (who spoke, when) while the words go |
| Identity | A Telegram user ID is **not** a Dupli1 identity. No order, payment, or account data is returned to a chat under this spec. This is the hard boundary between Tier 2 and Tier 3 |
| Channel/group abuse | Ignore updates from `channel` chat types and from groups; this bot serves private chats only |

---

## Deferred: authenticated lookups

"어디쯤 왔나요?" answered with real order data requires binding a Telegram user to a Dupli1 customer. The intended shape, for when it is scheduled:

1. A signed-in shopper opens the storefront, which mints a single-use, short-TTL token bound to their `sub`.
2. That token is the `?start=` payload; the bot redeems it once and stores the binding.
3. Order reads still go through the gateway under that customer's own authority.

This runs straight into the ABAC rule that the JWT `sub` must match the resource owner — a Telegram user ID cannot stand in for a `sub`. Until the binding above exists and is auditable, no order data goes down this channel.

---

## Rollout

| Phase | Work | Done when |
|---|---|---|
| **0** ✅ | Extract the Bot API client to `shared/pkg/telegram`. `notification` imports it | **Done.** Both suites green, all 33 telegram test functions preserved; `notification` keeps only ops-specific code |
| **1** ✅ | Extend `shared/pkg/telegram`: typed send payload with `reply_markup`, `CallbackQuery`, `answerCallbackQuery`, `editMessageText` | **Done.** 16 tests against a fake API server; `notification` needed zero source changes |
| **2** ✅ | `support` service skeleton: module, health, settings, webhook endpoint, in-memory repos, compose entry (DB `5440`, service `8089`), nginx route, CI job | **Done.** Verified against a mock Bot API: `/start` returns the five-button root menu, a tap is acknowledged, a group chat is ignored. A throwaway `@BotFather` bot was not needed — `TELEGRAM_SUPPORT_API_BASE` points the bot at a mock instead |
| **3** ✅ | Menu router, conversation state, Postgres repos, canned answers + seed | **Done.** Every node renders when tapped (asserted over `domain.Nodes`, so a new node cannot be added without copy); stale and malformed callbacks reopen the root; repo tests run against a real Postgres 16 |
| **4** ✅ | Handoff: `support.inquiry_opened`, `alert_support` flag, `notification` subscriber. Business-hours window and after-hours copy | **Done.** Verified live across both services on real Postgres and NATS: an escalation reaches the opted-in ops chat with the shopper's own words quoted, and an after-hours one states the window and never a day. Postgres caught a foreign-key ordering bug the in-memory store could not |
| **5** ✅ | Manager inbox API + manage-web `/support` tab (대기 / 내 상담 / 완료, claim, reply, close) + permissions | **Done.** Verified live: a manager replied from the console and the shopper received it; against a Bot API returning 403 the reply was stored with `delivery: failed` and came back `delivered: false`, which the tab renders as 미전송. Inquiry JSON carries no chat id |
| **6** ✅ | Storefront deep-link payload; point the button at the bot | **Done.** The button opens `@dupli1_support_bot` with `?start=<surface>_<ref>_<lang>`; verified in a browser and through the live bot, where `c_b-louis-vuitton_ko` landed in `entry_payload` and surfaced in the inbox as the inquiry's entry context |
| **7** ◐ | Production: separate secret, webhook registration, ECS task, retention job | **Prepared, not applied.** ECR repo, log group, Cloud Map entry, task definition and service are written; `dupli1-support` is in both CI matrices; the retention purge runs on a 24h sweep and was verified against a real Postgres. What is left is not code — see [Deploying](#deploying-phase-7) |

Phases 0–1 are prerequisites with no user-visible change and can land first, independently, and need no Telegram account at all. Phase 6 is the only change to `dupli1-web`, and the first phase that needs the real bot handle.

---

## Deploying (Phase 7)

Everything that is code is in the tree. What remains needs an AWS console and the
bot token, so it is an operator's work, not a commit.

### What the repo already carries

| Piece | Where | Note |
|---|---|---|
| ECR repo `dupli1-support` | `infra/terraform/ecr.tf` | Terraform creates it; CI cannot push before the first apply |
| Image tag wiring | `infra/terraform/data.tf` | `service_images.support` |
| Log group | `infra/terraform/logs.tf` | Same retention as every other service |
| Cloud Map `support.dupli1.local` | `infra/terraform/ecs_services.tf` | Registered with the namespace the gateway resolves |
| Task definition + service | `infra/terraform/ecs_services.tf` | 256 CPU / 512 MB, depends on auth and nats |
| Gateway route `/api/v1/support/` | `api/nginx.ecs.conf`, `api/nginx.ecs.conf.template`, `api/nginx.prod.conf` | Telegram posts the webhook through the ALB, so a missing block here is a dead bot, not just a dead admin tab |
| Build + deploy rows | `.github/workflows/aws.yml` | `dupli1-support` in both matrices |
| Retention purge | `support/pkg/service/inbox.go`, wired in `bootstrap` | Sweeps at start, then every 24h |

### What only an operator can do

1. **Create the bot** with `@BotFather` and keep the token out of chat, tickets and this repo — it carries full send rights as the bot.
2. **Create `dupli1/production/telegram-support`** in Secrets Manager with two keys: `TELEGRAM_SUPPORT_BOT_TOKEN` and `TELEGRAM_SUPPORT_WEBHOOK_SECRET` (any long random string; Telegram echoes it back on every update and the handler rejects a mismatch with `403`).
3. **Create `dupli1/production/support-db-url`** pointing at a `support` database on the existing RDS instance (`bash infra/scripts/create-rds-databases.sh` makes the database).
4. **Set both ARNs** as `telegram_support_secret_arn` and `support_db_url_secret_arn` in tfvars, then `terraform apply`.
5. **Opt a chat into `alert_support`** from the manage-web `/telegram` tab, or no escalation reaches anyone.

### What happens if a step is skipped

Each omission degrades rather than crashes, which is the trap — the service comes
up green either way:

- **No DB secret:** conversations, inquiries and transcripts live in memory. Every open consultation is lost on the next deploy and the manager inbox comes back empty.
- **No Telegram secret:** the webhook URL is not injected either (the task definition ties them together deliberately), so the bot falls back to `getUpdates` polling. That works, but it is not how production should run, and with no token it does not run at all.
- **No `alert_support` chat:** inquiries open and sit in the inbox, and nobody is told. There is no env fallback by design — an ops alert going to a chat that never asked for customer messages is the thing that fallback would cause.

---

## Testing

- **Router and state machine:** table-driven unit tests over (node, input) → (next node, outbound message). No network.
- **Bot API adapter:** fake API server, following `shared/pkg/telegram/client_test.go` and `NewTestClient`.
- **Webhook handler:** secret mismatch → `403`; missing secret config → `503`; malformed update → `200` with no side effect (Telegram retries anything else).
- **Handoff:** publish assertion on the NATS subject, plus a `notification` subscriber test that an `alert_support` chat receives it and a non-opted chat does not.
- **Business hours:** table-driven over a fixed clock — inside the window, outside it, a weekend, and the 21:55 near-closing edge. Assert the after-hours copy names no specific day. Inject the clock; never call `time.Now()` in the hours logic, or the suite goes red at 22:00 KST.
- **Compose smoke:** a scripted `/start` → menu tap → handoff → manager reply round trip, in the style of `scripts/smoke-money-path.sh`. `TELEGRAM_SUPPORT_API_BASE` points the bot at a mock Bot API so the script never messages a real person; the service logs a warning whenever that base is set, because in production it would mean the bot is not talking to Telegram.

---

## Decisions taken

All open questions are settled; nothing blocks Phase 0.

| Question | Decision | Consequence |
|---|---|---|
| **Service hours** | Weekdays 10:00–22:00 KST; closed weekends and public holidays | [Business hours](#business-hours) — config-driven window; holidays are staff behavior, not code ([no calendar](#holidays-are-not-tracked)) |
| **Bot account** | Created later; not a prerequisite | Phases 0–5 need no real bot (see below). The handle binds at Phase 6, and it cannot be `@Dupli1212` — [bot usernames must end in `bot`](#the-bot-cannot-have-the-handle-dupli1212) |
| **Service vs adapter** | Separate `support` service | [Is separating worth it?](#is-separating-worth-it) — measured: ~420 lines, no new RDS/ALB/NAT/instance |
| **Retention** | 180 days for message bodies | Purge job in Phase 7; inquiry metadata kept |
| **Language** | Korean only at launch | [Language policy](#language-policy) — `language` in the answer key now, one static notice for `en`/`zh` arrivals |

### Creating the bot later is fine

Nothing before Phase 6 depends on the production account:

- **Phases 0–1** touch only `shared/pkg/telegram` and run against a fake API server, exactly as `NewTestClient` already allows in `notification`'s tests. No token of any kind.
- **Phases 2–5** need *a* token to exercise a real chat, but not *the* token. A throwaway bot from `@BotFather` costs a minute, stays in a developer's own Telegram, and is configured by env var — no code knows its name.
- **Phase 6** is the first step that binds the handle, because that is when the storefront button re-points.
- **Phase 7** needs the production token in Secrets Manager.

Until Phase 6 the storefront keeps pointing at `@Dupli1212` and behaves exactly as it does today, so this sequencing costs nothing.

#### The bot cannot have the handle `@Dupli1212`

**Telegram requires every bot username to end in `bot`.** The ops bot shows it already: `@MHYM7_BOT`. So `@Dupli1212` is not a name a bot can hold, freed from the human account or not — `@BotFather` refuses it.

There is therefore nothing to decide and nothing to transfer. `@Dupli1212` stays the human account, the bot takes a name like `@Dupli1212_bot`, and the storefront button re-points to it at Phase 6. No release, no gap, no window where the button leads nowhere.

*(An earlier revision of this doc described freeing `@Dupli1212` and re-registering it through `@BotFather`. That path does not exist, and the sequencing above replaces it.)*

What Phase 6 needs is only the **username**. The token is a separate thing, issued with the bot and never in this repo: it carries full send rights as the bot, so it goes straight to Secrets Manager (`dupli1/production/telegram-support`) as `TELEGRAM_SUPPORT_BOT_TOKEN`, and the client redacts it from every error it logs.

## Docs to update when this ships

Per [AGENTS.md](../AGENTS.md) and [docs/README.md](README.md): [current-state.md](current-state.md), [api.md](api.md), [endpoints.md](endpoints.md), [openapi.yaml](openapi.yaml), [permissions.md](permissions.md), [service-layout.md](service-layout.md), and the root `CLAUDE.md` service-ownership and dev-credentials tables.
