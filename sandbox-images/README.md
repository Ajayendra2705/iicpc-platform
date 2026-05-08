# Sandbox Build Images

Each language has a fixed Dockerfile under this directory. Contestants do **not** ship a Dockerfile; the platform picks one based on the `language` field in their submission.

## Conventions per language

### Go
- `go.mod` at archive root.
- Build command (run by the platform): `CGO_ENABLED=0 go build ./...`.
- Output binary placed at `${ENTRYPOINT_PATH}` (default `/app/main`).
- Final image: `gcr.io/distroless/static-debian12:nonroot`.

### Rust
- `Cargo.toml` at archive root.
- Build command: `cargo build --release`.
- Output: first executable in `target/release/` (single-binary projects).
- Final image: `gcr.io/distroless/cc-debian12:nonroot`.

### C++
- `Makefile` at archive root that produces `./main` when `make` is run.
- Pre-installed build deps in build stage: `build-essential`, `clang`, `cmake`, `make`, `pkg-config`, `libboost-all-dev`, `libssl-dev`, `zlib1g-dev`.
- Output: `./main` is copied to `${ENTRYPOINT_PATH}`.
- Final image: `gcr.io/distroless/cc-debian12:nonroot`.

## Runtime contract

Every built image must:

1. Listen on TCP port `9100` for contestant traffic (REST/WS/FIX).
2. Accept `SIGTERM` for graceful shutdown.
3. Run as `nonroot` (UID `65532`).
4. Operate read-only; only `/tmp` is writable at runtime.

## Build args

| Arg | Default | Purpose |
|---|---|---|
| `ENTRYPOINT_PATH` | `/app/main` | Where the final binary lands inside the image. Mirrored back to the runtime container. |

## Why no contestant-supplied Dockerfile?

Two reasons: predictable build environments make scoring fair, and disallowing custom Dockerfiles closes a class of escape vectors (privileged builds, host volume mounts, image label tricks). Contestants own their source; the platform owns the image.
