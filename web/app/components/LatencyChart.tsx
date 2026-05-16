"use client";

import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import type { MetricsSnapshot } from "../types";

function formatNs(ns: number): string {
  if (ns < 1_000) return `${ns} ns`;
  if (ns < 1_000_000) return `${(ns / 1_000).toFixed(1)} µs`;
  if (ns < 1_000_000_000) return `${(ns / 1_000_000).toFixed(2)} ms`;
  return `${(ns / 1_000_000_000).toFixed(2)} s`;
}

export function LatencyChart({ metrics }: { metrics: MetricsSnapshot | null }) {
  const data = metrics
    ? [
        { name: "P50", ns: metrics.p50_ns },
        { name: "P90", ns: metrics.p90_ns },
        { name: "P99", ns: metrics.p99_ns },
        { name: "P99.9", ns: metrics.p999_ns },
      ]
    : [];

  return (
    <div className="card">
      <h3>Latency Percentiles</h3>
      <div className="chart">
        <ResponsiveContainer width="100%" height={220}>
          <BarChart data={data}>
            <CartesianGrid stroke="#2a3454" strokeDasharray="3 3" />
            <XAxis dataKey="name" stroke="#8b96b3" />
            <YAxis stroke="#8b96b3" tickFormatter={formatNs} width={80} />
            <Tooltip
              contentStyle={{
                background: "#131a2e",
                border: "1px solid #2a3454",
                borderRadius: 8,
              }}
              labelStyle={{ color: "#e6edf6" }}
              formatter={(v: number) => formatNs(v)}
            />
            <Bar dataKey="ns" fill="#4ade80" radius={[6, 6, 0, 0]} />
          </BarChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}
