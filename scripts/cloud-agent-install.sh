#!/usr/bin/env bash
# Idempotent Cloud Agent install: refresh Go module cache and local .env.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if [[ ! -f .env ]]; then
  cp .env.example .env
fi

for d in auth product order cart payment notification shared; do
  echo "== go mod download: $d =="
  (cd "$d" && go mod download)
done

echo "cloud-agent-install: ok"
