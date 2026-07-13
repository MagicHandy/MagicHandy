<#
.SYNOPSIS
    Updates a source installation of MagicHandy without discarding setup choices.

.DESCRIPTION
    Reads the non-secret choices saved by install.ps1, displays them, and asks
    whether they should be modified. Unless reconfiguration is requested, the
    same data directory, port, managed llama.cpp backend, Ollama preference,
    Parakeet choice, and launcher choice are reused.

    The updater refuses to pull over local source changes and only performs a
    fast-forward Git update. It then invokes the current install.ps1 so newly
    added dependencies and worker binaries are provisioned consistently.

.PARAMETER Yes
    Preserve all saved choices and accept required package prompts without
    asking whether to reconfigure.

.PARAMETER Reconfigure
    Skip the initial question and walk through every saved installation choice.

.PARAMETER NoPull
    Rebuild the current checkout without running git pull --ff-only.

.PARAMETER NoLaunch
    Update and rebuild without starting the app.

.PARAMETER StatePath
    Override the saved installer-state path.

.PARAMETER PlanOnly
    Show what the preserved or revised setup would provision without pulling,
    installing, building, saving state, or launching.

.EXAMPLE
    .\update.ps1

.EXAMPLE
    .\update.ps1 -Yes -NoLaunch
    Fast-forward and rebuild unattended using the previous choices.

.EXAMPLE
    .\update.ps1 -Reconfigure
    Fast-forward, then revisit choices such as managed llama.cpp and Parakeet.
#>
#Requires -Version 5.1
[CmdletBinding()]
param(
    [switch]$Yes,
    [switch]$Reconfigure,
    [switch]$NoPull,
    [switch]$NoLaunch,
    [string]$StatePath,
    [switch]$PlanOnly
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$Repo = Split-Path -Parent $MyInvocation.MyCommand.Path
$support = Join-Path $Repo 'scripts\installer\InstallerSupport.psm1'
if (-not (Test-Path -LiteralPath $support)) {
    throw "Installer support module not found at '$support'."
}
Import-Module $support -Force -DisableNameChecking
if (-not $StatePath) {
    $StatePath = Get-MagicHandyInstallStatePath
}

Write-MagicHandyBanner -Operation Update
Write-Host '  Existing provider and storage choices are preserved by default.' -ForegroundColor DarkGray

$state = Read-MagicHandyInstallState -Path $StatePath
Write-InstallerHeading 'Current installation choices'
Show-MagicHandyInstallState -State $state

$modifyChoices = if ($Reconfigure) {
    $true
} elseif ($Yes) {
    $false
} else {
    Confirm-MagicHandyChoice -Question 'Modify previous installation choices before rebuilding?' -Default $false
}

if (-not $NoPull -and -not $PlanOnly) {
    Write-InstallerHeading 'Update source'
    Update-MagicHandySource -RepositoryPath $Repo -AssumeYes:$Yes
} elseif ($PlanOnly) {
    Write-Host 'Plan-only mode: source update skipped.' -ForegroundColor DarkGray
} else {
    Write-Host 'Source update skipped by -NoPull.' -ForegroundColor DarkGray
}

$installer = Join-Path $Repo 'install.ps1'
if (-not (Test-Path -LiteralPath $installer)) {
    throw "Installer not found after source update at '$installer'."
}
$arguments = @{
    StatePath = $StatePath
    UpdateRun = $true
}
if ($modifyChoices) {
    $arguments.Reconfigure = $true
} else {
    $arguments.UseSavedChoices = $true
}
if ($Yes) {
    $arguments.Yes = $true
}
if ($NoLaunch) {
    $arguments.NoLaunch = $true
}
if ($PlanOnly) {
    $arguments.PlanOnly = $true
}

& $installer @arguments
if ($PlanOnly) {
    Write-MagicHandyCompletionArt -Operation UpdatePlan
} else {
    Write-MagicHandyCompletionArt -Operation Update
}
