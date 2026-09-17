# Order live events (admin SSE)

**Status:** Client + mock gateway implemented (`dupli1-manage-web`); **backend route not yet registered** in `dupli1-order` as of 2026-09-14. Production admin reconnects will 404 until the order service ships the hub.

## Goal

Give operators a **single long-lived stream** of order changes so the manage dashboard can update lists, tiles, and toast notifications without polling. The stream must survive React Router navigation and work through the manage-web **session gateway** (httpOnly cookie — no Bearer token in the browser).

## Why SSE, not WebSocket

| Constraint | Implication |
|------------|-------------|
| BFF proxies with `fetch` | No HTTP upgrade to WebSocket |
| Browser `WebSocket` API | Cannot send `Authorization: Bearer …` |
| Session gateway | Attaches access token server-side from Redis/`dupli1_sid` cookie |

Server-Sent Events over `GET /api/v1/orders/events` reuse the same gateway path as REST calls (`/auth/session/gateway/order/api/v1/orders/events` in manage-web).

## Planned endpoint

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

When the client reconnects with `Last-Event-ID`, the server should either:

1. Replay missed events after that id, or
2. Emit `event: reset` if history is unavailable (task restart, cold start, cursor too old).

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

## Backend implementation checklist (open)

When adding the hub to `dupli1-order`:

1. Register `GET /api/v1/orders/events` in `order/pkg/handler` **before** the `/api/v1/orders/` catch-all.
2. Require `order.read.all` (same as list-all).
3. Subscribe to outbox drain or in-process bus after successful commits — publish `{ type, order }` JSON matching `GET /orders/{id}`.
4. Support `Last-Event-ID` with bounded in-memory buffer or “reset on gap” for v1.
5. Set `X-Accel-Buffering: no` and disable response buffering on nginx for this location.
6. Document in [endpoints.md](endpoints.md), [openapi.yaml](openapi.yaml), and [permissions.md](permissions.md).

## Related docs

- [order-service.md](order-service.md) — order outbox and NATS subjects
- [dupli1-manage-web CLAUDE.md](../../dupli1-manage-web/CLAUDE.md) — admin BFF session gateway
