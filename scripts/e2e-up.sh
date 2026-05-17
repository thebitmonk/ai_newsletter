#!/usr/bin/env bash
# Bring up the full local stack for Playwright e2e:
#   docker compose (postgres, redis, nsqd, nsqlookupd, firebase emulator)
#   migrations applied
#   Go backend on :8080 with FIREBASE_AUTH_EMULATOR_HOST set
#   Nuxt dev on :3000 with NUXT_PUBLIC_FIREBASE_AUTH_EMULATOR=true
#
# Stops everything with scripts/e2e-down.sh.
set -euo pipefail

cd "$(dirname "$0")/.."
ROOT="$(pwd)"

echo "→ docker compose up"
docker compose up -d

echo "→ wait for postgres"
until docker exec ai_newsletter_postgres pg_isready -U ai_newsletter >/dev/null 2>&1; do sleep 1; done

echo "→ wait for firebase emulator"
until curl -sf "http://localhost:9099/" >/dev/null 2>&1; do sleep 1; done

echo "→ migrations"
make migrate-up

echo "→ start backend (logs: /tmp/e2e-backend.log)"
(
  set -a; . ./.env; set +a
  export FIREBASE_AUTH_EMULATOR_HOST=localhost:9099
  export FIREBASE_PROJECT_ID="${FIREBASE_PROJECT_ID:-ai-newsletter-dev}"
  go run ./cmd/server
) > /tmp/e2e-backend.log 2>&1 &
echo $! > /tmp/e2e-backend.pid

until curl -sf http://localhost:8080/healthz >/dev/null 2>&1; do sleep 1; done
echo "  backend ready"

echo "→ start nuxt (logs: /tmp/e2e-nuxt.log)"
(
  cd "$ROOT/web"
  NUXT_PUBLIC_FIREBASE_AUTH_EMULATOR=true \
  NUXT_PUBLIC_FIREBASE_API_KEY="${NUXT_PUBLIC_FIREBASE_API_KEY:-emulator-key}" \
  NUXT_PUBLIC_FIREBASE_AUTH_DOMAIN="${NUXT_PUBLIC_FIREBASE_AUTH_DOMAIN:-localhost}" \
  NUXT_PUBLIC_FIREBASE_PROJECT_ID="${FIREBASE_PROJECT_ID:-ai-newsletter-dev}" \
  npm run dev
) > /tmp/e2e-nuxt.log 2>&1 &
echo $! > /tmp/e2e-nuxt.pid

until curl -sf http://localhost:3000/ >/dev/null 2>&1; do sleep 1; done
echo "  nuxt ready"

echo "→ stack up. Run: cd web && FIREBASE_PROJECT_ID=$FIREBASE_PROJECT_ID npx playwright test"
