# Launcher invoked by prepare.ps1 - runs the Next.js dev server.
param(
    [int]$Port = 3000,
    [int]$LeaderboardPort = 8086,
    [string]$LogPath
)

if ($LogPath) {
    $LogPath = [System.IO.Path]::GetFullPath($LogPath)
}

$env:PORT = "$Port"
$env:NEXT_PUBLIC_LEADERBOARD_WS = "ws://127.0.0.1:$LeaderboardPort/live"
$env:LEADERBOARD_URL = "http://127.0.0.1:$LeaderboardPort"
Set-Location "$PSScriptRoot\..\..\web"

if ($LogPath) {
    npm run dev *> $LogPath
} else {
    npm run dev
}
