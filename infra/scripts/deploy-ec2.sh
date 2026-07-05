#!/usr/bin/env bash
# Build and start Dupli1 on the current EC2 host.
set -euo pipefail

DUPLI1_HOME="${DUPLI1_HOME:-/opt/dupli1}"
APP_DIR="${APP_DIR:-${DUPLI1_HOME}/app}"
ENV_FILE="${ENV_FILE:-${APP_DIR}/.env.prod}"
SECRETS_DIR="${DUPLI1_SECRETS_DIR:-${DUPLI1_HOME}/secrets}"

cd "$APP_DIR"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "missing ${ENV_FILE}; run ec2-bootstrap.sh first" >&2
  exit 1
fi

export DUPLI1_SECRETS_DIR="$SECRETS_DIR"

echo "Pulling latest code..."
git pull origin "$(git branch --show-current)"

echo "Building and starting services..."
if ! docker compose \
  -f docker-compose.yml \
  -f docker-compose.prod.yml \
  --env-file "$ENV_FILE" \
  up -d --build --remove-orphans; then
  echo "First start had dependency races; retrying in 30s..."
  sleep 30
  docker compose \
    -f docker-compose.yml \
    -f docker-compose.prod.yml \
    --env-file "$ENV_FILE" \
    up -d --remove-orphans
fi

echo "Waiting for gateway health..."
for _ in $(seq 1 30); do
  if curl -sf http://localhost/gateway/health >/dev/null 2>&1; then
    echo "Gateway is healthy."
    docker compose -f docker-compose.yml -f docker-compose.prod.yml ps
    exit 0
  fi
  sleep 5
done

echo "Gateway did not become healthy in time. Check logs:" >&2
docker compose -f docker-compose.yml -f docker-compose.prod.yml logs --tail=50 dupli1-proxy dupli1-auth
exit 1
