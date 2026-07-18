# Product API error wrapping

Strategy for classifying store/service failures so HTTP handlers can map status
codes without leaking PostgreSQL messages to clients.

## Layers

```text
pgx / pgconn error
        │
        ▼
infra/pg.wrapDB(op, err)     ← classify at the DB boundary
        │
        ▼
ports.ErrNotFound | ErrConflict | ErrInvalid | (raw wrapped cause)
        │
        ▼
handler.respondServiceError  ← HTTP status + client-safe body
```

## Sentinel errors (`product/pkg/ports/errors.go`)

| Sentinel | Typical HTTP | Meaning |
|----------|--------------|---------|
| `ErrNotFound` | 404 | Missing product, variant, coupon, etc. |
| `ErrConflict` | 409 | Duplicate key / resource already exists |
| `ErrInvalid` | 400 | Validation / client-correctable input |
| (unclassified) | 500 | Unexpected failure (incl. raw SQL) |

Domain catalog sentinels remain in `domain` (`ErrMasterNotFound`, `ErrMasterExists`,
`ErrMasterInUse`, `ErrMissingSKUCodes`) and are also recognized by the handler.

Helpers:

- `ports.Invalid(msg)`, `ports.NotFound(msg)`, `ports.Conflict(msg)` wrap a message with `%w`.

## `wrapDB`

`product/pkg/infra/pg/db_error.go` maps:

| Driver signal | Result |
|---------------|--------|
| `pgx.ErrNoRows` | `ErrNotFound` |
| unique violation `23505` | `ErrConflict` |
| FK violation `23503` | `ErrInvalid` (unless caller maps first, e.g. delete → `ErrMasterInUse`) |
| anything else | `fmt.Errorf("%s: %w", op, err)` — **not** a client sentinel |

Call sites that need a domain-specific sentinel (master in-use, master missing on
FK insert) check and return **before** `wrapDB`.

## HTTP responses

`handler.respondServiceError`:

- Known sentinels → status + `err.Error()` (safe, no SQL).
- Default → **500** with body `"internal error"`, and the real error is logged
  (`log.Printf("product: internal error: %v", err)`).

Do not return `err.Error()` on 500s from product handlers.

## Adding a new store method

1. On validation failure: `return ports.Invalid("…")`.
2. On missing row: `return wrapDB("op", err)` (or `fmt.Errorf("…: %w", ports.ErrNotFound)`).
3. On unique conflict: rely on `wrapDB` or return `ports.Conflict("…")`.
4. In the handler: call `h.respondServiceError(w, err)` — never `err.Error()` for 500.
