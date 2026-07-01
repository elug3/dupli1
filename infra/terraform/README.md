# Dupli1 AWS RDS

Terraform for the production PostgreSQL database used by `dupli1-auth` and `dupli1-product`.

## What this creates

- RDS PostgreSQL 16 (`dupli1-production`) in private subnets
- Dedicated RDS security group (port 5432 from ECS tasks only)
- Secrets Manager entries:
  - `dupli1/production/database`
  - `dupli1/production/auth-db-url`
  - `dupli1/production/product-db-url`

Local development continues to use Docker Compose Postgres containers. Production ECS services read connection strings from Secrets Manager.

## Prerequisites

- Terraform 1.5+
- AWS credentials with permission to manage RDS, Secrets Manager, EC2 security groups, and ECS
- Existing VPC + private subnets + ECS security group (defaults match the current `production` cluster)

## Apply

```bash
cd infra/terraform
cp terraform.tfvars.example terraform.tfvars
terraform init
terraform plan
terraform apply
```

## Cutover from ECS Postgres

After `terraform apply`:

```bash
export AWS_REGION=us-east-1

# 1. Create the products database on RDS
bash infra/scripts/create-product-database.sh

# 2. Optional: copy data from the legacy ECS postgres container
bash infra/scripts/migrate-ecs-postgres-to-rds.sh

# 3. Point auth/product ECS services at Secrets Manager
bash infra/scripts/update-ecs-for-rds.sh

# 4. Retire the old ECS postgres service
bash infra/scripts/retire-ecs-postgres.sh
```

## Local development against RDS

RDS is in a private subnet. Connect via VPN, then:

```bash
bash infra/scripts/fetch-rds-env.sh
docker compose -f docker-compose.yml -f docker-compose.rds.yml --env-file .env.rds up --build
```

## Connection strings

| Service | Env var | Database |
|---------|---------|----------|
| `dupli1-auth` | `DB_URL` | `dupli1_db` |
| `dupli1-product` | `DUPLI1_PRODUCT_DB` | `products` |

Production URLs use `sslmode=require`. Auth automatically selects SSL mode based on host (local/docker → disable, RDS → require).

## Notes

- RDS creates `dupli1_db` on first boot. Run `create-product-database.sh` for `products`.
- The legacy `dupli1-postgres` ECS service is no longer needed after cutover.
- Rotate credentials via Secrets Manager and redeploy ECS services.
