"use client";

import { useEffect, useRef, useState } from "react";
import { DEMO_MODE, synthesizeMetrics, synthesizeValidator } from "../lib/demo";
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
      // In demo mode there is no backend — skip the network entirely and drive
      // straight from synthetic data so the hosted page never waits on a dead
      // proxy each second.
      const [m, v] = DEMO_MODE
        ? (["not_found", "not_found"] as const)
        : await Promise.all([
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
