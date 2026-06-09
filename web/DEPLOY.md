# Deploying the web frontend to Vercel (free)

The dashboard is a standard Next.js 15 app. It can be hosted on Vercel's free
(Hobby) tier in a couple of minutes. Because there is no public backend, it runs
in **demo mode**: a deterministic, gently-drifting synthetic dataset drives the
leaderboard and contestant pages so the live URL looks alive without a cluster.

When the real backend *is* reachable (local `kind`/docker-compose, or a hosted
cluster), leave demo mode off and point the env vars at it — the UI uses live
WebSocket/REST and only falls back to synthetic data per page if a backend is
missing.

## Option A — Vercel dashboard (no CLI)

1. Push this repo to GitHub (already done).
2. <https://vercel.com> → **Add New… → Project** → import this repo.
3. In project settings, set **Root Directory** to `web` (the app is in a
   subdirectory of the monorepo). Framework auto-detects as **Next.js**.
4. Under **Environment Variables**, add:
   - `NEXT_PUBLIC_DEMO_MODE` = `1`   (drives the synthetic demo dataset)
5. **Deploy**. You get a public `*.vercel.app` URL. Every push to `main`
   redeploys automatically.

## Option B — Vercel CLI

```bash
npm i -g vercel
cd web
vercel link            # creates the project; set Root Directory = . when asked
vercel env add NEXT_PUBLIC_DEMO_MODE production   # value: 1
vercel --prod
```

## Pointing at a real backend instead of demo mode

Leave `NEXT_PUBLIC_DEMO_MODE` unset (or `0`) and set the upstream URLs so the
Next.js rewrites (see `next.config.js`) proxy to your services:

| Env var                     | Points at            | Default (local)        |
| --------------------------- | -------------------- | ---------------------- |
| `LEADERBOARD_URL`           | leaderboard-svc REST | `http://127.0.0.1:8086` |
| `AGGREGATOR_URL`            | aggregator           | `http://127.0.0.1:8084` |
| `VALIDATOR_URL`             | validator            | `http://127.0.0.1:8085` |
| `SUBMISSION_URL`            | submission-svc       | `http://127.0.0.1:8080` |
| `NEXT_PUBLIC_LEADERBOARD_WS`| leaderboard WebSocket| `ws://127.0.0.1:8086/live` |

> Note: a serverless Vercel deployment cannot reach a backend that only listens
> on localhost. Live mode on Vercel requires the backend to be publicly
> reachable; otherwise keep `NEXT_PUBLIC_DEMO_MODE=1`. For local development the
> defaults above already work with the docker-compose stack.
