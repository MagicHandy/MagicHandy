[CmdletBinding()]
param([string]$OutputPath)
$ErrorActionPreference = 'Stop'
$repository = Split-Path $PSScriptRoot -Parent
if (-not $OutputPath) { $OutputPath = Join-Path $repository '.scratch/magichandy-labs.exe' }
New-Item -ItemType Directory -Force (Split-Path $OutputPath -Parent) | Out-Null
Push-Location (Join-Path $repository 'web')
try {
    npm.cmd run build:labs
    if ($LASTEXITCODE -ne 0) { throw 'UI build failed.' }
} finally { Pop-Location }
$previousCgo = $env:CGO_ENABLED
Push-Location $repository
try {
    $env:CGO_ENABLED = '0'
    go build -o $OutputPath ./cmd/magichandy
    if ($LASTEXITCODE -ne 0) { throw 'Go build failed.' }
} finally { $env:CGO_ENABLED = $previousCgo; Pop-Location }
Write-Output "Built: $OutputPath. Enable Labs in Settings > General."
