param(
    [string]$Server = "ws://127.0.0.1:18081/ws",
    [string]$Version = "0.3.2"
)
$ErrorActionPreference = "Stop"
$root = Split-Path $PSScriptRoot -Parent
Push-Location $root
$previousOS = $env:GOOS
$previousArch = $env:GOARCH
$previousCGO = $env:CGO_ENABLED
try {
    if ($Server -notmatch '^wss?://[^\s]+$' -or $Version -notmatch '^[a-zA-Z0-9._-]+$') { throw "Invalid build arguments" }
    $package = "dist/rooms-client-$Version"
    New-Item -ItemType Directory -Force -Path $package | Out-Null
    $env:CGO_ENABLED = "0"
    $env:GOARCH = "amd64"
    $env:GOOS = "windows"
    go build -trimpath -ldflags "-s -w -X main.defaultServer=$Server -X main.version=$Version" -o "dist/xchat-rooms-$Version.exe" ./cmd/xchat
    if ($LASTEXITCODE -ne 0) { throw "Client build failed" }
    Copy-Item "dist/xchat-rooms-$Version.exe" "$package/xchat-rooms.exe" -Force
    Copy-Item README.md "$package/README.md" -Force
    New-Item -ItemType Directory -Force -Path "$package/config" | Out-Null
    Copy-Item "config/kaomoji.json" "$package/config/kaomoji.json" -Force
    Compress-Archive -Path "$package/*" -DestinationPath "dist/xchat-rooms-windows-amd64-$Version.zip" -Force
    $sourcePaths = @("cmd", "config", "internal", "scripts", "deploy", "docs", "go.mod", "go.sum", "README.md", ".gitignore", ".gitattributes")
    Compress-Archive -Path $sourcePaths -DestinationPath "dist/xchat-rooms-source-$Version.zip" -Force
    Write-Output "Client-only release built: $Version (no server changes)"
}
finally {
    $env:GOOS = $previousOS
    $env:GOARCH = $previousArch
    $env:CGO_ENABLED = $previousCGO
    Pop-Location
}
