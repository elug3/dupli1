# Auth RBAC Implementation Brief

## Goal

Implement role-based access control for the auth package with `owner`, `admin`, and `user` roles.

Keep public registration open and assign new users the `user` role by default.

Use database-backed authorization checks so role updates take effect immediately on the next request.

## Current State

Users currently have only `id`, `email`, and `password`.

JWTs currently carry `user_id`, token `type`, expiry fields, and optional `session_id`.

There is no middleware or admin route group yet.

## Target Model

Roles:

- `owner`
- `admin`
- `user`

Permissions:

- `users.read`
- `roles.read`
- `users.roles.update`
- `users.owner.update`

Role behavior:

- `owner` has all permissions.
- `admin` can list users and roles, and can update non-owner roles.
- `user` has no admin permissions.

## Implementation Rules

Do not embed roles or permissions in JWTs.

Validate the access token, load the current user from Postgres, compute permissions, and authorize per request.

Add admin routes under `/api/v1/auth/admin`.

Add startup bootstrap for a first owner using config or environment-provided email and password.

Preserve existing login, register, refresh, and logout behavior unless RBAC explicitly requires a change.

## Safety Rules

Only `owner` may grant or revoke `owner`.

Never allow removal of the final remaining `owner`.

Unknown roles must fail validation.

Missing or invalid tokens must return `401`.

Valid tokens without the required permission must return `403`.

## Verification

Add service tests for `owner`, `admin`, and `user` permission behavior.

Add route tests for `401`, `403`, and successful admin access.

Confirm role changes apply on the next request without issuing a new token.

Run:

```sh
go test ./...
```

## Assumptions

This document is an implementation brief, not a persistent global AI behavior guide.

No code should be changed as part of creating this document.
