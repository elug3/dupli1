# Deployment Plans

This document describes how to deploy the current Schick Go services and what must be true before a release is promoted.

## Current Deployable Units

Schick is organized as a Go workspace with independently runnable service binaries:

| Service | Entrypoint | Purpose | Primary dependencies |
| --- | --- | --- | --- |
| Auth API | `cmd/schick-auth` | Login, logout, token refresh, and session handling | PostgreSQL, Redis, JWT signing secret |
| Product search API | `cmd/schick-product` | Customer-facing product lookup and category search | PostgreSQL |

Both services should be deployed as separate processes so they can scale, roll back, and receive configuration independently.

## Release Readiness Gates

Complete these checks before deploying any environment:

1. Build from a clean checkout of the release commit.
2. Run `go test ./...` from the workspace root.
3. Run focused tests for changed services, for example `go test ./pkg/auth/...` or `go test ./pkg/product/...`.
4. Build service binaries:
   - `go build -o bin/schick-auth ./cmd/schick-auth`
   - `go build -o bin/schick-product ./cmd/schick-product`
5. Confirm database migrations for the release are reviewed, reversible where practical, and safe to run once.
6. Confirm all required secrets are present in the target environment.
7. Confirm observability is configured before traffic is shifted: logs, metrics, uptime checks, and alerts.

## Shared Infrastructure Plan

### PostgreSQL

- Provision a managed PostgreSQL instance per environment.
- Use one database per environment, not shared staging/production data.
- Store connection strings as secrets and pass them as `DB_URL` for auth and `-db` for product until product has environment-variable loading.
- Require TLS for production database connections when the provider supports it.
- Take a backup immediately before applying production migrations.

### Redis

- Provision Redis for auth session and refresh-token workflows.
- Pass the connection string as `REDIS_URL`.
- Configure persistence according to the risk profile of active sessions. If Redis is ephemeral, users may need to log in again after failover.

### Secrets

Store these values in the deployment platform secret manager:

| Secret | Used by | Notes |
| --- | --- | --- |
| `DB_URL` | Auth | PostgreSQL connection URL. |
| Product database URL | Product | Passed through the `-db` flag today. |
| `REDIS_URL` | Auth | Redis connection URL. |
| `JWT_SECRET` | Auth | Must be high entropy and rotated through a planned key-rotation process. |

Never bake secrets into images, command lines committed to the repository, or plaintext deployment manifests.

## Staging Deployment Plan

Use staging as the rehearsal environment for each release.

1. Create or select the immutable release artifact from the target commit.
2. Apply pending database migrations against staging.
3. Deploy `schick-auth` with staging secrets:
   - `SCHICK_AUTH_ADDR=:8080`
   - `SCHICK_AUTH_PUBLIC_ADDR=https://auth.staging.example.com`
   - `DB_URL=<staging auth database URL>`
   - `REDIS_URL=<staging redis URL>`
   - `JWT_SECRET=<staging signing secret>`
   - `SCHICK_AUTH_DEBUG=false`
4. Deploy `schick-product` with staging flags:
   - `-host 0.0.0.0`
   - `-port 8080`
   - `-db <staging product database URL>`
5. Run smoke checks:
   - `GET /health` on auth should return `200 OK`.
   - Product should pass the platform readiness check. Use TCP readiness until product HTTP route wiring is confirmed for the deployed binary.
   - Exercise login with a seeded test user.
   - Exercise product category/search paths once product routes are wired in the service binary.
6. Review application logs for startup errors, database connection churn, Redis failures, and unexpected 4xx/5xx responses.
7. Keep the previous staging revision available until smoke checks pass.

## Production Deployment Plan

Production deploys should be progressive and reversible.

