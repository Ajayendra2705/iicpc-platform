# ADR 0005: Scoring pipeline consistency & crash-recovery model

**Status:** Accepted
**Date:** 2026-06-07

## Context

The scoring pipeline turns a stream of `OrderEvent`s into a live leaderboard:

```
bot-worker → telemetry-ingester → Redpanda → aggregator + validator → leaderboard-svc → Redis/WS
```

The aggregator (HDR latency histograms) and validator (replay against a
reference orderbook) both hold **per-contestant state in memory** and expose it
over HTTP, which leaderboard-svc polls and folds into a composite score. Several
consistency questions fall out of that design and are decided here so the
trade-offs are explicit.

## Decisions

1. **Partition affinity by `contestant_id`.** The telemetry-ingester produces to
   Redpanda keyed by `contestant_id` (`kafka.Hash` balancer). All of a
   contestant's events — across every submission — land on one partition, hence
   one consumer, so a contestant's book/histograms are never split across
   processes.

2. **aggregator & validator run single-replica.** Their state is in-memory and
   partition-sharded, and they are scraped over a load-balanced ClusterIP. More
   than one replica would serve only the slice of contestants on whichever pod
   answered a given scrape, so the leaderboard would score on partial,
   fluctuating data. A single consumer holds the whole picture; HDR record + map
   ops run at millions/sec on one core, which is ample for contest scale.
   (Enforced in Helm: pinned `replicas {min:1,max:1}`, no HPA/PDB.)

3. **Whole-run scoring via a cumulative accumulator.** Each run keeps a
   never-evicted cumulative HDR histogram in addition to a bounded recent-window
   ring. The leaderboard scores on the cumulative one, so a benchmark longer than
   the ring (`historyCap`, 60×1s) is still scored on all of its data, with
   percentiles merged bucket-by-bucket (never averaged).

4. **Per-submission isolation, ranked best-of.** State is keyed by `submission_id`
   when present (else `contestant_id` for back-compat), so a re-submission gets a
   fresh book/histogram. The leaderboard keeps the **max** score across a
   contestant's submissions via an atomic Redis Lua upsert (safe across multiple
   leaderboard-svc replicas).

5. **Fail-closed correctness.** A submission is scored only once the validator has
   actually checked ≥1 authoritative fill (`TotalChecked > 0`). A submission with
   telemetry but no verdict — or a `correctness=1.0, TotalChecked=0` default — is
   **skipped**, never credited, so an unverified (or fast-but-wrong) engine can't
   bank free correctness points.

## Known limitation: crash recovery

The aggregator/validator consume with a Kafka consumer group and commit offsets
as they process, but their state is **in memory only**. So a mid-run crash
loses the pre-crash aggregation while the committed offset advances past it —
on restart the consumer resumes *after* the lost events and never rebuilds them.

This affects only a mid-run process crash; it does not affect normal operation,
the demo, or a clean contest run. It is **not** fixed in this submission because
the correct fix is a deliberate change to the ingestion path (the highest-risk,
CI-untested surface — the Kafka path runs against stubs in tests), and a working,
verified pipeline was judged more valuable right before submission than a more
ambitious one that might be subtly broken.

**Planned fix (post-submission):** treat Redpanda as the source of truth and
rebuild state from the log on startup rather than trusting committed offsets —
e.g. consume from the earliest retained offset under an ephemeral group per pod
start (single-replica already consumes all partitions). Bounded contest topics
make full replay-on-restart cheap. The same change would let aggregator/validator
scale horizontally if state were externalised to a shared store (tracked in
`IDEAS.md`).

## Consequences

- Correct under all normal operation; the scoring path is deterministic and
  the matcher is differentially tested against an independent oracle.
- The two stateful scoring services do not scale horizontally today — an
  accepted trade-off for contest scale, with a clear path to remove it.
- The crash-recovery gap is documented and bounded rather than hidden.
