# e2e-pipeline.ps1
#
# Runs the REAL benchmarking pipeline end-to-end on the local machine — no
# SEED_DEMO, no mocks in the scoring path:
#
#   bot-worker --REST--> reference-orderbook
#       |gRPC OrderEvents
#       v
#   telemetry-ingester --Kafka(Redpanda)--> aggregator (latency/TPS) + validator (correctness)
#                                               |                          |
#                                               +----- leaderboard-svc -----+  --> composite score
#
# Proves the full "Distributed Load Testing -> Real-Time Scoring" half of the
# pipeline with real telemetry, and exercises the live scoring chain (real
# fills/side/timeouts feeding correctness + stability). Evidence is captured to
# docs/artifacts/e2e-pipeline/.
#
# By default the only external dependency is Redpanda (Kafka API) via
# docker-compose; the aggregator uses an in-memory writer and leaderboard an
# in-memory store, so no Postgres/Redis needed for the lightweight proof.
#
# Pass -RealStores to additionally bring up real Redis + TimescaleDB and route
# the leaderboard ZSET through Redis and the aggregator snapshots through
# TimescaleDB (schema auto-applied). This proves the production data-store
# paths end-to-end, not just the in-memory ones. All local, no cloud cost.
param(
    [int]$DurationS    = 30,
    [int]$Workers      = 25,
    [int]$RPS          = 20,
    [switch]$RealStores
)

$ErrorActionPreference = "Stop"
$root = Resolve-Path "$PSScriptRoot\.."
$bin  = Join-Path $root ".bin\e2e"
$ev   = Join-Path $root "docs\artifacts\e2e-pipeline"
New-Item -ItemType Directory -Force -Path $bin | Out-Null
New-Item -ItemType Directory -Force -Path $ev  | Out-Null

