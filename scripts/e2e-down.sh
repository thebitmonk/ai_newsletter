#!/usr/bin/env bash
# Tears down what e2e-up.sh started.
set -euo pipefail

cd "$(dirname "$0")/.."

for pidfile in /tmp/e2e-backend.pid /tmp/e2e-nuxt.pid; do
  if [ -f "$pidfile" ]; then
    pid=$(cat "$pidfile")
    kill "$pid" 2>/dev/null || true
    rm -f "$pidfile"
  fi
done
pkill -f "cmd/server" 2>/dev/null || true
pkill -f "nuxt dev" 2>/dev/null || true

docker compose down
echo "→ stack down"
