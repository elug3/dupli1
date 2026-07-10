#!/usr/bin/env bash
# Pause Dupli1 production AWS compute to reduce costs during migration.
# Scales ECS services to 0, stops RDS, and stops the internal VPN EC2 instance.
#
# Does NOT delete or stop: ALB, NAT Gateway, ECR, Secrets Manager, or VPC resources.
# Those continue to incur charges until removed manually.
set -euo pipefail

AWS_REGION="${AWS_REGION:-us-east-1}"
CLUSTER="${ECS_CLUSTER:-production}"
RDS_INSTANCE="${RDS_INSTANCE:-dupli1-production}"
VPN_INSTANCE_ID="${VPN_INSTANCE_ID:-i-0f7a516c42a8b7afd}"

SERVICES=(
  dupli1-web
  dupli1-notification
  dupli1-manage-web
  dupli1-proxy
  dupli1-product
  dupli1-auth
  dupli1-order
)

echo "Scaling ECS services in cluster $CLUSTER to 0..."
for svc in "${SERVICES[@]}"; do
  aws ecs update-service \
    --region "$AWS_REGION" \
    --cluster "$CLUSTER" \
    --service "$svc" \
    --desired-count 0 \
    --no-cli-pager >/dev/null
  echo "  $svc -> desiredCount=0"
done

echo "Stopping RDS instance $RDS_INSTANCE..."
aws rds stop-db-instance \
  --region "$AWS_REGION" \
  --db-instance-identifier "$RDS_INSTANCE" \
  --no-cli-pager >/dev/null
echo "  RDS stop requested (may take several minutes)"

echo "Stopping EC2 VPN instance $VPN_INSTANCE_ID..."
aws ec2 stop-instances \
  --region "$AWS_REGION" \
  --instance-ids "$VPN_INSTANCE_ID" \
  --no-cli-pager >/dev/null
echo "  EC2 stop requested"

echo ""
echo "Pause initiated. Remaining billable resources:"
echo "  - Application Load Balancer (dupli1-prod-alb)"
echo "  - NAT Gateway (nat-168d96b459ab0cf17)"
echo "  - RDS storage (while stopped, storage still bills)"
echo "  - ECR image storage, Secrets Manager, VPC endpoints"
echo ""
echo "Note: RDS auto-restarts after 7 days. GitHub Actions deploys on main"
echo "will scale ECS services back up unless the workflow is disabled."
