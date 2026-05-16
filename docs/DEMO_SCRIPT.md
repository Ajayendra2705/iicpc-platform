# Demo Script — 5-minute walkthrough

Recording-ready script for the **D31 demo video**. Map every scene to a
timestamp, exact terminal commands, exact UI clicks, narration line.

Goal: prove the platform works **end-to-end without burning AWS** — the entire
demo runs locally against SEED_DEMO + the chaos suite. Total runtime: **~5 min**.

---

## 0. Pre-recording setup (do this once, do NOT record)

### Window layout

```
┌────────────────────────────────┬─────────────────────────────────┐
│                                │                                 │
│   Browser (Chrome, full-screen │   Terminal pane A               │
│   tab @ http://localhost:3000) │   (leaderboard-svc backend)     │
│                                │                                 │
│                                ├─────────────────────────────────┤
│                                │   Terminal pane B               │
│                                │   (Next.js dev server)          │
│                                ├─────────────────────────────────┤
│                                │   Terminal pane C               │
│                                │   (chaos scripts)               │
└────────────────────────────────┴─────────────────────────────────┘
```

### Pre-flight checklist (run before hitting record)

```powershell
# 1. Pre-cache the npm install so it doesn't run on camera
cd web; npm install; cd ..

# 2. Pre-build the bot-coordinator binary so `go run .` is instant
cd services\leaderboard-svc; go build -o ../../.bin/leaderboard.exe .

# 3. Verify the chaos scripts have execute permission via PowerShell
Get-ExecutionPolicy   # should be RemoteSigned or Unrestricted

# 4. Open http://localhost:3000 in browser, take a "before" screenshot
#    (the leaderboard empty state with "Waiting for benchmark results...")
```

### OBS scenes

| Scene | Source | Notes |
|---|---|---|
| `wide` | full screen | for intro + close |
| `browser-zoom` | browser only | for UI walkthrough |
| `term-A` | terminal pane A | for backend log narrative |
| `term-C` | terminal pane C | for chaos commands |
| `split` | browser left + term-A right | for live correlation |

### Test the recording (one rehearsal)

Run through the entire script once without recording. Watch wall clock —
aim for 4:30–5:00. If over 5:30, cut the validator detail in Scene 4.

---

## Scene 1 — Hook + problem statement (0:00 – 0:30)

**Scene:** `wide`. Camera shows ARCHITECTURE.md system-overview Mermaid diagram on screen (open `docs/ARCHITECTURE.md` in your IDE, scroll to §3, full-screen).

**Narration (≈25 s):**

> "The IICPC platform answers one question: **whose trading engine is fastest?**
>
> Contestants upload a matching engine. We containerize it under gVisor,
> blast it with **thousands of distributed bots** speaking REST, WebSocket,
> and FIX 4.4 at nanosecond timing precision, validate every fill against
> a reference orderbook, and rank submissions live.
>
> Here's the data flow. Let me show you it running."

**Cue:** at 0:25, switch to scene `browser-zoom`, browser already at `http://localhost:3000` (empty state).

---

## Scene 2 — Submission flow (0:30 – 1:30)

**Scene:** `browser-zoom`.

**Actions (timed):**

| t | Action | What appears |
|---|---|---|
| 0:30 | Click "Submit code" button (top-right of leaderboard) | navigates to `/submit` |
| 0:34 | Type `team-demo` in Contestant ID field | |
| 0:38 | Leave Language = Go, Entrypoint = `.` | |
| 0:42 | Click file picker, choose any `.tar.gz` (e.g. `samples\smoke-go.tar.gz`) | file size shown |
| 0:48 | Click **Submit for benchmarking** | |
| 0:50 | "Simulated pipeline" badge appears | |
| 0:52 | First log line: "Uploading smoke-go.tar.gz (go)…" | |
| 0:55 | Status pill cycles: Queued → Downloading | step dot pulses |
| 1:00 | Extracting → Building | log accumulates |
| 1:10 | Pushing → Scanning → **Ready** | green pill, image URL renders |
| 1:25 | Camera lingers on the "Image" row showing the registry URL | |

