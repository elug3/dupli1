#!/usr/bin/env bash
#
# Campaign dry run for the sign-up promotional code (WELCOME50).
#
# This is the dry run the promotional-code plan asks for before the marketing
# date, written as a script so it is repeatable rather than a checklist:
#
#   seeded inactive -> register -> auto-issued entitlement -> enable campaign
#   -> below minimum spend refused with a reason -> above it discounts
#   -> order carries the code -> paid consumes the use -> a second use refused
#   -> cancel releases it -> usable again
#
# It covers the seams unit tests cannot: the gateway's promotion routes, order
# asking product to price a code over HTTP, the registration subscriber over
# NATS, and the redemption ledger moving reserve -> consume -> release against
# a real database.
#
# Usage:
#   BASE=http://localhost:8080 scripts/smoke-promotion-campaign.sh
#
# Environment:
#   BASE            gateway base URL (default http://localhost:8080)
#   OWNER_EMAIL     manager/owner login (default admin@dupli1.com)
#   OWNER_PASSWORD  manager/owner password (default password)
#   CODE            campaign to exercise (default WELCOME50)
#   MIN_SPEND_WON   the campaign's minimum spend (default 100000)
#   DISCOUNT_WON    the campaign's fixed discount (default 50000)
#
# THE CAMPAIGN IS ENABLED AND THEN RESTORED. The run switches the code on,
# because that is the state being tested, and puts `active` back to whatever it
# found on the way out — including if a check fails. Do not point this at
# production while customers are shopping: between those two moments the
# discount is live for everyone who holds an entitlement.
#
# The run also creates a customer account, a throwaway product and a real
# order, like scripts/smoke-money-path.sh.
set -uo pipefail

