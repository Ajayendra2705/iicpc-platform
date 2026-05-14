# IICPC Hackathon — Handoff Document

> **Read this first if you are a new Claude (or new dev) picking up this project.**
> This file is the single source of truth for project state, decisions, conventions, and what's next.
> Last updated after Day 5 completion. **Day 6 not yet started.**

**GitHub:** https://github.com/Ajayendra2705/iicpc-platform (private). Default branch: `main`. Local working branch: `main`.
**CI:** GitHub Actions, all jobs green as of commit `018b413` (Day 4; Day 5 adds no CI-breaking changes). Workflow file: `.github/workflows/ci.yml`. Per-module matrix (test + golangci-lint) + buf lint.

---

## 1. Project Overview

**Hackathon:** IICPC Summer Hackathon 2026 (May 9 – June 10, 2026, ~32 days). Solo developer (1 person).

**Deliverable:** A "Distributed Benchmarking and Hosting Platform" that lets contestants submit trading-infrastructure code (orderbook / matching engine), runs it in an isolated sandbox, drives synthetic load (HFT-style FIX/REST/WS bot fleet), measures latency & correctness, and ranks contestants on a public leaderboard.

**Key qualities the judges will score on:**
- Multi-protocol load generation (REST + WebSocket + FIX 4.4)
- Realistic order arrival distributions (Poisson)
- Nanosecond-resolution latency measurement (HDR histograms — P50/P90/P99)
- Strong sandbox isolation (gVisor + cgroup limits + NetworkPolicy + restricted PSA)
- Reproducibility, observability, demoability

**Brief / pipeline source-of-truth:** `IDEATION_IMPLEMENTATION_PIPELINE.md` (untracked but in repo root).
**Day-by-day plan:** `PLAN.md` (committed).

---

## 2. User / Collaboration Profile

The user driving this project:
- **Solo developer**, Windows 11, **PowerShell** primary shell.
- **Runs commands themselves** — does NOT delegate terminal control to Claude. Claude proposes commands; user executes and pastes back output.
- **Prefers caveman mode** (terse, drop articles, fragments OK). Code/commits/PRs/security warnings still written normal. The session has a `caveman` skill with `lite|full|ultra` levels.
- **Time-pressed** — 32 days, solo. Scope cuts aggressively. No premature optimization.
- Treats response density as a feature. No filler, no recaps unless asked.

**Interaction protocol:**
- User says `"go"` → execute the next planned day/step.
- User says `"do not start X"` → respect literally. Confirm only.
- User pastes terminal output → diagnose from it; do not re-run unless told.

---

## 3. Stack & Architecture (Decisions, Locked)

| Layer | Choice | Why |
|---|---|---|
| Language | **Go 1.22+** everywhere | Solo dev; one toolchain; good gRPC/k8s ergonomics. Rejected: Rust hot path. |
| Repo | **Monorepo** with `go.work` (10 modules) | Cross-service refactor speed. ADR-0001. |
| Proto | **buf** toolchain, `proto/<svc>/v1/<svc>.proto` | Versioned dirs avoid Go pkg collision. |
| RPC | **gRPC** (services) + REST (contestant-facing) | gRPC for internal, REST for ingest. |
| Storage | **MinIO** (S3-compat) for source archives + built images metadata | Local + AWS-compat. |
| DB | **PostgreSQL + TimescaleDB** | Submissions + telemetry time-series. |
| Cache/Leaderboard | **Redis** (ZSET) | O(log N) ranked leaderboard. |
| Event bus | **Redpanda** (Kafka API) | Lower ops than Kafka. ADR-0003. |
| Build | **`docker build` shelled out** prototype → **Kaniko Job** production | ADR-0004. Same `Builder` interface. |
| Registry | Local Docker registry on `:5000` (dev) | Prod: ECR. |
| Image scan | **Trivy** (optional, no-op if not on PATH) | Cheap supply-chain check. |
| Orchestration | **Kubernetes**, **kind** local, **EKS** planned | Manifests in `infra/manifests/`. |
| Sandbox | **gVisor** runtimeClass + cgroup v2 + NetworkPolicy + restricted PSA + distroless images | Defense-in-depth. ADR-0002. |
| Logging | `log/slog` JSON handler + RequestID middleware | Structured, greppable. |
| FIX | QuickFIX-Go (planned for bots) | Rejected: custom FIX encoder. |
| CI | GitHub Actions: vet + test (-race) + build per module + buf lint + golangci-lint | `.github/workflows/ci.yml`. |

