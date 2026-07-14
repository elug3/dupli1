# AWS cost reduction plan

Action plan to cut Dupli1 AWS spend from the **~$350–420/mo** run-rate (6×`t3.large` + idle orphans) down to **~$210–230/mo** core, or lower when paused.

Companion review with inventory and Cost Explorer detail: [aws-cost-optimization.md](aws-cost-optimization.md).

## Target outcomes

| Mode | Monthly estimate | When to use |
|------|------------------|-------------|
| **A — Steady production (shrunk)** | **~$210–230** | Site must stay up 24/7 |
| **B — Demo / idle (paused)** | **~$50–70** | ALB + NAT still up; compute/RDS stopped |
| **C — Deep idle** | **~$20–40** | Mode B + `DELETE_NAT=1`; slower resume |
| **D — Single EC2 Compose** | **~$30–60** | Accept [deployment-ec2.md](deployment-ec2.md) trade-offs |

Do **not** delete ALB, Route53, or `dupli1-production` RDS unless moving to Mode D.

---

## Phase 0 — Baseline (before changing anything)

```bash
export AWS_REGION=us-east-1

# ASG size
aws autoscaling describe-auto-scaling-groups \
  --auto-scaling-group-names dupli1-production-ecs-asg \
  --query 'AutoScalingGroups[0].{min:MinSize,desired:DesiredCapacity,max:MaxSize,n:length(Instances)}'

# Running tasks (expect launchType=EC2)
aws ecs list-services --cluster production --query 'serviceArns' --output text

# Idle Global Accelerators
aws globalaccelerator list-accelerators --region us-west-2 \
  --query 'Accelerators[].{name:Name,enabled:Enabled}'

# Optional: last complete month by service
aws ce get-cost-and-usage \
  --time-period Start=2026-06-01,End=2026-07-01 \
  --granularity MONTHLY --metrics UnblendedCost \
  --group-by Type=DIMENSION,Key=SERVICE
```

Record ASG size and daily burn (~$7–9/day with 6 instances) so you can confirm savings after Phase 1–2.

---

## Phase 1 — Delete pure waste (same day, low risk)

**Est. save: ~$50–60/mo** · No Dupli1 traffic dependency.

| Step | Action | Save |
|------|--------|------|
| 1.1 | Delete empty Global Accelerators `MyAcc`, `MyAccelerator` | ~$36/mo |
| 1.2 | Confirm Sydney `schick-test` / `mweb-vpn` unused → stop (then terminate later) | ~$15–25/mo |
| 1.3 | Snapshot + delete stopped RDS `dupli1-ec2` | storage + avoids 7-day restart |

```bash
# Dry-run first
DELETE_GA=1 STOP_SYDNEY=1 DELETE_RDS_EC2=1 bash infra/scripts/cleanup-aws-orphans.sh

# Apply
APPLY=1 DELETE_GA=1 bash infra/scripts/cleanup-aws-orphans.sh

# Only after confirming Sydney VMs are disposable:
APPLY=1 STOP_SYDNEY=1 bash infra/scripts/cleanup-aws-orphans.sh

# Only after confirming dupli1-ec2 has no needed data:
APPLY=1 DELETE_RDS_EC2=1 bash infra/scripts/cleanup-aws-orphans.sh
```

**Done when:** `list-accelerators` is empty (or disabled/gone); Sydney instances stopped if flagged; `dupli1-ec2` absent or deleting.

---

## Phase 2 — Shrink ECS ASG (largest Dupli1 saving)

**Est. save: ~$240–300/mo** · Trunking already packs 11 tasks; 2×`t3.large` is enough (`manage-web` needs ~1 vCPU).

| Step | Action |
|------|--------|
| 2.1 | Set ASG **min=1, desired=2, max=4** |
| 2.2 | Wait for 4 instances to terminate; confirm all services `runningCount=1` |
| 2.3 | Smoke-test `https://dupli1.com` gateway health + login + catalog |
| 2.4 | Apply Terraform so next `terraform apply` does not scale back up |

