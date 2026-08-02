<#
.SYNOPSIS
    Builds and configures MagicHandy on a 64-bit Windows machine.

.DESCRIPTION
    The default interactive path bootstraps only the Go toolchain needed to
    build the core, then opens MagicHandy's setup wizard. Device, LLM, model,
    and voice choices live in that GUI. This keeps one decision tree and avoids
    installing compilers, CUDA, models, or Python environments before the user
    selects them.

    Explicit command-line feature flags and -Yes retain the unattended source
    provisioning path. That path can start on a machine without Go, Git, CMake,
    a C++ compiler, CUDA, or Ollama and installs selected dependencies with
    WinGet after consent.

    The core app and all first-party Go voice adapters are built with CGO
    disabled. Managed llama.cpp, Ollama, and the checksum-verified Parakeet
    runner/model and local TTS remain explicit choices. When local TTS is
    selected, the installer bootstraps uv, a module-compatible Python runtime,
    PyTorch, and the speech model in an isolated data-directory environment.
    These never become core dependencies, and no model is downloaded at app
    startup.

    Non-secret bootstrap and unattended choices are stored under LocalAppData.
    update.ps1 preserves that compatibility state but does not replay optional
    installs; the app database owns interactive product choices. API keys and
    the Handy connection key are never written to installer state.

.PARAMETER Port
    Local HTTP port. Default: 49717.

.PARAMETER DataDir
    Settings/model/data directory. The default is the Windows profile data
    directory. Pass an explicit path for a portable or managed layout.

.PARAMETER UILanguage
    Bootstrap and app UI locale: en, es, pt-BR, zh-Hans, or ja. The normal path
    starts with English and lets the user choose in the GUI. Unattended default: en.

.PARAMETER ChatLanguage
    Built-in chat reply locale: en, es, pt-BR, zh-Hans, or ja. The normal path
    asks in the GUI. Unattended default: the selected UI locale.

.PARAMETER LlamaBackend
    Managed llama.cpp backend: auto, cpu, or cuda. Auto selects CUDA only when an
    NVIDIA GPU is detected and the user accepts installing a missing CUDA Toolkit.

.PARAMETER SkipLlamaBuild
    Skip the app-owned llama.cpp source build, then ensure Ollama is available.

.PARAMETER OllamaModel
    Optional model name to ensure with Ollama. Blank leaves its model library
    unchanged.

.PARAMETER SkipParakeet
    Do not install the optional 644 MiB Parakeet ASR model and CPU runner.

.PARAMETER TTSModule
    Optional managed local TTS module for unattended provisioning: none,
    faster-qwen3-tts, or chatterbox. Guided setup owns the normal choice.

.PARAMETER TTSDevice
    Local TTS execution device: auto, cpu, or cuda. Faster Qwen3-TTS requires
    CUDA. Auto prefers CUDA for Chatterbox when an NVIDIA GPU is detected.

.PARAMETER NoTTSAutoLaunch
    Configure the selected managed TTS server without launching it with the app.

.PARAMETER NoLauncher
    Do not create Start-MagicHandy.ps1.

.PARAMETER Yes
    Accept the documented defaults and third-party package/license prompts. This
    installs the complete selected source-build toolchain without stopping for
    input. Local TTS remains off unless TTSModule is passed explicitly.

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
    Provision the CUDA source-build toolchain, managed llama.cpp, Ollama,
    Parakeet, and all app/voice adapter binaries without launching.

.EXAMPLE
    .\install.ps1 -Yes -SkipLlamaBuild -NoLaunch
    Use Ollama instead of storing managed llama.cpp.

.EXAMPLE
    .\install.ps1 -Yes -TTSModule chatterbox -TTSDevice cpu -NoLaunch
    Bootstrap Chatterbox, uv, managed Python, PyTorch, and its model on a clean
    CPU-only Windows machine.

.EXAMPLE
    .\install.ps1 -Yes -UILanguage ja -ChatLanguage es -NoLaunch
    Use Japanese for the installer/app UI and Spanish for built-in chat replies.
