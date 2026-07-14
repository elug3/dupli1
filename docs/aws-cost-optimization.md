# AWS cost optimization review (2026-07-14)

Live Cost Explorer + resource inventory for account `845061289093` (`us-east-1` production). All figures are **unblended** USD.

## Current spend snapshot

| Period | Approx total | Notes |
|--------|--------------|-------|
| June 2026 (full month) | **~$52** | Stack mostly paused; idle NAT/ALB/GA still billed |
| July 1–13 (MTD, estimated) | **~$119** | Fargate early month + EC2 ASG resumed mid-month |
| Run-rate if left as-is (6×`t3.large` 24/7) | **~$350–420/mo** | Matches prior Terraform estimate |

### July MTD by service (top line items)

| Service | July 1–13 | Driver |
|---------|-----------|--------|
| EC2 Compute | ~$25 | 6×`t3.large` ECS ASG + VPN + Sydney micros |
| ECS (Fargate) | ~$22 | **Stopped 2026-07-03** — historical only |
| EC2 Other | ~$20 | NAT hours, EBS, Public IPv4 |
| Global Accelerator | ~$15 | **2 idle accelerators** (~$0.025/hr each) |
| VPC | ~$15 | NAT Gateway hours |
| ELB | ~$4 | ALB |
| RDS | ~$5 | `dupli1-production` + stopped `dupli1-ec2` storage |
| Tax / other | ~$13 | — |

Daily total after Fargate cutover (Jul 8–10 pause-ish): ~$3/day. With full EC2 ASG (Jul 12–13): **~$7–9/day**.

## Live inventory (what is actually running)

| Resource | State | Cost impact |
|----------|-------|-------------|
| ECS ASG `dupli1-production-ecs-asg` | **desired=6, min=5, max=6** × `t3.large` | Largest bill (~$60/instance-month) |
| ECS services (11) | All **EC2** launch type, desired=1 | CPU/mem mostly idle on each host |
| NAT Gateway | 1× available | ~$32/mo + data |
| ALB `dupli1-production-alb` | active (2 AZs → 2 public IPv4) | ~$16–22/mo + ~$7/mo IPv4 |
| RDS `dupli1-production` | `db.t3.micro` available | ~$13–17/mo |
| RDS `dupli1-ec2` | **stopped** (20 GB gp3) | Storage + auto-restart after 7 days |
| `dupli1-vpn` | `t3.micro` + EIP | **Stopped 2026-07-14**; EIP disassociated (~$8–12/mo saved). Admin: `https://manage.dupli1.com` |
| Global Accelerator `MyAcc`, `MyAccelerator` | **enabled, empty endpoint groups** | **~$36/mo fixed** — no traffic |
| `ap-southeast-2`: `schick-test`, `mweb-vpn` | running `t3.micro` + public IPs | ~$15–25/mo |

### awsvpc trunking status

Container instances report `ecs.capability.task-eni-trunking`. Eleven awsvpc/bridge tasks are spread across **six** hosts with large spare CPU/memory (typical remaining ~1.2–1.9 vCPU and 5–7 GB RAM per host).

**ENI packing is no longer the bottleneck.** ASG `min=5` is leftover from the pre-trunking sizing and is the main controllable waste.

Rough capacity check for current task sizes (~3.3 vCPU / ~6.5 GB requested total, dominated by `dupli1-manage-web` at 1 vCPU / 2 GB): **2×`t3.large` is enough**; 1× is tight because of `manage-web`.

## Prioritized actions

### P0 — delete / stop pure waste (no Dupli1 traffic dependency)

| Action | Est. monthly save | How |
|--------|-------------------|-----|
| Delete both Global Accelerators (`MyAcc`, `MyAccelerator`) | **~$36** | `bash infra/scripts/cleanup-aws-orphans.sh` (or console). Endpoints are empty. |
| Stop or terminate Sydney `schick-test` + `mweb-vpn` if unused | **~$15–25** | Same script (opt-in). Confirm ownership first. |
| Snapshot + delete stopped RDS `dupli1-ec2` | **~$2–15** (avoids storage + surprise restart) | Same script (opt-in). |