BASE=${BASE:-http://localhost:8080}
OWNER_EMAIL=${OWNER_EMAIL:-admin@dupli1.com}
OWNER_PASSWORD=${OWNER_PASSWORD:-password}
CODE=${CODE:-WELCOME50}
MIN_SPEND_WON=${MIN_SPEND_WON:-100000}
DISCOUNT_WON=${DISCOUNT_WON:-50000}
JSON='Content-Type: application/json'

# Two units must clear the minimum spend and one must fall short of it, so the
# refusal and the discount are both reachable from the same product.
UNIT_PRICE_WON=$(( (MIN_SPEND_WON / 2) + 10000 ))

failures=0
CAMPAIGN_WAS_ACTIVE=""
OWNER=""

step() { printf '\n\033[1m== %s\033[0m\n' "$1"; }
pass() { printf '  ok    %s\n' "$1"; }
fail() { printf '  FAIL  %s\n' "$1"; failures=$((failures + 1)); }

check() { # check <description> <actual> <expected>
  if [ "$2" = "$3" ]; then pass "$1 ($2)"; else fail "$1: got '$2', want '$3'"; fi
}

# read a field from JSON on stdin, e.g. field '["order"]["id"]'
field() { python3 -c "import json,sys;d=json.load(sys.stdin);print($1)" 2>/dev/null; }

api() { # api <method> <path> [token] [body]
  local method=$1 path=$2 token=${3:-} body=${4:-}
  local args=(-s -X "$method" "$BASE$path" -H "$JSON")
  [ -n "$token" ] && args+=(-H "Authorization: Bearer $token")
  [ -n "$body" ] && args+=(-d "$body")
  curl "${args[@]}"
}

status_of() { # status_of <method> <path> [token] [body]
  local method=$1 path=$2 token=${3:-} body=${4:-}
  local args=(-s -o /dev/null -w '%{http_code}' -X "$method" "$BASE$path" -H "$JSON")
  [ -n "$token" ] && args+=(-H "Authorization: Bearer $token")
  [ -n "$body" ] && args+=(-d "$body")
  curl "${args[@]}"
}

# capture sets REPLY_CODE and REPLY_BODY from one call, for the refusals where
# both the status and the reason matter and a second call would change state.
REPLY_CODE=""
REPLY_BODY=""
capture() { # capture <method> <path> [token] [body]
  local method=$1 path=$2 token=${3:-} body=${4:-} out
  local args=(-s -w $'\n%{http_code}' -X "$method" "$BASE$path" -H "$JSON")
  [ -n "$token" ] && args+=(-H "Authorization: Bearer $token")
  [ -n "$body" ] && args+=(-d "$body")
  out=$(curl "${args[@]}")
  REPLY_CODE=${out##*$'\n'}
  REPLY_BODY=${out%$'\n'*}
}

access_token() { # access_token <email> <password>
  local refresh
  refresh=$(api POST /api/v1/auth/login "" "{\"email\":\"$1\",\"password\":\"$2\"}" | field 'd["refresh_token"]')
  [ -n "$refresh" ] || return 1
  api POST /api/v1/auth/refresh "" "{\"refresh_token\":\"$refresh\"}" | field 'd["token"]'
}

campaign_field() { # campaign_field <python-expr over the definition>
  api GET "/api/v1/products/promotions" "$OWNER" \
    | python3 -c "
import json,sys
rows = json.load(sys.stdin).get('results') or []
d = next((r for r in rows if r.get('code') == '$CODE'), None)
print($1 if d else '')
" 2>/dev/null
}

set_campaign_active() { # set_campaign_active <true|false>
  api PUT "/api/v1/products/promotions/by-code/$CODE" "$OWNER" "{\"active\":$1}" >/dev/null
}

# Put the campaign back however it was found, whatever happens after this point.
restore_campaign() {
  [ -n "$CAMPAIGN_WAS_ACTIVE" ] || return 0
  [ -n "$OWNER" ] || return 0
  set_campaign_active "$CAMPAIGN_WAS_ACTIVE"
  printf '\n  campaign %s restored to active=%s\n' "$CODE" "$CAMPAIGN_WAS_ACTIVE"
}
trap restore_campaign EXIT

new_session() { # new_session <customer-token> <customer-id> <quantity> -> session id
  local token=$1 customer=$2 qty=$3 sid
  sid=$(api POST /api/v1/orders/checkout/sessions "$token" "{\"customer_id\":\"$customer\"}" | field 'd["id"]')
  [ -n "$sid" ] || return 1
  api POST "/api/v1/orders/checkout/sessions/$sid/items" "$token" \
    "{\"sku_id\":\"$SKU_ID\",\"quantity\":$qty}" >/dev/null
  echo "$sid"
}

step "Gateway and service health"
for svc in gateway/health api/v1/auth/health api/v1/products/health api/v1/orders/health \
           api/v1/cart/health api/v1/payments/health; do
  check "$svc" "$(status_of GET "/$svc")" 200
done

step "Manager sign-in"
OWNER=$(access_token "$OWNER_EMAIL" "$OWNER_PASSWORD")
[ -n "$OWNER" ] || { fail "manager login as $OWNER_EMAIL"; exit 1; }
pass "manager access token acquired"

step "The campaign is seeded, and seeded off"
CAMPAIGN_WAS_ACTIVE=$(campaign_field 'str(d["active"]).lower()')
[ -n "$CAMPAIGN_WAS_ACTIVE" ] || { fail "$CODE is not seeded — the issuer has nothing to issue"; exit 1; }
pass "$CODE exists (active=$CAMPAIGN_WAS_ACTIVE)"
check "scope" "$(campaign_field 'd["scope"]')" single_user
check "benefit is a fixed amount" "$(campaign_field 'str(d["benefit"]["discount_fixed_won"])')" "$DISCOUNT_WON"
check "minimum spend" \
  "$(campaign_field 'str(d["conditions"]["all"][0]["value"])')" "$MIN_SPEND_WON"
if [ "$CAMPAIGN_WAS_ACTIVE" = "false" ]; then
  pass "seeded inactive — enabling it is a manager action, not a deploy"
else
  printf '  note  %s was already active on this environment\n' "$CODE"
fi

step "Seed a throwaway product priced to straddle the minimum spend"
STYLE_CODE="PR$(date +%H%M%S)"
for path_body in \
  "/api/v1/products/catalog/brands|{\"code\":\"PRM\",\"name\":\"Promo Test\"}" \
  "/api/v1/products/catalog/brands/PRM/styles|{\"code\":\"$STYLE_CODE\",\"name\":\"Promo Style\"}" \
  "/api/v1/products/catalog/colors|{\"code\":\"BLK\",\"name\":\"Black\"}" \
  "/api/v1/products/catalog/sizes|{\"code\":\"M\",\"name\":\"Medium\"}"
do
  code=$(status_of POST "${path_body%%|*}" "$OWNER" "${path_body#*|}")
  case "$code" in
    201|409) pass "master data ${path_body%%|*} ($code)" ;;
    *) fail "master data ${path_body%%|*} returned $code" ;;
  esac
