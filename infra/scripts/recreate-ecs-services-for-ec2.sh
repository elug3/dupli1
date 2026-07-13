#!/usr/bin/env bash
# Delete paused Fargate ECS services so Terraform can recreate them on EC2.
# Optionally runs terraform apply afterward.
#
# Usage:
#   bash infra/scripts/recreate-ecs-services-for-ec2.sh
#   APPLY=1 bash infra/scripts/recreate-ecs-services-for-ec2.sh
set -euo pipefail

AWS_REGION="${AWS_REGION:-us-east-1}"
CLUSTER="${ECS_CLUSTER:-production}"
APPLY="${APPLY:-0}"

SERVICES_TO_REPLACE=(
  dupli1-auth
  dupli1-product
  dupli1-order
  dupli1-notification
  dupli1-proxy
)

echo "Removing blackhole default routes (deleted NAT) from private route tables..."
for rtb in $(aws ec2 describe-route-tables \
  --region "$AWS_REGION" \
  --filters "Name=vpc-id,Values=vpc-0e143b53ca2a4714c" "Name=tag:Name,Values=*private*" \
  --query 'RouteTables[].RouteTableId' --output text); do
  aws ec2 delete-route \
    --region "$AWS_REGION" \
    --route-table-id "$rtb" \
    --destination-cidr-block 0.0.0.0/0 2>/dev/null \
    && echo "  deleted 0.0.0.0/0 on $rtb" \
    || echo "  no default route on $rtb (ok)"
done

echo "Force-deleting ECS services to recreate on EC2 capacity provider..."
for svc in "${SERVICES_TO_REPLACE[@]}"; do
  if aws ecs describe-services --region "$AWS_REGION" --cluster "$CLUSTER" --services "$svc" \
    --query 'services[?status==`ACTIVE`].serviceName' --output text | grep -q "$svc"; then
    aws ecs delete-service \
      --region "$AWS_REGION" \
      --cluster "$CLUSTER" \
      --service "$svc" \
      --force \
      --no-cli-pager >/dev/null
    echo "  deleted $svc"
  else
    echo "  $svc not active (skip)"
  fi
done

echo "Waiting for service deletions..."
for svc in "${SERVICES_TO_REPLACE[@]}"; do
  for _ in $(seq 1 60); do
    status="$(aws ecs describe-services --region "$AWS_REGION" --cluster "$CLUSTER" --services "$svc" \
      --query 'services[0].status' --output text 2>/dev/null || echo MISSING)"
    if [[ "$status" == "MISSING" || "$status" == "None" || "$status" == "INACTIVE" ]]; then
      echo "  $svc gone"
      break
    fi
    sleep 5
  done
done

if [[ "$APPLY" == "1" ]]; then
  ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
  cd "$ROOT/infra/terraform"
  terraform init -input=false
  terraform apply -auto-approve -input=false
  echo "Gateway: $(terraform output -raw gateway_health_url)"
fi

echo "Done."
