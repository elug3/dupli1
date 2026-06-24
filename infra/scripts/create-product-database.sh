#!/usr/bin/env bash
set -euo pipefail

AWS_REGION="${AWS_REGION:-us-east-1}"
CLUSTER="${ECS_CLUSTER:-production}"
PRODUCT_DB_NAME="${PRODUCT_DB_NAME:-products}"

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

DB_HOST="$(echo "$SECRET_JSON" | jq -r .host)"
DB_PORT="$(echo "$SECRET_JSON" | jq -r .port)"
DB_USER="$(echo "$SECRET_JSON" | jq -r .username)"
DB_PASSWORD="$(echo "$SECRET_JSON" | jq -r .password)"

echo "Ensuring database '$PRODUCT_DB_NAME' exists on $DB_HOST..."

NETWORK_JSON="$(aws ecs describe-services \
  --region "$AWS_REGION" \
  --cluster "$CLUSTER" \
  --services schick-auth \
  --query 'services[0].networkConfiguration.awsvpcConfiguration' \
  --output json)"

SUBNETS="$(echo "$NETWORK_JSON" | jq -r '.subnets | join(",")')"
SECURITY_GROUPS="$(echo "$NETWORK_JSON" | jq -r '.securityGroups | join(",")')"

INIT_CMD="set -euo pipefail
export PGPASSWORD='$DB_PASSWORD'
exists=\$(psql -h '$DB_HOST' -p '$DB_PORT' -U '$DB_USER' -d postgres -tAc \"SELECT 1 FROM pg_database WHERE datname = '$PRODUCT_DB_NAME'\")
if [ \"\$exists\" != \"1\" ]; then
  createdb -h '$DB_HOST' -p '$DB_PORT' -U '$DB_USER' '$PRODUCT_DB_NAME'
fi"

TASK_DEF_FILE="$(mktemp)"
jq -n \
  --arg cmd "$INIT_CMD" \
  '{
    family: "schick-db-init",
    networkMode: "awsvpc",
    requiresCompatibilities: ["FARGATE"],
    cpu: "256",
    memory: "512",
    executionRoleArn: "arn:aws:iam::845061289093:role/ecsTaskExecutionRole",
    containerDefinitions: [
      {
        name: "db-init",
        image: "postgres:16-alpine",
        essential: true,
        command: ["sh", "-c", $cmd]
      }
    ]
  }' >"$TASK_DEF_FILE"

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

echo "Started db-init task: $TASK_ARN"
aws ecs wait tasks-stopped --region "$AWS_REGION" --cluster "$CLUSTER" --tasks "$TASK_ARN"

EXIT_CODE="$(aws ecs describe-tasks \
  --region "$AWS_REGION" \
  --cluster "$CLUSTER" \
  --tasks "$TASK_ARN" \
  --query 'tasks[0].containers[0].exitCode' \
  --output text)"

if [[ "$EXIT_CODE" != "0" ]]; then
  echo "db-init task failed with exit code $EXIT_CODE" >&2
  aws ecs describe-tasks --region "$AWS_REGION" --cluster "$CLUSTER" --tasks "$TASK_ARN" --output json
  exit 1
fi

echo "Database '$PRODUCT_DB_NAME' is ready."
