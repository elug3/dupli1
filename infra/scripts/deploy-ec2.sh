#!/usr/bin/env bash
# Build and start Dupli1 on the current EC2 host.
set -euo pipefail

DUPLI1_HOME="${DUPLI1_HOME:-/opt/dupli1}"
APP_DIR="${APP_DIR:-${DUPLI1_HOME}/app}"
WEB_DIR="${WEB_DIR:-${DUPLI1_HOME}/web}"
ENV_FILE="${ENV_FILE:-${APP_DIR}/.env.prod}"
SECRETS_DIR="${DUPLI1_SECRETS_DIR:-${DUPLI1_HOME}/secrets}"
DUPLI1_WEB_REPO="${DUPLI1_WEB_REPO:-https://github.com/elug3/dupli1-web.git}"
DUPLI1_WEB_BRANCH="${DUPLI1_WEB_BRANCH:-master}"

cd "$APP_DIR"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "missing ${ENV_FILE}; run ec2-bootstrap.sh first" >&2
  exit 1
fi

export DUPLI1_SECRETS_DIR="$SECRETS_DIR"
export DUPLI1_WEB_DIR="$WEB_DIR"

echo "Pulling latest backend code..."
git pull origin "$(git branch --show-current)"

if [[ ! -d "${WEB_DIR}/.git" ]]; then
  echo "Cloning dupli1-web into ${WEB_DIR}..."
  git clone --branch "$DUPLI1_WEB_BRANCH" "$DUPLI1_WEB_REPO" "$WEB_DIR"
else
  echo "Pulling latest dupli1-web..."
  git -C "$WEB_DIR" fetch origin "$DUPLI1_WEB_BRANCH"
  git -C "$WEB_DIR" checkout "$DUPLI1_WEB_BRANCH"
  git -C "$WEB_DIR" pull origin "$DUPLI1_WEB_BRANCH"
fi

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
  if curl -sf http://localhost:8080/gateway/health >/dev/null 2>&1; then
    echo "Gateway is healthy on :8080."
    break
  fi
  sleep 5
done

echo "Ensuring dupli1-web service account token..."
bash "${APP_DIR}/infra/scripts/setup-web-service-token.sh" || true

if [[ -n "$(grep '^DUPLI1_WEB_SERVICE_TOKEN=' "$ENV_FILE" | cut -d= -f2)" ]]; then
  echo "Restarting dupli1-web with service token..."
  docker compose \
    -f docker-compose.yml \
    -f docker-compose.prod.yml \
    --env-file "$ENV_FILE" \
    up -d dupli1-web
fi

echo "Waiting for storefront on :80..."
for _ in $(seq 1 30); do
  if curl -sf http://localhost/ | grep -q "Dupli1"; then
    echo "Storefront is healthy on :80."
    docker compose -f docker-compose.yml -f docker-compose.prod.yml --env-file "$ENV_FILE" ps
    exit 0
  fi
  sleep 5
done

echo "Gateway did not become healthy in time. Check logs:" >&2
docker compose -f docker-compose.yml -f docker-compose.prod.yml --env-file "$ENV_FILE" logs --tail=50 dupli1-proxy dupli1-web dupli1-auth
exit 1
