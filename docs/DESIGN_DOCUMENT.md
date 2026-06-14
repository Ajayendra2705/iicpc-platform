# IICPC Summer Trading Hackathon 2026 — Design Document

| | |
|---|---|
| **Team name** | Ajay_2705 |
| **Participant(s)** | Ajayendra Kumar Bansod, IIT Kharagpur (solo submission) |
| **Project** | A distributed benchmarking & hosting platform for contestant trading engines |
| **Repository** | https://github.com/Ajayendra2705/iicpc-platform *(public)* |
| **Live demo (UI)** | https://iicpc-platform.vercel.app/ |
| **Date** | June 2026 |

---

## 1. Problem statement

The hackathon brief asks for a platform that hosts contestant-submitted trading
engines, subjects them to realistic exchange traffic, and ranks them fairly on
**latency, throughput, stability, and correctness** — with a strong emphasis on
*"hardcore engineering excellence and a deep understanding of scale and
distributed systems."*

That breaks down into four hard sub-problems:

1. **Safe hosting** — run arbitrary contestant code without letting it escape,
   cheat, or interfere with other contestants.
2. **Realistic load** — drive each engine with traffic that resembles a real
   exchange (multiple order types, transports, and trader behaviours), at scale.
3. **Trustworthy scoring** — measure latency/throughput accurately and verify
   *correctness* against an authoritative matching engine, so a fast-but-wrong
   engine cannot win.
4. **Live, fair ranking** — aggregate all of this into a single composite score,
   updated in real time, that is reproducible and resistant to gaming.

## 2. Objectives & non-goals

**Objectives**
- Correct-by-construction scoring: every number on the leaderboard traceable to
  real telemetry, never estimated.
- Defence-in-depth isolation for untrusted contestant code.
- Horizontally scalable, cloud-native deployment described entirely as code.
- Verifiable engineering: tests, CI, and reproducibility as first-class artifacts.

**Non-goals**
- Running a real money market (this is a benchmark harness, not an exchange).
- A persisted long-term history store beyond contest-scale retention.

## 3. High-level architecture

The platform is a Go microservice monorepo (`go.work`, 10 services, ~13K LOC)
plus a Next.js 15 web dashboard. The scoring pipeline is a streaming dataflow:

```
                     ┌─────────────┐
  contestant code →  │ submission  │ → MinIO/S3 → sandboxed image build
                     │   -svc      │
                     └─────────────┘
                            │ image
                            ▼
                     ┌─────────────┐      ┌──────────────┐
                     │ sandbox-    │─────▶│  contestant  │  (gVisor pod,
                     │  runner     │ runs │   engine     │   Guaranteed QoS,
                     └─────────────┘      └──────────────┘   CPU-pinned)
                            ▲                     ▲
              spawns Jobs   │                     │ orders (REST / WS / FIX 4.4)
                     ┌─────────────┐      ┌──────────────┐
                     │ bot-        │─────▶│  bot-worker  │  (load fleet:
                     │ coordinator │ Jobs │   fleet      │   Poisson + burst,
                     └─────────────┘      └──────────────┘   4 trader profiles)
                                                  │ per-order telemetry (gRPC stream)
                                                  ▼
                                          ┌──────────────┐
                                          │ telemetry-   │ → batched produce
                                          │  ingester    │
                                          └──────────────┘
                                                  │
                                                  ▼
                                          ┌──────────────┐
                                          │  Redpanda    │  (Kafka API; keyed by
                                          │  (log)       │   contestant_id)
                                          └──────────────┘
                                              │        │
                            ┌─────────────────┘        └────────────────┐
                            ▼                                            ▼
                     ┌─────────────┐                            ┌──────────────┐
                     │ aggregator  │  HDR latency histograms    │  validator   │  replays orders
                     │             │  + TPS, 1s windows         │              │  through a reference
                     └─────────────┘                            └──────────────┘  book → fill accuracy
                            │  /metrics                                  │  /validate
                            └──────────────────┬─────────────────────────┘
                                               ▼
                                       ┌──────────────┐
                                       │ leaderboard- │  composite score → Redis ZSET
                                       │   svc        │  → WebSocket broadcast
                                       └──────────────┘
                                               │
                                               ▼
                                       ┌──────────────┐
                                       │  web (Next)  │  live leaderboard + per-contestant
                                       └──────────────┘
```

`submission_id` is threaded end-to-end so every event, histogram, and verdict is
attributable to a specific attempt.

## 4. Components

