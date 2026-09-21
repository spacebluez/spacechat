param([Parameter(Mandatory = $true)][string]$Package)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$terminalVersion = '1.24.11911.0'
$expectedHash = '7691efeb71c8dd0b95536c84e366fa4cf809a42c534912f9cefa1056534383bd'
$cache = Join-Path $root 'dist/terminal-cache'
$archive = Join-Path $cache "Microsoft.WindowsTerminal_${terminalVersion}_x64.zip"
$expanded = Join-Path $cache "unpacked-$terminalVersion"
New-Item -ItemType Directory -Force -Path $cache | Out-Null
if (-not (Test-Path -LiteralPath $archive)) {
    Invoke-WebRequest -UseBasicParsing -Uri "https://github.com/microsoft/terminal/releases/download/v$terminalVersion/Microsoft.WindowsTerminal_${terminalVersion}_x64.zip" -OutFile $archive
}
if ((Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash -ne $expectedHash) {
    throw 'Windows Terminal archive SHA256 mismatch'
}
Expand-Archive -LiteralPath $archive -DestinationPath $expanded -Force
$terminal = Join-Path $Package 'terminal'
New-Item -ItemType Directory -Force -Path $terminal | Out-Null
Copy-Item -Path (Join-Path $expanded "terminal-$terminalVersion/*") -Destination $terminal -Recurse -Force
New-Item -ItemType Directory -Force -Path (Join-Path $terminal 'settings') | Out-Null
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'windows-terminal/.portable') -Destination (Join-Path $terminal '.portable') -Force
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'windows-terminal/settings.json') -Destination (Join-Path $terminal 'settings/settings.json') -Force
Write-Output "Packaged Windows Terminal $terminalVersion with isolated font fallback settings"
