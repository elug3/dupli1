---
name: verify-service
description: Run the Go tests CI runs for the service modules a change touched (per-module, with Postgres where CI supplies it) plus the gateway route test when nginx config changed. Use before saying a backend change is done, before committing, or when asked to "run tests" or "verify".
---

# Verify a backend change

Each service is its own Go module and the root `go.mod` is a stub, so **`go test ./...` at the repo root does not work**. Test the modules the diff touches (`git diff --name-only master...`), and `shared` too if anything under `shared/` changed, since every service imports it.

```bash
cd <service> && go vet ./... && go test ./... -count=1
```

Services: `auth product order cart payment notification profile support shared`.

## Postgres-backed tests

Without `POSTGRES_URL` those tests **skip silently**, so a green run proves less than it looks. CI supplies a database for `auth`, `product`, `order` and `support`. If the change touches repositories, SQL or migrations in one of those, start the stack's database (`sudo docker compose up -d postgres-<service>`, daemon: `sudo dockerd >/tmp/dockerd.log 2>&1 &`) and re-run with the URL from [AGENTS.md](../../../AGENTS.md), for example:

```bash
cd order && POSTGRES_URL='postgres://dupli1:dupli1_dev@localhost:5435/orders?sslmode=disable' go test ./... -count=1
```

If you could not run them, say the Postgres tests were skipped.

## Gateway changes

If `api/gateway/*.conf`, `api/nginx*.conf` or `deploy/venus/nginx-gateway.conf` changed:

```bash
DOCKER="sudo docker" api/gateway/test.sh
```

and, for local Compose, `sudo docker compose up -d --build dupli1-proxy`.

## Docs

If the API surface changed, `docs/current-state.md`, `docs/api.md` and (for routes) `docs/endpoints.md` must be updated in the same change.

## Report

List the modules tested, which ran with Postgres, and what was not run. On failure, show the output and the cause; never skip or weaken a test to get green.
