# Self-hosted production on VENUS

Runbook for moving Dupli1 production off AWS onto **VENUS** (Debian 13, 16 cores,
12 GB RAM, on the home network) and exposing it through a Cloudflare Tunnel.
Config lives in [`deploy/venus/`](../deploy/venus/); secrets never enter git.

Status (2026-09-26): **prepared and rehearsed, not cut over.** AWS still serves
production. VENUS runs a full copy loaded from a fresh production dump, reachable
over plain HTTP on port 80 from the home network (`http://192.168.0.69`).

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
    /api/*, /gateway/* ──▶ proxy (API gateway = production's nginx.ecs.conf)
    /product-images/*  ──▶ s3 (SeaweedFS, anonymous read-only on product-images)
    everything else    ──▶ web
  auth product order cart payment profile notification     ← exact ECS images
  postgres 16 (cart, orders, payments, products, schick_db) ← was RDS
  redis, nats                                              ← as on ECS, no persistence
```

- **Same images as ECS.** The running task images were pulled from ECR by digest
  and tagged `dupli1-prod/<name>:2026-09-26`; a copy is in the backup
  (`images/dupli1-prod-images-2026-09-26.tar.gz`, restore with `docker load`).
  Deploying new code later means building images from the repos and bumping
  `DUPLI1_IMAGE_TAG`; there is no CI deploy to VENUS yet.
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
   Tunnels → Create → Cloudflared). Copy the token into `/opt/dupli1/.env` as
   `CLOUDFLARE_TUNNEL_TOKEN='…'`. Do **not** add public hostnames yet — adding
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

Downtime ≈ ECS scale-down + 4½ min + DNS switch.

1. `deploy/venus/cutover.sh cutover` — type `CUTOVER`. It records ECS desired
   counts, scales all 12 ECS services to 0, takes the final in-VPC dump, syncs
   images, reimports with `--replace`, moves the Telegram ops-bot values into
   place (they stay empty until then so AWS and VENUS never poll the same bot),
   and starts the stack with `cloudflared`.
2. Check `docker logs dupli1-cloudflared-1` shows registered connections.
3. Verify locally: `curl -H Host:dupli1.com http://127.0.0.1/gateway/health`.
4. **Switch DNS:** in the tunnel's *Public Hostname* tab add
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

## After cutover (separate, later)

- Decommission AWS (ECS, RDS — final snapshot optional, ALB, NAT gateway, ECR,
  CloudFront, images bucket, temp backup resources, Route53 zone which is not
  authoritative — `dupli1.com` DNS is on Cloudflare).
- Rotate the `Agent` IAM access key, then close the IAM user/role access.
- Set up backups **on VENUS** (nightly `pg_dump` + image volume) to somewhere
  off the machine; there is no RDS/S3 durability any more.
- Consider giving `profile` and `notification` databases, and deploying
  `support` — both gaps predate the migration.
