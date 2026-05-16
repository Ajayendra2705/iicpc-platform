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

## Features (Day 22)

- Live WebSocket subscription with automatic reconnect (3s)
- REST polling fallback (every 2s) while the socket is down
- Sortable columns: Rank, Contestant, Score (click headers)
- Connection-status badge (connecting / connected / polling / disconnected)
- Empty state until first benchmark arrives
- Tabular numerics, top-rank highlight

Detail pages (latency histograms, TPS over time) come in Day 23.
