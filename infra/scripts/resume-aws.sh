#!/usr/bin/env bash
# Resume Dupli1 production AWS compute after a pause.
# Starts RDS and EC2 VPN, then scales ECS services back to 1 task each.
set -euo pipefail

AWS_REGION="${AWS_REGION:-us-east-1}"
CLUSTER="${ECS_CLUSTER:-production}"
RDS_INSTANCE="${RDS_INSTANCE:-dupli1-production}"
VPN_INSTANCE_ID="${VPN_INSTANCE_ID:-i-0f7a516c42a8b7afd}"
DESIRED_COUNT="${DESIRED_COUNT:-1}"

SERVICES=(
  dupli1-web
  dupli1-notification
  dupli1-manage-web
  dupli1-proxy
  dupli1-product
  dupli1-auth
  dupli1-order
)

echo "Starting RDS instance $RDS_INSTANCE..."
aws rds start-db-instance \
  --region "$AWS_REGION" \
  --db-instance-identifier "$RDS_INSTANCE" \
  --no-cli-pager >/dev/null
echo "  RDS start requested (wait until status=available before traffic)"

echo "Starting EC2 VPN instance $VPN_INSTANCE_ID..."
aws ec2 start-instances \
  --region "$AWS_REGION" \
  --instance-ids "$VPN_INSTANCE_ID" \
  --no-cli-pager >/dev/null
echo "  EC2 start requested"

echo "Scaling ECS services in cluster $CLUSTER to $DESIRED_COUNT..."
for svc in "${SERVICES[@]}"; do
  aws ecs update-service \
    --region "$AWS_REGION" \
    --cluster "$CLUSTER" \
    --service "$svc" \
    --desired-count "$DESIRED_COUNT" \
    --no-cli-pager >/dev/null
  echo "  $svc -> desiredCount=$DESIRED_COUNT"
done

echo ""
echo "Resume initiated. Wait for RDS to reach 'available' before relying on the API."
