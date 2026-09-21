param(
    [string]$Server = "wss://127.0.0.1:18081/ws",
    [string]$Version = "0.4.0",
    [string]$MinimumVersion = "0.0.0",
    [string]$TLSCA = "",
    [Parameter(Mandatory = $true)]
    [string]$SigningKey
)

$ErrorActionPreference = "Stop"
$root = Split-Path $PSScriptRoot -Parent
Push-Location $root
$previousOS = $env:GOOS
$previousArch = $env:GOARCH
$previousCGO = $env:CGO_ENABLED
try {
    if ($Server -notmatch '^wss?://[^\s"\\]+$' -or
        $Version -notmatch '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$' -or
        $MinimumVersion -notmatch '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$') {
        throw "Invalid build arguments"
    }
    New-Item -ItemType Directory -Force -Path dist | Out-Null
    $publicKey = (go run ./cmd/spacechat-release public-key -private-key $SigningKey).Trim()
    if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($publicKey)) { throw "Cannot derive update public key" }
    $clientFlags = "-s -w -X main.defaultServer=$Server -X main.version=$Version -X main.updatePublicKey=$publicKey"
    if ($TLSCA) {
        $caBytes = [System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $TLSCA).Path)
        $caText = [System.Text.Encoding]::UTF8.GetString($caBytes)
        if ($caText -notmatch '-----BEGIN CERTIFICATE-----' -or $caText -match 'PRIVATE KEY') { throw "TLSCA must contain public PEM certificates only" }
        $clientFlags += " -X main.defaultTLSCA=$([Convert]::ToBase64String($caBytes))"
    }

    $windowsLauncher = "dist/spacechat-windows-amd64.exe"
    $linuxLauncher = "dist/spacechat-linux-amd64"
    $windowsClient = "dist/spacechat-client-windows-amd64-$Version.exe"
    $linuxClient = "dist/spacechat-client-linux-amd64-$Version"
    $updateDirectory = "dist/updates-$Version"
    $installerDirectory = "dist/installers-$Version"
    $windowsPackage = "dist/spacechat-windows-amd64-$Version"
    $linuxPackage = "dist/spacechat-linux-amd64-$Version"
    foreach ($path in @($updateDirectory, $installerDirectory, $windowsPackage, $linuxPackage)) {
        if (Test-Path $path) { Remove-Item -LiteralPath $path -Recurse -Force }
    }

    $env:CGO_ENABLED = "0"
    $env:GOARCH = "amd64"
    $env:GOOS = "windows"
    go build -trimpath -ldflags "-s -w" -o $windowsLauncher ./cmd/spacechat
    if ($LASTEXITCODE -ne 0) { throw "Windows launcher build failed" }
    go build -trimpath -ldflags $clientFlags -o $windowsClient ./cmd/xchat
    if ($LASTEXITCODE -ne 0) { throw "Windows client build failed" }

    $env:GOOS = "linux"
    go build -trimpath -ldflags "-s -w" -o $linuxLauncher ./cmd/spacechat
    if ($LASTEXITCODE -ne 0) { throw "Linux launcher build failed" }
    go build -trimpath -ldflags $clientFlags -o $linuxClient ./cmd/xchat
    if ($LASTEXITCODE -ne 0) { throw "Linux client build failed" }

    $env:GOOS = $previousOS
    $env:GOARCH = $previousArch
    $env:CGO_ENABLED = $previousCGO
    go run ./cmd/spacechat-release manifest -version $Version -minimum $MinimumVersion -private-key $SigningKey -windows $windowsClient -linux $linuxClient -out $updateDirectory
    if ($LASTEXITCODE -ne 0) { throw "Signed manifest build failed" }

    New-Item -ItemType Directory -Path $windowsPackage, $linuxPackage | Out-Null
    Copy-Item $windowsLauncher "$windowsPackage/spacechat.exe"
    Copy-Item $windowsClient "$windowsPackage/spacechat-client.exe"
    Copy-Item deploy/client/install.ps1 "$windowsPackage/install.ps1"
    Copy-Item README.md "$windowsPackage/README.md"
    & "$PSScriptRoot/package-terminal.ps1" -Package $windowsPackage
    Copy-Item $linuxLauncher "$linuxPackage/spacechat"
    Copy-Item $linuxClient "$linuxPackage/spacechat-client"
    Copy-Item deploy/client/install.sh "$linuxPackage/install.sh"
    Copy-Item README.md "$linuxPackage/README.md"

    $windowsArchive = "dist/spacechat-windows-amd64-$Version.zip"
    $linuxArchive = "dist/spacechat-linux-amd64-$Version.zip"
    Compress-Archive -Path "$windowsPackage/*" -DestinationPath $windowsArchive -Force
    Compress-Archive -Path "$linuxPackage/*" -DestinationPath $linuxArchive -Force
    go run ./cmd/spacechat-release installers -version $Version -server $Server -private-key $SigningKey -windows-package $windowsArchive -linux-package $linuxArchive -out $installerDirectory
    if ($LASTEXITCODE -ne 0) { throw "Signed installer catalog build failed" }

    Compress-Archive -Path "$updateDirectory/*" -DestinationPath "dist/spacechat-updates-$Version.zip" -Force
    Compress-Archive -Path "$installerDirectory/*" -DestinationPath "dist/spacechat-installers-$Version.zip" -Force
    $sourcePaths = @("cmd", "internal", "scripts", "deploy", "docs", "go.mod", "go.sum", "README.md", ".gitignore", ".gitattributes")
    Compress-Archive -Path $sourcePaths -DestinationPath "dist/xchat-rooms-source-$Version.zip" -Force
    Write-Output "Signed client release built: $Version"
}
finally {
    $env:GOOS = $previousOS
    $env:GOARCH = $previousArch
    $env:CGO_ENABLED = $previousCGO
    Pop-Location
}
