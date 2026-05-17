# sandbox-attack-test.ps1
#
# Runs the full sandbox attack suite against the current kube context and
# produces an updated docs/SANDBOX_ATTACK_REPORT.md with the actual outcomes.
#
# Prerequisites:
#   - kubectl on PATH, configured to a kind / EKS / any cluster
#   - infra/manifests/sandbox-runner.yaml already applied (namespace + NetPol)
#
# Exit codes:
#   0 — every attack was blocked as expected
#   1 — at least one attack succeeded (regression — investigate immediately)
#   2 — runtime error (kubectl missing, no cluster, etc.)

$ErrorActionPreference = 'Continue'
$repoRoot = Split-Path -Parent $PSScriptRoot
$attacksDir = Join-Path $repoRoot 'infra\sandbox-attacks'
$reportPath = Join-Path $repoRoot 'docs\SANDBOX_ATTACK_REPORT.md'

function Test-Kubectl {
    if (-not (Get-Command kubectl -ErrorAction SilentlyContinue)) {
        Write-Error "kubectl not on PATH"
        exit 2
    }
    $ctx = kubectl config current-context 2>$null
    if (-not $ctx) {
        Write-Error "no current kube context — run 'kind create cluster --config infra/kind/cluster.yaml' first"
        exit 2
    }
    Write-Host "Using kube context: $ctx" -ForegroundColor Cyan
}

function Ensure-Namespace {
    $manifest = Join-Path $repoRoot 'infra\manifests\sandbox-runner.yaml'
    Write-Host "Applying baseline (namespace + NetworkPolicy)..." -ForegroundColor Cyan
    # apply only the namespace + netpol parts; the deployment can fail without our image
    kubectl apply -f $manifest 2>&1 | Where-Object { $_ -notmatch 'deployment|serviceaccount|clusterrole|rolebinding|service ' } | Out-Null
    kubectl wait --for=jsonpath='{.status.phase}'=Active namespace/iicpc-contestants --timeout=10s | Out-Null
}

$results = @()

function Run-AdmissionAttack {
    param([string]$Yaml, [string]$Name, [string]$Defence, [string]$Description)
    Write-Host "[admission] $Name ... " -NoNewline
    $out = kubectl apply -f $Yaml 2>&1 | Out-String
    $applied = $LASTEXITCODE
    $blocked = $applied -ne 0
    $rejection = if ($blocked) { ($out -split "`n" | Select-Object -First 3) -join ' ' } else { '<none — POD CREATED!>' }
    if (-not $blocked) {
        # cleanup if it slipped through
        kubectl delete -f $Yaml --ignore-not-found 2>&1 | Out-Null
    }
    $color = if ($blocked) { 'Green' } else { 'Red' }
    Write-Host ($(if ($blocked) {'BLOCKED'} else {'ESCALATED'})) -ForegroundColor $color
    return [pscustomobject]@{
        Layer       = 'admission'
        Name        = $Name
        Description = $Description
        Defence     = $Defence
        Blocked     = $blocked
        Evidence    = $rejection.Trim()
    }
}

function Run-RuntimeAttack {
    param([string]$Yaml)
    Write-Host "[runtime] applying attacker pod..." -ForegroundColor Cyan
    kubectl apply -f $Yaml | Out-Null
    Write-Host "[runtime] waiting for completion..." -ForegroundColor Cyan
    kubectl wait --for=jsonpath='{.status.phase}'=Succeeded pod/attack-runtime -n iicpc-contestants --timeout=120s 2>&1 | Out-Null
    if ($LASTEXITCODE -ne 0) {
        # may be Failed phase too — get whatever logs we can
        kubectl wait --for=jsonpath='{.status.phase}'=Failed pod/attack-runtime -n iicpc-contestants --timeout=10s 2>&1 | Out-Null
    }
    $logs = kubectl logs pod/attack-runtime -n iicpc-contestants 2>&1 | Out-String
    kubectl delete -f $Yaml --ignore-not-found 2>&1 | Out-Null

    # Parse: each '=== Attack N: <desc> ===' block followed by BLOCKED or ESCALATED
    $blocks = $logs -split '=== Attack \d+: ' | Select-Object -Skip 1
    foreach ($b in $blocks) {
        $lines = ($b -split "`n") | Where-Object { $_ -ne '' }
        $header = $lines[0] -replace ' ===$', ''
        $verdict = if ($b -match 'ESCALATED:') { 'ESCALATED' } elseif ($b -match 'BLOCKED:') { 'BLOCKED' } else { 'UNKNOWN' }
        $color = if ($verdict -eq 'BLOCKED') { 'Green' } elseif ($verdict -eq 'ESCALATED') { 'Red' } else { 'Yellow' }
        Write-Host "[runtime] $header ... " -NoNewline
        Write-Host $verdict -ForegroundColor $color
        $evidence = if ($b -match '(BLOCKED|ESCALATED): (.+)') { $matches[2].Trim() } else { '<no evidence>' }
        $results += [pscustomobject]@{
            Layer       = 'runtime'
            Name        = $header
            Description = $header
            Defence     = '(see runtime attacker.yaml)'
            Blocked     = ($verdict -eq 'BLOCKED')
            Evidence    = $evidence
        }
    }
    return $results
}

