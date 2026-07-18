<#
.SYNOPSIS
    Builds and configures MagicHandy on a 64-bit Windows machine.

.DESCRIPTION
    The installer can start on a machine without Go, Git, CMake, a C++ compiler,
    Rust, LLVM/libclang, CUDA, or Ollama. Missing selected dependencies are installed with WinGet
    after explicit consent, then verified before the build continues. If WinGet
    itself is unavailable, the script offers the official Microsoft repair path.

    The core app and all first-party Go voice adapters are built with CGO
    disabled. Managed llama.cpp, Ollama, and the checksum-verified Parakeet
    runner/model remain explicit choices. Selecting managed llama.cpp also builds
    MagicHandy's persistent NeuTTS runner with the selected CPU or CUDA backend
    and installs its verified decoder, Air Q4 backbone, and local
    WAV-to-reference encoder. Skipping managed llama.cpp skips NeuTTS. Users
    supply a reference WAV and its exact transcript; the app generates the codes
    without Python. No model is downloaded at app startup.

    Non-secret installation choices are stored under LocalAppData so update.ps1
    can preserve or revise them. API keys and the Handy connection key are never
    written to installer state.

.PARAMETER Port
    Local HTTP port. Default: 49717.

.PARAMETER DataDir
    Settings/model/data directory. The unattended default is the Windows profile
    data directory. Interactive setup can instead choose a portable .\data folder.

.PARAMETER LlamaBackend
    Managed llama.cpp backend: auto, cpu, or cuda. Auto selects CUDA only when an
    NVIDIA GPU is detected and the user accepts installing a missing CUDA Toolkit.

.PARAMETER SkipLlamaBuild
    Skip the app-owned llama.cpp source build and the coupled NeuTTS runtime/model
    installation, then ensure Ollama is available.

.PARAMETER OllamaModel
    Optional model name to ensure with Ollama. Blank leaves its model library
    unchanged.

.PARAMETER SkipParakeet
    Do not install the optional 644 MiB Parakeet ASR model and CPU runner.

.PARAMETER NoLauncher
    Do not create Start-MagicHandy.ps1.

.PARAMETER Yes
    Accept the documented defaults and third-party package/license prompts. This
    installs the complete selected source-build toolchain and the coupled NeuTTS
    runtime/model assets without stopping for input.

.PARAMETER NoLaunch
    Build and configure without starting the app.

.PARAMETER StatePath
    Override the installer-state path. Intended for testing or managed installs.

.PARAMETER PlanOnly
    Print the selected provisioning plan without installing, building, saving
    state, or launching.

.EXAMPLE
    .\install.ps1

.EXAMPLE
    .\install.ps1 -Yes -LlamaBackend cuda -NoLaunch
    Provision the CUDA source-build toolchain, managed llama.cpp, NeuTTS, Ollama,
    Parakeet, and all app/voice adapter binaries without launching.

.EXAMPLE
    .\install.ps1 -Yes -SkipLlamaBuild -NoLaunch
    Use Ollama instead of storing managed llama.cpp; NeuTTS is also skipped.
