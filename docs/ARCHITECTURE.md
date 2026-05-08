# Architecture Blueprint

> Deliverable #2 for IICPC Summer Hackathon 2026. This document is a living spec; it will be expanded over the course of the hackathon as decisions firm up.

## 1. Goals

Build a distributed benchmarking and hosting platform that:

1. Accepts contestant trading-engine source (C++, Rust, Go).
2. Containerizes and sandboxes the submission with strong isolation.
3. Spawns thousands of distributed bots that bombard the contestant endpoint with FIX/REST/WebSocket traffic.
4. Captures latency, throughput, and correctness telemetry at nanosecond precision.
5. Streams a real-time, ranked leaderboard.
6. Scales horizontally on Kubernetes.

## 2. Non-Goals (for the prototype)

- Multi-region deployment.
- Replay debugging tools for contestants.
- Per-contestant historical comparison UI beyond simple charts.
- Anti-cheat / static analysis of submitted code (resource limits + network isolation are the safety net).

## 3. High-Level Architecture

```
              ┌──────────────┐
   browser ──►│  Web (Next)  │ live updates over WebSocket
              └──────┬───────┘
                     │ REST + WS
              ┌──────▼───────┐
              │ API Gateway  │ JWT, rate limits, routing
              └──┬─────┬─────┘
        upload  │     │  scores
                ▼     ▼
   ┌────────────────┐  ┌──────────────────┐
   │ Submission Svc │  │ Leaderboard Svc  │◄──┐
   └───────┬────────┘  └─────────▲────────┘   │
           │ build+push image     │  ZSET     │
           ▼                      │           │
   ┌────────────────┐         ┌───┴────┐      │
   │ Sandbox Runner │         │ Redis  │      │
   │ (K8s API)      │         └────────┘      │
   └───────┬────────┘                         │
           │ creates Pod (gVisor + limits)    │
           ▼                                  │
   ┌──────────────────┐                       │
   │ Contestant Pod   │◄── orders ──┐         │
   │ (sandboxed)      │             │         │
   └──────────────────┘             │         │
                                    │         │
                          ┌─────────┴────────┐│
                          │ Bot Coordinator  ││
                          └────────┬─────────┘│
                                   │ K8s Jobs │
                                   ▼          │
                          ┌──────────────────┐│
                          │ Bot Workers (N)  ││
                          └────────┬─────────┘│
                                   │ telemetry│
                                   ▼          │
                          ┌──────────────────┐│
                          │ Telemetry Ingest ││
                          └────────┬─────────┘│
                                   │ Redpanda │
                                   ▼          │
                  ┌──────────┐  ┌────────────┐│
                  │Aggregator│  │ Validator  ├┘
                  └────┬─────┘  └─────┬──────┘
                       │              │
                       ▼              ▼
                 ┌──────────┐    ┌─────────────┐
                 │TimescaleDB│   │Score Calc   │
                 └──────────┘    └─────────────┘
```

## 4. Component Contracts

All inter-service communication uses gRPC. Source-of-truth contracts live in `proto/`:

- `submission.proto` — upload, build status, log streaming
- `bot_control.proto` — start/stop load tests, traffic profiles
- `telemetry.proto` — order-event streaming, metric queries
- `leaderboard.proto` — ranked queries, score submission, live update stream

External-facing HTTP/WebSocket is exposed only by `api-gateway` and `leaderboard-svc` (for browser WS).

## 5. Data Stores

| Store | Use |
|---|---|
| MinIO / S3 | Raw submission archives, build artifacts |
| Container registry | Built contestant images |
| TimescaleDB | Time-series metrics (hypertables, continuous aggregates) |
| Redis | Live leaderboard ZSET, ephemeral load-test state |
| Redpanda | Event bus: order events, score updates, audit log |
| Postgres (separate from Timescale) | Submissions, contestants, runs metadata |

## 6. Isolation Strategy

Contestant pods are deployed with the following hardening:

- `runtimeClassName: gvisor` — userspace kernel between syscalls and host.
- `securityContext`:
  - `runAsNonRoot: true`
  - `readOnlyRootFilesystem: true`
  - `allowPrivilegeEscalation: false`
  - `capabilities.drop: [ALL]`
  - `seccompProfile.type: RuntimeDefault`
- Resource limits (cgroups v2): 2 CPU, 512Mi memory, 256 pids.
- NetworkPolicy: ingress from `bot-worker` namespace only; no egress except DNS.
- Dedicated node pool (`iicpc.io/pool=contestants`) with `NoSchedule` taint, so platform services never co-locate.
- Image scanned with Trivy before deployment; HIGH/CRITICAL CVEs block.

## 7. Scoring Formula

```
score = 0.4 * latency_norm + 0.3 * tps_norm + 0.3 * correctness
penalties:
  - crash:   -10000
  - timeout:    -10 per occurrence
  - fill mismatch: -100 per occurrence
```

`latency_norm` is `1 - (p99_ns / max_p99_ns_in_cohort)`, clamped to `[0, 1]`.
`tps_norm` is `min(1, observed_tps / target_tps)`.
`correctness` is `valid_fills / total_fills` (price-time priority + spread sanity).

## 8. Deployment Topology

- **Local:** kind cluster with 4 nodes (control-plane + 3 workers). Workers labeled `services`, `contestants`, `bots`.
- **Cloud:** AWS EKS, three managed node groups mirroring the local labels. Spot instances for `bots` pool, on-demand for `services`, dedicated tenancy for `contestants` (cost vs isolation tradeoff).

## 9. Observability

- Structured logs (zerolog/zap, JSON) → Loki.
- Metrics (Prometheus) → Grafana, with HDR-histogram exposition for latency.
- Distributed tracing (OpenTelemetry) → Tempo or Jaeger. `trace_id` propagated through every order event.

## 10. Open Decisions

- Final choice between MSK Serverless vs Redpanda Cloud for managed bus.
- Whether to implement FIX 4.4 in week 2 or push to stretch goal.
- gVisor performance overhead for C++ submissions — fallback profile if measurable degradation.

(See `docs/ADR/` for individual decision records as they accumulate.)
