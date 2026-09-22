# Dupli1 AWS (ECS on EC2)

Terraform provisions the production compute path on the existing VPC and RDS:

| Resource | Purpose |
|----------|---------|
| NAT Gateway (1 AZ) | Outbound for private ECS tasks (ECR, Secrets Manager, Logs) |
| ALB | Public HTTP + HTTPS → storefront + `dupli1-proxy` |
| Route53 aliases | `dupli1.com` / `www` → ALB |
| EC2 ASG (`t3.large`, default 2) | ECS container instances (awsvpc trunking via instance-role opt-in) |
| ECS capacity provider | EC2 launch type for backend services |
| S3 | Private product-image bucket |
| CloudFront + OAC | Public CDN for product images (`images.dupli1.com`) |
| CloudWatch Logs | `/ecs/dupli1-*` log groups |
| ECS services | auth, product, order, cart, payment, notification, profile, proxy, web, manage-web, redis, nats |
| EFS (`redis_storage.tf`) | Durable `/data` for the Redis task — see [Redis persistence](#redis-persistence) |

Existing resources reused (not recreated): VPC `dupli1-prod-vpc`, ECS cluster `production`, RDS `dupli1-production`, ECR repos **except** `dupli1-profile`, Cloud Map `dupli1.local`, Secrets Manager DB URLs / JWT / Telegram.

`dupli1-profile` is Terraform-managed (`aws_ecr_repository.profile`). Older service repos stay as `data.aws_ecr_repository` lookups. GitHub Actions `AmazonEC2ContainerRegistryPowerUser` cannot create repositories; `aws_iam_role_policy.github_actions_ecr_create` grants `ecr:CreateRepository` on `dupli1-*` so `.github/workflows/aws.yml` can create a missing matrix repo after this policy is applied.

## Redis persistence

Redis is **not** a cache in this stack. auth's refresh-token ledger lives in
it, and `Refresh` treats a lookup miss as revoked
(`auth/pkg/service/service.go`), so an empty Redis rejects every refresh token
in existence — every customer and every operator is signed out at once.
manage-web's admin sessions are in there too. The rate-limit counters (auth
per-IP, product per-code) are the only genuinely disposable keys.

`redis_storage.tf` gives the task an EFS file system mounted at `/data`, and
the task runs `redis-server --appendonly yes`. EFS rather than a host path
because ECS places the task on either instance and only a network file system
survives the move. `appendfsync` stays at the default `everysec`.

Three constraints hold this together — changing any one of them risks a
corrupt append-only file:

- **One writer.** `aws_ecs_service.redis` pins `desired_count = 1` rather than
  following `var.desired_count`. Two tasks would append to the same AOF.
- **Stop before start.** `deployment_minimum_healthy_percent = 0` /
  `deployment_maximum_percent = 100`. The ECS default (100/200) overlaps the
  old and new task, which is the two-writer case during every deploy.
- **The access point.** The task mounts through
  `aws_efs_access_point.redis`, which pins writes to uid/gid 999 (the `redis`
  user in `redis:7-alpine`) and roots the task at `/redis`. ECS requires
  `transit_encryption = "ENABLED"` whenever an access point is named, which is
  why the instance user-data installs `amazon-efs-utils`.

### Applying it

Two things to expect, neither of them zero-impact:

- **The instance user-data changed**, and the ASG has
  `instance_refresh { triggers = ["launch_template"] }`, so the apply rolls
  both EC2 hosts at `min_healthy_percentage = 50`. Every task is rescheduled.
- **Replacing the Redis task is a brief outage** — by design, since the old
  task must stop first. During it auth refresh answers `503`, which both BFFs
  now treat as "retry, keep the session" rather than as a dead token, so
  nobody is signed out. manage-web's own session store still throws while
  Redis is away, so the console errors for a few seconds. Prefer applying
  outside business hours, but it is no longer a forced logout.

The first task to start creates an empty AOF; everything already in the
old in-memory Redis is lost at that point, which is the same forced re-login
described above. Nothing needs migrating.

