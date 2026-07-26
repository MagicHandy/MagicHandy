<#
.SYNOPSIS
    Changes MagicHandy's app UI and chat reply languages.

.DESCRIPTION
    Presents the language list before any language-dependent text, updates the
    SQLite-backed app settings through magichandy.exe, and updates existing
    non-secret installer state. If this checkout is running, the script sends
    Emergency Stop and restarts it so stale in-memory settings cannot overwrite
    the new selection.

.PARAMETER UILanguage
    App UI and script language: en, es, pt-BR, zh-Hans, or ja.

.PARAMETER ChatLanguage
    Built-in prompt language for chat replies: en, es, pt-BR, zh-Hans, or ja.

.PARAMETER DataDir
    Override the app data directory. Existing installer state is used by default.

.PARAMETER StatePath
    Override the installer-state path.

.PARAMETER Port
    Override the running app port. Existing installer state is used by default.

.PARAMETER Yes
    Use supplied or saved defaults without prompting and approve a safe restart.

.PARAMETER NoLaunch
    Do not restart MagicHandy after applying the change.

.EXAMPLE
    .\change-language.ps1

.EXAMPLE
    .\change-language.ps1 -UILanguage es -ChatLanguage ja -Yes
#>
#Requires -Version 5.1
[CmdletBinding()]
param(
    [ValidateSet('en', 'es', 'pt-BR', 'zh-Hans', 'ja')]
    [string]$UILanguage,
    [ValidateSet('en', 'es', 'pt-BR', 'zh-Hans', 'ja')]
    [string]$ChatLanguage,
    [string]$DataDir,
    [string]$StatePath,
    [ValidateRange(0, 65535)]
    [int]$Port = 0,
    [switch]$Yes,
    [switch]$NoLaunch
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$Repo = Split-Path -Parent $MyInvocation.MyCommand.Path
$support = Join-Path $Repo 'scripts\installer\InstallerSupport.psm1'
if (-not (Test-Path -LiteralPath $support -PathType Leaf)) {
    throw "Installer support module not found at '$support'."
}
Import-Module $support -Force -DisableNameChecking

if (-not $StatePath) {
    $StatePath = Get-MagicHandyInstallStatePath
}
$StatePath = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($StatePath)
$stateExists = Test-Path -LiteralPath $StatePath -PathType Leaf
$state = if ($stateExists) { Read-MagicHandyInstallState -Path $StatePath } else { $null }
$defaultUI = if ($null -ne $state) { [string]$state.ui_locale } else { 'en' }

$selectedUI = if (-not [string]::IsNullOrWhiteSpace($UILanguage)) {
    $UILanguage
} elseif ($Yes) {
    $defaultUI
} else {
    Read-MagicHandyLanguage -QuestionKey 'language_selector' -Default $defaultUI
}
Set-MagicHandyInstallerLocale -Locale $selectedUI
$defaultChat = if ($null -ne $state) { [string]$state.chat_locale } else { $selectedUI }
$selectedChat = if (-not [string]::IsNullOrWhiteSpace($ChatLanguage)) {
    $ChatLanguage
} elseif ($Yes) {
    $defaultChat
} else {
    Read-MagicHandyLanguage -QuestionKey 'chat_language_selector' -Default $defaultChat
}

Write-InstallerHeading (Get-MagicHandyText -Key 'change_heading')

$resolvedDataDir = if (-not [string]::IsNullOrWhiteSpace($DataDir)) {
    [System.IO.Path]::GetFullPath($DataDir)
} elseif ($null -ne $state) {
    [string]$state.data_dir
} elseif (-not [string]::IsNullOrWhiteSpace($env:APPDATA)) {
    Join-Path $env:APPDATA 'MagicHandy'
} else {
    Join-Path ([Environment]::GetFolderPath('ApplicationData')) 'MagicHandy'
}
$resolvedPort = if ($Port -ne 0) {
    $Port
} elseif ($null -ne $state) {
    [int]$state.port
} else {
    49717
}

$wasRunning = Test-MagicHandyAppRunning -RepositoryPath $Repo
if ($wasRunning) {
    $approved = $Yes -or (Confirm-MagicHandyChoice -Question (Get-MagicHandyText -Key 'change_restart_question') -Default $true)
    if (-not $approved) {
        return
    }
    Stop-MagicHandyAppForRebuild `
        -RepositoryPath $Repo `
        -Port $resolvedPort `
        -AllowPhysicalStopConfirmation:(-not $Yes)
}

Set-MagicHandyAppLanguages `
    -RepositoryPath $Repo `
    -DataDir $resolvedDataDir `
    -UILocale $selectedUI `
    -ChatLocale $selectedChat
Write-Host (Get-MagicHandyText -Key 'change_applied' -Values @(
    (Get-MagicHandyLanguageName -Locale $selectedUI),
    (Get-MagicHandyLanguageName -Locale $selectedChat)
)) -ForegroundColor Green

if ($null -ne $state) {
    $state.ui_locale = $selectedUI
    $state.chat_locale = $selectedChat
    $state.updated_at = [DateTimeOffset]::UtcNow.ToString('o')
    Write-MagicHandyInstallState -State $state -Path $StatePath
    Write-Host (Get-MagicHandyText -Key 'change_state_saved' -Values @($StatePath)) -ForegroundColor Green
} else {
    Write-Host (Get-MagicHandyText -Key 'change_no_state') -ForegroundColor DarkGray
}

if ($wasRunning -and -not $NoLaunch) {
    Start-MagicHandyApp -RepositoryPath $Repo -DataDir $resolvedDataDir -Port $resolvedPort -NoBrowser
    Write-Host (Get-MagicHandyText -Key 'change_restarted') -ForegroundColor Green
} elseif ($wasRunning) {
    Write-Host (Get-MagicHandyText -Key 'change_restart_manual') -ForegroundColor Yellow
}