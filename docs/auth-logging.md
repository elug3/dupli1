# Auth service logging

**Status:** Implemented in `auth/` (zerolog). This document describes **what the code does today**, not the future shared `shared/pkg/log` contract in [v1.1-release-plan.md](v1.1-release-plan.md).

Auth is the reference for structured logging among Dupli1 services: handlers attach a stable `event` string, use levels by outcome class, and never put secrets in log fields or SQL text in HTTP 500 bodies.

---

## Rules (decision guide)

Log when you would later grep CloudWatch / container logs for **incidents, abuse, or deploy risk**. Otherwise stay silent.

| Do log | Do not log |
|--------|------------|
| Unexpected / internal failures (5xx path) | Ordinary client 4xx outside login/register/refresh |
| Login, register, logout, refresh outcomes (success + auth failure) | Happy-path reads (`/me`, list users, profile/address OK) |
| Soft-fail after success (e.g. NATS `user.registered` publish) | Auth middleware denials (missing/invalid Bearer, insufficient permission) |
| Process lifecycle and risky config | Rate-limit `429` responses |
| Bootstrap seed / service-account sync | Admin user CRUD success; profile/address validation 4xx |

**Levels**

| Level | When |
|-------|------|
| **Error** | Unexpected failure; best-effort side effect failed after durable success |
| **Warn** | Expected but notable (bad credentials, locked/deactivated, conflict, rejected refresh, risky config) |
| **Info** | Successful session events (login/register/logout/refresh) and bootstrap milestones |

**Safety**

- Never log passwords, refresh tokens, or access tokens.
- Never return SQL/driver text in JSON error bodies on 500 — log the real `err` server-side; client gets `"internal error"`.
- Prefer identifiers (`user_id`, `email`, `ip`) over full profile PII. Do not log full phone/address ([auth-profile-extension-plan.md](auth-profile-extension-plan.md)).

**Shape**

```go
h.logger.Warn().
    Str("event", "login_failed").
    Str("email", req.Email).
    Str("ip", ip).
    Msg("login failed: invalid credentials")
```

- Library: `github.com/rs/zerolog`
- Config: `LogOutput` (`json` default, or `text` / console) and `LogLevel` (`debug` / `info` / `warn` / `error`; default `info`) via auth options / env
- Request logs use `.Str("event", …)`; include `Err(err)` on failures when useful

---

## Where logging lives

| Area | Package / file |
|------|----------------|
| HTTP handlers | `auth/pkg/handler/handler.go`, `profile.go` |
| Soft-fail NATS | `auth/pkg/service/service.go` (`Register`) |
| Server lifecycle | `auth/pkg/server.go` |
| Bootstrap / DB / seed | `auth/pkg/bootstrap/` |
| Logger factory | `auth/pkg/server.go` (`newLogger`) |

Auth middleware (`RequireAuth`, `OptionalAuth`, `RequirePermission`) and the Redis IP rate limiter **do not log**.

---

## Named `event` catalog

### Login — `POST /api/v1/auth/login`

| `event` | Level | When | Typical fields |
|---------|-------|------|----------------|
| `login_bad_request` | Warn | Invalid JSON / binding | `ip`, `error` |
| `login_failed` | Warn | Invalid credentials | `email`, `ip`, `user_agent` |
| `login_locked` | Warn | Account locked | `email`, `ip` |
| `login_deactivated` | Warn | Account deactivated | `email`, `ip` |
| `login_error` | Error | Unexpected failure | `email`, `ip`, `err` |
| `login_success` | Info | Credentials OK; refresh token issued | `email`, `ip`, `user_agent` |

### Register — `POST /api/v1/auth/register`

| `event` | Level | When | Typical fields |
|---------|-------|------|----------------|
| `register_bad_request` | Warn | Invalid JSON / binding | `ip`, `error` |
| `register_conflict` | Warn | Email already exists | `email`, `ip` |
| `register_error` | Error | Unexpected failure after validation | `email`, `ip`, `err` |
| `register_has_owner_error` | Error | `HasOwner` lookup failed | via `respondInternalError` |
| `register_success` | Info | User created | `user_id`, `email`, `ip`, `user_agent` |
| `user_registered_publish_failed` | Error | Account saved but NATS `user.registered` publish failed (soft-fail in service layer) | `user_id`, `err` |

