"use client";

import { useMemo, useState } from "react";
import type { Entry, SortDir, SortKey } from "../types";

export function LeaderboardTable({ entries }: { entries: Entry[] }) {
  const [sortKey, setSortKey] = useState<SortKey>("rank");
  const [sortDir, setSortDir] = useState<SortDir>("asc");

  const sorted = useMemo(() => {
    const indexed = entries.map((e, i) => ({ ...e, rank: i + 1 }));
    if (sortKey === "rank") {
      return sortDir === "asc" ? indexed : [...indexed].reverse();
    }
    const sign = sortDir === "asc" ? 1 : -1;
    return [...indexed].sort((a, b) => {
      if (sortKey === "contestant_id") {
        return sign * a.contestant_id.localeCompare(b.contestant_id);
      }
      return sign * (a.score - b.score);
    });
  }, [entries, sortKey, sortDir]);

  function toggle(key: SortKey) {
    if (sortKey === key) {
      setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    } else {
      setSortKey(key);
      setSortDir(key === "score" ? "desc" : "asc");
    }
  }

  function indicator(key: SortKey) {
    if (sortKey !== key) return "";
    return sortDir === "asc" ? "▲" : "▼";
  }

  if (entries.length === 0) {
    return <div className="empty">Waiting for benchmark results…</div>;
  }

  return (
    <table>
      <thead>
        <tr>
          <th onClick={() => toggle("rank")}>
            Rank<span className="sort-indicator">{indicator("rank")}</span>
          </th>
          <th onClick={() => toggle("contestant_id")}>
            Contestant<span className="sort-indicator">{indicator("contestant_id")}</span>
          </th>
          <th onClick={() => toggle("score")} style={{ textAlign: "right" }}>
            Score<span className="sort-indicator">{indicator("score")}</span>
          </th>
        </tr>
      </thead>
      <tbody>
        {sorted.map((row) => (
          <tr key={row.contestant_id}>
            <td className={`rank ${row.rank === 1 ? "top-1" : ""}`}>#{row.rank}</td>
            <td className="contestant">{row.contestant_id}</td>
            <td className="score">{row.score.toLocaleString()}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
