#requires -Version 5.1
<#
.SYNOPSIS
Installs an optional local TTS server and configures MagicHandy to use it.

.DESCRIPTION
The core app remains pure Go. This script creates an isolated uv/Python
environment below the MagicHandy data directory, checks out a pinned upstream
source revision, installs its dependencies, downloads the selected model after
consent, and writes module settings through magichandy.exe.

.EXAMPLE
.\scripts\install-tts-module.ps1 -Module faster-qwen3-tts -AutoLaunch

.EXAMPLE
.\scripts\install-tts-module.ps1 -Module chatterbox -Device cpu -AutoLaunch
#>
[CmdletBinding()]
param(
    [ValidateSet('', 'faster-qwen3-tts', 'chatterbox')]
    [string]$Module = '',
    [string]$DataDir = '',
    [string]$InstallRoot = '',
    [string]$ReferenceWav = '',
    [string]$Model = '',
    [string]$Voice = '',
    [string]$Language = 'Auto',
    [ValidateSet('auto', 'cuda', 'cpu')]
    [string]$Device = 'auto',
    [ValidateRange(0, 65535)]
    [int]$Port = 0,
    [switch]$AutoLaunch,
    [switch]$SpeakReplies,
    [switch]$Update,
    [switch]$PlanOnly,
    [switch]$SkipAppConfiguration,
    [switch]$Yes
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$repository = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$supportPath = Join-Path $PSScriptRoot 'installer\InstallerSupport.psm1'
Import-Module $supportPath -Force -DisableNameChecking -ErrorAction Stop

$fasterSource = 'https://github.com/andimarafioti/faster-qwen3-tts.git'
$fasterRevision = 'a70afc0f81f7f5f8801c3227968f1102f43f211c'
$chatterboxSource = 'https://github.com/devnen/Chatterbox-TTS-Server.git'
$chatterboxRevision = '915ae289340e10c6047f27f47e22eae9bf350c32'
$chatterboxEngine = 'git+https://github.com/devnen/chatterbox-v2.git@cc0357396d9c73fc1e6c544ee40bb596020edd09'

function Read-TTSChoice {
    param([string]$Question, [string]$Default)
    $answer = Read-Host "$Question [$Default]"
    if ([string]::IsNullOrWhiteSpace($answer)) {
        return $Default
    }
    return $answer.Trim()
}

function Confirm-TTSAction {
    param([string]$Question)
    if ($Yes) {
        return
    }
    $answer = Read-Host "$Question [y/N]"
    if ($answer -notmatch '^(?i:y|yes)$') {
        throw 'TTS module installation was cancelled.'
    }
}

function Resolve-Uv {
    $command = Get-Command 'uv.exe' -ErrorAction SilentlyContinue
    if ($command) {
        return [string]$command.Source
    }
    foreach ($candidate in @(
        (Join-Path $env:USERPROFILE '.local\bin\uv.exe'),
        (Join-Path $env:LOCALAPPDATA 'Programs\uv\uv.exe'),
        (Join-Path $env:LOCALAPPDATA 'Microsoft\WinGet\Links\uv.exe')
    )) {
        if (Test-Path -LiteralPath $candidate -PathType Leaf) {
            return $candidate
        }
    }
    return $null
}

function Ensure-Uv {
    $uv = Resolve-Uv
    if ($uv) {
        return $uv
    }
    Confirm-TTSAction 'uv is required to create an isolated Python environment. Install uv with WinGet?'
    Invoke-MagicHandyWinGetInstall -ID 'astral-sh.uv' -AssumeYes:$Yes
    $uv = Resolve-Uv
    if (-not $uv) {
        throw 'uv was installed but is not visible yet. Restart PowerShell and rerun this script.'
    }
    return $uv
}

function Initialize-TTSPythonEnvironment {
    param(
        [Parameter(Mandatory = $true)][string]$Uv,
        [Parameter(Mandatory = $true)][string]$Root,
        [Parameter(Mandatory = $true)][string]$PythonVersion
    )

    Invoke-Checked -Executable $Uv -Arguments @('python', 'install', $PythonVersion) -Description "Python $PythonVersion installation"

    $venv = Join-Path $Root '.venv'
    $python = Join-Path $venv 'Scripts\python.exe'
    Assert-MagicHandyChildPath -Root $Root -Candidate $venv
    if (Test-Path -LiteralPath $python -PathType Leaf) {
        $reportedVersion = @(& $python --version 2>&1) -join ' '
        if ($LASTEXITCODE -eq 0 -and $reportedVersion -match "^Python $([regex]::Escape($PythonVersion))\.") {
            Write-Host "Reusing the existing $reportedVersion environment; dependency checks will repair only changed packages."
            return [pscustomobject]@{
                Root = $venv
                Python = $python
                Version = $reportedVersion
            }
        }
        Write-Warning "Replacing the module environment because it does not use required Python $PythonVersion."
        Remove-Item -LiteralPath $venv -Recurse -Force
    }

    Invoke-Checked -Executable $Uv -Arguments @('venv', '--python', $PythonVersion, '--allow-existing', $venv) -Description 'Python environment creation'
    if (-not (Test-Path -LiteralPath $python -PathType Leaf)) {
        throw "The isolated Python executable was not created at '$python'."
    }
    $verifiedVersion = @(& $python --version 2>&1) -join ' '
    if ($LASTEXITCODE -ne 0 -or $verifiedVersion -notmatch "^Python $([regex]::Escape($PythonVersion))\.") {
        throw "The isolated environment did not resolve required Python $PythonVersion (reported '$verifiedVersion')."
    }
    return [pscustomobject]@{
        Root = $venv
        Python = $python
        Version = $verifiedVersion
    }
}

function Get-ChatterboxRequirements {
    param([Parameter(Mandatory = $true)][string]$RuntimeDevice)

    if ($RuntimeDevice -eq 'cpu') {
        return 'requirements.txt'
    }

    $nvidia = Get-Command 'nvidia-smi.exe' -ErrorAction SilentlyContinue
    if (-not $nvidia) {
        if ($RuntimeDevice -eq 'cuda') {
            throw 'Chatterbox CUDA was selected, but nvidia-smi.exe is unavailable. Install or repair the NVIDIA driver, or choose CPU.'
        }
        return 'requirements.txt'
    }

    $capabilityLines = @(& $nvidia.Source --query-gpu=compute_cap --format=csv,noheader 2>$null)
    if ($LASTEXITCODE -eq 0) {
        foreach ($line in $capabilityLines) {
            $capability = 0.0
            if ([double]::TryParse(
                    [string]$line,
                    [System.Globalization.NumberStyles]::Float,
                    [System.Globalization.CultureInfo]::InvariantCulture,
                    [ref]$capability
                ) -and $capability -ge 12.0) {
                return 'requirements-nvidia-cu128.txt'
            }
        }
        return 'requirements-nvidia.txt'
    }

    $gpuNames = @(& $nvidia.Source --query-gpu=name --format=csv,noheader 2>$null)
    if ($LASTEXITCODE -eq 0 -and [bool]($gpuNames | Where-Object { $_ -match '(?i)\bRTX\s+50\d{2}\b|\bBlackwell\b|\bB(?:100|200)\b' })) {
        return 'requirements-nvidia-cu128.txt'
    }

    Write-Warning 'The NVIDIA compute capability could not be detected; using the broadly compatible CUDA 12.1 dependency set.'
    return 'requirements-nvidia.txt'
}

function Invoke-Checked {
    param(
        [Parameter(Mandatory = $true)][string]$Executable,
        [Parameter(Mandatory = $true)][string[]]$Arguments,
        [Parameter(Mandatory = $true)][string]$Description
    )
    & $Executable @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$Description failed (exit $LASTEXITCODE)."
    }
}