**Silent on register:** missing Bearer when open register is off (`401`); management forbidden / owner already exists (`403`); weak password / invalid email / invalid account type (`422`).

### Logout — `POST /api/v1/auth/logout`

| `event` | Level | When | Typical fields |
|---------|-------|------|----------------|
| `logout` | Info | Session revoke succeeded | `ip` |
| `logout_error` | Error | Unexpected failure | `ip`, `err` |

**Silent:** bad request body (`400`).

### Refresh — `POST /api/v1/auth/refresh`

| `event` | Level | When | Typical fields |
|---------|-------|------|----------------|
| `refresh_bad_request` | Warn | Invalid JSON / binding | `ip`, `error` |
| `refresh_rejected` | Warn | Invalid, expired, or deactivated | `ip`, `err` |
| `refresh_error` | Error | Unexpected failure (HTTP still `401`) | `ip`, `err` |
| `refresh_success` | Info | Access token issued | `ip` |

### User admin — `/api/v1/auth/users…`

Only unexpected failures are logged (via `respondInternalError`):

| `event` | When |
|---------|------|
| `list_users_error` | List users failed |
| `set_permissions_lookup_error` | Target user lookup failed (non–not-found) |
| `set_permissions_has_owner_error` | Owner check failed |
| `set_permissions_error` | Persist permissions failed |
| `update_password_lookup_error` | Target user lookup failed (non–not-found) |
| `update_password_error` | Password update failed |
| `set_status_lookup_error` | Target user lookup failed (non–not-found) |
| `set_status_error` | Status update failed |

**Silent:** validation / not found / forbidden; successful admin updates.

### Profile & addresses — `/api/v1/auth/me/profile`, `/me/addresses`

Known domain errors (`ErrInvalidProfile`, `ErrInvalidAddress`, `ErrAddressNotFound`, `ErrAddressLimitReached`) return 4xx **without** a log. Unexpected failures use:

| `event` | Handler |
|---------|---------|
| `get_profile_error` | `GetProfile` |
| `patch_profile_error` | `PatchProfile` |
| `list_addresses_error` | `ListAddresses` |
| `create_address_error` | `CreateAddress` |
| `get_address_error` | `GetAddress` |
| `patch_address_error` | `PatchAddress` |
| `delete_address_error` | `DeleteAddress` |
| `set_default_address_error` | `SetDefaultAddress` |

### Bootstrap / process

| `event` (or message) | Level | When |
|----------------------|-------|------|
| *(msg)* `Starting auth server...` | Info | Server start (`server.go`) |
| *(msg)* `stopping` | Info | Graceful shutdown |
| `open_register_enabled` | Warn | `AUTH_OPEN_REGISTER` allows unauthenticated customer signup |
| *(msg)* ephemeral RSA key warning | Warn | No `JWT_PRIVATE_KEY` / file — tokens die on restart |
| *(msg)* DB connecting / connected / connection timeout | Info / Error | Postgres open retry loop |
| `owner_seeded` | Info | Owner user created |
| `web_service_account_seeded` | Info | `dupli1-web` service account created |
| `web_service_account_synced` | Info | Web service account credentials/permissions updated |
| `order_service_account_seeded` | Info | `dupli1-order` service account created |

---

## Internal error helper

```go
// respondInternalError logs the real error and returns a generic 500 body.
func (h *Handler) respondInternalError(c *gin.Context, event string, err error) {
	h.logger.Error().Str("event", event).Err(err).Msg("internal error")
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
}
```

Profile handlers call `respondProfileError`, which maps known sentinels to 4xx (no log) and delegates unknowns to `respondInternalError`.

---

## Gaps vs v1.1 target

[v1.1-release-plan.md](v1.1-release-plan.md) Slice 1 aims for a shared factory (`shared/pkg/log`), root field `service`, and fields `op` / `code` / `request_id` / `route`. Auth today uses **`event`** (keep stable when aligning). Machine-readable API `code` in JSON error bodies is not yet wired on these paths.

See also: [product-error-wrapping.md](product-error-wrapping.md) (same 500 sanitization idea on product).
