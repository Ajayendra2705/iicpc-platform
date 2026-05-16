"use client";

import { Cell, Pie, PieChart, ResponsiveContainer, Tooltip } from "recharts";
import type { MetricsSnapshot, ValidatorReport } from "../types";

const colors = ["#4ade80", "#fbbf24", "#f87171", "#a78bfa"];

export function ErrorBreakdown({
  metrics,
  validator,
}: {
  metrics: MetricsSnapshot | null;
  validator: ValidatorReport | null;
}) {
  const accepted = Math.max(
    0,
    (metrics?.count ?? 0) - (metrics?.rejected ?? 0) - (metrics?.timeouts ?? 0),
  );
  const data = [
    { name: "Accepted", value: accepted },
    { name: "Rejected", value: metrics?.rejected ?? 0 },
    { name: "Timeouts", value: metrics?.timeouts ?? 0 },
    { name: "Mismatches", value: validator?.mismatches ?? 0 },
  ].filter((d) => d.value > 0);

  const correctness = validator?.correctness ?? 0;

  return (
    <div className="card">
      <h3>Outcome Breakdown</h3>
      <div className="chart">
        <ResponsiveContainer width="100%" height={220}>
          <PieChart>
            <Pie data={data} dataKey="value" innerRadius={50} outerRadius={80} paddingAngle={2}>
              {data.map((_, i) => (
                <Cell key={i} fill={colors[i % colors.length]} stroke="#131a2e" strokeWidth={2} />
              ))}
            </Pie>
            <Tooltip
              contentStyle={{
                background: "#131a2e",
                border: "1px solid #2a3454",
                borderRadius: 8,
              }}
              labelStyle={{ color: "#e6edf6" }}
            />
          </PieChart>
        </ResponsiveContainer>
      </div>
      <div className="legend">
        {data.map((d, i) => (
          <div key={d.name} className="legend-row">
            <span className="swatch" style={{ background: colors[i % colors.length] }} />
            <span>{d.name}</span>
            <span className="legend-value">{d.value.toLocaleString()}</span>
          </div>
        ))}
        <div className="legend-row legend-summary">
          <span>Correctness</span>
          <span className="legend-value">{(correctness * 100).toFixed(2)}%</span>
        </div>
      </div>
    </div>
  );
}
