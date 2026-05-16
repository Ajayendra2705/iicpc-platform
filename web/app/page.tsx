"use client";

import { LeaderboardTable } from "./components/LeaderboardTable";
import { StatusBadge } from "./components/StatusBadge";
import { useLeaderboard } from "./hooks/useLeaderboard";

const WS_URL =
  process.env.NEXT_PUBLIC_LEADERBOARD_WS ?? "ws://localhost:8086/live";
const REST_URL = "/api/leaderboard?top=100";

export default function Home() {
  const { entries, status, lastUpdate } = useLeaderboard({ wsUrl: WS_URL, restUrl: REST_URL });

  return (
    <main>
      <div className="header">
        <div>
          <h1>IICPC Live Leaderboard</h1>
          <div className="subtitle">
            Real-time trading-infrastructure benchmark scores. Sort by clicking
            a column header.
          </div>
        </div>
        <StatusBadge status={status} />
      </div>

      <LeaderboardTable entries={entries} />

      {lastUpdate && (
        <div className="footer">
          Last update: {new Date(lastUpdate).toLocaleTimeString()} ·{" "}
          {entries.length} contestants
        </div>
      )}
    </main>
  );
}
