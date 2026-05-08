# IICPC Summer Hackathon 2026 — Solo Build Plan

**Dates:** May 9 – Jun 10, 2026 (32 days)
**Team:** 1 person
**Goal:** Distributed Benchmarking & Hosting Platform for trading infra eval.

---

## 1. Scope Lock

Build minimum-viable end-to-end pipeline:
`Code Upload → Containerized Deployment → Distributed Load Testing → Real-Time Scoring`

Cut what does not move the needle for a solo build:
- No Rust hot path (Go everywhere)
- No Firecracker (gVisor sufficient for isolation)
- No multi-cloud Terraform (AWS only)
- No custom FIX parser (use QuickFIX-Go)
- No mobile UI, no auth providers beyond JWT, no admin dashboard

---

## 2. Stack Decisions

| Layer | Choice | Reason |
|---|---|---|
| Orchestrator + API gateway | Go (Gin/Chi) | fast dev, goroutine concurrency, native K8s client |
| Bot fleet workers | Go | 10K+ goroutines/node trivial |
| Telemetry ingester | Go + HDR histogram + ring buffer | sub-ms ingestion |
| Sandbox runtime | Docker + gVisor (`runsc`) | strong syscall isolation, fast cold start |
| Resource limits | cgroups v2: `--cpus`, `--memory`, `--pids-limit`, `--read-only` | fair allocation |
| Message bus | Redpanda (Kafka API) | single binary, no ZooKeeper |
| Time-series store | TimescaleDB (Postgres + hypertables) | SQL, percentile_cont, retention policies |
| Live leaderboard state | Redis (ZSET) | O(log N) ranked queries |
| Inter-service RPC | gRPC | low latency, typed contracts |
| Frontend | Next.js 14 + React + Recharts + WebSocket | quick build, SSR optional |
| Container orchestration | Kubernetes (kind locally, EKS cloud) | required by deliverable |
| IaC | Terraform (AWS) + Helm charts + raw manifests | deliverable #3 |
| Local dev | kind + skaffold + tilt (optional) | no cloud burn during dev |
| CI | GitHub Actions | matrix build per service |

---

## 3. Repo Layout

```
iicpc-platform/
├── go.work                          # Go workspaces, all services
├── proto/                           # gRPC contracts (source of truth)
│   ├── submission.proto
│   ├── telemetry.proto
│   ├── bot_control.proto
│   └── leaderboard.proto
├── services/
│   ├── api-gateway/                 # HTTP API, JWT auth, upload endpoint
│   ├── submission-svc/              # build + push container image
│   ├── sandbox-runner/              # K8s pod spawner per submission
│   ├── bot-coordinator/             # spawns bot Jobs, traffic profiles
│   ├── bot-worker/                  # actual load gen (REST/WS/FIX)
│   ├── telemetry-ingester/          # gRPC server, ring buf, Redpanda producer
│   ├── aggregator/                  # consumer, percentiles → Timescale
│   ├── validator/                   # price-time priority, fill correctness
│   └── leaderboard-svc/             # Redis ZSET ops, WS broadcast
├── web/                             # Next.js leaderboard UI
├── sandbox-images/                  # base Dockerfiles for cpp/rust/go
│   ├── Dockerfile.cpp
│   ├── Dockerfile.rust
│   └── Dockerfile.go
├── infra/
│   ├── terraform/                   # VPC, EKS, RDS, ElastiCache, MSK
│   ├── helm/                        # charts per service
│   └── manifests/                   # raw K8s yaml (NetworkPolicies, PSA, HPA)
├── samples/
│   └── reference-orderbook/         # working contestant submission for demo
├── scripts/                         # kind-up.sh, seed.sh, chaos.sh
└── docs/
    ├── ARCHITECTURE.md              # blueprint deliverable
    ├── ADR/                         # decision records
    └── api.md
```

---

## 4. Architecture (high-level)

