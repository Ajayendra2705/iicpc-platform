#!/usr/bin/env bash
set -euo pipefail

# Regenerate Go bindings from .proto files.
# Requires: protoc, protoc-gen-go, protoc-gen-go-grpc

cd "$(dirname "$0")/.."

OUT_BASE="proto"

for proto in proto/*.proto; do
  echo "Generating: ${proto}"
  protoc \
    --go_out=. --go_opt=module=github.com/iicpc/platform \
    --go-grpc_out=. --go-grpc_opt=module=github.com/iicpc/platform \
    "${proto}"
done

echo "Done. Generated .pb.go files are gitignored; run this on each clone."
