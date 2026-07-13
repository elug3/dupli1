# AWS deployment

Dupli1 production runs on **ECS (EC2 launch type)** in `us-east-1`, fronted by an **Application Load Balancer**. Images are built and pushed by `.github/workflows/aws.yml`.

## Architecture

```text
Internet → ALB → dupli1-proxy (nginx)
                   ├── auth.dupli1.local
                   ├── product.dupli1.local
                   ├── order.dupli1.local
                   └── notification.dupli1.local
         EC2 ASG (ECS capacity provider) in private subnets
         NAT Gateway → ECR / Secrets Manager / CloudWatch
         RDS PostgreSQL (private)
         S3 (product images)
```

IaC lives in [`infra/terraform/`](../infra/terraform/README.md).

## Database

Production uses **Amazon RDS PostgreSQL 16** (`dupli1-production`).

| Component | Details |
|-----------|---------|
| Databases | `schick_db` (auth), `products` (product) |
| Credentials | AWS Secrets Manager (`dupli1/production/*`) |
| Network | Private subnets; tasks use SG rule to port 5432 |
| SSL | `sslmode=require` |

## ECS services

| Service | Purpose |
|---------|---------|
| `dupli1-auth` | Authentication API |
| `dupli1-product` | Product catalog + inventory |
| `dupli1-order` | Order / checkout API |
| `dupli1-notification` | Notification consumer |
| `dupli1-proxy` | nginx gateway (ALB target) |
| `dupli1-redis` | Auth rate-limit / session cache |
| `dupli1-nats` | Event bus |

## Required GitHub configuration

| Type | Name | Purpose |
|------|------|---------|
| Secret | `AWS_ACCESS_KEY_ID` | CI deploy credentials |
| Secret | `AWS_SECRET_ACCESS_KEY` | CI deploy credentials |
| Variable | `AWS_REGION` | `us-east-1` |
| Variable | `ECS_CLUSTER` | `production` |

## Local development

Local development still uses Docker Compose. See the root `README.md`. For a single-box EC2 alternative, see [deployment-ec2.md](deployment-ec2.md).
