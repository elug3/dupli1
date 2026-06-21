#!/usr/bin/env bash
set -euo pipefail

AWS_REGION="${AWS_REGION:-us-east-1}"
CLUSTER="${ECS_CLUSTER:-production}"
SERVICE="${ECS_POSTGRES_SERVICE:-schick-postgres}"

aws ecs update-service \
  --region "$AWS_REGION" \
  --cluster "$CLUSTER" \
  --service "$SERVICE" \
  --desired-count 0 >/dev/null

echo "Scaled $SERVICE to 0 tasks in cluster $CLUSTER."