function Invoke-HuggingFaceModelDownload {
    param(
        [Parameter(Mandatory = $true)][string]$Executable,
        [Parameter(Mandatory = $true)][string]$Repository,
        [Parameter(Mandatory = $true)][string]$CacheDirectory
    )

    $arguments = @('download', $Repository, '--cache-dir', $CacheDirectory)
    $isWindows = [System.Environment]::OSVersion.Platform -eq [System.PlatformID]::Win32NT
    if ($isWindows) {
        # huggingface_hub probes symlink support lazily. Concurrent first-use
        # workers can observe the optimistic probe value and fail with WinError
        # 1314 before the normal copy fallback is selected.
        $arguments += @('--max-workers', '1')
        Write-Host 'Using serialized model-file finalization for standard Windows accounts.'
    }

    $previousSymlinkWarning = [System.Environment]::GetEnvironmentVariable(
        'HF_HUB_DISABLE_SYMLINKS_WARNING',
        [System.EnvironmentVariableTarget]::Process
    )
    $exitCode = 1
    try {
        if ($isWindows) {
            [System.Environment]::SetEnvironmentVariable(
                'HF_HUB_DISABLE_SYMLINKS_WARNING',
                '1',
                [System.EnvironmentVariableTarget]::Process
            )
        }
        for ($attempt = 1; $attempt -le 3; $attempt++) {
            & $Executable @arguments
            $exitCode = $LASTEXITCODE
            if ($exitCode -eq 0) {
                return
            }
            if ($attempt -lt 3) {
                Write-Warning "Model download attempt $attempt failed (exit $exitCode). Retrying with the resumable cache..."
                Start-Sleep -Seconds (2 * $attempt)
            }
        }
    } finally {
        if ($isWindows) {
            [System.Environment]::SetEnvironmentVariable(
                'HF_HUB_DISABLE_SYMLINKS_WARNING',
                $previousSymlinkWarning,
                [System.EnvironmentVariableTarget]::Process
            )
        }
    }

    throw "Model download failed after 3 attempts (last exit $exitCode). Downloaded files were kept; rerun the installer to resume."
}

