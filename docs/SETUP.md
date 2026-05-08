# Setup Guide

Fresh-machine instructions for the IICPC platform.

## Prerequisites

| Tool | Version | Install |
|---|---|---|
| Go | 1.22+ | https://go.dev/dl/ or `winget install GoLang.Go` |
| Docker Desktop | 24+ | https://www.docker.com/products/docker-desktop/ |
| `buf` | latest | `go install github.com/bufbuild/buf/cmd/buf@latest` |
| `protoc-gen-go` | latest | `go install google.golang.org/protobuf/cmd/protoc-gen-go@latest` |
| `protoc-gen-go-grpc` | latest | `go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest` |
| `kind` (later, for K8s) | latest | https://kind.sigs.k8s.io/docs/user/quick-start/#installation |
| `kubectl` (later) | latest | https://kubernetes.io/docs/tasks/tools/ |
| `trivy` (optional) | latest | https://aquasecurity.github.io/trivy |

After `go install`, ensure `$GOPATH/bin` (default `~/go/bin` on Linux/macOS, `%USERPROFILE%\go\bin` on Windows) is on `PATH`.

## Docker Desktop registry trust

The local container registry runs at `localhost:5000` without TLS. Docker Desktop trusts `localhost:5000` by default; if you change the address, add it to **Settings → Docker Engine → "insecure-registries"**:

```json
{
  "insecure-registries": ["localhost:5000"]
}
```

Apply & Restart.

## Local dev loop

```bash
# 1. Start dependency containers (MinIO, Postgres+Timescale, Redis, Redpanda, registry)
docker compose up -d

# 2. Regenerate proto bindings (only when .proto files change)
make proto

# 3. Sync the Go workspace
go work sync

# 4. Run tests
go test ./...

# 5. Run the submission service
export BUILDER_KIND=buildkit
export STORE_KIND=postgres
export STORE_DSN="postgres://iicpc:iicpc@localhost:5432/iicpc?sslmode=disable"
export MINIO_ENDPOINT=localhost:9000
export MINIO_ACCESS_KEY=minioadmin
export MINIO_SECRET_KEY=minioadmin
export MINIO_BUCKET=submissions
export REGISTRY_ADDR=localhost:5000
go run ./services/submission-svc
```

Windows PowerShell uses `$env:KEY="value"` instead of `export`.

## Environment variables

| Var | Default | Purpose |
|---|---|---|
| `SUBMISSION_HTTP_ADDR` | `:8081` | HTTP listen address |
| `STORE_KIND` | `memory` | `memory` or `postgres` |
| `STORE_DSN` | `postgres://iicpc:iicpc@localhost:5432/iicpc?sslmode=disable` | Postgres DSN when `STORE_KIND=postgres` |
| `BUILDER_KIND` | `stub` | `stub` (instant) or `buildkit` (real Docker build) |
| `BUILD_TIMEOUT` | `5m` | Per-build timeout |
| `SANDBOX_IMAGE_DIR` | `<cwd>/sandbox-images` | Where Dockerfile.{go,rust,cpp} live |
| `REGISTRY_ADDR` | `localhost:5000` | Registry to push built images to |
| `MINIO_ENDPOINT` | `minio:9000` | MinIO host:port |
| `MINIO_ACCESS_KEY` / `MINIO_SECRET_KEY` | `minioadmin` / `minioadmin` | Object-store credentials |
| `MINIO_BUCKET` | `submissions` | Bucket for source archives |
| `MINIO_USE_SSL` | `false` | TLS toggle |
| `INTERNAL_TOKEN` | (empty = disabled) | Shared secret enforced on all routes except `/healthz` |
| `TRIVY_ENABLED` | `false` | Run Trivy scan after each build (requires `trivy` on PATH) |

## Ports

| Service | Port | Purpose |
|---|---|---|
| MinIO API | 9000 | S3-compatible object store |
| MinIO Console | 9001 | Browser UI |
| Postgres + Timescale | 5432 | Submission metadata + time-series |
| Redis | 6379 | Live leaderboard |
| Redpanda | 9092 | Kafka-API event bus |
| Registry | 5000 | Local container registry |
| Submission svc | 8081 | HTTP upload API |
| Contestant pods | 9100 | Order traffic (REST/WS/FIX) |

## Smoke test

```bash
# package the smoke-go sample
cd samples/smoke-go && tar -czf ../../smoke-go.tar.gz . && cd ../..

curl -X POST http://localhost:8081/submissions \
  -F "contestant_id=team1" \
  -F "language=go" \
  -F "entrypoint=/app/main" \
  -F "archive=@smoke-go.tar.gz"
```

Wait ~30-90s on first build (image pulls), then:

```bash
curl http://localhost:8081/submissions/<id>
```

Expect `"status":"ready"` plus `"image_uri":"localhost:5000/contestants/<id>:latest"`.

## Cleanup

```bash
docker compose down       # stop containers, keep volumes
docker compose down -v    # also delete volumes (fresh state)
```
