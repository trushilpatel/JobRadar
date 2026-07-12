#!/usr/bin/env bash
# Dry-runs the exact sequence the `e2e` CI job (.github/workflows/ci.yml)
# executes, but locally via your own `doppler login` session instead of
# DOPPLER_TOKEN. Use this to validate the CI job's steps actually work
# before trusting them in GitHub Actions.
#
# Usage: ./scripts/ci-e2e-local.sh
#
# Requires: doppler CLI logged in, Go, Node, and the frontend's
# node_modules already installed (npm ci in frontend/ if not).

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
API_LOG="$(mktemp)"
VITE_LOG="$(mktemp)"
API_PID=""
VITE_PID=""

cleanup() {
  echo "--- cleanup ---"
  [ -n "$API_PID" ] && kill "$API_PID" 2>/dev/null || true
  [ -n "$VITE_PID" ] && kill "$VITE_PID" 2>/dev/null || true
  lsof -ti:8080 2>/dev/null | xargs -r kill 2>/dev/null || true
  lsof -ti:5173 2>/dev/null | xargs -r kill 2>/dev/null || true
  echo "API log:  $API_LOG"
  echo "Vite log: $VITE_LOG"
}
trap cleanup EXIT

echo "--- stopping any stale :8080/:5173 processes ---"
lsof -ti:8080 2>/dev/null | xargs -r kill 2>/dev/null || true
lsof -ti:5173 2>/dev/null | xargs -r kill 2>/dev/null || true

echo "--- run DB migrations ---"
(cd "$REPO_ROOT/backend" && doppler run --config dev -- go run ./apps/migrate)

echo "--- start API server ---"
(cd "$REPO_ROOT/backend" && doppler run --config dev -- go run ./apps/api) > "$API_LOG" 2>&1 &
API_PID=$!

echo "--- start Vite dev server ---"
(cd "$REPO_ROOT/frontend" && doppler run --config dev -- npm run dev) > "$VITE_LOG" 2>&1 &
VITE_PID=$!

echo "--- wait for API and Vite to be ready ---"
API_UP=""
VITE_UP=""
for _ in $(seq 1 30); do
  curl -sf http://localhost:9100/health > /dev/null && API_UP=1 && break
  sleep 2
done
for _ in $(seq 1 30); do
  curl -sf http://localhost:5173/ > /dev/null && VITE_UP=1 && break
  sleep 2
done
if [ -z "$API_UP" ] || [ -z "$VITE_UP" ]; then
  echo "API up: ${API_UP:-no}, Vite up: ${VITE_UP:-no}"
  echo "Stack failed to come up in time — see $API_LOG / $VITE_LOG"
  exit 1
fi
echo "API and Vite are up."

echo "--- install Playwright browsers ---"
(cd "$REPO_ROOT/frontend" && npx playwright install --with-deps chromium)

echo "--- run E2E suite ---"
(cd "$REPO_ROOT/frontend" && npm run test:e2e)
