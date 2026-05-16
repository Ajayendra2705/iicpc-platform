# Chaos test playbook

Three scripted scenarios. Each one proves a specific resilience property of
the platform. Run against any cluster where the chart in
`infra/helm/iicpc-platform/` is installed and at least one benchmark is in
progress (or use `SEED_DEMO=true` for the leaderboard layer only).

| # | Scenario | What it proves | Observable signal |
|---|---|---|---|
| 1 | Kill bot-worker pod mid-benchmark | Job + Deployment auto-replace; benchmark continues | new pod within ~10s; no gap in TPS line on `/contestant/{id}` |
| 2 | Network-isolate a contestant pod (NetworkPolicy chaos) | Failure-path scoring works — score drops, validator records mismatches | `correctness` drops on detail page; rejected count climbs |
| 3 | Inject 100 ms ± 20 ms latency into contestant pod (Pumba) | Latency penalty propagates through aggregator → score | P99 latency bar grows; leaderboard rank changes |

## Prerequisites

- kubectl pointed at the cluster (kind, EKS staging, etc.)
- `iicpc` and `iicpc-contestants` namespaces present
- (scenario 3 only) Pumba-capable cluster — Pumba needs a privileged
  container with access to the target pod's network namespace. On EKS this
  is fine; on locked-down gVisor pods it won't work — see "Limitations"
  below.

## Running the scenarios

```powershell
# kill a random bot pod
./scripts/chaos/kill-bot-pod.ps1

# isolate one contestant pod for 60s
./scripts/chaos/isolate-contestant.ps1 -ContestantID team-alpha -DurationS 60

# inject network latency on contestant pods (deploys Pumba Job)
./scripts/chaos/inject-latency.ps1 -DelayMs 100 -JitterMs 20 -DurationS 30

# run all three back-to-back with reporting
./scripts/chaos/run-suite.ps1
```

Each script prints the observable signal it expects so you can correlate
against the leaderboard UI live.

## Expected outcomes — full table

### Scenario 1: kill bot-worker pod

| Time | Observable |
|---|---|
| t=0 | `kubectl delete pod ...` returns |
| t=0–5s | Deployment controller detects missing replica; ReplicaSet scales up |
| t=5–10s | New pod `Pending` → `Running` |
| t=10s+ | bot-worker emits telemetry again; **no TPS gap** wider than `BURST_DURATION_MS` |

If the new pod takes longer than 30s, investigate node capacity (HPA may have
hit max replicas) or registry pull failures.

### Scenario 2: network-isolate contestant

| Time | Observable |
|---|---|
| t=0 | NetworkPolicy `chaos-isolate-{id}` applied; pod labelled `chaos: isolated` |
| t=0–5s | bot-worker → contestant pod connections start timing out |
| t=5–10s | telemetry-ingester sees `ORDER_RESULT_TIMEOUT` flood |
| t=10s+ | aggregator's `timeouts` counter climbs; validator's `correctness` drops because expected fills no longer happen |
| t=15s+ | leaderboard score for that contestant drops sharply (timeout penalty −1000/event) |
| t=60s (default) | NetworkPolicy removed; recovery observable within 5s |

### Scenario 3: latency injection (Pumba)

| Time | Observable |
|---|---|
| t=0 | Pumba Job applies `tc qdisc add netem delay 100ms 20ms` to contestant pods |
| t=0–10s | aggregator's open window shows `p99_ns` jumping from `~µs` to `~100ms+` |
| t=10s+ | `/contestant/{id}` LatencyChart renders dramatically taller P99 bar |
| t=15s+ | leaderboard score drops as `latency_norm = clamp01(1 - P99/100ms)` shrinks |
| t=30s | Pumba Job completes; tc rules cleared; latency returns to baseline |

## Limitations

- **Pumba + gVisor**: gVisor sandbox blocks raw NETLINK socket calls that
  `tc` uses, so Pumba can only run against contestant pods that opted out of
  gVisor (`runtimeClassName: ""`). For the demo, the easier substitute is
  to run Pumba against the **bot-worker** side instead — same observable
  P99 latency increase, just from a different vantage point.
- **NetworkPolicy isolation requires a CNI that honors policy** — kind's
  default CNI does not. Install Calico or use EKS (Calico-compatible). Otherwise
  scenario 2 is a no-op.
- **Scripts assume `iicpc-bots` / `iicpc-contestants` namespaces** as
  created by the helm chart + sandbox-runner manifests.

## CI

`infra/manifests/chaos/*.yaml` is schema-checked by the `helm-lint` CI job
(kubeconform reads it alongside the chart output).
