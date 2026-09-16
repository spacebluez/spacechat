param(
    [string]$Server = "ws://127.0.0.1:18080/ws",
    [string]$Version = "0.2.1"
)
$ErrorActionPreference = "Stop"
$root = Split-Path $PSScriptRoot -Parent
Push-Location $root
$previousOS = $env:GOOS
$previousArch = $env:GOARCH
$previousCGO = $env:CGO_ENABLED
try {
    if ($Server -notmatch '^wss?://[^\s]+$' -or $Version -notmatch '^[a-zA-Z0-9._-]+$') { throw "Invalid build arguments" }
    New-Item -ItemType Directory -Force -Path dist | Out-Null
    $env:CGO_ENABLED = "0"
    $env:GOARCH = "amd64"
    $env:GOOS = "windows"
    go build -trimpath -ldflags "-s -w -X main.defaultServer=$Server -X main.version=$Version" -o dist/xchat.exe ./cmd/xchat
    if ($LASTEXITCODE -ne 0) { throw "Client build failed" }
    $env:GOOS = "linux"
    go build -trimpath -ldflags "-s -w" -o dist/xchat-server-linux-amd64 ./cmd/xchat-server
    if ($LASTEXITCODE -ne 0) { throw "Server build failed" }
    Copy-Item deploy/install.sh, deploy/xchat.service, deploy/cleanup.sh, deploy/xchat-cleanup.service, deploy/xchat-cleanup.timer, README.md -Destination dist
    Compress-Archive -Path dist/xchat.exe, README.md -DestinationPath dist/xchat-windows-amd64.zip -Force
    $sourcePaths = @("cmd", "internal", "scripts", "deploy", "docs", "go.mod", "go.sum", "README.md", ".gitignore")
    Compress-Archive -Path $sourcePaths -DestinationPath dist/xchat-source.zip -Force
    Write-Output "Artifacts built in $root\dist"
}
finally {
    $env:GOOS = $previousOS
    $env:GOARCH = $previousArch
    $env:CGO_ENABLED = $previousCGO
    Pop-Location
}
