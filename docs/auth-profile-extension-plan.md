# Auth profile extension plan

> **Phases A+B shipped** (auth profile/addresses + order checkout snapshot). Phase D (profile module extraction) is now underway: the `profile/` Go module and its deployment/infra wiring (Compose, nginx, CI, Terraform) are in place; data migration and cutover from `auth` are not done yet. As-built endpoints: [endpoints.md](endpoints.md) Auth section.

**Status:** Phase A implemented in `auth/` (profile + addresses). **Phase B** implemented in `order/` (checkout snapshot). Phase D (profile module extraction) — `profile/` module and deployment/infra (Compose, nginx one-release aliases, CI, Terraform) done; one-time data copy, dual-run verification, and frontend/auth cutover still pending; wishlists were evaluated for inclusion and intentionally excluded (see Phase D decision log).

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
| `pccc` | `TEXT` | Personal Customs Clearance Code (optional; overseas-purchase customs ID, `P` + 12 digits) |
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
      "pccc": "P123456789012",
      "is_default": true
    }
  ]
}
```

`pccc` is omitted from the response when not set — it is only required for overseas-sourced shipments that clear Korean customs as a personal import.

#### Validation (KR-first)

| Field | Rule |
|-------|------|
| `display_name` / `recipient_name` | 1–50 chars, trim whitespace |
| `phone` / `recipient_phone` | Normalize digits; accept `010-XXXX-XXXX`; store canonical (digits only or E.164 `+82…`) |
| `postal_code` | Exactly 5 digits |
| `address_line1` | Required, max 200 chars |
| `pccc` | Optional; when present must match `P` + 12 digits (case-insensitive, normalized to uppercase) |
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
    "province": "서울특별시",
    "pccc": "P123456789012"
  },
  "address_id": "addr_000001"
}
```

| Field | Required | Notes |
|-------|----------|-------|
| `recipient_name` | Yes at complete | Prefill from profile or selected address |
| `recipient_phone` | Yes at complete | Prefill from profile or address |
| `shipping_address` | Yes at complete | Snapshot object on order row |
| `shipping_address.pccc` | No | Personal Customs Clearance Code; required by the shipper only for overseas-purchase customs clearance |
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

