# IICPC Hackathon — Handoff Document

> **Read this first if you are a new Claude (or new dev) picking up this project.**
> This file is the single source of truth for project state, decisions, conventions, and what's next.
> Last updated after Day 27 completion. **Day 28 (EKS staging smoke) next. 5 days of buffer remain.**

**GitHub:** https://github.com/Ajayendra2705/iicpc-platform (private). Default branch: `main`. Local working branch: `main`.
**CI:** GitHub Actions, all jobs green as of commit `9762d63` (Day 27). Workflow file: `.github/workflows/ci.yml`. Jobs: per-module Go matrix (test -race, build, vet), per-module golangci-lint, buf-lint, **terraform-validate** (D25), **helm-lint + kubeconform** (D26, extended in D27 to validate chaos manifests).

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
| **6** | **api-gateway:** stdlib HS256 JWT, per-IP token bucket, reverse proxy to submission-svc. 11 tests | ✅ done | `fd77674` |
| **7** | **Buffer:** sandbox wiring, pipeline integration test, CI fixes (go-version, golangci-lint, GOPRIVATE) | ✅ done | `cbd2834` |
| **8** | **bot-worker:** REST load generator, stats recorder (P50/P90/P99), /healthz + /metrics | ✅ done | `f9a8625` |
| **9** | **bot-worker:** WebSocket client, FIX 4.4 client (QuickFIX-Go), multi-protocol via PROTOCOL env | ✅ done | `8ea33e6` |
| **10** | **bot-worker:** Poisson arrivals (inverse-CDF), timer-based worker loop, gen package | ✅ done | `effb317` |
| **11** | **bot-coordinator:** K8s Job spawner (client-go), StubSpawner, HTTP API (POST/GET/DELETE /benchmarks), graceful shutdown | ✅ done | `b159754` |
| **12** | **Clock sync:** `clocksync.EstimateOffset` (NTP midpoint), GET /time endpoint, `clock_offset_ns` in /metrics, chrony DaemonSet manifest | ✅ done | `a15c97a` |
| **13** | **Burst + jitter + FIX cancel:** `StartBurst` (10× for 100ms), `WithJitter` (0–5ms), FIX `OrderCancelRequest` (35=F) | ✅ done | `9fbac7c` |
| **14** | Found+fixed REST contract bug (ref-orderbook didn't return id); 200-worker integration test passes 0 errors; ctx-shutdown handling; `scripts/load-test.ps1` for 1K manual | ✅ done | `de045fc` |
| **15** | **telemetry-ingester:** gRPC `IngestStream`, MPSC buffer (atomic counters, drop-on-full), Kafka (segmentio) + Stub producer, drainLoop batch flush | ✅ done | `0af7556` |
| **16** | **aggregator:** HDR histograms per contestant (1ns–60s, 3 sig digits), 1s tumbling windows, Kafka consumer + Stub, GET /metrics endpoints | ✅ done | `fced5f3` |
| **17** | **TimescaleDB:** SQL migration (hypertable + 1-min continuous aggregate + 7d/30d retention); aggregator Timescale writer (pgx CopyFrom); `IDEAS.md` differentiator backlog | ✅ done | `dfd623e` |
| **18** | **validator:** sorted-slice price-time-priority orderbook, replay validator, correctness scoring (1 − mismatches/total), Kafka consumer, HTTP /validate | ✅ done | `4f8f836` |
| **19** | **Score formula:** `0.4·latency_norm + 0.3·tps_norm + 0.3·correctness` with crash/timeout penalties, 11 tests covering perfect/clamp/penalty/edge | ✅ done | `11d4552` |
| **20** | **leaderboard-svc:** Redis ZSET store + Stub, WS hub (bounded buffers, drop on slow), ingest poller (HTTPFetcher → score → upsert → broadcast), GET /leaderboard + WS /live | ✅ done | `5856175` |
| **21** | **Pipeline wired:** bot-worker → telemetry-ingester gRPC streaming (non-blocking emit, reconnect+backoff). Closes the bot → ingester → Kafka → aggregator+validator → leaderboard chain | ✅ done | `735adc2` |
| **22** | **Next.js leaderboard UI:** WS live subscription + REST fallback, sortable columns, connection-status badge, dark theme | ✅ done | `b8d22a8` |
| **22.5** | **SEED_DEMO mode** in leaderboard-svc (6 synthetic contestants drift each tick); 127.0.0.1 defaults to bypass Windows IPv6/IPv4 loopback issue | ✅ done | `262e5cd` |
| **23** | **Per-contestant detail page:** 4 stat tiles + Recharts (latency bars, TPS line, outcome pie). Synthetic deterministic fallback when aggregator/validator not running | ✅ done | `3472b3e` |
| **24** | **Submission UI:** UploadForm + 7-stage SubmissionStatus visualization + LogViewer (auto-scroll, color-coded), real upload + synthetic pipeline fallback | ✅ done | `f161719` |
| **25** | **Terraform AWS:** VPC + 3-AZ subnets, EKS + 3 node pools (services/contestants taint/bots, Graviton AMI), RDS Postgres 16 + Timescale param group, ElastiCache Redis 7.1, MSK Serverless SASL/IAM, S3 versioned/encrypted, 10 ECR repos, **terraform-validate CI gate** | ✅ done | `0e1c0f9` |
| **26** | **Helm umbrella chart:** 9 deploys + UI iterated from values.services, HPA (CPU 80%), PDB minAvailable=1, PSA `restricted` namespace, 5 NetworkPolicies (default-deny + DNS + same-ns + AWS data-plane, IMDS blocked). **helm-lint + kubeconform CI gate** | ✅ done | `93801dd` |
| **27** | **Chaos test playbook:** 3 reproducible scenarios (kill bot pod, isolate contestant via NetworkPolicy, Pumba latency injection) with PowerShell scripts, static YAML templates, kubeconform CI gate, full timeline-table docs in `docs/CHAOS.md` | ✅ done | `9762d63` |
| **28** | End-to-end smoke on EKS staging (one-shot `terraform apply` + `helm upgrade` + benchmark + measure) | ⏳ **next** | |
| 29–32 | ARCHITECTURE.md polish, README + demo script, demo video, final submit | pending | |

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

`.github/workflows/ci.yml` runs on push/PR — **5 job categories, all green as of D26**:

1. **test matrix** (per Go module): `go vet ./...`, `go test -race -count=1 ./...`, `go build ./...`
2. **golangci-lint matrix** (per Go module): config in `.golangci.yml` (errcheck, gosimple, govet, ineffassign, staticcheck, unused, gofmt, misspell, unconvert)
3. **proto-lint**: `buf lint`
4. **terraform-validate** (D25): `terraform fmt -check`, `init -backend=false`, `validate` against terraform 1.15.3
5. **helm-lint** (D26): `helm lint`, `helm template` (dev + prod overlays), `kubeconform` schema check against K8s 1.30

`make ci-local` mirrors the Go matrix minus `-race` (Windows GCC limitation).

---

## 11. What's Next (Day 28 — EKS staging smoke)

D28 is the first real cloud run:
1. `terraform apply` in `infra/terraform/` (~15–20 min for VPC + EKS + RDS)
2. Build + push service images to ECR (`--platform linux/arm64` per Graviton)
3. `aws eks update-kubeconfig` to populate `~/.kube/config`
4. `helm upgrade --install iicpc ./infra/helm/iicpc-platform -f values.yaml -f values.production.yaml --set global.imageRegistry=<ECR>`
5. Apply chrony DaemonSet + sandbox-runner manifest + TimescaleDB migration
6. Start a benchmark via `bot-coordinator`, watch end-to-end through web UI
7. Capture: cold-start time per service, end-to-end leaderboard latency, max sustained TPS
8. `terraform destroy` to halt the meter

Then D29–32: ARCHITECTURE.md polish, README + demo script, demo video (5 min: upload → run → leaderboard → chaos), final submit.

**Do NOT start without explicit `"go"` from user.** D28 burns real AWS credit.

---

## 11h. Two-terminal demo recipe (no Docker, no Kafka, no cluster)

```powershell
# Terminal 1 — leaderboard in SEED_DEMO mode (no upstreams needed)
cd services\leaderboard-svc; $env:SEED_DEMO = "true"; go run .

# Terminal 2 — Next.js UI
cd web; npm run dev    # http://localhost:3000
```

Browser:
- `/`             — live leaderboard (WS pulse badge, sortable, 6 teams drifting)
- `/contestant/team-alpha` — 4 stat tiles + 3 Recharts visualizations (synthetic-data badge)
- `/submit`       — upload form + 7-stage pipeline visualization + log viewer (synthetic pipeline badge)

To switch any synthetic fallback to "Live": start the matching backend (aggregator/validator/submission-svc) on its default port. UI auto-detects via 404 fallback logic in hooks.

---

## 11e. bot-worker — Detailed State (Days 8–13 complete)

**Entry:** `services/bot-worker/main.go`. Multi-protocol load generator.

**Config env vars:**
| Var | Default | Notes |
|---|---|---|
| `HTTP_ADDR` | `:9090` | /healthz + /metrics + /time |
| `TARGET_URL` | `http://localhost:8082` | contestant pod base URL |
| `PROTOCOL` | `rest` | `rest` \| `ws` \| `fix` |
| `ORDERS_PER_SECOND` | `10` | mean rate |
| `NUM_WORKERS` | `1` | concurrent goroutines |
| `ARRIVAL_MODE` | `uniform` | `uniform` \| `poisson` |
| `JITTER_MS` | `0` | max uniform jitter per arrival (ms) |
| `BURST_ENABLED` | `false` | periodic 10× spike |
| `BURST_MULTIPLIER` | `10` | burst rate multiplier |
| `BURST_EVERY_S` | `30` | seconds between bursts |
| `BURST_DURATION_MS` | `100` | burst duration |
| `MID_PRICE` | `100` | order price center |
| `PRICE_SIGMA` | `1.0` | price std dev |
| `CANCEL_RATIO` | `0.70` | fraction of events that try a cancel |
| `WORKER_ID` | `bot-0` | FIX SenderCompID prefix |
| `WS_PATH` | `/ws` | WebSocket path appended to TARGET_URL |
| `FIX_HOST` | `localhost` | FIX acceptor host |
| `FIX_PORT` | `5001` | FIX acceptor port |

**Internal packages:**
- `internal/gen`: `Generator` (price/side/qty/cancelRatio), `Arrivals` (uniform/Poisson + jitter + burst)
- `internal/client`: `OrderClient` interface; `REST`, `WSClient`, `FIXClient` implementations
- `internal/stats`: `Recorder` (P50/P90/P99/TPS, atomic, copy-sort), `Snapshot` JSON with `clock_offset_ns`
- `internal/clocksync`: `EstimateOffset(ctx, *http.Client, url)` — NTP midpoint formula against `/time`

**FIX:** QuickFIX-Go v0.9.10. `NewOrderSingle` (35=D), `OrderCancelRequest` (35=F, tag-41 OrigClOrdID). Async ExecutionReport routing via `pending map[clOrdID → chan fixResult]`. 5s timeout guard.

**HTTP endpoints:** `GET /healthz`, `GET /metrics` (JSON Snapshot), `GET /time` (`{"now_ns": <unix_ns>}`)

**Tests:** 5 packages, all pass. Key: `client/*_test.go` (REST/WS/FIX/clocksync), `gen/arrival_test.go` (Poisson CV, jitter range, burst), `stats/recorder_test.go`.

---

## 11f. bot-coordinator — Detailed State (Day 11 complete)

**Entry:** `services/bot-coordinator/main.go`. Spawns bot-worker K8s Jobs, exposes HTTP control plane.

**Config env vars:**
| Var | Default | Notes |
|---|---|---|
| `BOT_COORDINATOR_ADDR` | `:8083` | |
| `SPAWNER_KIND` | `stub` | `stub` (in-memory) \| `k8s` |
| `K8S_NAMESPACE` | `iicpc` | namespace for bot Jobs |
| `BOT_WORKER_IMAGE` | `localhost:5000/bot-worker:latest` | |
| `KUBECONFIG` | `` | empty = in-cluster config |

**HTTP API:**
- `POST /benchmarks` body `{benchmark_id, target_url, num_workers, orders_per_second, protocol, arrival_mode, duration_seconds}` → 201 `{benchmark_id}` / 409 duplicate / 400 missing fields
- `GET /benchmarks/{id}` → `{active, succeeded, failed, phase}`
- `DELETE /benchmarks/{id}` → 204 / 404

**K8s Job spec:** `parallelism=completions=num_workers`, `RestartPolicy=Never`, `TTLSecondsAfterFinished=300`, `ActiveDeadlineSeconds=duration+60`, 100m/64Mi requests, 500m/256Mi limits.

**Tests:** 14 tests across `internal/spawn` (6) and `internal/server` (8).

---

## 11g. infra/manifests additions (Days 11–12)

- `chrony-daemonset.yaml` — DaemonSet syncing all nodes to `time.aws.com` + `pool.ntp.org`. `hostNetwork: true`, privileged, `system-node-critical` priority.

---

## 11d. api-gateway — Detailed State (Day 6 complete)

**Entry:** `services/api-gateway/main.go`. Zero external deps (all stdlib).

**Config env vars:**
| Var | Default | Notes |
|---|---|---|
| `API_GATEWAY_ADDR` | `:8080` | listen address |
| `JWT_SECRET` | `change-me-in-prod` | HS256 HMAC key — **must be set in prod** |
| `SUBMISSION_SVC_URL` | `http://localhost:8081` | submission-svc backend; set `HTTP_ADDR=:8081` on submission-svc for local co-run |

**Rate limits:** 20 rps sustained, burst 50, per source IP. Token bucket with 5-min idle eviction.

**Routes:**
- `GET /healthz` — no auth, no rate key
- `POST /auth/token` body `{"contestant_id":"..."}` → issues 15-min HS256 JWT
- `POST /submissions` / `GET /submissions` / `GET /submissions/{id}` — JWT required → proxied to submission-svc

**JWT:** stdlib HS256 (no external lib). `X-Contestant-ID` injected on proxy requests.

**Middleware chain:** AccessLog → RequestID → RateLimit → mux → [JWT] → proxy

**Tests:** 11 across 3 packages (main, auth, ratelimit). Commit: `fd77674`

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

## 11i. telemetry-ingester — Detailed State (Day 15 complete)

**Entry:** `services/telemetry-ingester/main.go`. gRPC `IngestStream` (client streaming) → bounded MPSC buffer → batched Kafka publish.

| Env | Default | Notes |
|---|---|---|
| `GRPC_ADDR` | `:9091` | |
| `BUFFER_CAPACITY` | `16384` | drop-on-full counter exposed via atomics |
| `BATCH_SIZE` | `256` | publish triggers at this size OR `FLUSH_INTERVAL_MS` |
| `FLUSH_INTERVAL_MS` | `100` | |
| `PRODUCER_KIND` | `stub` | `stub` \| `kafka` |
| `KAFKA_BROKERS` / `KAFKA_TOPIC` | `localhost:9092` / `telemetry-events` | |

**Internal packages:**
- `internal/buffer`: MPSC channel + atomic pushed/dropped, non-blocking Push, Recv exposes channel for single consumer
- `internal/producer`: `Producer` interface, `Kafka` (segmentio, protobuf, contestant_id key) + `Stub`
- `internal/server`: gRPC `TelemetryIngester` impl; `IngestStream` reads stream → `buf.Push`, returns `IngestAck{events_received}`

**Tests:** buffer (6 incl. 100×100 concurrent push), producer stub (3), server (3 via bufconn). All pass.

---

## 11j. aggregator — Detailed State (Days 16–17 complete)

**Entry:** `services/aggregator/main.go`. Kafka consumer → HDR histograms per contestant → 1s tumbling windows → TimescaleDB.

| Env | Default | Notes |
|---|---|---|
| `HTTP_ADDR` | `:8084` | |
| `CONSUMER_KIND` | `stub` | `stub` \| `kafka` |
| `WRITER_KIND` | `stub` | `stub` \| `timescale` |
| `WINDOW_MS` | `1000` | |
| `POSTGRES_DSN` | — | required when WRITER_KIND=timescale |

**Internal packages:**
- `internal/windowing`: `Aggregator` — per-contestant `hdrhistogram.New(1, 60_000_000_000, 3)`, `Record` adds to open window, `Flush(t)` rolls every open window into `Snapshot{P50/P90/P99/P999/TPS/count/rejected/timeouts}`
- `internal/consumer`: Kafka (segmentio + protobuf decode + offset commit) + Stub
- `internal/writer`: `Timescale` (`pgx.CopyFrom`) + `Stub`
- `internal/httpapi`: `GET /healthz`, `GET /metrics`, `GET /metrics/{contestant_id}`

**Tests:** 19 across 4 packages incl. P50≈500ms ±1% across 1000 samples, per-contestant isolation, error counting, 50-goroutine concurrent writes.

**TimescaleDB schema:** `infra/timescaledb/migrations/001_telemetry_schema.sql` — `telemetry_snapshots` hypertable (1-day chunks) + `telemetry_1m` continuous aggregate (30s refresh, 10min lookback) + retention (7d raw / 30d 1m).

---

## 11k. validator — Detailed State (Day 18 complete)

**Entry:** `services/validator/main.go`. Replay each contestant's event stream through a reference price-time-priority orderbook, compare contestant-reported FilledQuantity, drop correctness for mismatches.

| Env | Default | Notes |
|---|---|---|
| `HTTP_ADDR` | `:8085` | |
| `CONSUMER_KIND` | `stub` | `stub` \| `kafka` |
| `KAFKA_*` | — | per consumer kind |

**Internal packages:**
- `internal/replay/book`: sorted-slice orderbook, `Place` walks opposing levels, `Cancel` by id, BestBid/BestAsk. Correctness > speed (validator is offline)
- `internal/replay/validator`: per-contestant Book + counters; on LIMIT/MARKET → expected fill vs `event.FilledQuantity`; CANCEL → book op but not counted; `Correctness = 1 − mismatches/total`
- `internal/consumer` + `internal/httpapi`: same shape as aggregator

**Side encoding:** Heuristic — order_id prefix `s-` = Sell, else Buy. (Proto doesn't carry side; planned proto extension noted in IDEAS.md.)

**Tests:** 22 — book matching/priority/multi-level walk/cancel, validator perfect-correctness/mismatch detection/per-contestant isolation, HTTP endpoints, consumer replay.

---

## 11l. leaderboard-svc — Detailed State (Days 19–20 + 22.5 SEED_DEMO)

**Entry:** `services/leaderboard-svc/main.go`. Polls aggregator + validator, computes composite score, upserts to Redis ZSET, broadcasts top-N over WS.

| Env | Default | Notes |
|---|---|---|
| `HTTP_ADDR` | `:8086` | |
| `STORE_KIND` | `stub` | `stub` \| `redis` |
| `REDIS_ADDR` / `REDIS_KEY` | `localhost:6379` / `leaderboard:scores` | |
| `AGGREGATOR_URL` | `http://127.0.0.1:8084` | **127.0.0.1, NOT localhost** (Windows IPv6 trap) |
| `VALIDATOR_URL` | `http://127.0.0.1:8085` | same |
| `TICK_MS` | `1000` | |
| `TOP_N` | `100` | |
| `SEED_DEMO` | `false` | **`true` injects 6 synthetic contestants drifting each tick; bypasses ingester** |

**Internal packages:**
- `internal/score` (D19): `Calculator` — `base = 1000·(0.4·latency_norm + 0.3·tps_norm + 0.3·correctness)`, `final = max(0, round(base) − crashes·10000 − timeouts·1000)`. Configurable weights/caps/penalties; zero-value Config falls back to defaults
- `internal/store`: `Store` interface, `Redis` (go-redis v9 ZADD/ZREVRANGE), `Stub` (sorted-map)
- `internal/ws`: bounded fan-out hub, slow consumers drop, atomic Count
- `internal/ingest`: Fetcher interface, `HTTPFetcher`, `Ingester.Tick` (fetch → score → upsert → broadcast)
- `internal/seeder` (D22.5): demo data generator
- `internal/httpapi`: `GET /healthz`, `GET /leaderboard?top=N`, `WS /live` (gorilla/websocket, hub-driven push)

**Tests:** 28 across all packages incl. score perfect/clamp/penalty/never-negative, hub register/broadcast/slow-drop, ingester httptest end-to-end, seeder bounded scores.

---

## 11m. web/ — Detailed State (Days 22–24)

**Tech:** Next.js 15.1 + React 19 + TypeScript 5.6 + recharts 2.13. App Router. No CSS framework — single `globals.css`, ~520 lines.

| Env | Default | Notes |
|---|---|---|
| `LEADERBOARD_URL` | `http://127.0.0.1:8086` | rewrite target for `/api/leaderboard` |
| `AGGREGATOR_URL` | `http://127.0.0.1:8084` | rewrite target for `/api/metrics/:id` |
| `VALIDATOR_URL` | `http://127.0.0.1:8085` | rewrite target for `/api/validate/:id` |
| `SUBMISSION_URL` | `http://127.0.0.1:8080` | rewrite target for `/api/submissions{,/:id}` |
| `NEXT_PUBLIC_LEADERBOARD_WS` | `ws://127.0.0.1:8086/live` | browser WS URL |

**Pages:**
- `/` (`app/page.tsx`) — `useLeaderboard` hook → WS subscription with REST fallback, sortable table, top-1 highlight, "Submit code" nav
- `/contestant/[id]` (`app/contestant/[id]/page.tsx`) — `useContestantMetrics` polls aggregator+validator every 1s, 60-sample TPS history; **deterministic synthetic fallback** keyed off contestant-id hash when both upstreams 404; renders 4 stat tiles + LatencyChart (BarChart auto-units ns/µs/ms) + TPSChart (LineChart) + ErrorBreakdown (PieChart with correctness summary)
- `/submit` (`app/submit/page.tsx`) — `useSubmission` hook: real upload + 1.5s status polling, OR synthetic 7-stage pipeline advancing every 1.2s with log lines; UploadForm + SubmissionStatus (pulsing current step) + LogViewer (color-coded auto-scroll)

**Build:** `npm run typecheck` + `npm run build` both clean. Production build emits 3 routes: `/` (static), `/contestant/[id]` (dynamic SSR), `/submit` (static + WS upgrades server-side).

---

## 11n. infra/terraform — Detailed State (Day 25 complete)

`infra/terraform/` — AWS production scaffold. **13 files, ~660 lines HCL**, validated via CI gate.

- `vpc.tf` — terraform-aws-modules/vpc v5.13, 3-AZ public+private, NAT (single in staging / per-AZ in prod), ALB-controller subnet tags
- `eks.tf` — terraform-aws-modules/eks v20.24, 3 managed node pools (services/contestants taint/bots), Graviton AL2023 AMI, coredns + vpc-cni + ebs-csi addons
- `rds.tf` — Postgres 16 + parameter group loading `timescaledb` in `shared_preload_libraries`, gp3 encrypted, enhanced monitoring, multi-AZ in prod
- `redis.tf` — ElastiCache 7.1, at-rest+transit encryption, failover only in prod
- `msk.tf` — MSK Serverless SASL/IAM on 9098
- `s3.tf` — submissions bucket: versioned, AES-256, all-public-blocked, 30d noncurrent expiration, **`filter {}` in lifecycle rule** to suppress future-major warning
- `ecr.tf` — 10 repos (incl. web), scan-on-push, lifecycle keeps 20 tagged + expires untagged after 7d
- `outputs.tf` — cluster endpoint, kubectl bootstrap cmd, all endpoints, ECR repo URL map

**Cost ballpark (README):** ~$19/day staging, ~$60/day prod (ap-south-1, on-demand).

**Validation:** Local + CI run `terraform fmt -check`, `init -backend=false`, `validate` — **0 warnings, 0 errors**.

**Not yet wired:** IRSA service-account ↔ IAM role mappings (D26 helm gap), AWS Secrets Manager backend for `db_password`. Tracked in IDEAS.md.

---

## 11o. infra/helm — Detailed State (Day 26 complete)

`infra/helm/iicpc-platform/` — single umbrella chart, 12 files. **9 deployments + web UI iterated from `values.services`** — adding a 10th service is one row in values, not a new chart.

**Per-service auto-generated:** Deployment (PSA-restricted-compliant securityContext: runAsNonRoot, readOnlyRootFilesystem, drop ALL caps, seccomp RuntimeDefault, tmp emptyDir 64Mi) + Service (ClusterIP) + ServiceAccount + HPA (CPU 80%) + PDB (minAvailable=1).

**Cluster-wide once:**
- Namespace with `pod-security.kubernetes.io/{enforce,audit,warn}: restricted`
- **5 NetworkPolicies:** default-deny baseline, allow-same-namespace (ingress + egress), DNS egress to kube-dns, AWS data-plane egress (only submission-svc/telemetry-ingester/aggregator/validator/leaderboard-svc can reach S3/RDS/Redis/MSK), **IMDS 169.254.169.254 explicitly blocked**

**Values overlays:** `values.yaml` (dev defaults, localhost:5000 registry) + `values.production.yaml` (raises replicas + resource requests, ECR registry placeholder).

**Validation:** Local `helm lint` clean, `helm template` renders 1610 lines. CI gate runs `helm lint`, both renders, then **kubeconform** schema-validates against K8s 1.30 (no cluster needed).

**Not yet wired:** IRSA SA annotations, external-secrets-operator, Ingress (class choice between ALB controller vs nginx), ConfigMap for long env-var lists.

---

## 11p. IDEAS.md (D17+)

Repo-root file `IDEAS.md` holds the differentiator backlog. Update it as new ideas surface during development. Auto-memory `project_ideas_backlog.md` tracks the discipline. Current sections: differentiators-beyond-brief, architecture-future-state, demo/UX-polish, tech-debt-to-track.

---

## 11q. Chaos tests — Detailed State (Day 27 complete)

Three reproducible chaos scenarios. Full playbook in `docs/CHAOS.md` (scenario matrix + timeline tables + prerequisites + limitations).

| # | Script | Tests | Cluster requirement |
|---|---|---|---|
| 1 | `scripts/chaos/kill-bot-pod.ps1` | Deployment + HPA self-heal | any K8s ≥ 1.27 |
| 2 | `scripts/chaos/isolate-contestant.ps1 -ContestantID <id>` | Failure-path scoring (timeouts → score drops) | CNI with NetworkPolicy enforcement (Calico / Cilium / EKS) |
| 3 | `scripts/chaos/inject-latency.ps1 -DelayMs 100 -JitterMs 20` | Latency penalty in score formula via Pumba `netem` | Pumba-capable nodes (won't work on gVisor — see CHAOS.md "Limitations") |
| ★ | `scripts/chaos/run-suite.ps1` | All three back-to-back with timed pauses (built for demo-video capture) | as above |

**Static manifests** (`infra/manifests/chaos/`):
- `isolate-policy.yaml` — deny-all NetworkPolicy template (scoped by `chaos.iicpc.io/isolated: <id>` label; scripts patch the id at runtime)
- `pumba-network-delay.yaml` — Pumba Job template (`tc qdisc add netem delay 100ms 20ms`, NET_ADMIN cap, `tc-image: gaiadocker/iproute2`)
- `kustomization.yaml` — bundles both with `app.kubernetes.io/component: chaos`

**CI gate**: existing `helm-lint` job extended to kubeconform-validate both chaos manifests alongside the rendered helm output (K8s 1.30 schema). Locally: 2/2 valid.

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
# expect (newest first): 9762d63 (D27 chaos), a9cb8e8 (HANDOFF refresh), 93801dd (D26 helm), 0e1c0f9 (D25-fix), f547c8a (D25), f161719 (D24), 3472b3e (D23), 262e5cd (D22.5), b8d22a8 (D22), 735adc2 (D21)

git status --short
# expect: clean except .claude/ and IDEATION_IMPLEMENTATION_PIPELINE.md (untracked, intentional)

# Workspace-wide Go check
foreach ($m in (Get-Content go.work | Select-String "services/|samples/" | ForEach-Object { ($_ -split '\s+')[1] })) { Push-Location $m; $null = go test ./... 2>&1; Write-Host "$(if ($LASTEXITCODE -eq 0) {'PASS'} else {'FAIL'}) $m"; Pop-Location }
# expect: 10 PASS

# Web typecheck
cd web; npm run typecheck
# expect: no errors

# Terraform
cd infra/terraform; terraform validate
# expect: Success! The configuration is valid.

# Helm
cd infra/helm; helm lint iicpc-platform
# expect: 0 chart(s) failed
```

If any of those fail, do NOT start new work. Diagnose first.

---

**End of handoff.** When in doubt: read `PLAN.md` for the day-by-day, `docs/ARCHITECTURE.md` for the system shape, and the latest commit message for what was just done.