1. Announce the release window to operators and stakeholders.
2. Confirm the staging artifact, commit SHA, database migration set, and configuration match the production release candidate.
3. Take a fresh PostgreSQL backup.
4. Apply backward-compatible database migrations first.
5. Deploy `schick-auth` with production secrets and at least two replicas.
6. Wait for all new auth replicas to pass readiness.
7. Shift a small percentage of auth traffic to the new version.
8. Monitor error rate, latency, authentication failures, Redis errors, and database pool saturation.
9. Continue traffic rollout only while health and business metrics remain stable.
10. Deploy `schick-product` with production database configuration.
11. Wait for product readiness, then shift traffic progressively.
12. Run production smoke checks from an external network:
    - Auth health endpoint.
    - Login/refresh path with a production-safe test account.
    - Product search/category path after product HTTP routes are wired.
13. Keep the previous production revision warm until the post-deploy watch window is complete.

## Rollback Plan

Rollback should restore service health without requiring emergency code changes.

1. Stop traffic rollout immediately.
2. Route traffic back to the previous healthy service revision.
3. Confirm old replicas can still connect to PostgreSQL and Redis.
4. If migrations were backward-compatible, leave them in place and open a follow-up revert migration only after impact is understood.
5. If a migration must be reverted, restore from backup or apply the reviewed down migration according to the database runbook.
6. Rotate any secret that may have been exposed during the failed deploy.
7. Record the failing commit SHA, symptoms, rollback action, and follow-up owner in the incident notes.

## Service Configuration Reference

### Auth API

Auth accepts environment variables and flags. Environment variables should be preferred in managed deployments.

| Setting | Production guidance |
| --- | --- |
| `SCHICK_AUTH_ADDR` | Bind to `:8080` or the platform-provided port. |
| `SERVER_HOST`, `SERVER_PORT` | Alternative host/port configuration. |
| `SCHICK_AUTH_PUBLIC_ADDR` | External URL used for links and redirects. |
| `DB_URL` | Required PostgreSQL URL. |
| `REDIS_URL` | Redis URL for sessions/cache. |
| `JWT_SECRET` | Required signing key. |
| `JWT_EXPIRATION` | Access token lifetime, for example `15m`. |
| `SCHICK_AUTH_REFRESH_TOKEN_EXPIRY` | Refresh token lifetime, for example `24h`. |
| `SCHICK_AUTH_READ_TIMEOUT` | HTTP read timeout. |
| `SCHICK_AUTH_WRITE_TIMEOUT` | HTTP write timeout. |
| `SCHICK_AUTH_IDLE_TIMEOUT` | HTTP idle timeout. |
| `SCHICK_AUTH_SHUTDOWN_TIMEOUT` | Graceful shutdown timeout. |
| `SCHICK_AUTH_DEBUG` | Must be `false` in production. |

### Product Search API

Product currently uses command-line flags for deployment configuration.

| Flag | Production guidance |
| --- | --- |
| `-host` | Use `0.0.0.0` in containers or managed runtimes. |
| `-port` | Use `8080` or the platform-provided port. |
| `-db` | PostgreSQL connection string from the secret manager. |
| `-read-timeout` | Start with `15` seconds unless load tests justify a change. |
| `-write-timeout` | Start with `15` seconds unless load tests justify a change. |

## Operational Checks

Track these signals during and after each deployment:

- HTTP 5xx rate by service and endpoint.
- p95 and p99 latency by service.
- PostgreSQL connection count, slow queries, locks, and replication lag.
- Redis connection failures, command latency, and memory pressure.
- Auth login, refresh, and logout success rates.
- Product search success rate and empty-result rate.
- Process restarts and graceful-shutdown completion.

## Known Deployment Follow-ups

These items should be resolved before treating the services as fully production-ready:

- Add a migration runner and document the exact production migration command.
- Add container images or platform manifests for the two service binaries.
- Wire product HTTP routes into `cmd/schick-product` if they are not registered in the deployed binary.
- Add product environment-variable parsing so secrets do not need to be passed through process flags.
- Add structured logging and metrics exporters for both services.
- Add readiness endpoints that verify database and Redis dependencies, not only process liveness.
