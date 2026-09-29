#!/usr/bin/env bash
# Route test for every API gateway config. Runs real nginx (the image the
# proxy ships) with each wrapper plus api/gateway/*.conf, in front of stub
# upstreams that answer with their own service name, and checks that each
# route reaches the right service, that product's internal APIs answer 404 on
# the public listener (:80) and pass on the internal one (:8081), and that
# nginx's own errors are JSON.
#
#   api/gateway/test.sh            # needs docker; DOCKER="sudo docker" if you must
#
# CI: .github/workflows/test.yml → gateway.
set -euo pipefail

DOCKER=${DOCKER:-docker}
IMAGE=${NGINX_IMAGE:-nginx:1.27-alpine}
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
NET=gwtest-$$
WORK=$(mktemp -d)
SERVICES=(auth product order cart payment notification support profile)
FAILED=0

cleanup() {
  $DOCKER rm -f $($DOCKER ps -aq --filter "label=gwtest=$NET") >/dev/null 2>&1 || true
  $DOCKER network rm "$NET" >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT

$DOCKER network create "$NET" >/dev/null

# One stub per service, reachable under both naming schemes the wrappers use:
# Compose service names (dupli1-<svc>) and Cloud Map names (<svc>.dupli1.local).
for s in "${SERVICES[@]}" minio; do
  cat >"$WORK/stub-$s.conf" <<EOF
server {
    listen 8080;
    listen 9000;
    location / { default_type text/plain; return 200 "SVC=$s URI=\$request_uri"; }
}
EOF
  $DOCKER run -d --label "gwtest=$NET" --network "$NET" \
    --network-alias "dupli1-$s" --network-alias "$s.dupli1.local" --network-alias "$s" \
    -v "$WORK/stub-$s.conf:/etc/nginx/conf.d/default.conf:ro" "$IMAGE" >/dev/null
done

# A curl client on the same network. --path-as-is sends paths exactly as
# written (no client-side // or ../ cleanup), so nginx does the normalizing.
CLIENT=gwtest-client-$NET
$DOCKER run -d --label "gwtest=$NET" --name "$CLIENT" --network "$NET" \
  --entrypoint sleep "${CURL_IMAGE:-curlimages/curl:8.10.1}" infinity >/dev/null

# Prints "<status> <body>".
request() { # gateway-container port path
  local resp
  resp=$($DOCKER exec "$CLIENT" curl -s --path-as-is -m 5 -w '\n%{http_code}' "http://$1:$2$3" || true)
  echo "${resp##*$'\n'} ${resp%$'\n'*}"
}

check() { # container label port path want-status want-body-substring
  local got status
  got=$(request "$1" "$3" "$4")
  status=${got%% *}
  if [[ $status == "$5" && $got == *"$6"* ]]; then
    return 0
  fi
  echo "FAIL [$2] :$3 $4 → $got (want $5 containing '$6')" >&2
  FAILED=1
}

# path → service, on the public listener. The same table must hold on :8081.
ROUTES=(
  /api/v1/auth/login=auth
  /api/v1/auth/refresh=auth
  /api/v1/auth/tokens=auth
  /api/v1/auth/me/profile=profile
  /api/v1/auth/me/addresses=profile
  /api/v1/profile/me=profile
  /api/v1/products=product
  /api/v1/products/p1=product
  /api/v1/products/promotions/evaluate=product
  /api/v1/products/promotions/reserved=product
  /api/v1/products/promotions/me/tier=product
  /api/v1/products/inventory/items/SKU1=product
  /api/v1/catalog/brands=product
  /api/v1/variants/by-sku/X=product
  /api/v1/promotions/redeem=product
  /api/v1/coupons/redeem=product
  /api/v1/inventory/items/SKU1=product
  /api/v1/orders=order
  /api/v1/orders/events=order
  /api/v1/checkout/sessions=order
  /api/v1/cart=cart
  /api/v1/carts/c1=cart
  /api/v1/payments=payment
  /api/v1/payments/p1=payment
  /api/v1/notification/telegram/subscriptions=notification
  /api/v1/support/inquiries=support
)

# Product's internal APIs, including spellings nginx must normalize first.
INTERNAL=(
  /api/v1/products/promotions/reserve
  /api/v1/products/promotions/consume
  /api/v1/products/promotions/release
  /api/v1/products/promotions/tier
  /api/v1/products/inventory/reservations
  /api/v1/products/inventory/reservations/r1/commit
  /api/v1/products/inventory/reservations/r1/release
  /api/v1/inventory/reservations
  /api/v1/inventory/reservations/r1/release
  /api/v1/products/promotions/%72eserve
  /api/v1/products/promotions%2freserve
  /api/v1/inventory/%2e%2e/products/inventory/reservations
  //api//v1/products/inventory/reservations
  /api/v1/products/x/../inventory/reservations
  /API/V1/PRODUCTS/PROMOTIONS/RESERVE
)

# Auth's API key exchange: internal only too, but served by auth.
INTERNAL_AUTH=(
  /api/v1/auth/token
  /api/v1/auth/%74oken
  //api/v1/auth//token
  /api/v1/auth/x/../token
  /API/V1/AUTH/TOKEN
)

run_suite() { # label wrapper-file [extra checks: "path=service" ...]
  local label=$1 wrapper=$2 name="gw-$1-$NET" r path svc port
  shift 2
  $DOCKER run -d --label "gwtest=$NET" --name "$name" --network "$NET" \
    -v "$wrapper:/etc/nginx/conf.d/default.conf:ro" \
    -v "$ROOT/api/gateway:/etc/nginx/dupli1:ro" "$IMAGE" >/dev/null
  if ! $DOCKER exec "$name" nginx -t >/dev/null 2>&1; then
    echo "FAIL [$label] nginx -t:" >&2
    $DOCKER exec "$name" nginx -t >&2 || true
    FAILED=1
    return
  fi
  for port in 80 8081; do
    for r in "${ROUTES[@]}" "$@"; do
      path=${r%=*} svc=${r##*=}
      check "$name" "$label" "$port" "$path" 200 "SVC=$svc URI=$path"
    done
    check "$name" "$label" "$port" /gateway/health 200 '{"status":"ok"}'
    check "$name" "$label" "$port" /api/v1/nope 404 '{"error":"not found"}'
  done
  for path in "${INTERNAL[@]}"; do
    check "$name" "$label" 80 "$path" 404 '{"error":"not found"}'
    check "$name" "$label" 8081 "$path" 200 "SVC=product"
  done
  for path in "${INTERNAL_AUTH[@]}"; do
    check "$name" "$label" 80 "$path" 404 '{"error":"not found"}'
    check "$name" "$label" 8081 "$path" 200 "SVC=auth"
  done
  echo "ok   [$label]"
}

# The ECS wrapper names the AWS VPC resolver, unreachable here; test it with
# Docker's, exactly as VENUS runs it.
sed 's/^resolver 10\.0\.0\.2 /resolver 127.0.0.11 /' "$ROOT/api/nginx.ecs.conf" >"$WORK/nginx.ecs.conf"

run_suite local "$ROOT/api/nginx.conf" /product-images/a.jpg=minio
run_suite ecs "$WORK/nginx.ecs.conf"
run_suite venus "$ROOT/deploy/venus/nginx-gateway.conf"
run_suite prod "$ROOT/api/nginx.prod.conf" /product-images/a.jpg=minio

if ((FAILED)); then
  echo "gateway route test FAILED" >&2
  exit 1
fi
echo "gateway route test passed"
