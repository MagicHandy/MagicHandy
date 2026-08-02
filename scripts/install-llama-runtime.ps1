#Requires -Version 5.1
<#
.SYNOPSIS
Installs MagicHandy's pinned, verified managed llama.cpp runtime.

.DESCRIPTION
This narrow wrapper is shared by the in-app setup wizard and unattended source
installation. The GUI owns the user's backend choice and consent; this script
keeps the pinned runtime download and activation in one recoverable PowerShell
path. No compiler or CUDA Toolkit is required.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$DataDir,
    [ValidateSet('auto', 'cpu', 'cuda')]
    [string]$Backend = 'auto',
    [string]$BuildScript = '',
    [switch]$Yes
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if ([string]::IsNullOrWhiteSpace($BuildScript)) {
    $candidates = @(
        (Join-Path $PSScriptRoot 'build-managed-llama.ps1'),
        (Join-Path $PSScriptRoot '..\internal\llm\runtimeassets\build-managed-llama.ps1')
    )
    $BuildScript = $candidates |
        Where-Object { Test-Path -LiteralPath $_ -PathType Leaf } |
        Select-Object -First 1
}
if ([string]::IsNullOrWhiteSpace($BuildScript) -or -not (Test-Path -LiteralPath $BuildScript -PathType Leaf)) {
    throw 'The pinned managed llama.cpp installer helper is unavailable. Repair MagicHandy and retry.'
}

$resolvedDataDir = [System.IO.Path]::GetFullPath($DataDir)
New-Item -ItemType Directory -Force -Path $resolvedDataDir | Out-Null

Write-Host 'Managed llama.cpp is optional. Installing it keeps inference under MagicHandy control and avoids requiring Ollama.'
Write-Host 'MagicHandy downloads official b9966 Windows bundles and verifies pinned SHA-256 digests. CPU is about 18 MiB; CUDA is about 628 MiB and only requires a compatible NVIDIA driver.' -ForegroundColor DarkGray

& $BuildScript -DataDir $resolvedDataDir -Backend $Backend
if ($LASTEXITCODE -ne 0) {
    throw "Managed llama.cpp installation failed (exit $LASTEXITCODE)."
}
