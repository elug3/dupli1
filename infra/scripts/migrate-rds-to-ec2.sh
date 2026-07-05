#!/usr/bin/env bash
# Export production data from RDS and import into the EC2 Postgres container.
#
# Prerequisites:
#   - AWS CLI configured
#   - Dupli1 stack running on EC2 (deploy-ec2.sh)
#   - RDS instance started (script will start it if stopped)
#
# Usage (on EC2 or any host with VPC access to RDS):
#   bash infra/scripts/migrate-rds-to-ec2.sh
set -euo pipefail

AWS_REGION="${AWS_REGION:-us-east-1}"
RDS_INSTANCE="${RDS_INSTANCE:-dupli1-production}"
DUPLI1_HOME="${DUPLI1_HOME:-/opt/dupli1}"
APP_DIR="${APP_DIR:-${DUPLI1_HOME}/app}"
ENV_FILE="${ENV_FILE:-${APP_DIR}/.env.prod}"
DUMP_DIR="${DUMP_DIR:-/tmp/dupli1-rds-dump}"

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || { echo "missing: $1" >&2; exit 1; }
}

require_cmd aws
require_cmd jq
require_cmd docker

mkdir -p "$DUMP_DIR"

RDS_STATUS="$(aws rds describe-db-instances \
  --region "$AWS_REGION" \
  --db-instance-identifier "$RDS_INSTANCE" \
  --query 'DBInstances[0].DBInstanceStatus' \
  --output text)"

if [[ "$RDS_STATUS" == "stopped" ]]; then
  echo "Starting RDS $RDS_INSTANCE for export..."
  aws rds start-db-instance --region "$AWS_REGION" --db-instance-identifier "$RDS_INSTANCE" --no-cli-pager >/dev/null
  echo "Waiting for RDS to become available..."
  aws rds wait db-instance-available --region "$AWS_REGION" --db-instance-identifier "$RDS_INSTANCE"
elif [[ "$RDS_STATUS" != "available" ]]; then
  echo "RDS is in state '$RDS_STATUS'; wait and retry" >&2
  exit 1
fi

SECRET_ARN="${DB_SECRET_ARN:-$(aws secretsmanager list-secrets \
  --region "$AWS_REGION" \
  --query "SecretList[?contains(Name, 'dupli1/production/database')].ARN | [0]" \
  --output text)}"

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

echo "Dumping $AUTH_DB and $PRODUCT_DB from RDS ($RDS_HOST)..."
docker run --rm \
  -e PGPASSWORD="$RDS_PASSWORD" \
  -v "$DUMP_DIR:/dump" \
  postgres:16-alpine \
  sh -c "
    pg_dump -h '$RDS_HOST' -p '$RDS_PORT' -U '$RDS_USER' -d '$AUTH_DB' --no-owner --no-acl -Fc -f /dump/auth.dump
    pg_dump -h '$RDS_HOST' -p '$RDS_PORT' -U '$RDS_USER' -d '$PRODUCT_DB' --no-owner --no-acl -Fc -f /dump/product.dump || true
  "

if [[ ! -f "$ENV_FILE" ]]; then
  echo "missing $ENV_FILE" >&2
  exit 1
fi
# shellcheck disable=SC1090
source "$ENV_FILE"

LOCAL_PG="postgres://dupli1:${POSTGRES_PASSWORD}@localhost:5432"

echo "Ensuring local databases exist..."
docker compose -f "$APP_DIR/docker-compose.yml" -f "$APP_DIR/docker-compose.prod.yml" \
  --env-file "$ENV_FILE" exec -T postgres psql -U dupli1 -d dupli1_db -c "SELECT 1" >/dev/null

for db in products inventory orders; do
  docker compose -f "$APP_DIR/docker-compose.yml" -f "$APP_DIR/docker-compose.prod.yml" \
    --env-file "$ENV_FILE" exec -T postgres \
    psql -U dupli1 -d dupli1_db -tc "SELECT 1 FROM pg_database WHERE datname='$db'" | grep -q 1 \
    || docker compose -f "$APP_DIR/docker-compose.yml" -f "$APP_DIR/docker-compose.prod.yml" \
         --env-file "$ENV_FILE" exec -T postgres \
         psql -U dupli1 -d dupli1_db -c "CREATE DATABASE $db"
done

echo "Restoring auth database..."
docker run --rm \
  --network host \
  -e PGPASSWORD="$POSTGRES_PASSWORD" \
  -v "$DUMP_DIR:/dump:ro" \
  postgres:16-alpine \
  pg_restore -h localhost -p 5432 -U dupli1 -d dupli1_db --no-owner --no-acl --if-exists --clean /dump/auth.dump

if [[ -f "$DUMP_DIR/product.dump" ]]; then
  echo "Restoring product database..."
  docker run --rm \
    --network host \
    -e PGPASSWORD="$POSTGRES_PASSWORD" \
    -v "$DUMP_DIR:/dump:ro" \
    postgres:16-alpine \
    pg_restore -h localhost -p 5432 -U dupli1 -d products --no-owner --no-acl --if-exists --clean /dump/product.dump || true
fi

echo "Migration complete. Inventory and orders start empty (were not on RDS)."
echo "Stop RDS again to save costs: aws rds stop-db-instance --db-instance-identifier $RDS_INSTANCE"
