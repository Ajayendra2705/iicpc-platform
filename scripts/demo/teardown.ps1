# Kill processes started by prepare.ps1. Run BEFORE re-running prepare,
# AFTER the recording session ends, or if something goes wrong mid-take.
param(
    [switch]$Quiet
)

$ErrorActionPreference = "SilentlyContinue"
$root   = Resolve-Path "$PSScriptRoot\..\.."
$pidDir = "$root\.bin"

function Stop-FromPidFile {
    param([string]$Path, [string]$Label)
    if (-not (Test-Path $Path)) { return }
    $procId = (Get-Content $Path -Raw).Trim()
    if (-not $procId) { return }

    # Kill the launcher process tree (Start-Process spawned a cmd.exe)
    try {
        Get-CimInstance -ClassName Win32_Process -Filter "ParentProcessId = $procId" |
            ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
        Stop-Process -Id $procId -Force -ErrorAction SilentlyContinue
        if (-not $Quiet) { Write-Host "[demo:teardown] stopped $Label (pid $procId)" }
    } catch {
        if (-not $Quiet) { Write-Host "[demo:teardown] $Label pid $procId already gone" }
    }
    Remove-Item $Path -Force -ErrorAction SilentlyContinue
}

Stop-FromPidFile -Path "$pidDir\leaderboard.pid" -Label "leaderboard-svc"
Stop-FromPidFile -Path "$pidDir\web.pid"         -Label "web (Next.js)"

# Also kill anything still on the standard demo ports (last-resort cleanup)
foreach ($p in 8086, 3000) {
    Get-NetTCPConnection -LocalPort $p -ErrorAction SilentlyContinue |
        ForEach-Object {
            Stop-Process -Id $_.OwningProcess -Force -ErrorAction SilentlyContinue
            if (-not $Quiet) { Write-Host "[demo:teardown] freed port $p (pid $($_.OwningProcess))" }
        }
}

if (-not $Quiet) { Write-Host "[demo:teardown] DONE" }