done

product=$(api POST /api/v1/products "$OWNER" "{
  \"name\":\"Promo Test Bag\",\"description\":\"created by smoke-promotion-campaign.sh\",
  \"brand\":\"Promo Test\",\"brandCode\":\"PRM\",\"styleCode\":\"$STYLE_CODE\",
  \"category\":\"bags\",\"subCategory\":\"cross\",\"style\":\"casual\",\"target\":\"women\",
  \"material\":\"leather\",\"price\":$UNIT_PRICE_WON,\"officialPrice\":$UNIT_PRICE_WON,
  \"status\":\"active\"
}")
PRODUCT_ID=$(echo "$product" | field 'd["id"]')
[ -n "$PRODUCT_ID" ] || { fail "create product: $product"; exit 1; }
variant=$(api POST "/api/v1/products/$PRODUCT_ID/variants" "$OWNER" \
  '{"colorCode":"BLK","sizeCode":"M","status":"active"}')
SKU_ID=$(echo "$variant" | field 'd["skuId"]')
[ -n "$SKU_ID" ] || { fail "create variant: $variant"; exit 1; }
api PUT "/api/v1/products/inventory/items/by-sku-id/$SKU_ID" "$OWNER" '{"quantity":10}' >/dev/null
pass "variant $SKU_ID at $UNIT_PRICE_WON원 (1 below the minimum, 2 above)"

step "Registering issues the welcome code (user.registered -> product)"
EMAIL="promo-$(date +%s)-$RANDOM@example.com"
check "register" "$(status_of POST /api/v1/auth/register "" \
  "{\"email\":\"$EMAIL\",\"password\":\"promo-test-password\"}")" 201
CUSTOMER=$(access_token "$EMAIL" "promo-test-password")
CUSTOMER_ID=$(api GET /api/v1/auth/me "$CUSTOMER" | field 'd["user_id"]')
[ -n "$CUSTOMER_ID" ] || { fail "customer sign-in"; exit 1; }

# The issue rides a NATS event, so it is not synchronous with registration.
wallet_codes=""
for _ in $(seq 1 15); do
  wallet_codes=$(api GET /api/v1/products/promotions/me "$CUSTOMER" \
    | field '",".join(r["entitlement"]["code"] for r in (d.get("results") or []))')
  [ -n "$wallet_codes" ] && break
  sleep 2
done
check "wallet holds the welcome code" "$wallet_codes" "$CODE"

step "Enable the campaign"
set_campaign_active true
check "active" "$(campaign_field 'str(d["active"]).lower()')" true

step "Below the minimum spend, the code is refused with a reason"
SESSION_SMALL=$(new_session "$CUSTOMER" "$CUSTOMER_ID" 1)
[ -n "$SESSION_SMALL" ] || { fail "create small session"; exit 1; }
capture POST "/api/v1/orders/checkout/sessions/$SESSION_SMALL/promotion" "$CUSTOMER" \
  "{\"code\":\"$CODE\"}"
check "status" "$REPLY_CODE" 422
check "reason" "$(echo "$REPLY_BODY" | field 'd.get("reason","")')" not_eligible
check "sub_reason" "$(echo "$REPLY_BODY" | field 'd.get("sub_reason","")')" min_spend

step "Above it, the code discounts"
SESSION=$(new_session "$CUSTOMER" "$CUSTOMER_ID" 2)
[ -n "$SESSION" ] || { fail "create session"; exit 1; }
subtotal=$((UNIT_PRICE_WON * 2))
capture POST "/api/v1/orders/checkout/sessions/$SESSION/promotion" "$CUSTOMER" \
  "{\"code\":\"$CODE\"}"
