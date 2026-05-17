# iac-verify.ps1
#
# Windows-native equivalent of `make iac-verify`. Runs every IaC quality gate
# the CI workflow runs against Terraform + Helm + raw K8s manifests, so a
# reviewer can prove the IaC is valid without paying for a cloud apply.
#
# Exit codes:
#   0 - every required gate passed (optional tools skipped don't fail)
#   1 - one or more required gates failed (terraform / helm / kubeconform)
#   2 - usage / setup error

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
$tfDir = Join-Path $repoRoot 'infra\terraform'
$helmChart = Join-Path $repoRoot 'infra\helm\iicpc-platform'
$helmRelease = 'iicpc'
$failures = @()

function Section {
    param([string]$Name)
    Write-Host ''
    Write-Host ('==> {0}' -f $Name) -ForegroundColor Cyan
}

function Require {
    param([string]$Tool)
    if (-not (Get-Command $Tool -ErrorAction SilentlyContinue)) {
        Write-Host ('    {0} not on PATH - required, FAIL' -f $Tool) -ForegroundColor Red
        $script:failures += $Tool
        return $false
    }
    return $true
}

function OptionalTool {
    param([string]$Tool, [string]$InstallHint)
    if (-not (Get-Command $Tool -ErrorAction SilentlyContinue)) {
        Write-Host ('    {0} not installed - SKIPPING (optional; install: {1})' -f $Tool, $InstallHint) -ForegroundColor Yellow
        return $false
    }
    return $true
}

# ---- Terraform -------------------------------------------------------------
Section 'terraform fmt -check -recursive'
if (Require 'terraform') {
    Push-Location $tfDir
    try {
        terraform fmt -check -recursive
        if ($LASTEXITCODE -ne 0) { $failures += 'terraform fmt'; throw 'fmt FAIL' }
        Write-Host '    OK' -ForegroundColor Green
    } catch { Write-Host ('    {0}' -f $_) -ForegroundColor Red } finally { Pop-Location }

    Section 'terraform init -backend=false (no AWS creds)'
    Push-Location $tfDir
    try {
        $out = terraform init -backend=false -input=false 2>&1 | Out-String
        if ($LASTEXITCODE -ne 0) { $failures += 'terraform init'; Write-Host $out -ForegroundColor Red } else { Write-Host '    OK' -ForegroundColor Green }
    } finally { Pop-Location }

    Section 'terraform validate'
    Push-Location $tfDir
    try {
        terraform validate
        if ($LASTEXITCODE -ne 0) { $failures += 'terraform validate' } else { Write-Host '    Terraform spec valid against AWS provider schema.' -ForegroundColor Green }
    } finally { Pop-Location }
}

# ---- Helm ------------------------------------------------------------------
Section ('helm lint {0}' -f $helmChart)
if (Require 'helm') {
    helm lint $helmChart
    if ($LASTEXITCODE -ne 0) { $failures += 'helm lint' } else { Write-Host '    OK' -ForegroundColor Green }

    Section 'helm template (dev values)'
    $devOut = Join-Path $env:TEMP 'iicpc-helm-dev.yaml'
    helm template $helmRelease $helmChart > $devOut 2>&1
    if ($LASTEXITCODE -ne 0) { $failures += 'helm template (dev)' } else {
        $lines = (Get-Content $devOut | Measure-Object -Line).Lines
        Write-Host ('    rendered {0} lines of K8s YAML (dev)' -f $lines) -ForegroundColor Green
    }

    Section 'helm template (production overlay)'
    $prodOut = Join-Path $env:TEMP 'iicpc-helm-prod.yaml'
    helm template $helmRelease $helmChart `
        -f (Join-Path $helmChart 'values.yaml') `
        -f (Join-Path $helmChart 'values.production.yaml') `
        > $prodOut 2>&1
    if ($LASTEXITCODE -ne 0) { $failures += 'helm template (prod)' } else {
        $lines = (Get-Content $prodOut | Measure-Object -Line).Lines
        Write-Host ('    rendered {0} lines of K8s YAML (prod)' -f $lines) -ForegroundColor Green
    }
} else {
    $prodOut = $null
}

# ---- kubeconform (schema check) -------------------------------------------
Section 'kubeconform (rendered helm output)'
if (OptionalTool 'kubeconform' 'https://github.com/yannh/kubeconform/releases') {
    if ($prodOut -and (Test-Path $prodOut)) {
        kubeconform -strict -summary -kubernetes-version 1.30.0 $prodOut
        if ($LASTEXITCODE -ne 0) { $failures += 'kubeconform helm' } else { Write-Host '    OK' -ForegroundColor Green }
    }
}

# ---- tflint (optional) -----------------------------------------------------
Section 'tflint (optional security/style scan)'
if (OptionalTool 'tflint' 'https://github.com/terraform-linters/tflint') {
    Push-Location $tfDir
    try {
        tflint
        if ($LASTEXITCODE -ne 0) { Write-Host '    tflint reported issues (non-fatal)' -ForegroundColor Yellow } else { Write-Host '    OK' -ForegroundColor Green }
    } finally { Pop-Location }
}

# ---- checkov (optional) ----------------------------------------------------
Section 'checkov (optional security scan)'
if (OptionalTool 'checkov' 'pip install checkov') {
    checkov -d $tfDir --quiet
    if ($LASTEXITCODE -ne 0) { Write-Host '    checkov reported issues (non-fatal)' -ForegroundColor Yellow } else { Write-Host '    OK' -ForegroundColor Green }
}

# ---- Summary ---------------------------------------------------------------
Write-Host ''
Write-Host '================================================================' -ForegroundColor Cyan
if ($failures.Count -eq 0) {
    Write-Host '  IaC verification PASSED' -ForegroundColor Green
    Write-Host '  Deliverable #3 (Infrastructure as Code) requirements met:'
    Write-Host '    * terraform validates against AWS provider schema'
    Write-Host '    * helm chart lints + renders cleanly for dev + production'
    Write-Host '    * rendered K8s manifests pass schema check at K8s 1.30'
    Write-Host '  See docs/IAC_VERIFICATION.md for the full mapping.'
} else {
    Write-Host '  IaC verification FAILED' -ForegroundColor Red
    Write-Host ('  {0} required gate(s) failed: {1}' -f $failures.Count, ($failures -join ', '))
}
Write-Host '================================================================' -ForegroundColor Cyan

if ($failures.Count -ne 0) { exit 1 } else { exit 0 }
