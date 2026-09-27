#!/usr/bin/env bash
# Deploy images from GHCR to the VENUS stack, rolling back if the new images
# don't come up healthy. Run by the self-hosted runners from the deploy
# checkout (/opt/dupli1/repo); see docs/deployment-venus.md → "Deploying new code".
#
#   deploy/venus/deploy.sh sha-abc1234                # backend (this repo)
#   deploy/venus/deploy.sh web sha-abc1234            # dupli1-web
#   deploy/venus/deploy.sh manage-web sha-abc1234     # dupli1-manage-web
#
# Each target moves only its own services and its own tag variable in
# /opt/dupli1/.env; redis and nats keep DUPLI1_IMAGE_TAG. Deploys from the
# three repos take a shared lock, so they never edit .env at the same time.
set -euo pipefail

usage() { echo "usage: deploy.sh [backend|web|manage-web] <tag, e.g. sha-abc1234>" >&2; exit 2; }
case $# in
  1) TARGET=backend; TAG=$1 ;;
  2) TARGET=$1; TAG=$2 ;;
  *) usage ;;
esac
# A lone target name ("deploy.sh web") is a missing tag, not a backend tag.
case $TAG in backend | web | manage-web | "") usage ;; esac
[[ $TAG =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]] || { echo "bad tag: $TAG" >&2; exit 2; }
OWNER=${GHCR_OWNER:-elug3}
ENV_FILE=${ENV_FILE:-/opt/dupli1/.env}
LOCK_FILE=${LOCK_FILE:-$(dirname "$ENV_FILE")/.deploy.lock}
HERE=$(cd "$(dirname "$0")" && pwd)

# Per target: services, their GHCR and local image names, the .env variable
# holding their tag, and "host path" pairs that must answer 200 via the edge.
declare -A GHCR LOCAL
INTERNAL_GATEWAY=0
case $TARGET in
  backend)
    SERVICES=(auth product order cart payment profile notification proxy)
    for s in "${SERVICES[@]}"; do GHCR[$s]=dupli1-$s; LOCAL[$s]=dupli1-prod/$s; done
    TAG_VAR=DUPLI1_BACKEND_TAG
    # Gateway routes that reach each service's health handler (notification has none).
    HEALTH=("dupli1.com /gateway/health" "dupli1.com /api/v1/auth/health"
      "dupli1.com /api/v1/products/health" "dupli1.com /api/v1/orders/health"
      "dupli1.com /api/v1/cart/health" "dupli1.com /api/v1/payments/health"
      "dupli1.com /api/v1/profile/health")
    INTERNAL_GATEWAY=1
    ;;
  web)
    SERVICES=(web)
    GHCR[web]=dupli1-web; LOCAL[web]=dupli1-prod/web
    TAG_VAR=DUPLI1_WEB_TAG
    HEALTH=("dupli1.com /" "dupli1.com /login")
    ;;
  manage-web)
    SERVICES=(manage-web)
    GHCR[manage-web]=dupli1-manage-web; LOCAL[manage-web]=dupli1-prod/manage-web-container
    TAG_VAR=DUPLI1_MANAGE_WEB_TAG
    HEALTH=("manage.dupli1.com /" "manage.dupli1.com /login")
    ;;
  *) echo "unknown target: $TARGET (backend, web or manage-web)" >&2; exit 2 ;;
esac

dc() { docker compose -f "$HERE/docker-compose.yml" --env-file "$ENV_FILE" "$@"; }

env_value() { grep -E "^$1=" "$ENV_FILE" | tail -1 | cut -d= -f2- | tr -d "'\""; }

current_tag() {
  local t
  t=$(env_value "$TAG_VAR")
  [[ -n $t ]] || t=$(env_value DUPLI1_IMAGE_TAG)
  echo "${t:-2026-09-26}"
}

set_tag() {
  if grep -qE "^$TAG_VAR=" "$ENV_FILE"; then
    sed -i "s|^$TAG_VAR=.*|$TAG_VAR=$1|" "$ENV_FILE"
  else
    echo "$TAG_VAR=$1" >>"$ENV_FILE"
  fi
}

# The gateway's internal listener, where order sends product's internal APIs
# (reservations, promotion ledger). The health routes above only go through
# :80, so without this a proxy missing :8081 would pass while checkout fails.
internal_gateway_ok() {
  dc exec -T proxy wget -q -O /dev/null -T 5 http://proxy.dupli1.local:8081/gateway/health >/dev/null 2>&1
}

state_of() { docker inspect -f '{{.State.Status}}/{{.RestartCount}}' "dupli1-$1-1" 2>/dev/null || echo missing; }

# Every service running with the restart count it had right after `up`, every
# health route answering 200 (and, for the backend, the internal gateway
# listener answering), held for 15 s.
healthy() {
  local -A base
  local s h host path code deadline=$((SECONDS + 120)) bad=""
  for s in "${SERVICES[@]}"; do base[$s]=$(state_of "$s"); done
  while ((SECONDS < deadline)); do
    bad=""
    for s in "${SERVICES[@]}"; do
      [[ $(state_of "$s") == running/"${base[$s]#*/}" ]] || { bad="$s is $(state_of "$s")"; break; }
    done
    if [[ -z $bad ]]; then
      for h in "${HEALTH[@]}"; do
        host=${h%% *}; path=${h#* }
        code=$(curl -s -o /dev/null -m 5 -w '%{http_code}' -H "Host: $host" "http://127.0.0.1$path" || true)
        [[ $code == 200 ]] || { bad="$host$path → $code"; break; }
      done
    fi
    if [[ -z $bad ]] && ((INTERNAL_GATEWAY)) && ! internal_gateway_ok; then
      bad="gateway internal listener :8081 not answering"
    fi
    if [[ -z $bad ]]; then
      sleep 15
      for s in "${SERVICES[@]}"; do
        [[ $(state_of "$s") == running/"${base[$s]#*/}" ]] || { echo "unhealthy: $s is $(state_of "$s")" >&2; return 1; }
      done
      return 0
    fi
    sleep 5
  done
  echo "unhealthy: $bad" >&2
  return 1
}

exec 9>"$LOCK_FILE"
if ! flock -n 9; then
  echo "waiting for another deploy to finish…"
  flock -w 900 9 || { echo "gave up waiting for the deploy lock ($LOCK_FILE)" >&2; exit 1; }
fi

PREV=$(current_tag)
echo "deploying $TARGET $TAG (current $PREV)"

for s in "${SERVICES[@]}"; do
  docker pull -q "ghcr.io/$OWNER/${GHCR[$s]}:$TAG"
  docker tag "ghcr.io/$OWNER/${GHCR[$s]}:$TAG" "${LOCAL[$s]}:$TAG"
done

set_tag "$TAG"
dc up -d "${SERVICES[@]}"

if healthy; then
  echo "deployed $TARGET $TAG"
  exit 0
fi

echo "rolling back $TARGET to $PREV" >&2
dc logs --tail 50 "${SERVICES[@]}" >&2 || true
set_tag "$PREV"
dc up -d "${SERVICES[@]}"
healthy || echo "rollback to $PREV is unhealthy too — check the stack by hand" >&2
exit 1