**Architecture diagram:** `docs/ARCHITECTURE.md` (ASCII). **ADRs:** `docs/ADR/0001..0004`.

---

## 4. Repository Layout

```
.
├── PLAN.md                       # 32-day plan
├── HANDOFF.md                    # this file
├── README.md
├── IDEATION_IMPLEMENTATION_PIPELINE.md  # original brief (untracked)
├── buf.yaml / buf.gen.yaml       # at repo root (NOT inside proto/)
├── go.work / go.work.sum
├── docker-compose.yaml           # 5 services: minio, postgres+timescale, redis, redpanda, registry:5000
├── .github/workflows/ci.yml
├── .golangci.yml
├── Makefile
├── docs/
│   ├── ARCHITECTURE.md
│   ├── SETUP.md                  # prereqs, env vars, ports map, smoke test
│   └── ADR/0001-0004-*.md
├── infra/
│   ├── kind/cluster.yaml         # 4 nodes: cp + workers labelled services/contestants/bots
│   └── manifests/{submission-svc,minio}.yaml  # restricted PSA, hardened securityContext
├── proto/
│   ├── {submission,telemetry,botcontrol,leaderboard}/v1/*.proto
│   └── gen/go/                   # buf generates here; committed
├── sandbox-images/Dockerfile.{go,rust,cpp}
├── samples/smoke-go/             # smallest contestant satisfying runtime contract
├── samples/reference-orderbook/  # Day 5: full heap-based price-time priority orderbook
├── scripts/{dev-up,kind-up,proto-gen}.sh
└── services/
    ├── api-gateway/             (skeleton, Day 6)
    ├── submission-svc/          ✅ done (Days 1-3)
    ├── sandbox-runner/          (skeleton, Day 4 next)
    ├── bot-coordinator/         (skeleton, Days 8-14)
    ├── bot-worker/              (skeleton, Days 8-14)
    ├── telemetry-ingester/      (skeleton, Days 15-21)
    ├── aggregator/              (skeleton, Days 15-21)
    ├── validator/               (skeleton, Days 15-21)
    └── leaderboard-svc/         (skeleton, Days 15-21)
```

Each service is its own Go module (independent `go.mod`); workspace ties them together.

---

## 5. Day-by-Day Status

| Day | Scope | Status | Commit |
|---|---|---|---|
| 1 | Bootstrap monorepo, `go.work`, all 9 service skeletons, proto contracts, ADRs, kind cluster spec, Compose | ✅ done | `ba5a3ef` |
| 2 | submission-svc: HTTP upload endpoint, MinIO storage, in-memory store, stub builder, validation | ✅ done | `2ef477e` |
| 3 | Real Docker builder (`buildkit.go`): MinIO download → tar extract → `docker build` w/ sandbox Dockerfile → push to local registry. Sandbox Dockerfiles for Go/Rust/C++. Smoke-go sample. | ✅ done | `63ccbba` |
| 3.5 | **Tech-debt sweep:** 20 audit issues fixed (auth, gzip sniff, queue 503, Postgres backend, retry, drain, Trivy, CI, slog, ...). | ✅ done | `0158c3d` |
| 3.6 | buf lint cleanup (drop `iicpc.` pkg prefix, move buf.yaml to root, naming-rule exclusions) | ✅ done | `57f3de1` |
| 3.7 | HANDOFF.md added | ✅ done | `04a7079` |
| 3.8 | GitHub repo created + pushed; CI workspace-mode + proto-fmt fixes (per-module matrix; reorder option/import; collapse comment spacing); .gitignore tightened | ✅ done | `cf4a017` |
| 3.9 | staticcheck fixes for submission-svc (drop deprecated `tar.TypeRegA`; restructure trivy nil-check to compare concrete `*TrivyScanner` before interface assignment) — **CI fully green** | ✅ done | `117baca` |
| **4** | **sandbox-runner:** K8s pod spawn with gVisor runtimeClassName, NetworkPolicy isolation, cgroup resource limits | ✅ done | `018b413` |
| **5** | **Reference orderbook:** heap-based price-time priority matching engine, HTTP server, 8 unit tests | ✅ done | `f53a549` |
| **6** | api-gateway (JWT, rate limiting, routing) | ⏳ **next** |  |
| 7 | Buffer / catch-up | pending |  |
| 7 | Buffer / catch-up | pending |  |
| 8-14 | Bot fleet: bot-worker + bot-coordinator, REST/WS/FIX, Poisson order gen | pending |  |
| 15-21 | Telemetry: ingester + aggregator (HDR histograms) + validator + leaderboard (Redis ZSET) | pending |  |
| 22-28 | Frontend (Next.js), Terraform AWS, Helm charts, chaos tests | pending |  |
| 29-32 | Docs polish, demo video, submission | pending |  |

