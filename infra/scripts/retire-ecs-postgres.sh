#!/usr/bin/env bash
set -euo pipefail

AWS_REGION="${AWS_REGION:-us-east-1}"
CLUSTER="${ECS_CLUSTER:-production}"
SERVICE="${ECS_POSTGRES_SERVICE:-dupli1-postgres}"
DELETE_SERVICE="${DELETE_SERVICE:-true}"

aws ecs update-service \
  --region "$AWS_REGION" \
  --cluster "$CLUSTER" \
  --service "$SERVICE" \
  --desired-count 0 >/dev/null

echo "Scaled $SERVICE to 0 tasks in cluster $CLUSTER."

if [[ "$DELETE_SERVICE" == "true" ]]; then
  aws ecs delete-service \
    --region "$AWS_REGION" \
    --cluster "$CLUSTER" \
    --service "$SERVICE" \
    --force >/dev/null
  echo "Deleted ECS service $SERVICE."
else
  echo "Set DELETE_SERVICE=false to keep the service definition."
fi
