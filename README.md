# IICPC Platform — Distributed Benchmarking & Hosting

> Submission for the **IICPC Summer Hackathon 2026**.
> A platform that hosts contestant trading infrastructure in sandboxed containers,
> blasts it with thousands of distributed bots, and ranks submissions on
> **latency**, **throughput**, and **correctness** in real time.

**Pipeline:** `Code Upload → Containerized Deployment → Distributed Load Testing → Real-Time Scoring`

[Architecture blueprint](docs/ARCHITECTURE.md) ·
[IaC verification](docs/IAC_VERIFICATION.md) ·
[Performance report — 5K bots, p99 = 6.4 ms](docs/PERFORMANCE_REPORT.md) ·
[Sandbox attack report — 12/12 blocked](docs/SANDBOX_ATTACK_REPORT.md) ·
[EKS staging runbook](docs/EKS_STAGING_RUNBOOK.md) ·
[Chaos test playbook](docs/CHAOS.md)

---

## Try it in 2 terminals (no Docker, no Kafka, no cluster)

```powershell
# Terminal 1 — leaderboard backend in synthetic-demo mode
cd services\leaderboard-svc; $env:SEED_DEMO = "true"; go run .

# Terminal 2 — Next.js UI
cd web; npm install; npm run dev
```

Open `http://localhost:3000`:

- `/`              live leaderboard (WS pulse badge, sortable columns, 6 teams drifting)
- `/contestant/team-alpha` 4 stat tiles + 3 Recharts (latency / TPS / outcomes)
- `/submit`        upload form + 7-stage pipeline visualization + log viewer

To wire it to the **real** pipeline (no SEED_DEMO), start aggregator + validator + telemetry-ingester + bot-coordinator + reference-orderbook + bot-worker. See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) §6 for the dataflow.

For production EKS: `./scripts/deploy/build-images.ps1 -Push` then follow [docs/EKS_STAGING_RUNBOOK.md](docs/EKS_STAGING_RUNBOOK.md).

---

## Components

| Service | Purpose | Status |
|---|---|---|
| `api-gateway` | HTTP entry, JWT auth, per-IP rate limit | ✅ D6 |
| `submission-svc` | Accepts uploads, builds container images | ✅ D2–3 |
| `sandbox-runner` | Deploys submissions as gVisor-sandboxed K8s pods | ✅ D4 |
| `bot-coordinator` | Spawns bot Jobs with traffic profiles | ✅ D11 |
| `bot-worker` | Load gen: REST + WS + FIX 4.4, Poisson arrivals + burst + jitter | ✅ D8–13 |
| `telemetry-ingester` | gRPC stream → ring buffer → Redpanda | ✅ D15 |
| `aggregator` | HDR histograms, 1s tumbling windows → TimescaleDB | ✅ D16–17 |
| `validator` | Replay log through ref orderbook, score correctness | ✅ D18 |
| `leaderboard-svc` | Redis ZSET ranking, WS broadcast | ✅ D19–20 |
| `web` | Next.js 15 leaderboard + contestant detail + upload UI | ✅ D22–24 |

## Stack

**Languages:** Go 1.26 (services) · TypeScript / React 19 (web)
**Runtime:** Kubernetes + gVisor · Redpanda (Kafka API) · TimescaleDB · Redis · gRPC
**IaC:** Terraform (AWS) · Helm umbrella chart (HPA + PSA `restricted` + NetworkPolicies)
**CI gates:** per-module `go test -race` + `golangci-lint` · `buf lint` · `terraform validate` · `helm lint` + `kubeconform` · `hadolint` + image-build smoke

## Status

| Phase | Days | Status |
|---|---|---|
| Foundation + sandbox | D1–7 | ✅ done |
| Bot fleet (REST/WS/FIX) | D8–14 | ✅ done — 200-worker integration test, 0 errors |
| Telemetry + scoring | D15–21 | ✅ done — full chain wired bot → ingester → aggregator+validator → leaderboard |
| UI | D22–24 | ✅ done — 3 routes, synthetic fallbacks, dark theme |
| IaC + Helm + chaos | D25–28 | ✅ done — Terraform AWS, umbrella chart, 3 chaos scenarios, deploy artifacts |
| Polish + demo + submit | D29–32 | in progress |

**~28/32 days complete · ~3 days buffer remaining.**

## Repo layout

```
├── services/                Go modules (10), one per service
├── samples/reference-orderbook    minimal contestant for testing
├── web/                     Next.js 15 UI
├── proto/                   gRPC contracts + generated code
├── infra/
│   ├── terraform/           AWS: VPC, EKS, RDS+Timescale, ElastiCache, MSK, S3, ECR
│   ├── helm/iicpc-platform/ umbrella chart: deploys + HPA + NetworkPolicies
│   ├── manifests/           sandbox-runner, chrony, submission-svc, chaos templates
│   ├── docker/              shared Dockerfile.service
│   └── kind/                local cluster spec
├── scripts/
│   ├── chaos/               kill-bot-pod / isolate-contestant / inject-latency
│   ├── deploy/              build-images, smoke-eks
│   └── load-test.ps1        1K-worker manual load test driver
├── docs/                    ARCHITECTURE, CHAOS, EKS_STAGING_RUNBOOK, SETUP, ADRs
└── sandbox-images/          contestant Dockerfiles (Go / Rust / C++)
```

## Caveats / known not-yet-wired

- **IRSA service-account ↔ IAM role mappings** in the Helm chart (annotation slot exists, mappings deferred)
- **External-secrets-operator** → AWS Secrets Manager (currently helm `env:` inlines plain values)
- **Ingress class choice** (ALB controller vs nginx) — `expose: true` services have no Ingress yet
- **Build-log streaming** in submission-svc (synthetic placeholder in UI; audit item #7 deferred)

Tracked in [IDEAS.md](IDEAS.md).

## License

**Proprietary — all rights reserved.**

This repository is the author's IICPC Summer Hackathon 2026 submission. It is
made publicly visible solely to allow hackathon judges and reviewers to
inspect the code for evaluation purposes. No license is granted to copy,
modify, redistribute, sublicense, or use the code or any part of it in
derivative works without the author's prior written consent.

Copyright © 2026 Ajayendra. All rights reserved.
