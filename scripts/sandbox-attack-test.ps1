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
#   0 - every attack was blocked as expected
#   1 - at least one attack succeeded (regression - investigate immediately)
#   2 - runtime error (kubectl missing, no cluster, etc.)

$ErrorActionPreference = 'Continue'
$repoRoot = Split-Path -Parent $PSScriptRoot
$attacksDir = Join-Path $repoRoot 'infra\sandbox-attacks'
$reportPath = Join-Path $repoRoot 'docs\SANDBOX_ATTACK_REPORT.md'
$BT = [char]96  # backtick char, used in markdown without confusing the parser

function Test-Kubectl {
    if (-not (Get-Command kubectl -ErrorAction SilentlyContinue)) {
        Write-Error 'kubectl not on PATH'
        exit 2
    }
    $ctx = kubectl config current-context 2>$null
    if (-not $ctx) {
        Write-Error 'no current kube context - bring up a cluster first'
        exit 2
    }
    Write-Host "Using kube context: $ctx" -ForegroundColor Cyan
}

function Enable-KindnetNetworkPolicy {
    # kindnet does NOT enforce NetworkPolicy by default. Without this flag,
    # the egress attack will ESCALATE (kindnet allows all egress). This step
    # is a no-op on non-kind clusters or if the flag is already set.
    $hasKindnet = kubectl get ds kindnet -n kube-system --ignore-not-found -o name 2>$null
    if (-not $hasKindnet) { return }
    $env = kubectl get ds kindnet -n kube-system -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="KINDNET_NETWORK_POLICY")].value}' 2>$null
    $needsRestart = $false
    if ($env -ne 'true') {
        Write-Host 'enabling kindnet NetworkPolicy enforcement (set KINDNET_NETWORK_POLICY=true)' -ForegroundColor Cyan
        kubectl set env ds/kindnet -n kube-system KINDNET_NETWORK_POLICY=true 2>&1 | Out-Null
        $needsRestart = $true
    } else {
        Write-Host 'kindnet NetworkPolicy enforcement already enabled' -ForegroundColor Cyan
    }
    # Always wait for rollout + give kindnet a few seconds to program iptables
    # rules from any existing NetworkPolicy in the cluster. On a brand-new
    # cluster, the rollout completes instantly but firewall programming lags
    # by a few seconds — without this delay the egress attack races kindnet.
    kubectl rollout status ds/kindnet -n kube-system --timeout=60s 2>&1 | Out-Null
    if ($needsRestart) {
        Write-Host 'waiting 10s for kindnet to program NetworkPolicy iptables rules...' -ForegroundColor Cyan
        Start-Sleep -Seconds 10
    }
}

$script:results = New-Object System.Collections.ArrayList

function Add-Result {
    param([string]$Layer, [string]$Name, [string]$Defence, [bool]$Blocked, [string]$Evidence)
    [void]$script:results.Add([pscustomobject]@{
        Layer    = $Layer
        Name     = $Name
        Defence  = $Defence
        Blocked  = $Blocked
        Evidence = $Evidence
    })
}

function Run-AdmissionAttack {
    param([string]$Yaml, [string]$Name, [string]$Defence)
    Write-Host ("[admission] {0} ... " -f $Name) -NoNewline
    $out = kubectl apply -f $Yaml 2>&1 | Out-String
    $applied = $LASTEXITCODE
    $blocked = $applied -ne 0
    if ($blocked) {
        $rejection = ($out -split "`n" | Where-Object { $_ -match 'forbidden|error|Error' } | Select-Object -First 1) -join ' '
        if (-not $rejection) { $rejection = ($out -split "`n" | Select-Object -First 1) }
    } else {
        $rejection = 'POD WAS CREATED (regression)'
        kubectl delete -f $Yaml --ignore-not-found 2>&1 | Out-Null
    }
    if ($blocked) {
        Write-Host 'BLOCKED' -ForegroundColor Green
    } else {
        Write-Host 'ESCALATED' -ForegroundColor Red
    }
    Add-Result -Layer 'admission' -Name $Name -Defence $Defence -Blocked $blocked -Evidence $rejection.Trim()
}

