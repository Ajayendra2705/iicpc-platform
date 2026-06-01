# kind-scale-demo.ps1
#
# Proves horizontal scale-out on a REAL multi-node Kubernetes cluster -- not a
# screenshot, not a gitignored artifact, but a single command anyone can run.
#
#   1. Brings up (or reuses) the 4-node kind cluster from infra/kind/cluster.yaml
#   2. Builds the real leaderboard-svc image (shared Dockerfile.service) and
#      loads it into every kind node
#   3. Deploys it as a Deployment with a topology spread across worker nodes
#   4. Scales 3 -> 9 replicas and captures the per-node pod distribution,
#      proving pods actually spread across multiple worker nodes
#
# A single representative service is used (not the full 8-service chart) so the
# demo is fast and dependency-light; it exercises the identical scheduler,
# taint/toleration, topology-spread and `kubectl scale` mechanics a cloud
# cluster uses. The full chart's scalability is separately lint/template-checked
# in CI (helm-lint) and the EKS path is in docs/EKS_STAGING_RUNBOOK.md.
#
# Evidence -> docs/artifacts/kind-multinode/. ~5-7 min, no cloud cost.
#
# Exit codes: 0 ok | 1 a step failed | 2 prerequisite missing.

param(
    [int]$Replicas = 9,        # scale-out target
    [switch]$Teardown          # delete the cluster at the end (default: keep)
)

$ErrorActionPreference = 'Continue'  # native kind/kubectl/docker write to stderr
$repoRoot = Split-Path -Parent $PSScriptRoot
$cluster  = 'iicpc'
$ns       = 'iicpc-scale-demo'
$image    = 'iicpc/leaderboard-svc:demo'
$ev       = Join-Path $repoRoot 'docs\artifacts\kind-multinode'

function Section { param([string]$m) Write-Host ('==> {0}' -f $m) -ForegroundColor Cyan }
function Pass    { param([string]$m) Write-Host ('    PASS: {0}' -f $m) -ForegroundColor Green }
function Fail    { param([string]$m) Write-Host ('    FAIL: {0}' -f $m) -ForegroundColor Red }

foreach ($tool in @('kind', 'kubectl', 'helm', 'docker')) {
    if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) {
        Write-Error ("{0} not on PATH" -f $tool); exit 2
    }
}
docker info --format '{{.ServerVersion}}' 2>&1 | Out-Null
if ($LASTEXITCODE -ne 0) { Write-Error 'Docker daemon not responding'; exit 2 }

New-Item -ItemType Directory -Force -Path $ev | Out-Null
$failed = $false

