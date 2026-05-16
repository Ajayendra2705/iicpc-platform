# Chaos #3 — inject network latency into contestant pods via Pumba.
# Watches P99 latency climb on the contestant detail page, then recovers.
#
# Requires:
#   - Pumba-capable nodes (not gVisor — see docs/CHAOS.md "Limitations")
#   - infra/manifests/chaos/pumba-network-delay.yaml applied or templated
#
# Usage:
#   ./inject-latency.ps1 -DelayMs 100 -JitterMs 20 -DurationS 30
param(
    [string]$Namespace = "iicpc-contestants",
    [int]$DelayMs      = 100,
    [int]$JitterMs     = 20,
    [int]$DurationS    = 30,
    [string]$Selector  = "app=contestant"
)

$ErrorActionPreference = "Stop"
$root = Resolve-Path "$PSScriptRoot\..\.."

Write-Host "[chaos:latency] target ns=$Namespace selector=$Selector delay=${DelayMs}ms±${JitterMs}ms duration=${DurationS}s"

$jobName = "pumba-netem-$([int](Get-Random -Maximum 99999))"

$manifest = @"
apiVersion: batch/v1
kind: Job
metadata:
  name: $jobName
  namespace: $Namespace
spec:
  ttlSecondsAfterFinished: 60
  backoffLimit: 0
  template:
    spec:
      restartPolicy: Never
      hostPID: true
      hostNetwork: false
      containers:
        - name: pumba
          image: gaiaadm/pumba:0.10.2
          securityContext:
            capabilities:
              add: ["NET_ADMIN"]
          args:
            - "--log-level=info"
            - "netem"
            - "--duration=${DurationS}s"
            - "--tc-image=gaiadocker/iproute2:latest"
            - "delay"
            - "--time=${DelayMs}"
            - "--jitter=${JitterMs}"
            - "re2:^${Selector -replace '.*=',''}"
"@

$manifest | kubectl apply -f - | Out-Null
Write-Host "[chaos:latency] Pumba Job $jobName launched"
Write-Host "[chaos:latency] watch /contestant/<id> — P99 bar should climb to ${DelayMs}+ms"

Start-Sleep -Seconds ($DurationS + 5)

Write-Host "[chaos:latency] cleanup..."
kubectl delete job -n $Namespace $jobName --ignore-not-found | Out-Null
Write-Host "[chaos:latency] DONE — latency should be back to baseline"