check "status" "$REPLY_CODE" 200
check "session discount" "$(echo "$REPLY_BODY" | field 'd["discount_won"]')" "$DISCOUNT_WON"
session_fee=$(echo "$REPLY_BODY" | field 'd.get("shipping_fee_won",0)')
check "session total = subtotal - discount + shipping" \
  "$(echo "$REPLY_BODY" | field 'd["total_won"]')" \
  "$((subtotal - DISCOUNT_WON + session_fee))"

step "The order carries the code and the discount"
completed=$(api POST "/api/v1/orders/checkout/sessions/$SESSION/complete" "$CUSTOMER" \
  '{"recipient_name":"Promo Test","recipient_phone":"01012345678","shipping_address":{"postal_code":"06236","address_line1":"123 Teheran-ro","city":"Gangnam-gu","province":"Seoul"}}')
ORDER_ID=$(echo "$completed" | field 'd["order"]["id"]')
[ -n "$ORDER_ID" ] || { fail "complete checkout: $completed"; exit 1; }
check "promotion_code" "$(echo "$completed" | field 'd["order"]["promotion_code"]')" "$CODE"
# The pre-rename key order still emits for one release.
check "coupon_code alias" "$(echo "$completed" | field 'd["order"]["coupon_code"]')" "$CODE"
check "order discount" "$(echo "$completed" | field 'd["order"]["discount_won"]')" "$DISCOUNT_WON"
order_fee=$(echo "$completed" | field 'd["order"].get("shipping_fee_won",0)')
check "order total" "$(echo "$completed" | field 'd["order"]["total_won"]')" \
  "$((subtotal - DISCOUNT_WON + order_fee))"

step "Paying consumes the use"
payment=$(api POST /api/v1/payments "$OWNER" \
  "{\"order_id\":\"$ORDER_ID\",\"method\":\"bypass\",\"note\":\"promotion dry run\"}")
[ -n "$(echo "$payment" | field 'd["id"]')" ] || { fail "bypass payment: $payment"; exit 1; }
order_status=""
for _ in $(seq 1 15); do
  order_status=$(api GET "/api/v1/orders/$ORDER_ID" "$CUSTOMER" | field 'd["status"]')
  [ "$order_status" = "paid" ] && break
  sleep 2
done
check "order status" "$order_status" paid

SESSION_AGAIN=$(new_session "$CUSTOMER" "$CUSTOMER_ID" 2)
capture POST "/api/v1/orders/checkout/sessions/$SESSION_AGAIN/promotion" "$CUSTOMER" \
  "{\"code\":\"$CODE\"}"
check "second use refused" "$REPLY_CODE" 422
check "reason" "$(echo "$REPLY_BODY" | field 'd.get("reason","")')" already_used

step "Cancelling before shipment gives the use back"
check "canceled" \
  "$(api PUT "/api/v1/orders/$ORDER_ID/status" "$OWNER" '{"status":"canceled"}' | field 'd["status"]')" \
  canceled
# The release travels with the cancel, so retry briefly rather than racing it.
released=""
for _ in $(seq 1 10); do
  SESSION_AFTER=$(new_session "$CUSTOMER" "$CUSTOMER_ID" 2)
  capture POST "/api/v1/orders/checkout/sessions/$SESSION_AFTER/promotion" "$CUSTOMER" \
    "{\"code\":\"$CODE\"}"
  [ "$REPLY_CODE" = "200" ] && { released=yes; break; }
  sleep 2
done
check "code usable again after cancel" "${released:-no}" yes
check "discount again" "$(echo "$REPLY_BODY" | field 'd["discount_won"]')" "$DISCOUNT_WON"

printf '\n'
if [ "$failures" -eq 0 ]; then
  printf 'campaign dry run OK — %s issued to %s, spent on order %s, released on cancel\n' \
    "$CODE" "$EMAIL" "$ORDER_ID"
else
  printf '%d check(s) FAILED\n' "$failures"
fi
exit $((failures > 0))
