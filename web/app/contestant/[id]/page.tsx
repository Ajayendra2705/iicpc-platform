"use client";

import Link from "next/link";
import { use } from "react";
import { ErrorBreakdown } from "../../components/ErrorBreakdown";
import { LatencyChart } from "../../components/LatencyChart";
import { TPSChart } from "../../components/TPSChart";
import { useContestantMetrics } from "../../hooks/useContestantMetrics";

function formatNs(ns: number | undefined): string {
  if (!ns) return "—";
  if (ns < 1_000) return `${ns} ns`;
  if (ns < 1_000_000) return `${(ns / 1_000).toFixed(1)} µs`;
  return `${(ns / 1_000_000).toFixed(2)} ms`;
}

export default function ContestantPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const contestantID = decodeURIComponent(id);
  const { metrics, validator, tpsHistory, source } = useContestantMetrics({ contestantID });

  return (
    <main>
      <div className="header">
        <div>
          <Link href="/" className="back-link">← Back to leaderboard</Link>
          <h1>{contestantID}</h1>
          <div className="subtitle">
            {source === "simulated" && (
              <span className="badge polling">
                <span className="dot" /> Simulated data (aggregator not running)
              </span>
            )}
            {source === "live" && (
              <span className="badge connected">
                <span className="dot" /> Live data
              </span>
            )}
            {source === "loading" && <span>Loading…</span>}
          </div>
        </div>
      </div>

      <div className="stat-row">
        <Stat label="Current TPS" value={metrics ? Math.round(metrics.tps).toLocaleString() : "—"} />
        <Stat label="P50 latency" value={formatNs(metrics?.p50_ns)} />
        <Stat label="P99 latency" value={formatNs(metrics?.p99_ns)} />
        <Stat
          label="Correctness"
          value={validator ? `${(validator.correctness * 100).toFixed(2)}%` : "—"}
        />
      </div>

      <div className="charts">
        <LatencyChart metrics={metrics} />
        <TPSChart history={tpsHistory} />
        <ErrorBreakdown metrics={metrics} validator={validator} />
      </div>
    </main>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="stat">
      <div className="stat-label">{label}</div>
      <div className="stat-value">{value}</div>
    </div>
  );
}
