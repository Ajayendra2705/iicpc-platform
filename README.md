# IICPC Platform — Distributed Benchmarking & Hosting

> Submission for the **IICPC Summer Hackathon 2026**.
> A production-grade platform that hosts contestant trading engines in hardened,
> gVisor-sandboxed containers, bombards them with a distributed fleet of trading
> bots, and ranks submissions live on **latency**, **throughput**, **stability**,
> and **correctness** — with every score traceable to real telemetry.

**Pipeline:** `Code Upload → Containerized Deployment → Distributed Load Testing → Real-Time Scoring`

[Architecture blueprint](docs/ARCHITECTURE.md) ·
[End-to-end live run — real telemetry → real score](docs/E2E_PIPELINE_REPORT.md) ·
[Performance — 13K req/s sustained, sub-ms p50](docs/PERFORMANCE_REPORT.md) ·
[Sandbox security — 12/12 attacks blocked](docs/SANDBOX_ATTACK_REPORT.md) ·
[IaC verification](docs/IAC_VERIFICATION.md) ·
[Cloud deploy runbook](docs/EKS_STAGING_RUNBOOK.md) ·
[Chaos playbook](docs/CHAOS.md)

---

## Engineering highlights

This is built for the brief's bar — *"hardcore engineering excellence… deep
understanding of scale and distributed systems"* — not a demo. The parts worth
a reviewer's time:

- **Proven end-to-end, live.** A real run drives the full scoring chain
  (bot → gRPC → Redpanda → aggregator + validator → leaderboard) and produces a
  genuine composite score from real telemetry — including a validator that
  checked **11,151 fills** and caught real mismatches. See
  [E2E_PIPELINE_REPORT](docs/E2E_PIPELINE_REPORT.md).
- **Defence-in-depth sandbox, turned into evidence.** gVisor + PSA-`restricted` +
  seccomp + dropped capabilities + read-only rootfs + NetworkPolicies. A
  **12-attack suite (6 admission + 6 runtime) blocks 12/12 on a live cluster** —
  [SANDBOX_ATTACK_REPORT](docs/SANDBOX_ATTACK_REPORT.md).
- **Statistically-correct telemetry.** Nanosecond-precision HDR histograms with
  **bucket-by-bucket percentile merging** — the leaderboard never averages
  percentiles (which is mathematically wrong); it merges raw histograms.
- **Reproducibility as a test.** A determinism check asserts the scoring pipeline
  is **byte-identical (SHA-256) across replays** — no map-order or RNG leakage.
- **Real exchange semantics.** Price-time-priority matching, FIX 4.4 (QuickFIX),
  REST, and WebSocket transports; Limit / Market (IOC) / Cancel order types;
  Poisson arrivals with burst + jitter; four diverse trader archetypes.
- **Fair, isolated execution.** Guaranteed-QoS contestant pods with integer CPUs
  for **kubelet CPU-Manager core pinning** + strict memory limits.
- **Horizontal scale, demonstrated.** Deployed on a **4-node Kubernetes cluster**
  and scaled 6→12 replicas across worker nodes (see `docs/artifacts/kind-multinode/`).
- **Engineered to ship.** 10 microservices, **250+ unit + integration tests**, and a
  **green CI matrix** (race tests, golangci-lint, buf, terraform validate, helm +
  kubeconform, hadolint, image build) on every push.

---

## Try it in 2 terminals (no Docker, no Kafka, no cluster)

```powershell
# Terminal 1 — leaderboard backend (demo data)
cd services\leaderboard-svc; $env:SEED_DEMO = "true"; go run .

# Terminal 2 — Next.js UI
cd web; npm install; npm run dev
```

Open `http://localhost:3000`:

- `/` — live leaderboard (WebSocket pulse, sortable columns)
- `/contestant/team-alpha` — 4 stat tiles + latency / TPS / outcome charts
- `/submit` — upload form + 7-stage build pipeline + log viewer

**Run the real scoring pipeline** (bot → telemetry → aggregator+validator →
leaderboard, against the reference orderbook): `./scripts/e2e-pipeline.ps1`.
**Deploy to cloud:** `./scripts/deploy/build-images.ps1 -Push` then follow
[docs/EKS_STAGING_RUNBOOK.md](docs/EKS_STAGING_RUNBOOK.md).

---

## Components

