#!/usr/bin/env bash
# Pause Dupli1 production AWS compute to cut idle spend.
#
# Stops billing for:
#   - ECS tasks (desiredCount → 0)
#   - ECS EC2 capacity (ASG min/desired → 0)
#   - RDS instance hours (stop-db-instance)
#
# Still bills while paused (delete separately if you need zero idle cost):
#   - ALB (~$16–22/mo)
#   - NAT Gateway (~$32/mo + data)
#   - RDS storage, ECR, Secrets Manager, S3
#
# Optional: DELETE_NAT=1 also deletes the NAT Gateway (+ EIP) for ~$32/mo more
# savings. Resume with: APPLY_NAT=1 bash infra/scripts/resume-aws.sh
#
# Usage:
#   bash infra/scripts/pause-aws.sh
#   DELETE_NAT=1 bash infra/scripts/pause-aws.sh
set -euo pipefail

AWS_REGION="${AWS_REGION:-us-east-1}"
CLUSTER="${ECS_CLUSTER:-production}"
RDS_INSTANCE="${RDS_INSTANCE:-dupli1-production}"
ASG_NAME="${ECS_ASG_NAME:-dupli1-production-ecs-asg}"
NAT_GATEWAY_ID="${NAT_GATEWAY_ID:-}"
DELETE_NAT="${DELETE_NAT:-0}"
VPN_INSTANCE_ID="${VPN_INSTANCE_ID:-}"

SERVICES=(
  dupli1-auth
  dupli1-product
  dupli1-order
  dupli1-notification
  dupli1-proxy
  dupli1-redis
  dupli1-nats
  # Legacy / frontend services (no-op if missing)
  dupli1-web
  dupli1-manage-web
  dupli1-inventory
)

log() { echo "$*"; }

scale_service_to_zero() {
  local svc="$1"
  if ! aws ecs describe-services \
    --region "$AWS_REGION" \
    --cluster "$CLUSTER" \
    --services "$svc" \
    --query 'services[?status==`ACTIVE`].serviceName' \
    --output text 2>/dev/null | grep -qx "$svc"; then
    log "  $svc not active (skip)"
    return 0
  fi
  aws ecs update-service \
    --region "$AWS_REGION" \
    --cluster "$CLUSTER" \
    --service "$svc" \
    --desired-count 0 \
    --no-cli-pager >/dev/null
  log "  $svc -> desiredCount=0"
}

log "Scaling ECS services in cluster $CLUSTER to 0..."
for svc in "${SERVICES[@]}"; do
  scale_service_to_zero "$svc"
done

log "Scaling ECS ASG $ASG_NAME to 0 (stops EC2 instances)..."
if aws autoscaling describe-auto-scaling-groups \
  --region "$AWS_REGION" \
  --auto-scaling-group-names "$ASG_NAME" \
  --query 'AutoScalingGroups[0].AutoScalingGroupName' \
  --output text 2>/dev/null | grep -qx "$ASG_NAME"; then
  aws autoscaling update-auto-scaling-group \
    --region "$AWS_REGION" \
    --auto-scaling-group-name "$ASG_NAME" \
    --min-size 0 \
    --desired-capacity 0 \
    --no-cli-pager
  log "  $ASG_NAME -> min=0 desired=0"
else
  log "  ASG $ASG_NAME not found (skip)"
fi

log "Stopping RDS instance $RDS_INSTANCE..."
RDS_STATUS="$(aws rds describe-db-instances \
  --region "$AWS_REGION" \
  --db-instance-identifier "$RDS_INSTANCE" \
  --query 'DBInstances[0].DBInstanceStatus' \
  --output text 2>/dev/null || echo missing)"
case "$RDS_STATUS" in
  available)
    aws rds stop-db-instance \
      --region "$AWS_REGION" \
      --db-instance-identifier "$RDS_INSTANCE" \
      --no-cli-pager >/dev/null
    log "  RDS stop requested (status was available)"
    ;;
  stopping|stopped)
    log "  RDS already $RDS_STATUS"
    ;;
  missing|None)
    log "  RDS $RDS_INSTANCE not found (skip)"
    ;;
  *)
    log "  RDS status=$RDS_STATUS — skip stop (retry when available)"
    ;;
