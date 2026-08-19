#!/usr/bin/env bash
# Per-boot Cloud Agent start: dockerd + Docker Compose stack with readiness check.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if [[ ! -f .env ]]; then
  cp .env.example .env
fi

# Ensure Docker daemon.json for nested Firecracker VMs (idempotent).
if [[ ! -f /etc/docker/daemon.json ]]; then
  sudo mkdir -p /etc/docker
  sudo tee /etc/docker/daemon.json >/dev/null <<'JSON'
{
  "storage-driver": "fuse-overlayfs",
  "features": {
    "containerd-snapshotter": false
  }
}
JSON
fi

# Prefer iptables-legacy when available (Docker-in-Firecracker).
if [[ -x /usr/sbin/iptables-legacy ]]; then
  sudo update-alternatives --set iptables /usr/sbin/iptables-legacy >/dev/null 2>&1 || true
  sudo update-alternatives --set ip6tables /usr/sbin/ip6tables-legacy >/dev/null 2>&1 || true
fi

if ! sudo docker info >/dev/null 2>&1; then
  sudo dockerd >/tmp/dockerd.log 2>&1 &
  for _ in $(seq 1 60); do
    if sudo docker info >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done
fi

if ! sudo docker info >/dev/null 2>&1; then
  echo "cloud-agent-start: dockerd failed to become ready" >&2
  tail -n 50 /tmp/dockerd.log >&2 || true
  exit 1
fi

sudo docker info 2>/dev/null | grep -qi 'fuse-overlayfs' || {
  echo "cloud-agent-start: warning: storage driver is not fuse-overlayfs" >&2
  sudo docker info 2>/dev/null | grep -i 'Storage Driver' >&2 || true
}

# Reconcile the full local stack. --build is cheap when layers are cached.
sudo docker compose up -d --build

echo "cloud-agent-start: waiting for gateway health..."
for _ in $(seq 1 90); do
  if curl -fsS http://localhost:8080/gateway/health >/dev/null 2>&1; then
    echo "cloud-agent-start: gateway healthy"
    exit 0
  fi
  sleep 2
done

echo "cloud-agent-start: gateway not healthy in time" >&2
sudo docker compose ps >&2 || true
exit 1
