# Service account API keys

**Status:** design (not implemented). Supersedes password-based machine login for
`account_type: service` accounts.

Today every machine caller — the `dupli1-web` BFF, the `dupli1-order` service —
authenticates with an **email and password**, exactly like a human operator
(`auth/pkg/bootstrap/seed_service_account.go`,
`order/pkg/infra/httpauth/client.go`). That works for two accounts whose
passwords live in Secrets Manager, but it does not extend to operator-minted
automation: a password is an all-or-nothing account credential, it cannot be
scoped below the account's permissions, it cannot expire, and revoking one
means rotating the shared secret and restarting auth.

This document designs API keys as a first-class credential for service accounts.

**Scope boundary: Dupli1 does not expose a public API.** Keys are for
first-party callers only — our own services, and automation an operator mints
for an internal AI agent, ops script, or cron job. Nothing here is a
partner-integration or developer-portal feature. See
[Explicit non-goals](#explicit-non-goals) for what that removes.

## Design at a glance

```
                        ┌─────────────────────────────────────────┐
  mint (operator)       │ auth                                    │
  manage-web ──────────▶│  POST /users/{id}/api-keys              │
                        │   → plaintext returned ONCE             │
                        │   → SHA-256 hash stored                 │
                        └─────────────────────────────────────────┘

  use (machine)
  agent ──ApiKey dk_live_…──▶ POST /api/v1/auth/token
        ◀──── access JWT (15 min, RS256, permissions claim) ──────

  agent ──Bearer <JWT>──▶ gateway ──▶ order / product / payment / cart
                                      (unchanged — JWKS validation as today)
```

The key buys a normal access token and nothing more. Because the minted JWT is
byte-shape-identical to one from `POST /refresh`, **no service outside auth
changes at all** — no new middleware, no new auth dependency, no gateway trust
boundary. That is the central reason for choosing exchange over direct key
validation.

## Data model

New table in the auth DB, created by `migrateSchema`
(`auth/pkg/bootstrap/migrate.go`) alongside `users` and `auth_outbox`:

```sql
CREATE TABLE IF NOT EXISTS service_api_keys (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    prefix       TEXT NOT NULL,
    key_hash     TEXT NOT NULL,
    permissions  TEXT[]      NOT NULL DEFAULT '{}',
    source       TEXT        NOT NULL DEFAULT 'api',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by   TEXT        NOT NULL DEFAULT '',
    expires_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_service_api_keys_hash
    ON service_api_keys (key_hash);
CREATE INDEX IF NOT EXISTS idx_service_api_keys_user
    ON service_api_keys (user_id) WHERE revoked_at IS NULL;
```

| Column | Role |
|---|---|
| `prefix` | First 12 chars of the plaintext (`dk_live_A1b2`). Display and log identification only — never used for authentication |
| `key_hash` | SHA-256 hex of the full plaintext. The unique index makes verification one indexed point lookup |
| `permissions` | Key scope. Empty means "inherit the account's permissions" |
| `source` | `api` (minted through the UI) or `env` (seeded from a bootstrap env var). Env rows are re-synced on every auth boot and are read-only in the UI |
| `created_by` | Operator user id, for audit. Empty for `source='env'` |

`ON DELETE CASCADE` means `DELETE /users/{id}` already revokes that account's
keys — no extra work in `DeleteUser`.

### Why SHA-256 and not bcrypt

The `users.password` column is bcrypt, and copying that here would be the
obvious move. It is the wrong one, for three reasons:

1. **There is nothing to slow down.** bcrypt's cost factor defends low-entropy
   human-chosen passwords against offline cracking. An API key is 32 bytes from
   `crypto/rand` — 256 bits. No cost factor is relevant to a search space that
   size.
2. **bcrypt cannot be looked up.** Every bcrypt hash carries its own random
   salt, so you cannot index it and match. Verification would mean scanning
   every key row and running a deliberately-slow comparison against each —
   O(n) slow hashes per request, which gets worse as the table grows.
3. **bcrypt silently truncates at 72 bytes**, a footgun for a format we might
   later lengthen.

SHA-256 over a high-entropy secret is the standard construction for exactly
this case. The stored hash is not attackable by dictionary or rainbow table
because the input is uniform random, so no salt is needed either.

### Key format

```
dk_live_<43 chars base64url>      production
dk_test_<43 chars base64url>      local / dev
└──┬───┘└────────┬─────────┘
prefix       32 random bytes, base64url, unpadded
```

The `dk_` prefix and environment marker make a leaked key greppable in logs and
recognizable by secret scanners. Generation, with no modulo bias:

```go
buf := make([]byte, 32)
if _, err := rand.Read(buf); err != nil { return "", err }
plaintext := envPrefix + base64.RawURLEncoding.EncodeToString(buf)
sum := sha256.Sum256([]byte(plaintext))
hash := hex.EncodeToString(sum[:])
```

## Scope: intersection, recomputed every exchange

A key's effective permissions are **the intersection of the key's scope and the
account's current permissions**, computed fresh at every exchange — never
frozen into the row.

```go
// effectivePermissions returns what the minted JWT will carry.
func effectivePermissions(key *APIKey, u *domain.User) []string {
    if len(key.Permissions) == 0 {
        return u.Permissions // inherit
    }
    out := make([]string, 0, len(key.Permissions))
    for _, p := range key.Permissions {
        if permissions.Has(u.Permissions, p) { // wildcard-aware
            out = append(out, p)
        }
    }
    return out
}
```

Reusing `permissions.Has` means wildcards resolve correctly for free: an
account holding `product.*` and a key scoped to `product.create` yields
`product.create`. Recomputing at exchange time means narrowing an account's
permissions in manage-web **immediately narrows every key it owns**, with no
key-by-key cleanup and no stale-grant window beyond the 15-minute token life.

At creation, each requested permission must satisfy
`permissions.Has(account.Permissions, p)` — a key can never exceed its account.
Requesting one the account lacks is a `400`, not a silent drop, so a typo in a
scope surfaces at mint time rather than as a mysterious `403` at 3am.

## Exchange endpoint

```
POST /api/v1/auth/token
Authorization: ApiKey dk_live_…
```

```json
{
  "token": "eyJhbGciOiJSUzI1NiIs…",
  "token_type": "Bearer",
  "expires_in": 900
}
```

**No refresh token is issued.** The key *is* the long-lived credential, so the
rotation dance in `Service.Refresh` — session store, `Rotate`, replay
detection — simply does not apply. That removes the single sharpest edge in the
current machine path: `ServiceAccountTokenSource` must persist a rotating
refresh token, and dropping the replacement strands the caller on a dead token
(the failure that cost manage-web its 30-day sessions,
[CLAUDE.md](../CLAUDE.md) auth notes). A key holder that loses its access token
just exchanges again.

The minted JWT is produced by the same `tokenGen.Generate` used by `/refresh`,
plus two non-secret claims for traceability:

| Claim | Value |
|---|---|
| `sub` | Service account user id (unchanged) |
| `permissions` | Effective permissions (intersection above) |
| `email` | Service account email (unchanged) |
| `token_use` | `"api_key"` |
| `akid` | API key id |

`akid` is an identifier, not a secret, so it is safe in logs — it lets an
operator trace a downstream request back to the exact key that authorized it.
Downstream services read only `sub` and `permissions`
(`shared/pkg/authjwt`), so extra claims are ignored and require no coordination.

### Checks at exchange

In order, all failing as an indistinguishable `401 invalid_api_key` so the
endpoint never reveals which condition tripped:

1. Row exists for `sha256(plaintext)`.
2. `revoked_at IS NULL`.
3. `expires_at IS NULL OR expires_at > NOW()`.
4. Account exists and `is_active`.
5. `account_type = 'service'`.

**Account lockout is deliberately not checked.** `users.locked_at` is a
failed-*password* brute-force defense, and service accounts are `ClassCustomer`
under `UserClass`, so they are not lock-exempt. If it gated key exchange, an
attacker who merely guessed a service account's email could lock it with ten
bad password attempts and take down the order service's outbound calls — a
trivial unauthenticated DoS. Key auth has no password to brute-force and is
rate-limited independently, so the lock is irrelevant here. `is_active` stays
the account-wide off switch for exchange, and revoking the key is the targeted
one.

`last_used_at` is written best-effort after a successful exchange, throttled to
at most one write per key per 60s, and never fails the request.

### Rate limiting

Reuses `redisinfra.NewIPRateLimiter` next to the existing `login` (10/60) and
`refresh` (30/60) limiters, as `token` at **60/60**. A service exchanges roughly
once per 14 minutes, so this is far above legitimate traffic while still
capping an online guessing attempt — which is already hopeless against 256 bits
of entropy, making this defense-in-depth rather than the primary control.

## Management API

Two new permissions in `shared/pkg/permissions/catalog.go`, added to `Catalog`
and to the `user_admin` bundle (`admin.*` and `*` already cover them by
wildcard):

| Permission | Grants |
|---|---|
| `user.apikey.read` | List a service account's keys (metadata only) |
| `user.apikey.manage` | Mint and revoke keys |

Routes, registered in `auth/pkg/bootstrap/router.go` in the existing
`/api/v1/auth` group, following the `/users/:id/permissions` shape:

| Method | Path | Permission |
|---|---|---|
| `GET` | `/users/:id/api-keys` | `user.apikey.read` |
| `POST` | `/users/:id/api-keys` | `user.apikey.manage` |
| `DELETE` | `/api-keys/:keyId` | `user.apikey.manage` |

Every route additionally passes ABAC: the caller must satisfy
`domain.CanManageUser(caller, target)`, so the existing hierarchy governs who
may mint for whom, and the self-management block still applies. The target must
be `account_type: service` — `400 invalid_account_type` otherwise. This is the
enforcement point for "keys attach to service accounts only"; keeping it in one
place means the rule cannot drift between the API and the UI.

**Create** — `POST /users/:id/api-keys`

```json
{ "name": "claude-ops-agent", "permissions": ["order.read.all", "product.read"], "expires_in_days": 90 }
```

`permissions` omitted or `[]` means inherit the account's. `expires_in_days`
omitted means no expiry.

`201` — the only response anywhere that contains `api_key`:

```json
{
  "id": "01JD…",
  "name": "claude-ops-agent",
  "api_key": "dk_live_xK3n…",
  "prefix": "dk_live_xK3n",
  "permissions": ["order.read.all", "product.read"],
  "expires_at": "2026-12-14T00:00:00Z",
  "created_at": "2026-09-15T00:00:00Z"
}
```

**List** — `GET /users/:id/api-keys` returns the same shape **without
`api_key`**, plus `last_used_at`, `revoked_at`, and `source`. The plaintext is
unrecoverable by construction: only its SHA-256 was stored. Lost key ⇒ mint a
new one and revoke the old.

**Revoke** — `DELETE /api-keys/:keyId` sets `revoked_at` and returns `204`.
Soft delete keeps the audit trail intact.

Revocation stops **new exchanges** immediately. An access token already minted
from that key keeps working downstream until it expires, up to 15 minutes,
and deactivating the account does not shorten that: `product`, `order`,
`cart`, `payment` and `notification` validate tokens **offline** against JWKS
(`shared/pkg/authjwt`, `shared/pkg/authmiddleware`) and never call back to
auth. Only auth's own routes run `GetMe`, which re-reads the user row.

So the honest guarantee is *revocation within one access-token lifetime*, and
that is not new — it is the same window that already applies when an operator
deactivates a human account. If an incident needs a harder stop, the levers are
rotating the auth signing key (invalidates every token platform-wide) or
blocking the caller at the gateway. Shrinking the window generally would mean
either a shorter `TokenExpiry` or a shared revocation list that downstream
services consult, both of which trade away the offline validation that keeps
those services independent of auth's availability — a deliberate property of
the current architecture and out of scope here.

Keys with `source='env'` reject `DELETE` with `409 env_managed_key`, naming the
env var to rotate instead. Without this, revoking the order service's key in
the UI would appear to work and then silently reappear on the next auth
restart.

## Bootstrap seeding

`DUPLI1_WEB_SERVICE_API_KEY` and `DUPLI1_ORDER_SERVICE_API_KEY` join the
existing `*_SERVICE_EMAIL` / `*_SERVICE_PASSWORD` pairs in
`bootstrap.Config`. When set, the seeders upsert a `source='env'`,
`permissions='{}'` (inherit), never-expiring row for that account — idempotent
and re-synced on every boot, matching how the passwords behave today. Rotation
is: update the Secrets Manager value, restart auth.

Env and API keys coexist, which is what makes the rollout below safe.

## Consumer change

`order/pkg/infra/httpauth` gains an `APIKeyTokenSource` beside
`ServiceAccountTokenSource`, satisfying the same `TokenSource` interface:

```go
type APIKeyTokenSource struct {
    authBaseURL string
    apiKey      string
    client      *http.Client
    skew        time.Duration
    now         func() time.Time

    mu          sync.Mutex
    accessToken string
    accessExpiry time.Time
}

func (s *APIKeyTokenSource) Token(ctx context.Context) (string, error)
func (s *APIKeyTokenSource) Invalidate()
```

It is strictly simpler than the password source — cache an access token,
re-exchange when it is within `skew` of expiry, no refresh token to persist and
no re-login fallback. Because it satisfies the existing interface, the
`Invalidate()`-on-401 retry path and every call site stay untouched.

Bootstrap picks the key source when `DUPLI1_ORDER_SERVICE_API_KEY` is set and
falls back to password login otherwise. The same swap applies to the
`dupli1-web` BFF.

## manage-web

The Services tab already exists at `/users`. Service account detail
(`/users/:id`) gains an API keys panel:

```
API keys                                      [+ Create key]
┌──────────────────────────────────────────────────────────┐
│ bootstrap      dk_live_xK3n…   env      used 2m ago      │
│ claude-ops     dk_live_9fQm…   90d      used 1h ago  [×] │
│ nightly-sync   dk_live_bT4w…   expired  —            [×] │
└──────────────────────────────────────────────────────────┘
```

Create opens a dialog for name, scope (checkbox list drawn from the account's
own permissions — you cannot pick what the account lacks) and optional expiry.
On success the plaintext is shown once in a copy-to-clipboard panel stating
plainly that it will not be shown again; it is never written to component state
that outlives the dialog.

All calls go through `authedFetch` → `/auth/session/gateway/auth/api/v1/auth/…`,
so no token and no key ever touches the browser's storage — consistent with the
existing session model. Types (`ServiceApiKey`, `CreateApiKeyRequest`) go in
`app/lib/api.ts` next to `PERMISSION_CATALOG`.

## Audit logging

New zerolog events for [auth-logging.md](auth-logging.md):

| Event | Level | When |
|---|---|---|
| `api_key_created` | Info | Key minted (`key_id`, `prefix`, `user_id`, `created_by`, `scope_count`) |
| `api_key_revoked` | Info | Key revoked (`key_id`, `prefix`, `revoked_by`) |
| `api_key_exchanged` | Info | Successful exchange (`key_id`, `prefix`, `user_id`) |
| `api_key_rejected` | Warn | Failed exchange (`reason`: `unknown`/`revoked`/`expired`/`inactive`/`not_service`) |
| `api_key_seeded` / `api_key_synced` | Info | Env-sourced key upserted at boot |

The plaintext and the hash are never logged. `reason` is recorded internally
even though the HTTP response is uniformly `401` — the operator needs the
distinction, the caller must not have it.

## Testing

| Area | Coverage |
|---|---|
| `domain` | Key generation entropy/format; `effectivePermissions` intersection incl. wildcards, empty-scope inherit, and scope exceeding the account |
| `service` | Exchange happy path; each rejection branch; lockout explicitly **not** blocking exchange; `last_used_at` throttle |
| `handler` | ABAC on all three routes; plaintext present on create and absent from list; `env` key revoke → `409` |
| `postgres` | Repo CRUD against the dev DB (`POSTGRES_URL`, per [CLAUDE.md](../CLAUDE.md)); hash uniqueness; `ON DELETE CASCADE` on user delete |
| `httpauth` | `APIKeyTokenSource` caching, skew-based re-exchange, `Invalidate()` |
| manage-web | Panel renders, create shows plaintext once, revoke updates the row |

An in-memory key repository mirrors the existing memory fallbacks so service
and handler tests need no database.

## Rollout

Each phase ships and is verified independently; nothing is a flag day.

1. **Auth core** — migration, repo, domain, exchange endpoint, seeding. Nothing
   consumes it yet.
2. **Management API + permissions** — routes, ABAC, audit events.
3. **Consumers** — `APIKeyTokenSource`; set `DUPLI1_ORDER_SERVICE_API_KEY` in
   one environment, confirm exchange in logs, then roll forward. Password login
   still works throughout.
4. **manage-web** — the keys panel.
5. **Cleanup** — once every machine caller uses a key, remove the service
   passwords from Secrets Manager and drop the `*_SERVICE_PASSWORD` seeding.
   Service accounts then have no password at all, which is the real security
   win: the credential that could be phished, reused, or sprayed stops existing.

Docs to update when this lands: [current-state.md](current-state.md),
[api.md](api.md), [endpoints.md](endpoints.md), [openapi.yaml](openapi.yaml),
[permissions.md](permissions.md), [auth-logging.md](auth-logging.md),
[README.md](README.md).

## Explicit non-goals

Because there is no public API, this design deliberately omits machinery that
would otherwise be expected:

- **No self-service issuance.** Only an operator with `user.apikey.manage`
  mints a key. No signup, no developer portal, no customer-facing keys.
- **No per-key quotas or billing tiers.** The IP rate limiter is an abuse
  control, not a metering product.
- **No OAuth2 `client_credentials`.** The exchange endpoint is a two-line
  contract we control on both ends; a full OAuth server would be ceremony with
  no second party to interoperate with.
- **No IP allowlists or mTLS per key.** Callers are already inside the VPC.
- **Keys never attach to `customer` or `manager` accounts.** Human access stays
  password + session, so there is no long-lived bearer credential for an
  account that can manage other users.

## Open deployment question

The exchange endpoint sits behind the existing internal gateway, alongside the
rest of `/api/v1/auth/`. That is correct for in-VPC callers — our services, and
an agent running as an ECS task or on a bastion.

An agent running **outside** the VPC (an operator's laptop, a hosted agent
runtime) could not reach it. Widening that is a deployment decision with real
exposure consequences, not an implementation detail, so this design does not
pre-judge it: if such a caller is needed, front it the way manage-web is
fronted and treat it as a separate, explicitly-reviewed change.