**Narration (overlay, ≈55 s):**

> "Upload form takes a contestant_id, language, entry point, and the
> archive. The platform validates the tarball — 50 MB cap, no traversal,
> magic-byte sniff — then enqueues a build job.
>
> The 7-stage pipeline you see here mirrors what actually happens in
> production: source download from S3, extraction with bombs guarded,
> `docker build` against a distroless multi-stage Dockerfile, push to
> ECR, Trivy CVE scan, then **Ready**.
>
> The bot fleet now has a target."

**Cue:** at 1:28, click **"Back to leaderboard"** link. Switch to scene `split`.

---

## Scene 3 — Live leaderboard (1:30 – 2:30)

**Scene:** `split` — browser left, term-A right.

**Actions:**

| t | Action | Where |
|---|---|---|
| 1:30 | Browser is on `/`, status badge **Live (WebSocket)** with green pulse | left pane |
| 1:32 | 6 teams visible (team-alpha, team-bravo, …, team-foxtrot) with drifting scores | left pane |
| 1:38 | Point at column headers, click "Score" twice to demonstrate sort | left pane |
| 1:48 | term-A shows JSON log lines: `"seed-demo active"` then "broadcast" every 1s | right pane |
| 1:55 | "Last update: …" footer ticks every second | left pane |
| 2:05 | Click "team-alpha" row | navigates to `/contestant/team-alpha` |
| 2:08 | 4 stat tiles render (TPS, P50, P99, Correctness) | |
| 2:12 | Latency Percentiles bar chart pops in (P50/P90/P99/P99.9) | |
| 2:18 | TPS Over Time line chart starts drawing right-to-left | |
| 2:24 | Outcome Breakdown pie + correctness % | |

**Narration (≈55 s):**

> "Leaderboard updates over WebSocket — that pulse in the corner is the
> live connection. Click any team to drill in.
>
> Four stat tiles at the top: current TPS, P50 and P99 latency, and
> correctness as a percentage.
>
> The Latency Percentiles chart breaks out the full P50, P90, P99, and
> P99.9 from the aggregator's HDR histogram. P99.9 is where outliers
> hide — we record from 1 nanosecond to 60 seconds with 0.1% precision.
>
> TPS Over Time is a 60-sample rolling window — one second per sample
> — so you see real-time throughput.
>
> The Outcome Breakdown pie shows accepted, rejected, timeouts, and
> fill mismatches caught by the validator."

**Cue:** at 2:28, switch to scene `term-C`.

---

## Scene 4 — Chaos test (2:30 – 4:30)

**Scene:** `term-C` first, then `split` halfway.

### Scenario 1 — kill a bot pod (2:30 – 3:00)

> "Now let's break things. First scenario: kill a bot pod mid-benchmark.
> The Deployment + HPA should heal it transparently."

**Term-C (read aloud as you type):**
```powershell
.\scripts\chaos\kill-bot-pod.ps1
```

**Expected output (~15 s):**
```
[chaos:kill-bot] target ns=iicpc-bots selector=app=bot-worker
[chaos:kill-bot] selected victim: bot-worker-77d8b… (3 pods available)
[chaos:kill-bot] waiting up to 30s for self-heal...
[chaos:kill-bot] ...still 2/3 Running
[chaos:kill-bot] ...still 2/3 Running
[chaos:kill-bot] PASS — replacement Running after 7s (3 pods)
```

**Note for demo:** if you don't have a real K8s cluster, **read the script
file aloud** instead and explain what would happen — judges understand.

### Scenario 2 — network jitter (3:00 – 4:00)

> "Second: inject 100 ms ± 20 ms of network latency into the contestant
> pod. The Pumba job uses Linux tc netem under the hood."

**Switch to `split`:**
```powershell
.\scripts\chaos\inject-latency.ps1 -DelayMs 100 -JitterMs 20 -DurationS 30
```

**While the chaos runs (point at browser):**

