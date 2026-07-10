# AWS deployment

Dupli1 production runs on **ECS Fargate** in `us-east-1`. Images are built and pushed by `.github/workflows/aws.yml`.

## Database

Production uses **Amazon RDS PostgreSQL 16**, not the legacy `dupli1-postgres` ECS container.

| Component | Details |
|-----------|---------|
| Instance | `dupli1-production` |
| Databases | `dupli1_db` (auth), `products` (product) |
| Credentials | AWS Secrets Manager (`dupli1/production/*`) |
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
| `dupli1-auth` | Authentication API |
| `dupli1-product` | Product catalog API (also stock/reservations) |
| `dupli1-proxy` | nginx reverse proxy (ALB mode) |
| `dupli1-order` | Order API |
| `dupli1-notification` | Notification API |

`dupli1-postgres` is deprecated after RDS cutover.

## Required GitHub configuration

| Type | Name | Purpose |
|------|------|---------|
| Secret | `AWS_ACCESS_KEY_ID` | CI deploy credentials |
| Secret | `AWS_SECRET_ACCESS_KEY` | CI deploy credentials |
| Variable | `AWS_REGION` | `us-east-1` |
| Variable | `ECS_CLUSTER` | `production` |

Database URLs are injected via ECS task secrets, not GitHub secrets.

## Local development

Local development still uses Docker Compose Postgres containers (`postgres-auth`, `postgres-product`). See the root `README.md`.