function Run-RuntimeAttack {
    param([string]$Yaml)
    Write-Host '[runtime] applying attacker pod...' -ForegroundColor Cyan
    kubectl apply -f $Yaml | Out-Null
    Write-Host '[runtime] waiting for completion (up to 120s)...' -ForegroundColor Cyan
    $deadline = (Get-Date).AddSeconds(120)
    $phase = ''
    while ((Get-Date) -lt $deadline) {
        $phase = kubectl get pod attack-runtime -n iicpc-contestants -o jsonpath='{.status.phase}' 2>$null
        if ($phase -in @('Succeeded', 'Failed')) { break }
        Start-Sleep -Seconds 2
    }
    Write-Host ("[runtime] final phase: {0}" -f $phase) -ForegroundColor Cyan
    $logs = kubectl logs pod/attack-runtime -n iicpc-contestants 2>&1 | Out-String
    kubectl delete -f $Yaml --ignore-not-found 2>&1 | Out-Null

    # Parse: each '=== Attack N: <desc> (defence) ===' line followed by
    # BLOCKED: or ESCALATED: line.
    $lines = $logs -split "`n"
    $currentHeader = $null
    foreach ($ln in $lines) {
        if ($ln -match '^=== Attack \d+: (.+?) \((.+?)\) ===') {
            $currentHeader = @{ Name = $matches[1].Trim(); Defence = $matches[2].Trim() }
        } elseif ($currentHeader -and ($ln -match '^(BLOCKED|ESCALATED): (.+)$')) {
            $verdict = $matches[1]
            $evidence = $matches[2].Trim()
            $blocked = ($verdict -eq 'BLOCKED')
            $color = if ($blocked) { 'Green' } else { 'Red' }
            Write-Host ("[runtime] {0} ... {1}" -f $currentHeader.Name, $verdict) -ForegroundColor $color
            Add-Result -Layer 'runtime' -Name $currentHeader.Name -Defence $currentHeader.Defence -Blocked $blocked -Evidence $evidence
            $currentHeader = $null
        }
    }
}

# --- main ---
Test-Kubectl
Enable-KindnetNetworkPolicy

Run-AdmissionAttack -Yaml (Join-Path $attacksDir 'admission\01-run-as-root.yaml') `
    -Name 'Run as uid 0' -Defence 'PSA restricted: runAsNonRoot'
Run-AdmissionAttack -Yaml (Join-Path $attacksDir 'admission\02-host-network.yaml') `
    -Name 'hostNetwork=true' -Defence 'PSA restricted: hostNetwork forbidden'
Run-AdmissionAttack -Yaml (Join-Path $attacksDir 'admission\03-host-path.yaml') `
    -Name 'hostPath mount of root' -Defence 'PSA restricted: hostPath volumes forbidden'
Run-AdmissionAttack -Yaml (Join-Path $attacksDir 'admission\04-privileged.yaml') `
    -Name 'privileged=true' -Defence 'PSA restricted: privileged forbidden'
Run-AdmissionAttack -Yaml (Join-Path $attacksDir 'admission\05-cap-sys-admin.yaml') `
    -Name 'CAP_SYS_ADMIN add' -Defence 'PSA restricted: capability additions forbidden'
Run-AdmissionAttack -Yaml (Join-Path $attacksDir 'admission\06-host-pid.yaml') `
    -Name 'hostPID=true' -Defence 'PSA restricted: hostPID forbidden'

Run-RuntimeAttack -Yaml (Join-Path $attacksDir 'runtime\attacker.yaml')

# Render report
$now = (Get-Date).ToString('yyyy-MM-dd HH:mm:ss zzz')
$ctx = kubectl config current-context
$escalations = ($script:results | Where-Object { -not $_.Blocked }).Count
$total = $script:results.Count

if ($escalations -eq 0) {
    $summaryLine = ('All {0} attacks blocked. Sandbox defences are intact.' -f $total)
    $summaryEmoji = '+'
} else {
    $summaryLine = ('{0} / {1} attacks ESCALATED - REGRESSION.' -f $escalations, $total)
    $summaryEmoji = '!'
}

