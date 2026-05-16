# Runs all three chaos scenarios back-to-back. Each is given enough time
# to show its observable signal on the leaderboard UI.
#
# Useful for the demo video: one terminal, one command, three distinct
# UI events the camera can capture.
param(
    [string]$ContestantID = "team-alpha",
    [int]$PauseS          = 15
)

$ErrorActionPreference = "Stop"
$here = $PSScriptRoot

Write-Host "=== chaos suite ===" -ForegroundColor Cyan
Write-Host ""

Write-Host "[1/3] scenario: kill-bot-pod" -ForegroundColor Yellow
& "$here\kill-bot-pod.ps1"
Write-Host ""
Write-Host "pausing ${PauseS}s — observe TPS recovery on UI"
Start-Sleep -Seconds $PauseS

Write-Host "[2/3] scenario: isolate-contestant ($ContestantID)" -ForegroundColor Yellow
& "$here\isolate-contestant.ps1" -ContestantID $ContestantID -DurationS 30
Write-Host ""
Write-Host "pausing ${PauseS}s — observe score recovery"
Start-Sleep -Seconds $PauseS

Write-Host "[3/3] scenario: inject-latency" -ForegroundColor Yellow
& "$here\inject-latency.ps1" -DurationS 20

Write-Host ""
Write-Host "=== chaos suite complete ===" -ForegroundColor Cyan