```bash
# Live shrink (or use Terraform apply with updated defaults)
APPLY=1 SHRINK_ASG=1 bash infra/scripts/cleanup-aws-orphans.sh

# Watch placement
watch -n 15 'aws ecs describe-services --cluster production \
  --services dupli1-auth dupli1-product dupli1-order dupli1-cart dupli1-payment \
  dupli1-notification dupli1-proxy dupli1-web dupli1-manage-web dupli1-redis dupli1-nats \
  --query "services[].{n:serviceName,r:runningCount,d:desiredCount,e:events[0].message}"'
```

Terraform defaults in-repo are already **2 / 1 / 4** (`infra/terraform/variables.tf`). After shrink:

```bash
cd infra/terraform && terraform plan   # expect ASG size match, not a scale-up
```

**Done when:** 2 ECS instances; all 11 services healthy; public site OK for ~30 minutes.

**Rollback:** `ASG_DESIRED=5 ASG_MIN=5 ASG_MAX=6 APPLY=1 SHRINK_ASG=1 bash infra/scripts/cleanup-aws-orphans.sh`

---

## Phase 3 — Operating rules (keep the saving)

| Rule | Command / habit |
|------|-----------------|
| Pause when not demoing | `bash infra/scripts/pause-aws.sh` |
| Multi-day pause | `DELETE_NAT=1 bash infra/scripts/pause-aws.sh` |
| Resume | `bash infra/scripts/resume-aws.sh` (use `APPLY_NAT=1` if NAT was deleted) |
| Never leave ASG min ≥ 5 | Trunking removes the old ENI reason for 5 hosts |
| Prefer OIDC for CI keys | Avoid long-lived access keys (see TODO) |

Pause/resume now include **cart** and **payment**.

**Done when:** team uses pause for idle weeks; monthly bill reflects Mode A or B, not 6× large.

---

## Phase 4 — Optional further cuts

Only if Mode A is still too high:

| Option | Extra save | Trade-off |
|--------|------------|-----------|
| Stop `dupli1-vpn` when unused | ~$8–12/mo | No VPN admin path until started |
| VPC endpoints (ECR/Logs/Secrets) ± drop NAT | up to ~$32/mo NAT | Endpoint hourly fees; more Terraform |
| Right-size to `t3.medium` after Phase 2 | ~$30–40/mo | Tight for `manage-web` (1 vCPU) |
| Move to single EC2 Compose (Mode D) | large (drop ALB/NAT/RDS) | Ops model change — [deployment-ec2.md](deployment-ec2.md) |

---

## Expected burn after each phase

```text
Today (6× t3.large + GA + Sydney)     ~$350–420/mo
After Phase 1 (drop GA/orphans)       ~$300–360/mo
After Phase 2 (2× t3.large)           ~$210–230/mo   ← primary goal
After Phase 3 (paused most days)      ~$50–70/mo average if often idle
```

Verify with Cost Explorer 3–5 days after Phase 2 (`EC2 - Compute` and `EC2 - Other` should fall; `Global Accelerator` → $0).

---

## Checklist

- [ ] Phase 0 baseline captured
- [ ] Phase 1.1 Global Accelerators deleted
- [ ] Phase 1.2 Sydney stopped (if unused)
- [ ] Phase 1.3 `dupli1-ec2` deleted (if unused)
- [ ] Phase 2 ASG at 2/1/4; site healthy
- [ ] Phase 2 Terraform plan does not scale ASG up
- [ ] Phase 3 pause/resume documented for operators
- [ ] Phase 4 decided (defer / VPN / endpoints / EC2 Compose)

## Owners / scripts

| Artifact | Role |
|----------|------|
| `infra/scripts/cleanup-aws-orphans.sh` | Phase 1–2 opt-in actions |
| `infra/scripts/pause-aws.sh` / `resume-aws.sh` | Phase 3 idle control |
| `infra/terraform/` | Persistent ASG sizing |
| [aws-cost-optimization.md](aws-cost-optimization.md) | Why / evidence |