function ConvertTo-YamlSingleQuotedScalar {
    param([Parameter(Mandatory = $true)][string]$Value)
    return "'" + $Value.Replace("'", "''") + "'"
}

function Write-TTSModuleState {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][object]$State
    )
    $partial = "$Path.partial-$PID"
    try {
        $encoding = New-Object System.Text.UTF8Encoding($false)
        $json = $State | ConvertTo-Json -Depth 4
        [System.IO.File]::WriteAllText($partial, $json, $encoding)
        Move-Item -LiteralPath $partial -Destination $Path -Force
    } finally {
        if (Test-Path -LiteralPath $partial) {
            Remove-Item -LiteralPath $partial -Force -ErrorAction SilentlyContinue
        }
    }
}

function Sync-PinnedSource {
    param(
        [Parameter(Mandatory = $true)][string]$Git,
        [Parameter(Mandatory = $true)][string]$URL,
        [Parameter(Mandatory = $true)][string]$Revision,
        [Parameter(Mandatory = $true)][string]$Destination,
        [string[]]$InstallerGeneratedPaths = @()
    )
    if (Test-Path -LiteralPath $Destination) {
        if (-not (Test-Path -LiteralPath (Join-Path $Destination '.git') -PathType Container)) {
            throw "The module source path exists but is not a Git checkout: '$Destination'."
        }
    } else {
        $parent = Split-Path -Parent $Destination
        New-Item -ItemType Directory -Path $parent -Force | Out-Null
        Invoke-Checked -Executable $Git -Arguments @('clone', '--filter=blob:none', '--no-checkout', $URL, $Destination) -Description 'Source clone'
    }
    InstallerSupport\Add-MagicHandyGitInfoExclusions -RepositoryPath $Destination -RelativePaths $InstallerGeneratedPaths
    $dirty = @(& $Git -C $Destination status --porcelain)
    if ($LASTEXITCODE -ne 0 -or $dirty.Count -gt 0) {
        throw "The managed module source has local changes. Preserve or remove '$Destination' before updating."
    }
    Invoke-Checked -Executable $Git -Arguments @('-C', $Destination, 'fetch', '--depth', '1', 'origin', $Revision) -Description 'Pinned source fetch'
    Invoke-Checked -Executable $Git -Arguments @('-C', $Destination, 'checkout', '--detach', '--force', 'FETCH_HEAD') -Description 'Pinned source checkout'
    $actual = (& $Git -C $Destination rev-parse HEAD).Trim()
    if ($LASTEXITCODE -ne 0 -or $actual -ne $Revision) {
        throw "Pinned source verification failed. Expected $Revision, got '$actual'."
    }
}

