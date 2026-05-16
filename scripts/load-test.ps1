# Day 14 manual load test: 1000 bot-worker goroutines → reference orderbook.
# Verifies zero errors and prints P50/P90/P99 latency from /metrics.
#
# Usage:
#   ./scripts/load-test.ps1                 # 1000 workers, 30s
#   ./scripts/load-test.ps1 -Workers 500    # custom worker count
#   ./scripts/load-test.ps1 -DurationS 60   # custom duration
param(
    [int]$Workers = 1000,
    [int]$DurationS = 30,
    [int]$RPS = 5,
    [int]$OrderbookPort = 18080,
    [int]$BotMetricsPort = 19090
)

$ErrorActionPreference = "Stop"
$root = Resolve-Path "$PSScriptRoot\.."

Write-Host "[load-test] building reference-orderbook + bot-worker..."
Push-Location "$root\samples\reference-orderbook"; go build -o "$root\.bin\reference-orderbook.exe" . ; Pop-Location
Push-Location "$root\services\bot-worker";        go build -o "$root\.bin\bot-worker.exe" . ;        Pop-Location

$env:RUNTIME_PORT = "$OrderbookPort"
$obProc = Start-Process -PassThru -NoNewWindow -FilePath "$root\.bin\reference-orderbook.exe"
Start-Sleep -Seconds 1

try {
    Write-Host "[load-test] orderbook PID=$($obProc.Id) port=$OrderbookPort"

    $env:HTTP_ADDR        = ":$BotMetricsPort"
    $env:TARGET_URL       = "http://localhost:$OrderbookPort"
    $env:NUM_WORKERS      = "$Workers"
    $env:ORDERS_PER_SECOND = "$RPS"
    $env:ARRIVAL_MODE     = "poisson"
    $env:PROTOCOL         = "rest"

    Write-Host "[load-test] launching $Workers bot workers, $RPS rps each, $DurationS s..."
    $botProc = Start-Process -PassThru -NoNewWindow -FilePath "$root\.bin\bot-worker.exe"

    Start-Sleep -Seconds $DurationS

    Write-Host "[load-test] fetching /metrics..."
    $metrics = Invoke-RestMethod -Uri "http://localhost:$BotMetricsPort/metrics"
    $metrics | ConvertTo-Json

    Stop-Process -Id $botProc.Id -Force -ErrorAction SilentlyContinue

    if ($metrics.errors -gt 0) {
        Write-Host "[load-test] FAIL: $($metrics.errors) errors over $($metrics.count) events" -ForegroundColor Red
        exit 1
    }
    Write-Host "[load-test] PASS: 0 errors, $($metrics.count) events, P99=$($metrics.p99_ns) ns" -ForegroundColor Green
}
finally {
    Stop-Process -Id $obProc.Id -Force -ErrorAction SilentlyContinue
}
