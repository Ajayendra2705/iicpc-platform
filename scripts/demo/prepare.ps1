# Demo pre-flight. Starts leaderboard-svc in SEED_DEMO mode + Next.js dev
# server, waits for both to be reachable, then opens the browser.
# Run BEFORE you hit Record.
#
# Usage:
#   ./scripts/demo/prepare.ps1
#   ./scripts/demo/prepare.ps1 -NoBrowser
#
# Cleanup: ./scripts/demo/teardown.ps1
param(
    [int]$LeaderboardPort = 8086,
    [int]$WebPort         = 3000,
    [int]$ReadyTimeoutS   = 60,
    [switch]$NoBrowser
)

$ErrorActionPreference = "Stop"
$root = Resolve-Path "$PSScriptRoot\..\.."
$pidDir = Join-Path $root ".bin"
if (-not (Test-Path $pidDir)) { New-Item -ItemType Directory -Path $pidDir | Out-Null }
$leaderboardPidFile = Join-Path $pidDir "leaderboard.pid"
$webPidFile         = Join-Path $pidDir "web.pid"
$leaderboardLog     = Join-Path $pidDir "leaderboard.log"
$webLog             = Join-Path $pidDir "web.log"

function Wait-Healthy {
    param([string]$Url, [int]$TimeoutS, [string]$Label)
    $deadline = (Get-Date).AddSeconds($TimeoutS)
    while ((Get-Date) -lt $deadline) {
        try {
            $r = Invoke-WebRequest -Uri $Url -TimeoutSec 2 -UseBasicParsing -ErrorAction Stop
            if ($r.StatusCode -lt 500) {
                Write-Host "[demo:prepare] $Label ready" -ForegroundColor Green
                return $true
            }
        } catch { Start-Sleep -Milliseconds 500 }
    }
    Write-Host "[demo:prepare] $Label did NOT come up within ${TimeoutS}s" -ForegroundColor Red
    return $false
}

# Stop any previous run
if ((Test-Path $leaderboardPidFile) -or (Test-Path $webPidFile)) {
    & "$PSScriptRoot\teardown.ps1" -Quiet
    Start-Sleep -Seconds 1
}

# 1. Leaderboard backend
Write-Host "[demo:prepare] starting leaderboard-svc (SEED_DEMO=true) ..."
$lbScript = Join-Path $PSScriptRoot "launch-leaderboard.ps1"
# Quote any path that may contain spaces; Start-Process otherwise splits on them.
$lbCmd = "-NoProfile -ExecutionPolicy Bypass -File `"$lbScript`" -Port $LeaderboardPort -LogPath `"$leaderboardLog`""
$lp = Start-Process -PassThru -WindowStyle Minimized -FilePath "powershell.exe" -ArgumentList $lbCmd
$lp.Id | Out-File $leaderboardPidFile -Encoding ascii

if (-not (Wait-Healthy -Url "http://127.0.0.1:$LeaderboardPort/healthz" -TimeoutS $ReadyTimeoutS -Label "leaderboard-svc")) {
    Write-Host "[demo:prepare] aborted - tail $leaderboardLog for details" -ForegroundColor Red
    exit 1
}

# 2. Next.js
Write-Host "[demo:prepare] starting Next.js dev server ..."
$wbScript = Join-Path $PSScriptRoot "launch-web.ps1"
$wbCmd = "-NoProfile -ExecutionPolicy Bypass -File `"$wbScript`" -Port $WebPort -LeaderboardPort $LeaderboardPort -LogPath `"$webLog`""
$wp = Start-Process -PassThru -WindowStyle Minimized -FilePath "powershell.exe" -ArgumentList $wbCmd
$wp.Id | Out-File $webPidFile -Encoding ascii

if (-not (Wait-Healthy -Url "http://127.0.0.1:$WebPort" -TimeoutS $ReadyTimeoutS -Label "web (Next.js)")) {
    Write-Host "[demo:prepare] aborted - tail $webLog for details" -ForegroundColor Red
    & "$PSScriptRoot\teardown.ps1" -Quiet
    exit 1
}

Write-Host ""
Write-Host "==============================================" -ForegroundColor Cyan
Write-Host " DEMO READY"                                    -ForegroundColor Cyan
Write-Host "   leaderboard : http://127.0.0.1:$LeaderboardPort/leaderboard"
Write-Host "   web         : http://127.0.0.1:$WebPort"
Write-Host "   logs        : $leaderboardLog"
Write-Host "                 $webLog"
Write-Host ""
Write-Host " teardown    : ./scripts/demo/teardown.ps1"
Write-Host "==============================================" -ForegroundColor Cyan

if (-not $NoBrowser) {
    Start-Sleep -Seconds 2
    Start-Process "http://127.0.0.1:$WebPort"
}
