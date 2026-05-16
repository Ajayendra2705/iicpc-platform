# Launcher invoked by prepare.ps1 - runs leaderboard-svc in SEED_DEMO mode.
param(
    [int]$Port = 8086,
    [string]$LogPath
)

# Resolve LogPath BEFORE Set-Location so a relative path keeps working.
if ($LogPath) {
    $LogPath = [System.IO.Path]::GetFullPath($LogPath)
}

$env:SEED_DEMO = "true"
$env:HTTP_ADDR = ":$Port"
Set-Location "$PSScriptRoot\..\..\services\leaderboard-svc"

if ($LogPath) {
    go run . *> $LogPath
} else {
    go run .
}
