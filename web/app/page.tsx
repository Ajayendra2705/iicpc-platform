"use client";

import Link from "next/link";
import { LeaderboardTable } from "./components/LeaderboardTable";
import { StatusBadge } from "./components/StatusBadge";
import { useLeaderboard } from "./hooks/useLeaderboard";

const WS_URL =
  process.env.NEXT_PUBLIC_LEADERBOARD_WS ?? "ws://127.0.0.1:8086/live";
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
        <div className="header-actions">
          <Link href="/submit" className="primary submit-link">
            Submit code
          </Link>
          <StatusBadge status={status} />
        </div>
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