| Service | Purpose |
|---|---|
| `api-gateway` | HTTP entry · stdlib HS256 JWT · per-IP token-bucket rate limit |
| `submission-svc` | Multipart upload · MinIO/S3 storage · sandboxed image builds (traversal- & bomb-safe extraction) |
| `sandbox-runner` | Spawns gVisor-sandboxed contestant pods · watches crash signals |
| `bot-coordinator` | Spawns bot-worker K8s Jobs with traffic profiles |
| `bot-worker` | Load gen: REST + WS + FIX 4.4 · Poisson arrivals + burst + jitter · 4 trader profiles |
| `telemetry-ingester` | gRPC client-stream → ring buffer → batched Redpanda produce |
| `aggregator` | HDR histograms · 1s tumbling windows · exact percentile merge · TimescaleDB |
| `validator` | Replays orders through a reference book · price-time-priority + fill-accuracy scoring |
| `leaderboard-svc` | Composite scoring · Redis ZSET ranking · WebSocket broadcast |
| `web` | Next.js 15 — live leaderboard · contestant detail · submission UI |

## Stack

**Languages:** Go 1.26 (services) · TypeScript / React 19 (web)
**Runtime:** Kubernetes + gVisor · Redpanda (Kafka API) · TimescaleDB · Redis · gRPC
**IaC:** Terraform (AWS: VPC / EKS / RDS+Timescale / ElastiCache / MSK / S3 / ECR; CPU-Manager static policy on the contestants node group) · Helm umbrella chart (HPA + PSA `restricted` + NetworkPolicies + Guaranteed-QoS contestant pods)
**Bot fleet:** 4 trader profiles (market_maker / aggressive_taker / retail / noise) · Limit + Market + Cancel · FIX 4.4 / REST / WebSocket
**CI:** per-module `go test -race` + `golangci-lint` · `buf lint` · `terraform validate` · `helm lint` + `kubeconform` · `hadolint` + image-build smoke

## Scoring

Contestants are ranked on a composite of all four dimensions the brief names:

```
base    = 1000 · ( 0.4·latency_norm + 0.3·tps_norm + 0.3·correctness )
final   = max(0, round(base) − crashes·10 000 − timeouts·1 000)
```

Every input is live and traceable: latency/TPS from the aggregator's HDR
histograms, correctness from the validator's replay, stability from real
timeout + crash signals. See [ARCHITECTURE §7](docs/ARCHITECTURE.md).

## Evidence

| Claim | Where |
|---|---|
| Full pipeline scores from real telemetry | [E2E_PIPELINE_REPORT.md](docs/E2E_PIPELINE_REPORT.md) |
| 13K req/s sustained, sub-ms p50, saturation curve | [PERFORMANCE_REPORT.md](docs/PERFORMANCE_REPORT.md) |
| 12/12 sandbox attacks blocked on a live cluster | [SANDBOX_ATTACK_REPORT.md](docs/SANDBOX_ATTACK_REPORT.md) |
| IaC validates + deploys on multi-node Kubernetes | [IAC_VERIFICATION.md](docs/IAC_VERIFICATION.md) |
| Resilience under chaos (pod kill / isolation / latency) | [CHAOS.md](docs/CHAOS.md) |

## Repo layout

```
├── services/                Go modules (10), one per service
├── samples/reference-orderbook    worked-example contestant (heap-based matcher)
├── web/                     Next.js 15 UI
├── proto/                   gRPC contracts + generated code
├── infra/
│   ├── terraform/           AWS: VPC, EKS, RDS+Timescale, ElastiCache, MSK, S3, ECR
│   ├── helm/iicpc-platform/ umbrella chart: deploys + HPA + PSA + NetworkPolicies
│   ├── manifests/           sandbox-runner, bot namespace + RBAC, chrony, chaos
│   ├── docker/              shared Dockerfile.service
│   └── kind/                local 4-node cluster spec
├── scripts/                 e2e-pipeline, chaos, deploy, load-test
├── docs/                    ARCHITECTURE, reports, runbook, ADRs
└── sandbox-images/          contestant Dockerfiles (Go / Rust / C++)
```

## Architecture decisions

Locked design choices are recorded as ADRs in [docs/ADR/](docs/ADR/):
Go monorepo with `go.work`, gVisor for isolation, Redpanda over Kafka, and the
BuildKit→Kaniko build strategy.

## Production roadmap

The core platform is complete; the following hardening items are scoped and
ready to wire for a long-running production deployment:

- **IRSA** — per-service IAM roles via ServiceAccount annotations (slot in place)
- **External-secrets-operator** → AWS Secrets Manager for credentials
- **Ingress** — ALB/Gateway-API in front of the `expose: true` services
- **Build-log streaming** — surface live build logs to the submission UI

Forward-looking differentiators are catalogued in [IDEAS.md](IDEAS.md).

## License

**Proprietary — all rights reserved.** Made publicly visible solely for
hackathon judging. No license is granted to copy, modify, redistribute, or reuse
without the author's prior written consent. Copyright © 2026 Ajayendra.
