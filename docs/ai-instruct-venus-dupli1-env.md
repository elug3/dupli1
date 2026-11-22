# AI instruction: roll out `DUPLI1_ENV` (prod mode) and the NATS token on VENUS

**Where:** VENUS, the production host (`serial` user, Docker stack in `/opt/dupli1/`).
**Audience:** an AI agent or operator with a shell on VENUS, and any agent changing service configuration.
**PR:** dupli1 #389. **Design:** `shared/pkg/runenv`, [deployment-venus.md → Production mode](deployment-venus.md#production-mode-dupli1_env).

---

## Goal

After #389, every Go service reads `DUPLI1_ENV`. **Unset means `prod`.** In prod a service refuses to start, naming every problem in one log line, when a setting it would otherwise fall back on is missing or holds a dev credential. VENUS sets `prod` explicitly, and NATS now runs with `--auth`, so every backend service sends `NATS_TOKEN`.

The only new value production needs is `NATS_TOKEN`. Everything else prod requires is already set on VENUS. So this whole job is: **add one secret to `.env` before the merge, then check the deploy.**

## Stop and ask the human if

- Any check in step 1 fails, or step 1 reports a dev value or a missing name.
- A service refuses to start because of anything other than `NATS_TOKEN` (step 5). Those values are database passwords, the S3 key, API keys or the signing key, and changing one needs more than an `.env` edit.
- The deploy job failed and rolled back twice.
- You are asked to merge #389 and nobody has confirmed a quiet time. Restarting NATS can drop an event in flight, because core NATS does not redeliver.

## Rules

- **Never set `DUPLI1_ENV=dev` on VENUS** to get a service past a refusal. Dev mode turns on exactly the fallbacks prod exists to forbid: in-memory stores that lose orders on restart, a temporary signing key, published passwords. Fix the value instead, or stop and ask.
- **Never print, log or commit `NATS_TOKEN`** or any other secret. Don't `cat` the `.env` or `grep` its values, and don't run `docker inspect` on `nats` without a filter (its command line holds the token). Check secrets by presence or length only.
- Change only `/opt/dupli1/.env`. Do not edit `/opt/dupli1/repo` (the deploy checkout): the next deploy discards edits there.
- Generate only `NATS_TOKEN`. Never regenerate an existing value.

Command prefix:

```bash
DC="docker compose -f /opt/dupli1/repo/deploy/venus/docker-compose.yml --env-file /opt/dupli1/.env"
```

---

## 1. Check `.env` before the merge

Names only, and a dev-value scan that prints names, never values:

```bash
# Every variable the new compose file requires (prints the missing ones)
for v in PG_SUPERUSER_PASSWORD DB_APP_PASSWORD S3_SECRET_KEY JWT_SECRET \
         DUPLI1_ORDER_SERVICE_EMAIL DUPLI1_ORDER_SERVICE_API_KEY DUPLI1_WEB_SERVICE_API_KEY; do
  grep -qE "^$v=." /opt/dupli1/.env || echo "MISSING $v"
done

# Values the services would refuse (prints names only)
python3 - /opt/dupli1/.env <<'EOF'
import sys
dev = {"password", "dupli1_dev", "dupli1_nats_dev", "dev-jwt-secret-do-not-use-in-production"}
for line in open(sys.argv[1]):
    if "=" not in line or line.lstrip().startswith("#"):
        continue
    k, v = line.rstrip("\n").split("=", 1)
    v = v.strip().strip("'\"")
    if v in dev or v.startswith(("dk_test_", "change-me")):
        print("DEV VALUE", k)
    if k.endswith("_SERVICE_API_KEY") and v and not v.startswith("dk_live_"):
        print("NOT A LIVE KEY", k)
EOF

# The signing key auth mounts
test -s /opt/dupli1/secrets/jwt_private_key.pem && echo "jwt key ok"
```

Expect no `MISSING`, no `DEV VALUE`, no `NOT A LIVE KEY`, and `jwt key ok`. Anything else: stop and ask.

## 2. Back up `.env` and add `NATS_TOKEN`

```bash
sudo cp -p /opt/dupli1/.env /opt/dupli1/.env.bak-$(date +%Y%m%d-%H%M)

grep -q '^NATS_TOKEN=' /opt/dupli1/.env && echo "already set" || \
  echo "NATS_TOKEN='$(python3 -c 'import secrets; print(secrets.token_urlsafe(32))')'" | sudo tee -a /opt/dupli1/.env >/dev/null

grep -c '^NATS_TOKEN=.' /opt/dupli1/.env   # expect 1
```

(`/opt/dupli1` is mode 700; drop `sudo` if you are its owner.) The running stack ignores the new line until the deploy, so this changes nothing yet. `token_urlsafe` contains no quote or `$`, so Compose reads it as is.

## 3. Merge and let CI deploy

Merging is the human's call (see **Stop and ask**). A push to `main` runs `.github/workflows/images.yml`, whose `deploy` job runs `deploy/venus/deploy.sh` on VENUS. That one deploy:

- recreates `nats` with `--auth` (Compose recreates a dependency whose command changed), and
- recreates the 8 backend services with `DUPLI1_ENV=prod` and `NATS_TOKEN`.

If the compose file does not load with `.env` (a missing `NATS_TOKEN`, for example), `deploy.sh` says `not deploying … compose file does not load`, puts the checkout back and fails before pulling or restarting anything. If a service comes up unhealthy, it rolls back to the previous images and compose file on its own.

## 4. Check it

```bash
# The deploy checkout has #389
grep -c 'DUPLI1_ENV: prod' /opt/dupli1/repo/deploy/venus/docker-compose.yml   # expect 1 or more

# Every backend service runs as prod and has a token (presence only)
for s in auth product order cart payment profile notification support; do
  printf '%-13s ' $s
  docker exec dupli1-$s-1 sh -c 'printf "%s " "${DUPLI1_ENV:-unset}"; [ -n "$NATS_TOKEN" ] && echo token-set || echo TOKEN-MISSING'
done
# expect: prod token-set, for each

# NATS demands auth (reads the server's greeting, which holds no token)
docker exec dupli1-auth-1 sh -c 'nc -w 2 nats.dupli1.local 4222 </dev/null | head -c 600' | grep -oE '"auth_required":(true|false)'
# expect: "auth_required":true

# No refusals and no NATS auth errors since the deploy
for s in auth product order cart payment profile notification support; do
  docker logs --since 15m dupli1-$s-1 2>&1 | grep -E 'refusing to start|Authorization Violation' | sed "s/^/$s: /"
done
# expect: nothing

# Health through the edge (notification has no health route)
for p in /gateway/health /api/v1/auth/health /api/v1/products/health /api/v1/orders/health \
         /api/v1/cart/health /api/v1/payments/health /api/v1/profile/health /api/v1/support/health; do
  printf '%s %s\n' "$(curl -s -o /dev/null -w '%{http_code}' -H 'Host: dupli1.com' "http://127.0.0.1$p")" "$p"
done
# expect: 200 for each

docker ps --filter name=dupli1- --format '{{.Names}} {{.Status}}' | grep -i restarting   # expect nothing
```

Report the time of the deploy (KST), the commit it deployed, and that every check passed. Never include the token.

## 5. If a service refuses to start

The deploy has rolled back, so production is still running the previous release. Read why:

```bash
docker logs --since 30m dupli1-<service>-1 2>&1 | grep 'refusing to start'
```

The line lists every problem, for example:

```
refusing to start in production (DUPLI1_ENV unset or prod): NATS_TOKEN is required; DUPLI1_ORDER_DB (or DB_URL) carries a development password; set DUPLI1_ENV=dev for local development
```

| Problem | What to do |
|---------|------------|
| `NATS_TOKEN is required` | Step 2 did not land. Redo it, then have the human re-run the failed `deploy` job |
| `NATS_TOKEN is set to a development value` | Replace that one line with a generated token (`sudo sed -i '/^NATS_TOKEN=/d' /opt/dupli1/.env`, then step 2), and re-run the job |
| anything else (`… is required`, `… development value`, `… development password`, `JWT_PRIVATE_KEY_FILE or JWT_PRIVATE_KEY is required`, `DUPLI1_API_KEY_ENV=test …`) | **Stop and ask.** Don't change it yourself, and don't switch to dev |

Ignore the closing hint `set DUPLI1_ENV=dev for local development`: it is meant for laptops, never for VENUS.

## Roll back

The proper rollback is reverting #389 on `main`: CI then deploys the revert, which recreates `nats` without `--auth` and the services without the new checks. The `NATS_TOKEN` line can stay in `.env`, because the old compose file ignores it.

If CI is unavailable, have the human run the previous release by hand from VENUS, with the previous commit and tag (`git -C /opt/dupli1/repo log` and `grep DUPLI1_BACKEND_TAG /opt/dupli1/.env.bak-*`):

```bash
/opt/dupli1/repo/deploy/venus/deploy.sh --ref <previous commit> sha-<previous short sha>
```

---

## When you change service code or configuration

- **Running a binary outside Compose** (`go run ./cmd`, a built binary) runs it as prod. Give it `DUPLI1_ENV=dev` to use dev settings. The local `docker-compose.yml` already sets `dev` for every backend service, through the `x-backend-env` anchor.
- **A new secret or required setting** gets a check in that service's `cmd/runenv.go` (`c.Secret`, `c.Require`, `c.URL`, `c.NotDev`), and its value goes into `/opt/dupli1/.env` *before* the release that requires it is merged. Add it to `make-env.sh` too if a fresh machine should generate it.
- **A new dev credential** in `docker-compose.yml` or an `.env.example` belongs in `runenv.DevValues` (or `devPrefixes`), so it can never reach prod.
- **A new backend service** calls `checkProd` from its `cmd/main.go`, and takes `<<: *backend-env` in both the local and the VENUS compose files.
- **Tests** never go through `cmd/`, so they do not need `DUPLI1_ENV`.