#>
#Requires -Version 5.1
[CmdletBinding()]
param(
    [ValidateRange(1, 65535)]
    [int]$Port = 49717,
    [string]$DataDir,
    [ValidateSet('en', 'es', 'pt-BR', 'zh-Hans', 'ja')]
    [string]$UILanguage,
    [ValidateSet('en', 'es', 'pt-BR', 'zh-Hans', 'ja')]
    [string]$ChatLanguage,
    [ValidateSet('auto', 'cpu', 'cuda')]
    [string]$LlamaBackend = 'auto',
    [switch]$SkipLlamaBuild,
    [string]$OllamaModel,
    [switch]$SkipParakeet,
    [ValidateSet('', 'none', 'faster-qwen3-tts', 'chatterbox')]
    [string]$TTSModule = '',
    [ValidateSet('auto', 'cpu', 'cuda')]
    [string]$TTSDevice = 'auto',
    [switch]$NoTTSAutoLaunch,
    [switch]$NoLauncher,
    [switch]$Yes,
    [switch]$NoLaunch,
    [string]$StatePath,
    [switch]$PlanOnly,

    # update.ps1 uses these internal modes.
    [switch]$UseSavedChoices,
    [switch]$Reconfigure,
    [switch]$UpdateRun,
    [switch]$OpenSetup,
    [switch]$CoreOnly
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
$explicitFeatureParameters = @(
    'LlamaBackend', 'SkipLlamaBuild', 'OllamaModel', 'SkipParakeet',
    'TTSModule', 'TTSDevice', 'NoTTSAutoLaunch'
)
$hasExplicitFeatureChoice = [bool]($explicitFeatureParameters | Where-Object {
    $PSBoundParameters.ContainsKey($_)
} | Select-Object -First 1)
$guiOwnsFreshChoices = -not $Yes -and -not $UseSavedChoices -and -not $Reconfigure -and
    -not $UpdateRun -and -not $hasExplicitFeatureChoice
if (-not $StatePath) {
    $StatePath = Get-MagicHandyInstallStatePath
}
$StatePath = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($StatePath)
if (-not [string]::IsNullOrWhiteSpace($DataDir)) {
    $DataDir = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($DataDir)
}
Set-Location $Repo

$preloadedState = $null
if ($UseSavedChoices -or $Reconfigure) {
    $preloadedState = Read-MagicHandyInstallState -Path $StatePath
    Set-MagicHandyInstallerLocale -Locale ([string]$preloadedState.ui_locale)
    $resolvedUILanguage = [string]$preloadedState.ui_locale
    $resolvedChatLanguage = [string]$preloadedState.chat_locale
} else {
    $resolvedUILanguage = if (-not [string]::IsNullOrWhiteSpace($UILanguage)) {
        $UILanguage
    } elseif ($Yes -or $guiOwnsFreshChoices) {
        'en'
    } else {
        Read-MagicHandyLanguage -QuestionKey 'language_selector' -Default 'en'
    }
    Set-MagicHandyInstallerLocale -Locale $resolvedUILanguage
    $resolvedChatLanguage = if (-not [string]::IsNullOrWhiteSpace($ChatLanguage)) {
        $ChatLanguage
    } elseif ($Yes -or $guiOwnsFreshChoices) {
        $resolvedUILanguage
    } else {
        Read-MagicHandyLanguage -QuestionKey 'chat_language_selector' -Default $resolvedUILanguage
    }
}

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
    if ($Yes -or $guiOwnsFreshChoices) {
        return [System.IO.Path]::GetFullPath((Get-ProfileDataDir))
    }
    $portable = Confirm-MagicHandyChoice -Question (Get-MagicHandyText -Key 'portable_question') -Default $false
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
        Write-Host (Get-MagicHandyText -Key 'nvidia_missing')
        return 'cpu'
    }
    Write-Host (Get-MagicHandyText -Key 'cuda_benefit') -ForegroundColor DarkGray
    Write-Host (Get-MagicHandyText -Key 'cuda_detected') -ForegroundColor Green
    Write-Host (Get-MagicHandyText -Key 'cuda_tradeoff') -ForegroundColor DarkGray
    if ($Yes -or (Confirm-MagicHandyChoice -Question (Get-MagicHandyText -Key 'cuda_question') -Default $true)) {
        return 'cuda'
    }
    return 'cpu'
}