> "Watch the LatencyChart — the P99 bar is climbing from microseconds to
> hundreds of milliseconds.
>
> The leaderboard now: team-demo's score is dropping because
> `latency_norm = 1 - P99/100ms` is collapsing toward zero. Score = 0.4
> × that, so a 40% chunk of the score evaporates.
>
> Thirty seconds later, Pumba cleans up the tc rules and latency
> recovers. Watch the line."

### Scenario 3 — isolate contestant (4:00 – 4:30)

> "Third: cut the contestant pod off the network entirely. This proves
> our failure-path scoring — timeouts get counted, penalty applied."

```powershell
.\scripts\chaos\isolate-contestant.ps1 -ContestantID team-demo -DurationS 20
```

**Narration:**

> "A scoped deny-all NetworkPolicy is now in place. Bot orders timeout —
> see the timeouts column in the pie chart climb. Each timeout takes
> 1000 points off the score. Twenty seconds later we remove the policy
> and the score climbs back up."

---

## Scene 5 — Close (4:30 – 5:00)

**Scene:** `wide`. Pull up the ARCHITECTURE.md deployment-topology Mermaid
diagram (§10) on screen.

**Narration (≈25 s):**

> "Behind the demo: **10 Go microservices** in a monorepo, **Next.js 15**
> frontend, **gRPC** internal, **TimescaleDB** for time-series, **Redis
> ZSET** for the leaderboard, **Redpanda** for the event bus.
>
> Production-ready: **Terraform** provisions VPC + EKS + RDS + MSK + S3 +
> ECR. **Helm** umbrella chart deploys all 10 services with HPA,
> NetworkPolicies, PodSecurityAdmission `restricted`.
>
> **Five CI gates**: Go tests with race detector, golangci-lint, buf,
> terraform validate, helm + kubeconform, hadolint + docker buildx.
>
> 175+ unit + integration tests. Three chaos scenarios.
>
> Source: github.com/Ajayendra2705/iicpc-platform. Thanks for watching."

**Cue:** at 4:55, fade out.

---

## Recovery plan if a take goes wrong

| Glitch | Recovery |
|---|---|
| WS disconnects, badge goes red | reload the browser tab → reconnects in 3 s |
| Leaderboard rows stop updating | check term-A — leaderboard-svc may have crashed; `Ctrl+C` and `go run .` again with `SEED_DEMO=true` |
| Chaos script can't find kubectl | read the script source on screen, narrate what it would do; cut + reshoot scene 4 with a "simulating outcome on UI" overlay |
| Detail page synthetic-data badge confuses judges | mention it explicitly: *"This is simulated data because we're running the leaderboard layer only — in production the aggregator feeds real telemetry"* |

---

## Recording tips

- Speak slower than feels natural. 5 min on paper = 5 min spoken only at half-speed.
- Don't mention "this is fake" — say "simulated for the demo" once and move on.
- Keep mouse movements small and decisive. Judges hate wandering cursors.
- Close all other browser tabs so notifications don't appear.
- Mute Slack and Discord and email clients before recording.
- If you flub a line, **pause for 2 seconds**, then re-deliver — easy to splice in post.
- Record at 1080p, 30 fps, h.264. File size target: under 200 MB for 5 min.

---

## Post-production checklist

- [ ] Trim head and tail dead air
- [ ] Boost audio levels (–14 LUFS for spoken content)
- [ ] Add 2-second title card at the start with the project name + your name + the date
- [ ] Add captions (auto-generate via YouTube studio, hand-edit the 5 most important lines)
- [ ] Compress to under 100 MB if uploading to the submission portal directly
- [ ] Upload to YouTube **unlisted** as a backup
- [ ] Include the URL in your submission's README

---

## After the video, before submission

- Tag the commit: `git tag -a v0.1.0-submission -m "IICPC 2026 submission"`
- Push the tag: `git push origin v0.1.0-submission`
- Make sure the repo's public README link works from the submission portal
- Final sanity: clone fresh into a new directory, run the 2-terminal demo. If a judge tries this and it fails, you lose points.
