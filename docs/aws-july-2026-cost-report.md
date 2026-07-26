# AWS cost report — July 2026

Account `845061289093` · Region primarily `us-east-1` · Figures are **unblended** USD from Cost Explorer (queried **2026-07-26**). July 26 daily data was still `$0` (CE lag); totals below cover **Jul 1–25** unless noted.

Companion docs: [aws-cost-optimization.md](aws-cost-optimization.md) (mid-month review), [aws-cost-reduction-plan.md](aws-cost-reduction-plan.md) (phased cut plan — **mostly not yet applied**).

---

## Executive summary

| Metric | Amount |
|--------|--------|
| **June 2026 (full month)** | **~$52** |
| **July 1–25 (MTD)** | **~$365** |
| **AWS Cost Explorer forecast (full July)** | **~$474** |
| Steady daily burn (Jul 18–24) | **~$18.50/day** |
| June → July change | **~7–9× higher** |

**Why so expensive?** The production stack ran nearly full-time on an **oversized ECS ASG (6× `t3.large`)** after early-July Fargate spend, while **idle Global Accelerators**, **Sydney test VMs**, a **second RDS**, **NAT**, and **ALB/IPv4** kept billing every hour. June was cheap because compute was mostly paused; July is the bill for “always on + waste left in place.”

**Can it be saved?** Yes. The in-repo cut plan already targets **~$210–230/mo** for a healthy 24/7 Dupli1 core (2× `t3.large`), or **~$50–70/mo** when paused. None of the large Phase 1–2 actions (delete GA, shrink ASG) have been applied yet — live inventory on 2026-07-26 still shows **ASG 5/6/6**, **2 empty Global Accelerators**, and **Sydney VMs running**.

---

## Month comparison

| Period | Total | Character |
|--------|-------|-----------|
| May 2026 | ~$0 | Little/no usage in CE |
| June 2026 | **$51.92** | Stack mostly idle; NAT + GA + ALB still billed |
| July 1–25 | **$364.63** | Fargate early month → EC2 ASG at scale mid/late month |
| July forecast (full month) | **~$474** | Extrapolates ~$18.50/day for remaining days |

### June vs July top services

| Service | June | July MTD | What changed |
|---------|------|----------|--------------|
| EC2 Compute | $5.34 | **$175.76** | 6× `t3.large` + Sydney micros + early Fargate-era spill |
| EC2 - Other | $1.47 | **$41.69** | EBS, Public IPv4 (incl. GA / ALB / EIPs) |
| Tax | $1.22 | **$33.14** | Scales with spend |
| Global Accelerator | $12.20 | **$29.85** | Two idle accelerators × full month |
| VPC | $19.63 | **$29.52** | NAT hours (plus related) |
| ECS (Fargate line) | — | **$22.14** | Early July only (cut over ~Jul 3) |
| RDS | $4.77 | **$16.81** | Production + `dupli1-ec2` (auto-restarted) |
| ELB | $5.75 | **$10.76** | ALB hours |

---

## July daily burn

| Dates | $/day | What was happening |
|-------|-------|--------------------|
| Jul 1 | **$50.09** | Peak day — Fargate still heavy + compute ramp |
| Jul 2–3 | $12–17 | Fargate tapering off |
| Jul 4–7 | $6–9 | Mixed / partial |
| Jul 8–10 | **~$3** | Pause-ish floor (idle NAT/ALB/GA) |
| Jul 11–12 | $5–9 | Ramping ASG back up |
| Jul 13–24 | **$17–18.50** | Full 6× `t3.large` steady state |
| Jul 25 | $15.02 | Slight dip (CE estimate) |
| Jul 26 | $0.00* | Incomplete in CE at query time |

\*Do not treat Jul 26 as a real zero; forecast still assumes continued spend.

---

## Where the money went (July MTD by usage type)

