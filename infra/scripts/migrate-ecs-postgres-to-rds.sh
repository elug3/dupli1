#!/usr/bin/env bash
set -euo pipefail

AWS_REGION="${AWS_REGION:-us-east-1}"
CLUSTER="${ECS_CLUSTER:-production}"
LEGACY_HOST="${LEGACY_DB_HOST:-postgres.schick.local}"

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

require_cmd aws
require_cmd jq

SECRET_ARN="${DB_SECRET_ARN:-$(aws secretsmanager list-secrets \
  --region "$AWS_REGION" \
  --query "SecretList[?contains(Name, 'schick/production/database')].ARN | [0]" \
  --output text)}"

if [[ -z "$SECRET_ARN" || "$SECRET_ARN" == "None" ]]; then
  echo "could not resolve DB secret ARN; set DB_SECRET_ARN" >&2
  exit 1
fi

SECRET_JSON="$(aws secretsmanager get-secret-value \
  --region "$AWS_REGION" \
  --secret-id "$SECRET_ARN" \
  --query SecretString \
  --output text)"

RDS_HOST="$(echo "$SECRET_JSON" | jq -r .host)"
RDS_PORT="$(echo "$SECRET_JSON" | jq -r .port)"
RDS_USER="$(echo "$SECRET_JSON" | jq -r .username)"
RDS_PASSWORD="$(echo "$SECRET_JSON" | jq -r .password)"
AUTH_DB="$(echo "$SECRET_JSON" | jq -r .dbname_auth)"
PRODUCT_DB="$(echo "$SECRET_JSON" | jq -r .dbname_product)"

NETWORK_JSON="$(aws ecs describe-services \
  --region "$AWS_REGION" \
  --cluster "$CLUSTER" \
  --services schick-auth \
  --query 'services[0].networkConfiguration.awsvpcConfiguration' \
  --output json)"

SUBNETS="$(echo "$NETWORK_JSON" | jq -r '.subnets | join(",")')"
SECURITY_GROUPS="$(echo "$NETWORK_JSON" | jq -r '.securityGroups | join(",")')"

MIGRATE_CMD="set -euo pipefail
export PGPASSWORD='schick_dev'
pg_dump -h '$LEGACY_HOST' -p 5432 -U schick -d '$AUTH_DB' --no-owner --no-acl -Fc -f /tmp/auth.dump
pg_dump -h '$LEGACY_HOST' -p 5432 -U schick -d '$PRODUCT_DB' --no-owner --no-acl -Fc -f /tmp/product.dump || true
export PGPASSWORD='$RDS_PASSWORD'
pg_restore -h '$RDS_HOST' -p '$RDS_PORT' -U '$RDS_USER' -d '$AUTH_DB' --no-owner --no-acl --if-exists --clean /tmp/auth.dump
if [ -f /tmp/product.dump ]; then
  pg_restore -h '$RDS_HOST' -p '$RDS_PORT' -U '$RDS_USER' -d '$PRODUCT_DB' --no-owner --no-acl --if-exists --clean /tmp/product.dump || true
fi"

TASK_DEF_JSON="$(jq -n \
  --arg cmd "$MIGRATE_CMD" \
  '{
    family: "schick-db-migrate",
    networkMode: "awsvpc",
    requiresCompatibilities: ["FARGATE"],
    cpu: "512",
    memory: "1024",
    executionRoleArn: "arn:aws:iam::845061289093:role/ecsTaskExecutionRole",
    containerDefinitions: [
      {
        name: "db-migrate",
        image: "postgres:16-alpine",
        essential: true,
        command: ["sh", "-c", $cmd]
      }
    ]
  }')"

TASK_DEF_FILE="$(mktemp)"
echo "$TASK_DEF_JSON" >"$TASK_DEF_FILE"

REGISTERED_ARN="$(aws ecs register-task-definition \
  --region "$AWS_REGION" \
  --cli-input-json "file://$TASK_DEF_FILE" \
  --query 'taskDefinition.taskDefinitionArn' \
  --output text)"
rm -f "$TASK_DEF_FILE"

TASK_ARN="$(aws ecs run-task \
  --region "$AWS_REGION" \
  --cluster "$CLUSTER" \
  --launch-type FARGATE \
  --task-definition "$REGISTERED_ARN" \
  --network-configuration "awsvpcConfiguration={subnets=[$SUBNETS],securityGroups=[$SECURITY_GROUPS],assignPublicIp=DISABLED}" \
  --query 'tasks[0].taskArn' \
  --output text)"

echo "Started migration task: $TASK_ARN"
aws ecs wait tasks-stopped --region "$AWS_REGION" --cluster "$CLUSTER" --tasks "$TASK_ARN"

EXIT_CODE="$(aws ecs describe-tasks \
  --region "$AWS_REGION" \
  --cluster "$CLUSTER" \
  --tasks "$TASK_ARN" \
  --query 'tasks[0].containers[0].exitCode' \
  --output text)"

if [[ "$EXIT_CODE" != "0" ]]; then
  echo "migration task failed with exit code $EXIT_CODE" >&2
  aws ecs describe-tasks --region "$AWS_REGION" --cluster "$CLUSTER" --tasks "$TASK_ARN" --output json
  exit 1
fi

echo "Migration from $LEGACY_HOST to RDS completed."
