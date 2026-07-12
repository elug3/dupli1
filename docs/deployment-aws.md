# AWS deployment

Dupli1 production runs on **ECS with an EC2 capacity provider** in `us-east-1`. Images are built and pushed by `.github/workflows/aws.yml`.

## Architecture

```text
Internet → ALB → dupli1-proxy (ECS)
                    ├── auth / product / order / cart / payment / notification
                    └── NATS (ECS)
               ECS EC2 ASG (t3.large)
RDS PostgreSQL (private)   S3 product-images   CloudWatch Logs
```

Provision with Terraform:

```bash
cd infra/terraform
cp terraform.tfvars.example terraform.tfvars
terraform init && terraform apply
bash infra/scripts/create-rds-databases.sh
```

See [infra/terraform/README.md](../infra/terraform/README.md) for cost estimate, outputs, and cutover details.

## Database

| Component | Details |
|-----------|---------|
| Engine | Amazon RDS PostgreSQL 16 |
| Instance | `dupli1-production` (`db.t3.micro` default) |
| Databases | `dupli1_db`, `products`, `orders`, `cart`, `payments` |
| Credentials | AWS Secrets Manager (`dupli1/production/*`) |
| Network | Private subnets |
| SSL | `sslmode=require` |

RDS creates `dupli1_db` on boot. Run `infra/scripts/create-rds-databases.sh` for the remaining databases.

## ECS services

| Service | Purpose |
|---------|---------|
| `dupli1-auth` | Authentication API |
| `dupli1-product` | Product catalog + stock/reservations |
| `dupli1-order` | Order API |
| `dupli1-cart` | Cart API |
| `dupli1-payment` | Payment API |
| `dupli1-notification` | Notification API |
| `dupli1-proxy` | nginx reverse proxy (ALB target) |
| `dupli1-nats` | NATS JetStream bus |

Inter-service DNS uses Cloud Map (`*.dupli1.local`).

## Required GitHub configuration

| Type | Name | Purpose |
|------|------|---------|
| Secret | `AWS_ACCESS_KEY_ID` | CI deploy credentials |
| Secret | `AWS_SECRET_ACCESS_KEY` | CI deploy credentials |
| Variable | `AWS_REGION` | `us-east-1` |
| Variable | `ECS_CLUSTER` | `production` (Terraform output `ecs_cluster_name`) |

Database URLs and app secrets are injected via ECS task secrets from Secrets Manager, not GitHub secrets.

## Local development

Local development still uses Docker Compose Postgres containers. See the root `README.md`.

For a lower-cost single-instance alternative (Docker Compose on one EC2), see [deployment-ec2.md](deployment-ec2.md).