| Cost | Usage type | Plain English |
|------|------------|---------------|
| **$157** | `BoxUsage:t3.large` (~1,889 hrs) | **6 ECS hosts** — largest line item |
| **$33** | Tax | — |
| **$30** | Global Accelerator fixed fee | **2 idle accelerators** (~$0.025/hr each) |
| **$17** | Fargate vCPU-hours | Early July only |
| **$17 + $14** | NAT Gateway hours (USE1 + Regional) | Always-on NAT |
| **$16** | `APS2-BoxUsage:t3.micro` | Sydney `schick-test` + `mweb-vpn` |
| **$13** | RDS `db.t3.micro` hours | 2 instances |
| **$12** | Public IPv4 in `us-west-2` | Tied to Global Accelerator |
| **$11** | ALB hours | `dupli1-production-alb` |
| **$9** | Public IPv4 in `us-east-1` | ALB / NAT / EIPs |
| **$6** | Public IPv4 in Sydney | Test VMs |
| **$5** | Fargate GB-hours | Early July |
| **$9** | EBS gp3 | Root volumes for EC2 fleet |
| **$2** | Idle Public IPv4 | Includes stopped VPN EIP |

### By region

| Region | July MTD | Notes |
|--------|----------|-------|
| `us-east-1` | ~$265 | Dupli1 production |
| NoRegion (tax) | ~$33 | — |
| `global` | ~$32 | Mostly Global Accelerator |
| `ap-southeast-2` | ~$23 | Unrelated/test VMs |
| `us-west-2` | ~$12 | GA Public IPv4 |

---

## Live inventory (2026-07-26) — still oversized

| Resource | State | Cost impact |
|----------|-------|-------------|
| ASG `dupli1-production-ecs-asg` | **min=5, desired=6, max=6** × **`t3.large`** | Dominant bill |
| ECS services (11) | All desired/running=1 on EC2 capacity provider | Tasks fit on far fewer hosts |
| Cluster utilization | **~25% CPU / ~13% memory** across 6 instances | Clear over-provisioning |
| Global Accelerator `MyAcc`, `MyAccelerator` | Enabled; **0 endpoints** each | Pure waste (~$36/mo + IPv4) |
| NAT Gateway | 1× available | ~$32/mo + data |
| ALB | Active, 2 AZs | ~$16–22/mo + IPv4 |
| RDS `dupli1-production` | `db.t3.micro` available | Needed |
| RDS `dupli1-ec2` | **available** (was stopped mid-month; auto-restarted) | Avoidable |
| `dupli1-vpn` | Stopped `t3.micro`; **EIP still allocated** | Idle IPv4 charge |
| Sydney `schick-test`, `mweb-vpn` | Running `t3.micro` | ~$15–25/mo |

Trunking is already enabled; packing is **not** why there are six hosts. ASG `min=5` is leftover sizing. Rough task demand (~3 vCPU / ~6 GB requested) fits on **2× `t3.large`**.

---

## Root causes (ranked)

1. **Oversized ECS ASG (6× `t3.large` 24/7)** — ~$157 MTD for BoxUsage alone; drives most of the ~$18.50/day steady burn. Planned shrink to 2 instances was never applied.
2. **Idle Global Accelerators** — ~$30 fixed fee + ~$12 Public IPv4; empty endpoint groups; unrelated to Dupli1 traffic.
3. **Always-on networking** — NAT (~$31 hours) + ALB + Public IPv4 even when traffic is low.
4. **Early-July Fargate** — ~$22 before EC2 cutover (historical; not recurring if stay on EC2).
5. **Orphan / non-prod compute** — Sydney micros (~$16+), second RDS, idle VPN EIP.
6. **Tax** — ~$33 on the inflated base.

June looked “cheap” because compute was largely paused; fixed costs (NAT/GA/ALB) still showed up. July is what the current architecture costs when left running.

---

## Can it be saved? Yes — concrete targets

From [aws-cost-reduction-plan.md](aws-cost-reduction-plan.md). Status as of this report:

| Mode | Monthly estimate | Status |
|------|------------------|--------|
| **Current path (no action)** | **~$450–480** (July run-rate) | Happening now |
| **A — Steady prod, shrunk ASG** | **~$210–230** | Not applied (ASG still 6) |
| **B — Paused (demo/idle)** | **~$50–70** | Used briefly Jul 8–10 (~$3/day) |
| **C — Deep idle (`DELETE_NAT=1`)** | **~$20–40** | Optional |
| **D — Single EC2 Compose** | **~$30–60** | Architecture change |

