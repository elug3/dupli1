# Self-hosted production on VENUS

Runbook for moving Dupli1 production off AWS onto **VENUS** (Debian 13, 16 cores,
12 GB RAM, on the home network) and exposing it through a Cloudflare Tunnel.
Config lives in [`deploy/venus/`](../deploy/venus/); secrets never enter git.

Status (2026-09-27): **cut over.** VENUS serves production through the tunnel
(`cutover.sh cutover` 07:04–07:11 UTC; site back at ~08:10 after the stray-connector
502s described under [Troubleshooting](#troubleshooting)). AWS is scaled to 0 but
not yet deleted. The stack is also reachable over plain HTTP on port 80 from the
home network (`http://192.168.0.69`).

## Architecture

```text
browser ──TLS──▶ Cloudflare (dupli1.com DNS + proxy, already in use today)
                   │ today: origin = AWS ALB
                   │ after cutover: Cloudflare Tunnel ─▶ cloudflared (on VENUS, outbound only)
                   ▼
VENUS  docker compose project "dupli1"  (deploy/venus/docker-compose.yml)
  edge (nginx)            ALB + CloudFront replacement
    :8080 cloudflared only (trusts CF-Connecting-IP), not published
    :80   direct HTTP, published on 0.0.0.0 for the home network
    manage.dupli1.com  ──▶ manage-web
    /api/*, /gateway/* ──▶ proxy :80 (API gateway = production's nginx.ecs.conf)
                             internal APIs answer 404 here; order uses proxy :8081
    /product-images/*  ──▶ s3 (SeaweedFS, anonymous read-only on product-images)
    everything else    ──▶ web
  auth product order cart payment profile notification     ← exact ECS images
  postgres 16 (cart, orders, payments, products, schick_db) ← was RDS
  redis, nats                                              ← as on ECS, no persistence
```

- **Same images as ECS.** The running task images were pulled from ECR by digest
  and tagged `dupli1-prod/<name>:2026-09-26`; a copy is in the backup
  (`images/dupli1-prod-images-2026-09-26.tar.gz`, restore with `docker load`).
  New code comes from GHCR instead — see
  [Deploying new code](#deploying-new-code).
- **Same environment as ECS.** Every container has its Cloud Map name
  (`auth.dupli1.local`, …) as a Docker network alias, so the task-definition
  URLs are unchanged. Only DB URLs, S3 settings and the gateway's DNS resolver
  differ.
- **Same JWT keys.** JWKS is byte-identical to production, so existing customer
  and staff sessions survive the cutover.
- **Images.** Production stored absolute CloudFront URLs; the import rewrites
  them to `https://dupli1.com/product-images/<key>` (the product service's
  `S3_PUBLIC_ENDPOINT`). Object keys and Content-Types are unchanged.
- **SeaweedFS, not MinIO.** MinIO no longer publishes community container
  images (Docker Hub, Quay and Bitnami all fail to pull). SeaweedFS speaks the
  same S3 API the product service's minio-go client uses. Note the repo's
  local `docker-compose.yml` still points at `minio/minio:latest` and will fail
  to pull on a fresh machine.
- **Kept as production had it:** `profile` and `notification` run without a
  database (in-memory), `support` is not deployed, redis and NATS JetStream
  have no volumes. These are pre-existing gaps, not migration changes.

## Files and locations

| What | Where |
|------|-------|
| Compose, nginx, scripts | `deploy/venus/` in this repo |
| Secrets (`.env`, JWT key) | `/opt/dupli1/` (mode 700), written by `make-env.sh` |
| Data | Docker volumes `dupli1_pgdata`, `dupli1_s3data` |
| AWS backup | `~/backups/dupli1-2026-09-26/` (see its `MANIFEST`) |
| Secrets backup key | `~/dupli1-backup-age-key.txt` → move to a password manager |

Command prefix used below:

```bash
DC="docker compose -f deploy/venus/docker-compose.yml --env-file /opt/dupli1/.env"
```

## How it was set up (already done)

```bash
docker load < ~/backups/dupli1-2026-09-26/images/dupli1-prod-images-2026-09-26.tar.gz   # only on a fresh machine
deploy/venus/make-env.sh ~/backups/dupli1-2026-09-26 ~/dupli1-backup-age-key.txt       # /opt/dupli1/.env
deploy/venus/import-backup.sh ~/backups/dupli1-2026-09-26                              # DBs + images + URL rewrite
$DC up -d
```

Host changes: `apache2` disabled (it held port 80), `vm.overcommit_memory = 1`
(`/etc/sysctl.d/99-dupli1-redis.conf`), `age` installed.

The edge publishes port 80 on all interfaces, so anything on the home network
reaches the stack directly over plain HTTP. Docker-published ports bypass the
host firewall; don't forward port 80 on the router — public traffic belongs on
the tunnel. Direct clients can't spoof their IP: only the tunnel listener
(`:8080`) honours `CF-Connecting-IP`.

## Deploying new code

`.github/workflows/images.yml` builds every backend image on each pull request
(build only) and publishes on pushes to `main`, `v*` tags and manual runs:

| Tag | Meaning |
|-----|---------|
| `ghcr.io/elug3/dupli1-<service>:sha-<short>` | every published build — pin these |
| `ghcr.io/elug3/dupli1-<service>:main` | latest `main` |
| `ghcr.io/elug3/dupli1-<service>:v1.2.3` | release tags |

A push to `main` then deploys: the `deploy` job runs on a self-hosted runner on
VENUS (label `venus`, `production` environment), so nothing listens inbound. It
moves the deploy checkout `/opt/dupli1/repo` to the pushed commit and runs
`deploy/venus/deploy.sh sha-<short>` from there, which:

1. pulls the 8 backend images (`auth product order cart payment profile
   notification proxy`) and tags them `dupli1-prod/<name>:<tag>`;
2. sets `DUPLI1_BACKEND_TAG=<tag>` in `/opt/dupli1/.env` and runs `up -d` for
   those services (`redis`, `nats`, `web`, `manage-web` stay on
   `DUPLI1_IMAGE_TAG`);
3. waits up to 2 min for every service to run without restarting, every
   gateway health route to answer 200 and the gateway's internal listener
   (`:8081`) to answer, then holds 15 s;
4. otherwise puts the previous tag back and restarts on it, failing the job.

The gateway config `nginx-gateway.conf` is mounted, not baked into the proxy
image, so it is only read when the proxy container is created. A deploy always
recreates it (its image tag changes), and step 3 also checks the internal
listener (`proxy.dupli1.local:8081`, where order sends the internal APIs). After
editing the file by hand, recreate the proxy yourself:
`$DC up -d --force-recreate proxy`.

After the first CI deploy the stack runs from `/opt/dupli1/repo`, so use
`DC="docker compose -f /opt/dupli1/repo/deploy/venus/docker-compose.yml --env-file /opt/dupli1/.env"`
rather than a development checkout. Deploy or roll back by hand with the same
script: `/opt/dupli1/repo/deploy/venus/deploy.sh sha-abc1234` (needs
`docker login ghcr.io` with a `read:packages` token while the packages are
private).

**Runner safety.** The repo is public and pull-request workflows run code from
the PR, so a fork PR must never reach this runner. Required settings: Actions →
*Approval for running fork pull request workflows* = **all external
contributors**, and the `production` environment limited to the `main` branch.
The runner runs as `serial` (in the `docker` group, i.e. root-equivalent), as
the systemd unit `actions.runner.elug3-dupli1.venus.service`.

## Before cutover — checklist

1. **Smoke-test the copy as owner** (the copy is replaced at cutover, so test
   orders are harmless). In your own terminal, so the password stays out of logs:
   ```bash
   read -rs OWNER_PASSWORD; export OWNER_PASSWORD OWNER_EMAIL=<owner email>
   BASE=http://127.0.0.1 scripts/smoke-money-path.sh
   ```
   Also sign in to the storefront and manage-web against VENUS: add
   `127.0.0.1 dupli1.com manage.dupli1.com` to `/etc/hosts` on VENUS
   temporarily and browse `http://dupli1.com` / `http://manage.dupli1.com`.
2. **Nano Pay IP allowlist** — **done.** Nano confirmed API calls are not
   restricted by source IP, so the switch from the AWS NAT gateway to the home
   connection's dynamic public IP doesn't affect card payments.
3. **Create the tunnel** (Cloudflare dashboard → Zero Trust → Networks →
   Tunnels → Create → Cloudflared). Copy only the token into `/opt/dupli1/.env` as
   `CLOUDFLARE_TUNNEL_TOKEN='…'`. **Don't run the install command the dashboard
   shows** (`docker run … cloudflared`, `cloudflared service install`): it starts an
   extra connector outside the stack's network — see
   [Troubleshooting](#troubleshooting). Do **not** add public hostnames yet — adding
   `dupli1.com` replaces the live DNS record and is the actual switch (step 4 below).
4. **Keep the temporary AWS backup resources** (bucket
   `dupli1-migration-backup-20260926`, role `dupli1-migration-backup-task`,
   task definition `dupli1-migration-backup`, log group
   `/dupli1/migration-backup`) — `cutover.sh` uses them for the final dump.
5. **Machine:** wired Ethernet — **done** (`enp1s0`, DHCP, Wi-Fi disabled).
   Still recommended: a UPS and BIOS "power on after AC loss". Docker and all
   containers start on boot.
6. **Rehearse** as often as you like; it doesn't touch AWS:
   `deploy/venus/cutover.sh rehearse` (≈4½ min on 2026-09-26).

## Cutover

Downtime ≈ ECS scale-down + 4½ min + DNS switch (the script took 7 min on
2026-09-27).

1. `deploy/venus/cutover.sh cutover` — type `CUTOVER`. It records ECS desired
   counts, scales all 12 ECS services to 0, takes the final in-VPC dump, syncs
   images, reimports with `--replace`, moves the Telegram ops-bot values into
   place (they stay empty until then so AWS and VENUS never poll the same bot),
   and starts the stack with `cloudflared`.
2. Check `docker logs dupli1-cloudflared-1` shows registered connections, and
   that it is the **only** connector: `docker ps -a | grep cloudflared` lists just
   `dupli1-cloudflared-1`, `pgrep -a cloudflared` shows one process, and the
   tunnel's page in the dashboard lists one connector (4 connections).
3. Verify locally: `curl -H Host:dupli1.com http://127.0.0.1/gateway/health`.
4. **Switch DNS:** in the tunnel's *Published application routes* tab add
   `dupli1.com` → `http://edge:8080` and `manage.dupli1.com` → `http://edge:8080`,
   accepting the replacement of the existing records (those pointed at the ALB).
5. Verify publicly: storefront, sign-in (existing sessions should survive),
   product images, manage-web order feed, a Telegram ops alert, one real
   low-value card payment if possible.

### Rollback

Remove the tunnel public hostnames and restore the previous DNS records to the
ALB (`dupli1-production-alb-1509499664.us-east-1.elb.amazonaws.com`), then
scale ECS back to the counts saved in
`~/backups/dupli1-cutover-*/config/ecs-services-before-*.json` (all were 1).
Anything written on VENUS after the cutover is **not** on RDS — copy it back
before rolling back if it matters. Clear the `TELEGRAM_*` values in
`/opt/dupli1/.env` and restart `notification` on VENUS so the bot isn't polled twice.

### Troubleshooting

**Cloudflare `502` ("error code: 502", `text/plain`) after the switch, while
`curl -H Host:dupli1.com http://127.0.0.1/gateway/health` works locally.** Another
connector is attached to the tunnel. Cloudflare spreads requests across every
connector, and one outside the compose network can't resolve `edge:8080`. Signs:
nothing but local traffic in `docker logs dupli1-edge-1`, and
`cloudflared_tunnel_total_requests` not rising on the stack's connector
(`docker run --rm --network container:dupli1-cloudflared-1 curlimages/curl -s
localhost:20241/metrics`). On 2026-09-27 this caused ~1 h of downtime: the
dashboard's `docker run cloudflare/cloudflared:latest … --token …` had been run
again after rotating the token, leaving a container with a random name.

Find and remove every connector but `dupli1-cloudflared-1`:

```bash
# find
docker ps -a --format '{{.Names}} {{.Image}}' | grep cloudflared
pgrep -a cloudflared                     # host processes
systemctl is-active cloudflared          # host service

# remove (each container listed above except dupli1-cloudflared-1)
docker rm -f <container-name>
sudo cloudflared service uninstall       # if the host service is active

# confirm: one container, one process, requests reaching the edge
docker ps -a --format '{{.Names}}' | grep cloudflared
curl -s -o /dev/null -w '%{http_code}\n' https://dupli1.com/gateway/health
```

The tunnel page in the dashboard (or the API's `cfd_tunnel/<id>/connections`)
lists each connector with its start time; a connector on another machine on the
home network shows the same origin IP. Rotating the tunnel token also cuts off
every connector that isn't using the new one.

## After cutover (separate, later)

- Decommission AWS (ECS, RDS — final snapshot optional, ALB, NAT gateway, ECR,
  CloudFront, images bucket, temp backup resources, Route53 zone which is not
  authoritative — `dupli1.com` DNS is on Cloudflare).
- Rotate the `Agent` IAM access key, then close the IAM user/role access.
- Set up backups **on VENUS** (nightly `pg_dump` + image volume) to somewhere
  off the machine; there is no RDS/S3 durability any more.
- Consider giving `profile` and `notification` databases, and deploying
  `support` — both gaps predate the migration.
