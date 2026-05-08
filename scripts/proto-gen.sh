#!/usr/bin/env bash
set -euo pipefail

# Regenerate Go bindings from .proto files using buf.
# Requires: buf, protoc-gen-go, protoc-gen-go-grpc on PATH.
# Install:
#   go install github.com/bufbuild/buf/cmd/buf@latest
#   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

cd "$(dirname "$0")/.."

if ! command -v buf >/dev/null 2>&1; then
  echo "buf not found on PATH. Install: go install github.com/bufbuild/buf/cmd/buf@latest" >&2
  exit 1
fi

echo "Generating Go protobuf bindings..."
buf generate

echo "Done. Output under proto/gen/go/"
