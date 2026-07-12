#!/usr/bin/env bash
# Create application databases on Dupli1 RDS after terraform apply.
# Auth DB (dupli1_db) is created by RDS on first boot; this script creates the rest.
#
# Requires: aws CLI, jq, and network reachability to RDS (VPN / bastion / SSM port-forward).
set -euo pipefail

AWS_REGION="${AWS_REGION:-us-east-1}"
SECRET_ID="${DB_SECRET_ID:-dupli1/production/database}"
DATABASES=("products" "orders" "cart" "payments")

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

require_cmd aws
require_cmd jq
require_cmd psql

secret_json="$(aws secretsmanager get-secret-value \
  --region "$AWS_REGION" \
  --secret-id "$SECRET_ID" \
  --query SecretString \
  --output text)"

host="$(jq -r .host <<<"$secret_json")"
port="$(jq -r .port <<<"$secret_json")"
user="$(jq -r .username <<<"$secret_json")"
pass="$(jq -r .password <<<"$secret_json")"
auth_db="$(jq -r .dbname_auth <<<"$secret_json")"

export PGPASSWORD="$pass"

echo "Connecting to $host:$port as $user (db=$auth_db)..."
for db in "${DATABASES[@]}"; do
  exists="$(psql -h "$host" -p "$port" -U "$user" -d "$auth_db" -tAc "SELECT 1 FROM pg_database WHERE datname='${db}'")"
  if [[ "$exists" == "1" ]]; then
    echo "  $db already exists"
  else
    echo "  creating $db"
    psql -h "$host" -p "$port" -U "$user" -d "$auth_db" -c "CREATE DATABASE ${db};"
  fi
done

echo "Done."
