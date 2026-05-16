# End-to-end smoke test for a deployed cluster (EKS staging or local kind).
# Submits a benchmark, waits for it to run, asserts the leaderboard updated.
#
# Run AFTER `helm upgrade --install iicpc ...` and at least one contestant
# image is registered. Uses kubectl port-forward so it works against any
# cluster you can reach with kubectl.
param(
    [string]$Namespace      = "iicpc",
    [string]$ContestantID   = "smoke-contestant",
    [string]$TargetURL      = "http://reference-orderbook:8082",
    [int]$NumWorkers        = 100,
    [int]$OrdersPerSecond   = 50,
    [int]$DurationS         = 60
)

$ErrorActionPreference = "Stop"

$benchmarkID = "smoke-$(Get-Date -Format 'yyyyMMddHHmmss')"
$coordPort   = 18083
$leadPort    = 18086

Write-Host "[smoke] benchmark_id=$benchmarkID contestant=$ContestantID workers=$NumWorkers ops=$OrdersPerSecond dur=${DurationS}s"

# Start port-forwards in the background
$pfCoord = Start-Process -PassThru -NoNewWindow kubectl @("port-forward", "-n", $Namespace, "svc/bot-coordinator", "${coordPort}:8083")
$pfLead  = Start-Process -PassThru -NoNewWindow kubectl @("port-forward", "-n", $Namespace, "svc/leaderboard-svc", "${leadPort}:8086")
Start-Sleep -Seconds 3

try {
    # 1. Submit
    $body = @{
        benchmark_id      = $benchmarkID
        target_url        = $TargetURL
        num_workers       = $NumWorkers
        orders_per_second = $OrdersPerSecond
        protocol          = "rest"
        arrival_mode      = "poisson"
        duration_seconds  = $DurationS
        submission_id     = $ContestantID
    } | ConvertTo-Json
    Write-Host "[smoke] POST /benchmarks ..."
    $resp = Invoke-RestMethod -Uri "http://127.0.0.1:${coordPort}/benchmarks" -Method POST -ContentType "application/json" -Body $body
    Write-Host "[smoke] accepted: $($resp | ConvertTo-Json -Compress)"

    # 2. Wait
    Write-Host "[smoke] running for ${DurationS}s + 10s settle"
    Start-Sleep -Seconds ($DurationS + 10)

    # 3. Leaderboard check
    Write-Host "[smoke] GET /leaderboard ..."
    $top = Invoke-RestMethod -Uri "http://127.0.0.1:${leadPort}/leaderboard?top=20"
    Write-Host "[smoke] top contestants:"
    $top | Format-Table

    $match = $top | Where-Object { $_.contestant_id -eq $ContestantID }
    if ($match) {
        Write-Host "[smoke] PASS — contestant $ContestantID present with score $($match.score)" -ForegroundColor Green
        exit 0
    } else {
        Write-Host "[smoke] WARN — contestant $ContestantID not yet on leaderboard (may need longer settle)" -ForegroundColor Yellow
        exit 2
    }
} finally {
    Stop-Process -Id $pfCoord.Id -Force -ErrorAction SilentlyContinue
    Stop-Process -Id $pfLead.Id  -Force -ErrorAction SilentlyContinue
}
