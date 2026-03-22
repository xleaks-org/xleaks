#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
echo "Building frontend..."
(cd web && npm run build)
echo "Building Go binaries for all platforms..."
make build-all
echo "Done."
