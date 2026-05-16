# IICPC Platform — Improvement Ideas Backlog

> Living list of differentiator / polish ideas surfaced during development.
> Add new ideas here as they come up. Promote to PLAN.md when scheduled.

## Differentiators beyond hackathon brief (top-10 ammo)

- **Chaos testing visible in demo:** `kubectl delete pod` contestant mid-benchmark, score drops live on UI. Single most memorable demo moment.
- **Live replay debugger:** record bot order stream per benchmark; let contestant replay it against a new submission and diff results. Turns leaderboard from "score" into "learn".
- **Sandbox escape demo:** deliberately try `nsenter`, raw socket egress, /proc poking. Show NetworkPolicy + seccomp + gVisor blocking each.
- **Replay determinism check:** capture bot stream → replay same input twice → score must match. Proves system is reproducible to judges.
- **FIX adversarial mode:** bots emit malformed 35=D (bad TransactTime, missing tag 38, oversized strings). Verify contestant orderbook rejects cleanly without crashing.
- **Multi-region geo-latency:** bot fleet in us-east, orderbook in ap-south → measure realistic 200ms+ RTT. Most hackathon submissions never leave one region.
- **Cost dashboard:** show $/benchmark for each contestant pod. Reframes "high TPS" as "high cost".
- **Score-formula explainer:** UI panel breaking down per-contestant `latency_norm × 0.4 + tps_norm × 0.3 + correctness × 0.3`. Transparency rare in similar projects.
- **WebSocket delta encoding:** leaderboard updates only send `(contestant_id, new_score)` not full table. 10× bandwidth reduction.

## Architecture future-state (post-hackathon)

- **Submission queue with priority + fairness:** currently FIFO; add per-team rate limit, premium queue tier.
- **Submission result cache:** SHA(source) → score. Skip re-benchmark of unchanged code.
- **Postgres read replicas:** leaderboard reads against replica, writes to primary.
- **Sharded bot-coordinator:** currently single instance; partition by contestant_id.
- **HDR histogram log-merging:** instead of averaging P99 across windows (statistically wrong), merge raw histograms.

## Demo / UX polish

- **Recharts for latency distribution:** histogram bars per contestant, log-scale x-axis.
- **"How am I scored?" tooltip on every leaderboard column.**
- **Demo seed data:** pre-loaded 3 fake contestants so leaderboard never empty.
- **One-click "Run benchmark" for judges:** no auth, no upload, just click → see live load test against reference orderbook.

## Tech debt to track

- bot-worker telemetry → telemetry-ingester wire not yet connected (planned post D18).
- `services/aggregator` averages P99 across windows in the continuous aggregate — note the statistical caveat in docs.
- go.work uses go 1.26.0 but services pinned to 1.22 — pick one and align.

---

_Last updated: D17 start (2026-05-15)._
