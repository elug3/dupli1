# Single EC2 deployment

Run the full Dupli1 backend on one EC2 instance using Docker Compose. This replaces ECS Fargate, RDS, ALB, and NAT Gateway for lower monthly cost.

## Architecture

```text
Internet → EC2 (Elastic IP)
             └── docker compose
                   ├── dupli1-proxy (nginx :80/:443)
                   ├── dupli1-auth, product, order, cart, payment, notification
                   ├── postgres (single instance, multiple databases)
                   │     dupli1_db, products, orders, cart, payments, notifications
                   ├── redis, nats, minio
```

Frontends (`dupli1-web`, `dupli1-manage-web`) live in separate repositories. Add them as Compose services and extend `api/nginx.prod.conf` when ready.

Use [docker-compose.prod.yml](../docker-compose.prod.yml) (or the provisioned overlay on the instance) together with the main compose file. Prefer the multi-Postgres Compose layout from `docker-compose.yml` when bringing the full stack up locally; the single-EC2 path consolidates DBs on one Postgres with `infra/postgres/init-databases.sh`.

## Cost comparison

| Resource | ECS + RDS (paused compute) | Single EC2 |
|----------|---------------------------|------------|
| Fargate / ECS tasks | ~$80–150/mo when running | — |
| RDS db.t3.micro | ~$15/mo + storage | — |
| ALB | ~$16–22/mo | — |
| NAT Gateway | ~$32/mo | — |
| **EC2 t3.large** | — | ~$60/mo |
| **EBS 50 GB** | — | ~$4/mo |

After cutover, delete ALB, NAT Gateway, ECS services, and RDS to eliminate idle charges.

## Quick start

### 1. Provision EC2

From a machine with AWS CLI credentials:

```bash
export DUPLI1_BRANCH=main   # or your feature branch during testing
bash infra/scripts/provision-ec2.sh
```

This creates a `t3.large` Ubuntu 24.04 instance, security group (22/80/443), key pair, and attaches an available Elastic IP. Bootstrap runs automatically via user-data.

### 2. Configure secrets

SSH into the instance (see script output), then edit:

```bash
sudo nano /opt/dupli1/app/.env.prod
```

Set at minimum:

- `OWNER_PASSWORD`
- `DUPLI1_WEB_SERVICE_PASSWORD`
- `DUPLI1_ORDER_SERVICE_PASSWORD`
- `MINIO_SECRET_KEY`

`POSTGRES_PASSWORD` and `JWT_SECRET` are auto-generated on first bootstrap.

### 3. Deploy

```bash
bash /opt/dupli1/app/infra/scripts/deploy-ec2.sh
```

Verify: `curl http://<public-ip>:8080/gateway/health`

### 4. Migrate data from RDS (optional)

If you have production data on RDS:

```bash
bash /opt/dupli1/app/infra/scripts/migrate-rds-to-ec2.sh
```

This starts RDS if stopped, dumps application databases (`dupli1_db`, `products`, `orders`, `cart`, `payments`, and `notifications` when present), and restores into local Postgres.

Stop RDS again after migration:

```bash
aws rds stop-db-instance --db-instance-identifier dupli1-production
```

### 5. DNS cutover

Point your domain A record at the EC2 Elastic IP. Test API endpoints through the gateway.

### 6. Retire old AWS resources

Once validated:

```bash
# Already paused — now delete to stop ALB/NAT/RDS charges
bash infra/scripts/pause-aws.sh   # ensure ECS/RDS/VPN are stopped

# Manual cleanup in AWS Console or CLI:
# - Delete dupli1-prod-alb
# - Delete NAT Gateway
# - Delete RDS dupli1-production (after final snapshot)
```

For production ECS (not single-EC2), see [deployment-aws.md](deployment-aws.md).