### Highest-ROI actions (still open)

| Priority | Action | Est. monthly save |
|----------|--------|-------------------|
| P0 | Delete empty Global Accelerators | **~$36** (+ IPv4) |
| P0 | Stop/terminate Sydney VMs if unused | **~$15–25** |
| P0 | Snapshot + delete RDS `dupli1-ec2`; release VPN EIP | **~$5–15** |
| **P1** | **ASG → min=1, desired=2, max=4** | **~$240–300** vs 6× large |
| P2 | Pause when not demoing; `DELETE_NAT=1` for multi-day idle | Large average-bill cut |
| P3 | Optional: `t3.medium`, VPC endpoints vs NAT, Mode D | Extra |

**Primary lever:** shrink the ASG. Cluster CPU/mem utilization (~25% / ~13%) shows four of six hosts are unnecessary for current tasks.

```bash
# Phase 1–2 helpers (dry-run first)
DELETE_GA=1 STOP_SYDNEY=1 DELETE_RDS_EC2=1 bash infra/scripts/cleanup-aws-orphans.sh
APPLY=1 SHRINK_ASG=1 bash infra/scripts/cleanup-aws-orphans.sh

# Idle weeks
bash infra/scripts/pause-aws.sh
```

### What not to cut blindly

- **ALB** — needed for `dupli1.com` HTTPS (unless Mode D).
- **NAT** — private ECS egress (or replace with VPC endpoints).
- **RDS `dupli1-production`** — app databases.
- **Route53 / ACM** — negligible.

---

## Verification (re-run after cuts)

```bash
export AWS_REGION=us-east-1

aws ce get-cost-and-usage \
  --time-period Start=2026-07-01,End=2026-08-01 \
  --granularity DAILY --metrics UnblendedCost

aws autoscaling describe-auto-scaling-groups \
  --auto-scaling-group-names dupli1-production-ecs-asg \
  --query 'AutoScalingGroups[0].{min:MinSize,desired:DesiredCapacity,max:MaxSize,n:length(Instances)}'

aws globalaccelerator list-accelerators --region us-west-2
```

**Success signal:** daily burn falls from ~$18.50 toward **~$5–7** (Mode A) or **~$3** (paused with NAT/ALB still up).

---

## Actions taken 2026-07-26

| Action | Result |
|--------|--------|
| Deleted Global Accelerators `MyAcc`, `MyAccelerator` | Done (was ~$36/mo + IPv4) |
| Stopped Sydney `schick-test`, `mweb-vpn` | Done |
| Deleted RDS `dupli1-ec2` (final snapshot `dupli1-ec2-final-20260726`) | Done |
| Released idle `dupli1-vpn` EIP | Done |
| ASG shrink to 2× `t3.large` | **Rolled back** — hit `RESOURCE:ENI` |

### Why shrink-to-2 failed

Hosts advertise `ecs.capability.task-eni-trunking` and user-data sets `ECS_ENABLE_AWSVPC_TRUNKING=true`, but **no trunk ENIs attach**. `awsvpcTrunking` is enabled for IAM user `cursor-agent` (and we set the account default for `root`), yet container instances still get only the normal **3 ENIs / 2 awsvpc tasks** per `t3.large`. With **10 awsvpc** services (+ 1 bridge `dupli1-web`), the floor is **~5 hosts**, not 2.

ASG left at **min=5 / desired=5 / max=6** with all 11 services healthy and `https://dupli1.com` / manage / products API returning 200.

### Remaining unlock for ~$210–230/mo

An account admin (root) must ensure `awsvpcTrunking=enabled` applies to role `dupli1-production-ecs-instance`, **replace** ECS instances so each gets a trunk ENI, then shrink to 2× `t3.large`. Until then, do not set desired below 5.

---

## Bottom line

July is expensive because Dupli1 ran **6× `t3.large`** plus orphans. On 2026-07-26 we removed the orphans and one host (→ **5×**). Further cut to 2× needs **working ENI trunking** on the instance role; without it, shrink breaks awsvpc placement.
