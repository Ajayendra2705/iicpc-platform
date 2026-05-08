# Sandbox Build Images

Each language has a fixed Dockerfile under this directory. Contestants do **not** ship a Dockerfile; the platform picks one based on the `language` field in their submission.

## Conventions per language

### Go
- `go.mod` at archive root.
- Build command: `CGO_ENABLED=0 go build ./...`.
- Output: single binary copied to `${ENTRYPOINT_PATH}` (default `/app/main`).
- Final image: `gcr.io/distroless/static-debian12:nonroot`.

### Rust
- `Cargo.toml` at archive root.
- Build command: `cargo build --release`.
- The platform parses `cargo`'s JSON output and copies the last `compiler-artifact` whose `executable` field is set. Multi-bin crates: only the last bin is taken.
- Final image: `gcr.io/distroless/cc-debian12:nonroot`.

### C++
- Either a `Makefile` or a `CMakeLists.txt` at archive root.
- The platform runs `make` or `cmake --build build` automatically.
- Output binary must end up at one of: `./main`, `build/main`, `bin/main`.
- Pre-installed build deps: `build-essential`, `clang`, `cmake`, `make`, `pkg-config`, `libboost-all-dev`, `libssl-dev`, `zlib1g-dev`.
- Final image: `gcr.io/distroless/cc-debian12:nonroot`.

## Runtime contract

Every built image must:

1. Listen on TCP port `${RUNTIME_PORT}` (default `9100`) for contestant traffic (REST/WS/FIX). The port is exposed as both a Docker `EXPOSE` directive and the `RUNTIME_PORT` env var; the contestant binary should bind to `:${RUNTIME_PORT}` rather than hardcoding `9100`.
2. Expose `GET /health` returning `200 OK` for K8s readiness/liveness probes.
3. Accept `SIGTERM` for graceful shutdown.
4. Run as `nonroot` (UID `65532`).
5. Operate read-only; only `/tmp` is writable at runtime.

## Build args

| Arg | Default | Purpose |
|---|---|---|
| `ENTRYPOINT_PATH` | `/app/main` | Where the built binary lands inside the runtime image. |
| `RUNTIME_PORT` | `9100` | Port the contestant binary should listen on; mirrored into the image as an env var. |

## Why no contestant-supplied Dockerfile?

Two reasons: predictable build environments make scoring fair, and disallowing custom Dockerfiles closes a class of escape vectors (privileged builds, host volume mounts, image label tricks). Contestants own their source; the platform owns the image.
