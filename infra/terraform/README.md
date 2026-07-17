# Dupli1 AWS (ECS on EC2)

Terraform provisions the production compute path on the existing VPC and RDS:

| Resource | Purpose |
|----------|---------|
| NAT Gateway (1 AZ) | Outbound for private ECS tasks (ECR, Secrets Manager, Logs) |
| ALB | Public HTTP + HTTPS → storefront + `dupli1-proxy` |
| Route53 aliases | `dupli1.com` / `www` → ALB |
| EC2 ASG (`t3.large`, default 2) | ECS container instances (awsvpc trunking) |
| ECS capacity provider | EC2 launch type for backend services |
| S3 | Private product-image bucket |
| CloudFront + OAC | Public CDN for product images (`images.dupli1.com`) |
| CloudWatch Logs | `/ecs/dupli1-*` log groups |
| ECS services | auth, product, order, cart, payment, notification, proxy, web, manage-web, redis, nats |

Existing resources reused (not recreated): VPC `dupli1-prod-vpc`, ECS cluster `production`, RDS `dupli1-production`, ECR repos, Cloud Map `dupli1.local`, Secrets Manager DB URLs / JWT.

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

GitHub Actions (`.github/workflows/aws.yml`) builds and pushes to ECR (including `dupli1-cart` / `dupli1-payment`), then force-redeploys ECS services. Proxy uses `api/Dockerfile.ecs` (Cloud Map DNS).

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
