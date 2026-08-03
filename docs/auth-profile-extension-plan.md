# Auth profile extension plan

**Status:** Phase A implemented in `auth/` (profile + addresses). Phase B (order snapshot) and profile module extraction remain planned.

**Related:** [payment-service.md](payment-service.md), [payment-methods-plan.md](payment-methods-plan.md), [checkout-session.md](checkout-session.md), [permissions.md](permissions.md), [current-state.md](current-state.md).

## Goals

Add **customer commerce profile** data (display name, phone, saved addresses) without a new microservice yet. Auth already owns identity (`user_id`, email, credentials); this extension owns **PII used for checkout and fulfillment prefill**.

Later, the same bounded context can move to a dedicated **profile module** (separate deployable) with minimal API churn.

| Goal | Detail |
|------|--------|
| Prefill checkout | Customer picks a saved address or edits once; order snapshots at complete |
| NANO / PG fields | Payment reads **order** recipient name/phone — profile is optional prefill only |
| Stay modular | Profile logic lives in `auth/pkg/profile/` (or similar) so extraction is a move, not a rewrite |
| Self-service first | Customers manage own profile; admin read is phase 2 |

## Non-goals (this plan)

- Replacing `GET /api/v1/auth/me` (account/permissions stay as today)
- Storing names or addresses on the **payment** service
- Manager user admin changes (still `PATCH /users/:id/permissions`, etc.)
- Guest checkout profile (guest orders still enter recipient on checkout only)
- International address formats beyond **Korea-first** (design for extension)

---

## Three layers of “who / where”

```text
┌─────────────────────────────────────────────────────────────────┐
│ Auth User (today)          identity + credentials                 │
│   id, email, password, permissions, account_type                  │
├─────────────────────────────────────────────────────────────────┤
│ Auth Profile (this plan)   reusable customer defaults           │
│   display_name, phone, saved addresses                          │
├─────────────────────────────────────────────────────────────────┤
│ Order snapshot (order PR)  immutable per purchase               │
│   recipient_name, recipient_phone, shipping_address JSON        │
└─────────────────────────────────────────────────────────────────┘

Checkout complete:  profile/address ──copy──► order snapshot
Payment (NANO):     order snapshot ──read──► orderName, orderTel, …
```

**Rule:** Profile is the editable default; **order** is the legal/ops record for that transaction.

---

## Phase A — Auth extension (implement first)

### Data model (`dupli1_db`, auth Postgres)

Keep credentials on `users`. Add profile tables keyed by `user_id` (= JWT `sub`).

#### `customer_profiles` (1:1 with `users`)

| Column | Type | Notes |
|--------|------|--------|
| `user_id` | `TEXT PK FK → users(id)` | Same id as auth user |
| `display_name` | `TEXT` | Maps to NANO `orderName` when prefilled |
| `phone` | `TEXT` | KR mobile; normalize on write |
| `created_at` | `TIMESTAMPTZ` | |
| `updated_at` | `TIMESTAMPTZ` | |

- Row created lazily on first `PATCH /me/profile` or first address create.
- `email` stays on `users` only — do not duplicate.

#### `customer_addresses` (1:N)

| Column | Type | Notes |
|--------|------|--------|
| `id` | `TEXT PK` | `addr_…` sequential or ULID |
| `user_id` | `TEXT FK` | Owner |
| `label` | `TEXT` | e.g. `home`, `office` — optional |
| `recipient_name` | `TEXT NOT NULL` | May differ from profile `display_name` |
| `recipient_phone` | `TEXT NOT NULL` | |
| `postal_code` | `TEXT NOT NULL` | 5-digit KR |
| `address_line1` | `TEXT NOT NULL` | Road name + building |
| `address_line2` | `TEXT` | Unit / detail |
| `city` | `TEXT` | 시/군/구 |
| `province` | `TEXT` | 시/도 |
| `is_default` | `BOOLEAN` | At most one `true` per user |
| `created_at` / `updated_at` | `TIMESTAMPTZ` | |

**Indexes:** `(user_id)`, unique partial `(user_id) WHERE is_default` (enforce in app if partial unique is awkward).

### Package layout (inside `auth/`)

```text
auth/pkg/
  domain/
    user.go              # unchanged — credentials
    profile.go           # Profile, Address entities + validation
  ports/
    profile_repository.go
  service/
    profile.go           # GetProfile, UpdateProfile, address CRUD
  infra/postgres/
    profile_repository.go
  handler/
    profile.go           # HTTP handlers
```

Router wires profile routes beside existing `/me` — profile handlers do **not** mix into `User` domain struct.

### API

