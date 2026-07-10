# Single EC2 deployment

Run the full Dupli1 backend on one EC2 instance using Docker Compose. This replaces ECS Fargate, RDS, ALB, and NAT Gateway for lower monthly cost.

## Architecture

```text
Internet → EC2 (Elastic IP)
             └── docker compose
                   ├── dupli1-proxy (nginx :80/:443)
                   ├── dupli1-auth, product, order, notification
                   ├── postgres (single instance, 3 databases)
                   ├── redis, nats, minio
```

Frontends (`dupli1-web`, `dupli1-manage-web`) live in separate repositories. Add them as Compose services and extend `api/nginx.prod.conf` when ready.

## Cost comparison

| Resource | ECS + RDS (paused compute) | Single EC2 |
|----------|---------------------------|------------|
| Fargate tasks (8 services) | ~$80–150/mo when running | — |
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

Verify: `curl http://<public-ip>/gateway/health`

### 4. Migrate data from RDS (optional)

If you have production data on RDS:

```bash
bash /opt/dupli1/app/infra/scripts/migrate-rds-to-ec2.sh
```

This starts RDS if stopped, dumps `dupli1_db` and `products`, and restores into local Postgres. Orders was not on RDS and starts empty.

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
# - Delete NAT Gateway nat-168d96b459ab0cf17
# - Delete RDS dupli1-production (after final snapshot)
# - Delete ECS cluster services / cluster
# - Disable .github/workflows/aws.yml ECS deploys
```

## Files

| File | Purpose |
|------|---------|
| `docker-compose.prod.yml` | Production overlay (single Postgres, no exposed internal ports) |
| `.env.prod.example` | Secret template |
| `api/nginx.prod.conf` | Gateway config without VPC Cloud Map DNS |
| `infra/scripts/provision-ec2.sh` | Launch EC2 + Elastic IP |
| `infra/scripts/ec2-bootstrap.sh` | Install Docker, clone repo, generate JWT key |
| `infra/scripts/deploy-ec2.sh` | `docker compose up` on EC2 |
| `infra/scripts/migrate-rds-to-ec2.sh` | RDS → local Postgres import |
| `infra/scripts/pause-aws.sh` | Scale down legacy ECS/RDS |

## Updating the app

On EC2:

```bash
bash /opt/dupli1/app/infra/scripts/deploy-ec2.sh
```

Or set up a GitHub Actions workflow that SSHes into EC2 and runs the same script.

## TLS

Ports 443 are exposed but TLS is not configured yet. Options:

- Terminate TLS at nginx using certs in `certs/`
- Use Caddy or Certbot on the host
- Terminate TLS at Cloudflare in front of the Elastic IP

## Troubleshooting

| Issue | Fix |
|-------|-----|
| Gateway unhealthy | `docker compose -f docker-compose.yml -f docker-compose.prod.yml logs dupli1-auth` |
| RDS migration fails | Ensure EC2 security group is allowed on RDS SG (provision script adds this) |
| Auth tokens invalid after restart | Confirm `jwt_private_key.pem` exists in `/opt/dupli1/secrets/` |
| Out of memory | Upgrade to `t3.xlarge` or reduce services |