Verify after the apply:

```bash
# The task is mounting EFS, not a local volume.
aws ecs describe-tasks --cluster production \
  --tasks "$(aws ecs list-tasks --cluster production --service-name dupli1-redis \
    --query 'taskArns[0]' --output text)" \
  --query 'tasks[0].attachments[?type==`AmazonElasticFileSystem`]'

# Persistence is actually on (expects appendonly:yes).
# From any task in the VPC, e.g. exec into auth:
redis-cli -h redis.dupli1.local CONFIG GET appendonly
```


## Frontend deploys — who owns the task definitions

The two frontend services carry `ignore_changes = [desired_count,
task_definition]`, so **their images are deployed by their own pipelines and
Terraform never moves them**. This is load-bearing, not tidiness: the
Terraform-rendered definitions use `:${var.image_tag}` = `:latest`, and
neither frontend pipeline pushes that tag — `dupli1-web` pushes `<full-sha>`
only, `dupli1-manage-web` `<full-sha>` and `v<ver>-b<build>`. Without the
ignore, an apply would point both at a tag that does not exist and take the
storefront and admin console down. Terraform still owns those services, their
target groups and listener rules.

This is a settled choice, not a pending cleanup: GitHub Actions deploys the
frontends, Terraform does not. Two consequences follow from it.

- **`aws_ecs_task_definition.web` / `.manage_web` are rendered but never
  deployed.** New revisions accumulate unused on each apply, and an env change
  made there — `REDIS_URL` among them — does **not** reach production. To
  change a frontend's environment, CPU or memory, edit the pipeline's own task
  definition instead.