Base path: **`/api/v1/auth/me/…`** (profile is “my account data”, not admin user CRUD).

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/v1/auth/me/profile` | Bearer access | Return profile + `addresses` array (or empty defaults) |
| `PATCH` | `/api/v1/auth/me/profile` | Bearer access | JSON Merge Patch: `display_name`, `phone` |
| `GET` | `/api/v1/auth/me/addresses` | Bearer access | List addresses |
| `POST` | `/api/v1/auth/me/addresses` | Bearer access | Create address; `is_default: true` clears other defaults |
| `GET` | `/api/v1/auth/me/addresses/{id}` | Bearer access | Own address only |
| `PATCH` | `/api/v1/auth/me/addresses/{id}` | Bearer access | Update; ABAC on `user_id` |
| `DELETE` | `/api/v1/auth/me/addresses/{id}` | Bearer access | Delete; if default removed, clear default flag |
| `POST` | `/api/v1/auth/me/addresses/{id}/default` | Bearer access | Set sole default |

**`GET /api/v1/auth/me`** — unchanged for backward compatibility. Optional later: add `"profile_complete": true|false` hint without embedding full profile.

#### Example `GET /api/v1/auth/me/profile`

```json
{
  "user_id": "usr_000001",
  "display_name": "윤라희",
  "phone": "01041125167",
  "default_address_id": "addr_000001",
  "addresses": [
    {
      "id": "addr_000001",
      "label": "home",
      "recipient_name": "윤라희",
      "recipient_phone": "01041125167",
      "postal_code": "06194",
      "address_line1": "테헤란로 78길 14-12",
      "address_line2": "9층",
      "city": "강남구",
      "province": "서울특별시",
      "is_default": true
    }
  ]
}
```

#### Validation (KR-first)

| Field | Rule |
|-------|------|
| `display_name` / `recipient_name` | 1–50 chars, trim whitespace |
| `phone` / `recipient_phone` | Normalize digits; accept `010-XXXX-XXXX`; store canonical (digits only or E.164 `+82…`) |
| `postal_code` | Exactly 5 digits |
| `address_line1` | Required, max 200 chars |
| Max addresses per user | **10** (configurable constant) |

### Authorization

| Caller | Access |
|--------|--------|
| Customer (empty permissions) | Own `sub` only — standard ABAC |
| Manager with `user.read` | **Phase A:** no cross-user profile API |
| Manager with `user.read` | **Phase B (optional):** `GET /api/v1/auth/users/{id}/profile` for support |

No new permission required for self-service (same pattern as cart: authenticated + owner).

Optional later permissions:

| Permission | Use |
|------------|-----|
| `profile.read` | Support read any customer profile |
| `profile.update` | Support edit (rare; prefer order notes) |

Defer until manage-web needs support tools.

### Events (optional, phase A.2)

Best-effort NATS publish on profile/address change — **not required for MVP**.

| Event | When |
|-------|------|
| `profile.updated` | After PATCH profile |
| `profile.address.created` | After POST address |

Consumers (analytics, search) can subscribe later. Order/payment do **not** subscribe — they use order snapshot.

### Configuration

No new env vars for phase A. Uses existing `DUPLI1_AUTH_DB` / `postgres-auth`.

---

## Phase B — Order checkout integration (order service PR)

Profile extension alone does not satisfy NANO; **order** must snapshot at purchase.

### Checkout session `complete` body (additive)

```json
{
  "recipient_name": "윤라희",
  "recipient_phone": "01041125167",
  "shipping_address": {
    "postal_code": "06194",
    "address_line1": "테헤란로 78길 14-12",
    "address_line2": "9층",
    "city": "강남구",
    "province": "서울특별시"
  },
  "address_id": "addr_000001"
}
```

| Field | Required | Notes |
|-------|----------|-------|
| `recipient_name` | Yes at complete | Prefill from profile or selected address |
| `recipient_phone` | Yes at complete | Prefill from profile or address |
| `shipping_address` | Yes at complete | Snapshot object on order row |
| `address_id` | No | If set, storefront copied from auth address; order still stores snapshot |

**Order service** does not call auth at complete if the client sends the snapshot (simplest). Optional server-side verify: order calls auth internal endpoint with user token to validate `address_id` belongs to user — phase B.1.

### Order schema (order DB)

| Column | Type |
|--------|------|
| `recipient_name` | `TEXT NOT NULL` |
| `recipient_phone` | `TEXT NOT NULL` |
| `shipping_address` | `JSONB NOT NULL` |

### Payment service

Extend `OrderSummary` / order HTTP client with recipient fields. NANO adapter reads order — **no profile HTTP call**.

---

## Phase C — Storefront UX (client)

| Screen | Behavior |
|--------|----------|
| Account → Profile | Edit name, phone; manage addresses |
| Checkout | Load `GET /me/profile`; select default address; allow edit before complete |
| Guest | No profile APIs; manual entry on checkout complete only |

---

## Phase D — Profile module (later extraction)

When to split (any of):

- Profile features grow (preferences, marketing consents, avatar, locale)
- Compliance wants PII in a separate DB / retention policy
- Auth deploy cadence should not ship with profile changes

### Target shape

```text
profile/                    # new Go module (like cart/, payment/)
  cmd/
  pkg/
    domain/
    service/
    infra/pg/               # postgres-profile (new Compose DB)
    handler/

