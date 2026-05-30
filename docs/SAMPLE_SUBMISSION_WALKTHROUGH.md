# Sample submission walkthrough

End-to-end recipe for contestants. Takes you from "I have an orderbook
written in Go / Rust / C++" to "my engine is on the leaderboard".

The reference orderbook in `samples/reference-orderbook/` is a worked
example of every requirement.

---

## 1. The runtime contract

Your binary must serve HTTP on the port given by the `RUNTIME_PORT` env var
(default 9100) and implement these four endpoints:

| Method | Path | Body / Params | Response |
|---|---|---|---|
| `GET` | `/health` | — | `200 OK` (no body required) |
| `POST` | `/order` | JSON: `{"side":"buy\|sell", "kind":"limit\|market" (default limit), "price":<float, omit for market>, "qty":<int>, "id":<string opt>}` | `200 OK` + JSON: `{"id":"<server-assigned-or-echoed>", "fills":[Fill]}` |
| `DELETE` | `/order/{id}` | — | `204 No Content` if cancelled, `404` if id unknown |
| `GET` | `/orderbook` | — | JSON snapshot: `{"bids":[Level], "asks":[Level]}` |

Where:

```json
Fill: {"buy_order_id": "ord-1", "sell_order_id": "ord-2", "price": 100.0, "qty": 5}
Level: {"price": 100.0, "qty": 50}
```

If your engine's design doesn't use server-generated IDs, accept the
client's `id` field on `POST /order` and echo it back. The bot fleet sends
its own IDs but is happy to accept yours instead — see
`samples/reference-orderbook/main.go` for the canonical handler shape.

**Market order semantics (IOC):** when `kind == "market"`, `price` is
absent and the engine must match the order against the opposite book at
any price, returning fills for whatever it could match. The unfilled
remainder **must not rest** on the book — market orders are
Immediate-Or-Cancel by convention. Limit orders (the default) rest the
unfilled remainder as usual.

---

## 2. Build constraints

| Constraint | Rule |
|---|---|
| Source archive | `.tar.gz`, ≤ 50 MB |
| Tar shape | extracts cleanly with no path-traversal entries |
| Build system | the corresponding `sandbox-images/Dockerfile.{go,rust,cpp}` must be able to build your tree |
| Entry point | a single binary at `./main` (or whatever your `Dockerfile.<lang>` produces) |
| Network | egress is **blocked**; ingress only on `RUNTIME_PORT` |
| Filesystem | rootfs is read-only; only `/tmp` is writable (size capped) |
| Identity | runs as uid 65532 — don't try `chown root` |
| CPU / memory | hard cap at 1 CPU / 512 MiB (Guaranteed QoS — requests==limits, integer CPUs → pinned by kubelet CPU Manager static policy on EKS); pid limit 256 |
| Privileges | `capabilities.drop: [ALL]`, `runtimeClassName: gvisor` |

If your code does `unsafe` syscalls or pokes `/dev`, gVisor will block it.
Plain user-space code is fine.

---

## 3. Try it locally first (no upload needed)

```powershell
# Build the reference orderbook
cd samples\reference-orderbook
go build -o ../../.bin/reference-orderbook.exe .

# Run it
$env:RUNTIME_PORT = "18080"
..\..\.bin\reference-orderbook.exe
```

In another terminal, hammer it from the bot-worker:

```powershell
cd services\bot-worker
$env:TARGET_URL       = "http://localhost:18080"
$env:NUM_WORKERS      = "100"
$env:ORDERS_PER_SECOND = "50"
$env:ARRIVAL_MODE     = "poisson"
go run .
```

Then in a third terminal:

```powershell
# Watch live latency stats
Invoke-RestMethod http://localhost:9090/metrics
```

A passing engine produces something like:
```json
{
  "count": 1500,
  "errors": 0,
  "tps": 50.0,
  "p50_ns": 87100,
  "p90_ns": 612000,
  "p99_ns": 2150000
}
```

If `errors > 0`, your engine is rejecting orders the bot considers valid.
Check `bot-worker` stderr — it logs the rejection reason.

If `p99_ns > 10_000_000` (10 ms) on loopback, your code is slow even
without contention. Optimize before submitting.

