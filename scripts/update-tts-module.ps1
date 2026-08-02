#requires -Version 5.1
<#
.SYNOPSIS
Updates an installed scripted TTS module while preserving its prior choices.
#>
[CmdletBinding()]
param(
    [string]$InstallRoot = '',
    [switch]$ModifyChoices,
    [ValidateSet('', 'auto', 'cuda', 'cpu')]
    [string]$Device = '',
    [switch]$AutoLaunch,
    [switch]$NoAutoLaunch,
    [switch]$ApplyInstallerChoices,
    [switch]$CheckOnly,
    [switch]$PlanOnly,
    [switch]$Yes
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if ($AutoLaunch -and $NoAutoLaunch) {
    throw 'AutoLaunch and NoAutoLaunch cannot be combined.'
}
if ($ModifyChoices -and $ApplyInstallerChoices) {
    throw 'ModifyChoices and ApplyInstallerChoices cannot be combined.'
}
if ($CheckOnly -and ($ModifyChoices -or $ApplyInstallerChoices -or $AutoLaunch -or $NoAutoLaunch -or
        -not [string]::IsNullOrWhiteSpace($Device) -or $PlanOnly)) {
    throw 'CheckOnly cannot be combined with update or override choices.'
}

$repository = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$supportPath = Join-Path $PSScriptRoot 'installer\InstallerSupport.psm1'
Import-Module $supportPath -Force -DisableNameChecking -ErrorAction Stop

function Read-TTSModuleState {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$ExpectedRoot
    )
    try {
        $moduleState = [System.IO.File]::ReadAllText($Path, [System.Text.Encoding]::UTF8) | ConvertFrom-Json
    } catch {
        throw "TTS module state '$Path' is not valid JSON: $($_.Exception.Message)"
    }
    $required = @(
        'schema_version', 'module', 'provider', 'install_root', 'data_dir',
        'source_url', 'source_revision', 'model', 'voice', 'device', 'port',
        'auto_launch', 'speak_replies'
    )
    foreach ($name in $required) {
        if ($moduleState.PSObject.Properties.Name -notcontains $name) {
            throw "TTS module state '$Path' is missing '$name'."
        }
    }
    if ($moduleState.schema_version -isnot [int] -or [int]$moduleState.schema_version -notin @(1, 2)) {
        throw "Unsupported TTS module state schema '$($moduleState.schema_version)'."
    }
    if ([string]$moduleState.module -notin @('faster-qwen3-tts', 'chatterbox')) {
        throw "TTS module state '$Path' has an unsupported module."
    }
    $expectedProvider = if ([string]$moduleState.module -eq 'faster-qwen3-tts') { 'faster_qwen3_tts' } else { 'chatterbox_tts' }
    if ([string]$moduleState.provider -ne $expectedProvider) {
        throw "TTS module state '$Path' does not match its provider."
    }
    foreach ($name in @('install_root', 'data_dir')) {
        $value = [string]$moduleState.$name
        if ([string]::IsNullOrWhiteSpace($value) -or -not [System.IO.Path]::IsPathRooted($value)) {
            throw "TTS module state '$Path' field '$name' must be an absolute path."
        }
    }
    $savedRoot = [System.IO.Path]::GetFullPath([string]$moduleState.install_root).TrimEnd('\')
    if (-not $savedRoot.Equals($ExpectedRoot.TrimEnd('\'), [StringComparison]::OrdinalIgnoreCase)) {
        throw "TTS module state '$Path' belongs to '$savedRoot', not '$ExpectedRoot'."
    }
    if ($moduleState.port -isnot [int] -or [int]$moduleState.port -lt 1 -or [int]$moduleState.port -gt 65535) {
        throw "TTS module state '$Path' has an invalid port."
    }
    foreach ($name in @('auto_launch', 'speak_replies')) {
        if ($moduleState.$name -isnot [bool]) {
            throw "TTS module state '$Path' field '$name' must be boolean."
        }
    }
    foreach ($name in @('model', 'voice', 'device')) {
        if ($moduleState.$name -isnot [string]) {
            throw "TTS module state '$Path' field '$name' must be text."
        }
    }
    return $moduleState
}

function Get-InstalledTTSPythonVersion {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$PythonVersion
    )

    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw "Managed TTS Python environment requires repair: '$Path' is missing."
    }

    $previousPreference = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    $exitCode = -1
    try {
        $reported = @(& $Path --version 2>&1) -join ' '
        $exitCode = $LASTEXITCODE
    } catch {
        throw "Managed TTS Python environment requires repair: '$Path' could not start: $($_.Exception.Message)"
    } finally {
        $ErrorActionPreference = $previousPreference
    }
    if ($exitCode -ne 0 -or $reported -notmatch "^Python $([regex]::Escape($PythonVersion))\.") {
        throw "Managed TTS Python environment requires repair: '$Path' did not report Python $PythonVersion (exit $exitCode; reported '$reported')."
    }
    return $reported
}

function Assert-InstalledTTSPythonEnvironment {
    param(
        [Parameter(Mandatory = $true)][string]$Root,
        [Parameter(Mandatory = $true)][string]$Module
    )

    $pythonVersion = if ($Module -eq 'chatterbox') { '3.10' } else { '3.11' }
    $managedRoot = [System.IO.Path]::GetFullPath((Join-Path $Root 'managed-python'))
    $venv = Join-Path $Root '.venv'
    $python = Join-Path $venv 'Scripts\python.exe'
    $configPath = Join-Path $venv 'pyvenv.cfg'
    if (-not (Test-Path -LiteralPath $configPath -PathType Leaf)) {
        throw "Managed TTS Python environment requires repair: '$configPath' is missing."
    }

    try {
        $config = [System.IO.File]::ReadAllText($configPath)
        $homeMatch = [regex]::Match($config, '(?im)^home\s*=\s*(?<path>[^\r\n]+?)\s*$')
        if (-not $homeMatch.Success -or
            $config -notmatch '(?im)^include-system-site-packages\s*=\s*false\s*$') {
            throw 'pyvenv.cfg does not describe an isolated Python environment.'
        }
        $pythonHome = [System.IO.Path]::GetFullPath($homeMatch.Groups['path'].Value.Trim())
        $managedPrefix = $managedRoot.TrimEnd('\') + '\'
        $homeName = Split-Path -Leaf $pythonHome
        if (-not $pythonHome.StartsWith($managedPrefix, [StringComparison]::OrdinalIgnoreCase) -or
            $homeName -notmatch "^cpython-$([regex]::Escape($pythonVersion))\.\d+-windows-x86_64-none$") {
            throw "pyvenv.cfg does not use an app-owned, patch-specific Python $pythonVersion home."
        }
    } catch {
        throw "Managed TTS Python environment requires repair: $($_.Exception.Message)"
    }

    $basePython = Join-Path $pythonHome 'python.exe'
    Get-InstalledTTSPythonVersion -Path $basePython -PythonVersion $pythonVersion | Out-Null
    return Get-InstalledTTSPythonVersion -Path $python -PythonVersion $pythonVersion
}

if ([string]::IsNullOrWhiteSpace($InstallRoot)) {
    $statePath = InstallerSupport\Get-MagicHandyInstallStatePath
    if (-not (Test-Path -LiteralPath $statePath -PathType Leaf)) {
        throw 'MagicHandy install state was not found. Pass -InstallRoot explicitly.'
    }
    $installState = InstallerSupport\Read-MagicHandyInstallState -Path $statePath
    $voiceRoot = Join-Path ([string]$installState.data_dir) 'voice'
    $candidates = @(
        (Join-Path $voiceRoot 'faster-qwen3-tts'),
        (Join-Path $voiceRoot 'chatterbox-tts')
    ) | Where-Object { Test-Path -LiteralPath (Join-Path $_ 'module-state.json') -PathType Leaf }
    if ($candidates.Count -ne 1) {
        throw 'Could not identify one installed TTS module. Pass -InstallRoot explicitly.'
    }
    $InstallRoot = $candidates[0]
}
$InstallRoot = [System.IO.Path]::GetFullPath($InstallRoot)
$moduleStatePath = Join-Path $InstallRoot 'module-state.json'
if (-not (Test-Path -LiteralPath $moduleStatePath -PathType Leaf)) {
    throw "Module state was not found at '$moduleStatePath'. Run install-tts-module.ps1 first."
}
$state = Read-TTSModuleState -Path $moduleStatePath -ExpectedRoot $InstallRoot

Write-Host ''
Write-Host 'MagicHandy local TTS updater' -ForegroundColor Cyan
Write-Host "Module:      $($state.module)"
Write-Host "Model:       $($state.model)"
Write-Host "Device:      $($state.device)"
Write-Host "Port:        $($state.port)"
Write-Host "Auto-launch: $([bool]$state.auto_launch)"

if ($CheckOnly) {
    $pythonVersion = Assert-InstalledTTSPythonEnvironment -Root $InstallRoot -Module ([string]$state.module)
    Write-Host "Module state verified. Managed $pythonVersion environment verified." -ForegroundColor Green
    return
}

$hasInstallerOverrides = $ApplyInstallerChoices -or $AutoLaunch -or $NoAutoLaunch -or
    -not [string]::IsNullOrWhiteSpace($Device)
if (-not $ModifyChoices -and -not $hasInstallerOverrides -and -not $Yes -and -not $PlanOnly) {
    $answer = Read-Host 'Modify the previous module, model, device, port, or auto-launch choices? [y/N]'
    $ModifyChoices = $answer -match '^(?i:y|yes)$'
}

$installer = Join-Path $PSScriptRoot 'install-tts-module.ps1'
if ($ModifyChoices) {
    Write-Host 'The installer will prompt for new choices and use that module type''s normal data folder.' -ForegroundColor DarkGray
    & $installer -DataDir ([string]$state.data_dir) -Update -PlanOnly:$PlanOnly -Yes:$Yes
    if (-not $?) {
        throw 'TTS module installer failed while applying modified choices.'
    }
    return
}

$arguments = @{
    Module = [string]$state.module
    DataDir = [string]$state.data_dir
    InstallRoot = $InstallRoot
    Model = [string]$state.model
    Voice = [string]$state.voice
    Language = if ($state.PSObject.Properties.Name -notcontains 'language' -or [string]::IsNullOrWhiteSpace([string]$state.language)) { 'Auto' } else { [string]$state.language }
    Device = if ([string]::IsNullOrWhiteSpace($Device)) { [string]$state.device } else { $Device }
    Port = [int]$state.port
    AutoLaunch = if ($AutoLaunch) {
        $true
    } elseif ($NoAutoLaunch) {
        $false
    } else {
        [bool]$state.auto_launch
    }
    SpeakReplies = [bool]$state.speak_replies
    Update = $true
    PlanOnly = [bool]$PlanOnly
    Yes = [bool]$Yes
}
& $installer @arguments
if (-not $?) {
    throw 'TTS module update failed.'
}