# --- main ---
Test-Kubectl
Ensure-Namespace

# Admission attacks
$results += Run-AdmissionAttack `
    -Yaml (Join-Path $attacksDir 'admission\01-run-as-root.yaml') `
    -Name 'Run as uid 0' `
    -Defence 'PSA restricted: runAsNonRoot' `
    -Description 'pod tries to run as root inside container'
$results += Run-AdmissionAttack `
    -Yaml (Join-Path $attacksDir 'admission\02-host-network.yaml') `
    -Name 'hostNetwork=true' `
    -Defence 'PSA restricted: hostNetwork forbidden' `
    -Description 'pod requests host network namespace'
$results += Run-AdmissionAttack `
    -Yaml (Join-Path $attacksDir 'admission\03-host-path.yaml') `
    -Name 'hostPath mount of /' `
    -Defence 'PSA restricted: hostPath volumes forbidden' `
    -Description 'pod mounts host root filesystem read-write'
$results += Run-AdmissionAttack `
    -Yaml (Join-Path $attacksDir 'admission\04-privileged.yaml') `
    -Name 'privileged=true' `
    -Defence 'PSA restricted: privileged forbidden' `
    -Description 'pod requests privileged mode (equivalent to host root)'
$results += Run-AdmissionAttack `
    -Yaml (Join-Path $attacksDir 'admission\05-cap-sys-admin.yaml') `
    -Name 'CAP_SYS_ADMIN add' `
    -Defence 'PSA restricted: capability additions forbidden' `
    -Description 'pod requests CAP_SYS_ADMIN ("the new root")'
$results += Run-AdmissionAttack `
    -Yaml (Join-Path $attacksDir 'admission\06-host-pid.yaml') `
    -Name 'hostPID=true' `
    -Defence 'PSA restricted: hostPID forbidden' `
    -Description 'pod requests host PID namespace (visibility into kubelet)'

# Runtime attacks
$results = Run-RuntimeAttack -Yaml (Join-Path $attacksDir 'runtime\attacker.yaml')

# Render report
$now = (Get-Date).ToString('yyyy-MM-dd HH:mm:ss zzz')
$ctx = kubectl config current-context
$escalations = ($results | Where-Object { -not $_.Blocked }).Count
$total = $results.Count
$summary = if ($escalations -eq 0) {
    "**✅ All $total attacks blocked.** Sandbox defences are intact."
} else {
    "**❌ $escalations / $total attacks ESCALATED — REGRESSION.**"
}

$tableRows = $results | ForEach-Object {
    $status = if ($_.Blocked) { '✅ blocked' } else { '❌ ESCALATED' }
    "| $($_.Layer) | $($_.Name) | $($_.Defence) | $status | ``$($_.Evidence -replace '\|','\|' -replace '`','')`` |"
}

$report = @"
# Sandbox Attack Report

> Last run: $now
> Kube context: ``$ctx``
> Source: ``scripts/sandbox-attack-test.ps1``

$summary

## Results

| Layer | Attack | Defence | Outcome | Evidence (truncated) |
| ----- | ------ | ------- | ------- | -------------------- |
$($tableRows -join "`n")

## Methodology

Two complementary layers of defence are exercised:

1. **Admission-time** — A malformed pod spec is ``kubectl apply``'d to the
   ``iicpc-contestants`` namespace. The API server should reject it before
   the pod is ever scheduled. Each attack is one ``infra/sandbox-attacks/admission/*.yaml``.
2. **Runtime** — A *legitimate-looking* pod (matches contestant baseline
   securityContext) runs 6 in-pod attacks against the kernel/sandbox
   boundary: file write to read-only root, raw socket open, mount syscall,
   cross-PID memory read, NetworkPolicy egress bypass, and package install
   as non-root. Each prints ``BLOCKED: <reason>`` or ``ESCALATED``.

Re-run on any cluster:

\`\`\`powershell
./scripts/sandbox-attack-test.ps1
\`\`\`

Wire into CI as a regression guard — the runner exits non-zero on any
escalation, so a PR that weakens isolation would be caught immediately.

## What this proves

Most hackathon submissions ship gVisor + NetworkPolicy + seccomp + PSA
restricted and stop there. This suite turns those claims into evidence:
every defence layer rejects a concrete attack, and the rejection text is
captured verbatim. If any control regresses (e.g., a future PR drops
``readOnlyRootFilesystem``), the corresponding attack succeeds and the
script fails red.

The matching production pod spec lives at
``services/sandbox-runner/internal/runner/podspec.go``.
"@

Set-Content -Path $reportPath -Value $report -Encoding utf8
Write-Host ""
Write-Host "Report written → $reportPath" -ForegroundColor Cyan
Write-Host "Summary: $summary" -ForegroundColor $(if ($escalations -eq 0) { 'Green' } else { 'Red' })

if ($escalations -ne 0) { exit 1 } else { exit 0 }