---

## 4. Package and submit

```powershell
# Tar your source
cd path\to\your\engine
tar -czf my-engine.tar.gz --exclude='.git' --exclude='target' --exclude='node_modules' .
```

Through the **web UI** (`http://localhost:3000/submit` or the production
URL on submission day):

1. Contestant ID: any unique slug (`team-foxtrot`).
2. Language: `go` / `rust` / `cpp`.
3. Entrypoint: usually `.` (the project root).
4. File: pick `my-engine.tar.gz`.
5. Submit. Watch the 7-stage pipeline progress; when status hits
   **Ready** your image is registered and a benchmark will start.

Or via the **API gateway** (CI/CD-friendly):

```powershell
# Get a JWT
$tok = (Invoke-RestMethod -Method POST `
        -Uri http://localhost:8080/auth/token `
        -ContentType "application/json" `
        -Body (@{contestant_id="team-foxtrot"} | ConvertTo-Json)).token

# Upload
$form = @{
  file = Get-Item -Path .\my-engine.tar.gz
  lang = "go"
  entrypoint = "."
  contestant_id = "team-foxtrot"
}
Invoke-RestMethod -Method POST `
  -Uri http://localhost:8080/submissions `
  -Headers @{Authorization = "Bearer $tok"} `
  -Form $form
```

The response gives you a submission ID. Poll `GET /submissions/<id>` to
watch status transition `queued → ready` or `failed`.

---

## 5. What the platform measures

Once your pod is running, the bot fleet starts. Every order it sends
produces an **OrderEvent** containing nanosecond send + ack timestamps.

| Metric | Captured by | Window |
|---|---|---|
| Latency P50/P90/P99/P99.9 | aggregator (HDR histograms) | rolling 1 s |
| TPS | aggregator (count ÷ window) | rolling 1 s |
| Correctness | validator (replay through reference orderbook) | cumulative |
| Crashes | crashloop detection on contestant pod | cumulative |
| Timeouts | bot-worker → result enum `TIMEOUT` | per event |

Score:
```
score = 1000 · (0.4·latency_norm + 0.3·tps_norm + 0.3·correctness)
       − crashes·10000 − timeouts·1000
       (floored at 0)
```

Higher P99 → lower latency_norm → lower score.
Lower TPS than 10 K target → lower tps_norm → lower score.
Fill mismatches vs reference orderbook → lower correctness.

The leaderboard ZSET is sorted by this score, top to bottom.

---

## 6. Debugging a failed submission

| Status | What it means | What to do |
|---|---|---|
| `failed` early in pipeline | tarball validation or build failed | check the LogViewer in the UI — the error line is in red |
| `ready` but score = 0 | engine ran but timed out 100% of bot orders | engine probably isn't listening on `RUNTIME_PORT` — check the `/health` handler |
| `ready` with low correctness | engine is responding but fills don't match reference | replay your bot order log locally against the engine and a known-good orderbook, diff fills |
| `ready` with high P99 | engine is too slow under contention | profile with `go tool pprof` or `perf`; the bot fleet sends Poisson-distributed bursts so check tail latency, not average |

---

## 7. Reference orderbook as a starting point

`samples/reference-orderbook/` is a complete, passing submission you can
copy and modify:

- `engine/orderbook.go` — heap-based price-time priority matching
- `main.go` — HTTP server with all four required handlers
- `engine/orderbook_test.go` — 8 unit tests for matching correctness

It's roughly 250 lines of Go. If yours is significantly bigger and slower,
you're probably overengineering.

---

## 8. Pre-submit checklist

- [ ] `tar -tzf my-engine.tar.gz` lists no `..` or absolute paths
- [ ] Archive is under 50 MB
- [ ] `samples/reference-orderbook` smoke-test passes locally
- [ ] Your `/health` endpoint returns 200 within 5 s of pod start
- [ ] You handle malformed JSON (don't panic — return 400)
- [ ] You handle `DELETE` on unknown id (return 404, don't crash)
- [ ] No background goroutines that leak between requests
- [ ] No `os.Exit(0)` on first error (crash → 10 K penalty)
- [ ] Memory under 512 MiB at 10 K orders queued (you'll be OOM-killed otherwise)
