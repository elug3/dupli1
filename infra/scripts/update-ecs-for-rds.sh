#!/usr/bin/env bash
set -euo pipefail

AWS_REGION="${AWS_REGION:-us-east-1}"
CLUSTER="${ECS_CLUSTER:-production}"

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

require_cmd aws
require_cmd jq

update_service_db_secret() {
  local service="$1"
  local container="$2"
  local env_name="$3"
  local secret_arn="$4"

  local current_td
  current_td="$(aws ecs describe-services \
    --region "$AWS_REGION" \
    --cluster "$CLUSTER" \
    --services "$service" \
    --query 'services[0].taskDefinition' \
    --output text)"

  local new_td
  new_td="$(aws ecs describe-task-definition \
    --region "$AWS_REGION" \
    --task-definition "$current_td" \
    --query 'taskDefinition' \
    --output json | jq \
      --arg service "$service" \
      --arg container "$container" \
      --arg env_name "$env_name" \
      --arg secret_arn "$secret_arn" \
      'del(.taskDefinitionArn, .revision, .status, .requiresAttributes, .compatibilities, .registeredAt, .registeredBy)
       | .containerDefinitions = (.containerDefinitions | map(
           if .name == $container then
             .environment = ((.environment // []) | map(select(.name != $env_name)))
             | .secrets = ((.secrets // []) | map(select(.name != $env_name)) + [{name: $env_name, valueFrom: $secret_arn}])
           else . end
         ))')"

  local tmp
  tmp="$(mktemp)"
  echo "$new_td" >"$tmp"

  local registered_arn
  registered_arn="$(aws ecs register-task-definition \
    --region "$AWS_REGION" \
    --cli-input-json "file://$tmp" \
    --query 'taskDefinition.taskDefinitionArn' \
    --output text)"
  rm -f "$tmp"

  aws ecs update-service \
    --region "$AWS_REGION" \
    --cluster "$CLUSTER" \
    --service "$service" \
    --task-definition "$registered_arn" \
    --force-new-deployment >/dev/null

  echo "Updated $service -> $registered_arn"
}

AUTH_SECRET_ARN="${AUTH_DB_URL_SECRET_ARN:-$(aws secretsmanager list-secrets \
  --region "$AWS_REGION" \
  --query "SecretList[?contains(Name, 'dupli1/production/auth-db-url')].ARN | [0]" \
  --output text)}"

PRODUCT_SECRET_ARN="${PRODUCT_DB_URL_SECRET_ARN:-$(aws secretsmanager list-secrets \
  --region "$AWS_REGION" \
  --query "SecretList[?contains(Name, 'dupli1/production/product-db-url')].ARN | [0]" \
  --output text)}"

if [[ -z "$AUTH_SECRET_ARN" || "$AUTH_SECRET_ARN" == "None" ]]; then
  echo "could not resolve auth DB secret ARN; set AUTH_DB_URL_SECRET_ARN" >&2
  exit 1
fi

if [[ -z "$PRODUCT_SECRET_ARN" || "$PRODUCT_SECRET_ARN" == "None" ]]; then
  echo "could not resolve product DB secret ARN; set PRODUCT_DB_URL_SECRET_ARN" >&2
  exit 1
fi

update_service_db_secret "dupli1-auth" "auth-container" "DB_URL" "$AUTH_SECRET_ARN"
update_service_db_secret "dupli1-product" "product-container" "DUPLI1_PRODUCT_DB" "$PRODUCT_SECRET_ARN"

echo "ECS services now read database URLs from Secrets Manager."
