#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="$REPO_ROOT/docs/api"

mkdir -p "$OUT_DIR"

cd "$REPO_ROOT"

# Pinned binary built by `make doc-tools` from tools/docs/go.mod.
GOMARKDOC="${GOMARKDOC:-$REPO_ROOT/.bin/gomarkdoc}"
if [ ! -x "$GOMARKDOC" ]; then
  echo "gen-docs: $GOMARKDOC missing; run 'make doc-tools'" >&2
  exit 1
fi

# Public packages only. Examples and internal/ are deliberately excluded.
"$GOMARKDOC" \
  --output "$OUT_DIR/API.md" \
  . ./accessibility ./ime ./backend/ebitengine ./backend/gpu

echo "Generated $OUT_DIR/API.md"
