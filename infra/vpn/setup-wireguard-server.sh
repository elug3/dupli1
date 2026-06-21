#!/usr/bin/env bash
set -euo pipefail

# Configures WireGuard on the internal-vpn EC2 host.
# Run on the VPN server as root (or via SSH):
#   sudo bash setup-wireguard-server.sh

WG_PORT="${WG_PORT:-51820}"
WG_INTERFACE="${WG_INTERFACE:-wg0}"
WG_SERVER_IP="${WG_SERVER_IP:-10.8.0.1/24}"
WG_CLIENT_IP="${WG_CLIENT_IP:-10.8.0.2/32}"
WG_DIR="/etc/wireguard"
CLIENT_NAME="${CLIENT_NAME:-schick-dev}"

if [[ "${EUID}" -ne 0 ]]; then
  echo "run as root" >&2
  exit 1
fi

install_packages() {
  if command -v dnf >/dev/null 2>&1; then
    dnf install -y wireguard-tools
  elif command -v apt-get >/dev/null 2>&1; then
    apt-get update
    apt-get install -y wireguard
  else
    echo "unsupported package manager" >&2
    exit 1
  fi
}

detect_public_endpoint() {
  if [[ -n "${WG_ENDPOINT:-}" ]]; then
    return
  fi
  local token
  token="$(curl -fsS -X PUT "http://169.254.169.254/latest/api/token" \
    -H "X-aws-ec2-metadata-token-ttl-seconds: 21600")"
  WG_ENDPOINT="$(curl -fsS "http://169.254.169.254/latest/meta-data/public-ipv4" \
    -H "X-aws-ec2-metadata-token: $token")"
}

write_server_config() {
  mkdir -p "$WG_DIR"
  chmod 700 "$WG_DIR"
  cd "$WG_DIR"

  if [[ ! -f server.key ]]; then
    umask 077
    wg genkey | tee server.key | wg pubkey > server.pub
  fi

  if [[ ! -f "${CLIENT_NAME}.key" ]]; then
    umask 077
    wg genkey | tee "${CLIENT_NAME}.key" | wg pubkey > "${CLIENT_NAME}.pub"
  fi

  SERVER_PRIVATE_KEY="$(cat server.key)"
  CLIENT_PUBLIC_KEY="$(cat "${CLIENT_NAME}.pub")"

  cat >"${WG_INTERFACE}.conf" <<EOF
[Interface]
Address = ${WG_SERVER_IP}
ListenPort = ${WG_PORT}
PrivateKey = ${SERVER_PRIVATE_KEY}
SaveConfig = false

PostUp = sysctl -w net.ipv4.ip_forward=1
PostDown = sysctl -w net.ipv4.ip_forward=0

[Peer]
# ${CLIENT_NAME}
PublicKey = ${CLIENT_PUBLIC_KEY}
AllowedIPs = ${WG_CLIENT_IP}
EOF

  chmod 600 "${WG_INTERFACE}.conf"
}

write_client_config() {
  SERVER_PUBLIC_KEY="$(cat server.pub)"
  CLIENT_PRIVATE_KEY="$(cat "${CLIENT_NAME}.key")"

  cat >"${WG_DIR}/${CLIENT_NAME}.conf" <<EOF
[Interface]
PrivateKey = ${CLIENT_PRIVATE_KEY}
Address = ${WG_CLIENT_IP}
DNS = 10.0.0.2

[Peer]
PublicKey = ${SERVER_PUBLIC_KEY}
Endpoint = ${WG_ENDPOINT}:${WG_PORT}
AllowedIPs = 10.0.0.0/16, 10.8.0.0/24
PersistentKeepalive = 25
EOF

  chmod 600 "${WG_DIR}/${CLIENT_NAME}.conf"
}

enable_service() {
  sysctl -w net.ipv4.ip_forward=1
  cat >/etc/sysctl.d/99-wireguard-forwarding.conf <<EOF
net.ipv4.ip_forward = 1
EOF

  systemctl enable "wg-quick@${WG_INTERFACE}"
  systemctl restart "wg-quick@${WG_INTERFACE}"
}

main() {
  install_packages
  detect_public_endpoint
  write_server_config
  write_client_config
  enable_service
  wg show
  echo
  echo "Client config written to ${WG_DIR}/${CLIENT_NAME}.conf"
  echo "Endpoint: ${WG_ENDPOINT}:${WG_PORT}"
}

main "$@"