**Current branch:** `main`. **Default PR base:** `main`.

---

## 6. submission-svc — Detailed State (the only "done" service)

**Entry:** `services/submission-svc/main.go`. Loads config from env, picks builder + store, runs HTTP server, drains workers on SIGTERM.

**Config env vars:**
| Var | Default | Notes |
|---|---|---|
| `HTTP_ADDR` | `:8080` |  |
| `BUILDER_KIND` | `stub` | `stub` or `buildkit` |
| `STORE_KIND` | `memory` | `memory` or `postgres` |
| `POSTGRES_DSN` | — | required if STORE_KIND=postgres |
| `MINIO_ENDPOINT` | `localhost:9000` |  |
| `MINIO_ACCESS_KEY` / `MINIO_SECRET_KEY` | `minioadmin` / `minioadmin` |  |
| `MINIO_BUCKET` | `submissions` |  |
| `MINIO_USE_SSL` | `false` |  |
| `REGISTRY_HOST` | `localhost:5000` |  |
| `INTERNAL_TOKEN` | empty | when set, required `Authorization: Bearer <token>` (skips `/healthz`) |
| `MAX_UPLOAD_BYTES` | `52428800` | 50 MB |
| `BUILD_QUEUE_SIZE` | `64` |  |
| `BUILD_WORKERS` | `2` |  |
| `PUSH_ATTEMPTS` | `3` | exp backoff retries on `docker push` |
| `MAX_EXTRACTED_BYTES` | `524288000` | 500 MB tar bomb cap |
| `MAX_FILE_BYTES` | `104857600` | 100 MB per-file cap |
| `TRIVY_ENABLED` | `false` | optional image scan |
| `RUNTIME_PORT` | `9100` | sandbox build-arg |
| `SHUTDOWN_TIMEOUT` | `30s` |  |

**Routes:**
- `POST /submissions` — multipart upload (`file` = .tar.gz, `lang`, `entrypoint`, `contestant_id`)
- `GET /submissions/{id}` — status + image URL
- `GET /submissions` — list
- `GET /healthz`

**Pipeline:**
1. HTTP handler validates form fields + path-traversal + gzip magic-byte sniff (`1f 8b`).
2. `MaxBytesReader` caps at 50 MB.
3. Source archive uploaded to MinIO `s3://submissions/<contestant_id>/<id>/source.tar.gz`.
4. Submission row inserted (status=`queued`).
5. Build job enqueued. Returns `ErrQueueFull` → HTTP 503 if backlog full.
6. Worker pulls: download → tar-extract (capped, traversal-safe, link entries skipped) → `docker build` w/ sandbox Dockerfile (build args `ENTRYPOINT_PATH`, `RUNTIME_PORT`) → tag → `docker push` w/ exp backoff (default 3 attempts) → optional Trivy scan → mark `ready`.
7. In-flight builds tracked; cancelled on shutdown signal.