| Service | Responsibility |
|---|---|
| `api-gateway` | HTTP entry; stdlib HS256 JWT; per-IP token-bucket rate limiting (proxy-hop aware). |
| `submission-svc` | Multipart upload; MinIO/S3 storage; sandboxed image builds with traversal- and zip-bomb-safe extraction. |
| `sandbox-runner` | Spawns gVisor-sandboxed contestant pods; watches crash/restart signals and serves a crash count. |
| `bot-coordinator` | Spawns `bot-worker` Kubernetes Jobs configured with traffic profiles. |
| `bot-worker` | Load generator: REST + WebSocket + FIX 4.4; Poisson arrivals with burst + jitter; 4 trader archetypes. |
| `telemetry-ingester` | gRPC client-streaming endpoint → ring buffer → batched Redpanda produce. |
| `aggregator` | Nanosecond HDR histograms; 1s tumbling windows; exact percentile merge; whole-run cumulative accumulator; TimescaleDB. |
| `validator` | Replays each order through a reference price-time-priority book; scores fill accuracy. |
| `leaderboard-svc` | Composite scoring; Redis ZSET ranking; WebSocket broadcast; best-of-attempts ranking. |
| `web` | Next.js 15 dashboard: live leaderboard, per-contestant detail, submission UI. |

## 5. Key design decisions

These are recorded as ADRs in the repo (`docs/ADR/`); the rationale in brief:

- **Go monorepo with `go.work`** (ADR 0001) — shared proto contracts and one
  toolchain across 10 services, while each service stays an independent module
  that builds and tests in isolation in CI.
- **gVisor for isolation** (ADR 0002) — a user-space kernel gives a far smaller
  host attack surface than runc for untrusted contestant code, layered with PSA
  `restricted`, seccomp, dropped capabilities, read-only rootfs, and
  NetworkPolicies.
- **Redpanda over Kafka** (ADR 0003) — Kafka API compatibility with a single
  binary, no JVM/ZooKeeper, lower footprint for contest-scale deployment.
- **BuildKit → Kaniko build strategy** (ADR 0004) — rootless, in-cluster image
  builds without a privileged Docker daemon.
- **Scoring consistency & recovery** (ADR 0005) — partition affinity by
  `contestant_id`, single-replica stateful scoring services, whole-run scoring,
  best-of ranking, and fail-closed correctness (see §6, §8).

## 6. Scoring model

Contestants are ranked on a composite of all four dimensions the brief names:

```
base  = 1000 · ( 0.4·latency_norm + 0.3·tps_norm + 0.3·correctness )
final = max(0, round(base) − crashes·10 000 − timeouts·1 000)
```

Every input is **live and traceable**:

- **Latency / throughput** come from the aggregator's HDR histograms. Percentiles
  are merged **bucket-by-bucket across windows — never averaged** (averaging
  percentiles is mathematically wrong). A never-evicted cumulative histogram
  means a benchmark longer than the recent-window ring is still scored on *all*
  of its data.
- **Correctness** comes from the validator replaying every order through a
  reference matching engine and comparing fills. Scoring is **fail-closed**: a
  submission is credited only once the validator has actually checked ≥1
  authoritative fill — an unverified or fast-but-wrong engine cannot bank free
  points.
- **Stability** comes from real timeout signals (the bot-worker emits an explicit
  timeout result on client timeout) and real crash signals (the sandbox-runner
  counts container restarts / failed pods).
- **Best-of attempts**: state is keyed by `submission_id`, so a re-submission gets
  a fresh book/histogram; the leaderboard keeps the **maximum** score across a
  contestant's attempts via an atomic Redis Lua upsert.

## 7. Load generation & realism

The bot fleet models a real exchange tape rather than a uniform flood:

- **Order types**: Limit, Market (IOC), and Cancel.
- **Transports**: FIX 4.4 (QuickFIX), REST, and WebSocket — exercising the same
  engine through three protocol surfaces.
- **Arrival process**: Poisson arrivals with configurable burst and jitter.
- **Trader archetypes**: `market_maker`, `aggressive_taker`, `retail`, and
  `noise`, mixed to produce realistic order-book dynamics.

## 8. Scalability & consistency model

- **Partition affinity**: telemetry is produced to Redpanda keyed by
  `contestant_id`, so all of a contestant's events land on one partition / one
  consumer — a contestant's book and histograms are never split across processes.
- **Stateless services scale horizontally** under HPA (e.g. `leaderboard-svc`
  demonstrated 6→12 replicas across a 4-node cluster).