function Read-TTSModuleChoice {
    param(
        [ValidateSet('faster-qwen3-tts', 'chatterbox')]
        [string]$Default,
        [bool]$HasNVIDIA
    )

    Write-Host ('  1. ' + (Get-MagicHandyText -Key 'tts_option_faster'))
    Write-Host ('  2. ' + (Get-MagicHandyText -Key 'tts_option_chatterbox'))
    $defaultValue = if ($Default -eq 'faster-qwen3-tts') { '1' } else { '2' }
    while ($true) {
        $answer = (Read-MagicHandyValue -Question (Get-MagicHandyText -Key 'tts_module_question') -Default $defaultValue).ToLowerInvariant()
        if ($answer -in @('1', 'faster', 'faster-qwen3-tts')) {
            if ($HasNVIDIA) {
                return 'faster-qwen3-tts'
            }
            Write-Warning (Get-MagicHandyText -Key 'tts_faster_unavailable')
            continue
        }
        if ($answer -in @('2', 'chatterbox')) {
            return 'chatterbox'
        }
        Write-Warning (Get-MagicHandyText -Key 'tts_module_invalid')
    }
}

function Read-TTSConfiguration {
    param(
        [ValidateSet('none', 'faster-qwen3-tts', 'chatterbox')]
        [string]$DefaultModule = 'none',
        [ValidateSet('cpu', 'cuda')]
        [string]$DefaultDevice = 'cpu',
        [bool]$DefaultAutoLaunch = $false,
        [switch]$UseCommandLineChoices
    )

    $hasNVIDIA = [bool](Resolve-MagicHandyExecutable -Name 'nvidia-smi')
    $module = if ($UseCommandLineChoices -and -not [string]::IsNullOrWhiteSpace($TTSModule)) {
        $TTSModule
    } elseif ($UseCommandLineChoices -and $Yes) {
        'none'
    } else {
        Write-Host ''
        Write-Host (Get-MagicHandyText -Key 'tts_benefit')
        Write-Host (Get-MagicHandyText -Key 'tts_zero_dependencies') -ForegroundColor DarkGray
        Write-Host (Get-MagicHandyText -Key 'tts_storage_tradeoff') -ForegroundColor DarkGray
        $installTTS = Confirm-MagicHandyChoice `
            -Question (Get-MagicHandyText -Key 'tts_question') `
            -Default ($DefaultModule -ne 'none')
        if (-not $installTTS) {
            'none'
        } else {
            Write-Host (Get-MagicHandyText -Key 'tts_faster_benefit')
            Write-Host (Get-MagicHandyText -Key 'tts_chatterbox_benefit')
            $choiceDefault = if ($DefaultModule -ne 'none') {
                $DefaultModule
            } elseif ($hasNVIDIA) {
                'faster-qwen3-tts'
            } else {
                'chatterbox'
            }
            Read-TTSModuleChoice -Default $choiceDefault -HasNVIDIA $hasNVIDIA
        }
    }

    if ($module -eq 'none') {
        return [pscustomobject]@{
            Module = 'none'
            Device = 'cpu'
            AutoLaunch = $false
        }
    }

    if ($module -eq 'faster-qwen3-tts') {
        if ($UseCommandLineChoices -and $TTSDevice -eq 'cpu') {
            throw (Get-MagicHandyText -Key 'tts_faster_cuda_only')
        }
        if (-not $hasNVIDIA -and -not $PlanOnly) {
            throw (Get-MagicHandyText -Key 'tts_faster_unavailable')
        }
        $device = 'cuda'
    } else {
        if ($UseCommandLineChoices -and $TTSDevice -ne 'auto') {
            $device = $TTSDevice
        } elseif (-not $hasNVIDIA) {
            $device = 'cpu'
        } elseif ($UseCommandLineChoices -and $Yes) {
            $device = 'cuda'
        } else {
            Write-Host (Get-MagicHandyText -Key 'tts_cuda_benefit') -ForegroundColor DarkGray
            $device = if (Confirm-MagicHandyChoice `
                    -Question (Get-MagicHandyText -Key 'tts_cuda_question') `
                    -Default ($DefaultDevice -eq 'cuda')) {
                'cuda'
            } else {
                'cpu'
            }
        }
        if ($device -eq 'cuda' -and -not $hasNVIDIA -and -not $PlanOnly) {
            throw (Get-MagicHandyText -Key 'tts_cuda_unavailable')
        }
    }

    $autoLaunch = if ($UseCommandLineChoices -and $NoTTSAutoLaunch) {
        $false
    } elseif ($UseCommandLineChoices -and $Yes) {
        $true
    } else {
        Write-Host (Get-MagicHandyText -Key 'tts_auto_launch_benefit') -ForegroundColor DarkGray
        Confirm-MagicHandyChoice `
            -Question (Get-MagicHandyText -Key 'tts_auto_launch_question') `
            -Default $(if ($DefaultModule -eq 'none') { $true } else { $DefaultAutoLaunch })
    }

    return [pscustomobject]@{
        Module = $module
        Device = $device
        AutoLaunch = [bool]$autoLaunch
    }
}