**Middleware chain (in order):** `RequestID` → `AccessLog` → `InternalToken` → router. In `services/submission-svc/internal/httpx/middleware.go`.

**Tests:** 16 across 4 packages (build/, server/, store/, validation/). All pass.
- `buildkit_test.go`: `KeyFromSourceURI`, `Truncate`, `DownloadAndExtractHappy`, `RejectsTooLarge`, `RejectsPerFileCap`, `RejectsPathTraversal`, `EnqueueQueueFull`, `PushWithRetrySucceedsOnSecondAttempt`.

**Sandbox Dockerfiles** (`sandbox-images/Dockerfile.{go,rust,cpp}`):
- Multi-stage; distroless final.
- Go: `static-debian12:nonroot`. Rust/C++: `cc-debian12:nonroot`.
- C++: accepts Makefile OR CMakeLists.txt; probes `./main`, `build/main`, `bin/main`.
- Rust: parses `cargo build --message-format=json` for executable detection.
- Build args: `ENTRYPOINT_PATH`, `RUNTIME_PORT` (default 9100).

---

## 7. Tech-Debt Audit — All 20 Items Status

| # | Issue | Status |
|---|---|---|
| 1 | Tar bomb risk | ✅ 500 MB total + 100 MB/file caps |
| 2 | No gzip magic-byte sniff | ✅ peek `1f 8b` before MinIO write |
| 3 | Proto Go code never generated | ✅ buf toolchain + 8 .pb.go committed |
| 4 | No auth on submission-svc | ✅ `INTERNAL_TOKEN` middleware |
| 5 | Queue silently drops | ✅ `ErrQueueFull` → HTTP 503 |
| 6 | In-memory store loses state | ✅ Postgres backend, `STORE_KIND` env |
| 7 | No build-log streaming | ⚠ deferred (proto declares; impl post-Day-4) |
| 8 | No Trivy scan | ✅ optional `TRIVY_ENABLED=true` |
| 9 | No CI | ✅ `.github/workflows/ci.yml` |
| 10 | No structured logs / req IDs | ✅ slog JSON + `RequestID` middleware |
| 11 | No retry on `docker push` | ✅ exp backoff, 3 attempts |
| 12 | C++ Dockerfile brittle | ✅ Makefile OR CMake, probes 3 paths |
| 13 | Rust Dockerfile brittle | ✅ parses `cargo --message-format=json` |
| 14 | Buildkit `process()` untested | ✅ tar cases + queue + retry tests |
| 15 | Registry trust undocumented | ✅ noted in `docs/SETUP.md` |
| 16 | Runtime port hardcoded | ✅ `RUNTIME_PORT` build-arg + env |
| 17 | No graceful drain on SIGTERM | ✅ cancel in-flight + worker drain |
| 18 | Smoke-go missing `/health` | ✅ added |
| 19 | PLAN.md outdated re smoke-go | ⚠ minor doc rot, cosmetic |
| 20 | No SETUP doc | ✅ `docs/SETUP.md` |

**18/20 fully closed. 2 explicitly deferred.**

---

## 8. Local Dev / Smoke Test Recipe

Full setup details in `docs/SETUP.md`. Quickref:

```powershell
# 1. Refresh PATH (after Go/kubectl/kind installs)
$env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")

# 2. Start infra
docker compose up -d   # minio, postgres+timescale, redis, redpanda, registry:5000

# 3. Run submission-svc with real builder
$env:BUILDER_KIND = "buildkit"
$env:STORE_KIND = "memory"
cd services/submission-svc
go run .

# 4. Pack and upload smoke-go
cd ../../samples/smoke-go
tar -czf /tmp/smoke.tar.gz .
curl -F "file=@/tmp/smoke.tar.gz" -F "lang=go" -F "entrypoint=." -F "contestant_id=t1" http://localhost:8080/submissions
# → returns {id, status:queued}
curl http://localhost:8080/submissions/<id>
# → eventually status:ready, image_url:localhost:5000/submissions/<id>:latest
```

