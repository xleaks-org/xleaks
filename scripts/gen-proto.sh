#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "Generating protobuf Go code..."
protoc \
  --go_out="$ROOT_DIR/proto/gen" \
  --go_opt=paths=source_relative \
  -I "$ROOT_DIR/proto" \
  "$ROOT_DIR/proto/messages.proto"

echo "Done."
