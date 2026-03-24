#!/usr/bin/env bash
set -euo pipefail
# Development mode: run the Go node with the embedded web UI.
cd "$(dirname "${BASH_SOURCE[0]}")/.."
echo "Starting Go node..."
go run ./cmd/xleaks/