**MinIO console:** http://localhost:9001 (minioadmin/minioadmin).

---

## 9. Known Gotchas & Recurring Issues

1. **PATH stale after install** — Windows installers don't refresh current shell. Run the `$env:Path = ...` line above in any new shell.
2. **Docker daemon not running** — start Docker Desktop, poll: `until docker ps; do sleep 3; done`.
3. **`go test -race` fails locally** — user has 32-bit GCC (`cc1.exe: sorry, unimplemented: 64-bit mode not compiled in`). **Decision: skip `-race` locally; Linux CI runs it.**
4. **Local registry untrusted** — Docker Desktop > Settings > Docker Engine, add `"insecure-registries": ["localhost:5000"]`. Documented in `docs/SETUP.md`.
5. **`/build/` gitignore** — root-only `/build/` (not `build/`) so we don't ignore the `internal/build/` package.
6. **buf.yaml is at repo root**, not inside `proto/`. `modules: [{path: proto}]`. Lint excludes naming-style rules (PACKAGE_VERSION_SUFFIX, SERVICE_SUFFIX, RPC_*_STANDARD_NAME, RPC_REQUEST_RESPONSE_UNIQUE, ENUM_VALUE_PREFIX, ENUM_ZERO_VALUE_SUFFIX).
7. **Proto packages dropped `iicpc.` prefix** — packages are `submission.v1`, `telemetry.v1`, `botcontrol.v1`, `leaderboard.v1`. `go_package = github.com/iicpc/platform/proto/gen/go/<svc>/v1;<svc>v1`.
8. **User often missed `BUILDER_KIND=buildkit` env on second run** — service silently fell back to stub. Always check env if "build did nothing weird."
9. **CI workspace gotcha** — `go vet ./...` / `golangci-lint ./...` run from repo root **fail** with `directory prefix . does not contain modules listed in go.work` because there's no root `go.mod`. Always run per-module via matrix (see `ci.yml`).
10. **buf format strict rules** — imports must come before `option go_package`; trailing inline comments use single space before `//`. Don't hand-format with multi-space alignment.
11. **`tar.TypeReg` only** — never use `tar.TypeRegA` (deprecated since Go 1.11; staticcheck SA1019 will fail CI).
12. **Typed-nil interface trap** — `func() *T` returning nil, assigned to interface var, makes interface non-nil. Compare concrete pointer first (see `main.go` trivy block for the pattern).

---

## 10. CI / Quality Gates

`.github/workflows/ci.yml` runs on push/PR:
- `go vet ./...` per module
- `go test -race -count=1 ./...` per module
- `go build ./...` per module
- `buf lint`
- `golangci-lint run` (config in `.golangci.yml`: errcheck, gosimple, govet, ineffassign, staticcheck, unused, gofmt, misspell, unconvert)

`make ci-local` mirrors CI minus `-race` (Windows GCC limitation).

---

## 11. What's Next (Day 6)

**api-gateway** — JWT auth, rate limiting, routing to backend services.

**Do NOT start Day 6 without explicit `"go"` from user.**

---

## 11c. reference-orderbook — Detailed State (Day 5 complete)

**Location:** `samples/reference-orderbook/` — standalone Go module (`module reference-orderbook`, no external deps).

**engine/orderbook.go:**
- `bidHeap` (max-price, then min-time) + `askHeap` (min-price, min-time) via `container/heap`
- `Order{ID, Side, Price float64, Qty, Remaining int64, At time.Time}`
- `Fill{BuyOrderID, SellOrderID string, Price float64, Qty int64}`
- `Place()` — matches immediately, lazy-deletes cancelled/exhausted heap entries, rests remainder on book
- `Cancel()` — marks `Remaining=0`; heap cleaned lazily on next match traversal
- `Snapshot()` — aggregates by price level, bids sorted desc / asks sorted asc