- **Stateful scoring services (aggregator, validator) run single-replica by
  design.** Their state is in-memory and partition-sharded; more than one replica
  would serve only the slice of contestants on whichever pod answered a scrape,
  producing partial/fluctuating scores. One consumer holds the whole picture, and
  HDR record + map ops run at millions/sec on one core — ample for contest scale.
  This is enforced in Helm (pinned replicas, no HPA/PDB).
- **Reproducibility**: a determinism check asserts the scoring pipeline is
  **byte-identical (SHA-256) across replays** — no map-ordering or RNG leakage.

## 9. Security & isolation

Untrusted contestant code runs under defence-in-depth:

- gVisor user-space kernel + PSA `restricted` + seccomp + dropped Linux
  capabilities + read-only root filesystem + NetworkPolicies.
- Guaranteed-QoS contestant pods with integer CPU requests so the kubelet
  CPU-Manager pins cores — fair, low-jitter execution.
- A **12-attack suite (6 admission-time + 6 runtime)** blocks **12/12** on a live
  cluster (`docs/SANDBOX_ATTACK_REPORT.md`).

## 10. Correctness & testing

- **Differential / model-based testing**: both matching engines (the heap-based
  reference orderbook contestants build against, and the validator's scoring
  engine) are each driven against an **independent brute-force oracle** over
  thousands of random order streams, asserting identical fills and book state.
  Both matching their oracle guarantees the engine contestants build against and
  the engine that scores them agree — the platform's core fairness invariant.
- **250+ unit + integration tests** across 10 Go modules.
- **Green CI matrix on every push**: per-module `go test -race`,
  `golangci-lint`, `buf lint`, `terraform validate`, `helm lint` +
  `kubeconform`, `hadolint`, and a Docker image-build smoke test.

## 11. Results & evidence

| Claim | Evidence |
|---|---|
| Full pipeline scores from real telemetry (validator checked 11,151 fills, caught real mismatches) | `docs/E2E_PIPELINE_REPORT.md` |
| 13K req/s sustained, sub-millisecond p50, saturation curve | `docs/PERFORMANCE_REPORT.md` |
| 12/12 sandbox attacks blocked on a live cluster | `docs/SANDBOX_ATTACK_REPORT.md` |
| IaC validates and deploys on multi-node Kubernetes | `docs/IAC_VERIFICATION.md` |
| Resilience under chaos (pod kill / isolation / induced latency) | `docs/CHAOS.md` |

## 12. Deployment

- **Local, zero-infra demo** (2 terminals, no Docker/Kafka/cluster): run
  `leaderboard-svc` with `SEED_DEMO=true` + the Next.js dev server.
- **Full pipeline locally**: `./scripts/e2e-pipeline.ps1` drives bot → telemetry →
  aggregator + validator → leaderboard against the reference orderbook.
- **Multi-node Kubernetes**: a `kind` 4-node spec + the Helm umbrella chart
  (HPA, PSA `restricted`, NetworkPolicies, Guaranteed-QoS contestant pods).
- **Cloud (IaC)**: Terraform provisions AWS VPC / EKS / RDS+Timescale /
  ElastiCache / MSK / S3 / ECR, with a CPU-Manager static policy on the
  contestants node group (`docs/EKS_STAGING_RUNBOOK.md`).
- **Hosted UI**: the dashboard is deployed free on Vercel
  (https://iicpc-platform.vercel.app/) in demo mode, since the multi-service
  backend is not publicly hosted.

## 13. Limitations & future work

Stated honestly:

- **Crash recovery**: aggregator/validator state is in memory; a mid-run process
  crash loses pre-crash aggregation while the committed offset advances. This
  affects only a mid-run crash (not normal operation or the demo). The planned
  fix is to treat Redpanda as the source of truth and rebuild state from the log
  on startup (documented in ADR 0005).
- **Telemetry backpressure** under extreme overload could desync; bounded and
  tracked in `IDEAS.md`.
- **Production hardening** scoped and ready to wire: IRSA per-service IAM roles,
  external-secrets-operator → AWS Secrets Manager, an ALB/Gateway-API ingress,
  and live build-log streaming to the submission UI.

## 14. References (in repo)

`docs/ARCHITECTURE.md` · `docs/ADR/0001`–`0005` · `docs/E2E_PIPELINE_REPORT.md` ·
`docs/PERFORMANCE_REPORT.md` · `docs/SANDBOX_ATTACK_REPORT.md` ·
`docs/IAC_VERIFICATION.md` · `docs/CHAOS.md` · `docs/EKS_STAGING_RUNBOOK.md` ·
`IDEAS.md`
