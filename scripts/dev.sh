#!/usr/bin/env bash
set -euo pipefail
# Development mode: run Go node + Next.js dev server concurrently
trap 'kill 0' EXIT
cd "$(dirname "${BASH_SOURCE[0]}")/.."
echo "Starting Next.js dev server..."
(cd web && npm run dev) &
echo "Starting Go node..."
go run ./cmd/xleaks/