function New-FreshConfiguration {
    $resolvedDataDir = Resolve-InitialDataDir
    $setupLLM = if ($Yes) {
        $true
    } else {
        Confirm-MagicHandyChoice -Question (Get-MagicHandyText -Key 'setup_llm_question') -Default $true
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
            Write-Host (Get-MagicHandyText -Key 'llama_benefit')
            Write-Host (Get-MagicHandyText -Key 'llama_ollama_tradeoff') -ForegroundColor DarkGray
            Confirm-MagicHandyChoice -Question (Get-MagicHandyText -Key 'llama_question') -Default $true
        }
        $backend = Resolve-InitialBackend -BuildManaged $buildManaged

        if (-not $buildManaged) {
            $ensureOllama = $true
        } elseif ($Yes) {
            $ensureOllama = $true
        } else {
            $ollamaDefault = [bool](Resolve-MagicHandyExecutable -Name 'ollama')
            $ensureOllama = Confirm-MagicHandyChoice -Question (Get-MagicHandyText -Key 'ollama_additional_question') -Default $ollamaDefault
        }
        if ($ensureOllama -and -not $Yes -and [string]::IsNullOrWhiteSpace($model)) {
            $model = Read-MagicHandyValue -Question (Get-MagicHandyText -Key 'ollama_model_optional')
        }
    }

    $parakeet = if ($SkipParakeet) {
        $false
    } elseif ($Yes) {
        $true
    } else {
        Write-Host ''
        Write-Host (Get-MagicHandyText -Key 'parakeet_benefit')
        Confirm-MagicHandyChoice -Question (Get-MagicHandyText -Key 'parakeet_question') -Default $false
    }
    $tts = Read-TTSConfiguration -UseCommandLineChoices
    $launcher = if ($NoLauncher) {
        $false
    } elseif ($Yes) {
        $true
    } else {
        Confirm-MagicHandyChoice -Question (Get-MagicHandyText -Key 'launcher_question') -Default $true
    }

    return New-MagicHandyInstallState `
        -RepositoryPath $Repo `
        -DataDir $resolvedDataDir `
        -Port $Port `
        -UILocale $resolvedUILanguage `
        -ChatLocale $resolvedChatLanguage `
        -SetupLLM $setupLLM `
        -BuildManagedLlama $buildManaged `
        -LlamaBackend $backend `
        -EnsureOllama $ensureOllama `
        -OllamaModel $model `
        -InstallParakeet $parakeet `
        -TTSModule ([string]$tts.Module) `
        -TTSDevice ([string]$tts.Device) `
        -TTSAutoLaunch ([bool]$tts.AutoLaunch) `
        -CreateLauncher $launcher
}

function New-GUIBootstrapConfiguration {
    return New-MagicHandyInstallState `
        -RepositoryPath $Repo `
        -DataDir (Resolve-InitialDataDir) `
        -Port $Port `
        -UILocale $resolvedUILanguage `
        -ChatLocale $resolvedChatLanguage `
        -SetupLLM $false `
        -BuildManagedLlama $false `
        -LlamaBackend 'cpu' `
        -EnsureOllama $false `
        -OllamaModel '' `
        -InstallParakeet $false `
        -TTSModule 'none' `
        -TTSDevice 'cpu' `
        -TTSAutoLaunch $false `
        -CreateLauncher (-not $NoLauncher)
}

