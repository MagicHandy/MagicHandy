#Requires -Version 5.1
<#
.SYNOPSIS
Installs the prerequisites for, then builds, MagicHandy's managed llama.cpp runtime.

.DESCRIPTION
This narrow wrapper is shared by the in-app setup wizard and unattended source
installation. The GUI owns the user's backend choice and consent; this script
keeps dependency provisioning and the pinned runtime build in one recoverable
PowerShell path.
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

$supportPath = Join-Path $PSScriptRoot 'installer\InstallerSupport.psm1'
if (-not (Test-Path -LiteralPath $supportPath -PathType Leaf)) {
    throw "Installer support module not found at '$supportPath'. Repair MagicHandy and retry."
}
Import-Module $supportPath -Force -DisableNameChecking

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
    throw 'The pinned managed llama.cpp build helper is unavailable. Repair MagicHandy and retry.'
}

$resolvedDataDir = [System.IO.Path]::GetFullPath($DataDir)
New-Item -ItemType Directory -Force -Path $resolvedDataDir | Out-Null

Write-Host 'Managed llama.cpp is optional. Building it keeps inference under MagicHandy control and avoids a separate Ollama model copy.'
Write-Host 'CPU builds can use MSYS2 UCRT64 GCC/CMake/Ninja or Visual Studio C++ Build Tools. CUDA builds require Visual Studio C++ and the NVIDIA CUDA Toolkit.' -ForegroundColor DarkGray

InstallerSupport\Ensure-MagicHandyGit -AssumeYes:$Yes | Out-Null
$cudaSelected = $Backend -eq 'cuda' -or (
    $Backend -eq 'auto' -and
    $null -ne (Get-Command 'nvidia-smi' -ErrorAction SilentlyContinue) -and
    -not [string]::IsNullOrWhiteSpace((InstallerSupport\Resolve-MagicHandyExecutable -Name 'nvcc'))
)
if ($cudaSelected) {
    InstallerSupport\Ensure-MagicHandyCMake -AssumeYes:$Yes | Out-Null
    InstallerSupport\Ensure-MagicHandyVCToolchain -AssumeYes:$Yes
    InstallerSupport\Ensure-MagicHandyCUDA -AssumeYes:$Yes | Out-Null
} else {
    InstallerSupport\Ensure-MagicHandyLlamaCPUToolchain -AssumeYes:$Yes | Out-Null
}

& $BuildScript -DataDir $resolvedDataDir -Backend $Backend
if ($LASTEXITCODE -ne 0) {
    throw "Managed llama.cpp build failed (exit $LASTEXITCODE)."
}