**Scope decision:** wishlists (`product/pkg/infra/pg/wishlist_store.go`) were evaluated for inclusion and rejected — see decision log. This extraction moves `customer_profiles` + `customer_addresses` only, both entities together (they're 1:1/1:N off the same `user_id` and already share one file/route group; splitting display-name/phone from addresses into different services would cut across a currently-atomic record for no reason).

### Target shape

```text
profile/                          # new Go module (like cart/, payment/)
├── go.mod                        # replace github.com/elug3/dupli1/shared => ../shared
├── cmd/{main.go,options.go}      # env config, starts server
└── pkg/
    ├── domain/profile.go         # Profile, Address, ProfileView, validators
    ├── ports/profile_repository.go
    ├── service/profile.go
    ├── infra/postgres/profile_repository.go
    ├── infra/memory/profile_repository.go   # for DB-less tests, same convention as order/cart/payment
    ├── handler/profile.go        # net/http, NOT gin — see framework note
    └── bootstrap/{bootstrap.go,settings.go,router.go}

Gateway:  /api/v1/profile/me/…   (frontend switches from /api/v1/auth/me/… )
Auth:     identity only (users, JWT/JWKS, login/refresh/logout, permissions admin, account locking)
```

**Framework note:** `auth`'s current profile handlers are Gin (`*gin.Context`), because they live inside `auth`, which is the one Gin service. Every other service — including the three this new service sits next to (`cart`, `order`, `product`) — is stdlib `net/http` + `shared/pkg/authjwt`. The extraction should rewrite `pkg/handler/profile.go` to stdlib for consistency: business logic (`domain`/`service`/`ports`/`infra`) is framework-agnostic and moves verbatim; only the ~7 handler functions' signatures and JSON decode/respond boilerplate change.

**Auth wiring note:** `profile` doesn't issue tokens, so unlike `auth`'s own `RequireAuth()` (which validates against its local signing key as the issuer), `profile` validates the way `cart`/`order`/`product`/`payment` already do: `authjwt.NewAccessTokenValidator(cfg.JWKSURL, cfg.JWTSecret)` in `bootstrap.go` (mirrors `cart/pkg/bootstrap/bootstrap.go:60`), fetching RS256 keys from `AUTH_JWKS_URL=http://dupli1-auth:8080/api/v1/auth/.well-known/jwks.json`. Ownership check stays trivial — `caller.ID` from `authjwt.Claims.UserID` compared to the row's `user_id`, same ABAC as today, no new permission.

### File move table

| From (`auth`) | To (`profile`) | Change needed |
|---|---|---|
| `pkg/domain/profile.go` | `pkg/domain/profile.go` | Verbatim move |
| `pkg/ports/profile_repository.go` | `pkg/ports/profile_repository.go` | Verbatim move |
| `pkg/service/profile.go` | `pkg/service/profile.go` | Verbatim move |
| `pkg/infra/postgres/profile_repository.go` | `pkg/infra/postgres/profile_repository.go` | Verbatim move — same `customer_profiles`/`customer_addresses` DDL |
| `pkg/infra/memory/profile_repository.go` | `pkg/infra/memory/profile_repository.go` | Verbatim move |
| `pkg/handler/profile.go` | `pkg/handler/profile.go` | **Rewrite Gin → stdlib** (see framework note) |
| `authed.GET/PATCH/POST/DELETE("/me/...")` block in `auth/pkg/bootstrap/router.go` | new `profile/pkg/bootstrap/router.go` | Route table carries over 1:1, mounted under `/api/v1/profile` instead of `/api/v1/auth` |

Routes carried over unchanged (path shape, just a new prefix):
```
GET    /me/profile
PATCH  /me/profile
GET    /me/addresses
POST   /me/addresses
GET    /me/addresses/{id}
PATCH  /me/addresses/{id}
DELETE /me/addresses/{id}
POST   /me/addresses/{id}/default
```

### Extraction steps

1. **Freeze APIs** — JSON shapes from phase A become the contract (no field changes during the move).
2. **Scaffold `profile/`** per Target shape above; `go.mod` module `github.com/elug3/dupli1/profile`.
3. **New database**: `postgres-profile` in `docker-compose.yml` (next open dev port — `5439`, db `profiles`, following the existing per-service port sequence), plus a `dupli1-profile` compose block modeled on `dupli1-product`'s (JWKS validator env vars, not `JWT_SECRET`-as-issuer). `profile` bootstraps its own tables on startup via the same `CREATE TABLE IF NOT EXISTS` / `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` inline pattern every service already uses — no new migration tooling.
4. **One-time data copy** — both source and destination are Postgres with identical schema, so this is a straight dump/restore, not a transform:
   ```bash
   pg_dump --table=customer_profiles --table=customer_addresses \
     postgres://dupli1:dupli1_dev@localhost:5432/dupli1_db | \
     psql postgres://dupli1:dupli1_dev@localhost:5439/profiles
   ```
5. **Deploy `dupli1-profile` dual-run** — both `auth` and `profile` serve profile/address data from their own copies; do not cut traffic yet.
6. **nginx** — add `/api/v1/profile` location block (points at `dupli1-profile:8080`); frontend switches its address-book calls from `/api/v1/auth/me/...` to `/api/v1/profile/me/...`.
7. **Verify** — confirm reads/writes against `profile` match expectations before touching `auth`. No other service reads these tables server-side today (`order` never calls `auth` at checkout — the client sends `shipping_address` directly), so there's no other consumer to coordinate.
8. **Remove** `pkg/domain/profile.go`, `pkg/service/profile.go`, `pkg/handler/profile.go`, `pkg/infra/*/profile_repository.go`, and the router block from `auth`; drop `customer_profiles`/`customer_addresses` from `auth`'s DB after cutover is confirmed stable.
9. **Events** — `profile.updated` from new service (still optional/deferred, per phase A.2); auth publishes `user.registered` only.

### Backward compatibility

Keep **`/api/v1/auth/me/profile`** as a gateway alias to the profile service for one release if clients already shipped against it.

### Out of scope for this extraction

The address-validation logic (`krPhoneDigits`/`postalCodeRE`/`pcccRE` regexes and normalizers) is duplicated today between `auth/pkg/domain/profile.go` and `order/pkg/domain/shipping.go` — `order` keeps its own copy regardless of who owns the address book, because it needs an immutable per-order snapshot independent of later address edits. Moving the owning service doesn't fix the duplication; that's a separate follow-up (hoist the shared normalize/validate functions into `shared/pkg/...`) worth doing opportunistically but tracked independently of this plan.

### What stays in auth forever

- `users`, passwords, lockout, permissions
- `user.registered`, login rate limits
- Service accounts (`dupli1-web`, `dupli1-order`)

---

## Security & privacy

1. **PII at rest** — same RDS/Postgres as auth today; production encryption via RDS. Profile DB split in phase D if required.
2. **Logs** — never log full phone/address; mask in structured logs. Auth event catalog and rules: [auth-logging.md](auth-logging.md).
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
- [x] [openapi.yaml](openapi.yaml) — paths TBD
- [x] [current-state.md](current-state.md) status bump

### Phase B — Order snapshot (blocks NANO card)

- [x] Order columns + checkout complete validation
- [x] Payment order client fields for NANO
- [x] NANO PG adapter (separate PR)

### Phase C — Frontends

- [ ] `dupli1-web` profile + checkout prefill
- [ ] `dupli1-manage-web` — optional read-only customer address on order detail

### Phase D — Profile module

- [x] Scaffold `profile/` service (hexagonal layout, stdlib `net/http` handlers)
- [x] `postgres-profile` DB + `dupli1-profile` compose block + nginx `/api/v1/profile` route (plus one-release `/api/v1/auth/me/profile` and `/api/v1/auth/me/addresses` aliases in `api/nginx.conf`, `api/nginx.prod.conf`, `api/nginx.ecs.conf`, `api/nginx.ecs.conf.template`) + CI (`test.yml` `profile` job, `aws.yml` build/deploy matrix) + Terraform (ECR, task def, ECS service, Cloud Map; `profile_db_url_secret_arn` still needs its Secrets Manager secret created before the DB secret takes effect in prod)
- [ ] Move domain/ports/service/infra verbatim; rewrite handler layer off Gin — **in progress**: `profile/` module already has domain/ports/service/infra/handler (stdlib `net/http`) implemented; not yet verified against `auth`'s Gin version for byte-for-byte behavior parity
- [ ] One-time data copy (`customer_profiles` + `customer_addresses`) + dual-run verification
- [x] Cut frontend over to `/api/v1/profile/me/...`; drop profile routes/code from `auth` (orphan auth DB tables may remain until manual drop)
- [x] Auth publishes `user.deleted`; profile subscribes and deletes owned PII
- [x] `DELETE /api/v1/auth/users/:id` (`user.delete`)
- [x] Docs: this file, `CLAUDE.md` service table + dev DB credentials table, `AGENTS.md` DB credentials table — updated. Still open: [current-state.md](current-state.md), [api.md](api.md), [endpoints.md](endpoints.md), [openapi.yaml](openapi.yaml), [service-layout.md](service-layout.md)

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
| Wishlists in profile module | No — stays in `product` | Wishlist count is maintained transactionally with `products.wishlist_count` in one DB tx (`product/pkg/infra/pg/wishlist_store.go`); splitting it out costs that atomicity for either eventual consistency or a cross-service dual write. Wishlist also supports **guest** owners (`"g:"+guestID` cookie, no `user_id` at all), which doesn't fit a service modeled around authenticated identity |
| Handler framework for `profile` | stdlib `net/http`, not Gin | Matches `cart`/`order`/`product`/`payment` (4 of 5 non-auth services); `auth` is the Gin outlier because it's the token issuer, not a pattern worth propagating |

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
