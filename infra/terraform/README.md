# Schick AWS RDS

Terraform for the production PostgreSQL database used by `schick-auth` and `schick-product`.

## What this creates

- RDS PostgreSQL 16 (`schick-production`) in private subnets
- Dedicated RDS security group (port 5432 from ECS tasks only)
- Secrets Manager entries:
  - `schick/production/database`
  - `schick/production/auth-db-url`
  - `schick/production/product-db-url`

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

## Connection strings

| Service | Env var | Database |
|---------|---------|----------|
| `schick-auth` | `DB_URL` | `schick_db` |
| `schick-product` | `SCHICK_PRODUCT_DB` | `products` |

Production URLs use `sslmode=require`. Auth automatically selects SSL mode based on host (local/docker → disable, RDS → require).

## Notes

- RDS creates `schick_db` on first boot. Run `create-product-database.sh` for `products`.
- The legacy `schick-postgres` ECS service is no longer needed after cutover.
- Rotate credentials via Secrets Manager and redeploy ECS services.