try {
    # ----------------------------------------------------------------------
    Section 'Stage 1: ensure 4-node kind cluster is up'
    $existing = (kind get clusters 2>&1) -split "`n"
    if ($existing -contains $cluster) {
        Pass 'reusing existing kind cluster'
    } else {
        kind create cluster --config (Join-Path $repoRoot 'infra\kind\cluster.yaml') --wait 120s
        if ($LASTEXITCODE -ne 0) { Fail 'kind create cluster'; throw 'cluster up' }
        Pass 'kind cluster created'
    }
    $nodes = kubectl get nodes -o wide 2>&1 | Out-String
    Write-Host $nodes
    $nodes | Out-File (Join-Path $ev 'nodes.txt') -Encoding utf8

    # ----------------------------------------------------------------------
    Section 'Stage 2: build leaderboard-svc image + load into kind'
    docker build -f (Join-Path $repoRoot 'infra\docker\Dockerfile.service') `
        --build-arg SERVICE=leaderboard-svc -t $image $repoRoot
    if ($LASTEXITCODE -ne 0) { Fail 'docker build'; throw 'build' }
    Pass 'image built'
    kind load docker-image $image --name $cluster
    if ($LASTEXITCODE -ne 0) { Fail 'kind load'; throw 'load' }
    Pass 'image loaded into all kind nodes'

    # ----------------------------------------------------------------------
    Section 'Stage 3: deploy leaderboard-svc (3 replicas, spread across workers)'
    $manifest = Join-Path $ev 'deploy.yaml'
    @"
apiVersion: v1
kind: Namespace
metadata:
  name: $ns
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: leaderboard-svc
  namespace: $ns
  labels: { app: leaderboard-svc }
spec:
  replicas: 3
  selector:
    matchLabels: { app: leaderboard-svc }
  template:
    metadata:
      labels: { app: leaderboard-svc }
    spec:
      # Spread evenly across worker nodes; soft so it always schedules.
      topologySpreadConstraints:
        - maxSkew: 1
          topologyKey: kubernetes.io/hostname
          whenUnsatisfiable: ScheduleAnyway
          labelSelector:
            matchLabels: { app: leaderboard-svc }
      containers:
        - name: leaderboard-svc
          image: $image
          imagePullPolicy: IfNotPresent
          env:
            - { name: HTTP_ADDR, value: ":8086" }
            - { name: STORE_KIND, value: "stub" }
          ports:
            - containerPort: 8086
          readinessProbe:
            httpGet: { path: /healthz, port: 8086 }
            initialDelaySeconds: 2
            periodSeconds: 3
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            runAsNonRoot: true
"@ | Out-File $manifest -Encoding utf8

    kubectl apply -f $manifest 2>&1 | Out-String | Write-Host
    kubectl -n $ns rollout status deploy/leaderboard-svc --timeout=120s
    if ($LASTEXITCODE -ne 0) { Fail 'initial rollout'; throw 'rollout' }
    $pods3 = kubectl -n $ns get pods -o wide 2>&1 | Out-String
    Write-Host $pods3
    $pods3 | Out-File (Join-Path $ev 'pods-3.txt') -Encoding utf8
    Pass '3 replicas Ready'

    # ----------------------------------------------------------------------
    Section ("Stage 4: scale 3 -> {0} replicas" -f $Replicas)
    kubectl -n $ns scale deploy/leaderboard-svc --replicas=$Replicas
    kubectl -n $ns rollout status deploy/leaderboard-svc --timeout=120s
    if ($LASTEXITCODE -ne 0) { Fail 'scale-out rollout'; throw 'scale' }
    $podsN = kubectl -n $ns get pods -o wide 2>&1 | Out-String
    Write-Host $podsN
    $podsN | Out-File (Join-Path $ev ("pods-{0}.txt" -f $Replicas)) -Encoding utf8

    # Per-node distribution: prove the replicas actually spread. Use a
    # space-separated jsonpath (no embedded newline escape, which PowerShell
    # mangles when passing to kubectl) and group on the host side.
    Section 'Stage 5: per-node distribution'
    $nodeNames = (kubectl -n $ns get pods -o jsonpath='{.items[*].spec.nodeName}' 2>&1) -split '\s+' |
        Where-Object { $_ }
    $dist = $nodeNames | Group-Object | Sort-Object Name |
        ForEach-Object { "{0}  {1} pods" -f $_.Name, $_.Count }
    $distText = ($dist -join "`n")
    Write-Host $distText
    $distText | Out-File (Join-Path $ev 'distribution.txt') -Encoding utf8
    $nodeCount = ($dist | Measure-Object).Count
    if ($nodeCount -lt 2) {
        Fail ("replicas landed on only {0} node -- expected spread across >= 2 workers" -f $nodeCount)
        $failed = $true
    } else {
        Pass ("{0} replicas spread across {1} worker nodes" -f $Replicas, $nodeCount)
    }
}
catch {
    Write-Host ('Scale demo aborted at: {0}' -f $_) -ForegroundColor Red
    $failed = $true
}
finally {
    if ($Teardown) {
        Section 'Teardown: delete kind cluster'
        kind delete cluster --name $cluster 2>&1 | Out-Null
        Pass 'cluster deleted'
    } else {
        Write-Host ("(cluster left running -- 'kind delete cluster --name {0}' to clean up)" -f $cluster) -ForegroundColor Yellow
    }
}

Write-Host ''
Write-Host '================================================================' -ForegroundColor Cyan
if (-not $failed) {
    Write-Host '  MULTI-NODE SCALE-OUT: PASSED' -ForegroundColor Green
    Write-Host ('  leaderboard-svc scaled to {0} replicas across worker nodes' -f $Replicas)
    Write-Host ('  Evidence in: {0}' -f $ev)
    exit 0
} else {
    Write-Host '  MULTI-NODE SCALE-OUT: FAILED' -ForegroundColor Red
    exit 1
}
