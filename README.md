# IICPC Platform — Distributed Benchmarking & Hosting

Submission for IICPC Summer Hackathon 2026 (May 9 – Jun 10).

A platform that hosts contestant trading infrastructure in sandboxed containers, blasts it with thousands of distributed bots, and ranks submissions on latency, throughput, and correctness in real time.

## Pipeline

`Code Upload → Containerized Deployment → Distributed Load Testing → Real-Time Scoring`

## Components

| Service | Purpose |
|---|---|
| `api-gateway` | HTTP entry, JWT auth, rate limiting |
| `submission-svc` | Accepts uploads, builds container images |
| `sandbox-runner` | Deploys submissions as gVisor-sandboxed K8s pods |
| `bot-coordinator` | Spawns bot Jobs with traffic profiles |
| `bot-worker` | Generates load via REST/WS/FIX |
| `telemetry-ingester` | Captures order-ack timings, batches to Redpanda |
| `aggregator` | Computes P50/P90/P99, writes to TimescaleDB |
| `validator` | Replays order log, verifies price-time priority |
| `leaderboard-svc` | Redis ZSET ranking, WS broadcast to UI |
| `web` | Next.js leaderboard frontend |

## Stack

Go (services) · Next.js (web) · Kubernetes + gVisor · Redpanda · TimescaleDB · Redis · gRPC · Terraform (AWS).

## Quick Start (local)

```bash
# Bring up local kind cluster + dependencies
./scripts/kind-up.sh

# Build all services
make build

# Deploy to kind
make deploy-local

# Open leaderboard
open http://localhost:3000
```

## Layout

See `PLAN.md` for full project layout, schedule, and design decisions.
See `docs/ARCHITECTURE.md` for the system blueprint deliverable.

## Status

Day 1 — Foundation. Repo initialized, proto contracts defined, monorepo skeleton in place.
