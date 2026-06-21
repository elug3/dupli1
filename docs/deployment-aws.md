# AWS deployment

Schick production runs on **ECS Fargate** in `us-east-1`. Images are built and pushed by `.github/workflows/aws.yml`.

## Database

Production uses **Amazon RDS PostgreSQL 16**, not the legacy `schick-postgres` ECS container.

| Component | Details |
|-----------|---------|
| Instance | `schick-production` |
| Databases | `schick_db` (auth), `products` (product) |
| Credentials | AWS Secrets Manager (`schick/production/*`) |
| Network | Private subnets in `web-prod-vpc` |
| SSL | `sslmode=require` |

Provision and cut over with:

```bash
cd infra/terraform && terraform apply
bash infra/scripts/create-product-database.sh
bash infra/scripts/update-ecs-for-rds.sh
bash infra/scripts/retire-ecs-postgres.sh
```

See [infra/terraform/README.md](../infra/terraform/README.md) for full steps.

## ECS services

| Service | Purpose |
|---------|---------|
| `schick-auth` | Authentication API |
| `schick-product` | Product catalog API |
| `schick-proxy` | nginx reverse proxy (ALB mode) |
| `schick-inventory` | Inventory API (in-memory) |
| `schick-order` | Order API (in-memory) |
| `schick-notification` | Notification API |

`schick-postgres` is deprecated after RDS cutover.

## Required GitHub configuration

| Type | Name | Purpose |
|------|------|---------|
| Secret | `AWS_ACCESS_KEY_ID` | CI deploy credentials |
| Secret | `AWS_SECRET_ACCESS_KEY` | CI deploy credentials |
| Variable | `AWS_REGION` | `us-east-1` |
| Variable | `ECS_CLUSTER` | `production` |

Database URLs are injected via ECS task secrets, not GitHub secrets.

## Internal API (VPN)

Backend APIs (`auth`, `product`, `proxy`) run in **private subnets** and are not internet-facing. Developers reach them over **WireGuard VPN** at:

- `http://internal.schick.local` — internal API gateway

See [vpn-access.md](vpn-access.md) for setup and client configuration.

## Local development

Local development still uses Docker Compose Postgres containers (`postgres-auth`, `postgres-product`). See the root `README.md`.
