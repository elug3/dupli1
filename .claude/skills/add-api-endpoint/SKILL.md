---
name: add-api-endpoint
description: Checklist for adding or changing an HTTP endpoint in a dupli1 Go service while keeping the hexagonal layering, permissions, gateway routes and docs consistent. Use when asked to add a route, a use case or a field to an API response.
---

# Add or change an endpoint

Layering is enforced: `handler → service → ports ← infra`, `domain` at the centre and depending on nothing. Business rules live only in `service/` and `domain/`; handlers validate input and translate HTTP. See [CLAUDE.md](../../../CLAUDE.md) and `docs/service-layout.md`.

## Steps

1. **Read first.** Find the service's doc in `docs/` (index: `docs/README.md`) and the neighbouring handler/service. Match their error mapping and response shapes.
2. **domain / service.** Put the rule and use case there, against `ports` interfaces. Add the repo method to the port, then implement it in `infra/` for Postgres **and** the in-memory fake tests use.
3. **handler.** Parse and validate, call the service, map errors to status codes. Use the service's auth middleware.
4. **Permission.** Fine-grained `{resource}.{action}`; add the constant in `shared/pkg/permissions` and the bundle(s) that should carry it, and document it in `docs/permissions.md`. Customers use ABAC (JWT `sub` owns the resource). Internal-only routes (service-to-service) additionally require the named service account and must be added to the internal regex in `api/gateway/routes.conf` so they answer `404` on the public listener.
5. **Gateway.** Public paths are `/api/v1/{service}/…`. Add or change routes only in `api/gateway/routes.conf`, never in a wrapper. Run the gateway test (see **verify-service**).
6. **Money** is whole won in `*_won` fields (JSON, Go, Postgres). Never `*_krw` / `*_cents`.
7. **Schema.** Migrations are inline on startup and additive only (`ADD COLUMN IF NOT EXISTS`). Breaking changes are not supported that way.
8. **Events.** A new cross-service event goes in `shared/pkg/events` (subject and payload once, for both sides). Order and payment write events through the transactional outbox in the same DB transaction as the state change.
9. **Tests.** Service-level tests against the in-memory repo; handler tests for the status mapping; Postgres tests when SQL changed.
10. **Docs.** Update `docs/current-state.md`, `docs/api.md` and `docs/endpoints.md`. Before creating a new doc, check `docs/README.md` for overlap and use the service-name prefix.
11. **Frontends.** If the contract changed for `dupli1-manage-web` or `dupli1-web`, say so in the hand-off; those repos track the contract in their own CLAUDE.md / AGENTS.md.

Then run **verify-service**, and ask the `reviewer` agent to look at the diff.