function Write-ChatterboxConfiguration {
    param(
        [Parameter(Mandatory = $true)][string]$Root,
        [Parameter(Mandatory = $true)][string]$Source,
        [Parameter(Mandatory = $true)][int]$ServerPort,
        [Parameter(Mandatory = $true)][string]$RuntimeDevice,
        [Parameter(Mandatory = $true)][string]$VoiceName,
        [string]$SourceWav
    )
    $runtime = Join-Path $Root 'runtime'
    $voices = Join-Path $runtime 'voices'
    New-Item -ItemType Directory -Path $voices -Force | Out-Null
    if (-not [string]::IsNullOrWhiteSpace($SourceWav)) {
        $destination = Join-Path $voices $VoiceName
        Copy-Item -LiteralPath $SourceWav -Destination $destination -Force
    } elseif (-not (Test-Path -LiteralPath (Join-Path $voices $VoiceName) -PathType Leaf)) {
        $bundled = Join-Path $Source "voices\$VoiceName"
        if (-not (Test-Path -LiteralPath $bundled -PathType Leaf)) {
            throw "Chatterbox voice '$VoiceName' was not found. Pass -ReferenceWav to install a voice."
        }
        Copy-Item -LiteralPath $bundled -Destination (Join-Path $voices $VoiceName) -Force
    }
    $cache = (Join-Path $Root 'model-cache').Replace('\', '/')
    $voicesPath = $voices.Replace('\', '/')
    $runtimePath = $runtime.Replace('\', '/')
    $yamlCache = ConvertTo-YamlSingleQuotedScalar $cache
    $yamlVoices = ConvertTo-YamlSingleQuotedScalar $voicesPath
    $yamlRuntimeLog = ConvertTo-YamlSingleQuotedScalar "$runtimePath/logs/tts_server.log"
    $yamlOutput = ConvertTo-YamlSingleQuotedScalar "$runtimePath/outputs"
    $yamlDevice = ConvertTo-YamlSingleQuotedScalar $RuntimeDevice
    $yamlVoice = ConvertTo-YamlSingleQuotedScalar $VoiceName
    $yaml = @"
server:
  host: '127.0.0.1'
  port: $ServerPort
  use_ngrok: false
  use_auth: false
  log_file_path: $yamlRuntimeLog
model:
  repo_id: 'chatterbox-turbo'
tts_engine:
  device: $yamlDevice
  predefined_voices_path: $yamlVoices
  reference_audio_path: $yamlVoices
  default_voice_id: $yamlVoice
paths:
  model_cache: $yamlCache
  output: $yamlOutput
audio_output:
  format: 'wav'
  sample_rate: 24000
  max_reference_duration_sec: 30
  save_to_disk: false
ui:
  title: 'Chatterbox TTS Server'
debug:
  save_intermediate_audio: false
"@
    Set-Content -LiteralPath (Join-Path $runtime 'config.yaml') -Value $yaml -Encoding UTF8
}

Write-Host ''
Write-Host '  __  __             _      _   _                 _       ' -ForegroundColor Cyan
Write-Host ' |  \/  | __ _  __ _(_) ___| | | | __ _ _ __   __| |_   _ ' -ForegroundColor Cyan
Write-Host ' | |\/| |/ _` |/ _` | |/ __| |_| |/ _` | `_ \ / _` | | | |' -ForegroundColor Cyan
Write-Host ' | |  | | (_| | (_| | | (__|  _  | (_| | | | | (_| | |_| |' -ForegroundColor Cyan
Write-Host ' |_|  |_|\__,_|\__, |_|\___|_| |_|\__,_|_| |_|\__,_|\__, |' -ForegroundColor Cyan
Write-Host '               |___/                                  |___/ ' -ForegroundColor Cyan
Write-Host '                   Local TTS module installer' -ForegroundColor DarkGray
Write-Host ''

if ([string]::IsNullOrWhiteSpace($Module)) {
    $choice = Read-TTSChoice -Question 'Choose 1 for Faster Qwen3-TTS (NVIDIA) or 2 for Chatterbox Turbo' -Default '1'
    $Module = if ($choice -eq '2') { 'chatterbox' } else { 'faster-qwen3-tts' }
}

if ([string]::IsNullOrWhiteSpace($DataDir)) {
    $statePath = InstallerSupport\Get-MagicHandyInstallStatePath
    if (Test-Path -LiteralPath $statePath -PathType Leaf) {
        $installState = InstallerSupport\Read-MagicHandyInstallState -Path $statePath
        $DataDir = [string]$installState.data_dir
    } else {
        $applicationData = if (-not [string]::IsNullOrWhiteSpace($env:APPDATA)) {
            $env:APPDATA
        } else {
            [Environment]::GetFolderPath('ApplicationData')
        }
        if ([string]::IsNullOrWhiteSpace($applicationData)) {
            throw 'The MagicHandy app data directory could not be resolved. Pass -DataDir explicitly.'
        }
        $DataDir = Join-Path $applicationData 'MagicHandy'
    }
}
$DataDir = [System.IO.Path]::GetFullPath($DataDir)

$provider = if ($Module -eq 'faster-qwen3-tts') { 'faster_qwen3_tts' } else { 'chatterbox_tts' }
$defaultPort = if ($Module -eq 'faster-qwen3-tts') { 8991 } else { 8992 }
if ($Port -eq 0) {
    $Port = $defaultPort
}
if ([string]::IsNullOrWhiteSpace($InstallRoot)) {
    $folder = if ($Module -eq 'faster-qwen3-tts') { 'faster-qwen3-tts' } else { 'chatterbox-tts' }
    $InstallRoot = Join-Path $DataDir "voice\$folder"
}
$InstallRoot = [System.IO.Path]::GetFullPath($InstallRoot)

if ($Module -eq 'faster-qwen3-tts') {
    if (-not [string]::IsNullOrWhiteSpace($ReferenceWav)) {
        throw 'Configure the Faster Qwen3-TTS reference WAV and transcript in Settings > Voice after installation.'
    }
    if ([string]::IsNullOrWhiteSpace($Model)) {
        $Model = 'Qwen/Qwen3-TTS-12Hz-0.6B-Base'
    }
    if ([string]::IsNullOrWhiteSpace($Voice)) {
        $Voice = 'default'
    }
    if ($Device -eq 'auto') {
        $Device = 'cuda'
    }
    if ($Device -ne 'cuda') {
        throw 'Faster Qwen3-TTS requires an NVIDIA GPU and the CUDA device. Choose Chatterbox for CPU operation.'
    }
} else {
    if ([string]::IsNullOrWhiteSpace($Model)) {
        $Model = 'chatterbox-turbo'
    }
    if ([string]::IsNullOrWhiteSpace($Voice)) {
        $Voice = if ([string]::IsNullOrWhiteSpace($ReferenceWav)) { 'Emily.wav' } else { 'magichandy-reference.wav' }
    }
    $invalidVoiceChars = [System.IO.Path]::GetInvalidFileNameChars()
    if ([System.IO.Path]::GetFileName($Voice) -ne $Voice -or
        [System.IO.Path]::GetExtension($Voice) -ne '.wav' -or
        $Voice.IndexOfAny($invalidVoiceChars) -ge 0) {
        throw 'Chatterbox voice must be a plain .wav file name without path separators or invalid characters.'
    }
}

$sourceURL = if ($Module -eq 'faster-qwen3-tts') { $fasterSource } else { $chatterboxSource }
$sourceRevision = if ($Module -eq 'faster-qwen3-tts') { $fasterRevision } else { $chatterboxRevision }
$modelDisplay = if ($Module -eq 'faster-qwen3-tts') { $Model } else { 'ResembleAI/chatterbox-turbo' }
$license = if ($Module -eq 'faster-qwen3-tts') {
    'faster-qwen3-tts: MIT; Qwen3-TTS model: Apache-2.0'
} else {
    'Chatterbox server and model: MIT'
}

Write-Host "Module:       $Module"
Write-Host "Install root: $InstallRoot"
Write-Host "Source:       $sourceURL@$sourceRevision"
Write-Host "Model:        $modelDisplay"
Write-Host "Device:       $Device"
Write-Host "Python:       $(if ($Module -eq 'chatterbox') { '3.10 (managed by uv)' } else { '3.11 (managed by uv)' })"
Write-Host "Endpoint:     http://127.0.0.1:$Port"
Write-Host "Auto-launch:  $([bool]$AutoLaunch)"
if ($Module -eq 'faster-qwen3-tts') {
    Write-Host 'Reference:    configure later in Settings > Voice'
}
Write-Host "License:      $license"
Write-Host 'Downloads include Python, PyTorch, the model, and transitive packages. Expect several GiB.' -ForegroundColor Yellow

if ($PlanOnly) {
    Write-Host ''
    Write-Host 'Plan only: no dependencies, files, models, processes, or settings were changed.' -ForegroundColor Green
    return
}

if (-not [string]::IsNullOrWhiteSpace($ReferenceWav)) {
    $ReferenceWav = [System.IO.Path]::GetFullPath($ReferenceWav)
    if (-not (Test-Path -LiteralPath $ReferenceWav -PathType Leaf) -or [System.IO.Path]::GetExtension($ReferenceWav) -ne '.wav') {
        throw "Reference WAV is unavailable or is not a .wav file: '$ReferenceWav'."
    }
}

Confirm-TTSAction 'Proceed with the optional TTS module download and installation?'
$git = InstallerSupport\Ensure-MagicHandyGit -AssumeYes:$Yes
$uv = Ensure-Uv
$sourceRoot = Join-Path $InstallRoot 'source'
$installerGeneratedPaths = if ($Module -eq 'faster-qwen3-tts') {
    @('faster_qwen3_tts.egg-info')
} else {
    @()
}
Sync-PinnedSource `
    -Git $git `
    -URL $sourceURL `
    -Revision $sourceRevision `
    -Destination $sourceRoot `
    -InstallerGeneratedPaths $installerGeneratedPaths

$pythonVersion = if ($Module -eq 'chatterbox') { '3.10' } else { '3.11' }
$pythonEnvironment = Initialize-TTSPythonEnvironment -Uv $uv -Root $InstallRoot -PythonVersion $pythonVersion
$venv = [string]$pythonEnvironment.Root
$python = [string]$pythonEnvironment.Python

if ($Module -eq 'faster-qwen3-tts') {
    Invoke-Checked -Executable $uv -Arguments @('pip', 'install', '--python', $python, '--torch-backend', 'cu128', '--editable', "$sourceRoot[demo]") -Description 'Faster Qwen3-TTS dependency installation'
} else {
    $requirements = Get-ChatterboxRequirements -RuntimeDevice $Device
    $requirementsPath = Join-Path $sourceRoot $requirements
    if (-not (Test-Path -LiteralPath $requirementsPath -PathType Leaf)) {
        throw "The pinned Chatterbox dependency set is unavailable: '$requirementsPath'."
    }
    Write-Host "Chatterbox dependency set: $requirements"
    Invoke-Checked -Executable $uv -Arguments @('pip', 'install', '--python', $python, '-r', $requirementsPath) -Description 'Chatterbox dependency installation'
    Invoke-Checked -Executable $uv -Arguments @('pip', 'install', '--python', $python, '--no-deps', $chatterboxEngine, 's3tokenizer==0.3.0', 'onnx==1.16.0') -Description 'Pinned Chatterbox engine installation'
}

$hf = Join-Path $venv 'Scripts\hf.exe'
if (-not (Test-Path -LiteralPath $hf -PathType Leaf)) {
    Invoke-Checked -Executable $uv -Arguments @('pip', 'install', '--python', $python, 'huggingface-hub>=0.36.0,<1.0') -Description 'Hugging Face client installation'
}
if (-not (Test-Path -LiteralPath $hf -PathType Leaf)) {
    throw "The Hugging Face client was not installed at '$hf'."
}
$modelRepo = if ($Module -eq 'faster-qwen3-tts') { $Model } else { 'ResembleAI/chatterbox-turbo' }
$modelCache = Join-Path $InstallRoot 'model-cache\hub'
Invoke-HuggingFaceModelDownload -Executable $hf -Repository $modelRepo -CacheDirectory $modelCache

$healthPath = '/health'
if ($Module -eq 'faster-qwen3-tts') {
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'tts\faster-qwen-server.py') -Destination (Join-Path $InstallRoot 'magichandy-faster-qwen-server.py') -Force
} else {
    Write-ChatterboxConfiguration -Root $InstallRoot -Source $sourceRoot -ServerPort $Port -RuntimeDevice $Device -VoiceName $Voice -SourceWav $ReferenceWav
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'tts\chatterbox-server.py') -Destination (Join-Path $InstallRoot 'magichandy-chatterbox-server.py') -Force
    $healthPath = '/api/model-info'
}

$moduleState = [ordered]@{
    schema_version = 2
    module = $Module
    provider = $provider
    install_root = $InstallRoot
    data_dir = $DataDir
    source_url = $sourceURL
    source_revision = $sourceRevision
    model = $Model
    voice = $Voice
    language = $Language
    device = $Device
    port = $Port
    auto_launch = [bool]$AutoLaunch
    speak_replies = [bool]$SpeakReplies
}

if ($SkipAppConfiguration) {
    Write-TTSModuleState -Path (Join-Path $InstallRoot 'module-state.json') -State $moduleState
    Write-Host 'Module files are ready. The running MagicHandy app owns the settings update.' -ForegroundColor Green
    return
}

$exe = Join-Path $repository 'magichandy.exe'
if (-not (Test-Path -LiteralPath $exe -PathType Leaf)) {
    throw "MagicHandy executable not found at '$exe'. Run install.ps1 or update.ps1 before installing a TTS module."
}

$wasRunning = InstallerSupport\Test-MagicHandyAppRunning -RepositoryPath $repository
$statePath = InstallerSupport\Get-MagicHandyInstallStatePath
$appPort = 49717
if (Test-Path -LiteralPath $statePath -PathType Leaf) {
    $appState = InstallerSupport\Read-MagicHandyInstallState -Path $statePath
    $appPort = [int]$appState.port
}
if ($wasRunning) {
    InstallerSupport\Stop-MagicHandyAppForRebuild -RepositoryPath $repository -Port $appPort -AllowPhysicalStopConfirmation
}

try {
    $baseURL = "http://127.0.0.1:$Port"
    $settingsArguments = @(
        '-data-dir', $DataDir,
        '-configure-tts-module', $provider,
        '-tts-module-root', $InstallRoot,
        '-tts-base-url', $baseURL,
        '-tts-model', $Model,
        '-tts-voice', $Voice,
        '-tts-response-format', 'wav',
        '-tts-health-path', $healthPath,
        '-tts-language', $Language,
        '-tts-device', $Device,
        '-tts-server-port', [string]$Port
    )
    if ($Module -eq 'chatterbox' -and -not [string]::IsNullOrWhiteSpace($ReferenceWav)) {
        $settingsArguments += @('-tts-reference-wav', $ReferenceWav)
    }
    if ($AutoLaunch) {
        $settingsArguments += '-tts-auto-launch'
    }
    if ($SpeakReplies) {
        $settingsArguments += '-tts-speak-replies'
    }
    Invoke-Checked -Executable $exe -Arguments $settingsArguments -Description 'MagicHandy TTS settings update'
    Write-TTSModuleState -Path (Join-Path $InstallRoot 'module-state.json') -State $moduleState
} finally {
    if ($wasRunning) {
        InstallerSupport\Start-MagicHandyApp -RepositoryPath $repository -DataDir $DataDir -Port $appPort
    }
}

Write-Host ''
Write-Host '  +------------------------------------------+' -ForegroundColor Green
Write-Host '  | Local voice module runtime installed.    |' -ForegroundColor Green
Write-Host '  +------------------------------------------+' -ForegroundColor Green
Write-Host "Provider: $provider"
Write-Host "Settings: $DataDir"
if ($Module -eq 'faster-qwen3-tts') {
    Write-Host 'Next: open Settings > Voice and add a reference WAV with its exact transcript.' -ForegroundColor Yellow
}
if (-not $AutoLaunch) {
    Write-Host 'Auto-launch is off. Start the server yourself before loading the TTS worker.' -ForegroundColor Yellow
}
