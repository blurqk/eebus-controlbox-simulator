#!/bin/sh
# Start ControlBox with frontend and ERC time service in one go:
#   ./start.sh [port]   (default SHIP port 4712, UI at http://localhost:7080/)
# Builds the frontend first if it is missing or older than its sources.
set -e
cd "$(dirname "$0")"

if [ ! -f frontend/dist/index.html ] || \
   [ -n "$(find frontend/src frontend/index.html frontend/package.json frontend/vite.config.ts -newer frontend/dist/index.html 2>/dev/null | head -1)" ]; then
  echo "Building frontend ..."
  (cd frontend && { [ -d node_modules ] || npm install; } && npm run build)
fi

exec go run . "${1:-4712}"