```
            ┌──────────────┐
            │  Web (Next)  │ ◄── WS live updates
            └──────┬───────┘
                   │ REST/WS
            ┌──────▼───────┐
            │ API Gateway  │ JWT, rate limit
            └──┬─────┬─────┘
               │     │
       upload  │     │ scores
               ▼     ▼
   ┌────────────────┐  ┌──────────────────┐
   │ Submission Svc │  │ Leaderboard Svc  │◄───┐
   └───────┬────────┘  └─────────▲────────┘    │
           │ build+push image    │              │
           ▼                     │ ZSET         │
   ┌────────────────┐         ┌──┴─────┐        │
   │ Sandbox Runner │         │ Redis  │        │
   │ (K8s API)      │         └────────┘        │
   └───────┬────────┘                           │
           │ creates Pod (gVisor)               │
           ▼                                    │
   ┌──────────────────┐                         │
   │ Contestant Pod   │◄── orders ──┐           │
   │ (sandboxed)      │             │           │
   └──────────────────┘             │           │
                                    │           │
                          ┌─────────┴────────┐  │
                          │ Bot Coordinator  │  │
                          └────────┬─────────┘  │
                                   │ spawns Jobs│
                                   ▼            │
                          ┌──────────────────┐  │
                          │ Bot Workers (N)  │  │
                          └────────┬─────────┘  │
                                   │ telemetry  │
                                   ▼            │
                          ┌──────────────────┐  │
                          │ Telemetry Ingest │  │
                          └────────┬─────────┘  │
                                   │ gRPC→Redpanda
                                   ▼            │
                  ┌──────────┐  ┌────────────┐  │
                  │Aggregator│  │ Validator  │  │
                  └────┬─────┘  └─────┬──────┘  │
                       │              │         │
                       ▼              ▼         │
                 ┌──────────┐    ┌─────────────┐│
                 │TimescaleDB│   │Score Calc   ├┘
                 └──────────┘    └─────────────┘
```

Data flow: contestant uploads code → submission-svc builds image → sandbox-runner deploys K8s pod under gVisor with strict limits → bot-coordinator spawns bot Jobs → bots blast orders → telemetry-ingester captures every ack with ns timestamps → aggregator computes P50/P90/P99 + TPS → validator replays order log for price-time priority + fill correctness → score-calc writes composite score to Redis ZSET → leaderboard-svc broadcasts via WS to web UI.

---

## 5. Day-by-Day Schedule

### Week 1 (May 9 – May 15) — Foundation + Sandbox
- **D1 (May 9):** Repo init, go.work, proto contracts drafted, kind cluster up.
- **D2:** Submission service: HTTP upload, MinIO storage, validation (size/lang).
- **D3:** Sandbox Dockerfiles for C++/Rust/Go. Buildkit pipeline.
- **D4:** Sandbox runner: K8s client-go, pod spec with gVisor `runtimeClassName: gvisor`, resource limits, NetworkPolicy isolation.
- **D5:** Sample reference orderbook (Go) deploys end-to-end.
- **D6:** API gateway scaffold, JWT auth, rate limit (token bucket).
- **D7:** Buffer + integration test: upload → build → run → ping.

### Week 2 (May 16 – May 22) — Bot Fleet
- **D8:** Bot worker: REST client, configurable order rate.
- **D9:** WebSocket client + FIX client (QuickFIX-Go integration).
- **D10:** Order generator: Poisson arrivals, price ~ N(mid, σ), 60–80% cancel ratio.
- **D11:** Bot coordinator: K8s Jobs, fan-out N replicas, traffic profile config.
- **D12:** Clock sync: chrony in pods, monotonic ns timestamps via `time.Now().UnixNano()`.
- **D13:** Burst mode (10x spike for 100ms), jitter (0–5ms random delay).
- **D14:** Buffer + load test: 1K bots → reference orderbook, no errors.

### Week 3 (May 23 – May 29) — Telemetry + Scoring
- **D15:** Telemetry ingester gRPC server, lock-free ring buf, batch flush to Redpanda.
- **D16:** Aggregator consumer, hdrhistogram-go for P50/P90/P99, 1s tumbling windows.
- **D17:** TimescaleDB schema + hypertable, continuous aggregates for 1m rollups.
- **D18:** Validator service: subscribe to order log topic, replay engine, check price-time priority + fill accuracy.
- **D19:** Score formula: `score = 0.4 * latency_norm + 0.3 * tps_norm + 0.3 * correctness`. Penalties: crash = -10K, timeout = -1K each.
- **D20:** Leaderboard svc: Redis ZADD on score change, ZREVRANGE for top N, WS pub/sub fanout.
- **D21:** Buffer + correctness validation against reference.

