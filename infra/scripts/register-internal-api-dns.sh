#!/usr/bin/env bash
set -euo pipefail

# Registers internal.schick.local to the same task IP as proxy.schick.local.
AWS_REGION="${AWS_REGION:-us-east-1}"
NAMESPACE_NAME="${CLOUD_MAP_NAMESPACE:-schick.local}"
PROXY_SERVICE="${PROXY_SERVICE_NAME:-proxy}"
INTERNAL_SERVICE="${INTERNAL_SERVICE_NAME:-internal}"

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

require_cmd aws
require_cmd jq

INSTANCES="$(aws servicediscovery discover-instances \
  --region "$AWS_REGION" \
  --namespace-name "$NAMESPACE_NAME" \
  --service-name "$PROXY_SERVICE" \
  --max-results 1 \
  --health-status ALL \
  --output json)"

IPV4="$(echo "$INSTANCES" | jq -r '.Instances[0].Attributes.AWS_INSTANCE_IPV4 // empty')"
PORT="$(echo "$INSTANCES" | jq -r '.Instances[0].Attributes.AWS_INSTANCE_PORT // "80"')"

if [[ -z "$IPV4" ]]; then
  echo "could not resolve $PROXY_SERVICE instance IP in $NAMESPACE_NAME" >&2
  exit 1
fi

SERVICE_ID="$(aws servicediscovery list-services \
  --region "$AWS_REGION" \
  --filters Name=NAMESPACE_ID,Values="$(aws servicediscovery list-namespaces --region "$AWS_REGION" --filters Name=NAME,Values="$NAMESPACE_NAME",Condition=EQ --query 'Namespaces[0].Id' --output text)" \
  --query "Services[?Name=='$INTERNAL_SERVICE'].Id | [0]" \
  --output text)"

if [[ -z "$SERVICE_ID" || "$SERVICE_ID" == "None" ]]; then
  echo "internal Cloud Map service not found; run terraform apply first" >&2
  exit 1
fi

INSTANCE_ID="internal-proxy-alias"

EXISTING="$(aws servicediscovery list-instances \
  --region "$AWS_REGION" \
  --service-id "$SERVICE_ID" \
  --query "Instances[?Id=='$INSTANCE_ID'].Id | [0]" \
  --output text)"

if [[ "$EXISTING" == "$INSTANCE_ID" ]]; then
  aws servicediscovery deregister-instance \
    --region "$AWS_REGION" \
    --service-id "$SERVICE_ID" \
    --instance-id "$INSTANCE_ID" >/dev/null || true
fi

aws servicediscovery register-instance \
  --region "$AWS_REGION" \
  --service-id "$SERVICE_ID" \
  --instance-id "$INSTANCE_ID" \
  --attributes "AWS_INSTANCE_IPV4=$IPV4,AWS_INSTANCE_PORT=$PORT" >/dev/null

echo "Registered internal.schick.local -> $IPV4:$PORT (via $PROXY_SERVICE)"
