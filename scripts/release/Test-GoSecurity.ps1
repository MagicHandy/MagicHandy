#requires -Version 5.1
<#
.SYNOPSIS
Checks the compiled Go version and known vulnerable symbols in a packaged binary.
#>
[CmdletBinding()]
param([Parameter(Mandatory = $true)][string]$Executable)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$repository = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$binary = (Resolve-Path -LiteralPath $Executable -ErrorAction Stop).Path
$module = Get-Content -LiteralPath (Join-Path $repository 'go.mod') -Raw
$required = [regex]::Match($module, '(?m)^go (\d+\.\d+\.\d+)\s*$')
if (-not $required.Success) { throw 'go.mod must declare a patched Go minimum.' }

Push-Location $repository
try {
    $metadata = @(& go version -m $binary) -join "`n"
    if ($LASTEXITCODE -ne 0) { throw 'Could not inspect the packaged Go toolchain.' }
    $compiled = [regex]::Match($metadata, '^.+:\s+go(\d+\.\d+\.\d+)(?:\s|$)')
    if (-not $compiled.Success -or [Version]$compiled.Groups[1].Value -lt [Version]$required.Groups[1].Value) {
        throw "The packaged executable must use Go $($required.Groups[1].Value) or newer: $binary"
    }
    & go tool nm $binary > $null
    if ($LASTEXITCODE -ne 0) {
        throw "The packaged executable must retain its symbol table for vulnerability scanning: $binary"
    }
    Write-Host "Scanning $([System.IO.Path]::GetFileName($binary)) (Go $($compiled.Groups[1].Value))..."
    & go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 -mode=binary $binary
    if ($LASTEXITCODE -ne 0) {
        throw "Go vulnerability scan failed for '$binary' (exit $LASTEXITCODE)."
    }
} finally {
    Pop-Location
}
