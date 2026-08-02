#Requires -Version 5.1
<#
.SYNOPSIS
Installs MagicHandy's pinned Parakeet speech-recognition runner and model.

.DESCRIPTION
The in-app setup wizard invokes this wrapper after explicit user consent. The
shared installer module owns checksum validation and interrupted-download
recovery so source and packaged installations use the same implementation.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$DataDir,
    [switch]$Yes
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$supportPath = Join-Path $PSScriptRoot 'installer\InstallerSupport.psm1'
if (-not (Test-Path -LiteralPath $supportPath -PathType Leaf)) {
    throw "Installer support module not found at '$supportPath'. Repair MagicHandy and retry."
}
Import-Module $supportPath -Force -DisableNameChecking

$resolvedDataDir = [System.IO.Path]::GetFullPath($DataDir)
New-Item -ItemType Directory -Force -Path $resolvedDataDir | Out-Null
InstallerSupport\Install-MagicHandyParakeet -DataDir $resolvedDataDir
