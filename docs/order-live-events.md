# Live order events (SSE)

**Status:** Implemented.
**Related:** [../order](../order), `shared/pkg/events`, manage-web `app/lib/order-events.ts`.

## Purpose

The admin dashboard used to fetch orders once per page load, so a new order was
invisible until someone reloaded. The Telegram ops bot already reacted to order
events; this puts the same information in the admin UI.

`GET /api/v1/orders/events` streams order changes as Server-Sent Events.

## Why SSE and not a WebSocket

| | SSE | WebSocket |
|---|---|---|
| Reaches the service through manage-web's BFF | Yes — a plain GET | No; the BFF proxies with `fetch`, which cannot perform an HTTP upgrade |
| Keeps the access token server-side | Yes, the session cookie authenticates | Browser `WebSocket` cannot send `Authorization`, so a token would have to reach the browser |
| Direction needed | server → client | bidirectional (unused; ship/cancel are REST) |
| Reconnect and resume | Built in, via `Last-Event-ID` | Hand-rolled |

Admin actions stay on REST. Nothing flows client → server over the stream.

## Contract

`GET /api/v1/orders/events` — requires `order.read.all`, the same gate as
`GET /api/v1/orders` with no `customer_id`. There is no per-customer stream.

```
retry: 3000

id: 1789272306123456789
event: order
data: {"type":"order.created","order":{ …the GET /orders/{id} representation… }}

: ping

event: reset
data: {"reason":"gap"}
```

| Frame | Meaning |
|---|---|
| `event: order` | One order changed. `type` is the subject that fired (`order.created`, `order.paid`, `order.status_updated`); `order` is the full snapshot, so a client needs no follow-up request. |
| `event: reset` | The server cannot say what the client missed. Reload from `GET /api/v1/orders`. |
| `: ping` | Heartbeat every 20s, so idle proxies (the ALB idles at 60s) see traffic. |

Event ids are the event's `occurred_at` in unix nanoseconds. Every order task
computes the same id for the same event, which is what lets a browser reconnect
to a *different* task and still resume from its `Last-Event-ID`.

A connection is closed after 15 minutes. The access token is only validated when
the stream opens, so expiring the stream is what forces the caller's token and
permissions to be checked again; `EventSource` reconnects by itself and an
unchanged cursor means nothing is resent.

## How events reach a stream

```
order task writes ──> outbox ──> NATS order.created / order.paid / order.status_updated
                                      │
                        ┌─────────────┴─────────────┐
                        ▼                           ▼
                   order task A                order task B      (no queue group:
                   stream.Hub                  stream.Hub         every task sees
                        │                           │             every event)
                   SSE clients                 SSE clients
```

The service subscribes to the subjects it publishes rather than fanning out
inline at write time: with more than one task, only the task that handled the
write would otherwise see the change, and clients on the other tasks would miss
it. Each task enriches the event with a repo read so the frame carries the same
representation as `GET /orders/{id}`; if that read fails the task sends `reset`
instead of a snapshot it knows is stale.

Each hub keeps the last 256 events. A client whose cursor is still inside that
window is replayed event by event; one that fell outside it — or that reconnects
to a task which has restarted and has no history — gets `reset`. A client too
slow to drain its queue is dropped with a `reset` rather than being allowed to
stall fan-out for everyone else.

Without `NATS_URL` the hub is not created and the endpoint reports
`503 live order events not configured`: no order change would ever reach that
task, and a silently empty stream is worse than a refusal.

## Proxies

The response sets `Cache-Control: no-cache, no-transform` and
`X-Accel-Buffering: no`, and the gateway has an explicit
`location = /api/v1/orders/events` with `proxy_buffering off`,
`proxy_http_version 1.1` and `proxy_read_timeout 1h`.

Measured through the real chain (nginx → manage-web's `react-router-serve` →
browser), frames arrive as they are produced both with and without those
opt-outs: nginx forwards a slow trickle promptly even with buffering on, and
`@react-router/node` calls `res.flush()` after every chunk, which defeats the
express `compression` middleware's buffering. They are kept as cheap insurance
against config drift (a gzip layer added later would buffer rather than fail
loudly) and to avoid gzipping 200-byte frames.