$procs = @()
function Start-Svc {
    param([string]$Name, [string]$Exe, [hashtable]$Env)
    foreach ($k in $Env.Keys) { Set-Item "env:$k" $Env[$k] }
    $log = Join-Path $bin "$Name.log"
    $p = Start-Process -PassThru -WindowStyle Hidden -FilePath $Exe `
        -RedirectStandardOutput $log -RedirectStandardError "$log.err"
    $script:procs += [pscustomobject]@{ Name = $Name; Proc = $p }
    Write-Host ("[e2e] started {0} (pid {1})" -f $Name, $p.Id)
}
function Wait-Http {
    param([string]$Url, [int]$TimeoutS = 30, [string]$Label)
    $deadline = (Get-Date).AddSeconds($TimeoutS)
    while ((Get-Date) -lt $deadline) {
        try { Invoke-WebRequest -Uri $Url -TimeoutSec 2 -UseBasicParsing -ErrorAction Stop | Out-Null; Write-Host "[e2e] $Label ready"; return } catch { Start-Sleep -Milliseconds 400 }
    }
    throw "$Label did not come up at $Url within ${TimeoutS}s"
}
function Stop-All {
    Write-Host "[e2e] tearing down services..."
    foreach ($s in $script:procs) { try { Stop-Process -Id $s.Proc.Id -Force -ErrorAction SilentlyContinue } catch {} }
}
# Docker writes progress to stderr; under $ErrorActionPreference='Stop' that
# stderr is wrapped as a terminating NativeCommandError (PS 5.1). Run the docker
# invocation (passed as a scriptblock, so its -flags aren't parsed by PowerShell)
# with stderr demoted to normal output, and fail only on a real non-zero exit.
function Invoke-Docker {
    param([Parameter(Mandatory = $true)][scriptblock] $Cmd)
    $ErrorActionPreference = 'Continue'  # function-scoped; reverts on return
    $out = & $Cmd 2>&1
    if ($LASTEXITCODE -ne 0) { throw "docker command failed ($LASTEXITCODE): $out" }
    return $out
}

try {
    # 1. Infra: Redpanda always; Redis + TimescaleDB when -RealStores.
    if ($RealStores) {
        Write-Host "[e2e] starting Redpanda + Redis + TimescaleDB..."
        Invoke-Docker { docker compose up -d redpanda redis postgres } | Out-Null
        Start-Sleep -Seconds 6
        # Wait for Postgres, then apply the telemetry schema (idempotent).
        Write-Host "[e2e] waiting for TimescaleDB..."
        $pgDeadline = (Get-Date).AddSeconds(40)
        $pgReady = $false
        while ((Get-Date) -lt $pgDeadline) {
            $eap = $ErrorActionPreference; $ErrorActionPreference = 'Continue'
            & docker.exe compose exec -T postgres pg_isready -U iicpc -d iicpc 2>&1 | Out-Null
            $ok = ($LASTEXITCODE -eq 0); $ErrorActionPreference = $eap
            if ($ok) { $pgReady = $true; break }
            Start-Sleep -Milliseconds 600
        }
        if (-not $pgReady) { throw "TimescaleDB did not become ready" }
        Write-Host "[e2e] applying telemetry schema..."
        $sql = Join-Path $root "infra\timescaledb\migrations\001_telemetry_schema.sql"
        Invoke-Docker { docker compose cp $sql postgres:/tmp/schema.sql } | Out-Null
        Invoke-Docker { docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U iicpc -d iicpc -f /tmp/schema.sql } | Out-Null
    } else {
        Write-Host "[e2e] starting Redpanda..."
        Invoke-Docker { docker compose up -d redpanda } | Out-Null
        Start-Sleep -Seconds 6
    }

    # 2. Build the binaries.
    Write-Host "[e2e] building binaries..."
    $env:CGO_ENABLED = "0"
    & go build -o (Join-Path $bin "reference-orderbook.exe") "$root\samples\reference-orderbook"; if ($LASTEXITCODE) { throw "build reference-orderbook" }
    foreach ($svc in "telemetry-ingester","aggregator","validator","leaderboard-svc","bot-worker") {
        & go build -o (Join-Path $bin "$svc.exe") "$root\services\$svc"; if ($LASTEXITCODE) { throw "build $svc" }
    }

    # 3. Start services (order: sink -> bus consumers -> contestant).
    # Store wiring: in-memory by default, real Redis + TimescaleDB with -RealStores.
    $aggWriter = @{ WRITER_KIND = "stub" }
    $lbStore   = @{ STORE_KIND  = "stub" }
    if ($RealStores) {
        $aggWriter = @{ WRITER_KIND = "timescale"; POSTGRES_DSN = "postgres://iicpc:iicpc@localhost:5432/iicpc?sslmode=disable" }
        $lbStore   = @{ STORE_KIND  = "redis"; REDIS_ADDR = "localhost:6379"; REDIS_KEY = "leaderboard:scores" }
    }

    Start-Svc "reference-orderbook" (Join-Path $bin "reference-orderbook.exe") @{ RUNTIME_PORT = "9100" }
    Start-Svc "telemetry-ingester" (Join-Path $bin "telemetry-ingester.exe") @{ GRPC_ADDR=":9091"; PRODUCER_KIND="kafka"; KAFKA_BROKERS="localhost:9092"; KAFKA_TOPIC="telemetry-events" }
    Start-Svc "aggregator" (Join-Path $bin "aggregator.exe") (@{ HTTP_ADDR=":8084"; CONSUMER_KIND="kafka"; KAFKA_BROKERS="localhost:9092"; KAFKA_TOPIC="telemetry-events"; KAFKA_GROUP_ID="aggregator"; WINDOW_MS="1000" } + $aggWriter)
    Start-Svc "validator" (Join-Path $bin "validator.exe") @{ HTTP_ADDR=":8085"; CONSUMER_KIND="kafka"; KAFKA_BROKERS="localhost:9092"; KAFKA_TOPIC="telemetry-events"; KAFKA_GROUP_ID="validator" }
    Start-Svc "leaderboard-svc" (Join-Path $bin "leaderboard-svc.exe") (@{ HTTP_ADDR=":8086"; AGGREGATOR_URL="http://127.0.0.1:8084"; VALIDATOR_URL="http://127.0.0.1:8085"; TICK_MS="1000" } + $lbStore)

    Wait-Http "http://127.0.0.1:9100/health" 20 "reference-orderbook"
    Wait-Http "http://127.0.0.1:8084/healthz" 20 "aggregator"
    Wait-Http "http://127.0.0.1:8085/healthz" 20 "validator"
    Wait-Http "http://127.0.0.1:8086/healthz" 20 "leaderboard-svc"
    Start-Sleep -Seconds 2  # let kafka consumers join their groups

    # 4. Run the bot fleet against the contestant for DurationS.
    Write-Host "[e2e] running bot fleet: $Workers workers x $RPS rps for ${DurationS}s against team-live..."
    Start-Svc "bot-worker" (Join-Path $bin "bot-worker.exe") @{ HTTP_ADDR=":9090"; TARGET_URL="http://127.0.0.1:9100"; NUM_WORKERS="$Workers"; ORDERS_PER_SECOND="$RPS"; PROTOCOL="rest"; ARRIVAL_MODE="poisson"; TELEMETRY_ADDR="127.0.0.1:9091"; CONTESTANT_ID="team-live" }
    Start-Sleep -Seconds $DurationS
    # capture the bot's own stats while it is still alive, then stop it.
    try { (Invoke-RestMethod "http://127.0.0.1:9090/metrics" | ConvertTo-Json -Depth 6) | Out-File (Join-Path $ev "bot-worker-metrics.json") -Encoding utf8 } catch {}
    ($script:procs | Where-Object { $_.Name -eq "bot-worker" }).Proc | ForEach-Object { Stop-Process -Id $_.Id -Force -ErrorAction SilentlyContinue }
    Write-Host "[e2e] bot stopped; settling 5s..."
    Start-Sleep -Seconds 5

    # 5. Capture evidence from the REAL scoring path.
    Write-Host "[e2e] capturing evidence -> $ev"
    (Invoke-RestMethod "http://127.0.0.1:8084/metrics"     | ConvertTo-Json -Depth 6) | Out-File (Join-Path $ev "aggregator-metrics.json") -Encoding utf8
    (Invoke-RestMethod "http://127.0.0.1:8085/validate"    | ConvertTo-Json -Depth 6) | Out-File (Join-Path $ev "validator-reports.json") -Encoding utf8
    (Invoke-RestMethod "http://127.0.0.1:8086/leaderboard" | ConvertTo-Json -Depth 6) | Out-File (Join-Path $ev "leaderboard.json") -Encoding utf8

    Write-Host ""
    Write-Host "=== LEADERBOARD (real pipeline) ===" -ForegroundColor Cyan
    Invoke-RestMethod "http://127.0.0.1:8086/leaderboard" | Format-Table -AutoSize
    Write-Host "=== AGGREGATOR /metrics ===" -ForegroundColor Cyan
    Invoke-RestMethod "http://127.0.0.1:8084/metrics" | Format-Table contestant_id, count, tps, p50_ns, p99_ns, rejected, timeouts -AutoSize
    Write-Host "=== VALIDATOR /validate ===" -ForegroundColor Cyan
    Invoke-RestMethod "http://127.0.0.1:8085/validate" | Format-Table -AutoSize

    if ($RealStores) {
        # Prove the real data-store paths: the Redis ZSET holds the live score,
        # and TimescaleDB holds the per-window snapshot rows.
        Write-Host "=== REDIS ZSET (leaderboard:scores) ===" -ForegroundColor Cyan
        $redisOut = Invoke-Docker { docker compose exec -T redis redis-cli ZREVRANGE leaderboard:scores 0 -1 WITHSCORES }
        $redisOut | Tee-Object (Join-Path $ev "redis-zset.txt") | Write-Host

        Write-Host "=== TIMESCALEDB telemetry_snapshots ===" -ForegroundColor Cyan
        $tsQuery = "SELECT contestant_id, count(*) AS windows, sum(count) AS total_orders, max(p99_ns) AS max_p99_ns FROM telemetry_snapshots GROUP BY contestant_id;"
        $tsOut = Invoke-Docker { docker compose exec -T postgres psql -U iicpc -d iicpc -c $tsQuery }
        $tsOut | Tee-Object (Join-Path $ev "timescale-snapshots.txt") | Write-Host
    }
}
finally {
    Stop-All
    Write-Host "[e2e] (Redpanda left running; 'docker compose down' to stop it)"
}
