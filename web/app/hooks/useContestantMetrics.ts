"use client";

import { useEffect, useRef, useState } from "react";
import type { MetricsSnapshot, TPSSample, ValidatorReport } from "../types";

const POLL_MS = 1000;
const HISTORY_LIMIT = 60; // 60 samples × 1s = 1-minute rolling window

type Args = { contestantID: string };

type State = {
  metrics: MetricsSnapshot | null;
  validator: ValidatorReport | null;
  tpsHistory: TPSSample[];
  source: "live" | "simulated" | "loading";
  error: string | null;
};

/**
 * Polls /api/metrics/{id} and /api/validate/{id} every second; appends each
 * TPS sample to a rolling 60-sample history. If both upstreams 404, falls
 * back to deterministic synthetic data so the demo flow keeps working when
 * only SEED_DEMO mode is active.
 */
export function useContestantMetrics({ contestantID }: Args) {
  const [state, setState] = useState<State>({
    metrics: null,
    validator: null,
    tpsHistory: [],
    source: "loading",
    error: null,
  });
  const historyRef = useRef<TPSSample[]>([]);

  useEffect(() => {
    let cancelled = false;
    historyRef.current = [];

    async function getJSON<T>(url: string): Promise<T | "not_found" | "error"> {
      try {
        const r = await fetch(url);
        if (r.status === 404) return "not_found";
        if (!r.ok) return "error";
        return (await r.json()) as T;
      } catch {
        return "error";
      }
    }

    async function tick() {
      const [m, v] = await Promise.all([
        getJSON<MetricsSnapshot>(`/api/metrics/${encodeURIComponent(contestantID)}`),
        getJSON<ValidatorReport>(`/api/validate/${encodeURIComponent(contestantID)}`),
      ]);
      if (cancelled) return;

      // both backends offline or no data → synthetic
      const liveMetrics = typeof m === "object" ? m : null;
      const liveValidator = typeof v === "object" ? v : null;
      const useSynthetic = !liveMetrics && !liveValidator;

      const metrics = liveMetrics ?? synthesizeMetrics(contestantID);
      const validator = liveValidator ?? synthesizeValidator(contestantID);

      const now = Date.now();
      historyRef.current = [...historyRef.current, { t: now, tps: metrics.tps }].slice(-HISTORY_LIMIT);

      setState({
        metrics,
        validator,
        tpsHistory: historyRef.current,
        source: useSynthetic ? "simulated" : "live",
        error: null,
      });
    }

    void tick();
    const id = setInterval(tick, POLL_MS);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [contestantID]);

  return state;
}

// ---- Synthetic fallback (deterministic per contestant, drifting over time) ----

function hash(s: string): number {
  let h = 0;
  for (let i = 0; i < s.length; i++) {
    h = (h << 5) - h + s.charCodeAt(i);
    h |= 0;
  }
  return Math.abs(h);
}

function synthesizeMetrics(id: string): MetricsSnapshot {
  const seed = hash(id);
  const phase = (Date.now() / 1000 + seed) * 0.1;
  // base TPS varies per contestant (300–3000), oscillates ±20%
  const baseTps = 300 + (seed % 2700);
  const tps = baseTps * (1 + 0.2 * Math.sin(phase));
  // P50 in 50µs–1ms range, P99 ~5–10× P50
  const p50 = 50_000 + (seed % 950_000);
  return {
    contestant_id: id,
    count: Math.floor(tps * 5),
    rejected: (seed % 7),
    timeouts: (seed % 3),
    tps,
    p50_ns: p50,
    p90_ns: Math.floor(p50 * 2.5),
    p99_ns: Math.floor(p50 * 6),
    p999_ns: Math.floor(p50 * 12),
  };
}

function synthesizeValidator(id: string): ValidatorReport {
  const seed = hash(id);
  const total = 500 + (seed % 1000);
  const mismatches = (seed % 13);
  return {
    contestant_id: id,
    total_checked: total,
    mismatches,
    correctness: 1 - mismatches / total,
  };
}
