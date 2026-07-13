#!/usr/bin/env bash
# Bootstrap a fresh Ubuntu EC2 instance for Dupli1 single-box deployment.
set -euo pipefail

DUPLI1_HOME="${DUPLI1_HOME:-/opt/dupli1}"
DUPLI1_REPO="${DUPLI1_REPO:-https://github.com/elug3/dupli1.git}"
DUPLI1_BRANCH="${DUPLI1_BRANCH:-main}"
SECRETS_DIR="${DUPLI1_SECRETS_DIR:-${DUPLI1_HOME}/secrets}"

log() { echo "[ec2-bootstrap] $*"; }

if [[ "${EUID}" -ne 0 ]]; then
  echo "run as root (e.g. sudo bash $0)" >&2
  exit 1
fi

log "Installing Docker..."
apt-get update -qq
apt-get install -y -qq ca-certificates curl gnupg jq openssl git
install -m 0755 -d /etc/apt/keyrings
if [[ ! -f /etc/apt/keyrings/docker.gpg ]]; then
  curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
  chmod a+r /etc/apt/keyrings/docker.gpg
  echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" \
    > /etc/apt/sources.list.d/docker.list
  apt-get update -qq
fi
apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-compose-plugin
systemctl enable --now docker
usermod -aG docker "${SUDO_USER:-ubuntu}" 2>/dev/null || true

log "Preparing ${DUPLI1_HOME}..."
mkdir -p "$DUPLI1_HOME" "$SECRETS_DIR"
chmod 700 "$SECRETS_DIR"

if [[ ! -d "${DUPLI1_HOME}/app/.git" ]]; then
  log "Cloning ${DUPLI1_REPO}..."
  git clone --branch "$DUPLI1_BRANCH" "$DUPLI1_REPO" "${DUPLI1_HOME}/app"
else
  log "Updating existing repo..."
  git -C "${DUPLI1_HOME}/app" fetch origin "$DUPLI1_BRANCH"
  git -C "${DUPLI1_HOME}/app" checkout "$DUPLI1_BRANCH"
  git -C "${DUPLI1_HOME}/app" pull origin "$DUPLI1_BRANCH"
fi

if [[ ! -f "${SECRETS_DIR}/jwt_private_key.pem" ]]; then
  log "Generating persistent RS256 JWT key..."
  openssl genrsa -out "${SECRETS_DIR}/jwt_private_key.pem" 2048
  chmod 600 "${SECRETS_DIR}/jwt_private_key.pem"
fi

if [[ ! -f "${DUPLI1_HOME}/app/.env.prod" ]]; then
  log "Creating .env.prod from template — edit secrets before going live."
  cp "${DUPLI1_HOME}/app/.env.prod.example" "${DUPLI1_HOME}/app/.env.prod"
  POSTGRES_PW="$(openssl rand -hex 24)"
  JWT_SEC="$(openssl rand -hex 32)"
  OWNER_PW="$(openssl rand -hex 16)"
  WEB_SVC_PW="$(openssl rand -hex 16)"
  ORDER_SVC_PW="$(openssl rand -hex 16)"
  MINIO_SEC="$(openssl rand -hex 24)"
  sed -i "s/change-me-strong-postgres-password/${POSTGRES_PW}/" "${DUPLI1_HOME}/app/.env.prod"
  sed -i "s/change-me-strong-jwt-secret/${JWT_SEC}/" "${DUPLI1_HOME}/app/.env.prod"
  sed -i "s/change-me-owner-password/${OWNER_PW}/" "${DUPLI1_HOME}/app/.env.prod"
  sed -i "s/change-me-web-service-password/${WEB_SVC_PW}/" "${DUPLI1_HOME}/app/.env.prod"
  sed -i "s/change-me-order-service-password/${ORDER_SVC_PW}/" "${DUPLI1_HOME}/app/.env.prod"
  sed -i "s/change-me-minio-secret/${MINIO_SEC}/" "${DUPLI1_HOME}/app/.env.prod"
  chmod 600 "${DUPLI1_HOME}/app/.env.prod"
fi

# Auto-deploy after bootstrap when requested (e.g. EC2 user-data).
if [[ "${DUPLI1_AUTO_DEPLOY:-}" == "1" ]]; then
  log "Running deploy-ec2.sh..."
  bash "${DUPLI1_HOME}/app/infra/scripts/deploy-ec2.sh"
fi

log "Bootstrap complete."
chown -R "${SUDO_USER:-ubuntu}:docker" "$DUPLI1_HOME" 2>/dev/null || chown -R ubuntu:ubuntu "$DUPLI1_HOME" 2>/dev/null || true
log "Next: edit ${DUPLI1_HOME}/app/.env.prod (owner password, service accounts, MinIO secret)"
log "Then:  bash ${DUPLI1_HOME}/app/infra/scripts/deploy-ec2.sh"
