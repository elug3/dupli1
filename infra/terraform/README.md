# Dupli1 AWS (ECS on EC2)

Terraform provisions a full Dupli1 backend in `us-east-1`:

| Resource | Purpose |
|----------|---------|
| **VPC** | Public subnets (ALB + ECS EC2) + private subnets (RDS). No NAT Gateway (cost-conscious). |
| **ECR** | Repositories for auth, product, order, cart, payment, notification, proxy |
| **ECS + EC2** | Cluster with Auto Scaling Group capacity provider (`t3.large`) |
| **ALB** | Internet-facing load balancer → `dupli1-proxy` |
| **RDS** | PostgreSQL 16 (`db.t3.micro`), encrypted, private |
| **S3** | Product images bucket (public read, private write) |
| **CloudWatch Logs** | `/dupli1/<env>/<service>` log groups (14-day retention) |
| **Secrets Manager** | DB URLs, JWT, seeded account passwords, S3 keys |
| **Cloud Map** | Private DNS `*.dupli1.local` for inter-service calls |

NATS runs as an ECS service (public `nats:2-alpine` image). Redis is optional for auth and is not provisioned by default.

## Monthly cost estimate (dev-sized, us-east-1, ~730h)

| Component | Approx. |
|-----------|---------|
| EC2 `t3.large` ×1 | ~$60 |
| ALB | ~$16–22 |
| RDS `db.t3.micro` + 20 GB gp3 | ~$15–18 |
| S3 + ECR + Secrets Manager + CloudWatch Logs | ~$5–10 |
| Data transfer | variable |
| **Total** | **~$100–120 / month** |

Not included: NAT Gateway (~$32/mo), Multi-AZ RDS, ElastiCache, ACM is free. Scale ASG or instance size up for production load.

## Prerequisites

- Terraform >= 1.5
- AWS credentials with rights to create VPC, ECS, EC2, ECR, ALB, RDS, S3, IAM, Secrets Manager, CloudWatch, Cloud Map
- Docker (to build/push images) or GitHub Actions secrets configured

## Apply

```bash
cd infra/terraform
cp terraform.tfvars.example terraform.tfvars
# edit owner_password / certificate_arn if desired
terraform init
terraform plan
terraform apply
```

After apply:

```bash
# 1. Create extra Postgres databases (products, orders, cart, payments)
#    Requires network path to RDS (VPN / bastion / SSM port-forward).
bash infra/scripts/create-rds-databases.sh

# 2. Push images (or merge to main so .github/workflows/aws.yml does it)
# 3. Force ECS redeploy if services started before images existed:
aws ecs update-service --cluster production --service dupli1-proxy --force-new-deployment
```

Gateway health:

```bash
curl http://$(terraform -chdir=infra/terraform output -raw alb_dns_name)/gateway/health
```

## GitHub Actions

Set repository secrets/variables:

| Type | Name | Value |
|------|------|-------|
| Secret | `AWS_ACCESS_KEY_ID` | deploy user |
| Secret | `AWS_SECRET_ACCESS_KEY` | deploy user |
| Variable | `AWS_REGION` | `us-east-1` |
| Variable | `ECS_CLUSTER` | `terraform output -raw ecs_cluster_name` (usually `production`) |

On push to `main`, the workflow builds all service images (including cart/payment) into ECR and force-redeploys ECS services.

## Security defaults

- RDS encrypted at rest, not publicly accessible, SG allows 5432 from ECS only
- ALB drops invalid headers; HTTPS when `certificate_arn` is set
- S3 bucket versioning + SSE-S3; writes via IAM user keys in Secrets Manager
- ECS task execution role least-privilege to Secrets Manager ARNs
- Container Insights off by default (`enable_container_insights`)

## Legacy notes

Older scripts under `infra/scripts/` (pause/resume, single-EC2 Compose, RDS cutover helpers) target a previous hand-built environment. Prefer this Terraform stack for new deploys.
