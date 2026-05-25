#!/usr/bin/env bash
# scripts/build.sh — full local build (frontend → Go binary).
#
# Usage: ./scripts/build.sh [--skip-web]
#
# Output: bin/minidon (statically linked, frontend assets embedded)
#
# Prerequisites:
#   - Go 1.26+ on PATH
#   - Node 20+ and npm on PATH (unless --skip-web is passed)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SKIP_WEB=false

for arg in "$@"; do
  case "$arg" in
    --skip-web) SKIP_WEB=true ;;
    *) echo "Unknown argument: $arg"; exit 1 ;;
  esac
done

# ── Frontend build ────────────────────────────────────────────────────────────
if [ "$SKIP_WEB" = false ]; then
  echo "Building frontend (web/) …"
  cd "$REPO_ROOT/web"
  npm ci --silent
  npm run build
  echo "Frontend built → web/dist/"
fi

# ── Go build ─────────────────────────────────────────────────────────────────
echo "Building Go binary …"
cd "$REPO_ROOT"
mkdir -p bin
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/minidon ./cmd/minidon

echo ""
echo "Build complete → bin/minidon"
echo "Run with: MINIDON_MASTODON_INSTANCE=https://mstdn.social ./bin/minidon"
