param(
    [string]$Server = "wss://127.0.0.1:18081/ws",
    [string]$Version = "0.4.0",
    [string]$TLSCA = ""
)
$ErrorActionPreference = "Stop"
$root = Split-Path $PSScriptRoot -Parent
Push-Location $root
$previousOS = $env:GOOS
$previousArch = $env:GOARCH
$previousCGO = $env:CGO_ENABLED
try {
    if ($Server -notmatch '^wss?://[^\s]+$' -or $Version -notmatch '^[a-zA-Z0-9._-]+$') { throw "Invalid build arguments" }
    New-Item -ItemType Directory -Force -Path dist/rooms-client, dist/rooms-server | Out-Null
    $env:CGO_ENABLED = "0"
    $env:GOARCH = "amd64"
    $env:GOOS = "windows"
    $linkFlags = "-s -w -X main.defaultServer=$Server -X main.version=$Version"
    if ($TLSCA) {
        $caBytes = [System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $TLSCA).Path)
        if ([System.Text.Encoding]::UTF8.GetString($caBytes) -notmatch '-----BEGIN CERTIFICATE-----' -or [System.Text.Encoding]::UTF8.GetString($caBytes) -match 'PRIVATE KEY') { throw "TLSCA must contain public PEM certificates only" }
        $encodedCA = [Convert]::ToBase64String($caBytes)
        $linkFlags += " -X main.defaultTLSCA=$encodedCA"
    }
    go build -trimpath -ldflags $linkFlags -o "dist/xchat-rooms-$Version.exe" ./cmd/xchat
    if ($LASTEXITCODE -ne 0) { throw "Client build failed" }
    Copy-Item "dist/xchat-rooms-$Version.exe" dist/rooms-client/xchat-rooms.exe -Force
    Copy-Item README.md dist/rooms-client/README.md -Force
    New-Item -ItemType Directory -Force -Path "dist/rooms-client/config" | Out-Null
    Copy-Item "internal/kaomoji/defaults.json" "dist/rooms-client/config/kaomoji.json" -Force
    & "$PSScriptRoot/package-terminal.ps1" -Package 'dist/rooms-client'
    $env:GOOS = "linux"
    go build -trimpath -ldflags "-s -w" -o dist/rooms-server/xchat-rooms-server-linux-amd64 ./cmd/xchat-server
    if ($LASTEXITCODE -ne 0) { throw "Server build failed" }
    Copy-Item deploy/rooms/*, deploy/clear_history.py, README.md -Destination dist/rooms-server -Force
    Copy-Item internal/kaomoji/defaults.json dist/rooms-server/kaomoji.json -Force
    Compress-Archive -Path dist/rooms-client/* -DestinationPath "dist/xchat-rooms-windows-amd64-$Version.zip" -Force
    Compress-Archive -Path dist/rooms-server/* -DestinationPath "dist/xchat-rooms-server-linux-amd64-$Version.zip" -Force
    $sourcePaths = @("cmd", "internal", "scripts", "deploy", "docs", "go.mod", "go.sum", "README.md", ".gitignore", ".gitattributes")
    Compress-Archive -Path $sourcePaths -DestinationPath "dist/xchat-rooms-source-$Version.zip" -Force
    Write-Output "Isolated rooms artifacts built in $root\dist"
}
finally {
    $env:GOOS = $previousOS
    $env:GOARCH = $previousArch
    $env:CGO_ENABLED = $previousCGO
    Pop-Location
}
