#!/usr/bin/env bash
# scripts/dev.sh — start the Go backend and Vite dev server concurrently.
#
# Usage: ./scripts/dev.sh
#
# Prerequisites:
#   - Go 1.26+ on PATH
#   - Node 20+ and npm on PATH
#   - A running MeiliSearch instance (or start via docker compose)
#
# TODO: replace manual process management with a tool like 'mage' or
# 'overmind'/'foreman' once the binary is runnable.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# ── Cleanup on exit ───────────────────────────────────────────────────────────
cleanup() {
  echo ""
  echo "Stopping dev servers…"
  kill "$GO_PID" "$VITE_PID" 2>/dev/null || true
  wait "$GO_PID" "$VITE_PID" 2>/dev/null || true
}
trap cleanup INT TERM EXIT

# ── Vite dev server ───────────────────────────────────────────────────────────
echo "Starting Vite dev server on http://localhost:5173 …"
cd "$REPO_ROOT/web"
npm install --silent
npm run dev &
VITE_PID=$!

# ── Go backend ────────────────────────────────────────────────────────────────
# TODO: replace with `go run` + `air` (or equivalent) for hot reload once
# business logic is implemented.
echo "Starting Go backend on http://localhost:8080 …"
cd "$REPO_ROOT"
go run ./cmd/minidon &
GO_PID=$!

echo ""
echo "Dev servers running — open http://localhost:5173 in your browser."
echo "Press Ctrl+C to stop."

wait
