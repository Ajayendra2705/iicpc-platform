"use client";

import { CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import type { TPSSample } from "../types";

export function TPSChart({ history }: { history: TPSSample[] }) {
  const data = history.map((s) => ({
    ts: new Date(s.t).toLocaleTimeString().split(" ")[0],
    tps: Math.round(s.tps),
  }));

  return (
    <div className="card">
      <h3>TPS Over Time (last {history.length}s)</h3>
      <div className="chart">
        <ResponsiveContainer width="100%" height={220}>
          <LineChart data={data}>
            <CartesianGrid stroke="#2a3454" strokeDasharray="3 3" />
            <XAxis dataKey="ts" stroke="#8b96b3" interval="preserveStartEnd" minTickGap={32} />
            <YAxis stroke="#8b96b3" width={64} />
            <Tooltip
              contentStyle={{
                background: "#131a2e",
                border: "1px solid #2a3454",
                borderRadius: 8,
              }}
              labelStyle={{ color: "#e6edf6" }}
            />
            <Line
              type="monotone"
              dataKey="tps"
              stroke="#60a5fa"
              strokeWidth={2}
              dot={false}
              isAnimationActive={false}
            />
          </LineChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}