#>
#Requires -Version 5.1
[CmdletBinding()]
param(
    [ValidateRange(1, 65535)]
    [int]$Port = 49717,
    [string]$DataDir,
    [ValidateSet('auto', 'cpu', 'cuda')]
    [string]$LlamaBackend = 'auto',
    [switch]$SkipLlamaBuild,
    [string]$OllamaModel,
    [switch]$SkipParakeet,
    [switch]$NoLauncher,
    [switch]$Yes,
    [switch]$NoLaunch,
    [string]$StatePath,
    [switch]$PlanOnly,

    # update.ps1 uses these mutually exclusive modes.
    [switch]$UseSavedChoices,
    [switch]$Reconfigure,
    [switch]$UpdateRun
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$Repo = Split-Path -Parent $MyInvocation.MyCommand.Path
$support = Join-Path $Repo 'scripts\installer\InstallerSupport.psm1'
if (-not (Test-Path -LiteralPath $support)) {
    throw "Installer support module not found at '$support'."
}
Import-Module $support -Force -DisableNameChecking

if (-not (Test-Path -LiteralPath (Join-Path $Repo 'cmd\magichandy'))) {
    throw "This script must run from the MagicHandy source folder."
}
if ($UseSavedChoices -and $Reconfigure) {
    throw 'UseSavedChoices and Reconfigure cannot be combined.'
}
if ($Reconfigure -and $Yes) {
    throw 'Reconfigure is interactive and cannot be combined with Yes.'
}
if (-not $StatePath) {
    $StatePath = Get-MagicHandyInstallStatePath
}
$StatePath = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($StatePath)
Set-Location $Repo

function Get-ProfileDataDir {
    if (-not [string]::IsNullOrWhiteSpace($env:APPDATA)) {
        return Join-Path $env:APPDATA 'MagicHandy'
    }
    return Join-Path ([Environment]::GetFolderPath('ApplicationData')) 'MagicHandy'
}

function Resolve-InitialDataDir {
    if (-not [string]::IsNullOrWhiteSpace($DataDir)) {
        return [System.IO.Path]::GetFullPath($DataDir)
    }
    if ($Yes) {
        return [System.IO.Path]::GetFullPath((Get-ProfileDataDir))
    }
    $portable = Confirm-MagicHandyChoice -Question 'Store app data in a portable .\data folder? (No uses your Windows profile)' -Default $false
    if ($portable) {
        return Join-Path $Repo 'data'
    }
    return [System.IO.Path]::GetFullPath((Get-ProfileDataDir))
}

function Resolve-InitialBackend([bool]$BuildManaged) {
    if (-not $BuildManaged) {
        return 'cpu'
    }
    if ($LlamaBackend -in @('cpu', 'cuda')) {
        return $LlamaBackend
    }
    if (-not (Resolve-MagicHandyExecutable -Name 'nvidia-smi')) {
        Write-Host 'No NVIDIA GPU was detected; managed llama.cpp will use the CPU backend.'
        return 'cpu'
    }
    if (Resolve-MagicHandyExecutable -Name 'nvcc') {
        Write-Host 'NVIDIA GPU and CUDA Toolkit detected; managed llama.cpp will use CUDA.' -ForegroundColor Green
        return 'cuda'
    }
    Write-Host 'An NVIDIA GPU was detected, but the CUDA Toolkit compiler is not installed.'
    Write-Host 'CUDA is proprietary and uses several GB. CPU mode avoids that dependency.' -ForegroundColor DarkGray
    if (Confirm-MagicHandyChoice -Question 'Select CUDA and install the Toolkit?' -Default $false -AssumeYes:$Yes) {
        return 'cuda'
    }
    return 'cpu'
}

function New-FreshConfiguration {
    $resolvedDataDir = Resolve-InitialDataDir
    $setupLLM = if ($Yes) {
        $true
    } else {
        Confirm-MagicHandyChoice -Question 'Set up local LLM providers for chat?' -Default $true
    }

    $buildManaged = $false
    $backend = 'cpu'
    $ensureOllama = $false
    $model = if ($null -eq $OllamaModel) { '' } else { $OllamaModel.Trim() }
    if ($setupLLM) {
        $buildManaged = if ($SkipLlamaBuild) {
            $false
        } elseif ($Yes) {
            $true
        } else {
            Write-Host ''
            Write-Host 'Managed llama.cpp gives MagicHandy direct control over runner version, startup, loading, and diagnostics.'
            Write-Host 'This choice also builds NeuTTS with its llama.cpp binding and local reference encoder, installing about 1.9 GiB of voice assets.'
            Write-Host 'Skipping managed llama.cpp also skips NeuTTS. Ollama saves that source-build and voice-runtime space and remains fully supported.' -ForegroundColor DarkGray
            Confirm-MagicHandyChoice -Question 'Build managed llama.cpp and install NeuTTS?' -Default $true
        }
        $backend = Resolve-InitialBackend -BuildManaged $buildManaged

        if (-not $buildManaged) {
            $ensureOllama = $true
        } elseif ($Yes) {
            $ensureOllama = $true
        } else {
            $ollamaDefault = [bool](Resolve-MagicHandyExecutable -Name 'ollama')
            $ensureOllama = Confirm-MagicHandyChoice -Question 'Install or keep Ollama as an additional provider?' -Default $ollamaDefault
        }
        if ($ensureOllama -and -not $Yes -and [string]::IsNullOrWhiteSpace($model)) {
            $model = Read-MagicHandyValue -Question 'Optional Ollama model to ensure now (blank leaves models unchanged)'
        }
    }

    $parakeet = if ($SkipParakeet) {
        $false
    } elseif ($Yes) {
        $true
    } else {
        Write-Host ''
        Write-Host 'Parakeet adds private offline speech recognition: 1.4 MB CPU runner plus a 644 MiB CC-BY-4.0 model.'
        Confirm-MagicHandyChoice -Question 'Install managed Parakeet speech input?' -Default $false
    }
    $launcher = if ($NoLauncher) {
        $false
    } elseif ($Yes) {
        $true
    } else {
        Confirm-MagicHandyChoice -Question 'Create Start-MagicHandy.ps1?' -Default $true
    }

    return New-MagicHandyInstallState `
        -RepositoryPath $Repo `
        -DataDir $resolvedDataDir `
        -Port $Port `
        -SetupLLM $setupLLM `
        -BuildManagedLlama $buildManaged `
        -LlamaBackend $backend `
        -EnsureOllama $ensureOllama `
        -OllamaModel $model `
        -InstallParakeet $parakeet `
        -CreateLauncher $launcher
}

function Read-ValidPort([int]$Default) {
    while ($true) {
        $raw = Read-MagicHandyValue -Question 'Local HTTP port' -Default ([string]$Default)
        $parsed = 0
        if ([int]::TryParse($raw, [ref]$parsed) -and $parsed -ge 1 -and $parsed -le 65535) {
            return $parsed
        }
        Write-Warning 'Enter a port from 1 through 65535.'
    }
}

function Read-ReconfiguredState([object]$Existing) {
    Write-InstallerHeading 'Modify installation choices'
    $newDataDir = Read-MagicHandyValue -Question 'Data directory' -Default ([string]$Existing.data_dir)
    $newDataDir = [System.IO.Path]::GetFullPath($newDataDir)
    $newPort = Read-ValidPort -Default ([int]$Existing.port)
    $setupLLM = Confirm-MagicHandyChoice -Question 'Set up local LLM providers for chat?' -Default ([bool]$Existing.setup_llm)

    $buildManaged = $false
    $backend = 'cpu'
    $ensureOllama = $false
    $model = ''
    if ($setupLLM) {
        Write-Host 'NeuTTS is installed with managed llama.cpp and skipped when managed llama.cpp is not selected.' -ForegroundColor DarkGray
        $buildManaged = Confirm-MagicHandyChoice -Question 'Build or keep managed llama.cpp and NeuTTS?' -Default ([bool]$Existing.build_managed_llama)
        if ($buildManaged) {
            $backendDefault = if ([string]$Existing.llama_backend -eq 'cuda') { 'cuda' } else { 'cpu' }
            $backend = Read-MagicHandyBackend -Default $backendDefault
        }
        $ollamaDefault = if (-not $buildManaged) { $true } else { [bool]$Existing.ensure_ollama }
        $ensureOllama = Confirm-MagicHandyChoice -Question 'Install or keep Ollama available?' -Default $ollamaDefault
        if (-not $buildManaged -and -not $ensureOllama) {
            Write-Host 'Ollama remains enabled because no managed llama.cpp runtime was selected.' -ForegroundColor Yellow
            $ensureOllama = $true
        }
        if ($ensureOllama) {
            $model = Read-MagicHandyOptionalValue -Question 'Ollama model to ensure' -Default ([string]$Existing.ollama_model)
        }
    }
    $parakeet = Confirm-MagicHandyChoice -Question 'Install or keep managed Parakeet speech input?' -Default ([bool]$Existing.install_parakeet)
    $launcher = Confirm-MagicHandyChoice -Question 'Create or refresh Start-MagicHandy.ps1?' -Default ([bool]$Existing.create_launcher)

    return New-MagicHandyInstallState `
        -RepositoryPath $Repo `
        -DataDir $newDataDir `
        -Port $newPort `
        -SetupLLM $setupLLM `
        -BuildManagedLlama $buildManaged `
        -LlamaBackend $backend `
        -EnsureOllama $ensureOllama `
        -OllamaModel $model `
        -InstallParakeet $parakeet `
        -CreateLauncher $launcher `
        -InstalledAt ([string]$Existing.installed_at)
}

function Copy-SavedState([object]$Existing) {
    return New-MagicHandyInstallState `
        -RepositoryPath $Repo `
        -DataDir ([string]$Existing.data_dir) `
        -Port ([int]$Existing.port) `
        -SetupLLM ([bool]$Existing.setup_llm) `
        -BuildManagedLlama ([bool]$Existing.build_managed_llama) `
        -LlamaBackend ([string]$Existing.llama_backend) `
        -EnsureOllama ([bool]$Existing.ensure_ollama) `
        -OllamaModel ([string]$Existing.ollama_model) `
        -InstallParakeet ([bool]$Existing.install_parakeet) `
        -CreateLauncher ([bool]$Existing.create_launcher) `
        -InstalledAt ([string]$Existing.installed_at)
}

if (-not $UpdateRun) {
    Write-MagicHandyBanner -Operation Install
}

$existing = $null
$state = if ($UseSavedChoices -or $Reconfigure) {
    $existing = Read-MagicHandyInstallState -Path $StatePath
    if ($Reconfigure) {
        Read-ReconfiguredState -Existing $existing
    } else {
        Write-InstallerHeading 'Preserved installation choices'
        Show-MagicHandyInstallState -State $existing
        Copy-SavedState -Existing $existing
    }
} else {
    New-FreshConfiguration
}

Write-InstallerHeading 'Selected installation'
Show-MagicHandyInstallState -State $state
$runningPort = if ($null -ne $existing) { [int]$existing.port } else { [int]$state.port }
Invoke-MagicHandyProvision -State $state -RepositoryPath $Repo -RunningPort $runningPort -AssumeYes:$Yes -PlanOnly:$PlanOnly

if ($PlanOnly) {
    Write-Host ''
    Write-Host 'Plan complete. No files, packages, state, or processes were changed.' -ForegroundColor Green
    if (-not $UpdateRun) {
        Write-MagicHandyCompletionArt -Operation InstallPlan
    }
    return
}

Write-MagicHandyInstallState -State $state -Path $StatePath
Write-Host "Saved non-secret installer choices to $StatePath" -ForegroundColor Green

$launch = -not $NoLaunch -and ($Yes -or (Confirm-MagicHandyChoice -Question 'Start MagicHandy now?' -Default $true))
if ($launch) {
    Start-MagicHandyApp -RepositoryPath $Repo -DataDir ([string]$state.data_dir) -Port ([int]$state.port)
}

if (-not $UpdateRun) {
    Write-MagicHandyCompletionArt -Operation Install
}
