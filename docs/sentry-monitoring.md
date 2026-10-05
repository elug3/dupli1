# Sentry error and log monitoring

Status: **as built**, off until a DSN is set. Works with sentry.io or a self-hosted Sentry; nothing here assumes either.

## What is reported

| App | Errors (Sentry Issues) | Logs (Sentry Logs) |
|-----|------------------------|--------------------|
| Go services (auth, product, order, cart, payment, notification, support, profile) | A panic in an HTTP handler (still re-panicked, so net/http logs it), and every `5xx` response, grouped by route pattern and status (`HTTP 502 GET /orders/{id}`) | Every standard-library `log` line, and auth's zerolog lines at their own level. Lines mentioning panic/fatal/error/fail are sent at error level, warn at warn, the rest at info |
| dupli1-web, dupli1-manage-web (server) | Loader, action and render errors (`app/entry.server.tsx`) | `console.warn` / `console.error` |
| dupli1-web, dupli1-manage-web (browser) | Uncaught errors and hydration/render errors (`app/entry.client.tsx`) | none |

Background workers (outbox drains, NATS subscribers, the Telegram pollers) are covered through their log lines, not as issues: a failure there that is only logged shows up in Sentry Logs, where an alert can be set on it.

## What is never sent

Request bodies, cookies and, on Node, stack-frame local variables. Headers and query params go through Sentry's sensitive-key filter (`Authorization`, tokens, IP-forwarding headers are dropped). Go keeps `SendDefaultPII` false; the web apps set `dataCollection` explicitly (`SENTRY_DATA_COLLECTION` in each repo's `app/lib/sentry.ts`). Log lines are sent as written, so a log line that prints customer data would reach Sentry. `SENTRY_LOGS=false` turns log forwarding off for one app while keeping error reporting.

## Configuration

Every app reads the same variables at runtime:

| Variable | Default | Meaning |
|---|---|---|
| `SENTRY_DSN` | unset = off | The Sentry project's DSN |
| `SENTRY_ENVIRONMENT` | `development` (`local` in `docker-compose.yml`, `production` on VENUS) | Environment tag |
| `SENTRY_RELEASE` | unset (manage-web: `APP_GIT_SHA`) | Release name; VENUS sets the image tag |
| `SENTRY_TRACES_SAMPLE_RATE` | `0` | Share of requests traced (`0`–`1`); `0` means errors and logs only |
| `SENTRY_LOGS` | on | `false` stops log forwarding |

Go services tag every event with `service` (`dupli1-order`, …) and set it as the server name, so one Sentry project can hold all of them.

### VENUS (production)

`deploy/venus/docker-compose.yml` wires a DSN per app from `/opt/dupli1/.env`:

```bash
SENTRY_DSN_BACKEND=https://<key>@<host>/<project>     # all Go services
SENTRY_DSN_WEB=https://<key>@<host>/<project>         # dupli1-web
SENTRY_DSN_MANAGE_WEB=https://<key>@<host>/<project>  # dupli1-manage-web
# optional
SENTRY_ENVIRONMENT=production
SENTRY_TRACES_SAMPLE_RATE=0
```

Then recreate the containers (`docker compose -f deploy/venus/docker-compose.yml --env-file /opt/dupli1/.env up -d`). A browser DSN is public by design; the web apps hand it to the page through the root loader. The `support` service is not on VENUS yet (Phase 7), so it reports only once it is.

### Local

`docker-compose.yml` passes `SENTRY_DSN`, `SENTRY_ENVIRONMENT` and `SENTRY_TRACES_SAMPLE_RATE` from the shell or `.env` to every Go service. For the web apps, set the same variables before `npm run start` (`npm run dev` does not load the server SDK).

## Where it lives

- Go: `shared/pkg/sentrymon` (`Init` from each `cmd/main.go`, `Handler` around each service's HTTP handler). A service with SSE (order, support) keeps streaming: the wrapper forwards `Flush`.
- Web apps: `instrument.server.mjs` (preloaded by `npm run start` with `node --import`; the script also sets `NODE_ENV=production` first, because the preload loads React before `react-router-serve` would set it, and a development React with a production react-dom fails every render), `app/entry.server.tsx`, `app/entry.client.tsx`, `app/lib/sentry.ts`.