- **`dupli1-manage-web` defines this service twice**, here and in its
  checked-in `.aws/task-definition.json`: different family
  (`dupli1-manage-web-task`), different launch type (FARGATE/awsvpc against
  this file's bridge/EC2). The pipeline's copy is the one that deploys. They
  are not interchangeable, so which one the service is running right now
  cannot be answered from the repository — check the live service before
  changing either. Both currently set `REDIS_URL`, so that setting holds
  either way, but the two drift apart unless kept in step by hand.

## Telegram (notification)

Design and runbook: [docs/notification-telegram-bot.md](../../docs/notification-telegram-bot.md).

Secret `dupli1/production/telegram` — **target:** `TELEGRAM_BOT_TOKEN` only. Transitional ECS wiring also injects chat IDs and allowlist from the same secret.

```bash
aws secretsmanager put-secret-value --secret-id dupli1/production/telegram --secret-string '{
  "TELEGRAM_BOT_TOKEN":"<token>",
  "TELEGRAM_ALLOWED_USER_IDS":"<user-id-1>,<user-id-2>",
  "TELEGRAM_ORDER_CHAT_ID":"<chat-id>",
  "TELEGRAM_PRODUCT_CHAT_ID":"<chat-id>"
}'
aws ecs update-service --cluster production --service dupli1-notification --force-new-deployment
```

Chat IDs: allowlisted users send `/start` to the bot. User IDs: [@userinfobot](https://t.me/userinfobot).

## NATS authorization

Secret: `dupli1/production/nats-token` (JSON key `NATS_TOKEN`). Terraform seeds a random token on first apply and injects it into the NATS server (`--auth`) and every publisher/subscriber task. Rotate by putting a new value and force-deploying nats plus auth, product, order, payment, notification, and profile together.

```bash
aws secretsmanager put-secret-value --secret-id dupli1/production/nats-token --secret-string '{"NATS_TOKEN":"<new-token>"}'
for svc in nats auth product order payment notification profile; do
  aws ecs update-service --cluster production --service dupli1-$svc --force-new-deployment
done
```

## NANO payment (credit card)

Secret: `dupli1/production/nano-payment` (JSON keys `NANO_API_KEY`, `NANO_LOGIN_ID`, `NANO_SHOPCODE`, `NANO_VER`).

Dupli1 uses **NANO Solution 인증결제** (certified payment API, e.g. guide v2.7) — not 수기결제. Terraform creates the secret shell and injects keys into `dupli1-payment`. Card checkout stays disabled until `NANO_API_KEY` is non-empty.

**연동 테스트** values from the NANO 인증결제 guide (auto-approve on `dev3`; no real charges):

| Key | Test value (dev3 only) | Production |
|-----|------------------------|------------|
| `NANO_BASE_URL` | `https://dev3.nanopay.co.kr` | `https://pay.nanopay.co.kr` (`var.nano_base_url`, default) |
| `NANO_VER` | `240000005` | from contract |
| `NANO_SHOPCODE` | `240000005` | from contract |
| `NANO_LOGIN_ID` | `shoptest` | from contract |
| `NANO_API_KEY` | `R7L9PxM5V8K2Jc4N6dWqY1Eb3T5XhZU2` | from contract |

```bash
# Production (after contract) — fill real keys, keep BaseURL on pay.nanopay.co.kr
aws secretsmanager put-secret-value --secret-id dupli1/production/nano-payment --secret-string '{
  "NANO_API_KEY":"<from-nano-contract>",
  "NANO_LOGIN_ID":"<loginId>",
  "NANO_SHOPCODE":"<shopcode>",
  "NANO_VER":"<ver>"
}'
aws ecs update-service --cluster production --service dupli1-payment --force-new-deployment
```

For local/dev against NANO’s published **연동 테스트** merchant, set `nano_base_url = "https://dev3.nanopay.co.kr"` and the test secret values above. Test keys will not work on `pay.nanopay.co.kr`.

Ask NANO to allowlist the production NAT egress IP and register webhook `https://dupli1.com/api/v1/payments/webhooks/nano` if JSON callbacks are needed. Browser return URL is `https://dupli1.com/api/v1/payments/nano/return`.

See [docs/payment-service.md](../../docs/payment-service.md).

## Web service account (customer registration)

Secret: `dupli1/production/web-service-account` (JSON keys `DUPLI1_WEB_SERVICE_EMAIL`, `DUPLI1_WEB_SERVICE_PASSWORD`).

Terraform creates this secret and injects it into:

- `dupli1-auth` — seeds/syncs the machine user (`user.create`) on boot
- `dupli1-web` — BFF logs in and calls `POST /api/v1/auth/register`

Rotate the password with:

```bash
EMAIL=dupli1-web@web.dupli1.com
PASSWORD="$(openssl rand -base64 24)"
aws secretsmanager put-secret-value --secret-id dupli1/production/web-service-account --secret-string "$(jq -n \
  --arg e "$EMAIL" --arg p "$PASSWORD" \
  '{DUPLI1_WEB_SERVICE_EMAIL:$e,DUPLI1_WEB_SERVICE_PASSWORD:$p}')"
aws ecs update-service --cluster production --service dupli1-auth --force-new-deployment
aws ecs update-service --cluster production --service dupli1-web --force-new-deployment
```

Auth syncs the DB password from the secret on startup. Do not rely on `DUPLI1_WEB_SERVICE_TOKEN` in production (access JWTs expire in ~15 minutes).

## Order service account (stock reservations)

Secret: `dupli1/production/order-service-account` (JSON keys `DUPLI1_ORDER_SERVICE_EMAIL`, `DUPLI1_ORDER_SERVICE_PASSWORD`).

Terraform creates this secret and injects it into:

- `dupli1-auth` — seeds/syncs the machine user (`order.ship`, `order.status.update`, `inventory.reservation.manage`) on boot
- `dupli1-order` — logs in via `DUPLI1_AUTH_URL` and calls product stock through `DUPLI1_GATEWAY_URL`

Without these credentials, checkout `POST .../complete` fails with `product stock request failed: unauthorized` / web **"internal error"**.

Rotate the password with:

```bash
EMAIL=dupli1-order@order.dupli1.com
PASSWORD="$(openssl rand -base64 24)"
aws secretsmanager put-secret-value --secret-id dupli1/production/order-service-account --secret-string "$(jq -n \
  --arg e "$EMAIL" --arg p "$PASSWORD" \
  '{DUPLI1_ORDER_SERVICE_EMAIL:$e,DUPLI1_ORDER_SERVICE_PASSWORD:$p}')"
aws ecs update-service --cluster production --service dupli1-auth --force-new-deployment
aws ecs update-service --cluster production --service dupli1-order --force-new-deployment
```

> **Note:** `dupli1-web` GitHub Actions deploy currently renders env from repo secrets. Prefer this Secrets Manager injection (Terraform task definition) so registration does not depend on short-lived tokens or empty Actions secrets. If the web deploy workflow passes an empty `environment-variables` multiline, it must omit the input entirely — a `::warning::` line breaks `amazon-ecs-render-task-definition`.

## Monthly cost (dev-sized, us-east-1, 24/7)

| Service | Estimate |
|---------|----------|
| EC2 t3.large (2× with trunking) | ~$120 |
| EBS 40 GB gp3 ×2 | ~$6 |
| NAT Gateway | ~$32 + data |
| ALB + public IPv4 | ~$22–30 |
| RDS db.t3.micro + storage | ~$17 |
| ECR / S3 / CloudWatch / Secrets | ~$5–10 |
| **Total (Dupli1 core)** | **~$210–230/mo** |

Avoid leaving the ASG at 5–6 instances (~+$240–300/mo) or idle Global Accelerators (~+$36/mo). See [docs/aws-cost-optimization.md](../../docs/aws-cost-optimization.md).

## Pause / resume (cost lightening)

```bash
# Stop ECS tasks, scale ASG to 0, stop RDS (~saves EC2 + RDS hours)
bash infra/scripts/pause-aws.sh

# Also delete NAT Gateway (~+$32/mo saved; slower resume)
DELETE_NAT=1 bash infra/scripts/pause-aws.sh

# Bring stack back
bash infra/scripts/resume-aws.sh
# If NAT was deleted:
APPLY_NAT=1 bash infra/scripts/resume-aws.sh
```

While paused, ALB (and NAT unless deleted) still bill. RDS storage continues to bill; RDS auto-restarts after 7 days.

```bash
cd infra/terraform
cp terraform.tfvars.example terraform.tfvars   # optional overrides
terraform init
terraform plan
terraform apply
```

Before the first apply, remove the paused Fargate services so Terraform can recreate them on EC2:

```bash
bash infra/scripts/recreate-ecs-services-for-ec2.sh
```

Or let the script call `terraform apply` after deleting the old services.

## Images

GitHub Actions (`.github/workflows/aws.yml`) builds and pushes to ECR (including `dupli1-cart` / `dupli1-payment`), then force-redeploys ECS services via **OIDC** (`github-actions-deploy-role`). Proxy uses `api/Dockerfile.ecs` (Cloud Map DNS).

### GitHub Actions OIDC role

`github_actions_oidc.tf` manages `github-actions-deploy-role` (ECR push + ECS deploy). The role already exists in the account; import before the first Terraform apply:

```bash
terraform import aws_iam_role.github_actions_deploy github-actions-deploy-role
terraform import aws_iam_role_policy.github_actions_ecs_deploy github-actions-deploy-role:ECSDeployPolicy
terraform import aws_iam_role_policy_attachment.github_actions_ecr_power_user github-actions-deploy-role/arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryPowerUser
```

## Product images (CloudFront)

The product-images bucket stays **private**. Browsers load objects via CloudFront Origin Access Control. After apply:

```bash
terraform output product_images_cdn_url
# → https://images.dupli1.com  (or https://dxxxx.cloudfront.net)
```

`dupli1-product` gets `S3_PUBLIC_ENDPOINT` from that value (task env + Secrets Manager). See [docs/product-images-browser-access.md](../../docs/product-images-browser-access.md).

If the ACM certificate does not include `images.dupli1.com`, either add the SAN or set `product_images_cdn_aliases = []` to use the default CloudFront domain.

## Gateway

After apply:

```bash
terraform output gateway_health_url
curl -k "$(terraform output -raw gateway_health_url)"
```
