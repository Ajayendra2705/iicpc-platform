# Build and (optionally) push all 9 service images + web UI image.
# Uses one shared Go Dockerfile parameterised by SERVICE.
#
# Local sanity-build (no push, single platform):
#   ./build-images.ps1 -Registry localhost:5000 -Platform linux/amd64
#
# Production push to ECR (multi-arch):
#   ./build-images.ps1 -Registry 1234.dkr.ecr.ap-south-1.amazonaws.com -Tag v0.1.0 -Push
param(
    [Parameter(Mandatory=$true)] [string]$Registry,
    [string]$Tag      = "latest",
    [string]$Platform = "linux/arm64",
    [switch]$Push,
    [string[]]$Only   = @()
)

$ErrorActionPreference = "Stop"
$root = Resolve-Path "$PSScriptRoot\..\.."

# Service name → exposed port (informational; Dockerfile doesn't EXPOSE).
$services = [ordered]@{
    "api-gateway"        = 8080
    "submission-svc"     = 8081
    "sandbox-runner"     = 9090
    "bot-coordinator"    = 8083
    "bot-worker"         = 9090
    "telemetry-ingester" = 9091
    "aggregator"         = 8084
    "validator"          = 8085
    "leaderboard-svc"    = 8086
}

function Build-One {
    param([string]$Name, [string]$Dockerfile, [string]$Context, [string]$BuildArg)
    $image = "${Registry}/iicpc/${Name}:${Tag}"
    Write-Host "==> $image" -ForegroundColor Cyan

    $args = @("buildx", "build", "--platform", $Platform)
    if ($BuildArg) { $args += @("--build-arg", $BuildArg) }
    $args += @("-f", $Dockerfile, "-t", $image)
    if ($Push) { $args += "--push" } else { $args += "--load" }
    $args += $Context

    & docker @args
    if ($LASTEXITCODE -ne 0) { Write-Error "build failed: $Name"; exit 1 }
}

$targets = if ($Only.Count -gt 0) { $Only } else { @($services.Keys) + "web" }

foreach ($t in $targets) {
    if ($t -eq "web") {
        Build-One -Name "web" `
                  -Dockerfile "$root\web\Dockerfile" `
                  -Context "$root\web"
    } elseif ($services.Contains($t)) {
        Build-One -Name $t `
                  -Dockerfile "$root\infra\docker\Dockerfile.service" `
                  -Context $root `
                  -BuildArg "SERVICE=$t"
    } else {
        Write-Warning "unknown target: $t (skipping)"
    }
}

Write-Host "DONE — built $($targets.Count) image(s) at tag $Tag" -ForegroundColor Green