$sb = New-Object System.Text.StringBuilder
[void]$sb.AppendLine('# Sandbox Attack Report')
[void]$sb.AppendLine()
[void]$sb.AppendLine(('> Last run: {0}' -f $now))
[void]$sb.AppendLine(('> Kube context: {0}{1}{0}' -f $BT, $ctx))
[void]$sb.AppendLine(('> Source: {0}scripts/sandbox-attack-test.ps1{0}' -f $BT))
[void]$sb.AppendLine()
[void]$sb.AppendLine(('**[{0}] {1}**' -f $summaryEmoji, $summaryLine))
[void]$sb.AppendLine()
[void]$sb.AppendLine('## Results')
[void]$sb.AppendLine()
[void]$sb.AppendLine('| Layer | Attack | Defence | Outcome | Evidence (truncated) |')
[void]$sb.AppendLine('| ----- | ------ | ------- | ------- | -------------------- |')
foreach ($r in $script:results) {
    $outcome = if ($r.Blocked) { 'blocked' } else { 'ESCALATED' }
    $ev = ($r.Evidence -replace '\|', '\|' -replace '`', "''" -replace "`r|`n", ' ').Trim()
    if ($ev.Length -gt 100) { $ev = $ev.Substring(0, 97) + '...' }
    [void]$sb.AppendLine(('| {0} | {1} | {2} | {3} | {4}{5}{4} |' -f $r.Layer, $r.Name, $r.Defence, $outcome, $BT, $ev))
}
[void]$sb.AppendLine()
[void]$sb.AppendLine('## Methodology')
[void]$sb.AppendLine()
[void]$sb.AppendLine('Two complementary layers of defence are exercised:')
[void]$sb.AppendLine()
[void]$sb.AppendLine(('1. **Admission-time** - A malformed pod spec is {0}kubectl apply{0}-d to the' -f $BT))
[void]$sb.AppendLine(('   {0}iicpc-contestants{0} namespace. The API server should reject it before' -f $BT))
[void]$sb.AppendLine(('   the pod is ever scheduled. Each attack is one {0}infra/sandbox-attacks/admission/*.yaml{0}.' -f $BT))
[void]$sb.AppendLine('2. **Runtime** - A *legitimate-looking* pod (matches contestant baseline')
[void]$sb.AppendLine('   securityContext) runs 6 in-pod attacks against the kernel/sandbox')
[void]$sb.AppendLine('   boundary: file write to read-only root, raw socket open, mount syscall,')
[void]$sb.AppendLine('   cross-PID memory read, NetworkPolicy egress bypass, and package install')
[void]$sb.AppendLine(('   as non-root. Each prints {0}BLOCKED: reason{0} or {0}ESCALATED{0}.' -f $BT))
[void]$sb.AppendLine()
[void]$sb.AppendLine('### A note on kindnet (the default kind CNI)')
[void]$sb.AppendLine()
[void]$sb.AppendLine(('Out of the box, kindnet does NOT enforce {0}NetworkPolicy{0} -- the egress' -f $BT))
[void]$sb.AppendLine(('attack would ESCALATE. This script auto-sets {0}KINDNET_NETWORK_POLICY=true{0}' -f $BT))
[void]$sb.AppendLine('on the kindnet DaemonSet so the policy is honoured. On production CNIs')
[void]$sb.AppendLine('(AWS VPC CNI, Calico, Cilium) NetworkPolicy is enforced natively and the')
[void]$sb.AppendLine('step is a no-op.')
[void]$sb.AppendLine()
[void]$sb.AppendLine('Re-run on any cluster:')
[void]$sb.AppendLine()
[void]$sb.AppendLine(('{0}{0}{0}powershell' -f $BT))
[void]$sb.AppendLine('./scripts/sandbox-attack-test.ps1')
[void]$sb.AppendLine(('{0}{0}{0}' -f $BT))
[void]$sb.AppendLine()
[void]$sb.AppendLine('Wire into CI as a regression guard - the runner exits non-zero on any')
[void]$sb.AppendLine('escalation, so a PR that weakens isolation would be caught immediately.')
[void]$sb.AppendLine()
[void]$sb.AppendLine('## What this proves')
[void]$sb.AppendLine()
[void]$sb.AppendLine('Most hackathon submissions ship gVisor + NetworkPolicy + seccomp + PSA')
[void]$sb.AppendLine('restricted and stop there. This suite turns those claims into evidence:')
[void]$sb.AppendLine('every defence layer rejects a concrete attack, and the rejection text is')
[void]$sb.AppendLine('captured verbatim. If any control regresses (e.g., a future PR drops')
[void]$sb.AppendLine(('{0}readOnlyRootFilesystem{0}), the corresponding attack succeeds and the' -f $BT))
[void]$sb.AppendLine('script fails red.')
[void]$sb.AppendLine()
[void]$sb.AppendLine('The matching production pod spec lives at')
[void]$sb.AppendLine(('{0}services/sandbox-runner/internal/runner/podspec.go{0}.' -f $BT))

Set-Content -Path $reportPath -Value $sb.ToString() -Encoding utf8
Write-Host ''
Write-Host ("Report written -> {0}" -f $reportPath) -ForegroundColor Cyan
$color = if ($escalations -eq 0) { 'Green' } else { 'Red' }
Write-Host ("Summary: {0}" -f $summaryLine) -ForegroundColor $color

if ($escalations -ne 0) { exit 1 } else { exit 0 }
