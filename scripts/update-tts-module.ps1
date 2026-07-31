#requires -Version 5.1
<#
.SYNOPSIS
Updates an installed scripted TTS module while preserving its prior choices.
#>
[CmdletBinding()]
param(
    [string]$InstallRoot = '',
    [switch]$ModifyChoices,
    [switch]$PlanOnly,
    [switch]$Yes
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$repository = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$supportPath = Join-Path $PSScriptRoot 'installer\InstallerSupport.psm1'
Import-Module $supportPath -Force

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
        'source_url', 'source_revision', 'model', 'voice', 'reference_wav',
        'reference_transcript', 'device', 'port', 'auto_launch', 'speak_replies'
    )
    foreach ($name in $required) {
        if ($moduleState.PSObject.Properties.Name -notcontains $name) {
            throw "TTS module state '$Path' is missing '$name'."
        }
    }
    if ($moduleState.schema_version -isnot [int] -or [int]$moduleState.schema_version -ne 1) {
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
    foreach ($name in @('model', 'voice', 'reference_wav', 'reference_transcript', 'device')) {
        if ($moduleState.$name -isnot [string]) {
            throw "TTS module state '$Path' field '$name' must be text."
        }
    }
    return $moduleState
}

if ([string]::IsNullOrWhiteSpace($InstallRoot)) {
    $statePath = Get-MagicHandyInstallStatePath
    if (-not (Test-Path -LiteralPath $statePath -PathType Leaf)) {
        throw 'MagicHandy install state was not found. Pass -InstallRoot explicitly.'
    }
    $installState = Read-MagicHandyInstallState -Path $statePath
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

if (-not $ModifyChoices -and -not $Yes -and -not $PlanOnly) {
    $answer = Read-Host 'Modify the previous module, model, device, port, reference, or auto-launch choices? [y/N]'
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
    ReferenceWav = [string]$state.reference_wav
    ReferenceTranscript = [string]$state.reference_transcript
    Model = [string]$state.model
    Voice = [string]$state.voice
    Language = if ($state.PSObject.Properties.Name -notcontains 'language' -or [string]::IsNullOrWhiteSpace([string]$state.language)) { 'Auto' } else { [string]$state.language }
    Device = [string]$state.device
    Port = [int]$state.port
    AutoLaunch = [bool]$state.auto_launch
    SpeakReplies = [bool]$state.speak_replies
    Update = $true
    PlanOnly = [bool]$PlanOnly
    Yes = [bool]$Yes
}
& $installer @arguments
if (-not $?) {
    throw 'TTS module update failed.'
}
