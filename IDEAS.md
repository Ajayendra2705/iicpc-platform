# IICPC Platform — Improvement Ideas Backlog

> Living list of differentiator / polish ideas surfaced during development.
> Add new ideas here as they come up. Promote to PLAN.md when scheduled.

## Differentiators beyond hackathon brief

The brief explicitly says "**not a demo-to-win** hackathon" and rewards
"hardcore engineering excellence", "deep understanding of scale and
distributed systems". Ideas grouped by which lever they pull:

### A. Correctness rigor (proves engineering depth)

- **HDR histogram log-merging:** today the `telemetry_1m` continuous aggregate `AVG(p99_ns)` across 60 one-second windows — this is **statistically wrong** (you can't average percentiles). Real fix: store the binned histogram per window, merge them with HDR's `add()` semantics, recompute the merged P99. Honest, judge-impressive, surfaces a real correctness bug we already documented.
- **Property-based testing with gopter:** generate random order streams, assert validator catches **every** fill-mismatch; run 10 000 randomised scenarios in CI. Proves the validator isn't just lucky on hand-rolled fixtures.
- **Differential testing:** for every contestant order, run the bot's claimed fills through the reference orderbook AND a second known-good engine (e.g. a sorted-map Python orderbook), diff the three. Catches subtle off-by-one bugs the unit tests miss.
- **TLA+ spec of the scoring pipeline:** model `bot → ingester → aggregator → leaderboard` as a TLA+ spec, verify "every emitted OrderEvent eventually contributes to exactly one window snapshot" under message loss + reorder. Quintessential "I understand distributed systems" signal.
- **FIX 4.4 conformance test suite:** drive the bot-worker's FIX client against the FIX OASIS test cases (35=D variants, sequence-gap recovery, logon/logout edge cases). Most hackathon FIX implementations skip this.

### B. Performance depth (proves "high-performance code")

- **Lock-free MPSC ring buffer:** rewrite `internal/buffer` from Go channel → a real lock-free SPSC/MPSC ring buffer with cache-line padding, prove < 50 ns p99 push under contention via benchmark.
- **eBPF latency profiler sidecar:** ship a sidecar container that uses eBPF (`bpftrace`/`libbpf-go`) to measure real syscall latency inside the contestant pod — kernel-eye-view of the engine. Hard but unmistakable engineering.
- **io_uring bot-worker variant:** replace `net/http` REST client with raw `io_uring` syscalls via [iouring-go]. Show 5–10× lower p99 on the bot side, proving your platform's measurement noise floor is bot-side, not engine-side.
- **HFT-style binary protocol:** implement an ITCH/OUCH-style fixed-width binary protocol alongside FIX/REST/WS. Shows you understand why real exchanges don't use JSON.
- **Sharded leaderboard with consistent hashing:** shard the Redis ZSET across N nodes by contestant_id hash, merge via top-K-of-shards-merge on read. Proves you've thought about > 1 Redis box.

### C. Reproducibility / trust (proves system thinking)

- **Live replay debugger:** record bot order stream per benchmark; let a contestant replay it against a new submission and diff results. Turns leaderboard from "score" into "learn".
- **Replay determinism check:** capture bot stream → replay same input twice → score must match exactly. Run as a nightly CI job; fail if non-deterministic.
- **Audit log Merkle tree:** every score change appends to a Merkle log; contestants get a proof they can verify off-platform. No contestant can dispute their score retroactively.
- **Per-pod cost accounting:** read cgroup v2 cpu.stat + memory.peak, attribute $$/benchmark to each contestant. Reframes "high TPS" as "high cost" — judges love efficiency stories.

### D. Resilience / chaos depth (proves "system resilience")

- **Chaos testing visible in demo:** `kubectl delete pod` contestant mid-benchmark, score drops live on UI. Single most memorable demo moment (already shipped in D27 — extend with auto-recovery time SLO).
- **Stress-the-chaos:** kill bot pods at increasing rates until score-recovery latency exceeds 5 s; publish the failure cliff in `docs/`. Shows you've explored your platform's limits.
- **Network-partition leaderboard split-brain test:** introduce a 30 s partition between the two leaderboard replicas, then heal; verify final state is deterministically merged. Tests the cluster's CP/AP behaviour explicitly.
- **Saga-style submission rollback:** if build fails halfway, all side effects (S3 object, registry tag, queued benchmark) are deterministically rolled back. Proves you've thought beyond happy path.

### E. Sandbox / security depth (proves "isolation strategy")

- **Sandbox escape demo:** deliberately try `nsenter`, raw socket egress, `/proc/<pid>/mem` poking. Show NetworkPolicy + seccomp + gVisor blocking each, with packet captures.
- **Capability fuzzer:** spawn a privileged pod that tries 200 syscalls one by one against the gVisor sandbox boundary, log which ones returned EPERM. Empirically proves your defence-in-depth.
- **FIX adversarial mode:** bots emit malformed 35=D (bad TransactTime, missing tag 38, oversized strings, integer overflow in qty). Verify contestant orderbook rejects cleanly without crashing.
- **Privilege-escalation fuzzer:** running as uid 65532 inside the contestant pod, try every known kernel CVE primitive in scope. Surface results to a "security score" column on the leaderboard.

### F. Observability / UX (judge-visible polish)

- **Live trace viewer:** every order's path (bot → ingester → aggregator → leaderboard) rendered as a flame graph on the contestant detail page, in real time.
- **Latency heatmap:** 2D heatmap of (contestant × time) latencies, Recharts → D3, hover to drill. Visually striking.
- **Score-formula explainer:** UI panel breaking down per-contestant `latency_norm × 0.4 + tps_norm × 0.3 + correctness × 0.3` with sparklines. Transparency rare in similar projects.
- **Replay-while-paused mode:** judge clicks "Pause" on a benchmark, scrubs back 30 s on the TPS chart, replays the exact 1 s window. Brings the platform from monitor → debugger.

### G. Scale claim proofs (proves "deep understanding of scale")

- **Honest 5K-bot run + writeup:** spin up the kind cluster on a beefy laptop, drive 5 000 concurrent bots against the reference orderbook for 60 s, publish the latency CDF + p99 + sustained TPS + memory headroom. Currently we only have 200-bot CI proof; the 5K target is the brief's hard requirement.
- **Multi-region geo-latency:** bot fleet in us-east, orderbook in ap-south → measure realistic 200 ms+ RTT. Most hackathon submissions never leave one region.
- **Cold-start budget:** measure `Pending → Running → /health 200` time per contestant pod across the 3 supported langs (Go / Rust / C++). Brief's target is < 5 s.
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

_Last updated: D32 — differentiator brainstorm (2026-05-17)._

## Tech debt picked up since last update

- D21: bot-worker → telemetry-ingester wire is live (closed pre-existing TODO).
- D22.5: Windows IPv6 loopback fix — all default endpoints use 127.0.0.1 not localhost. Document in SETUP.md if not already there.
- D24: submission-svc build log streaming still deferred (audit item #7); UI uses synthetic line sequence as placeholder.
- D25: IRSA service-account → IAM-role mapping not yet wired in helm chart (planned alongside external-secrets-operator).
- D26: helm chart inlines plain `env:` for service config; long lists (Kafka brokers) should move to ConfigMap; secrets need external-secrets-operator + AWS Secrets Manager.
- D28: go.work directive auto-bumps when running `go mod tidy` on a newer toolchain — pin `GOTOOLCHAIN=go1.26.0` locally to prevent drift.
- D29: P99 averaging in `telemetry_1m` continuous aggregate is statistically lossy — documented in ARCHITECTURE.md §9; use `max_p99_ns` column for exact bucket maxima.

## Resolved differentiators

(None yet — all D29+ work.)
