# Dupli1 AWS (ECS on EC2)

Terraform provisions the production compute path on the existing VPC and RDS:

| Resource | Purpose |
|----------|---------|
| NAT Gateway (1 AZ) | Outbound for private ECS tasks (ECR, Secrets Manager, Logs) |
| ALB | Public HTTP entry → `dupli1-proxy` |
| EC2 ASG (`t3.large`) | ECS container instances |
| ECS capacity provider | EC2 launch type for backend services |
| S3 | Product image bucket (public `GetObject`) |
| CloudWatch Logs | `/ecs/dupli1-*` log groups |
| ECS services | auth, product, order, notification, proxy, redis, nats |

Existing resources reused (not recreated): VPC `dupli1-prod-vpc`, ECS cluster `production`, RDS `dupli1-production`, ECR repos, Cloud Map `dupli1.local`, Secrets Manager DB URLs.

## Monthly cost (dev-sized, us-east-1, 24/7)

| Service | Estimate |
|---------|----------|
| EC2 t3.large | ~$60 |
| EBS 40 GB gp3 | ~$3 |
| NAT Gateway | ~$32 + data |
| ALB | ~$16–22 |
| RDS db.t3.micro + storage | ~$17 |
| ECR / S3 / CloudWatch / Secrets | ~$5 |
| **Total** | **~$130–140/mo** |

## Apply

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

GitHub Actions (`.github/workflows/aws.yml`) builds and pushes to ECR, then force-redeploys ECS services. Proxy uses `api/Dockerfile.ecs` (Cloud Map DNS).

## Gateway

After apply:

```bash
terraform output gateway_health_url
curl "$(terraform output -raw gateway_health_url)"
```
