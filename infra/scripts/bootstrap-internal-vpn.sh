#!/usr/bin/env bash
set -euo pipefail

# Bootstrap WireGuard on the internal-vpn EC2 instance via SSH.
# Requires EC2 Instance Connect or an SSH key with access to ec2-user@<vpn-host>.

AWS_REGION="${AWS_REGION:-us-east-1}"
VPN_INSTANCE_ID="${VPN_INSTANCE_ID:-i-0f7a516c42a8b7afd}"
SSH_USER="${SSH_USER:-ec2-user}"
SSH_KEY="${SSH_KEY:-/tmp/vpn-ec2-key}"
REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

require_cmd aws
require_cmd scp
require_cmd ssh
require_cmd ssh-keygen

PUBLIC_IP="$(aws ec2 describe-instances \
  --region "$AWS_REGION" \
  --instance-ids "$VPN_INSTANCE_ID" \
  --query 'Reservations[0].Instances[0].PublicIpAddress' \
  --output text)"

if [[ -z "$PUBLIC_IP" || "$PUBLIC_IP" == "None" ]]; then
  echo "VPN instance has no public IP: $VPN_INSTANCE_ID" >&2
  exit 1
fi

if [[ ! -f "${SSH_KEY}" ]]; then
  ssh-keygen -t ed25519 -f "$SSH_KEY" -N "" -q
fi

if [[ ! -f "${SSH_KEY}.pub" ]]; then
  echo "missing public key: ${SSH_KEY}.pub" >&2
  exit 1
fi

aws ec2-instance-connect send-ssh-public-key \
  --region "$AWS_REGION" \
  --instance-id "$VPN_INSTANCE_ID" \
  --instance-os-user "$SSH_USER" \
  --ssh-public-key "file://${SSH_KEY}.pub" >/dev/null

SSH_OPTS=(-i "$SSH_KEY" -o StrictHostKeyChecking=no -o ConnectTimeout=20)

scp "${SSH_OPTS[@]}" "$REPO_ROOT/infra/vpn/setup-wireguard-server.sh" "${SSH_USER}@${PUBLIC_IP}:/tmp/setup-wireguard-server.sh"
ssh "${SSH_OPTS[@]}" "${SSH_USER}@${PUBLIC_IP}" "sudo bash /tmp/setup-wireguard-server.sh"

CLIENT_CONFIG="$(mktemp)"
ssh "${SSH_OPTS[@]}" "${SSH_USER}@${PUBLIC_IP}" "sudo cat /etc/wireguard/schick-dev.conf" >"$CLIENT_CONFIG"
CLIENT_CONFIG="$CLIENT_CONFIG" bash "$REPO_ROOT/infra/scripts/upload-vpn-client-config.sh"

rm -f "$CLIENT_CONFIG"
echo "WireGuard configured on $PUBLIC_IP"