### Week 4 (May 30 – Jun 5) — Frontend + IaC + Chaos
- **D22:** Next.js leaderboard table, live WS subscription, sortable columns.
- **D23:** Per-contestant detail page: latency histogram, TPS over time, error breakdown (Recharts).
- **D24:** Submission upload UI, status polling, log viewer.
- **D25:** Terraform: VPC, EKS cluster, RDS Postgres + Timescale ext, ElastiCache Redis, MSK Serverless or Redpanda Cloud.
- **D26:** Helm charts per service, HPA configs (CPU 80%), NetworkPolicies, PodSecurityAdmission `restricted`.
- **D27:** Chaos tests: kill bot pod (self-heal), kill contestant pod (penalty applied), network jitter via toxiproxy/Pumba.
- **D28:** End-to-end smoke on EKS staging.

### Buffer (Jun 6 – Jun 10) — Polish + Submit
- **D29:** ARCHITECTURE.md blueprint with diagrams (mermaid + ASCII).
- **D30:** README, demo script, sample submission walkthrough.
- **D31:** Demo video (5 min): upload → run → leaderboard.
- **D32:** Final submit (last week of hackathon per rules).

---

## 6. Quality Gates per Service

Before merging any service into main:
- `go vet ./...` clean
- `golangci-lint run` zero errors
- Unit tests > 70% coverage (critical paths only, not chasing 100%)
- Integration test: spin in kind, hit endpoint, assert response
- gRPC contract back-compat check (buf breaking)

---

## 7. Security Hardening (non-negotiable)

- gVisor runtime for ALL contestant pods (`runtimeClassName: gvisor`)
- `securityContext`: `runAsNonRoot: true`, `readOnlyRootFilesystem: true`, drop ALL caps, `allowPrivilegeEscalation: false`
- NetworkPolicy: contestant pod can ONLY accept from bot-worker namespace, no egress to internet
- Resource limits enforced: 2 CPU max, 512Mi mem max per submission
- Image scan with Trivy in CI before deploy
- Secrets via K8s Secrets + sealed-secrets, never in images
- JWT short expiry (15min) + refresh rotation
- Rate limit on upload endpoint (5 submissions / hour / contestant)
- Submission size cap: 50MB

---

## 8. Performance Targets

| Metric | Target |
|---|---|
| Telemetry ingest latency | < 50µs p99 |
| Order ack measurement precision | nanoseconds |
| Bot fleet scale | 5000+ bots concurrent |
| Sustained TPS per contestant | 10K+ |
| Leaderboard update latency | < 1s end-to-end |
| Cold-start contestant pod | < 5s |
| Score computation cycle | < 500ms |

---

## 9. Risk Register

| Risk | Mitigation |
|---|---|
| Solo dev burnout | Hard week boundaries, no scope creep, daily commits |
| gVisor compat issues with C++ binaries | Fallback to runc + strict seccomp profile |
| EKS cost | Use kind locally for 80% of work, EKS only final week |
| Bot fleet noisy neighbor | Dedicated node pool with taints, anti-affinity |
| Time-series write amplification | Continuous aggregates + retention policy 7d raw, 30d 1m |
| FIX protocol depth | Skip if behind schedule, REST/WS sufficient for demo |

---

## 10. Demo Path (final submission)

1. `terraform apply` → cluster up.
2. Open web UI, register contestant.
3. Upload sample reference orderbook (provided in repo).
4. System builds, deploys sandboxed pod.
5. Click "Start Benchmark" → coordinator spawns 1000 bots.
6. Live leaderboard shows latency/TPS/score updating.
7. Inject chaos: `kubectl delete pod` reference orderbook → penalty applied, score drops.
8. Show TimescaleDB query: latency histograms over time.
9. Show isolation: try escape from contestant pod → blocked.

---

## 11. First Actions

Day 1 task list:
1. `git init` + commit this PLAN.md
2. Create monorepo structure (folders only)
3. Write `proto/*.proto` files for all 4 contracts
4. `go work init` + add stub modules per service
5. `kind create cluster --config infra/kind.yaml` (3 nodes)
6. Install gVisor on kind nodes (or note for cloud-only fallback)
7. Commit baseline, push to GitHub
