# IICPC Platform — Web UI

Next.js 15 leaderboard dashboard. Connects to `leaderboard-svc` via WebSocket
(`/live`) and falls back to REST polling (`/leaderboard`) if the socket drops.

## Run locally

Requires Node 20+.

```bash
cd web
npm install
npm run dev          # http://localhost:3000
```

By default the UI expects:

| Endpoint | URL |
|---|---|
| WebSocket | `ws://localhost:8086/live` |
| REST (proxied through Next.js rewrites) | `/api/leaderboard` → `http://localhost:8086/leaderboard` |

Override the WS URL with `NEXT_PUBLIC_LEADERBOARD_WS` and the proxy target with
`LEADERBOARD_URL`.

## Production build

```bash
npm run build && npm run start
```

## Features

**Leaderboard (`/`)**
- Live WebSocket subscription with automatic reconnect (3s)
- REST polling fallback (every 2s) while the socket is down
- Sortable columns: Rank, Contestant, Score
- Connection-status badge (connecting / connected / polling / disconnected)
- Top-rank highlight, tabular numerics

**Contestant detail (`/contestant/[id]`)**
- 4 stat tiles (current TPS, P50, P99, Correctness)
- Latency Percentiles bar chart (P50 / P90 / P99 / P99.9)
- TPS Over Time line chart (60-sample rolling window)
- Outcome Breakdown pie chart with correctness summary
- Deterministic synthetic fallback when aggregator/validator not running

**Submit (`/submit`)**
- Upload form + 7-stage pipeline visualisation
- LogViewer with color-coded auto-scroll
- Real upload + synthetic pipeline fallback (advances every 1.2s)
