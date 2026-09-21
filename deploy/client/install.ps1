param(
    [Parameter(Mandatory = $true)]
    [string]$Server,
    [Parameter(Mandatory = $true)]
    [string]$Version,
    [string]$Source = $PSScriptRoot
)

$ErrorActionPreference = "Stop"

if ($Version -notmatch '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$') {
    throw "Version must use MAJOR.MINOR.PATCH"
}
$serverUri = [Uri]::new($Server, [UriKind]::Absolute)
if (($serverUri.Scheme -ne "ws" -and $serverUri.Scheme -ne "wss") -or
    [string]::IsNullOrWhiteSpace($serverUri.Host) -or
    -not [string]::IsNullOrEmpty($serverUri.UserInfo) -or
    -not [string]::IsNullOrEmpty($serverUri.Fragment) -or
    $Server -notmatch '^wss?://[^\s"\\]+$') {
    throw "Server must be a ws:// or wss:// URL without credentials or a fragment"
}
if ([string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) {
    throw "LOCALAPPDATA is required"
}

$launcherSource = Join-Path $Source "spacechat.exe"
$clientSource = Join-Path $Source "spacechat-client.exe"
foreach ($path in @($launcherSource, $clientSource)) {
    $item = Get-Item -LiteralPath $path
    if ($item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
        throw "Installer inputs must be regular files"
    }
}

$root = Join-Path $env:LOCALAPPDATA "SpaceChat"
$bin = Join-Path $root "bin"
$versions = Join-Path $root "versions"
$targetDirectory = Join-Path $versions $Version
$targetClient = Join-Path $targetDirectory "spacechat-client.exe"
$configPath = Join-Path $root "config.json"
$currentPath = Join-Path $root "current"

foreach ($path in @($root, $bin, $versions)) {
    if (Test-Path -LiteralPath $path) {
        $item = Get-Item -LiteralPath $path
        if (-not $item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
            throw "Refusing unsafe installation path: $path"
        }
    }
}
if (Test-Path -LiteralPath $targetDirectory) {
    throw "Version $Version is already installed"
}

New-Item -ItemType Directory -Force -Path $root, $bin, $versions | Out-Null
$staging = Join-Path $versions ("." + $Version + ".new-" + [Guid]::NewGuid().ToString("N"))
try {
    New-Item -ItemType Directory -Path $staging | Out-Null
    $stagedClient = Join-Path $staging "spacechat-client.exe"
    Copy-Item -LiteralPath $clientSource -Destination $stagedClient
    & $stagedClient --self-check
    if ($LASTEXITCODE -ne 0) { throw "Client self-check failed" }
    Move-Item -LiteralPath $staging -Destination $targetDirectory

    $launcherTemporary = Join-Path $bin ("spacechat.exe.new-" + [Guid]::NewGuid().ToString("N"))
    Copy-Item -LiteralPath $launcherSource -Destination $launcherTemporary
    Move-Item -LiteralPath $launcherTemporary -Destination (Join-Path $bin "spacechat.exe") -Force

    $configTemporary = Join-Path $root ("config.json.new-" + [Guid]::NewGuid().ToString("N"))
    $configJson = @{ server = $Server } | ConvertTo-Json -Compress
    [IO.File]::WriteAllText($configTemporary, $configJson + "`n", (New-Object Text.UTF8Encoding($false)))
    Move-Item -LiteralPath $configTemporary -Destination $configPath -Force

    $currentTemporary = Join-Path $root ("current.new-" + [Guid]::NewGuid().ToString("N"))
    [IO.File]::WriteAllText($currentTemporary, $Version + "`n", [Text.Encoding]::ASCII)
    Move-Item -LiteralPath $currentTemporary -Destination $currentPath -Force
}
finally {
    if (Test-Path -LiteralPath $staging) {
        Remove-Item -LiteralPath $staging -Recurse -Force
    }
}

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
$pathEntries = @($userPath -split ';' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
if (-not ($pathEntries | Where-Object { $_.TrimEnd('\') -ieq $bin.TrimEnd('\') })) {
    $newPath = (@($pathEntries) + $bin) -join ';'
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
}
if (-not (($env:Path -split ';') | Where-Object { $_.TrimEnd('\') -ieq $bin.TrimEnd('\') })) {
    $env:Path = $env:Path + ';' + $bin
}

Write-Output "SpaceChat $Version installed. Open a new terminal and run: spacechat"
