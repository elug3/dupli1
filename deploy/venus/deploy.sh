#!/usr/bin/env bash
# Deploy backend images from GHCR to the VENUS stack, rolling back if the new
# images don't come up healthy. Run by the self-hosted runner from the deploy
# checkout (/opt/dupli1/repo); see docs/deployment-venus.md → "Deploying new code".
#
#   deploy/venus/deploy.sh sha-abc1234
#
# Only the services built by .github/workflows/images.yml move; redis, nats,
# web and manage-web keep DUPLI1_IMAGE_TAG.
set -euo pipefail

TAG=${1:?usage: deploy.sh <tag, e.g. sha-abc1234>}
OWNER=${GHCR_OWNER:-elug3}
ENV_FILE=${ENV_FILE:-/opt/dupli1/.env}
HERE=$(cd "$(dirname "$0")" && pwd)
SERVICES=(auth product order cart payment profile notification proxy)
# Gateway routes that reach each service's health handler (notification has none).
HEALTH=(/gateway/health /api/v1/auth/health /api/v1/products/health
  /api/v1/orders/health /api/v1/cart/health /api/v1/payments/health
  /api/v1/profile/health)

dc() { docker compose -f "$HERE/docker-compose.yml" --env-file "$ENV_FILE" "$@"; }

current_tag() {
  local t
  t=$(grep -E '^DUPLI1_BACKEND_TAG=' "$ENV_FILE" | tail -1 | cut -d= -f2- | tr -d "'\"")
  [[ -n $t ]] || t=$(grep -E '^DUPLI1_IMAGE_TAG=' "$ENV_FILE" | tail -1 | cut -d= -f2- | tr -d "'\"")
  echo "${t:-2026-09-26}"
}

set_tag() {
  if grep -qE '^DUPLI1_BACKEND_TAG=' "$ENV_FILE"; then
    sed -i "s|^DUPLI1_BACKEND_TAG=.*|DUPLI1_BACKEND_TAG=$1|" "$ENV_FILE"
  else
    echo "DUPLI1_BACKEND_TAG=$1" >>"$ENV_FILE"
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
# health route answering 200 and the internal gateway listener answering,
# held for 15 s.
healthy() {
  local -A base
  local s p code deadline=$((SECONDS + 120)) bad=""
  for s in "${SERVICES[@]}"; do base[$s]=$(state_of "$s"); done
  while ((SECONDS < deadline)); do
    bad=""
    for s in "${SERVICES[@]}"; do
      [[ $(state_of "$s") == running/"${base[$s]#*/}" ]] || { bad="$s is $(state_of "$s")"; break; }
    done
    if [[ -z $bad ]]; then
      for p in "${HEALTH[@]}"; do
        code=$(curl -s -o /dev/null -m 5 -w '%{http_code}' -H Host:dupli1.com "http://127.0.0.1$p" || true)
        [[ $code == 200 ]] || { bad="$p → $code"; break; }
      done
    fi
    if [[ -z $bad ]] && ! internal_gateway_ok; then
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

PREV=$(current_tag)
echo "deploying $TAG (current $PREV)"

for s in "${SERVICES[@]}"; do
  docker pull -q "ghcr.io/$OWNER/dupli1-$s:$TAG"
  docker tag "ghcr.io/$OWNER/dupli1-$s:$TAG" "dupli1-prod/$s:$TAG"
done

set_tag "$TAG"
dc up -d "${SERVICES[@]}"

if healthy; then
  echo "deployed $TAG"
  exit 0
fi

echo "rolling back to $PREV" >&2
dc logs --tail 50 "${SERVICES[@]}" >&2 || true
set_tag "$PREV"
dc up -d "${SERVICES[@]}"
healthy || echo "rollback to $PREV is unhealthy too — check the stack by hand" >&2
exit 1