esac

if [[ -n "$VPN_INSTANCE_ID" ]]; then
  log "Stopping VPN EC2 $VPN_INSTANCE_ID..."
  if aws ec2 stop-instances \
    --region "$AWS_REGION" \
    --instance-ids "$VPN_INSTANCE_ID" \
    --no-cli-pager >/dev/null 2>&1; then
    log "  VPN stop requested"
  else
    log "  VPN instance not running or missing (skip)"
  fi
fi

if [[ "$DELETE_NAT" == "1" ]]; then
  if [[ -z "$NAT_GATEWAY_ID" ]]; then
    NAT_GATEWAY_ID="$(aws ec2 describe-nat-gateways \
      --region "$AWS_REGION" \
      --filter "Name=tag:Name,Values=dupli1-production-nat" "Name=state,Values=available" \
      --query 'NatGateways[0].NatGatewayId' \
      --output text 2>/dev/null || true)"
  fi
  if [[ -n "$NAT_GATEWAY_ID" && "$NAT_GATEWAY_ID" != "None" ]]; then
    log "Deleting NAT Gateway $NAT_GATEWAY_ID (DELETE_NAT=1)..."
    EIP_ALLOC="$(aws ec2 describe-nat-gateways \
      --region "$AWS_REGION" \
      --nat-gateway-ids "$NAT_GATEWAY_ID" \
      --query 'NatGateways[0].NatGatewayAddresses[0].AllocationId' \
      --output text 2>/dev/null || true)"
    aws ec2 delete-nat-gateway \
      --region "$AWS_REGION" \
      --nat-gateway-id "$NAT_GATEWAY_ID" \
      --no-cli-pager >/dev/null
    log "  NAT delete requested"
    if [[ -n "$EIP_ALLOC" && "$EIP_ALLOC" != "None" ]]; then
      log "  Waiting for NAT to release EIP $EIP_ALLOC..."
      for _ in $(seq 1 60); do
        STATE="$(aws ec2 describe-nat-gateways \
          --region "$AWS_REGION" \
          --nat-gateway-ids "$NAT_GATEWAY_ID" \
          --query 'NatGateways[0].State' \
          --output text 2>/dev/null || echo deleted)"
        [[ "$STATE" == "deleted" || "$STATE" == "None" ]] && break
        sleep 10
      done
      aws ec2 release-address \
        --region "$AWS_REGION" \
        --allocation-id "$EIP_ALLOC" \
        --no-cli-pager >/dev/null 2>&1 \
        && log "  released EIP $EIP_ALLOC" \
        || log "  EIP $EIP_ALLOC still associated or already released"
    fi
    log "  Resume with: APPLY_NAT=1 bash infra/scripts/resume-aws.sh"
  else
    log "  No available NAT Gateway found (skip)"
  fi
fi

echo ""
echo "Pause initiated."
echo "Stopped / scaled down:"
echo "  - ECS services (desiredCount=0)"
echo "  - ECS ASG $ASG_NAME (no EC2 instances)"
echo "  - RDS $RDS_INSTANCE (stop in progress)"
if [[ "$DELETE_NAT" == "1" ]]; then
  echo "  - NAT Gateway (deleted)"
fi
echo ""
echo "Still billable while paused:"
echo "  - ALB dupli1-production-alb (~\$16–22/mo)"
if [[ "$DELETE_NAT" != "1" ]]; then
  echo "  - NAT Gateway (~\$32/mo) — re-run with DELETE_NAT=1 to remove"
fi
echo "  - RDS storage, ECR, Secrets Manager, S3"
echo ""
echo "Notes:"
echo "  - RDS auto-restarts after 7 days while stopped."
echo "  - GitHub Actions deploys on main may scale ECS back up."
echo "  - Resume: bash infra/scripts/resume-aws.sh"