function Copy-SavedState([object]$Existing) {
    return New-MagicHandyInstallState `
        -RepositoryPath $Repo `
        -DataDir ([string]$Existing.data_dir) `
        -Port ([int]$Existing.port) `
        -UILocale ([string]$Existing.ui_locale) `
        -ChatLocale ([string]$Existing.chat_locale) `
        -SetupLLM ([bool]$Existing.setup_llm) `
        -BuildManagedLlama ([bool]$Existing.build_managed_llama) `
        -LlamaBackend ([string]$Existing.llama_backend) `
        -EnsureOllama ([bool]$Existing.ensure_ollama) `
        -OllamaModel ([string]$Existing.ollama_model) `
        -InstallParakeet ([bool]$Existing.install_parakeet) `
        -TTSModule ([string]$Existing.tts_module) `
        -TTSDevice ([string]$Existing.tts_device) `
        -TTSAutoLaunch ([bool]$Existing.tts_auto_launch) `
        -CreateLauncher ([bool]$Existing.create_launcher) `
        -InstalledAt ([string]$Existing.installed_at)
}

if (-not $UpdateRun) {
    Write-MagicHandyBanner -Operation Install
}

$existing = $preloadedState
$state = if ($UseSavedChoices -or $Reconfigure) {
    if (-not $CoreOnly) {
        Write-InstallerHeading (Get-MagicHandyText -Key 'preserved_heading')
        Show-MagicHandyInstallState -State $existing
    }
    Copy-SavedState -Existing $existing
} elseif ($guiOwnsFreshChoices) {
    Write-Host ''
    Write-Host 'Optional device, model, and voice choices will open in the MagicHandy setup wizard.' -ForegroundColor Cyan
    New-GUIBootstrapConfiguration
} else {
    New-FreshConfiguration
}

if (-not $CoreOnly) {
    Write-InstallerHeading (Get-MagicHandyText -Key 'selected_heading')
    Show-MagicHandyInstallState -State $state
}
$runningPort = if ($null -ne $existing) { [int]$existing.port } else { [int]$state.port }
Invoke-MagicHandyProvision `
    -State $state `
    -RepositoryPath $Repo `
    -RunningPort $runningPort `
    -AssumeYes:$Yes `
    -PlanOnly:$PlanOnly `
    -PreserveAppLanguages:$UseSavedChoices `
    -ReconfigureTTS:$false `
    -CoreOnly:$CoreOnly `
    -CompleteSetup:(-not $guiOwnsFreshChoices -and -not $OpenSetup -and -not $Reconfigure)

if ($PlanOnly) {
    Write-Host ''
    Write-Host (Get-MagicHandyText -Key 'plan_complete') -ForegroundColor Green
    if (-not $UpdateRun) {
        Write-MagicHandyCompletionArt -Operation InstallPlan
    }
    return
}

Write-MagicHandyInstallState -State $state -Path $StatePath
Write-Host (Get-MagicHandyText -Key 'state_saved' -Values @($StatePath)) -ForegroundColor Green

$launch = -not $NoLaunch -and (
    $Yes -or $guiOwnsFreshChoices -or $OpenSetup -or $Reconfigure -or
    (Confirm-MagicHandyChoice -Question (Get-MagicHandyText -Key 'start_question') -Default $true)
)
if ($launch) {
    Start-MagicHandyApp `
        -RepositoryPath $Repo `
        -DataDir ([string]$state.data_dir) `
        -Port ([int]$state.port) `
        -Setup:($guiOwnsFreshChoices -or $OpenSetup -or $Reconfigure)
}

if (-not $UpdateRun) {
    Write-MagicHandyCompletionArt -Operation Install
}