### P1 — shrink ECS ASG (largest Dupli1 saving)

| Action | Est. monthly save | How |
|--------|-------------------|-----|
| Set ASG min=1, desired=2, max=4 | **~$240–300** vs 5–6×`t3.large` | Terraform defaults updated in this PR; apply or: `aws autoscaling update-auto-scaling-group --auto-scaling-group-name dupli1-production-ecs-asg --min-size 1 --desired-capacity 2 --max-size 4` |
| Keep managed scaling; verify task placement after shrink | — | `aws ecs list-tasks --cluster production` / service events |

Projected **Dupli1-only** steady state after P0+P1 (24/7):

| Item | Estimate |
|------|----------|
| 2×`t3.large` + 40 GB gp3 | ~$125–135 |
| NAT Gateway | ~$32 + data |
| ALB + public IPv4 | ~$22–30 |
| RDS `db.t3.micro` | ~$15 |
| VPN `t3.micro` (optional) | ~$10 |
| ECR / S3 / Secrets / logs | ~$5–10 |
| **Total** | **~$210–230/mo** (without VPN/Sydney/GA) |

### P2 — operational hygiene

| Action | Why |
|--------|-----|
| Include `dupli1-cart` / `dupli1-payment` in pause/resume scripts | Pause left those services running (fixed in this PR). |
| Prefer `DELETE_NAT=1` when pausing > a few days | NAT alone is ~$32/mo while idle. |
| Pause when not demoing | `bash infra/scripts/pause-aws.sh` — ALB still bills unless deleted. |
| Single-box Compose path | [deployment-ec2.md](deployment-ec2.md) if managed ECS/ALB/NAT is not needed. |

### P3 — further optimizations (optional)

| Idea | Trade-off |
|------|-----------|
| Right-size to `t3.medium` after shrink | Less headroom for `manage-web` (1 vCPU task). |
| Move Redis to ElastiCache / NATS off ECS | More ops cost; only if reliability needs it. |
| VPC endpoints (ECR/Logs/Secrets) instead of NAT | Can cut NAT data + sometimes NAT itself; endpoint hourly fees apply. |
| Spot / Graviton ECS instances | Extra ASG complexity. |
| CloudFront in front of ALB | Only if latency/CDN matters; not a large save at low traffic. |

## What not to cut blindly

- **ALB** — required for `dupli1.com` HTTPS unless replaced by the single-EC2 path.
- **NAT** — private ECS tasks need outbound to ECR/Secrets/Logs (or VPC endpoints).
- **RDS `dupli1-production`** — app databases.
- **Route53 / ACM** — negligible cost.

## Verification commands

```bash
# Cost by service (replace dates)
aws ce get-cost-and-usage \
  --time-period Start=2026-07-01,End=2026-07-14 \
  --granularity MONTHLY --metrics UnblendedCost \
  --group-by Type=DIMENSION,Key=SERVICE

# ASG size
aws autoscaling describe-auto-scaling-groups \
  --auto-scaling-group-names dupli1-production-ecs-asg \
  --query 'AutoScalingGroups[0].{min:MinSize,desired:DesiredCapacity,max:MaxSize,instances:length(Instances)}'

# Confirm no Fargate tasks remain
aws ecs list-tasks --cluster production --desired-status RUNNING \
  --query 'taskArns' --output text
# then describe-tasks → launchType should be EC2
```

## Related docs

- [aws-cost-reduction-plan.md](aws-cost-reduction-plan.md) — phased cut plan (targets, commands, checklist)
- [deployment-aws.md](deployment-aws.md) — production architecture
- [infra/terraform/README.md](../infra/terraform/README.md) — IaC + pause/resume
- [deployment-ec2.md](deployment-ec2.md) — lowest-cost single-instance alternative
