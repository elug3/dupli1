# Order live events (admin SSE)

**Status:** implemented (2026-09-27) — `order/pkg/livefeed` (hub) and `order/pkg/handler/events.go` (endpoint); the client (`dupli1-manage-web`, `app/lib/order-events.tsx`) and its mock gateway came first. Until this shipped, production streams 404'd and manage-web fell back to re-reading orders every 30s.

## Goal

Give operators a **single long-lived stream** of order changes so the manage dashboard can update lists, tiles, and toast notifications without polling. The stream must survive React Router navigation and work through the manage-web **session gateway** (httpOnly cookie — no Bearer token in the browser).

## Why SSE, not WebSocket

| Constraint | Implication |
|------------|-------------|
| BFF proxies with `fetch` | No HTTP upgrade to WebSocket |
| Browser `WebSocket` API | Cannot send `Authorization: Bearer …` |
| Session gateway | Attaches access token server-side from Redis/`dupli1_sid` cookie |

Server-Sent Events over `GET /api/v1/orders/events` reuse the same gateway path as REST calls (`/auth/session/gateway/order/api/v1/orders/events` in manage-web).

## Endpoint

```
GET /api/v1/orders/events
Authorization: Bearer <access_token>   # attached by BFF, not browser
Accept: text/event-stream
Last-Event-ID: <opaque>                # optional resume cursor
```

| Response | Meaning |
|----------|---------|
| `200` `text/event-stream` | Stream open |
| `401` | Missing/invalid token |
| `403` | Caller lacks `order.read.all` |
| `503` | Stream not configured in this process |

The stream **ends when the access token that opened it expires** (or after 15 minutes if it carries no `exp`): the token is checked only when the stream opens, so this bounds how long a revoked permission keeps receiving orders to one access-token lifetime, like every other API. The browser reconnects by itself with `Last-Event-ID`; manage-web's BFF attaches a fresh token (it refreshes 30s before `exp`), so the operator sees nothing but a replay.

### SSE frames

Initial line (server → client):

```text
retry: 3000
```

Heartbeat (keep proxies from timing out):

```text
: ping
```

Order change (authoritative snapshot — **no follow-up GET required**):

```text
id: 1726339200123456789
event: order
data: {"type":"order.paid","order":{ ... full order JSON ... }}
```

Gap / restart (client must reload list from REST):

```text
event: reset
data: {"reason":"gap"}
```

| `data.type` (inside JSON) | When |
|---------------------------|------|
| `order.created` | New order persisted |
| `order.paid` | Status became `paid` |
| `order.status_updated` | Any other status change |

The outer SSE `event` name is always `order` for snapshots; `type` inside the JSON distinguishes subjects (matches `shared/pkg/events` order subjects).

### Resume semantics

When the client reconnects with `Last-Event-ID`, the server either:

1. Replays the frames after that id, from a ring of the last 512 held in memory, or
2. Emits `event: reset` when it cannot vouch for the cursor — older than the ring, or issued before this process started (ids begin at the process start time, so an earlier process's cursor is always recognised as foreign).

A fresh connection (no `Last-Event-ID`) gets neither: the client has just loaded the list over REST.

Manage-web treats `reset` as “call `GET /api/v1/orders` again” — see `useOrderEvents` in `dupli1-manage-web/app/lib/order-events.tsx`.

## Manage-web integration

- **`OrderFeedProvider`** (`app/lib/order-events.tsx`) opens **one** stream for the whole signed-in admin shell (survives route changes).
- Pages join with `useOrderFeed(listener, enabled)`; they merge snapshots locally and gate on `enabled` until the first REST list load completes (avoids a lone streamed row).
- Only the provider toasts on `order.created` / `order.paid`; pages must not duplicate notifications.
- Permanent stream closure (403, 404, deploy without endpoint) triggers reconnect with backoff; auth refresh is **not** guessed from EventSource (see comments in `useOrderEvents`).

Browser tests: `npm run test:orders:browser` against `npm run mock:gateway`.

## Mock gateway (tests only)

`dupli1-manage-web/scripts/mock-gateway.mjs` implements the contract for local browser tests:

- `GET /api/v1/orders/events` — fan-out hub, 20s heartbeat
- `POST /__control/order` — inject `{ type, order }` and broadcast
- `POST /__control/reset` — broadcast `reset`
- `POST /__control/drop` — drop all streams (simulates task restart)

The mock keeps **no event history**; presenting `Last-Event-ID` always yields `reset` — the same behavior expected after an ECS task replacement.

## Backend implementation

**Source: the outbox.** Every order state change — manual, customer, or an SLA sweep (auto-confirm, auto-approve, auto-fulfill, payment expiry) — writes an `order.*` outbox row in the same transaction. The stream relays those, so it cannot miss a change the rest of the platform hears about. `order.created`, `order.paid` and `order.status_updated` are relayed; a payment emits both `order.paid` and `order.status_updated`.

**Fan-out: `livefeed.Hub`.** For each event it loads the order through `Service.GetOrder` — the presented order, with `confirmation_due_at` and the other SLA fields the console's badge needs — and sends one frame to every open stream. A stream that falls 64 frames behind is dropped rather than allowed to stall the others; its reconnect replays from the ring.

**Feeding the hub** (`order/pkg/bootstrap`):

- **With NATS** (production): every replica makes a *broadcast* subscription to `order.*` (`Subscriber.SubscribeAll`, no queue group — unlike the payment consumers, where one replica must do the work once). A console's stream lives on whichever replica it reached, and each replica relays every change, including those another replica committed.
- **Without NATS** (local dev, tests): `livefeed.Publisher` wraps the outbox drainer's publisher and feeds the hub as rows drain.

**HTTP** (`handler/events.go`): the server-wide `WriteTimeout` (25s) would cut every stream, so the handler lifts it for this response and sets a 10s deadline per write instead. `X-Accel-Buffering: no`, a `: ping` every 20s (under nginx's 60s read timeout), and the exact route `/api/v1/orders/events` registered ahead of the `/api/v1/orders/` subtree, which would otherwise read `events` as an order id.

**Gateway** (`api/gateway/routes.conf`): `location = /api/v1/orders/events` with `proxy_buffering off`. It sets no `proxy_set_header` — one there would drop every header inherited from the server block.

**Tests:** `order/pkg/livefeed` (replay, reset, slow stream dropped), `order/pkg/handler/events_test.go` (a real server with a 1s `WriteTimeout`: snapshots per change, replay/reset, 403, ending at token expiry), `order/pkg/infra/nats` (a broadcast subscription reaches both of two replicas; the queue group reaches one), and `api/gateway/test.sh` for the route.

## Related docs

- [order-service.md](order-service.md) — order outbox and NATS subjects
- [dupli1-manage-web CLAUDE.md](../../dupli1-manage-web/CLAUDE.md) — admin BFF session gateway
