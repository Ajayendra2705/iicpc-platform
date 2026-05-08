#!/usr/bin/env bash
set -euo pipefail

# Bring up local dev dependencies (MinIO, Postgres+Timescale, Redis, Redpanda)
# without Kubernetes. Use this for fast iteration; the kind cluster is for
# integration testing.

cd "$(dirname "$0")/.."

if ! command -v docker >/dev/null 2>&1; then
  echo "docker not installed" >&2
  exit 1
fi

docker compose up -d

echo ""
echo "Dependencies up:"
echo "  MinIO:    http://localhost:9001 (minioadmin / minioadmin)"
echo "  Postgres: postgres://iicpc:iicpc@localhost:5432/iicpc"
echo "  Redis:    localhost:6379"
echo "  Redpanda: localhost:9092"
