// Demo mode — drives the UI from deterministic, gently-drifting synthetic data
// when no backend is reachable (e.g. the Vercel-hosted frontend, which has no
// cluster behind it). Enabled by NEXT_PUBLIC_DEMO_MODE=1 at build time.
//
// The synthesis here is the single source of truth shared by the leaderboard
// (syntheticLeaderboard) and the per-contestant page (synthesizeMetrics /
// synthesizeValidator), so a contestant's leaderboard score is consistent with
// the metrics shown when you click into them.

import type { Entry, MetricsSnapshot, ValidatorReport } from "../types";

export const DEMO_MODE = process.env.NEXT_PUBLIC_DEMO_MODE === "1";

// A fixed roster so ranks shuffle among a stable set of names as scores drift.
export const DEMO_CONTESTANTS = [
  "team-quicksilver",
  "team-orderflow",
  "team-lowlatency",
  "team-matchpoint",
  "team-tickmaster",
  "team-nanosecond",
  "team-darkpool",
  "team-bidask",
  "team-coldpath",
  "team-hotloop",
  "team-zerocopy",
  "team-arbitrage",
];

// hash is a small stable string hash; identical for a given id across reloads.
export function hash(s: string): number {
  let h = 0;
  for (let i = 0; i < s.length; i++) {
    h = (h << 5) - h + s.charCodeAt(i);
    h |= 0;
  }
  return Math.abs(h);
}

export function synthesizeMetrics(id: string): MetricsSnapshot {
  const seed = hash(id);
  const phase = (Date.now() / 1000 + seed) * 0.1;
  // base TPS varies per contestant (300–3000), oscillates ±20%
  const baseTps = 300 + (seed % 2700);
  const tps = baseTps * (1 + 0.2 * Math.sin(phase));
  // P50 in 50µs–1ms range, P99 ~6× P50
  const p50 = 50_000 + (seed % 950_000);
  return {
    contestant_id: id,
    count: Math.floor(tps * 5),
    rejected: seed % 7,
    timeouts: seed % 3,
    tps,
    p50_ns: p50,
    p90_ns: Math.floor(p50 * 2.5),
    p99_ns: Math.floor(p50 * 6),
    p999_ns: Math.floor(p50 * 12),
  };
}

export function synthesizeValidator(id: string): ValidatorReport {
  const seed = hash(id);
  const total = 500 + (seed % 1000);
  const mismatches = seed % 13;
  return {
    contestant_id: id,
    total_checked: total,
    mismatches,
    correctness: 1 - mismatches / total,
  };
}

// scoreFor mirrors the platform's composite intent: reward high throughput and
// correctness, penalise tail latency. The exact constant is cosmetic — what
// matters is that it is monotonic in the same direction as the real score and
// drifts over time so ranks move, making the demo visibly live.
export function scoreFor(id: string): number {
  const m = synthesizeMetrics(id);
  const v = synthesizeValidator(id);
  const latencyFactor = 1_000_000 / m.p99_ns; // higher when P99 is lower
  return Math.round(v.correctness * m.tps * latencyFactor * 10);
}

export function syntheticLeaderboard(): Entry[] {
  return DEMO_CONTESTANTS.map((id) => ({ contestant_id: id, score: scoreFor(id) })).sort(
    (a, b) => b.score - a.score,
  );
}