**main.go:** Go 1.22 mux patterns (`POST /order`, `DELETE /order/{id}`, `GET /orderbook`, `GET /health`). `$RUNTIME_PORT` env. SIGTERM graceful shutdown (5s drain).

**Tests:** 8 unit tests (`engine/orderbook_test.go`). All pass. No real cluster or infra needed.

**Commit:** `f53a549`

---

## 11b. sandbox-runner — Detailed State (Day 4 complete)

**Entry:** `services/sandbox-runner/main.go`. gRPC server, graceful shutdown on SIGTERM.

**Config env vars:**
| Var | Default | Notes |
|---|---|---|
| `GRPC_ADDR` | `:9090` | |
| `K8S_NAMESPACE` | `iicpc-contestants` | namespace for contestant pods |
| `RUNTIME_CLASS` | `gvisor` | set empty to omit (plain runc for local dev) |
| `CPU_REQUEST` / `CPU_LIMIT` | `500m` / `1000m` | |
| `MEMORY_REQUEST` / `MEMORY_LIMIT` | `256Mi` / `512Mi` | |
| `EPHEMERAL_LIMIT` | `2Gi` | |
| `READINESS_TIMEOUT` | `60s` | pod ready wait |

**gRPC methods:** `RunSandbox` (spawn + wait ready → pod IP) · `StopSandbox` (delete pod) · `GetSandboxStatus`

**Pod spec features:** `runtimeClassName: gvisor`, drop ALL caps, readOnlyRootFilesystem, runAsNonRoot (uid 65532), RuntimeDefault seccomp, CPU/mem/ephemeral limits, nodeSelector `iicpc.io/pool=contestants`, `/health` readiness probe.

**Manifests:** `infra/manifests/sandbox-runner.yaml` — `iicpc-contestants` namespace (PSA restricted), ClusterRole + RoleBinding (pod CRUD scoped to contestants ns), NetworkPolicy (ingress: bot-worker only; egress: DNS only), Deployment + Service.

**Tests:** 8 unit tests using fake K8s client (no real cluster needed). `runner_test.go`.

**Module path note:** All modules renamed from `github.com/iicpc/platform/...` → `github.com/Ajayendra2705/iicpc-platform/...` in Day 4. Set `GOPRIVATE=github.com/Ajayendra2705` + `GONOSUMDB=github.com/Ajayendra2705` + run `gh auth setup-git` before `go mod tidy` in any new service.

---

## 12. Memory / Persistence Notes for Future Claude

- The user has a `caveman` skill active by default in this project. Respect it.
- Auto memory dir: `C:\Users\ajiit\.claude\projects\C--Users-ajiit-Desktop-Future-Interns-IICPC-Hackathon\memory\`. Write user/feedback/project/reference memories there per the auto-memory protocol.
- `MEMORY.md` is the index; one-line entries.
- This `HANDOFF.md` is the canonical project context. Update it when major milestones complete (e.g. after Day 4 done, mark sandbox-runner ✅ in section 5; bump table; note new env vars in section 6 if applicable).

---

## 13. Quick "Am I in the right place?" Sanity Check for New Claude

Run these to verify state:

```powershell
git log --oneline -10
# expect (newest first): f53a549, 79bff8e, 018b413, 9064257, 117baca, cf4a017, 04a7079, 57f3de1, 0158c3d, 63ccbba

git status --short
# expect: clean except .claude/ and IDEATION_IMPLEMENTATION_PIPELINE.md (untracked, intentional)

go work sync; foreach ($m in (Get-Content go.work | Select-String "services/|proto/gen/go" | ForEach-Object { ($_ -split '\s+')[1] })) { Write-Host "== $m =="; Push-Location $m; go vet ./...; go build ./...; Pop-Location }
# expect: silent (clean) for all 10 modules

cd services/submission-svc; go test ./...
# expect: 16 tests pass across 4 packages
```

If any of those fail, do NOT start new work. Diagnose first.

---

**End of handoff.** When in doubt: read `PLAN.md` for the day-by-day, `docs/ARCHITECTURE.md` for the system shape, and the latest commit message for what was just done.