Gateway:  /api/v1/profile/me/…   (or keep /api/v1/auth/me/profile via proxy alias)
Auth:     identity only
```

### Extraction steps

1. **Freeze APIs** — JSON shapes from phase A become the contract.
2. **New database** `profile` on `postgres-profile`; migrate `customer_profiles` + `customer_addresses`.
3. **Dual-write or migration script** — copy rows from `dupli1_db` once.
4. **Deploy `dupli1-profile`** — validate JWT via `AUTH_JWKS_URL` (same as cart/order).
5. **nginx** — route `/api/v1/profile/` or proxy alias for backward compat.
6. **Remove** profile tables from auth DB after cutover.
7. **Events** — `profile.updated` from new service; auth publishes `user.registered` only.

### Backward compatibility

Keep **`/api/v1/auth/me/profile`** as a gateway alias to profile service for one release if clients already shipped.

### What stays in auth forever

- `users`, passwords, lockout, permissions
- `user.registered`, login rate limits
- Service accounts (`dupli1-web`, `dupli1-order`)

---

## Security & privacy

1. **PII at rest** — same RDS/Postgres as auth today; production encryption via RDS. Profile DB split in phase D if required.
2. **Logs** — never log full phone/address; mask in structured logs.
3. **ABAC** — address `{id}` must belong to JWT `sub`; 404 not 403 to avoid id enumeration (match cart pattern).
4. **Managers** — no profile read until explicit permission + audit requirement.
5. **Deletion** — when `users` row deleted, cascade delete profile + addresses (FK `ON DELETE CASCADE`).

---

## Phased delivery checklist

### Phase A — Auth extension

- [x] Schema: `customer_profiles`, `customer_addresses`
- [x] Domain validation (KR phone, postal code)
- [x] Repository + service + handlers
- [x] Router: `/api/v1/auth/me/profile`, `/api/v1/auth/me/addresses/…`
- [x] Tests: ABAC, default address, max addresses, patch merge
- [x] Docs: [endpoints.md](endpoints.md), [api.md](api.md)
- [ ] [openapi.yaml](openapi.yaml) — paths TBD
- [x] [current-state.md](current-state.md) status bump

### Phase B — Order snapshot (blocks NANO card)

- [ ] Order columns + checkout complete validation
- [ ] Payment order client fields for NANO
- [ ] See NANO integration plan (payment PG adapter)

### Phase C — Frontends

- [ ] `dupli1-web` profile + checkout prefill
- [ ] `dupli1-manage-web` — optional read-only customer address on order detail

### Phase D — Profile module

- [ ] Separate service + DB + gateway routes
- [ ] Migration runbook
- [ ] Deprecate in-auth tables

---

## Decision log

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Host profile in auth first | Yes | 1:1 with `user_id`; no new ECS task for MVP |
| API prefix | `/api/v1/auth/me/profile` | Clear ownership; “my data” not admin `/users` |
| Order vs profile for NANO | Order snapshot | Payment stays thin; ops/shipping use same fields |
| New permissions for self-service | No | Authenticated owner ABAC matches cart |
| Email on profile | No | Single source: `users.email` |
| Module package path | `auth/pkg/.../profile` | Clean cut for phase D |
| Guest checkout | Checkout-only fields | No profile row without user |

---

## Doc / code touch list (phase A)

| Area | Change |
|------|--------|
| `auth/pkg/bootstrap/migrate.go` | New tables |
| `auth/pkg/domain/profile.go` | Entities |
| `auth/pkg/service/profile.go` | Use cases |
| `auth/pkg/handler/profile.go` | HTTP |
| `auth/pkg/bootstrap/router.go` | Routes under authed group |
| [endpoints.md](endpoints.md) | Route table |
| [api.md](api.md) | Request/response examples |
| [openapi.yaml](openapi.yaml) | Schemas |
| [current-state.md](current-state.md) | Profile status |

---

## Open questions

1. **Phone format** — store normalized digits only, or display-formatted? (Recommend: digits in DB, format in UI.)
2. **Register flow** — collect name/phone at signup, or only at first checkout? (Recommend: optional at signup, required at checkout complete.)
3. **Admin support view** — does manage-web need `GET /users/{id}/profile` in v1.0? (Recommend: defer; order snapshot enough for fulfillment.)
4. **Address verification** — integrate KR postal API later, or free-text only for MVP? (Recommend: free-text MVP.)
