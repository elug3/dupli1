#!/usr/bin/env bash
set -euo pipefail

# Upload WireGuard client config to Secrets Manager after server setup.
# Usage:
#   CLIENT_CONFIG=/etc/wireguard/schick-dev.conf bash upload-vpn-client-config.sh

AWS_REGION="${AWS_REGION:-us-east-1}"
SECRET_NAME="${VPN_CLIENT_SECRET_NAME:-schick/production/vpn/client-config}"
CLIENT_CONFIG="${CLIENT_CONFIG:-/etc/wireguard/schick-dev.conf}"

if [[ ! -f "$CLIENT_CONFIG" ]]; then
  echo "client config not found: $CLIENT_CONFIG" >&2
  exit 1
fi

if aws secretsmanager describe-secret --region "$AWS_REGION" --secret-id "$SECRET_NAME" >/dev/null 2>&1; then
  aws secretsmanager put-secret-value \
    --region "$AWS_REGION" \
    --secret-id "$SECRET_NAME" \
    --secret-string "file://$CLIENT_CONFIG"
else
  aws secretsmanager create-secret \
    --region "$AWS_REGION" \
    --name "$SECRET_NAME" \
    --description "WireGuard client config for Schick internal API access" \
    --secret-string "file://$CLIENT_CONFIG"
fi

echo "Uploaded VPN client config to $SECRET_NAME"
