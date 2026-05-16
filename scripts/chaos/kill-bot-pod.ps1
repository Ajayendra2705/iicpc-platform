# Chaos #1 — kill a random bot-worker pod and verify the Deployment heals.
#
# Usage:
#   ./kill-bot-pod.ps1                     # any pod
#   ./kill-bot-pod.ps1 -Namespace iicpc-bots
param(
    [string]$Namespace = "iicpc-bots",
    [string]$Selector  = "app=bot-worker",
    [int]$WaitS        = 30
)

$ErrorActionPreference = "Stop"

Write-Host "[chaos:kill-bot] target ns=$Namespace selector=$Selector"

$pods = (kubectl get pods -n $Namespace -l $Selector -o jsonpath='{.items[*].metadata.name}' 2>$null) -split ' '
$pods = $pods | Where-Object { $_ }
if (-not $pods) {
    Write-Error "[chaos:kill-bot] no bot pods found in $Namespace with $Selector"
    exit 1
}

$victim = $pods | Get-Random
Write-Host "[chaos:kill-bot] selected victim: $victim ($($pods.Count) pods available)"

$startCount = $pods.Count
kubectl delete pod -n $Namespace $victim --wait=false

Write-Host "[chaos:kill-bot] waiting up to ${WaitS}s for self-heal..."
$deadline = (Get-Date).AddSeconds($WaitS)
while ((Get-Date) -lt $deadline) {
    Start-Sleep -Seconds 2
    $running = (kubectl get pods -n $Namespace -l $Selector --field-selector status.phase=Running --no-headers 2>$null | Measure-Object -Line).Lines
    if ($running -ge $startCount) {
        $elapsed = [int]((Get-Date) - $deadline.AddSeconds(-$WaitS)).TotalSeconds
        Write-Host "[chaos:kill-bot] PASS — replacement Running after ${elapsed}s ($running pods)"
        exit 0
    }
    Write-Host "[chaos:kill-bot] ...still $running/$startCount Running"
}

Write-Host "[chaos:kill-bot] FAIL — Deployment did not self-heal within ${WaitS}s" -ForegroundColor Red
kubectl get pods -n $Namespace -l $Selector
exit 1
