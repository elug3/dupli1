# AWS deployment

Dupli1 production runs on **ECS (EC2 launch type)** in `us-east-1`, fronted by an **Application Load Balancer** (HTTP + HTTPS). Images are built and pushed by `.github/workflows/aws.yml`.

## Architecture

```text
Internet → Route53 (dupli1.com)
        → ALB (HTTPS :443, HTTP :80)
             ├── /api/*, /gateway/* → dupli1-proxy (nginx → Cloud Map)
             │     auth / product / order / cart / payment / notification
             └── /*                 → dupli1-web (storefront, bridge mode)
         EC2 ASG (ECS capacity provider) in private subnets
         NAT Gateway → ECR / Secrets Manager / CloudWatch
         RDS PostgreSQL (private)
         S3 (product images)
         manage.dupli1.local → dupli1-manage-web (VPN / private DNS only)
```

IaC lives in [`infra/terraform/`](../infra/terraform/README.md).

## Database

Production uses **Amazon RDS PostgreSQL 16** (`dupli1-production`).

| Component | Details |
|-----------|---------|
| Databases | `schick_db` (auth), `products`, `orders`, `cart`, `payments` |
| Credentials | AWS Secrets Manager (`dupli1/production/*-db-url`, `jwt-secret`) |
| Network | Private subnets; ECS tasks + ECS instances SG → port 5432 |
| SSL | `sslmode=require` |

Create app DBs after RDS is up: `bash infra/scripts/create-rds-databases.sh`.

## ECS services

| Service | Purpose |
|---------|---------|
| `dupli1-auth` | Authentication API |
| `dupli1-product` | Product catalog + inventory |
| `dupli1-order` | Order / checkout API |
| `dupli1-cart` | Shopping cart API |
| `dupli1-payment` | Payments (Stripe / dev simulate) |
| `dupli1-notification` | Notification consumer |
| `dupli1-proxy` | nginx gateway (ALB `/api/*`, `/gateway/*`) |
| `dupli1-web` | Public storefront (ALB default) |
| `dupli1-manage-web` | Admin UI (Cloud Map only) |
| `dupli1-redis` | Auth rate-limit / session cache |
| `dupli1-nats` | Event bus |

Cloud Map namespace: `dupli1.local` (short names: `auth`, `product`, `order`, `cart`, `payment`, …).

## Capacity notes

Without **account-level `awsvpcTrunking` enabled for the ECS instance role**, each `t3.large` supports ~2 awsvpc tasks. The ASG defaults to **5 instances** so auth/product/order/cart/payment/notification/proxy/redis/nats/manage-web can place. Prefer enabling trunking (account default + instance role) and then lowering desired capacity.

## Required GitHub configuration

| Type | Name | Purpose |
|------|------|---------|
| Secret | `AWS_ACCESS_KEY_ID` | CI deploy credentials (prefer OIDC role long-term) |
| Secret | `AWS_SECRET_ACCESS_KEY` | CI deploy credentials |
| Variable | `AWS_REGION` | `us-east-1` |
| Variable | `ECS_CLUSTER` | `production` |

Frontends (`dupli1-web`, `dupli1-manage-web`) deploy via OIDC role `github-actions-deploy-role`.

## Local development

Local development still uses Docker Compose. See the root `README.md`. For a single-box EC2 alternative, see [deployment-ec2.md](deployment-ec2.md).
