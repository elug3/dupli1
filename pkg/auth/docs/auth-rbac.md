# Auth RBAC

Role-based access control for the auth service.

## Status: Implemented

RBAC is live. Roles are stored in PostgreSQL and checked on every admin request — they are not embedded in JWTs.

## Roles

| Role | Description |
|------|-------------|
| `owner` | Full access; seeded on first startup |
| `admin` | User management (list, create, update role, delete) |
| `user` | Default for new registrations; no admin access |

## Public Auth Routes

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/auth/register` | Register (assigns `user` role) |
| POST | `/api/v1/auth/login` | Login, returns token pair |
| POST | `/api/v1/auth/refresh` | Refresh tokens |
| POST | `/api/v1/auth/logout` | Invalidate refresh token |
| GET | `/api/v1/auth/me` | Current user (includes `role`) |

## Admin Routes

Admin routes live at `/api/v1/users` (not `/api/v1/auth/admin` as originally planned in the design brief).

Requires `Authorization: Bearer <access_token>` from a user with `owner` or `admin` role.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/users` | List all users |
| POST | `/api/v1/users` | Create user (optional `role` field) |
| GET | `/api/v1/users/{id}` | Get user by ID |
| PUT | `/api/v1/users/{id}/role` | Update user role |
| DELETE | `/api/v1/users/{id}` | Delete user |

## Authorization Flow

1. `RequireAdmin()` middleware extracts Bearer token
2. Validates access token via JWT
3. Loads user from Postgres by token claims
4. Checks `user.Role` is `owner` or `admin`
5. Returns `401` for missing/invalid tokens, `403` for insufficient role

Role changes take effect on the next request without re-issuing tokens.

## Owner Bootstrap

On first startup, if `OWNER_EMAIL` and `OWNER_PASSWORD` are set and no owner exists, an owner account is created automatically.

Docker Compose defaults:

```
OWNER_EMAIL=admin@schick.com
OWNER_PASSWORD=password
```

## Safety Rules (enforced in service layer)

- Valid roles: `user`, `admin`, `owner`
- Unknown roles fail validation
- Missing or invalid tokens return `401`
- Valid tokens without admin role return `403`

## Tests

```bash
cd pkg/auth && go test ./...
```

Coverage includes service-level permission behavior and handler route tests.
