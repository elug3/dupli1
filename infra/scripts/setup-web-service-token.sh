#!/usr/bin/env bash
# Obtain a dupli1-web service account access token and write it to .env.prod.
set -euo pipefail

APP_DIR="${APP_DIR:-/opt/dupli1/app}"
ENV_FILE="${ENV_FILE:-${APP_DIR}/.env.prod}"
GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
AUTH_URL="${AUTH_URL:-http://localhost:18080}"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "missing ${ENV_FILE}" >&2
  exit 1
fi

# shellcheck disable=SC1090
source "$ENV_FILE"

if [[ -n "${DUPLI1_WEB_SERVICE_TOKEN:-}" ]]; then
  echo "DUPLI1_WEB_SERVICE_TOKEN already set"
  exit 0
fi

EMAIL="${DUPLI1_WEB_SERVICE_EMAIL:-dupli1-web@service.dupli1.com}"
PASSWORD="${DUPLI1_WEB_SERVICE_PASSWORD:-}"

if [[ -z "$PASSWORD" ]]; then
  echo "DUPLI1_WEB_SERVICE_PASSWORD is required in ${ENV_FILE}" >&2
  exit 1
fi

echo "Fetching service account access token for ${EMAIL}..."

LOGIN_RESPONSE="$(curl -sf -X POST "${AUTH_URL}/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"${EMAIL}\",\"password\":\"${PASSWORD}\"}")"

REFRESH_TOKEN="$(echo "$LOGIN_RESPONSE" | jq -r '.refresh_token // empty')"
if [[ -z "$REFRESH_TOKEN" ]]; then
  echo "login failed: ${LOGIN_RESPONSE}" >&2
  exit 1
fi

REFRESH_RESPONSE="$(curl -sf -X POST "${AUTH_URL}/api/v1/auth/refresh" \
  -H "Content-Type: application/json" \
  -d "{\"refresh_token\":\"${REFRESH_TOKEN}\"}")"

ACCESS_TOKEN="$(echo "$REFRESH_RESPONSE" | jq -r '.token // empty')"
if [[ -z "$ACCESS_TOKEN" ]]; then
  echo "refresh failed: ${REFRESH_RESPONSE}" >&2
  exit 1
fi

if grep -q '^DUPLI1_WEB_SERVICE_TOKEN=' "$ENV_FILE"; then
  sed -i "s|^DUPLI1_WEB_SERVICE_TOKEN=.*|DUPLI1_WEB_SERVICE_TOKEN=${ACCESS_TOKEN}|" "$ENV_FILE"
else
  echo "DUPLI1_WEB_SERVICE_TOKEN=${ACCESS_TOKEN}" >> "$ENV_FILE"
fi

echo "DUPLI1_WEB_SERVICE_TOKEN written to ${ENV_FILE}"
