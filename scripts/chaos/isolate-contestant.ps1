# Chaos #2 — apply a deny-all NetworkPolicy to one contestant pod,
# observe the score drop, then remove the policy.
#
# Requires a CNI that enforces NetworkPolicy (Calico, Cilium, EKS).
#
# Usage:
#   ./isolate-contestant.ps1 -ContestantID team-alpha -DurationS 60
param(
    [Parameter(Mandatory=$true)]
    [string]$ContestantID,
    [string]$Namespace = "iicpc-contestants",
    [int]$DurationS    = 60
)

$ErrorActionPreference = "Stop"

$policyName = "chaos-isolate-$ContestantID"
$labelKey   = "chaos.iicpc.io/isolated"
$labelVal   = $ContestantID

Write-Host "[chaos:isolate] target contestant=$ContestantID ns=$Namespace duration=${DurationS}s"

$pod = (kubectl get pods -n $Namespace -l "contestant-id=$ContestantID" -o jsonpath='{.items[0].metadata.name}' 2>$null)
if (-not $pod) {
    Write-Error "[chaos:isolate] no pod found with contestant-id=$ContestantID in $Namespace"
    exit 1
}
Write-Host "[chaos:isolate] selected pod: $pod"

# Label the pod so the NetworkPolicy matches it
kubectl label pod -n $Namespace $pod "${labelKey}=${labelVal}" --overwrite | Out-Null

# Apply a deny-all NetworkPolicy scoped to that label
$manifest = @"
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: $policyName
  namespace: $Namespace
spec:
  podSelector:
    matchLabels:
      ${labelKey}: ${labelVal}
  policyTypes: [Ingress, Egress]
"@

$manifest | kubectl apply -f - | Out-Null
Write-Host "[chaos:isolate] policy applied — score should drop within ~10s"
Write-Host "[chaos:isolate] watch the leaderboard or /contestant/$ContestantID detail page"

Start-Sleep -Seconds $DurationS

Write-Host "[chaos:isolate] removing policy + label, recovery should follow within 5s"
kubectl delete networkpolicy -n $Namespace $policyName --ignore-not-found | Out-Null
kubectl label pod -n $Namespace $pod "${labelKey}-" 2>$null | Out-Null

Write-Host "[chaos:isolate] DONE"
