<#
.SYNOPSIS
    Updates a source installation of MagicHandy without discarding setup choices.

.DESCRIPTION
    Reads the non-secret choices saved by install.ps1, displays them, and asks
    whether they should be modified. Unless reconfiguration is requested, the
    same data directory, port, managed llama.cpp selection, Ollama preference,
    Parakeet choice, managed local TTS install choice, and launcher choice are
    reused. An already installed TTS module is verified and reused without
    repeating its multi-gigabyte installation; use
    scripts\update-tts-module.ps1 when intentionally updating that module.

    The updater refuses to update over local source changes and only performs a
    fast-forward Git update. Main follows origin/main, live feature branches
    follow their configured upstream, and merged features whose remote branch
    was deleted may safely advance from origin/main. It then invokes the current
    install.ps1 so dependencies and worker binaries are provisioned consistently.
    A running app from this checkout receives Emergency Stop and is terminated
    before replacement; launch succeeds only after the rebuilt server is ready.

.PARAMETER Yes
    Preserve all saved choices and accept required package prompts without
    asking whether to reconfigure.

.PARAMETER Reconfigure
    Skip the initial question and walk through every saved installation choice.

.PARAMETER NoPull
    Rebuild the current checkout without fetching or fast-forwarding source.

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
    Resolve a safe fast-forward target and rebuild unattended using the previous choices.

.EXAMPLE
    .\update.ps1 -Reconfigure
    Safely fast-forward, then revisit choices such as managed llama.cpp,
    Parakeet, and managed local TTS.
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

if ($Reconfigure -and $Yes) {
    throw 'Reconfigure is interactive and cannot be combined with Yes.'
}
$Repo = Split-Path -Parent $MyInvocation.MyCommand.Path
$support = Join-Path $Repo 'scripts\installer\InstallerSupport.psm1'
if (-not (Test-Path -LiteralPath $support)) {
    throw "Installer support module not found at '$support'."
}
Import-Module $support -Force -DisableNameChecking
if (-not $StatePath) {
    $StatePath = Get-MagicHandyInstallStatePath
}
$StatePath = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($StatePath)

$state = Read-MagicHandyInstallState -Path $StatePath
Set-MagicHandyInstallerLocale -Locale ([string]$state.ui_locale)
Write-MagicHandyBanner -Operation Update
Write-Host ('  ' + (Get-MagicHandyText -Key 'update_preserved_note')) -ForegroundColor DarkGray

Write-InstallerHeading (Get-MagicHandyText -Key 'current_heading')
Show-MagicHandyInstallState -State $state

$modifyChoices = if ($Reconfigure) {
    $true
} elseif ($Yes) {
    $false
} else {
    Confirm-MagicHandyChoice -Question (Get-MagicHandyText -Key 'update_modify_question') -Default $false
}

if (-not $NoPull -and -not $PlanOnly) {
    Write-InstallerHeading (Get-MagicHandyText -Key 'update_source_heading')
    Update-MagicHandySource -RepositoryPath $Repo -AssumeYes:$Yes
} elseif ($PlanOnly) {
    Write-Host (Get-MagicHandyText -Key 'update_plan_skip') -ForegroundColor DarkGray
} else {
    Write-Host (Get-MagicHandyText -Key 'update_no_pull') -ForegroundColor DarkGray
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
