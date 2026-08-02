#Requires -Version 5.1
[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$Repo = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$support = Join-Path $Repo 'scripts\installer\InstallerSupport.psm1'
Import-Module $support -Force -DisableNameChecking
$supportModule = Get-Module InstallerSupport

function Assert-True([bool]$Condition, [string]$Message) {
    if (-not $Condition) {
        throw "Assertion failed: $Message"
    }
}

function Assert-Equal($Expected, $Actual, [string]$Message) {
    if ($Expected -ne $Actual) {
        throw "Assertion failed: $Message. Expected '$Expected', got '$Actual'."
    }
}

function Assert-PlanContains([string[]]$Plan, [string]$Pattern) {
    Assert-True -Condition ([bool]($Plan | Where-Object { $_ -match $Pattern })) -Message "plan should contain /$Pattern/"
}

function Assert-PlanExcludes([string[]]$Plan, [string]$Pattern) {
    Assert-True -Condition (-not [bool]($Plan | Where-Object { $_ -match $Pattern })) -Message "plan should exclude /$Pattern/"
}

function Assert-Throws([scriptblock]$Action, [string]$Pattern, [string]$Message) {
    $caught = ''
    try {
        & $Action
    } catch {
        $caught = $_.Exception.Message
    }
    Assert-True -Condition (-not [string]::IsNullOrWhiteSpace($caught)) -Message "$Message should throw"
    Assert-True -Condition ($caught -match $Pattern) -Message "$Message should match /$Pattern/, got '$caught'"
}

function Get-AvailableLoopbackPort {
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
    try {
        $listener.Start()
        return ([System.Net.IPEndPoint]$listener.LocalEndpoint).Port
    } finally {
        $listener.Stop()
    }
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("magichandy-installer-test-" + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
try {
    Write-Host 'Checking PowerShell 5.1 syntax...'
    $files = @(
        'bootstrap.ps1',
        'install.ps1',
        'update.ps1',
        'change-language.ps1',
        'scripts\install-llama-runtime.ps1',
        'scripts\install-parakeet-module.ps1',
        'scripts\install-tts-module.ps1',
        'scripts\update-tts-module.ps1',
        'scripts\release\Build-WindowsRelease.ps1',
        'scripts\release\Test-WindowsRelease.ps1',
        'scripts\installer\InstallerSupport.psm1',
        'internal\llm\runtimeassets\build-managed-llama.ps1'
    )
    foreach ($file in $files) {
        $tokens = $null
        $errors = $null
        $path = Join-Path $Repo $file
        [System.Management.Automation.Language.Parser]::ParseFile($path, [ref]$tokens, [ref]$errors) | Out-Null
        Assert-Equal -Expected 0 -Actual $errors.Count -Message "$file should parse"
    }
    Assert-True -Condition (Test-Path -LiteralPath (Join-Path $Repo 'scripts\tts\chatterbox-server.py') -PathType Leaf) -Message 'Chatterbox launcher shim should ship with the installer'
    $bootstrapSource = [System.IO.File]::ReadAllText((Join-Path $Repo 'bootstrap.ps1'))
    Assert-True -Condition ($bootstrapSource.Contains('Microsoft.WinGet.Client')) -Message 'clean-machine bootstrap should use the official WinGet repair path'
    Assert-True -Condition ($bootstrapSource.Contains('--id Git.Git')) -Message 'clean-machine bootstrap should install Git when missing'
    Assert-True -Condition ($bootstrapSource.Contains('& $installer @installerArguments')) -Message 'clean-machine bootstrap should delegate choices to install.ps1'
    $ttsInstaller = Join-Path $Repo 'scripts\install-tts-module.ps1'
    $ttsInstallerSource = [System.IO.File]::ReadAllText($ttsInstaller)
    Assert-True -Condition ($ttsInstallerSource.Contains('--query-gpu=compute_cap')) -Message 'Chatterbox install should detect NVIDIA compute capability'
    Assert-True -Condition ($ttsInstallerSource.Contains('requirements-nvidia.txt')) -Message 'Chatterbox install should retain the CUDA 12.1 dependency set'
    Assert-True -Condition ($ttsInstallerSource.Contains('requirements-nvidia-cu128.txt')) -Message 'Chatterbox install should retain the RTX 50-series CUDA 12.8 dependency set'
    Assert-True -Condition ($ttsInstallerSource.Contains("Microsoft\WinGet\Links\uv.exe")) -Message 'TTS install should resolve the WinGet portable uv link in the current process'
    Assert-True -Condition ($ttsInstallerSource.Contains("Invoke-MagicHandyWinGetInstall -ID 'astral-sh.uv'")) -Message 'TTS install should repair WinGet and refresh PATH after installing uv'
    Assert-True -Condition ($ttsInstallerSource.Contains("@('python', 'install', `$PythonVersion)")) -Message 'TTS install should explicitly provision its managed Python runtime'
    Assert-True -Condition ($ttsInstallerSource.Contains('Reusing the existing $reportedVersion environment')) -Message 'TTS reinstall should not replace an in-use compatible Python launcher'
    Assert-True -Condition (-not $ttsInstallerSource.Contains("Read-TTSChoice -Question 'Reference WAV path'")) -Message 'TTS install must leave Faster Qwen reference selection to the GUI'
    Assert-True -Condition (-not $ttsInstallerSource.Contains("Read-TTSChoice -Question 'Exact reference transcript'")) -Message 'TTS install must leave Faster Qwen transcription to the GUI'
    Assert-True -Condition (-not $ttsInstallerSource.Contains('[string]$ReferenceTranscript')) -Message 'TTS install must not expose a Faster Qwen transcript parameter'
    Assert-True -Condition (-not $ttsInstallerSource.Contains('requires a reference WAV and its exact transcript')) -Message 'empty Faster Qwen references must not fail installation'
    Assert-True -Condition ($ttsInstallerSource.Contains('Import-Module $supportPath -Force')) -Message 'standalone TTS install should initialize its isolated installer support module'
    Assert-True -Condition ($ttsInstallerSource.Contains("@('--max-workers', '1')")) -Message 'Windows TTS model downloads must serialize Hugging Face cache finalization'
    Assert-True -Condition ($ttsInstallerSource.Contains("'HF_HUB_DISABLE_SYMLINKS_WARNING'")) -Message 'Windows TTS installs should replace the Hugging Face symlink warning with installer-owned handling'
    Assert-True -Condition ($ttsInstallerSource.Contains('for ($attempt = 1; $attempt -le 3; $attempt++)')) -Message 'TTS model downloads should retry the resumable cache'
    Assert-True -Condition ($ttsInstallerSource.Contains('Downloaded files were kept; rerun the installer to resume.')) -Message 'TTS model failure should explain that completed downloads are retained'
    Assert-True -Condition ($ttsInstallerSource.Contains("@('faster_qwen3_tts.egg-info')")) -Message 'partial Faster Qwen installs should recognize installer-generated package metadata'
    $mainInstallerSource = [System.IO.File]::ReadAllText((Join-Path $Repo 'install.ps1'))
    Assert-True -Condition (-not $mainInstallerSource.Contains('TTSReferenceWav')) -Message 'main installer must not expose a reference WAV choice'
    Assert-True -Condition (-not $mainInstallerSource.Contains('TTSReferenceTranscript')) -Message 'main installer must not expose a reference transcript choice'
    Assert-True -Condition ($mainInstallerSource.Contains('$guiOwnsFreshChoices')) -Message 'plain source installation should delegate optional choices to guided setup'
    Assert-True -Condition (-not $mainInstallerSource.Contains('function Read-ReconfiguredState')) -Message 'console reconfiguration should be retired in favor of guided setup'
    $updateSource = [System.IO.File]::ReadAllText((Join-Path $Repo 'update.ps1'))
    Assert-True -Condition ($updateSource.Contains('CoreOnly = $true')) -Message 'updates should not replay stale optional installer choices before opening guided setup'
    $coreCommandSource = [System.IO.File]::ReadAllText((Join-Path $Repo 'cmd\magichandy\main.go'))
    Assert-True -Condition (-not $coreCommandSource.Contains('"tts-reference-text"')) -Message 'the internal install command must leave Faster Qwen transcript changes to the GUI'
    $ttsUpdaterSource = [System.IO.File]::ReadAllText((Join-Path $Repo 'scripts\update-tts-module.ps1'))
    Assert-True -Condition (-not $ttsUpdaterSource.Contains('ReferenceTranscript =')) -Message 'TTS updates must not restore stale command-line reference text'
    Assert-True -Condition ($ttsUpdaterSource.Contains('Import-Module $supportPath -Force')) -Message 'standalone TTS update should initialize its isolated installer support module'
    $installerModulePath = Join-Path $Repo 'scripts\installer\InstallerSupport.psm1'
    $installerModuleSource = [System.IO.File]::ReadAllText($installerModulePath)
    Assert-True -Condition ($installerModuleSource.Contains('function Invoke-MagicHandyPowerShellScript')) -Message 'main installer should isolate managed TTS scripts in a child PowerShell process'
    Assert-True -Condition ($installerModuleSource.Contains('-NoProfile -ExecutionPolicy Bypass -File $ScriptPath @Arguments')) -Message 'TTS process isolation should preserve script paths and argument boundaries'
    $nonASCIIBytes = @([System.IO.File]::ReadAllBytes($installerModulePath) | Where-Object { $_ -gt 127 })
    Assert-Equal -Expected 0 -Actual $nonASCIIBytes.Count -Message 'InstallerSupport.psm1 must remain ASCII-safe for Windows PowerShell 5.1'

    Write-Host 'Checking compatible TTS Python environment reuse...'
    $ttsTokens = $null
    $ttsErrors = $null
    $ttsAst = [System.Management.Automation.Language.Parser]::ParseFile(
        $ttsInstaller,
        [ref]$ttsTokens,
        [ref]$ttsErrors
    )
    foreach ($functionName in @('Invoke-Checked', 'Initialize-TTSPythonEnvironment')) {
        $functionAst = $ttsAst.Find({
            $args[0] -is [System.Management.Automation.Language.FunctionDefinitionAst] -and
                $args[0].Name -eq $functionName
        }, $true)
        Assert-True -Condition ($null -ne $functionAst) -Message "TTS installer should define $functionName"
        Invoke-Expression $functionAst.Extent.Text
    }

    $fakeRuntimeSource = Join-Path $tempRoot 'fake-tts-runtime.go'
    $fakeRuntimeBuild = Join-Path $tempRoot 'fake-tts-runtime.exe'
    [System.IO.File]::WriteAllText($fakeRuntimeSource, @'
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if strings.EqualFold(filepath.Base(os.Args[0]), "uv.exe") {
		if len(os.Args) > 1 && os.Args[1] == "venv" {
			fmt.Fprintln(os.Stderr, "compatible environments must not be recreated")
			os.Exit(97)
		}
		return
	}
	fmt.Println("Python 3.11.15")
}
'@)
    $goCommand = Get-Command 'go.exe' -ErrorAction SilentlyContinue
    if (-not $goCommand) {
        $goCommand = Get-Command 'go' -ErrorAction Stop
    }
    & $goCommand.Source build -trimpath -o $fakeRuntimeBuild $fakeRuntimeSource
    if ($LASTEXITCODE -ne 0) {
        throw "Could not build the isolated TTS recovery fixture (exit $LASTEXITCODE)."
    }

    $reuseRoot = Join-Path $tempRoot 'tts-reuse'
    $reuseScripts = Join-Path $reuseRoot '.venv\Scripts'
    New-Item -ItemType Directory -Force -Path $reuseScripts | Out-Null
    $fakeUv = Join-Path $tempRoot 'uv.exe'
    $fakePython = Join-Path $reuseScripts 'python.exe'
    Copy-Item -LiteralPath $fakeRuntimeBuild -Destination $fakeUv
    Copy-Item -LiteralPath $fakeRuntimeBuild -Destination $fakePython
    $reusedEnvironment = Initialize-TTSPythonEnvironment -Uv $fakeUv -Root $reuseRoot -PythonVersion '3.11'
    Assert-Equal -Expected $fakePython -Actual ([string]$reusedEnvironment.Python) -Message 'compatible TTS Python path should be reused'
    Assert-Equal -Expected 'Python 3.11.15' -Actual ([string]$reusedEnvironment.Version) -Message 'compatible TTS Python version should be retained'
    Assert-True -Condition (Test-Path -LiteralPath $fakePython -PathType Leaf) -Message 'compatible TTS Python launcher should remain in place'

    $innoSource = [System.IO.File]::ReadAllText((Join-Path $Repo 'installer\magichandy.iss'))
    Assert-True -Condition ($innoSource.Contains('DefaultDirName={autopf}\MagicHandy')) -Message 'Windows setup should default to Program Files'
    Assert-True -Condition ($innoSource.Contains('DisableDirPage=no')) -Message 'Windows setup should always expose the destination chooser'
    Assert-True -Condition ($innoSource.Contains('Name: "desktopicon"')) -Message 'Windows setup should offer a desktop shortcut'
    Assert-True -Condition ($innoSource.Contains('Flags: unchecked')) -Message 'desktop shortcut should remain opt-in'
    Assert-True -Condition ($innoSource.Contains("HasUninstallSwitch('/KEEPUSERDATA')")) -Message 'uninstall should support explicit data retention'
    Assert-True -Condition ($innoSource.Contains("HasUninstallSwitch('/PURGEUSERDATA')")) -Message 'uninstall should support explicit clean removal'
    Assert-True -Condition ($innoSource.Contains('PurgeRequested or UninstallSilent')) -Message 'silent uninstall should default to a clean reset'
    Assert-True -Condition ($innoSource.Contains("ExpandConstant('{userappdata}\MagicHandy')")) -Message 'clean uninstall should target only the packaged app data root'
    Assert-True -Condition ($innoSource.Contains('Parameters: "-prepare-uninstall"')) -Message 'uninstall should request graceful app and worker shutdown before removal'

    Write-Host 'Checking installer localization catalogs and prompt coverage...'
    $localeRoot = Join-Path $Repo 'scripts\installer\locales'
    $catalogs = @{}
    foreach ($locale in @('en', 'es', 'pt-BR', 'zh-Hans', 'ja')) {
        $catalogPath = Join-Path $localeRoot "$locale.json"
        $catalogText = [System.IO.File]::ReadAllText($catalogPath, [System.Text.Encoding]::UTF8)
        Assert-True -Condition (-not $catalogText.Contains([char]0xfffd)) -Message "$locale catalog should not contain replacement characters"
        $catalogs[$locale] = $catalogText | ConvertFrom-Json
    }
    $englishKeys = @($catalogs.en.PSObject.Properties.Name | Sort-Object)
    Assert-True -Condition ($englishKeys.Count -gt 100) -Message 'installer catalogs should cover the full decision tree'
    foreach ($locale in @('es', 'pt-BR', 'zh-Hans', 'ja')) {
        $keys = @($catalogs[$locale].PSObject.Properties.Name | Sort-Object)
        Assert-Equal -Expected ($englishKeys -join "`n") -Actual ($keys -join "`n") -Message "$locale catalog key parity"
        foreach ($key in $englishKeys) {
            $expected = @([regex]::Matches([string]$catalogs.en.$key, '\{\d+\}') | ForEach-Object Value | Sort-Object -Unique)
            $actual = @([regex]::Matches([string]$catalogs[$locale].$key, '\{\d+\}') | ForEach-Object Value | Sort-Object -Unique)
            Assert-Equal -Expected ($expected -join ',') -Actual ($actual -join ',') -Message "$locale/$key placeholder parity"
            Assert-True -Condition (-not [string]::IsNullOrWhiteSpace([string]$catalogs[$locale].$key)) -Message "$locale/$key should not be blank"
        }
        Set-MagicHandyInstallerLocale -Locale $locale
        Assert-Equal -Expected ([string]$catalogs[$locale].selected_heading) -Actual (Get-MagicHandyText -Key 'selected_heading') -Message "$locale runtime lookup"
        $nativeInput = [string]$catalogs[$locale].language_name
        $resolvedNative = & $supportModule { param([string]$Value) ConvertTo-MagicHandyLocale -Value $Value } $nativeInput
        Assert-Equal -Expected $locale -Actual $resolvedNative -Message "$locale native language-name input"
        Assert-True -Condition ((Get-MagicHandyText -Key 'selected_heading') -ne [string]$catalogs.en.selected_heading) -Message "$locale should translate selected installation heading"
    }
    Set-MagicHandyInstallerLocale -Locale 'en'
    foreach ($file in @('install.ps1', 'update.ps1', 'scripts\installer\InstallerSupport.psm1')) {
        $source = [System.IO.File]::ReadAllText((Join-Path $Repo $file))
        Assert-True -Condition (-not $source.Contains("-Question '")) -Message "$file should not pass a literal single-quoted decision prompt"
        Assert-True -Condition (-not $source.Contains('Read-Host ''')) -Message "$file should not use a literal Read-Host decision prompt"
    }
    Write-Host 'Checking same-process CUDA environment initialization...'
    $builderPath = Join-Path $Repo 'internal\llm\runtimeassets\build-managed-llama.ps1'
    $builderTokens = $null
    $builderErrors = $null
    $builderAst = [System.Management.Automation.Language.Parser]::ParseFile($builderPath, [ref]$builderTokens, [ref]$builderErrors)
    $initializerAst = $builderAst.Find({
        $args[0] -is [System.Management.Automation.Language.FunctionDefinitionAst] -and
            $args[0].Name -eq 'Initialize-CudaToolkitEnvironment'
    }, $true)
    Assert-True -Condition ($null -ne $initializerAst) -Message 'managed llama.cpp builder should define CUDA environment initialization'
    Invoke-Expression $initializerAst.Extent.Text

    $fakeToolkit = Join-Path $tempRoot 'CUDA\v99.1'
    $fakeNvcc = Join-Path $fakeToolkit 'bin\nvcc.exe'
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $fakeNvcc) | Out-Null
    [System.IO.File]::WriteAllText($fakeNvcc, '')
    $originalCUDAPath = [Environment]::GetEnvironmentVariable('CUDA_PATH', 'Process')
    $originalCudaToolkitDir = [Environment]::GetEnvironmentVariable('CudaToolkitDir', 'Process')
    try {
        $env:CUDA_PATH = 'stale'
        $env:CudaToolkitDir = ''
        Initialize-CudaToolkitEnvironment -Nvcc $fakeNvcc
        Assert-Equal -Expected $fakeToolkit -Actual $env:CUDA_PATH -Message 'CUDA_PATH should use the resolved nvcc toolkit root'
        Assert-Equal -Expected "$fakeToolkit\" -Actual $env:CudaToolkitDir -Message 'CudaToolkitDir should include the trailing separator required by MSBuild'
        $childEnvironment = & powershell.exe -NoProfile -Command '[Console]::Write($env:CUDA_PATH + [char]124 + $env:CudaToolkitDir)'
        Assert-Equal -Expected "$fakeToolkit|$fakeToolkit\" -Actual $childEnvironment -Message 'CUDA environment should reach child build processes'
    } finally {
        [Environment]::SetEnvironmentVariable('CUDA_PATH', $originalCUDAPath, 'Process')
        [Environment]::SetEnvironmentVariable('CudaToolkitDir', $originalCudaToolkitDir, 'Process')
    }

    Write-Host 'Checking installer branding and completion art...'
    $installBanner = Write-MagicHandyBanner -Operation Install 6>&1 | Out-String
    Assert-True -Condition ($installBanner -match 'INSTALL - local-first AI control for The Handy') -Message 'install banner should identify the product and operation'
    Assert-True -Condition ($installBanner -match 'Emergency Stop') -Message 'install banner should retain the safety reminder'
    $updateBanner = Write-MagicHandyBanner -Operation Update 6>&1 | Out-String
    Assert-True -Condition ($updateBanner -match 'UPDATE - local-first AI control for The Handy') -Message 'update banner should identify the product and operation'
    $installCompletion = Write-MagicHandyCompletionArt -Operation Install 6>&1 | Out-String
    Assert-True -Condition ($installCompletion -match 'INSTALL COMPLETE') -Message 'install completion should identify the finished operation'
    Assert-True -Condition ($installCompletion -match 'APP BUILD VERIFIED - CONFIGURATION APPLIED') -Message 'install completion should confirm that selected language configuration was applied'
    Assert-True -Condition ($installCompletion -match 'Open Settings.+select a model, voice provider, and device transport') -Message 'install completion should give relevant next steps'
    Assert-True -Condition ($installCompletion -match '\|\|=+\[\]') -Message 'completion should include the Handy motion-rail text art'
    $updateCompletion = Write-MagicHandyCompletionArt -Operation Update 6>&1 | Out-String
    Assert-True -Condition ($updateCompletion -match 'Congratulations.+Saved installation choices were applied') -Message 'update completion should confirm preserved installation choices'
    Assert-True -Condition ($updateCompletion -match 'current app language and prompt settings were preserved\s+unless explicitly reconfigured') -Message 'update completion should describe app language authority'
    $planCompletion = Write-MagicHandyCompletionArt -Operation UpdatePlan 6>&1 | Out-String
    Assert-True -Condition ($planCompletion -match 'NO CHANGES MADE') -Message 'plan completion should not claim that a build ran'

    Write-Host 'Checking optional TTS installer plans and saved-choice updates...'
    $ttsInstaller = Join-Path $Repo 'scripts\install-tts-module.ps1'
    $ttsUpdater = Join-Path $Repo 'scripts\update-tts-module.ps1'
    $ttsData = Join-Path $tempRoot 'tts-data'
    $qwenRoot = Join-Path $ttsData 'voice\faster-qwen3-tts'
    $qwenPlan = & $ttsInstaller -Module faster-qwen3-tts -DataDir $ttsData -InstallRoot $qwenRoot -PlanOnly -Yes 6>&1 | Out-String
    Assert-True -Condition ($qwenPlan -match 'faster-qwen3-tts') -Message 'Faster Qwen plan should identify its module'
    Assert-True -Condition ($qwenPlan -match 'Python:\s+3\.11 \(managed by uv\)') -Message 'Faster Qwen should use managed Python 3.11'
    Assert-True -Condition ($qwenPlan -match 'Reference:\s+configure later in Settings > Voice') -Message 'Faster Qwen should direct reference setup to the GUI'
    Assert-True -Condition ($qwenPlan -match 'no dependencies, files, models, processes, or settings were changed') -Message 'Faster Qwen plan should state its no-write contract'
    Assert-True -Condition (-not (Test-Path -LiteralPath $qwenRoot)) -Message 'Faster Qwen plan should not create its install root'

    $savedLocalAppData = $env:LOCALAPPDATA
    $savedAppData = $env:APPDATA
    try {
        $env:LOCALAPPDATA = Join-Path $tempRoot 'standalone-local-app-data'
        $env:APPDATA = Join-Path $tempRoot 'standalone-roaming-app-data'
        $standalonePlan = & $ttsInstaller -Module faster-qwen3-tts -PlanOnly -Yes 6>&1 | Out-String -Width 4096
        $expectedStandaloneData = Join-Path $env:APPDATA 'MagicHandy'
        Assert-True `
            -Condition ($standalonePlan -match [regex]::Escape($expectedStandaloneData)) `
            -Message 'standalone TTS install should share the core and uninstaller app-data root'
        Assert-True -Condition (-not (Test-Path -LiteralPath $expectedStandaloneData)) -Message 'standalone TTS plan should not create app data'
    } finally {
        $env:LOCALAPPDATA = $savedLocalAppData
        $env:APPDATA = $savedAppData
    }

    $chatterRoot = Join-Path $ttsData 'voice\chatterbox-tts'
    $chatterPlan = & $ttsInstaller -Module chatterbox -DataDir $ttsData -InstallRoot $chatterRoot -Device cpu -PlanOnly -Yes 6>&1 | Out-String
    Assert-True -Condition ($chatterPlan -match 'Chatterbox') -Message 'Chatterbox plan should identify its module'
    Assert-True -Condition ($chatterPlan -match 'Device:\s+cpu') -Message 'Chatterbox plan should preserve the selected device'
    Assert-True -Condition ($chatterPlan -match 'Python:\s+3\.10 \(managed by uv\)') -Message 'Chatterbox should use Python 3.10 for prebuilt Windows wheels'
    Assert-True -Condition (-not (Test-Path -LiteralPath $chatterRoot)) -Message 'Chatterbox plan should not create its install root'
    Assert-Throws -Action {
        & $ttsInstaller -Module faster-qwen3-tts -DataDir $ttsData -InstallRoot $qwenRoot -Device cpu -PlanOnly -Yes
    } -Pattern 'requires an NVIDIA GPU' -Message 'Faster Qwen CPU plan'
    Assert-Throws -Action {
        & $ttsInstaller -Module chatterbox -DataDir $ttsData -InstallRoot $chatterRoot -Voice '..\outside.wav' -PlanOnly -Yes
    } -Pattern 'plain .wav file name' -Message 'Chatterbox path-like voice'

    New-Item -ItemType Directory -Force -Path $qwenRoot | Out-Null
    $artifactState = [pscustomobject]@{
        tts_module = 'faster-qwen3-tts'
        data_dir = $ttsData
    }
    Sync-MagicHandyTTSModuleArtifacts -State $artifactState -RepositoryPath $Repo -InstallRoot $qwenRoot
    $installedQwenLauncher = Join-Path $qwenRoot 'magichandy-faster-qwen-server.py'
    Assert-True -Condition (Test-Path -LiteralPath $installedQwenLauncher -PathType Leaf) -Message 'main updater should sync the Faster Qwen launcher without reinstalling the model'
    Assert-Equal -Expected ((Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $Repo 'scripts\tts\faster-qwen-server.py')).Hash) -Actual ((Get-FileHash -Algorithm SHA256 -LiteralPath $installedQwenLauncher).Hash) -Message 'synced Faster Qwen launcher content'
    $installedQwenLauncherSource = Get-Content -Raw -LiteralPath $installedQwenLauncher
    Assert-True -Condition ($installedQwenLauncherSource -match 'instruct=request\.instruct or None') -Message 'synced Faster Qwen launcher should forward tone instructions'
    Assert-True -Condition ($installedQwenLauncherSource -match 'generate_voice_clone_streaming') -Message 'synced Faster Qwen launcher should retain streaming synthesis'
    Assert-True -Condition ($installedQwenLauncherSource -match 'threading\.Event\(\)') -Message 'synced Faster Qwen launcher should propagate client cancellation to its producer'
    Assert-True -Condition ($installedQwenLauncherSource -match 'queue\.Queue\(maxsize=2\)') -Message 'synced Faster Qwen launcher should bound abandoned streaming audio'
    Assert-True -Condition ($installedQwenLauncherSource -match 'stream\.close\(\)') -Message 'synced Faster Qwen launcher should close canceled model generators'
    $moduleState = [ordered]@{
        schema_version = 2
        module = 'faster-qwen3-tts'
        provider = 'faster_qwen3_tts'
        install_root = $qwenRoot
        data_dir = $ttsData
        source_url = 'https://example.invalid/faster-qwen3-tts.git'
        source_revision = 'fixture'
        model = 'Qwen/Qwen3-TTS-12Hz-0.6B-Base'
        voice = 'default'
        language = 'English'
        device = 'cuda'
        port = 8991
        auto_launch = $true
        speak_replies = $true
    }
    [System.IO.File]::WriteAllText((Join-Path $qwenRoot 'module-state.json'), ($moduleState | ConvertTo-Json))
    $updatePlan = & $ttsUpdater -InstallRoot $qwenRoot -PlanOnly -Yes 6>&1 | Out-String
    Assert-True -Condition ($updatePlan -match 'Auto-launch:\s+True') -Message 'TTS update plan should preserve auto-launch'
    Assert-True -Condition ($updatePlan -match 'Qwen/Qwen3-TTS-12Hz-0\.6B-Base') -Message 'TTS update plan should preserve the installed model'

    $moduleState.schema_version = 1
    $moduleState.reference_wav = 'C:\voices\sample.wav'
    $moduleState.reference_transcript = 'Exact transcript.'
    [System.IO.File]::WriteAllText((Join-Path $qwenRoot 'module-state.json'), ($moduleState | ConvertTo-Json))
    $checkOnly = & $ttsUpdater -InstallRoot $qwenRoot -CheckOnly -Yes 6>&1 | Out-String
    Assert-True -Condition ($checkOnly -match 'Module state verified') -Message 'main installer should validate legacy module state without restoring its reference fields'
    Assert-Throws -Action {
        & $ttsUpdater -InstallRoot $qwenRoot -CheckOnly -Device cuda -Yes
    } -Pattern 'CheckOnly cannot be combined' -Message 'TTS check-only override rejection'

    Write-Host 'Checking partial TTS source recovery...'
    $gitForTTS = Resolve-MagicHandyExecutable -Name 'git'
    Assert-True -Condition (-not [string]::IsNullOrWhiteSpace($gitForTTS)) -Message 'Git should be available for partial TTS source recovery tests'
    $partialSource = Join-Path $tempRoot 'partial-tts-source'
    New-Item -ItemType Directory -Force -Path $partialSource | Out-Null
    & $gitForTTS -C $partialSource init --quiet
    Assert-Equal -Expected 0 -Actual $LASTEXITCODE -Message 'partial TTS fixture repository initialization'
    & $gitForTTS -C $partialSource config user.email 'installer-test@magichandy.local'
    & $gitForTTS -C $partialSource config user.name 'MagicHandy Installer Test'
    [System.IO.File]::WriteAllText((Join-Path $partialSource 'pyproject.toml'), "[project]`nname = `"fixture`"`nversion = `"0.0.0`"`n")
    & $gitForTTS -C $partialSource add pyproject.toml
    & $gitForTTS -C $partialSource commit --quiet -m 'fixture'
    Assert-Equal -Expected 0 -Actual $LASTEXITCODE -Message 'partial TTS fixture repository commit'

    $generatedMetadata = Join-Path $partialSource 'faster_qwen3_tts.egg-info'
    New-Item -ItemType Directory -Force -Path $generatedMetadata | Out-Null
    [System.IO.File]::WriteAllText((Join-Path $generatedMetadata 'PKG-INFO'), 'installer generated')
    [System.IO.File]::WriteAllText((Join-Path $partialSource 'user-notes.txt'), 'preserve me')
    [System.IO.File]::AppendAllText((Join-Path $partialSource 'pyproject.toml'), '# preserve this edit')
    Add-MagicHandyGitInfoExclusions -RepositoryPath $partialSource -RelativePaths @('faster_qwen3_tts.egg-info')
    Add-MagicHandyGitInfoExclusions -RepositoryPath $partialSource -RelativePaths @('faster_qwen3_tts.egg-info')
    Add-MagicHandyGitInfoExclusions -RepositoryPath $partialSource -RelativePaths @()

    $partialStatus = @(& $gitForTTS -C $partialSource status --porcelain --untracked-files=all)
    Assert-Equal -Expected 0 -Actual $LASTEXITCODE -Message 'partial TTS fixture status'
    $partialStatusText = $partialStatus -join "`n"
    Assert-True -Condition ($partialStatusText -notmatch 'egg-info') -Message 'known installer metadata should not block a partial TTS retry'
    Assert-True -Condition ($partialStatusText -match '\?\? user-notes\.txt') -Message 'unknown untracked files should continue to block managed source replacement'
    Assert-True -Condition ($partialStatusText -match ' M pyproject\.toml') -Message 'tracked source edits should continue to block managed source replacement'

    $excludePath = Join-Path $partialSource '.git\info\exclude'
    $excludeLines = @([System.IO.File]::ReadAllLines($excludePath))
    $exclusionCount = @($excludeLines | Where-Object { $_ -eq '/faster_qwen3_tts.egg-info/' }).Count
    Assert-Equal -Expected 1 -Actual $exclusionCount -Message 'installer metadata exclusion should be idempotent'
    Assert-Throws -Action {
        Add-MagicHandyGitInfoExclusions -RepositoryPath $partialSource -RelativePaths @('..\outside')
    } -Pattern 'simple relative path' -Message 'path traversal in installer metadata exclusion'

    Write-Host 'Checking installer-state round trip and data hygiene...'
    $statePath = Join-Path $tempRoot 'install-state.json'
    $dataDir = Join-Path $tempRoot 'data'
    $state = New-MagicHandyInstallState `
        -RepositoryPath $Repo `
        -DataDir $dataDir `
        -Port 49800 `
        -SetupLLM $true `
        -BuildManagedLlama $true `
        -LlamaBackend 'cuda' `
        -EnsureOllama $true `
        -OllamaModel 'example/model:latest' `
        -InstallParakeet $true `
        -TTSModule 'chatterbox' `
        -TTSDevice 'cuda' `
        -TTSAutoLaunch $true `
        -CreateLauncher $true
    Write-MagicHandyInstallState -State $state -Path $statePath
    $loaded = Read-MagicHandyInstallState -Path $statePath
    Assert-Equal -Expected 3 -Actual ([int]$loaded.schema_version) -Message 'state schema'
    Assert-Equal -Expected 49800 -Actual ([int]$loaded.port) -Message 'saved port'
    Assert-Equal -Expected 'cuda' -Actual ([string]$loaded.llama_backend) -Message 'saved backend'
    Assert-Equal -Expected 'en' -Actual ([string]$loaded.ui_locale) -Message 'saved UI locale'
    Assert-Equal -Expected 'en' -Actual ([string]$loaded.chat_locale) -Message 'saved chat locale'
    Assert-True -Condition ([bool]$loaded.install_parakeet) -Message 'saved Parakeet choice'
    Assert-Equal -Expected 'chatterbox' -Actual ([string]$loaded.tts_module) -Message 'saved TTS module'
    Assert-Equal -Expected 'cuda' -Actual ([string]$loaded.tts_device) -Message 'saved TTS device'
    Assert-True -Condition ([bool]$loaded.tts_auto_launch) -Message 'saved TTS auto-launch choice'
    $json = Get-Content -LiteralPath $statePath -Raw
    Assert-True -Condition ($json -notmatch '(?i)api.?key|connection.?key|password|secret') -Message 'state must not define secret fields'
    Assert-True -Condition (-not (Test-Path -LiteralPath "$statePath.partial-$PID")) -Message 'state write must be atomic'

    Write-Host 'Checking TTS child-process module isolation...'
    $ttsProcessRepository = Join-Path $tempRoot 'tts process repository'
    $ttsProcessScripts = Join-Path $ttsProcessRepository 'scripts'
    $ttsProcessRoot = Join-Path $dataDir 'voice\chatterbox-tts'
    $ttsProcessMarker = Join-Path $tempRoot 'tts-process-marker.txt'
    New-Item -ItemType Directory -Force -Path $ttsProcessScripts, $ttsProcessRoot | Out-Null
    [System.IO.File]::WriteAllText((Join-Path $ttsProcessRoot 'module-state.json'), '{}')
    $ttsProcessUpdater = @'
#Requires -Version 5.1
[CmdletBinding()]
param(
    [string]$InstallRoot,
    [string]$Device,
    [switch]$ApplyInstallerChoices,
    [switch]$Yes,
    [switch]$AutoLaunch,
    [switch]$NoAutoLaunch,
    [switch]$CheckOnly
)
$ErrorActionPreference = 'Stop'
Import-Module $env:MAGICHANDY_TEST_INSTALLER_SUPPORT -Force -DisableNameChecking
$loaded = Read-MagicHandyInstallState -Path $env:MAGICHANDY_TEST_INSTALLER_STATE
if ($env:MAGICHANDY_TEST_TTS_FAIL -eq '1') {
    throw 'Intentional TTS process fixture failure.'
}
if ($Device -ne 'cuda' -or -not $ApplyInstallerChoices -or -not $Yes -or -not $AutoLaunch) {
    throw 'Parent installer choices were not preserved across the TTS process boundary.'
}
[System.IO.File]::WriteAllText($env:MAGICHANDY_TEST_TTS_MARKER, [string]$loaded.tts_module)
'@
    [System.IO.File]::WriteAllText((Join-Path $ttsProcessScripts 'update-tts-module.ps1'), $ttsProcessUpdater)
    [System.IO.File]::WriteAllText((Join-Path $ttsProcessScripts 'install-tts-module.ps1'), "#Requires -Version 5.1`nparam()`n")
    $originalTestSupport = $env:MAGICHANDY_TEST_INSTALLER_SUPPORT
    $originalTestState = $env:MAGICHANDY_TEST_INSTALLER_STATE
    $originalTestMarker = $env:MAGICHANDY_TEST_TTS_MARKER
    $originalTestFailure = $env:MAGICHANDY_TEST_TTS_FAIL
    try {
        $env:MAGICHANDY_TEST_INSTALLER_SUPPORT = $installerModulePath
        $env:MAGICHANDY_TEST_INSTALLER_STATE = $statePath
        $env:MAGICHANDY_TEST_TTS_MARKER = $ttsProcessMarker
        $env:MAGICHANDY_TEST_TTS_FAIL = '0'
        $parentModuleSurvivedUpdate = & $supportModule {
            param([object]$FixtureState, [string]$FixtureRepository)
            Install-MagicHandyTTSModule `
                -State $FixtureState `
                -RepositoryPath $FixtureRepository `
                -Reconfigure `
                -AssumeYes
            return $null -ne (Get-Command Write-MagicHandyLauncher -CommandType Function -ErrorAction SilentlyContinue)
        } $state $ttsProcessRepository
        $env:MAGICHANDY_TEST_TTS_FAIL = '1'
        Assert-Throws -Action {
            & $supportModule {
                param([object]$FixtureState, [string]$FixtureRepository)
                Install-MagicHandyTTSModule `
                    -State $FixtureState `
                    -RepositoryPath $FixtureRepository `
                    -Reconfigure `
                    -AssumeYes
            } $state $ttsProcessRepository
        } -Pattern 'TTS module update failed \(exit 1\)' -Message 'isolated TTS process failure propagation'
    } finally {
        $env:MAGICHANDY_TEST_INSTALLER_SUPPORT = $originalTestSupport
        $env:MAGICHANDY_TEST_INSTALLER_STATE = $originalTestState
        $env:MAGICHANDY_TEST_TTS_MARKER = $originalTestMarker
        $env:MAGICHANDY_TEST_TTS_FAIL = $originalTestFailure
    }
    Assert-True -Condition ([bool]$parentModuleSurvivedUpdate) -Message 'TTS update should not invalidate parent installer private helpers'
    Assert-Equal -Expected 'chatterbox' -Actual ([System.IO.File]::ReadAllText($ttsProcessMarker)) -Message 'isolated TTS update should retain initialized installer schema state'

    $unicodeStatePath = Join-Path $tempRoot 'unicode-install-state.json'
    $unicodeDataDir = Join-Path $tempRoot (-join ([char[]]@(0x5229, 0x7528, 0x8005, 0x30c7, 0x30fc, 0x30bf)))
    $unicodeModel = -join ([char[]]@(0x6a21, 0x578b, 0x2f, 0x97f3, 0x58f0, 0x3a, 0x6700, 0x65b0))
    $unicodeState = New-MagicHandyInstallState `
        -RepositoryPath $Repo `
        -DataDir $unicodeDataDir `
        -Port 49801 `
        -SetupLLM $true `
        -BuildManagedLlama $false `
        -LlamaBackend 'cpu' `
        -EnsureOllama $true `
        -OllamaModel $unicodeModel `
        -InstallParakeet $false `
        -CreateLauncher $false
    Write-MagicHandyInstallState -State $unicodeState -Path $unicodeStatePath
    $unicodeLoaded = Read-MagicHandyInstallState -Path $unicodeStatePath
    Assert-Equal -Expected ([System.IO.Path]::GetFullPath($unicodeDataDir)) -Actual ([string]$unicodeLoaded.data_dir) -Message 'BOM-less UTF-8 state data directory'
    Assert-Equal -Expected $unicodeModel -Actual ([string]$unicodeLoaded.ollama_model) -Message 'BOM-less UTF-8 state model'
    $unicodeBytes = [System.IO.File]::ReadAllBytes($unicodeStatePath)
    Assert-True -Condition (-not ($unicodeBytes.Length -ge 3 -and $unicodeBytes[0] -eq 0xef -and $unicodeBytes[1] -eq 0xbb -and $unicodeBytes[2] -eq 0xbf)) -Message 'state writer should remain BOM-less UTF-8'

    $legacyStatePath = Join-Path $tempRoot 'legacy-install-state.json'
    $legacyState = $json | ConvertFrom-Json
    $legacyState.schema_version = 1
    $legacyState.PSObject.Properties.Remove('ui_locale')
    $legacyState.PSObject.Properties.Remove('chat_locale')
    $legacyState.PSObject.Properties.Remove('tts_module')
    $legacyState.PSObject.Properties.Remove('tts_device')
    $legacyState.PSObject.Properties.Remove('tts_auto_launch')
    [System.IO.File]::WriteAllText($legacyStatePath, ($legacyState | ConvertTo-Json -Depth 5))
    $migratedState = Read-MagicHandyInstallState -Path $legacyStatePath
    Assert-Equal -Expected 3 -Actual ([int]$migratedState.schema_version) -Message 'legacy state should migrate to schema 3 in memory'
    Assert-Equal -Expected 'en' -Actual ([string]$migratedState.ui_locale) -Message 'legacy state UI locale default'
    Assert-Equal -Expected 'en' -Actual ([string]$migratedState.chat_locale) -Message 'legacy state chat locale default'
    Assert-Equal -Expected 'none' -Actual ([string]$migratedState.tts_module) -Message 'legacy state local TTS default'

    $schemaTwoStatePath = Join-Path $tempRoot 'schema-two-install-state.json'
    $schemaTwoState = $json | ConvertFrom-Json
    $schemaTwoState.schema_version = 2
    $schemaTwoState.PSObject.Properties.Remove('tts_module')
    $schemaTwoState.PSObject.Properties.Remove('tts_device')
    $schemaTwoState.PSObject.Properties.Remove('tts_auto_launch')
    [System.IO.File]::WriteAllText($schemaTwoStatePath, ($schemaTwoState | ConvertTo-Json -Depth 5))
    $migratedSchemaTwo = Read-MagicHandyInstallState -Path $schemaTwoStatePath
    Assert-Equal -Expected 3 -Actual ([int]$migratedSchemaTwo.schema_version) -Message 'schema 2 state should migrate to schema 3 in memory'
    Assert-Equal -Expected 'none' -Actual ([string]$migratedSchemaTwo.tts_module) -Message 'schema 2 TTS default'

    $invalidLocalePath = Join-Path $tempRoot 'invalid-locale-state.json'
    $invalidLocale = $json | ConvertFrom-Json
    $invalidLocale.ui_locale = 'fr'
    [System.IO.File]::WriteAllText($invalidLocalePath, ($invalidLocale | ConvertTo-Json -Depth 5))
    Assert-Throws -Action { Read-MagicHandyInstallState -Path $invalidLocalePath } -Pattern 'ui_locale.+one of' -Message 'unsupported installer locale'
    $invalidBooleanPath = Join-Path $tempRoot 'invalid-boolean-state.json'
    $invalidBoolean = $json | ConvertFrom-Json
    $invalidBoolean.build_managed_llama = 'false'
    [System.IO.File]::WriteAllText($invalidBooleanPath, ($invalidBoolean | ConvertTo-Json -Depth 5))
    Assert-Throws -Action { Read-MagicHandyInstallState -Path $invalidBooleanPath } -Pattern 'build_managed_llama.+boolean' -Message 'string-encoded installer boolean'

    $invalidTTSPath = Join-Path $tempRoot 'invalid-tts-state.json'
    $invalidTTS = $json | ConvertFrom-Json
    $invalidTTS.tts_module = 'faster-qwen3-tts'
    $invalidTTS.tts_device = 'cpu'
    [System.IO.File]::WriteAllText($invalidTTSPath, ($invalidTTS | ConvertTo-Json -Depth 5))
    Assert-Throws -Action { Read-MagicHandyInstallState -Path $invalidTTSPath } -Pattern 'must use CUDA' -Message 'Faster Qwen CPU installer state'

    $secretFieldPath = Join-Path $tempRoot 'secret-field-state.json'
    $secretField = $json | ConvertFrom-Json
    $secretField | Add-Member -NotePropertyName 'api_key' -NotePropertyValue 'must-not-be-retained'
    [System.IO.File]::WriteAllText($secretFieldPath, ($secretField | ConvertTo-Json -Depth 5))
    Assert-Throws -Action { Read-MagicHandyInstallState -Path $secretFieldPath } -Pattern "unsupported field 'api_key'" -Message 'unknown installer-state field'

    $inconsistentStatePath = Join-Path $tempRoot 'inconsistent-state.json'
    $inconsistentState = $json | ConvertFrom-Json
    $inconsistentState.setup_llm = $false
    [System.IO.File]::WriteAllText($inconsistentStatePath, ($inconsistentState | ConvertTo-Json -Depth 5))
    Assert-Throws -Action { Read-MagicHandyInstallState -Path $inconsistentStatePath } -Pattern 'cannot configure LLM runtimes' -Message 'inconsistent installer choices'

    Write-Host 'Checking update restores saved language before plan output...'
    $localizedStatePath = Join-Path $tempRoot 'localized-install-state.json'
    $localizedState = $json | ConvertFrom-Json
    $localizedState.ui_locale = 'ja'
    $localizedState.chat_locale = 'es'
    Write-MagicHandyInstallState -State $localizedState -Path $localizedStatePath
    $localizedBefore = [System.IO.File]::ReadAllText($localizedStatePath)
    $localizedOutput = & (Join-Path $Repo 'update.ps1') -Yes -NoPull -NoLaunch -PlanOnly -StatePath $localizedStatePath 6>&1 | Out-String
    Assert-True -Condition ($localizedOutput.Contains([string]$catalogs.ja.current_heading)) -Message 'update plan should restore Japanese before displaying choices'
    Assert-True -Condition ($localizedOutput.Contains((Get-MagicHandyLanguageName -Locale 'es'))) -Message 'update plan should display the saved Spanish chat language'
    Assert-True -Condition ($localizedOutput.Contains([string]$catalogs.ja.plan_languages_preserved)) -Message 'ordinary update should preserve backend language and prompt settings'
    Assert-Equal -Expected $localizedBefore -Actual ([System.IO.File]::ReadAllText($localizedStatePath)) -Message 'plan-only localized update must not rewrite installer state'
    Set-MagicHandyInstallerLocale -Locale 'en'
    Write-Host 'Checking PATH refresh preserves session-only tools...'
    $originalPath = $env:Path
    $sessionToolPath = Join-Path $tempRoot 'session-only-tools'
    try {
        $env:Path = "$sessionToolPath;$($sessionToolPath.ToUpperInvariant())"
        & $supportModule { Refresh-MagicHandyPath }
        $sessionMatches = @($env:Path -split ';' | Where-Object { $_ -ieq $sessionToolPath })
        Assert-Equal -Expected 1 -Actual $sessionMatches.Count -Message 'PATH refresh should preserve and deduplicate session-only entries'
    } finally {
        $env:Path = $originalPath
    }

    Write-Host 'Checking interrupted download resume and verification...'
    $downloadPort = Get-AvailableLoopbackPort
    $downloadSize = 256KB
    $downloadCut = 64KB
    $downloadSeed = 49717
    $downloadFixture = New-Object byte[] $downloadSize
    $downloadRandom = [System.Random]::new($downloadSeed)
    $downloadRandom.NextBytes($downloadFixture)
    $downloadFixturePath = Join-Path $tempRoot 'download-fixture.bin'
    $downloadDestination = Join-Path $tempRoot 'downloaded.bin'
    $downloadPartial = Join-Path $tempRoot 'download-cache\downloaded.partial'
    [System.IO.File]::WriteAllBytes($downloadFixturePath, $downloadFixture)
    $downloadHash = (Get-FileHash -LiteralPath $downloadFixturePath -Algorithm SHA256).Hash.ToLowerInvariant()
    $downloadJob = Start-Job -ArgumentList $downloadPort, $downloadSize, $downloadCut, $downloadSeed -ScriptBlock {
        param($Port, $Size, $Cut, $Seed)

        $payload = New-Object byte[] $Size
        $random = [System.Random]::new($Seed)
        $random.NextBytes($payload)
        $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, $Port)
        $ranges = New-Object System.Collections.Generic.List[string]
        try {
            $listener.Start()
            'READY'
            for ($requestNumber = 0; $requestNumber -lt 2; $requestNumber++) {
                $client = $listener.AcceptTcpClient()
                try {
                    $stream = $client.GetStream()
                    $reader = [System.IO.StreamReader]::new($stream, [System.Text.Encoding]::ASCII, $false, 1024, $true)
                    $range = ''
                    while ($true) {
                        $line = $reader.ReadLine()
                        if ([string]::IsNullOrEmpty($line)) {
                            break
                        }
                        if ($line.StartsWith('Range:', [StringComparison]::OrdinalIgnoreCase)) {
                            $range = $line.Substring(6).Trim()
                        }
                    }
                    $ranges.Add($range)

                    if ($requestNumber -eq 0) {
                        $header = "HTTP/1.1 200 OK`r`nContent-Length: $Size`r`nContent-Type: application/octet-stream`r`nConnection: close`r`n`r`n"
                        $headerBytes = [System.Text.Encoding]::ASCII.GetBytes($header)
                        $stream.Write($headerBytes, 0, $headerBytes.Length)
                        $stream.Write($payload, 0, $Cut)
                    } else {
                        if ($range -notmatch '^bytes=(\d+)-$') {
                            throw "Expected a resume Range header, got '$range'."
                        }
                        $start = [int]$Matches[1]
                        $remaining = $Size - $start
                        $end = $Size - 1
                        $header = "HTTP/1.1 206 Partial Content`r`nContent-Length: $remaining`r`nContent-Range: bytes $start-$end/$Size`r`nContent-Type: application/octet-stream`r`nConnection: close`r`n`r`n"
                        $headerBytes = [System.Text.Encoding]::ASCII.GetBytes($header)
                        $stream.Write($headerBytes, 0, $headerBytes.Length)
                        $stream.Write($payload, $start, $remaining)
                    }
                    $stream.Flush()
                } finally {
                    $client.Dispose()
                }
            }
            [pscustomobject]@{
                Requests = $ranges.Count
                FirstRange = $ranges[0]
                SecondRange = $ranges[1]
            }
        } finally {
            $listener.Stop()
        }
    }
    try {
        $downloadReady = $false
        $downloadReadyDeadline = [DateTime]::UtcNow.AddSeconds(10)
        do {
            $downloadJobOutput = @(Receive-Job -Job $downloadJob -Keep)
            if ($downloadJobOutput -contains 'READY') {
                $downloadReady = $true
                break
            }
            if ($downloadJob.State -eq 'Failed') {
                throw 'The interrupted-download test server failed before accepting requests.'
            }
            Start-Sleep -Milliseconds 50
        } while ([DateTime]::UtcNow -lt $downloadReadyDeadline)
        Assert-True -Condition $downloadReady -Message 'interrupted-download test server should become ready'

        & $supportModule {
            param($Uri, $Destination, $ExpectedSHA256, $PartialPath)
            Install-MagicHandyVerifiedDownload -Uri $Uri -Destination $Destination -ExpectedSHA256 $ExpectedSHA256 -PartialPath $PartialPath -MaxAttempts 3 -RetryDelayMilliseconds 1
        } "http://127.0.0.1:$downloadPort/model.bin" $downloadDestination $downloadHash $downloadPartial

        Wait-Job -Job $downloadJob -Timeout 10 | Out-Null
        $downloadResults = @(Receive-Job -Job $downloadJob | Where-Object { $_.PSObject.Properties.Name -contains 'SecondRange' })
        Assert-Equal -Expected 2 -Actual ([int]$downloadResults[-1].Requests) -Message 'interrupted download request count'
        Assert-Equal -Expected "bytes=$downloadCut-" -Actual ([string]$downloadResults[-1].SecondRange) -Message 'interrupted download should resume at the saved byte count'
        Assert-Equal -Expected $downloadHash -Actual ((Get-FileHash -LiteralPath $downloadDestination -Algorithm SHA256).Hash.ToLowerInvariant()) -Message 'resumed download checksum'
        Assert-True -Condition (-not (Test-Path -LiteralPath $downloadPartial)) -Message 'verified partial should be atomically promoted'
    } finally {
        Stop-Job -Job $downloadJob -ErrorAction SilentlyContinue
        Remove-Job -Job $downloadJob -Force -ErrorAction SilentlyContinue
    }

    $originalConsoleOutput = [Console]::Out
    $progressOutput = [System.IO.StringWriter]::new()
    try {
        [Console]::SetOut($progressOutput)
        & $supportModule { Write-MagicHandyDownloadProgress -Name 'model.bin' -CompletedBytes 1MB -TotalBytes 4MB } | Out-Null
    } finally {
        [Console]::SetOut($originalConsoleOutput)
    }
    Assert-True -Condition ($progressOutput.ToString() -match '25[\.,]0%') -Message 'inline download progress should preserve fractional percentages'

    Write-Host 'Checking pinned-file verification...'
    $pinnedFixture = Join-Path $tempRoot 'pinned-fixture.bin'
    [System.IO.File]::WriteAllText($pinnedFixture, 'verified bytes')
    $pinnedHash = (Get-FileHash -LiteralPath $pinnedFixture -Algorithm SHA256).Hash.ToLowerInvariant()
    $pinnedValid = & $supportModule { param($Path, $Hash) Test-MagicHandyPinnedFile -Path $Path -ExpectedSHA256 $Hash } $pinnedFixture $pinnedHash
    Assert-True -Condition $pinnedValid -Message 'pinned-file verifier should accept exact bytes'
    [System.IO.File]::AppendAllText($pinnedFixture, 'tampered')
    $pinnedTampered = & $supportModule { param($Path, $Hash) Test-MagicHandyPinnedFile -Path $Path -ExpectedSHA256 $Hash } $pinnedFixture $pinnedHash
    Assert-True -Condition (-not $pinnedTampered) -Message 'pinned-file verifier should reject changed bytes'


    Write-Host 'Checking generated launcher quoting and syntax...'
    $launcherRoot = Join-Path $tempRoot "launcher root's copy"
    $launcherData = Join-Path $tempRoot "data root's copy"
    New-Item -ItemType Directory -Force -Path $launcherRoot | Out-Null
    $supportModule = Get-Module InstallerSupport
    & $supportModule {
        param($RepositoryPath, $DataDir)
        Write-MagicHandyLauncher -RepositoryPath $RepositoryPath -DataDir $DataDir -Port 49900
    } $launcherRoot $launcherData
    $launcherPath = Join-Path $launcherRoot 'Start-MagicHandy.ps1'
    $tokens = $null
    $errors = $null
    [System.Management.Automation.Language.Parser]::ParseFile($launcherPath, [ref]$tokens, [ref]$errors) | Out-Null
    Assert-Equal -Expected 0 -Actual $errors.Count -Message 'generated launcher should parse'
    $launcherText = Get-Content -LiteralPath $launcherPath -Raw
    Assert-True -Condition ($launcherText -match 'Start-MagicHandyApp') -Message 'launcher should reuse the verified app startup path'
    Assert-True -Condition ($launcherText -match 'InstallerSupport\.psm1') -Message 'launcher should import shared installer support'
    Assert-True -Condition ($launcherText -match "root''s copy") -Message 'launcher should escape apostrophes in paths'
    & $supportModule { param($RepositoryPath) Remove-MagicHandyGeneratedLauncher -RepositoryPath $RepositoryPath } $launcherRoot
    Assert-True -Condition (-not (Test-Path -LiteralPath $launcherPath)) -Message 'disabling the installer launcher should remove the generated file'
    [System.IO.File]::WriteAllText($launcherPath, '# User-authored launcher')
    & $supportModule { param($RepositoryPath) Remove-MagicHandyGeneratedLauncher -RepositoryPath $RepositoryPath } $launcherRoot
    Assert-True -Condition (Test-Path -LiteralPath $launcherPath) -Message 'disabling the installer launcher should preserve a user-authored file'

    Write-Host 'Checking running app Stop and process-tree teardown before rebuild...'
    $runtimeRepo = Join-Path $tempRoot 'running-app-repo'
    $runtimeData = Join-Path $tempRoot 'running app data with spaces'
    $runtimePort = Get-AvailableLoopbackPort
    New-Item -ItemType Directory -Force -Path $runtimeRepo | Out-Null
    foreach ($file in @('go.mod', 'go.sum')) {
        Copy-Item -LiteralPath (Join-Path $Repo $file) -Destination $runtimeRepo
    }
    foreach ($directory in @('cmd', 'internal')) {
        Copy-Item -LiteralPath (Join-Path $Repo $directory) -Destination $runtimeRepo -Recurse
    }
    $runtimeWeb = Join-Path $runtimeRepo 'web'
    New-Item -ItemType Directory -Force -Path $runtimeWeb | Out-Null
    Copy-Item -LiteralPath (Join-Path $Repo 'web\assets.go') -Destination $runtimeWeb
    Copy-Item -LiteralPath (Join-Path $Repo 'web\dist') -Destination $runtimeWeb -Recurse
    $runtimeExe = Join-Path $runtimeRepo 'magichandy.exe'
    $go = Resolve-MagicHandyExecutable -Name 'go'
    Assert-True -Condition (-not [string]::IsNullOrWhiteSpace($go)) -Message 'Go is required by the Windows CI image'
    $previousCGO = $env:CGO_ENABLED
    try {
        $env:CGO_ENABLED = '0'
        & $go -C $runtimeRepo build -o $runtimeExe ./cmd/magichandy
        Assert-Equal -Expected 0 -Actual $LASTEXITCODE -Message 'test app build should succeed'
    } finally {
        $env:CGO_ENABLED = $previousCGO
    }
    Set-MagicHandyAppLanguages `
        -RepositoryPath $runtimeRepo `
        -DataDir ($runtimeData + '\') `
        -UILocale 'JA' `
        -ChatLocale 'ES'
    $runtimeArguments = & $supportModule {
        param($Address, $DataDir)
        New-MagicHandyAppArgumentLine -Address $Address -DataDir $DataDir
    } "127.0.0.1:$runtimePort" $runtimeData
    $runtimeProcess = Start-Process -FilePath $runtimeExe -ArgumentList $runtimeArguments -PassThru -WindowStyle Hidden
    try {
        $ready = $false
        $readyDeadline = [DateTime]::UtcNow.AddSeconds(10)
        do {
            try {
                Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:$runtimePort/api/state" -TimeoutSec 1 | Out-Null
                $ready = $true
                break
            } catch {
                Start-Sleep -Milliseconds 100
            }
        } while ([DateTime]::UtcNow -lt $readyDeadline)
        Assert-True -Condition $ready -Message 'test app should become ready'
        Assert-True -Condition (Test-Path -LiteralPath (Join-Path $runtimeData 'magichandy.db')) -Message 'quoted startup should keep the database under the intended spaced data path'
        $runtimeSettings = Invoke-RestMethod -Uri "http://127.0.0.1:$runtimePort/api/settings" -TimeoutSec 5
        Assert-Equal -Expected 'ja' -Actual ([string]$runtimeSettings.settings.ui.locale) -Message 'language helper should canonicalize UI locale'
        Assert-Equal -Expected 'magichandy_motion_v1_es' -Actual ([string]$runtimeSettings.settings.llm.prompt_set) -Message 'language helper should canonicalize chat locale'

        $foreignRepo = Join-Path $tempRoot 'foreign-app-repo'
        New-Item -ItemType Directory -Force -Path $foreignRepo | Out-Null
        Copy-Item -LiteralPath $runtimeExe -Destination (Join-Path $foreignRepo 'magichandy.exe')
        $foreignRejected = $false
        try {
            & $supportModule {
                param($RepositoryPath, $Port)
                Stop-MagicHandyAppForRebuild -RepositoryPath $RepositoryPath -Port $Port
            } $foreignRepo $runtimePort
        } catch {
            $foreignRejected = $_.Exception.Message -match 'owned by another process'
        }
        Assert-True -Condition $foreignRejected -Message 'rebuild preparation should refuse a listener from another checkout'
        $runtimeProcess.Refresh()
        Assert-True -Condition (-not $runtimeProcess.HasExited) -Message 'foreign-checkout refusal must leave the running app alive'

        [System.IO.File]::WriteAllText("$runtimeExe~", 'stale build backup')
        & $supportModule {
            param($RepositoryPath, $Port)
            Stop-MagicHandyAppForRebuild -RepositoryPath $RepositoryPath -Port $Port -AllowPhysicalStopConfirmation -PhysicalStopConfirmation { 'STOPPED' }
        } $runtimeRepo $runtimePort
        $runtimeProcess.Refresh()
        Assert-True -Condition $runtimeProcess.HasExited -Message 'rebuild preparation should stop the running app tree'
        Assert-True -Condition (-not (Test-Path -LiteralPath "$runtimeExe~")) -Message 'rebuild preparation should remove stale Go executable backups'
    } finally {
        $runtimeProcess.Refresh()
        if (-not $runtimeProcess.HasExited) {
            & "$env:SystemRoot\System32\taskkill.exe" /PID $runtimeProcess.Id /T /F | Out-Null
        }
    }

    Write-Host 'Checking coherent staged binary replacement and verified relaunch...'
    $binaryNames = @('magichandy.exe', 'voice-parakeet-worker.exe', 'voice-openai-tts-worker.exe', 'voice-elevenlabs-worker.exe')
    $staleHashes = @{}
    foreach ($name in $binaryNames) {
        $path = Join-Path $runtimeRepo $name
        [System.IO.File]::WriteAllText($path, "stale executable sentinel: $name")
        $staleHashes[$name] = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash
    }
    $brokenSource = Join-Path $runtimeRepo 'cmd\voice-parakeet-worker\main.go'
    $originalBrokenSource = [System.IO.File]::ReadAllText($brokenSource)
    $originalLocation = Get-Location
    $originalBuildCGO = $env:CGO_ENABLED
    $originalBuildGOOS = $env:GOOS
    $originalBuildGOARCH = $env:GOARCH
    try {
        [System.IO.File]::WriteAllText($brokenSource, $originalBrokenSource + "`r`nthis is not valid Go`r`n")
        Set-Location $tempRoot
        $env:CGO_ENABLED = '1'
        $env:GOOS = 'linux'
        $env:GOARCH = 'arm64'
        Assert-Throws -Action {
            & $supportModule {
                param($RepositoryPath, $GoExecutable)
                Build-MagicHandyBinaries -RepositoryPath $RepositoryPath -GoExecutable $GoExecutable
            } $runtimeRepo $go
        } -Pattern 'Building voice-parakeet-worker\.exe failed' -Message 'later worker build failure'
        foreach ($name in $binaryNames) {
            $path = Join-Path $runtimeRepo $name
            Assert-Equal -Expected $staleHashes[$name] -Actual ((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash) -Message "failed binary set should preserve $name"
        }
        Assert-Equal -Expected '1' -Actual $env:CGO_ENABLED -Message 'failed build should restore caller CGO setting'
        Assert-Equal -Expected 'linux' -Actual $env:GOOS -Message 'failed build should restore caller GOOS setting'
        Assert-Equal -Expected 'arm64' -Actual $env:GOARCH -Message 'failed build should restore caller GOARCH setting'

        [System.IO.File]::WriteAllText($brokenSource, $originalBrokenSource)
        & $supportModule {
            param($RepositoryPath, $GoExecutable)
            Build-MagicHandyBinaries -RepositoryPath $RepositoryPath -GoExecutable $GoExecutable
        } $runtimeRepo $go
        Assert-Equal -Expected '1' -Actual $env:CGO_ENABLED -Message 'successful build should restore caller CGO setting'
        Assert-Equal -Expected 'linux' -Actual $env:GOOS -Message 'successful build should restore caller GOOS setting'
        Assert-Equal -Expected 'arm64' -Actual $env:GOARCH -Message 'successful build should restore caller GOARCH setting'
    } finally {
        [System.IO.File]::WriteAllText($brokenSource, $originalBrokenSource)
        Set-Location $originalLocation
        $env:CGO_ENABLED = $originalBuildCGO
        $env:GOOS = $originalBuildGOOS
        $env:GOARCH = $originalBuildGOARCH
    }
    $staleHash = $staleHashes['magichandy.exe']
    $rebuiltHash = (Get-FileHash -LiteralPath $runtimeExe -Algorithm SHA256).Hash
    Assert-True -Condition ($rebuiltHash -ne $staleHash) -Message 'staged build should replace the stale executable only after a successful compile'
    foreach ($name in $binaryNames) {
        Assert-True -Condition ((Get-Item -LiteralPath (Join-Path $runtimeRepo $name)).Length -gt 1024) -Message "successful build should replace $name"
    }
    Assert-True -Condition (-not [bool](Get-ChildItem -LiteralPath $runtimeRepo -Filter '.installer-build-*' -Directory)) -Message 'staged builds should leave no temporary binary set'
    Assert-True -Condition (-not (Test-Path -LiteralPath "$runtimeExe~")) -Message 'staged replacement should not create a Go executable backup'
    $relaunchPort = Get-AvailableLoopbackPort
    $relaunchData = Join-Path $tempRoot 'verified relaunch data with spaces'
    Start-MagicHandyApp -RepositoryPath $runtimeRepo -DataDir $relaunchData -Port $relaunchPort -NoBrowser
    try {
        $index = (Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:$relaunchPort/" -TimeoutSec 5).Content
        $asset = [regex]::Match($index, '/assets/[^"'']+\.js').Value
        $javascript = (Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:$relaunchPort$asset" -TimeoutSec 5).Content
        Assert-True -Condition ($javascript -match 'Maximum output') -Message 'verified relaunch should serve the current embedded UI'
    } finally {
        & $supportModule { param($RepositoryPath, $Port) Stop-MagicHandyAppForRebuild -RepositoryPath $RepositoryPath -Port $Port -AllowPhysicalStopConfirmation -PhysicalStopConfirmation { 'STOPPED' } } $runtimeRepo $relaunchPort
    }

    Write-Host 'Checking rebuild Stop response classification...'
    $stopErrorJSON = '{"available":true,"engine":{"running":false,"paused":false,"completing":false},"error":"Intiface connection is stale"}'
    $stopErrorRecord = [System.Management.Automation.ErrorRecord]::new(
        [System.Net.WebException]::new('The remote server returned an error: (502) Bad Gateway.'),
        'MagicHandyStop502',
        [System.Management.Automation.ErrorCategory]::InvalidOperation,
        $null
    )
    $stopErrorRecord.ErrorDetails = [System.Management.Automation.ErrorDetails]::new($stopErrorJSON)
    $parsedStopError = & $supportModule {
        param($ErrorRecord)
        ConvertFrom-MagicHandyStopErrorResponse -ErrorRecord $ErrorRecord
    } $stopErrorRecord
    Assert-Equal -Expected 'Intiface connection is stale' -Actual ([string]$parsedStopError.error) -Message '502 Stop JSON should remain available for classification'
    $emptyStopErrorRecord = [System.Management.Automation.ErrorRecord]::new(
        [System.Net.WebException]::new('connection failed'),
        'MagicHandyStopNoBody',
        [System.Management.Automation.ErrorCategory]::ConnectionError,
        $null
    )
    $emptyStopError = & $supportModule {
        param($ErrorRecord)
        ConvertFrom-MagicHandyStopErrorResponse -ErrorRecord $ErrorRecord
    } $emptyStopErrorRecord
    Assert-True -Condition ($null -eq $emptyStopError) -Message 'a missing HTTP error body should fail parsing without a strict-mode exception'

    & $supportModule {
        Assert-MagicHandyRebuildStopResponse -AllowPhysicalStopConfirmation -PhysicalStopConfirmation { 'STOPPED' } -Response ([pscustomobject]@{
            available = $false
            stopped = $true
            error = 'configured transport unavailable'
        })
    } 3>$null
    & $supportModule {
        Assert-MagicHandyRebuildStopResponse -AllowPhysicalStopConfirmation -PhysicalStopConfirmation { 'STOPPED' } -Response ([pscustomobject]@{
            available = $true
            stopped = $true
            transport_result = [pscustomobject]@{ ok = $false }
            error = 'Intiface connection is stale'
        })
    } 3>$null
    & $supportModule {
        Assert-MagicHandyRebuildStopResponse -Response ([pscustomobject]@{
            available = $true
            transport_result = [pscustomobject]@{ ok = $true }
        })
    }
    $legacyTransportFailureRejected = $false
    try {
        & $supportModule {
            Assert-MagicHandyRebuildStopResponse -Response ([pscustomobject]@{
                available = $true
                engine = [pscustomobject]@{
                    running = $false
                    paused = $false
                    completing = $false
                }
                error = 'older endpoint transport failure'
            })
        }
    } catch {
        $legacyTransportFailureRejected = $_.Exception.Message -match 'Physical Stop delivery was not confirmed'
    }
    Assert-True -Condition $legacyTransportFailureRejected -Message 'an older endpoint failure must require explicit physical-stop confirmation'
    $wrongConfirmationRejected = $false
    try {
        & $supportModule {
            Assert-MagicHandyRebuildStopResponse -AllowPhysicalStopConfirmation -PhysicalStopConfirmation { 'stopped' } -Response ([pscustomobject]@{
                available = $false
                stopped = $true
                error = 'configured transport unavailable'
            })
        }
    } catch {
        $wrongConfirmationRejected = $_.Exception.Message -match 'Physical Stop delivery was not confirmed'
    }
    Assert-True -Condition $wrongConfirmationRejected -Message 'physical-stop confirmation must require exact STOPPED text'
    $malformedStopRejected = $false
    try {
        & $supportModule {
            Assert-MagicHandyRebuildStopResponse -Response ([pscustomobject]@{ message = 'gateway failure' })
        }
    } catch {
        $malformedStopRejected = $_.Exception.Message -match 'missing required field'
    }
    Assert-True -Condition $malformedStopRejected -Message 'an unexpected JSON response must fail closed'
    $runningStopRejected = $false
    try {
        & $supportModule {
            Assert-MagicHandyRebuildStopResponse -Response ([pscustomobject]@{
                available = $true
                engine = [pscustomobject]@{
                    running = $true
                    paused = $false
                    completing = $false
                }
            })
        }
    } catch {
        $runningStopRejected = $_.Exception.Message -match 'did not confirm local stopped state'
    }
    Assert-True -Condition $runningStopRejected -Message 'a response that still reports running motion must fail closed'
    $invalidBooleanRejected = $false
    try {
        & $supportModule {
            Assert-MagicHandyRebuildStopResponse -Response ([pscustomobject]@{
                available = $true
                stopped = 'false'
            })
        }
    } catch {
        $invalidBooleanRejected = $_.Exception.Message -match 'not boolean'
    }
    Assert-True -Condition $invalidBooleanRejected -Message 'string boolean fields must fail closed'
    $nullErrorRejected = $false
    try {
        & $supportModule {
            Assert-MagicHandyRebuildStopResponse -Response ([pscustomobject]@{
                available = $true
                stopped = $true
                error = $null
            })
        }
    } catch {
        $nullErrorRejected = $_.Exception.Message -match 'was not text'
    }
    Assert-True -Condition $nullErrorRejected -Message 'null error fields must fail closed'
    $incompleteEngineRejected = $false
    try {
        & $supportModule {
            Assert-MagicHandyRebuildStopResponse -Response ([pscustomobject]@{
                available = $true
                engine = [pscustomobject]@{ running = $false }
            })
        }
    } catch {
        $incompleteEngineRejected = $_.Exception.Message -match 'missing or not boolean'
    }
    Assert-True -Condition $incompleteEngineRejected -Message 'incomplete engine state must fail closed'
    $httpErrorWithoutMessageRejected = $false
    try {
        & $supportModule {
            Assert-MagicHandyRebuildStopResponse -Response ([pscustomobject]@{
                available = $true
                stopped = $true
                _http_error = $true
            })
        }
    } catch {
        $httpErrorWithoutMessageRejected = $_.Exception.Message -match 'did not contain an error message'
    }
    Assert-True -Condition $httpErrorWithoutMessageRejected -Message 'an HTTP error without a backend error message must fail closed'
    $unavailableWithoutMessageRejected = $false
    try {
        & $supportModule {
            Assert-MagicHandyRebuildStopResponse -Response ([pscustomobject]@{
                available = $false
                stopped = $true
            })
        }
    } catch {
        $unavailableWithoutMessageRejected = $_.Exception.Message -match 'transport failure without an error message'
    }
    Assert-True -Condition $unavailableWithoutMessageRejected -Message 'unavailable transport without an error message must fail closed'
    $failedResultWithoutMessageRejected = $false
    try {
        & $supportModule {
            Assert-MagicHandyRebuildStopResponse -Response ([pscustomobject]@{
                available = $true
                stopped = $true
                transport_result = [pscustomobject]@{ ok = $false }
            })
        }
    } catch {
        $failedResultWithoutMessageRejected = $_.Exception.Message -match 'transport failure without an error message'
    }
    Assert-True -Condition $failedResultWithoutMessageRejected -Message 'failed transport result without an error message must fail closed'
    $stopFailureRejected = $false
    try {
        & $supportModule {
            Assert-MagicHandyRebuildStopResponse -Response ([pscustomobject]@{
                available = $true
                stopped = $false
                error = 'active Stop failed'
            })
        }
    } catch {
        $stopFailureRejected = $_.Exception.Message -match 'active Stop failed'
    }
    Assert-True -Condition $stopFailureRejected -Message 'a failed active Stop must abort rebuild preparation'

    Write-Host 'Checking multiple checkout instances are refused before any forced teardown...'
    $multiPortA = Get-AvailableLoopbackPort
    do { $multiPortB = Get-AvailableLoopbackPort } while ($multiPortB -eq $multiPortA)
    $multiArgsA = & $supportModule { param($Address, $DataDir) New-MagicHandyAppArgumentLine -Address $Address -DataDir $DataDir } "127.0.0.1:$multiPortA" (Join-Path $tempRoot 'multi-data-a')
    $multiArgsB = & $supportModule { param($Address, $DataDir) New-MagicHandyAppArgumentLine -Address $Address -DataDir $DataDir } "127.0.0.1:$multiPortB" (Join-Path $tempRoot 'multi-data-b')
    $multiProcessA = Start-Process -FilePath $runtimeExe -ArgumentList $multiArgsA -PassThru -WindowStyle Hidden
    $multiProcessB = Start-Process -FilePath $runtimeExe -ArgumentList $multiArgsB -PassThru -WindowStyle Hidden
    try {
        $multiDeadline = [DateTime]::UtcNow.AddSeconds(10)
        do {
            $multiReadyA = $false
            $multiReadyB = $false
            try { Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:$multiPortA/api/state" -TimeoutSec 1 | Out-Null; $multiReadyA = $true } catch {}
            try { Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:$multiPortB/api/state" -TimeoutSec 1 | Out-Null; $multiReadyB = $true } catch {}
            if (-not ($multiReadyA -and $multiReadyB)) { Start-Sleep -Milliseconds 100 }
        } while (-not ($multiReadyA -and $multiReadyB) -and [DateTime]::UtcNow -lt $multiDeadline)
        Assert-True -Condition ($multiReadyA -and $multiReadyB) -Message 'both test app instances should become ready'
        $multipleRejected = $false
        try {
            & $supportModule { param($RepositoryPath, $Port) Stop-MagicHandyAppForRebuild -RepositoryPath $RepositoryPath -Port $Port } $runtimeRepo $multiPortA
        } catch {
            $multipleRejected = $_.Exception.Message -match 'Multiple MagicHandy instances'
        }
        Assert-True -Condition $multipleRejected -Message 'multiple checkout instances must be refused before teardown'
        $multiProcessA.Refresh()
        $multiProcessB.Refresh()
        Assert-True -Condition (-not $multiProcessA.HasExited -and -not $multiProcessB.HasExited) -Message 'multiple-instance refusal must leave every app alive'
    } finally {
        foreach ($process in @($multiProcessA, $multiProcessB)) {
            $process.Refresh()
            if (-not $process.HasExited) {
                & "$env:SystemRoot\System32\taskkill.exe" /PID $process.Id /T /F | Out-Null
            }
        }
    }

    Write-Host 'Checking selected-component plans...'
    $managedPlan = @(Get-MagicHandyProvisionPlan -State $loaded)
    Assert-PlanContains -Plan $managedPlan -Pattern 'Go 1\.25'
    Assert-PlanContains -Plan $managedPlan -Pattern 'Apply app UI language English and chat reply language English'
    Assert-PlanContains -Plan $managedPlan -Pattern 'Visual Studio C\+\+'
    Assert-PlanContains -Plan $managedPlan -Pattern 'CUDA Toolkit'
    Assert-PlanContains -Plan $managedPlan -Pattern 'Parakeet CPU runner'
    Assert-PlanContains -Plan $managedPlan -Pattern 'OpenAI-compatible TTS'
    Assert-PlanContains -Plan $managedPlan -Pattern 'managed Python.+chatterbox local TTS \(cuda; auto-launch: yes\)'
    Assert-PlanExcludes -Plan $managedPlan -Pattern 'NeuTTS|neutts-rs|eSpeak NG|LLVM/libclang|Rustup'

    $ollamaState = New-MagicHandyInstallState `
        -RepositoryPath $Repo `
        -DataDir $dataDir `
        -Port 49717 `
        -SetupLLM $true `
        -BuildManagedLlama $false `
        -LlamaBackend 'cpu' `
        -EnsureOllama $true `
        -InstallParakeet $false `
        -CreateLauncher $false
    $ollamaPlan = @(Get-MagicHandyProvisionPlan -State $ollamaState)
    Assert-PlanContains -Plan $ollamaPlan -Pattern 'Ensure Ollama'
    Assert-PlanContains -Plan $ollamaPlan -Pattern 'Remove an existing generated Start-MagicHandy\.ps1.*preserve any user-authored file'
    Assert-PlanExcludes -Plan $ollamaPlan -Pattern 'CMake|Visual Studio|CUDA|NeuTTS|neutts-rs|eSpeak NG|LLVM/libclang|Rustup|Parakeet CPU runner'

    Write-Host 'Checking updater fast-forward and dirty-worktree refusal...'
    $git = Resolve-MagicHandyExecutable -Name 'git'
    Assert-True -Condition (-not [string]::IsNullOrWhiteSpace($git)) -Message 'Git is required by the Windows CI image'
    $remote = Join-Path $tempRoot 'remote.git'
    $seed = Join-Path $tempRoot 'seed'
    $checkout = Join-Path $tempRoot 'checkout'
    & $git init --bare --initial-branch=main $remote | Out-Null
    & $git init --initial-branch=main $seed | Out-Null
    & $git -C $seed config user.email 'installer-test@magichandy.invalid'
    & $git -C $seed config user.name 'MagicHandy Installer Test'
    [System.IO.File]::WriteAllText((Join-Path $seed 'version.txt'), 'v1')
    & $git -C $seed add version.txt
    & $git -C $seed commit -m 'initial' | Out-Null
    & $git -C $seed remote add origin $remote
    & $git -C $seed push -u origin HEAD | Out-Null
    & $git clone $remote $checkout | Out-Null
    [System.IO.File]::WriteAllText((Join-Path $seed 'version.txt'), 'v2')
    & $git -C $seed add version.txt
    & $git -C $seed commit -m 'update' | Out-Null
    & $git -C $seed push | Out-Null
    Update-MagicHandySource -RepositoryPath $checkout -AssumeYes
    Assert-Equal -Expected 'v2' -Actual (Get-Content -LiteralPath (Join-Path $checkout 'version.txt') -Raw) -Message 'updater should fast-forward clean checkout'
    [System.IO.File]::WriteAllText((Join-Path $checkout 'version.txt'), 'dirty')
    $dirtyRejected = $false
    try {
        Update-MagicHandySource -RepositoryPath $checkout -AssumeYes
    } catch {
        $dirtyRejected = $_.Exception.Message -match 'local changes'
    }
    Assert-True -Condition $dirtyRejected -Message 'updater should reject a dirty checkout'

    Write-Host 'Checking updater follows a live feature upstream...'
    $activeCheckout = Join-Path $tempRoot 'active-feature-checkout'
    & $git -C $seed switch -c active-feature | Out-Null
    [System.IO.File]::WriteAllText((Join-Path $seed 'active.txt'), 'feature v1')
    & $git -C $seed add active.txt
    & $git -C $seed commit -m 'active feature' | Out-Null
    & $git -C $seed push -u origin active-feature | Out-Null
    & $git clone --branch active-feature $remote $activeCheckout | Out-Null
    [System.IO.File]::WriteAllText((Join-Path $seed 'active.txt'), 'feature v2')
    & $git -C $seed add active.txt
    & $git -C $seed commit -m 'advance active feature' | Out-Null
    & $git -C $seed push | Out-Null
    Update-MagicHandySource -RepositoryPath $activeCheckout -AssumeYes
    Assert-Equal -Expected 'active-feature' -Actual (& $git -C $activeCheckout branch --show-current) -Message 'live feature update should retain its branch'
    Assert-Equal -Expected 'feature v2' -Actual (Get-Content -LiteralPath (Join-Path $activeCheckout 'active.txt') -Raw) -Message 'live feature should follow its own upstream'
    & $git -C $seed switch main | Out-Null
    & $git -C $seed push origin --delete active-feature | Out-Null

    Write-Host 'Checking updater fallback for a merged and deleted feature branch...'
    $mergedCheckout = Join-Path $tempRoot 'merged-feature-checkout'
    & $git -C $seed switch -c merged-feature | Out-Null
    [System.IO.File]::WriteAllText((Join-Path $seed 'feature.txt'), 'merged feature')
    & $git -C $seed add feature.txt
    & $git -C $seed commit -m 'merged feature' | Out-Null
    & $git -C $seed push -u origin merged-feature | Out-Null
    & $git clone --single-branch --branch merged-feature $remote $mergedCheckout | Out-Null
    & $git -C $seed switch main | Out-Null
    & $git -C $seed merge --no-ff merged-feature -m 'merge feature' | Out-Null
    & $git -C $seed push origin main | Out-Null
    & $git -C $seed push origin --delete merged-feature | Out-Null
    Update-MagicHandySource -RepositoryPath $mergedCheckout -AssumeYes
    Assert-Equal -Expected 'merged-feature' -Actual (& $git -C $mergedCheckout branch --show-current) -Message 'deleted-feature fallback should retain the local branch name'
    Assert-Equal -Expected (& $git -C $mergedCheckout rev-parse refs/remotes/origin/main) -Actual (& $git -C $mergedCheckout rev-parse HEAD) -Message 'merged deleted feature should fast-forward to origin/main'
    Assert-Equal -Expected 'refs/heads/merged-feature' -Actual (& $git -C $mergedCheckout config --get branch.merged-feature.merge) -Message 'fallback should not rewrite upstream configuration'

    Write-Host 'Checking updater refusal for an unmerged deleted feature branch...'
    $unmergedCheckout = Join-Path $tempRoot 'unmerged-feature-checkout'
    & $git -C $seed switch -c unmerged-feature | Out-Null
    [System.IO.File]::WriteAllText((Join-Path $seed 'unmerged.txt'), 'local feature work')
    & $git -C $seed add unmerged.txt
    & $git -C $seed commit -m 'unmerged feature' | Out-Null
    & $git -C $seed push -u origin unmerged-feature | Out-Null
    & $git clone --branch unmerged-feature $remote $unmergedCheckout | Out-Null
    $unmergedHead = & $git -C $unmergedCheckout rev-parse HEAD
    & $git -C $seed switch main | Out-Null
    [System.IO.File]::WriteAllText((Join-Path $seed 'main-only.txt'), 'new release work')
    & $git -C $seed add main-only.txt
    & $git -C $seed commit -m 'advance main' | Out-Null
    & $git -C $seed push origin main | Out-Null
    & $git -C $seed push origin --delete unmerged-feature | Out-Null
    $unmergedRejected = $false
    try {
        Update-MagicHandySource -RepositoryPath $unmergedCheckout -AssumeYes
    } catch {
        $unmergedRejected = $_.Exception.Message -match 'contains commits not present'
    }
    Assert-True -Condition $unmergedRejected -Message 'updater should reject an unmerged feature whose upstream was deleted'
    Assert-Equal -Expected $unmergedHead -Actual (& $git -C $unmergedCheckout rev-parse HEAD) -Message 'unmerged deleted feature should keep its HEAD'
    Assert-Equal -Expected 'unmerged-feature' -Actual (& $git -C $unmergedCheckout branch --show-current) -Message 'unmerged deleted feature should keep its branch'

    Write-Host 'Checking install.ps1 plan-only behavior...'
    $freshPlanState = Join-Path $tempRoot 'fresh-plan-state.json'
    $guidedPlan = (& (Join-Path $Repo 'install.ps1') `
        -NoLaunch `
        -PlanOnly `
        -StatePath $freshPlanState 6>&1 | Out-String)
    Assert-True -Condition ($guidedPlan -match 'Optional device, model, and voice choices will open in the MagicHandy setup wizard') -Message 'plain installation should explain GUI-owned choices'
    Assert-PlanExcludes -Plan @($guidedPlan -split "`r?`n") -Pattern 'Ensure Git and CMake|Visual Studio C\+\+|CUDA Toolkit|Ensure Ollama is installed|Install checksum-verified Parakeet|Bootstrap uv'
    Assert-True -Condition (-not (Test-Path -LiteralPath $freshPlanState)) -Message 'guided install plan must not persist state'

    & (Join-Path $Repo 'install.ps1') `
        -Yes `
        -SkipLlamaBuild `
        -SkipParakeet `
        -NoLauncher `
        -NoLaunch `
        -PlanOnly `
        -StatePath $freshPlanState | Out-Host
    Assert-True -Condition (-not (Test-Path -LiteralPath $freshPlanState)) -Message 'install plan must not persist state'

    $ttsFreshPlanState = Join-Path $tempRoot 'fresh-tts-plan-state.json'
    $ttsFreshPlan = (& (Join-Path $Repo 'install.ps1') `
        -Yes `
        -SkipLlamaBuild `
        -SkipParakeet `
        -TTSModule chatterbox `
        -TTSDevice cpu `
        -NoLauncher `
        -NoLaunch `
        -PlanOnly `
        -StatePath $ttsFreshPlanState 6>&1 | Out-String)
    Assert-True -Condition ($ttsFreshPlan -match 'Bootstrap uv and managed Python.+chatterbox local TTS \(cpu; auto-launch: yes\)') -Message 'main installer should include selected TTS bootstrap in its plan'
    Assert-True -Condition ($ttsFreshPlan -match 'Installer-managed local TTS: chatterbox \(cpu; auto-launch: yes\)') -Message 'main installer should summarize selected TTS choices'
    Assert-True -Condition (-not (Test-Path -LiteralPath $ttsFreshPlanState)) -Message 'TTS install plan must not persist state'

    $qwenFreshPlan = (& (Join-Path $Repo 'install.ps1') `
        -Yes `
        -SkipLlamaBuild `
        -SkipParakeet `
        -TTSModule faster-qwen3-tts `
        -TTSDevice cuda `
        -NoLauncher `
        -NoLaunch `
        -PlanOnly `
        -StatePath $ttsFreshPlanState 6>&1 | Out-String)
    Assert-True -Condition ($qwenFreshPlan -match 'faster-qwen3-tts local TTS \(cuda; auto-launch: yes\)') -Message 'main installer should plan Faster Qwen without command-line reference data'
    Assert-True -Condition (-not (Test-Path -LiteralPath $ttsFreshPlanState)) -Message 'Faster Qwen install plan must not persist state'
    Assert-Throws -Action {
        & (Join-Path $Repo 'install.ps1') -Yes -TTSModule faster-qwen3-tts -TTSDevice cpu -PlanOnly -StatePath $ttsFreshPlanState
    } -Pattern 'cannot use CPU' -Message 'main installer Faster Qwen CPU selection'

    Write-Host 'Checking update.ps1 preserved-choice plan...'
    $beforeHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $statePath).Hash
    & (Join-Path $Repo 'update.ps1') `
        -Yes `
        -NoPull `
        -NoLaunch `
        -PlanOnly `
        -StatePath $statePath | Out-Host
    $afterHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $statePath).Hash
    Assert-Equal -Expected $beforeHash -Actual $afterHash -Message 'update plan must not rewrite saved choices'

    Write-Host 'Checking updater relative state-path stability across delegated scripts...'
    $relativeStateRoot = Join-Path $tempRoot 'relative-state-caller'
    $relativeStatePath = Join-Path $relativeStateRoot 'relative-state.json'
    New-Item -ItemType Directory -Force -Path $relativeStateRoot | Out-Null
    Copy-Item -LiteralPath $statePath -Destination $relativeStatePath
    $relativeBeforeHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $relativeStatePath).Hash
    Push-Location $relativeStateRoot
    try {
        & (Join-Path $Repo 'update.ps1') `
            -Yes `
            -NoPull `
            -NoLaunch `
            -PlanOnly `
            -StatePath '.\relative-state.json' | Out-Host
    } finally {
        Pop-Location
    }
    Assert-Equal -Expected $relativeBeforeHash -Actual ((Get-FileHash -Algorithm SHA256 -LiteralPath $relativeStatePath).Hash) -Message 'relative state path should resolve once in the updater caller context'

    Write-Host 'Checking updater runtime reconfiguration prompt...'
    $global:MagicHandyInstallerResponses = New-Object System.Collections.Generic.Queue[string]
    $global:MagicHandyInstallerPrompts = New-Object System.Collections.Generic.List[string]
    $global:MagicHandyInstallerResponses.Enqueue('y')
    function global:Read-Host {
        param([string]$Prompt)
        $global:MagicHandyInstallerPrompts.Add($Prompt)
        if ($global:MagicHandyInstallerResponses.Count -eq 0) {
            throw "No test response remains for prompt '$Prompt'."
        }
        return $global:MagicHandyInstallerResponses.Dequeue()
    }
    try {
        $reconfigureOutput = (& (Join-Path $Repo 'update.ps1') `
            -NoPull `
            -NoLaunch `
            -PlanOnly `
            -StatePath $statePath 6>&1 | Out-String)
    } finally {
        $remainingResponses = $global:MagicHandyInstallerResponses.Count
        $capturedPrompts = @($global:MagicHandyInstallerPrompts)
        Remove-Item Function:\global:Read-Host -ErrorAction SilentlyContinue
        Remove-Variable MagicHandyInstallerResponses -Scope Global -ErrorAction SilentlyContinue
        Remove-Variable MagicHandyInstallerPrompts -Scope Global -ErrorAction SilentlyContinue
    }
    Assert-True -Condition (($capturedPrompts -join "`n") -match 'Open guided setup after rebuilding') -Message 'updater should offer the app-owned setup flow'
    Assert-True -Condition ($reconfigureOutput -match 'Current source installation') -Message 'updater should show its compact source context'
    Assert-PlanExcludes -Plan @($reconfigureOutput -split "`r?`n") -Pattern 'Managed llama\.cpp:|Installer-managed local TTS:|Parakeet ASR:|Ensure Ollama:'
    Assert-PlanExcludes -Plan @($reconfigureOutput -split "`r?`n") -Pattern 'Ensure Git and CMake|Visual Studio C\+\+|CUDA Toolkit|Ensure Ollama is installed|Install checksum-verified Parakeet|Bootstrap uv'
    Assert-Equal -Expected $beforeHash -Actual ((Get-FileHash -Algorithm SHA256 -LiteralPath $statePath).Hash) -Message 'reconfiguration plan must not rewrite state'
    Assert-Equal -Expected 0 -Actual $remainingResponses -Message 'all expected prompts should be consumed'

    Write-Host 'Installer tests passed.' -ForegroundColor Green
} finally {
    $resolvedTemp = [System.IO.Path]::GetFullPath([System.IO.Path]::GetTempPath()).TrimEnd('\') + '\'
    $resolvedRoot = [System.IO.Path]::GetFullPath($tempRoot)
    if ($resolvedRoot.StartsWith($resolvedTemp, [StringComparison]::OrdinalIgnoreCase) -and (Test-Path -LiteralPath $resolvedRoot)) {
        Remove-Item -LiteralPath $resolvedRoot -Recurse -Force
    }
}
