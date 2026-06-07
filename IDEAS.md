# IICPC Platform — Improvement Ideas Backlog

> Living list of differentiator / polish ideas surfaced during development.
> Add new ideas here as they come up. Promote to PLAN.md when scheduled.

## Differentiators beyond hackathon brief

The brief explicitly says "**not a demo-to-win** hackathon" and rewards
"hardcore engineering excellence", "deep understanding of scale and
distributed systems". Ideas grouped by which lever they pull:

### A. Correctness rigor (proves engineering depth)

- **HDR histogram log-merging:** today the `telemetry_1m` continuous aggregate `AVG(p99_ns)` across 60 one-second windows — this is **statistically wrong** (you can't average percentiles). Real fix: store the binned histogram per window, merge them with HDR's `add()` semantics, recompute the merged P99. Honest, judge-impressive, surfaces a real correctness bug we already documented.
- **Property-based testing:** ✅ **DONE** — `services/validator/internal/replay/validator_property_test.go` generates 2 000 randomised order streams per property and asserts three invariants: a correct contestant always scores 1.0 (no false positives), every injected fill error is caught exactly (no false negatives), and scoring is deterministic. Dependency-free (no gopter needed); proves the validator isn't just lucky on hand-rolled fixtures. Next step toward differential testing below.
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

- ~~bot-worker telemetry → telemetry-ingester wire not yet connected~~ — **resolved D21** (commit `735adc2`).
- `services/aggregator` averages P99 across windows in the continuous aggregate — note the statistical caveat in docs. (Mitigated by A1 merged-percentile endpoint, but SQL-side averaging in the CAGG remains for backward-compat.)
- ~~go.work uses go 1.26.0 but services pinned to 1.22~~ — **resolved D28+**: services pinned to 1.25 toolchain, Dockerfiles use golang:1.26-alpine. `GOTOOLCHAIN=go1.26.0` env recommended locally to prevent `go mod tidy` drift.

---

_Last updated: D32 — all 4 differentiators shipped + 4 PS-audit gaps closed (2026-05-18)._

## Tech debt picked up since last update

- D21: bot-worker → telemetry-ingester wire is live (closed pre-existing TODO).
- D22.5: Windows IPv6 loopback fix — all default endpoints use 127.0.0.1 not localhost. Document in SETUP.md if not already there.
- D24: submission-svc build log streaming — DONE. Real `docker build`/`push` output is captured per submission (internal/buildlog) and served at `GET /submissions/{id}/logs?since=N`; the UI polls it live and only falls back to the synthetic stage narration when no real logs are present.
- D25: IRSA service-account → IAM-role mapping not yet wired in helm chart (planned alongside external-secrets-operator).
- D26: helm chart inlines plain `env:` for service config; long lists (Kafka brokers) should move to ConfigMap; secrets need external-secrets-operator + AWS Secrets Manager.
- D28: go.work directive auto-bumps when running `go mod tidy` on a newer toolchain — pin `GOTOOLCHAIN=go1.26.0` locally to prevent drift.
- D29: P99 averaging in `telemetry_1m` continuous aggregate is statistically lossy — documented in ARCHITECTURE.md §9; use `max_p99_ns` column for exact bucket maxima.

## Resolved differentiators

- **A1 — HDR histogram log-merging (shipped D32, 2026-05-17):** aggregator
  now keeps a 60-window ring of flushed histograms per contestant and exposes
  `GET /metrics/merged/{contestant_id}?windows=N` that returns
  bucket-by-bucket merged percentiles instead of averaging per-window p99s.
  New tests in `services/aggregator/internal/windowing/merge_test.go` prove
  merged ≠ averaged. SQL-side correctness gap explicitly documented in
  `infra/timescaledb/migrations/001_telemetry_schema.sql`.
- **G1 — Honest 5K-bot benchmark + writeup (shipped D32, 2026-05-17):**
  `services/bot-worker/benchmark_test.go::TestPerfReport_5K` drives 5 000
  concurrent worker goroutines via the production `runWorker` against an
  in-process httptest server, captures every latency into HDR, and writes
  a 50-point CDF + percentiles + TPS to JSON. Measured: **13 021 sustained
  req/s, 0.003 % error rate, p99 = 6.4 ms**. Full report:
  `docs/PERFORMANCE_REPORT.md` + raw JSON `docs/perf-report-5k.json`.
- **E1 — Sandbox attack suite (shipped D32, 2026-05-17):** 12 attacks (6
  admission-time + 6 runtime) prove each defence layer in the contestant
  pod's securityContext actually blocks a concrete attack. Runner script
  `scripts/sandbox-attack-test.ps1` produces `docs/SANDBOX_ATTACK_REPORT.md`
  with verified outcomes; exits non-zero on any ESCALATED → CI-wireable
  as a regression guard against future PRs that weaken isolation.
- **C2 — Replay determinism check (shipped D32, 2026-05-17):**
  `services/aggregator/internal/windowing/determinism_test.go` proves the
  scoring pipeline (synthetic-event-stream → aggregator → score formula)
  is byte-identical across replays. Two runs with the same seed + injected
  clock produce the same SHA-256 over the final snapshot stream AND the
  same derived score vector. Added `windowing.WithClock` option for the
  clock-injection plumbing. Catches hidden non-determinism (map-iteration
  order leaks, unseeded RNG, time.Now in hot path) at unit-test time.

## PS-audit gap closures (D32, 2026-05-18)

After a deliberate word-by-word PS re-read, four PS-language gaps were
closed before tagging `v0.1.0-submission-rc2`:

- **Market orders (PS §"Limit Orders, Market Orders, Cancels"):** new
  `gen.Kind` enum (Limit, Market), `OrderClient.PlaceMarketOrder`
  implemented for REST/WS/FIX (FIX OrdType=1 with no Price tag),
  reference-orderbook `PlaceMarket` IOC matcher, validator
  `Book.PlaceMarket`, telemetry `ORDER_TYPE_MARKET` plumbed. Commit
  `1cd7f06`.
- **CPU pinning (PS §"CPU pinning, strict memory limits"):** contestant
  pods now Guaranteed QoS (requests==limits, integer CPUs). EKS
  contestants node group sets `--cpu-manager-policy=static
  --reserved-cpus=0` via terraform `pre_bootstrap_user_data`. New
  `TestPodSpecGuaranteedQoS` asserts both invariants. Commit `1cd7f06`.
- **Diverse trader profiles (PS §"diverse market participants"):**
  4 archetypes — `market_maker` (tight spread, high cancel, no market
  orders), `aggressive_taker` (60 % market, near-zero cancel), `retail`
  (wide sigma, low rate), `noise` (mixed default). bot-coordinator
  spawns Indexed Job that rotates profiles across pod index via
  downward-API `JOB_COMPLETION_INDEX`. Commit `55c9535`.
- **Saturation curve (PS §"Maximum TPS handled before failure"):**
  `TestSaturationCurve` 7-step RPS ramp with breakpoint detection and
  Windows-tick noise filter; report in `docs/PERFORMANCE_REPORT.md`
  §"Saturation curve". Commit `55c9535`.

### Known limitations (surfaced in review, deferred)

- **Per-submission state is never evicted (P4, hygiene):** validator
  books/counters, aggregator `latest`/`history`, and the Redis hash
  `leaderboard:scores:subs:{contestant}` grow one entry per resubmission
  forever. Fine at demo/hackathon scale; for long-running prod add a TTL
  or prune on submission completion.
- **Redundant ingesters when leaderboard-svc scales out:** every replica
  runs its own `ingest.Run` loop (no leader election), so with the prod
  `replicas: {min: 3}` all three fetch upstream, score, and broadcast
  every tick — 3× the work and 3× WS fan-out. Now *correct* (the
  `UpsertSubmission` Lua script is atomic, commit pending) but wasteful;
  a real fix is single-writer election (k8s lease) or sharding contestants
  across replicas.

### Correctness audit — deferred findings (bottom-to-top review)

Confirmed real but not fixed in this pass (deeper design changes or low impact);
the high-impact items from the same audit (validator zero-checked fail-open,
aggregator/validator multi-replica partial-data, reference-book stale heap index)
were fixed.

- **At-least-once redelivery double-counts (validator + aggregator).** Both Kafka
  consumers commit the offset AFTER the handler runs (correct for at-least-once),
  but on crash/rebalance a redelivered event is reprocessed: the validator
  re-places the order on its book and re-increments total/mismatches; the
  aggregator re-records the latency. Correct fix is idempotency — dedup by
  (submission_id, order_id) or track committed offsets — but that's a non-trivial
  design change. Window is small (only on crash/rebalance) and both are now single
  replica, limiting blast radius.
- **Telemetry loss under backpressure can desync the validator.** The ingester's
  bounded buffer drops OrderEvents when full (observable via the dropped counter
  and IngestAck.EventsReceived). A dropped event the validator never sees leaves
  its reference book missing that order, which can cause FALSE correctness
  mismatches on later fills. Mitigations: larger buffer / more ingester replicas /
  block-with-timeout instead of drop. Capacity/policy tradeoff, not a logic bug.
- **FIX cancel uses exchange OrderID as OrigClOrdID(41) and hardcodes Side/Qty.**
  FIX 4.4 OrderCancelRequest should reference the original ClOrdID(11) via tag 41;
  the bot-worker passes the exchange OrderID(37) instead, and sets Side=1/Qty=1
  regardless of the original. Affects the LOAD GENERATOR's realism against
  contestant engines (cancels may be rejected), not platform scoring integrity.
  Pairs with the existing "FIX 4.4 conformance test suite" idea above.
- **Rate limiter keys on RemoteAddr.** Behind a k8s ingress/LB, RemoteAddr is the
  proxy IP, so all clients share one bucket. Needs trusted X-Forwarded-For
  handling (with a trusted-proxy allowlist to avoid spoofing) to limit per real
  client.
- **submission-svc Postgres Get/List swallow DB errors as "not found".** A
  transient DB error is indistinguishable from a missing row, so a real
  submission can momentarily look absent. Fixing means widening the Store
  interface to return errors (ripples through callers). UpdateStatus also does not
  validate status transitions (driven internally, so low risk).
