Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$script:InstallStateSchema = 1
$script:MinimumGoVersion = [Version]'1.25.0'
$script:ParakeetRunnerURL = 'https://github.com/mudler/parakeet.cpp/releases/download/v0.4.0/parakeet-v0.4.0-bin-win-cpu-x64.zip'
$script:ParakeetRunnerSHA256 = '2880150a1bad2944baed46f2e6bb9f1bc55263a9f2bb85573785a7ec4fa35f27'
$script:ParakeetModelURL = 'https://huggingface.co/mudler/parakeet-cpp-gguf/resolve/main/tdt-0.6b-v3-q4_k.gguf?download=true'
$script:ParakeetModelSHA256 = '993d73feb4206dadda865ab25bd64b50c48dc4d013c3bf6126a721f28b1d5ee8'
$script:NeuTTSSourceURL = 'https://github.com/eugenehp/neutts-rs.git'
$script:NeuTTSSourceTag = 'v0.1.1'
$script:NeuTTSSourceCommit = 'ae7ea9a2a8d93e63eacdc1f10522ad3f92cc725f'
$script:NeuTTSRustToolchain = '1.94.0-x86_64-pc-windows-msvc'
$script:NeuTTSRunnerProtocol = 'magichandy_neutts_stream_v1'
$script:NeuTTSPhonemizer = 'espeak-ng'
$script:NeuTTSPhonemizerVersion = '1.52.0'
$script:NeuTTSBackboneRevision = '008555972590ff2c599dd43736ba31c81df3f0bf'
$script:NeuTTSBackboneURL = "https://huggingface.co/neuphonic/neutts-air-q4-gguf/resolve/$($script:NeuTTSBackboneRevision)/neutts-air-Q4_0.gguf?download=true"
$script:NeuTTSBackboneSHA256 = 'bf66dc21b7588fe720cbdfeac1595e7b7c780515f8d8f1ff9a29062e4ac9119e'
$script:NeuTTSCodecRevision = '30c1fdd19e68aee65d542cf043750d4c0165893e'
$script:NeuTTSCodecURL = "https://huggingface.co/neuphonic/neucodec/resolve/$($script:NeuTTSCodecRevision)/pytorch_model.bin?download=true"
$script:NeuTTSCodecSHA256 = '30c3ea13ceeb2de693c56e5e33a1b7e00d44c95dcdd08a4ed0d552d0bf59ebdf'
$script:NeuTTSEncoderRevision = '2cd5cf022b7a1e689e561f0492787768cfe8395d'
$script:NeuTTSEncoderModelURL = "https://huggingface.co/KevinAHM/distill-neucodec-onnx/resolve/$($script:NeuTTSEncoderRevision)/onnx/distill_neucodec_encoder.onnx?download=true"
$script:NeuTTSEncoderModelSHA256 = '04af54f6af51a7573a8bbcfd691b4f2c68b6dbd03aef72b983cbb4e5140c3a23'
$script:NeuTTSEncoderWeightsURL = "https://huggingface.co/KevinAHM/distill-neucodec-onnx/resolve/$($script:NeuTTSEncoderRevision)/onnx/distill_neucodec_encoder.onnx.data?download=true"
$script:NeuTTSEncoderWeightsSHA256 = '935859ed7904671dc82da1c533b9bf2fd8bcf6d8fc702bdba5bc25c8f7329e4f'

function Write-InstallerHeading([string]$Text) {
    Write-Host ''
    Write-Host $Text -ForegroundColor Cyan
    Write-Host ('-' * $Text.Length) -ForegroundColor DarkGray
}

function Write-MagicHandyBanner {
    [CmdletBinding()]
    param([ValidateSet('Install', 'Update')][string]$Operation)

    $art = @'
  __  __             _      _   _                 _
 |  \/  | __ _  __ _(_) ___| | | | __ _ _ __   __| |_   _
 | |\/| |/ _` |/ _` | |/ __| |_| |/ _` | '_ \ / _` | | | |
 | |  | | (_| | (_| | | (__|  _  | (_| | | | | (_| | |_| |
 |_|  |_|\__,_|\__, |_|\___|_| |_|\__,_|_| |_|\__,_|\__, |
               |___/                                 |___/
'@
    Write-Host $art -ForegroundColor Cyan
    Write-Host ("  {0} - local-first AI control for The Handy" -f $Operation.ToUpperInvariant()) -ForegroundColor White
    Write-Host '  Adults only. Keep Emergency Stop within reach.' -ForegroundColor DarkGray
    Write-Host ''
}

function Write-MagicHandyCompletionArt {
    [CmdletBinding()]
    param([ValidateSet('Install', 'Update', 'InstallPlan', 'UpdatePlan')][string]$Operation)

    $title = switch ($Operation) {
        'Install' { 'INSTALL COMPLETE' }
        'Update' { 'UPDATE COMPLETE' }
        'InstallPlan' { 'INSTALL PLAN READY' }
        'UpdatePlan' { 'UPDATE PLAN READY' }
    }
    $status = if ($Operation -like '*Plan') { 'NO CHANGES MADE' } else { 'APP BUILD VERIFIED - CONFIGURATION REQUIRED' }
    $detail = switch ($Operation) {
        'Install' { 'Open Settings to select a model, voice provider, and device transport. Managed NeuTTS can create reference codes locally from a WAV and exact transcript.' }
        'Update' { 'Congratulations. Saved installation choices were reapplied to the current build.' }
        default { 'Review the plan above, then rerun without -PlanOnly when ready.' }
    }
    $art = @"
  +----------------------------------------------------------+
  |  MAGIC HANDY  $($title.PadRight(43))|
  +----------------------------------------------------------+
             ||==============================[]
             ||  $status
  $detail
  Emergency Stop is always on-screen and available with Esc.
"@
    Write-Host ''
    Write-Host $art -ForegroundColor Green
}

function Confirm-MagicHandyChoice {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)]
        [string]$Question,
        [bool]$Default = $true,
        [switch]$AssumeYes
    )

    if ($AssumeYes) {
        return $true
    }
    $hint = if ($Default) { 'Y/n' } else { 'y/N' }
    $answer = Read-Host "$Question [$hint]"
    if ([string]::IsNullOrWhiteSpace($answer)) {
        return $Default
    }
    return $answer -match '^(?i:y|yes)$'
}

function Read-MagicHandyValue {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)]
        [string]$Question,
        [string]$Default = ''
    )

    $display = if ([string]::IsNullOrWhiteSpace($Default)) { '' } else { " [$Default]" }
    $answer = Read-Host "$Question$display"
    if ([string]::IsNullOrWhiteSpace($answer)) {
        return $Default
    }
    return $answer.Trim()
}

function Read-MagicHandyOptionalValue {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)]
        [string]$Question,
        [string]$Default = ''
    )

    $display = if ([string]::IsNullOrWhiteSpace($Default)) { '' } else { " [$Default]" }
    $answer = Read-Host "$Question$display (enter - to clear)"
    if ([string]::IsNullOrWhiteSpace($answer)) {
        return $Default
    }
    if ($answer.Trim() -eq '-') {
        return ''
    }
    return $answer.Trim()
}

function Read-MagicHandyBackend {
    [CmdletBinding()]
    param(
        [ValidateSet('cpu', 'cuda')]
        [string]$Default = 'cpu'
    )

    while ($true) {
        $answer = Read-MagicHandyValue -Question 'Managed llama.cpp backend (cpu or cuda)' -Default $Default
        $answer = $answer.ToLowerInvariant()
        if ($answer -in @('cpu', 'cuda')) {
            return $answer
        }
        Write-Warning 'Enter cpu or cuda.'
    }
}

function Refresh-MagicHandyPath {
    $parts = New-Object System.Collections.Generic.List[string]
    foreach ($scope in @('Machine', 'User')) {
        $value = [Environment]::GetEnvironmentVariable('Path', $scope)
        if (-not [string]::IsNullOrWhiteSpace($value)) {
            foreach ($part in ($value -split ';')) {
                if (-not [string]::IsNullOrWhiteSpace($part) -and -not $parts.Contains($part)) {
                    $parts.Add($part)
                }
            }
        }
    }
    $env:Path = $parts -join ';'
}

function Resolve-MagicHandyExecutable {
    [CmdletBinding()]
    param([Parameter(Mandatory = $true)][string]$Name)

    $command = Get-Command $Name -ErrorAction SilentlyContinue
    if ($null -ne $command -and -not [string]::IsNullOrWhiteSpace($command.Source)) {
        return $command.Source
    }

    $candidates = switch ($Name.ToLowerInvariant()) {
        'go' { @((Join-Path $env:ProgramFiles 'Go\bin\go.exe')) }
        'git' { @((Join-Path $env:ProgramFiles 'Git\cmd\git.exe')) }
        'cmake' { @((Join-Path $env:ProgramFiles 'CMake\bin\cmake.exe')) }
        'winget' { @((Join-Path $env:LOCALAPPDATA 'Microsoft\WindowsApps\winget.exe')) }
        'ollama' { @((Join-Path $env:LOCALAPPDATA 'Programs\Ollama\ollama.exe')) }
        'rustup' { @((Join-Path $env:USERPROFILE '.cargo\bin\rustup.exe')) }
        default { @() }
    }
    foreach ($candidate in $candidates) {
        if (Test-Path -LiteralPath $candidate) {
            return $candidate
        }
    }

    if ($Name -eq 'nvcc') {
        $cudaRoot = Join-Path $env:ProgramFiles 'NVIDIA GPU Computing Toolkit\CUDA'
        if (Test-Path -LiteralPath $cudaRoot) {
            $candidate = Get-ChildItem -LiteralPath $cudaRoot -Directory -ErrorAction SilentlyContinue |
                Sort-Object Name -Descending |
                ForEach-Object { Join-Path $_.FullName 'bin\nvcc.exe' } |
                Where-Object { Test-Path -LiteralPath $_ } |
                Select-Object -First 1
            if ($candidate) {
                return $candidate
            }
        }
    }
    return $null
}

function Resolve-MagicHandyCMake {
    $cmake = Resolve-MagicHandyExecutable -Name 'cmake'
    if ($cmake) {
        return $cmake
    }

    $vswhere = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio\Installer\vswhere.exe'
    if (Test-Path -LiteralPath $vswhere) {
        $candidate = & $vswhere -latest -products * -find 'Common7\IDE\CommonExtensions\Microsoft\CMake\CMake\bin\cmake.exe' |
            Select-Object -First 1
        if ($candidate -and (Test-Path -LiteralPath $candidate)) {
            return $candidate
        }
    }
    return $null
}

function Test-MagicHandyVCToolchain {
    $vswhere = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio\Installer\vswhere.exe'
    if (-not (Test-Path -LiteralPath $vswhere)) {
        return $false
    }
    $installation = & $vswhere -latest -products * -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
    if ([string]::IsNullOrWhiteSpace(($installation | Select-Object -First 1))) {
        return $false
    }
    $sdkRoot = Join-Path ${env:ProgramFiles(x86)} 'Windows Kits\10\Include'
    if (-not (Test-Path -LiteralPath $sdkRoot -PathType Container)) {
        return $false
    }
    return [bool](Get-ChildItem -LiteralPath $sdkRoot -Directory -ErrorAction SilentlyContinue | Where-Object {
        Test-Path -LiteralPath (Join-Path $_.FullName 'um\Windows.h') -PathType Leaf
    } | Select-Object -First 1)
}

function Get-MagicHandyInstallStatePath {
    $root = if (-not [string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) {
        $env:LOCALAPPDATA
    } elseif (-not [string]::IsNullOrWhiteSpace($env:APPDATA)) {
        $env:APPDATA
    } else {
        [Environment]::GetFolderPath('LocalApplicationData')
    }
    return Join-Path $root 'MagicHandy\install-state.json'
}

function Write-MagicHandyUTF8 {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Content
    )
    $encoding = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($Path, $Content, $encoding)
}

function Assert-MagicHandyChildPath {
    param(
        [Parameter(Mandatory = $true)][string]$Root,
        [Parameter(Mandatory = $true)][string]$Candidate
    )
    $rootPrefix = [System.IO.Path]::GetFullPath($Root).TrimEnd('\') + '\'
    $resolvedCandidate = [System.IO.Path]::GetFullPath($Candidate)
    if (-not $resolvedCandidate.StartsWith($rootPrefix, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing to modify a path outside '$rootPrefix': $resolvedCandidate"
    }
}

function New-MagicHandyInstallState {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)][string]$RepositoryPath,
        [Parameter(Mandatory = $true)][string]$DataDir,
        [Parameter(Mandatory = $true)][ValidateRange(1, 65535)][int]$Port,
        [Parameter(Mandatory = $true)][bool]$SetupLLM,
        [Parameter(Mandatory = $true)][bool]$BuildManagedLlama,
        [Parameter(Mandatory = $true)][ValidateSet('cpu', 'cuda')][string]$LlamaBackend,
        [Parameter(Mandatory = $true)][bool]$EnsureOllama,
        [string]$OllamaModel = '',
        [Parameter(Mandatory = $true)][bool]$InstallParakeet,
        [Parameter(Mandatory = $true)][bool]$CreateLauncher,
        [string]$InstalledAt = ''
    )

    $now = [DateTimeOffset]::UtcNow.ToString('o')
    if ([string]::IsNullOrWhiteSpace($InstalledAt)) {
        $InstalledAt = $now
    }
    return [pscustomobject][ordered]@{
        schema_version = $script:InstallStateSchema
        installed_at = $InstalledAt
        updated_at = $now
        repository_path = [System.IO.Path]::GetFullPath($RepositoryPath)
        data_dir = [System.IO.Path]::GetFullPath($DataDir)
        port = $Port
        setup_llm = $SetupLLM
        build_managed_llama = $BuildManagedLlama
        llama_backend = $LlamaBackend
        ensure_ollama = $EnsureOllama
        ollama_model = $OllamaModel.Trim()
        install_parakeet = $InstallParakeet
        create_launcher = $CreateLauncher
    }
}

function Read-MagicHandyInstallState {
    [CmdletBinding()]
    param([Parameter(Mandatory = $true)][string]$Path)

    if (-not (Test-Path -LiteralPath $Path)) {
        throw "No installer state exists at '$Path'. Run install.ps1 first."
    }
    try {
        $state = Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json
    } catch {
        throw "Installer state '$Path' is not valid JSON: $_"
    }
    $required = @(
        'schema_version', 'installed_at', 'updated_at', 'repository_path', 'data_dir', 'port',
        'setup_llm', 'build_managed_llama', 'llama_backend', 'ensure_ollama',
        'ollama_model', 'install_parakeet', 'create_launcher'
    )
    foreach ($name in $required) {
        if ($state.PSObject.Properties.Name -notcontains $name) {
            throw "Installer state '$Path' is missing '$name'."
        }
    }
    if ([int]$state.schema_version -ne $script:InstallStateSchema) {
        throw "Installer state schema $($state.schema_version) is unsupported. Expected $script:InstallStateSchema."
    }
    if ([int]$state.port -lt 1 -or [int]$state.port -gt 65535) {
        throw "Installer state '$Path' has an invalid port."
    }
    if ([string]$state.llama_backend -notin @('cpu', 'cuda')) {
        throw "Installer state '$Path' has an invalid llama.cpp backend."
    }
    return $state
}

function Write-MagicHandyInstallState {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)][object]$State,
        [Parameter(Mandatory = $true)][string]$Path
    )

    $parent = Split-Path -Parent ([System.IO.Path]::GetFullPath($Path))
    New-Item -ItemType Directory -Force -Path $parent | Out-Null
    $partial = "$Path.partial-$PID"
    try {
        Write-MagicHandyUTF8 -Path $partial -Content ($State | ConvertTo-Json -Depth 5)
        Move-Item -LiteralPath $partial -Destination $Path -Force
    } finally {
        if (Test-Path -LiteralPath $partial) {
            Remove-Item -LiteralPath $partial -Force -ErrorAction SilentlyContinue
        }
    }
}

function Show-MagicHandyInstallState {
    [CmdletBinding()]
    param([Parameter(Mandatory = $true)][object]$State)

    $managed = if ([bool]$State.build_managed_llama) { "yes ($($State.llama_backend))" } else { 'no' }
    $neutts = if ([bool]$State.build_managed_llama) { 'yes (with managed llama.cpp)' } else { 'no (managed llama.cpp skipped)' }
    $ollama = if ([bool]$State.ensure_ollama) { 'yes' } else { 'no' }
    $parakeet = if ([bool]$State.install_parakeet) { 'yes' } else { 'no' }
    $launcher = if ([bool]$State.create_launcher) { 'yes' } else { 'no' }
    Write-Host "  Data directory:   $($State.data_dir)"
    Write-Host "  Local port:       $($State.port)"
    Write-Host "  Managed llama.cpp: $managed"
    Write-Host "  NeuTTS runtime:   $neutts"
    Write-Host "  Ensure Ollama:    $ollama"
    Write-Host "  Ollama model:     $(if ([string]::IsNullOrWhiteSpace([string]$State.ollama_model)) { '(unchanged)' } else { $State.ollama_model })"
    Write-Host "  Parakeet ASR:     $parakeet"
    Write-Host "  Launcher:         $launcher"
}

function Get-MagicHandyProvisionPlan {
    [CmdletBinding()]
    param([Parameter(Mandatory = $true)][object]$State)

    $plan = New-Object System.Collections.Generic.List[string]
    $plan.Add('Ensure Go 1.25+ is installed')
    $plan.Add('Build magichandy.exe with CGO disabled')
    $plan.Add('Build Parakeet, NeuTTS Air, and ElevenLabs Go protocol adapters')
    if ([bool]$State.build_managed_llama) {
        $plan.Add('Ensure Git and CMake are installed')
        $plan.Add('Ensure the Visual Studio C++ Build Tools workload and Windows SDK are installed')
        if ([string]$State.llama_backend -eq 'cuda') {
            $plan.Add('Ensure the NVIDIA CUDA Toolkit is installed')
        }
        $plan.Add("Build and activate pinned managed llama.cpp ($($State.llama_backend))")
        $plan.Add('Ensure LLVM/libclang, Rustup, and the pinned Rust 1.94.0 Windows MSVC toolchain are installed')
        $plan.Add("Ensure eSpeak NG $($script:NeuTTSPhonemizerVersion)+ is installed for NeuTTS phonemization")
        $neuttsAcceleration = if ([string]$State.llama_backend -eq 'cuda') { 'CUDA backbone + WGPU codec' } else { 'CPU backbone + CPU codec' }
        $neuttsInstalledSize = if ([string]$State.llama_backend -eq 'cuda') { 'about 2.0 GiB installed' } else { 'about 1.9 GiB installed' }
        $plan.Add("Build MagicHandy's persistent NeuTTS runner from pinned neutts-rs ($neuttsAcceleration)")
        $plan.Add('Build the MagicHandy NeuCodec ONNX reference encoder worker')
        $plan.Add("Install checksum-verified NeuTTS Air Q4, NeuCodec decoder, and reference encoder assets ($neuttsInstalledSize; about 1.3 GiB additional transient download)")
    } else {
        $plan.Add('Skip NeuTTS runtime build and model assets because managed llama.cpp is not selected')
    }
    if ([bool]$State.ensure_ollama) {
        $plan.Add('Ensure Ollama is installed')
        if (-not [string]::IsNullOrWhiteSpace([string]$State.ollama_model)) {
            $plan.Add("Ensure Ollama model '$($State.ollama_model)' is present")
        }
    }
    if ([bool]$State.install_parakeet) {
        $plan.Add('Install checksum-verified Parakeet CPU runner and 644 MiB model')
    }
    if ([bool]$State.create_launcher) {
        $plan.Add('Write Start-MagicHandy.ps1')
    }
    return $plan.ToArray()
}

function Ensure-MagicHandyWinGet {
    [CmdletBinding()]
    param([switch]$AssumeYes)

    $winget = Resolve-MagicHandyExecutable -Name 'winget'
    if ($winget) {
        return $winget
    }

    Write-Host 'Windows Package Manager is missing. The installer can repair/install it using the official Microsoft.WinGet.Client PowerShell module.'
    Write-Host 'This installs the NuGet provider and Microsoft.WinGet.Client for the current user.' -ForegroundColor DarkGray
    if (-not (Confirm-MagicHandyChoice -Question 'Install Windows Package Manager now?' -Default $true -AssumeYes:$AssumeYes)) {
        throw 'Windows Package Manager is required to provision a bare machine.'
    }
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    Install-PackageProvider -Name NuGet -Force -Scope CurrentUser | Out-Null
    Install-Module -Name Microsoft.WinGet.Client -Force -Repository PSGallery -Scope CurrentUser | Out-Null
    Import-Module Microsoft.WinGet.Client -Force
    Repair-WinGetPackageManager -Force -Latest | Out-Host
    Refresh-MagicHandyPath
    $winget = Resolve-MagicHandyExecutable -Name 'winget'
    if (-not $winget) {
        throw 'Windows Package Manager installation completed but winget.exe is still unavailable. Restart PowerShell and rerun install.ps1.'
    }
    return $winget
}

function Invoke-MagicHandyWinGetInstall {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)][string]$ID,
        [string]$Override = '',
        [switch]$AssumeYes
    )

    $winget = Ensure-MagicHandyWinGet -AssumeYes:$AssumeYes
    $arguments = @(
        'install', '--id', $ID, '--exact', '--source', 'winget',
        '--accept-source-agreements', '--accept-package-agreements', '--disable-interactivity'
    )
    if ([string]::IsNullOrWhiteSpace($Override)) {
        $arguments += '--silent'
    } else {
        # Build Tools may already exist without C++; force the installer so the
        # requested workload is applied instead of treating it as up to date.
        $arguments += @('--override', $Override, '--force')
    }
    & $winget @arguments | Out-Host
    $exitCode = $LASTEXITCODE
    if ($exitCode -notin @(0, 3010)) {
        throw "winget could not install $ID (exit $exitCode)."
    }
    if ($exitCode -eq 3010) {
        Write-Warning "$ID requested a restart. The installer will verify whether it can continue first."
    }
    Refresh-MagicHandyPath
}

function Confirm-MagicHandyPackageInstall {
    param(
        [Parameter(Mandatory = $true)][string]$Name,
        [Parameter(Mandatory = $true)][string]$Purpose,
        [Parameter(Mandatory = $true)][string]$License,
        [string]$Size = '',
        [switch]$AssumeYes
    )
    Write-Host "$Name is required for $Purpose."
    Write-Host "License: $License$(if ($Size) { "; approximate disk use: $Size" } else { '' })." -ForegroundColor DarkGray
    if (-not (Confirm-MagicHandyChoice -Question "Install $Name now?" -Default $true -AssumeYes:$AssumeYes)) {
        throw "$Name is required for the selected installation choices."
    }
}

function Get-MagicHandyGoVersion {
    $go = Resolve-MagicHandyExecutable -Name 'go'
    if (-not $go) {
        return $null
    }
    $output = & $go version
    if ($LASTEXITCODE -ne 0 -or $output -notmatch 'go(\d+)\.(\d+)(?:\.(\d+))?') {
        return $null
    }
    $patch = if ($Matches[3]) { [int]$Matches[3] } else { 0 }
    return [Version]::new([int]$Matches[1], [int]$Matches[2], $patch)
}

function Ensure-MagicHandyGo {
    [CmdletBinding()]
    param([switch]$AssumeYes)

    $version = Get-MagicHandyGoVersion
    if ($null -eq $version -or $version -lt $script:MinimumGoVersion) {
        Confirm-MagicHandyPackageInstall -Name 'Go' -Purpose 'building the pure-Go application and workers' -License 'BSD-3-Clause; https://go.dev/LICENSE' -AssumeYes:$AssumeYes
        Invoke-MagicHandyWinGetInstall -ID 'GoLang.Go' -AssumeYes:$AssumeYes
        $version = Get-MagicHandyGoVersion
    }
    if ($null -eq $version -or $version -lt $script:MinimumGoVersion) {
        throw "Go $script:MinimumGoVersion or newer is required. Restart PowerShell and rerun install.ps1 if Go was just installed."
    }
    $go = Resolve-MagicHandyExecutable -Name 'go'
    Write-Host "Found $(& $go version)" -ForegroundColor Green
    return $go
}

function Ensure-MagicHandyGit {
    [CmdletBinding()]
    param([switch]$AssumeYes)

    $git = Resolve-MagicHandyExecutable -Name 'git'
    if (-not $git) {
        Confirm-MagicHandyPackageInstall -Name 'Git for Windows' -Purpose 'updating MagicHandy and fetching pinned llama.cpp and NeuTTS source' -License 'GPL-2.0; https://gitforwindows.org/' -AssumeYes:$AssumeYes
        Invoke-MagicHandyWinGetInstall -ID 'Git.Git' -AssumeYes:$AssumeYes
        $git = Resolve-MagicHandyExecutable -Name 'git'
    }
    if (-not $git) {
        throw 'Git installation completed but git.exe is unavailable. Restart PowerShell and rerun the script.'
    }
    return $git
}

function Resolve-MagicHandyESpeak {
    $resolved = Resolve-MagicHandyExecutable -Name 'espeak-ng'
    if ($resolved) {
        return $resolved
    }
    $candidates = @(
        (Join-Path $env:ProgramFiles 'eSpeak NG\espeak-ng.exe')
    )
    if (${env:ProgramFiles(x86)}) {
        $candidates += Join-Path ${env:ProgramFiles(x86)} 'eSpeak NG\espeak-ng.exe'
    }
    foreach ($candidate in $candidates) {
        if (Test-Path -LiteralPath $candidate -PathType Leaf) {
            return [System.IO.Path]::GetFullPath($candidate)
        }
    }
    return $null
}

function Get-MagicHandyESpeakVersion {
    param([Parameter(Mandatory = $true)][string]$Executable)

    $output = (& $Executable --version 2>&1) -join "`n"
    if ($LASTEXITCODE -ne 0 -or $output -notmatch '(\d+\.\d+\.\d+)') {
        return $null
    }
    return [Version]$Matches[1]
}

function Ensure-MagicHandyESpeak {
    [CmdletBinding()]
    param([switch]$AssumeYes)

    $executable = Resolve-MagicHandyESpeak
    $version = if ($executable) { Get-MagicHandyESpeakVersion -Executable $executable } else { $null }
    if ($null -eq $version -or $version -lt [Version]$script:NeuTTSPhonemizerVersion) {
        Confirm-MagicHandyPackageInstall -Name 'eSpeak NG' -Purpose 'producing the phonemes NeuTTS was trained to synthesize' -License 'GPL-3.0-or-later; https://github.com/espeak-ng/espeak-ng' -Size 'about 25 MB' -AssumeYes:$AssumeYes
        Invoke-MagicHandyWinGetInstall -ID 'eSpeak-NG.eSpeak-NG' -AssumeYes:$AssumeYes
        $executable = Resolve-MagicHandyESpeak
        $version = if ($executable) { Get-MagicHandyESpeakVersion -Executable $executable } else { $null }
    }
    if ($null -eq $version -or $version -lt [Version]$script:NeuTTSPhonemizerVersion) {
        throw "eSpeak NG $($script:NeuTTSPhonemizerVersion) or newer is required for intelligible NeuTTS output."
    }
    Write-Host "Found eSpeak NG $version at $executable" -ForegroundColor Green
    return $executable
}

function Ensure-MagicHandyCMake {
    [CmdletBinding()]
    param([switch]$AssumeYes)

    $cmake = Resolve-MagicHandyCMake
    if (-not $cmake) {
        Confirm-MagicHandyPackageInstall -Name 'CMake' -Purpose 'configuring the managed llama.cpp and NeuTTS source builds' -License 'BSD-3-Clause; https://cmake.org/licensing/' -AssumeYes:$AssumeYes
        Invoke-MagicHandyWinGetInstall -ID 'Kitware.CMake' -AssumeYes:$AssumeYes
        $cmake = Resolve-MagicHandyCMake
    }
    if (-not $cmake) {
        throw 'CMake installation completed but cmake.exe is unavailable. Restart PowerShell and rerun the script.'
    }
    return $cmake
}

function Ensure-MagicHandyVCToolchain {
    [CmdletBinding()]
    param([switch]$AssumeYes)

    if (Test-MagicHandyVCToolchain) {
        return
    }
    Confirm-MagicHandyPackageInstall -Name 'Visual Studio Build Tools with Desktop C++' -Purpose 'compiling the managed llama.cpp and NeuTTS runners' -License 'Microsoft Visual Studio license; https://visualstudio.microsoft.com/license-terms/' -Size 'several GB' -AssumeYes:$AssumeYes
    $override = '--wait --quiet --norestart --nocache --add Microsoft.VisualStudio.Workload.VCTools --includeRecommended'
    Invoke-MagicHandyWinGetInstall -ID 'Microsoft.VisualStudio.BuildTools' -Override $override -AssumeYes:$AssumeYes
    if (-not (Test-MagicHandyVCToolchain)) {
        throw 'The Visual Studio C++ workload is still unavailable. Restart Windows if requested, then rerun install.ps1.'
    }
}

function Resolve-MagicHandyLibClang {
    $candidates = @(
        (Join-Path $env:ProgramFiles 'LLVM\bin\libclang.dll')
    )
    foreach ($candidate in $candidates) {
        if (Test-Path -LiteralPath $candidate -PathType Leaf) {
            return (Split-Path -Parent $candidate)
        }
    }
    return $null
}

function Ensure-MagicHandyRustup {
    [CmdletBinding()]
    param([switch]$AssumeYes)

    $rustup = Resolve-MagicHandyExecutable -Name 'rustup'
    if (-not $rustup) {
        Confirm-MagicHandyPackageInstall -Name 'Rustup' -Purpose 'building the selected NeuTTS stream_pcm runner and its llama.cpp binding' -License 'Apache-2.0 or MIT; https://github.com/rust-lang/rustup' -Size 'toolchain and build cache use several GB temporarily' -AssumeYes:$AssumeYes
        Invoke-MagicHandyWinGetInstall -ID 'Rustlang.Rustup' -AssumeYes:$AssumeYes
        $rustup = Resolve-MagicHandyExecutable -Name 'rustup'
    }
    if (-not $rustup) {
        throw 'Rustup installation completed but rustup.exe is unavailable. Restart PowerShell and rerun install.ps1.'
    }
    & $rustup toolchain install $script:NeuTTSRustToolchain --profile minimal | Out-Host
    if ($LASTEXITCODE -ne 0) {
        throw "Rustup could not install $($script:NeuTTSRustToolchain) (exit $LASTEXITCODE)."
    }
    $version = & $rustup run $script:NeuTTSRustToolchain rustc -vV
    if ($LASTEXITCODE -ne 0 -or
        ($version | Select-Object -First 1) -notmatch '^rustc 1\.94\.0 ' -or
        ($version -join "`n") -notmatch 'host: x86_64-pc-windows-msvc') {
        throw "The Rust $($script:NeuTTSRustToolchain) toolchain is unavailable."
    }
    Write-Host (($version | Select-Object -First 1) + " ($($script:NeuTTSRustToolchain))") -ForegroundColor Green
    return $rustup
}

function Ensure-MagicHandyLibClang {
    [CmdletBinding()]
    param([switch]$AssumeYes)

    $libClang = Resolve-MagicHandyLibClang
    if (-not $libClang) {
        Confirm-MagicHandyPackageInstall -Name 'LLVM' -Purpose 'providing libclang for the NeuTTS llama.cpp Rust bindings' -License 'Apache-2.0 with LLVM exceptions; https://llvm.org/LICENSE.txt' -Size 'approximately 2 GB' -AssumeYes:$AssumeYes
        Invoke-MagicHandyWinGetInstall -ID 'LLVM.LLVM' -AssumeYes:$AssumeYes
        $libClang = Resolve-MagicHandyLibClang
    }
    if (-not $libClang) {
        throw 'LLVM installation completed but libclang.dll is unavailable. Restart PowerShell and rerun install.ps1.'
    }
    Write-Host "Found libclang at $libClang" -ForegroundColor Green
    return $libClang
}

function Ensure-MagicHandyCUDA {
    [CmdletBinding()]
    param([switch]$AssumeYes)

    $nvcc = Resolve-MagicHandyExecutable -Name 'nvcc'
    if (-not $nvcc) {
        Confirm-MagicHandyPackageInstall -Name 'NVIDIA CUDA Toolkit' -Purpose 'building the selected CUDA llama.cpp backend' -License 'NVIDIA CUDA Toolkit EULA; https://docs.nvidia.com/cuda/eula/' -Size 'several GB' -AssumeYes:$AssumeYes
        Invoke-MagicHandyWinGetInstall -ID 'Nvidia.CUDA' -AssumeYes:$AssumeYes
        $nvcc = Resolve-MagicHandyExecutable -Name 'nvcc'
    }
    if (-not $nvcc) {
        throw 'CUDA installation completed but nvcc.exe is unavailable. Restart Windows if requested, then rerun install.ps1.'
    }
    return $nvcc
}

function Ensure-MagicHandyOllama {
    [CmdletBinding()]
    param([switch]$AssumeYes)

    $ollama = Resolve-MagicHandyExecutable -Name 'ollama'
    if (-not $ollama) {
        Confirm-MagicHandyPackageInstall -Name 'Ollama' -Purpose 'the selected external local-LLM provider' -License 'MIT; https://github.com/ollama/ollama/blob/main/LICENSE' -AssumeYes:$AssumeYes
        Invoke-MagicHandyWinGetInstall -ID 'Ollama.Ollama' -AssumeYes:$AssumeYes
        $ollama = Resolve-MagicHandyExecutable -Name 'ollama'
    }
    if (-not $ollama) {
        throw 'Ollama installation completed but ollama.exe is unavailable. Restart PowerShell and rerun install.ps1.'
    }
    return $ollama
}

function Get-MagicHandySHA256([string]$Path) {
    return (Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash.ToLowerInvariant()
}

function Format-MagicHandyByteCount([long]$Bytes) {
    if ($Bytes -ge 1GB) {
        return ('{0:N2} GiB' -f ($Bytes / 1GB))
    }
    if ($Bytes -ge 1MB) {
        return ('{0:N1} MiB' -f ($Bytes / 1MB))
    }
    if ($Bytes -ge 1KB) {
        return ('{0:N1} KiB' -f ($Bytes / 1KB))
    }
    return "$Bytes B"
}

function Write-MagicHandyDownloadProgress {
    param(
        [Parameter(Mandatory = $true)][string]$Name,
        [Parameter(Mandatory = $true)][long]$CompletedBytes,
        [long]$TotalBytes = -1,
        [int]$PreviousWidth = 0
    )

    if ($TotalBytes -gt 0) {
        $fraction = [Math]::Min(1.0, [double]$CompletedBytes / [double]$TotalBytes)
        $filled = [int][Math]::Floor($fraction * 20)
        $bar = ('#' * $filled) + ('-' * (20 - $filled))
        $line = '{0} [{1}] {2,5:N1}%  {3} / {4}' -f $Name, $bar, ($fraction * 100), (Format-MagicHandyByteCount $CompletedBytes), (Format-MagicHandyByteCount $TotalBytes)
    } else {
        $line = '{0}  {1} downloaded' -f $Name, (Format-MagicHandyByteCount $CompletedBytes)
    }
    $padding = ' ' * [Math]::Max(0, $PreviousWidth - $line.Length)
    [Console]::Write("`r$line$padding")
    return $line.Length
}

function Install-MagicHandyVerifiedDownload {
    param(
        [Parameter(Mandatory = $true)][string]$Uri,
        [Parameter(Mandatory = $true)][string]$Destination,
        [Parameter(Mandatory = $true)][string]$ExpectedSHA256,
        [string]$PartialPath = '',
        [ValidateRange(1, 10)][int]$MaxAttempts = 6,
        [ValidateRange(0, 60000)][int]$RetryDelayMilliseconds = 1000
    )

    $downloadUri = $null
    if (-not [Uri]::TryCreate($Uri, [UriKind]::Absolute, [ref]$downloadUri) -or
        $downloadUri.Scheme -notin @('http', 'https')) {
        throw 'Download URI must be an absolute HTTP or HTTPS URI.'
    }
    if ($ExpectedSHA256 -notmatch '^[0-9a-fA-F]{64}$') {
        throw 'Expected download SHA-256 must contain exactly 64 hexadecimal characters.'
    }
    $expected = $ExpectedSHA256.ToLowerInvariant()
    $parent = Split-Path -Parent $Destination
    New-Item -ItemType Directory -Force -Path $parent | Out-Null
    if (Test-Path -LiteralPath $Destination) {
        if ((Get-MagicHandySHA256 -Path $Destination) -eq $expected) {
            Write-Host "Verified existing $(Split-Path -Leaf $Destination)." -ForegroundColor Green
            return
        }
        Write-Warning "Replacing $(Split-Path -Leaf $Destination) because its SHA-256 did not match."
        Remove-Item -LiteralPath $Destination -Force
    }

    $partial = if ([string]::IsNullOrWhiteSpace($PartialPath)) { "$Destination.$expected.partial" } else { $PartialPath }
    if ([System.IO.Path]::GetFullPath($partial) -eq [System.IO.Path]::GetFullPath($Destination)) {
        throw 'Download partial path must differ from its destination.'
    }
    $destinationVolume = [System.IO.Path]::GetPathRoot([System.IO.Path]::GetFullPath($Destination))
    $partialVolume = [System.IO.Path]::GetPathRoot([System.IO.Path]::GetFullPath($partial))
    if (-not $destinationVolume.Equals($partialVolume, [StringComparison]::OrdinalIgnoreCase)) {
        throw 'Download partial and destination paths must be on the same volume for atomic promotion.'
    }
    $partialParent = Split-Path -Parent $partial
    New-Item -ItemType Directory -Force -Path $partialParent | Out-Null
    if (Test-Path -LiteralPath $partial -PathType Container) {
        throw "Download partial path is a directory: '$partial'."
    }

    $name = Split-Path -Leaf $Destination
    $startingBytes = if (Test-Path -LiteralPath $partial -PathType Leaf) { (Get-Item -LiteralPath $partial).Length } else { 0 }
    if ($startingBytes -gt 0) {
        Write-Host "Resuming $name from $(Format-MagicHandyByteCount $startingBytes)..."
    } else {
        Write-Host "Downloading $name..."
    }

    $showInlineProgress = $Host.Name -eq 'ConsoleHost'
    try {
        if ([Console]::IsOutputRedirected) {
            $showInlineProgress = $false
        }
    } catch {
        $showInlineProgress = $false
    }

    $progressWidth = 0
    $rangeReset = $false
    $downloadComplete = $false
    $knownTotal = -1L
    $lastRetryReason = ''
    for ($attempt = 1; $attempt -le $MaxAttempts; $attempt++) {
        $response = $null
        $input = $null
        $output = $null
        $retryReason = ''
        $retryAfterMilliseconds = 0
        $offset = if (Test-Path -LiteralPath $partial -PathType Leaf) { (Get-Item -LiteralPath $partial).Length } else { 0 }
        try {
            $requestUri = $downloadUri
            $redirectCount = 0
            while ($true) {
                $request = [System.Net.HttpWebRequest][System.Net.WebRequest]::Create($requestUri)
                $request.Method = 'GET'
                $request.UserAgent = 'MagicHandy-Installer/1.0'
                $request.AllowAutoRedirect = $false
                $request.AutomaticDecompression = [System.Net.DecompressionMethods]::None
                $request.Headers['Accept-Encoding'] = 'identity'
                $request.Timeout = 60000
                $request.ReadWriteTimeout = 60000
                $request.CachePolicy = [System.Net.Cache.RequestCachePolicy]::new([System.Net.Cache.RequestCacheLevel]::NoCacheNoStore)
                if ($offset -gt 0) {
                    $request.AddRange([long]$offset)
                }

                try {
                    $response = [System.Net.HttpWebResponse]$request.GetResponse()
                } catch [System.Net.WebException] {
                    $webError = $_.Exception
                    $response = [System.Net.HttpWebResponse]$webError.Response
                    if ($null -ne $response) {
                        $statusCode = [int]$response.StatusCode
                        if ($statusCode -eq 416 -and $offset -gt 0) {
                            $contentRange = $response.Headers['Content-Range']
                            if ($contentRange -match '^bytes\s+\*/(\d+)$' -and [long]$Matches[1] -eq $offset) {
                                $knownTotal = $offset
                                $downloadComplete = $true
                            } elseif (-not $rangeReset) {
                                Remove-Item -LiteralPath $partial -Force -ErrorAction SilentlyContinue
                                $rangeReset = $true
                                $retryReason = 'the server rejected the saved partial; restarting from zero'
                            } else {
                                throw 'Download server repeatedly rejected the saved partial range.'
                            }
                        } elseif (@(408, 429, 500, 502, 503, 504) -contains $statusCode) {
                            $retryReason = "HTTP $statusCode"
                            $retryAfter = $response.Headers['Retry-After']
                            $retryAfterSeconds = 0
                            $retryAfterDate = [DateTimeOffset]::MinValue
                            if ([int]::TryParse($retryAfter, [ref]$retryAfterSeconds)) {
                                $retryAfterMilliseconds = [int][Math]::Min(30000.0, [Math]::Max(0.0, [double]$retryAfterSeconds * 1000.0))
                            } elseif ([DateTimeOffset]::TryParse($retryAfter, [ref]$retryAfterDate)) {
                                $retryAfterMilliseconds = [int][Math]::Min(30000.0, [Math]::Max(0.0, ($retryAfterDate - [DateTimeOffset]::UtcNow).TotalMilliseconds))
                            }
                        } else {
                            throw "Download request failed with HTTP $statusCode ($($response.StatusDescription))."
                        }
                    } elseif (@(
                            [System.Net.WebExceptionStatus]::NameResolutionFailure,
                            [System.Net.WebExceptionStatus]::ProxyNameResolutionFailure,
                            [System.Net.WebExceptionStatus]::ConnectFailure,
                            [System.Net.WebExceptionStatus]::ConnectionClosed,
                            [System.Net.WebExceptionStatus]::KeepAliveFailure,
                            [System.Net.WebExceptionStatus]::PipelineFailure,
                            [System.Net.WebExceptionStatus]::ReceiveFailure,
                            [System.Net.WebExceptionStatus]::SendFailure,
                            [System.Net.WebExceptionStatus]::Timeout
                        ) -contains $webError.Status) {
                        $retryReason = $webError.Status.ToString()
                    } else {
                        throw "Download request failed: $($webError.Status)."
                    }
                }

                if ($downloadComplete -or -not [string]::IsNullOrWhiteSpace($retryReason)) {
                    break
                }
                $statusCode = [int]$response.StatusCode
                if (@(301, 302, 303, 307, 308) -notcontains $statusCode) {
                    break
                }
                $location = $response.Headers['Location']
                if ([string]::IsNullOrWhiteSpace($location)) {
                    throw 'Download server returned a redirect without a Location header.'
                }
                $nextUri = [Uri]::new($requestUri, $location)
                if ($requestUri.Scheme -eq 'https' -and $nextUri.Scheme -ne 'https') {
                    throw 'Download refused an HTTPS-to-HTTP redirect.'
                }
                $response.Dispose()
                $response = $null
                $redirectCount++
                if ($redirectCount -gt 10) {
                    throw 'Download server exceeded the redirect limit.'
                }
                $requestUri = $nextUri
            }

            if (-not $downloadComplete -and [string]::IsNullOrWhiteSpace($retryReason)) {
                if (-not [string]::IsNullOrWhiteSpace($response.ContentEncoding) -and $response.ContentEncoding -ne 'identity') {
                    throw "Download server returned unsupported content encoding '$($response.ContentEncoding)'."
                }

                $statusCode = [int]$response.StatusCode
                $append = $false
                $expectedSegmentBytes = -1L
                if ($statusCode -eq 206) {
                    $contentRange = $response.Headers['Content-Range']
                    if ($contentRange -notmatch '^bytes\s+(\d+)-(\d+)/(\d+|\*)$') {
                        throw "Download server returned an invalid Content-Range: '$contentRange'."
                    }
                    $rangeStart = [long]$Matches[1]
                    $rangeEnd = [long]$Matches[2]
                    $rangeTotal = $Matches[3]
                    if ($rangeStart -ne $offset -or $rangeEnd -lt $rangeStart) {
                        throw "Download server returned an unexpected byte range beginning at $rangeStart; expected $offset."
                    }
                    $expectedSegmentBytes = $rangeEnd - $rangeStart + 1
                    if ($response.ContentLength -ge 0 -and $response.ContentLength -ne $expectedSegmentBytes) {
                        throw 'Download Content-Length did not match Content-Range.'
                    }
                    if ($rangeTotal -ne '*') {
                        $knownTotal = [long]$rangeTotal
                        if ($rangeEnd -ge $knownTotal) {
                            throw 'Download Content-Range exceeded the advertised total size.'
                        }
                    }
                    $append = $offset -gt 0
                } elseif ($statusCode -eq 200) {
                    if ($response.ContentLength -ge 0) {
                        $knownTotal = $response.ContentLength
                        $expectedSegmentBytes = $response.ContentLength
                    }
                    $offset = 0
                } else {
                    throw "Download server returned unexpected HTTP status $statusCode."
                }

                $fileMode = if ($append) { [System.IO.FileMode]::OpenOrCreate } else { [System.IO.FileMode]::Create }
                $output = [System.IO.File]::Open($partial, $fileMode, [System.IO.FileAccess]::Write, [System.IO.FileShare]::None)
                if ($append) {
                    if ($output.Length -ne $offset) {
                        throw 'Download partial changed while opening it for resume.'
                    }
                    $output.Position = $offset
                }
                $input = $response.GetResponseStream()
                $buffer = New-Object byte[] (1MB)
                $segmentBytes = 0L
                $nextProgress = [DateTime]::UtcNow
                while ($true) {
                    try {
                        $count = $input.Read($buffer, 0, $buffer.Length)
                    } catch [System.IO.IOException] {
                        $retryReason = 'the response stream was interrupted'
                        break
                    } catch [System.Net.WebException] {
                        $retryReason = 'the response stream was interrupted'
                        break
                    }
                    if ($count -eq 0) {
                        break
                    }
                    $output.Write($buffer, 0, $count)
                    $segmentBytes += $count
                    if ($showInlineProgress -and [DateTime]::UtcNow -ge $nextProgress) {
                        $progressWidth = Write-MagicHandyDownloadProgress -Name $name -CompletedBytes $output.Position -TotalBytes $knownTotal -PreviousWidth $progressWidth
                        $nextProgress = [DateTime]::UtcNow.AddMilliseconds(250)
                    }
                }
                $output.Flush()

                if ([string]::IsNullOrWhiteSpace($retryReason) -and
                    $expectedSegmentBytes -ge 0 -and $segmentBytes -ne $expectedSegmentBytes) {
                    $retryReason = 'the response ended before its advertised length'
                }
                if ([string]::IsNullOrWhiteSpace($retryReason) -and $knownTotal -ge 0) {
                    if ($output.Length -lt $knownTotal) {
                        $retryReason = 'the response ended before the complete file arrived'
                    } elseif ($output.Length -gt $knownTotal) {
                        throw 'Downloaded bytes exceeded the advertised file size.'
                    }
                }
                if ([string]::IsNullOrWhiteSpace($retryReason)) {
                    $downloadComplete = $true
                }
            }
        } finally {
            if ($null -ne $input) {
                $input.Dispose()
            }
            if ($null -ne $output) {
                $output.Dispose()
            }
            if ($null -ne $response) {
                $response.Dispose()
            }
        }

        if ($downloadComplete) {
            break
        }
        $lastRetryReason = $retryReason
        if ($showInlineProgress -and $progressWidth -gt 0) {
            [Console]::Write("`r$(' ' * $progressWidth)`r")
            $progressWidth = 0
        }
        if ($attempt -eq $MaxAttempts) {
            throw "Downloading $name failed after $MaxAttempts attempts ($lastRetryReason). Partial data was retained at '$partial' for resume."
        }
        $resumeBytes = if (Test-Path -LiteralPath $partial -PathType Leaf) { (Get-Item -LiteralPath $partial).Length } else { 0 }
        Write-Warning "Download interrupted ($retryReason). Attempt $($attempt + 1) of $MaxAttempts will resume from $(Format-MagicHandyByteCount $resumeBytes)."
        $exponentialDelay = $RetryDelayMilliseconds * [Math]::Pow(2, $attempt - 1)
        $delay = [int][Math]::Min(30000, [Math]::Max($retryAfterMilliseconds, $exponentialDelay))
        if ($delay -gt 0) {
            Start-Sleep -Milliseconds $delay
        }
    }

    $completedBytes = (Get-Item -LiteralPath $partial).Length
    if ($showInlineProgress) {
        $progressWidth = Write-MagicHandyDownloadProgress -Name $name -CompletedBytes $completedBytes -TotalBytes $knownTotal -PreviousWidth $progressWidth
        [Console]::WriteLine()
    }
    Write-Host "Verifying $name SHA-256..."
    $actual = Get-MagicHandySHA256 -Path $partial
    if ($actual -ne $expected) {
        Remove-Item -LiteralPath $partial -Force -ErrorAction SilentlyContinue
        throw "SHA-256 verification failed for $name."
    }
    Move-Item -LiteralPath $partial -Destination $Destination -Force
    Write-Host "Verified $name." -ForegroundColor Green
}

function Build-MagicHandyBinaries {
    param(
        [Parameter(Mandatory = $true)][string]$RepositoryPath,
        [Parameter(Mandatory = $true)][string]$GoExecutable
    )
    $previousCGO = $env:CGO_ENABLED
    $env:CGO_ENABLED = '0'
    $targets = @(
        @{ Output = 'magichandy.exe'; Package = './cmd/magichandy' },
        @{ Output = 'voice-parakeet-worker.exe'; Package = './cmd/voice-parakeet-worker' },
        @{ Output = 'voice-neutts-worker.exe'; Package = './cmd/voice-neutts-worker' },
        @{ Output = 'voice-elevenlabs-worker.exe'; Package = './cmd/voice-elevenlabs-worker' }
    )
    try {
        foreach ($target in $targets) {
            $output = Join-Path $RepositoryPath $target.Output
            $partial = "$output.partial-$PID.exe"
            Write-Host "Building $($target.Output)..."
            try {
                if (Test-Path -LiteralPath $partial) {
                    Remove-Item -LiteralPath $partial -Force
                }
                & $GoExecutable build -o $partial $target.Package
                if ($LASTEXITCODE -ne 0) {
                    throw "Building $($target.Output) failed (exit $LASTEXITCODE)."
                }
                Move-Item -LiteralPath $partial -Destination $output -Force
            } finally {
                if (Test-Path -LiteralPath $partial) {
                    Remove-Item -LiteralPath $partial -Force -ErrorAction SilentlyContinue
                }
            }
        }
    } finally {
        $env:CGO_ENABLED = $previousCGO
    }
}

function Install-MagicHandyParakeet {
    param([Parameter(Mandatory = $true)][string]$DataDir)

    $root = Join-Path $DataDir 'voice\parakeet'
    $runnerDir = Join-Path $root 'runner'
    $serverExe = Join-Path $runnerDir 'parakeet-server.exe'
    $modelPath = Join-Path $root 'tdt-0.6b-v3-q4_k.gguf'
    $archive = Join-Path $root 'parakeet-v0.4.0-bin-win-cpu-x64.zip'
    Assert-MagicHandyChildPath -Root $DataDir -Candidate $root
    Assert-MagicHandyChildPath -Root $root -Candidate $runnerDir

    Write-Host 'Parakeet runner: parakeet.cpp v0.4.0, MIT, approximately 1.4 MB.'
    Write-Host 'Parakeet model: TDT 0.6B v3 Q4_K, CC-BY-4.0, approximately 644 MiB.'
    if (-not (Test-Path -LiteralPath $serverExe)) {
        Install-MagicHandyVerifiedDownload -Uri $script:ParakeetRunnerURL -Destination $archive -ExpectedSHA256 $script:ParakeetRunnerSHA256
        $stage = Join-Path $root "runner.partial-$PID"
        try {
            Expand-Archive -LiteralPath $archive -DestinationPath $stage -Force
            $candidate = Get-ChildItem -LiteralPath $stage -Filter 'parakeet-server.exe' -File -Recurse | Select-Object -First 1
            if ($null -eq $candidate) {
                throw 'The verified Parakeet archive did not contain parakeet-server.exe.'
            }
            if (Test-Path -LiteralPath $runnerDir) {
                Assert-MagicHandyChildPath -Root $root -Candidate $runnerDir
                Remove-Item -LiteralPath $runnerDir -Recurse -Force
            }
            New-Item -ItemType Directory -Force -Path $runnerDir | Out-Null
            Get-ChildItem -LiteralPath $candidate.Directory.FullName -Force | Move-Item -Destination $runnerDir -Force
        } finally {
            if (Test-Path -LiteralPath $stage) {
                Remove-Item -LiteralPath $stage -Recurse -Force -ErrorAction SilentlyContinue
            }
            if (Test-Path -LiteralPath $archive) {
                Remove-Item -LiteralPath $archive -Force -ErrorAction SilentlyContinue
            }
        }
    }
    if (-not (Test-Path -LiteralPath $serverExe)) {
        throw 'Parakeet runner installation did not produce parakeet-server.exe.'
    }
    Install-MagicHandyVerifiedDownload -Uri $script:ParakeetModelURL -Destination $modelPath -ExpectedSHA256 $script:ParakeetModelSHA256
    Write-Host "Parakeet runner: $serverExe" -ForegroundColor Green
    Write-Host "Parakeet model:  $modelPath" -ForegroundColor Green
    Write-Host 'In MagicHandy, open Settings > Voice, select Parakeet and the MagicHandy module, enable voice workers, save, then choose Start.' -ForegroundColor Cyan
}

function Test-MagicHandyNeuTTSInstall {
    param(
        [Parameter(Mandatory = $true)][string]$DataDir,
        [Parameter(Mandatory = $true)][ValidateSet('cpu', 'cuda')][string]$Backend
    )

    return Test-MagicHandyNeuTTSInstallRoot -InstallRoot (Join-Path $DataDir 'voice\neutts\active') -Backend $Backend
}

function Test-MagicHandyNeuTTSInstallRoot {
    param(
        [Parameter(Mandatory = $true)][string]$InstallRoot,
        [Parameter(Mandatory = $true)][ValidateSet('cpu', 'cuda')][string]$Backend
    )

    $runtime = Join-Path $InstallRoot 'runtime'
    $manifestPath = Join-Path $runtime 'runtime.json'
    $runner = Join-Path $runtime 'stream_pcm.exe'
    $decoder = Join-Path $runtime 'models\neucodec_decoder.safetensors'
    $encoder = Join-Path $runtime 'magichandy-neucodec-encoder.exe'
    $directML = Join-Path $runtime 'DirectML.dll'
    $encoderModel = Join-Path $InstallRoot 'encoder\distill_neucodec_encoder.onnx'
    $encoderWeights = "$encoderModel.data"
    $backboneRepo = Join-Path $InstallRoot 'hf\hub\models--neuphonic--neutts-air-q4-gguf'
    $backboneRef = Join-Path $backboneRepo 'refs\main'
    $gguf = Join-Path $backboneRepo "snapshots\$($script:NeuTTSBackboneRevision)\neutts-air-Q4_0.gguf"
    if (-not (Test-Path -LiteralPath $manifestPath -PathType Leaf) -or
        -not (Test-Path -LiteralPath $runner -PathType Leaf) -or
        -not (Test-Path -LiteralPath $decoder -PathType Leaf) -or
        -not (Test-Path -LiteralPath $encoder -PathType Leaf) -or
        -not (Test-Path -LiteralPath $directML -PathType Leaf) -or
        -not (Test-Path -LiteralPath $encoderModel -PathType Leaf) -or
        -not (Test-Path -LiteralPath $encoderWeights -PathType Leaf) -or
        -not (Test-Path -LiteralPath $backboneRef -PathType Leaf) -or
        -not (Test-Path -LiteralPath $gguf -PathType Leaf)) {
        return $false
    }
    try {
        $manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
        $required = @(
            'schema_version', 'source_commit', 'rust_toolchain', 'backend',
            'runner_protocol', 'backbone_acceleration', 'codec_acceleration', 'backbone_revision',
            'phonemizer', 'phonemizer_version',
            'backbone_sha256', 'codec_revision', 'codec_checkpoint_sha256',
            'runner_sha256', 'decoder_sha256', 'encoder_revision', 'encoder_sha256',
            'encoder_model_sha256', 'encoder_model_data_sha256', 'directml_sha256',
            'native_dependencies'
        )
        foreach ($name in $required) {
            if ($manifest.PSObject.Properties.Name -notcontains $name) {
                return $false
            }
        }
        $expectedBackboneAcceleration = if ($Backend -eq 'cuda') { 'cuda_all_layers' } else { 'cpu' }
        $expectedCodecAcceleration = if ($Backend -eq 'cuda') { 'wgpu' } else { 'cpu' }
        $expectedDependencies = @(if ($Backend -eq 'cuda') {
            'ggml-base.dll', 'ggml-cpu.dll', 'ggml-cuda.dll', 'ggml.dll', 'llama.dll'
        })
        $manifestDependencies = @($manifest.native_dependencies.PSObject.Properties)
        if ($manifestDependencies.Count -ne $expectedDependencies.Count) {
            return $false
        }
        foreach ($name in $expectedDependencies) {
            $dependency = Join-Path $runtime $name
            if (-not (Test-Path -LiteralPath $dependency -PathType Leaf) -or
                $manifest.native_dependencies.PSObject.Properties.Name -notcontains $name -or
                [string]$manifest.native_dependencies.$name -ne (Get-MagicHandySHA256 -Path $dependency)) {
                return $false
            }
        }
        return [int]$manifest.schema_version -eq 4 -and
            (Get-Content -LiteralPath $backboneRef -Raw).Trim() -eq $script:NeuTTSBackboneRevision -and
            [string]$manifest.source_commit -eq $script:NeuTTSSourceCommit -and
            [string]$manifest.rust_toolchain -eq $script:NeuTTSRustToolchain -and
            [string]$manifest.backend -eq $Backend -and
            [string]$manifest.runner_protocol -eq $script:NeuTTSRunnerProtocol -and
            [string]$manifest.phonemizer -eq $script:NeuTTSPhonemizer -and
            [string]$manifest.phonemizer_version -eq $script:NeuTTSPhonemizerVersion -and
            [string]$manifest.backbone_acceleration -eq $expectedBackboneAcceleration -and
            [string]$manifest.codec_acceleration -eq $expectedCodecAcceleration -and
            [string]$manifest.backbone_revision -eq $script:NeuTTSBackboneRevision -and
            [string]$manifest.backbone_sha256 -eq $script:NeuTTSBackboneSHA256 -and
            [string]$manifest.codec_revision -eq $script:NeuTTSCodecRevision -and
            [string]$manifest.codec_checkpoint_sha256 -eq $script:NeuTTSCodecSHA256 -and
            [string]$manifest.encoder_revision -eq $script:NeuTTSEncoderRevision -and
            [string]$manifest.runner_sha256 -eq (Get-MagicHandySHA256 -Path $runner) -and
            [string]$manifest.decoder_sha256 -eq (Get-MagicHandySHA256 -Path $decoder) -and
            [string]$manifest.encoder_sha256 -eq (Get-MagicHandySHA256 -Path $encoder) -and
            [string]$manifest.directml_sha256 -eq (Get-MagicHandySHA256 -Path $directML) -and
            [string]$manifest.encoder_model_sha256 -eq $script:NeuTTSEncoderModelSHA256 -and
            [string]$manifest.encoder_model_sha256 -eq (Get-MagicHandySHA256 -Path $encoderModel) -and
            [string]$manifest.encoder_model_data_sha256 -eq $script:NeuTTSEncoderWeightsSHA256 -and
            [string]$manifest.encoder_model_data_sha256 -eq (Get-MagicHandySHA256 -Path $encoderWeights) -and
            $script:NeuTTSBackboneSHA256 -eq (Get-MagicHandySHA256 -Path $gguf)
    } catch {
        return $false
    }
}

function Restore-MagicHandyNeuTTSBackup {
    param([Parameter(Mandatory = $true)][string]$DataDir)

    $root = Join-Path $DataDir 'voice\neutts'
    $active = Join-Path $root 'active'
    if (Test-Path -LiteralPath $active) {
        return
    }
    $backup = Get-ChildItem -LiteralPath $root -Directory -Filter 'active.backup-*' -ErrorAction SilentlyContinue |
        Sort-Object LastWriteTimeUtc -Descending |
        Select-Object -First 1
    if ($null -ne $backup) {
        Move-Item -LiteralPath $backup.FullName -Destination $active
        Write-Warning "Recovered the previous NeuTTS runtime after an interrupted installer swap: $active"
    }
}

function Repair-MagicHandyNeuTTSCargoLock {
    param([Parameter(Mandatory = $true)][string]$SourceRoot)

    $lockPath = Join-Path $SourceRoot 'Cargo.lock'
    if (-not (Test-Path -LiteralPath $lockPath -PathType Leaf)) {
        throw "Pinned neutts-rs source is missing '$lockPath'."
    }

    $content = [System.IO.File]::ReadAllText($lockPath)
    $pattern = '(?m)(\[\[package\]\]\r?\nname = "neutts"\r?\nversion = ")0\.1\.0("\r?\ndependencies = \[)'
    $matches = [System.Text.RegularExpressions.Regex]::Matches($content, $pattern)
    if ($matches.Count -ne 1) {
        throw 'Pinned neutts-rs Cargo.lock no longer contains the expected v0.1.1 root-package metadata defect.'
    }

    $patched = [System.Text.RegularExpressions.Regex]::Replace($content, $pattern, '${1}0.1.1${2}')
    Write-MagicHandyUTF8 -Path $lockPath -Content $patched
}

function Add-MagicHandyNeuTTSRunnerSource {
    param(
        [Parameter(Mandatory = $true)][string]$SourceRoot,
        [Parameter(Mandatory = $true)][string]$RepositoryRoot,
        [Parameter(Mandatory = $true)][string]$GitExecutable
    )

    $runnerSource = Join-Path $RepositoryRoot 'workers\neutts-runner\main.rs'
    $cudaPatch = Join-Path $RepositoryRoot 'workers\neutts-runner\neutts-rs-v0.1.1-cuda.patch'
    if (-not (Test-Path -LiteralPath $runnerSource -PathType Leaf) -or -not (Test-Path -LiteralPath $cudaPatch -PathType Leaf)) {
        throw 'MagicHandy persistent NeuTTS runner source or its pinned CUDA patch is missing.'
    }
    & $GitExecutable -C $SourceRoot apply --check $cudaPatch | Out-Host
    if ($LASTEXITCODE -ne 0) {
        throw 'The pinned neutts-rs CUDA offload patch no longer applies cleanly.'
    }
    & $GitExecutable -C $SourceRoot apply $cudaPatch | Out-Host
    if ($LASTEXITCODE -ne 0) {
        throw 'Applying the pinned neutts-rs CUDA offload patch failed.'
    }
    Copy-Item -LiteralPath $runnerSource -Destination (Join-Path $SourceRoot 'examples\magichandy_neutts.rs') -Force
}

function Test-MagicHandyNativeProbe {
    param(
        [Parameter(Mandatory = $true)][string]$Executable,
        [string[]]$ArgumentList = @()
    )

    $probeErrorAction = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        $global:LASTEXITCODE = -1
        & $Executable @ArgumentList *> $null
        return ($global:LASTEXITCODE -eq 0)
    } catch {
        return $false
    } finally {
        $ErrorActionPreference = $probeErrorAction
    }
}

function Confirm-MagicHandyNeuTTSInstall {
    param(
        [Parameter(Mandatory = $true)][ValidateSet('cpu', 'cuda')][string]$Backend,
        [switch]$AssumeYes
    )

    Write-Host "NeuTTS runner: neutts-rs v0.1.1 (MIT) with system eSpeak NG $($script:NeuTTSPhonemizerVersion)+ (GPL-3.0-or-later)."
    Write-Host "Source commit: $($script:NeuTTSSourceCommit); Rust toolchain: $($script:NeuTTSRustToolchain)." -ForegroundColor DarkGray
    $installedSize = if ($Backend -eq 'cuda') { 'about 2.0 GiB' } else { 'about 1.9 GiB' }
    Write-Host "NeuTTS models: Air Q4, NeuCodec, and DistillNeuCodec ONNX, Apache-2.0; $installedSize installed plus temporary build and conversion assets."
    if ($Backend -eq 'cuda') {
        Write-Host 'NeuTTS acceleration: CUDA runs the speech backbone on NVIDIA GPU layers and WGPU accelerates NeuCodec. This substantially reduces reply latency but reserves GPU memory while voice replies are enabled.' -ForegroundColor Cyan
    } else {
        Write-Host 'NeuTTS acceleration: CPU-only. This saves GPU memory and works without CUDA, but speech generation can be much slower than real time.' -ForegroundColor Yellow
    }
    Write-Host "Air Q4 SHA-256: $($script:NeuTTSBackboneSHA256)" -ForegroundColor DarkGray
    Write-Host "NeuCodec SHA-256: $($script:NeuTTSCodecSHA256)" -ForegroundColor DarkGray
    Write-Host "Reference encoder model SHA-256: $($script:NeuTTSEncoderModelSHA256)" -ForegroundColor DarkGray
    if (-not (Confirm-MagicHandyChoice -Question 'Download the pinned NeuTTS models and build the local runner now?' -Default $true -AssumeYes:$AssumeYes)) {
        throw 'NeuTTS installation is required when managed llama.cpp is selected. Rerun with -SkipLlamaBuild to skip both.'
    }
}

function Install-MagicHandyNeuTTS {
    param(
        [Parameter(Mandatory = $true)][string]$DataDir,
        [Parameter(Mandatory = $true)][string]$GitExecutable,
        [Parameter(Mandatory = $true)][string]$RustupExecutable,
        [Parameter(Mandatory = $true)][string]$LibClangPath,
        [Parameter(Mandatory = $true)][string]$CMakeExecutable,
        [Parameter(Mandatory = $true)][string]$ESpeakExecutable,
        [Parameter(Mandatory = $true)][ValidateSet('cpu', 'cuda')][string]$Backend,
        [string]$CUDAExecutable = ''
    )

    $root = Join-Path $DataDir 'voice\neutts'
    Assert-MagicHandyChildPath -Root $DataDir -Candidate $root
    if (Test-MagicHandyNeuTTSInstall -DataDir $DataDir -Backend $Backend) {
        Write-Host "NeuTTS runtime is already verified at $(Join-Path $root 'active\runtime')." -ForegroundColor Green
        return
    }
    if ($Backend -eq 'cuda' -and [string]::IsNullOrWhiteSpace($CUDAExecutable)) {
        throw 'A CUDA NeuTTS build requires the verified nvcc executable.'
    }

    $active = Join-Path $root 'active'
    $activeStage = Join-Path $root "active.partial-$PID"
    $activeBackup = Join-Path $root ("active.backup-" + [Guid]::NewGuid().ToString('N'))
    $runtimeStage = Join-Path $activeStage 'runtime'
    $encoderStage = Join-Path $activeStage 'encoder'
    $encoderModel = Join-Path $encoderStage 'distill_neucodec_encoder.onnx'
    $encoderWeights = "$encoderModel.data"
    $buildRoot = Join-Path $root "build.partial-$PID"
    $sourceRoot = Join-Path $buildRoot 'source'
    $targetRoot = Join-Path $buildRoot 'target'
    $cargoHome = Join-Path $buildRoot 'cargo-home'
    $localBuildData = Join-Path $buildRoot 'local-app-data'
    $hfRoot = Join-Path $activeStage 'hf'
    $backboneRepo = Join-Path $hfRoot 'hub\models--neuphonic--neutts-air-q4-gguf'
    $backboneSnapshot = Join-Path $backboneRepo "snapshots\$($script:NeuTTSBackboneRevision)"
    $backbonePath = Join-Path $backboneSnapshot 'neutts-air-Q4_0.gguf'
    $codecRepo = Join-Path $hfRoot 'hub\models--neuphonic--neucodec'
    $codecSnapshot = Join-Path $codecRepo "snapshots\$($script:NeuTTSCodecRevision)"
    $codecCheckpoint = Join-Path $codecSnapshot 'pytorch_model.bin'
    $downloadRoot = Join-Path $root 'downloads'
    $backbonePartial = Join-Path $downloadRoot "$($script:NeuTTSBackboneSHA256)-neutts-air-Q4_0.gguf.partial"
    $codecPartial = Join-Path $downloadRoot "$($script:NeuTTSCodecSHA256)-pytorch_model.bin.partial"
    $encoderModelPartial = Join-Path $downloadRoot "$($script:NeuTTSEncoderModelSHA256)-distill_neucodec_encoder.onnx.partial"
    $encoderWeightsPartial = Join-Path $downloadRoot "$($script:NeuTTSEncoderWeightsSHA256)-distill_neucodec_encoder.onnx.data.partial"
    $decoderStage = Join-Path $runtimeStage 'models\neucodec_decoder.safetensors'
    $repositoryRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
    $encoderManifest = Join-Path $repositoryRoot 'workers\neucodec-encoder\Cargo.toml'
    $encoderLock = Join-Path $repositoryRoot 'workers\neucodec-encoder\Cargo.lock'
    if (-not (Test-Path -LiteralPath $encoderManifest -PathType Leaf) -or -not (Test-Path -LiteralPath $encoderLock -PathType Leaf)) {
        throw 'MagicHandy NeuCodec encoder source is missing from workers\neucodec-encoder.'
    }
    foreach ($path in @($active, $activeStage, $activeBackup, $buildRoot, $hfRoot, $backbonePath, $codecCheckpoint, $encoderModel, $encoderWeights, $downloadRoot, $backbonePartial, $codecPartial, $encoderModelPartial, $encoderWeightsPartial)) {
        Assert-MagicHandyChildPath -Root $root -Candidate $path
    }

    $previousCargoHome = $env:CARGO_HOME
    $previousCargoTarget = $env:CARGO_TARGET_DIR
    $previousHFHome = $env:HF_HOME
    $previousOffline = $env:HF_HUB_OFFLINE
    $previousLibClang = $env:LIBCLANG_PATH
    $previousLocalAppData = $env:LOCALAPPDATA
    $previousCMake = $env:CMAKE
    $previousCUDAPath = $env:CUDA_PATH
    $previousCUDAToolkitDir = $env:CudaToolkitDir
    $previousCUDACXX = $env:CUDACXX
    $previousPath = $env:Path
    try {
        foreach ($path in @($activeStage, $buildRoot)) {
            if (Test-Path -LiteralPath $path) {
                Remove-Item -LiteralPath $path -Recurse -Force
            }
        }
        New-Item -ItemType Directory -Force -Path (Split-Path -Parent $decoderStage) | Out-Null
        New-Item -ItemType Directory -Force -Path $buildRoot, $localBuildData | Out-Null
        & $GitExecutable clone --branch $script:NeuTTSSourceTag --depth 1 $script:NeuTTSSourceURL $sourceRoot | Out-Host
        if ($LASTEXITCODE -ne 0) {
            throw "Fetching neutts-rs $($script:NeuTTSSourceTag) failed (exit $LASTEXITCODE)."
        }
        $actualCommit = (& $GitExecutable -C $sourceRoot rev-parse HEAD).Trim()
        if ($LASTEXITCODE -ne 0 -or $actualCommit -ne $script:NeuTTSSourceCommit) {
            throw "neutts-rs source verification failed: expected $($script:NeuTTSSourceCommit), got '$actualCommit'."
        }
        Repair-MagicHandyNeuTTSCargoLock -SourceRoot $sourceRoot
        Add-MagicHandyNeuTTSRunnerSource -SourceRoot $sourceRoot -RepositoryRoot $repositoryRoot -GitExecutable $GitExecutable

        Install-MagicHandyVerifiedDownload -Uri $script:NeuTTSBackboneURL -Destination $backbonePath -ExpectedSHA256 $script:NeuTTSBackboneSHA256 -PartialPath $backbonePartial
        New-Item -ItemType Directory -Force -Path (Join-Path $backboneRepo 'refs') | Out-Null
        Write-MagicHandyUTF8 -Path (Join-Path $backboneRepo 'refs\main') -Content $script:NeuTTSBackboneRevision
        Install-MagicHandyVerifiedDownload -Uri $script:NeuTTSCodecURL -Destination $codecCheckpoint -ExpectedSHA256 $script:NeuTTSCodecSHA256 -PartialPath $codecPartial
        New-Item -ItemType Directory -Force -Path (Join-Path $codecRepo 'refs') | Out-Null
        Write-MagicHandyUTF8 -Path (Join-Path $codecRepo 'refs\main') -Content $script:NeuTTSCodecRevision
        Install-MagicHandyVerifiedDownload -Uri $script:NeuTTSEncoderModelURL -Destination $encoderModel -ExpectedSHA256 $script:NeuTTSEncoderModelSHA256 -PartialPath $encoderModelPartial
        Install-MagicHandyVerifiedDownload -Uri $script:NeuTTSEncoderWeightsURL -Destination $encoderWeights -ExpectedSHA256 $script:NeuTTSEncoderWeightsSHA256 -PartialPath $encoderWeightsPartial

        $env:CARGO_HOME = $cargoHome
        $env:CARGO_TARGET_DIR = $targetRoot
        $env:HF_HOME = $hfRoot
        $env:HF_HUB_OFFLINE = '1'
        $env:LIBCLANG_PATH = $LibClangPath
        $env:LOCALAPPDATA = $localBuildData
        $env:CMAKE = $CMakeExecutable
        $env:Path = (Split-Path -Parent $CMakeExecutable) + ';' + $previousPath
        if ($Backend -eq 'cuda') {
            $cudaRoot = Split-Path -Parent (Split-Path -Parent ([System.IO.Path]::GetFullPath($CUDAExecutable)))
            $env:CUDA_PATH = $cudaRoot.TrimEnd('\')
            $env:CudaToolkitDir = $cudaRoot.TrimEnd('\') + '\'
            $env:CUDACXX = [System.IO.Path]::GetFullPath($CUDAExecutable)
            $env:Path = (Split-Path -Parent $CUDAExecutable) + ';' + $env:Path
        }
        Write-Host 'Converting the verified NeuCodec checkpoint without Python...'
        & $RustupExecutable run $script:NeuTTSRustToolchain cargo run --locked --release --no-default-features --manifest-path (Join-Path $sourceRoot 'Cargo.toml') --example convert_weights -- --out $decoderStage | Out-Host
        if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $decoderStage)) {
            throw "NeuCodec decoder conversion failed (exit $LASTEXITCODE)."
        }

        $runnerFeatures = if ($Backend -eq 'cuda') { 'cuda,wgpu' } else { 'backbone' }
        Write-Host "Building MagicHandy's persistent NeuTTS runner ($Backend; features $runnerFeatures)..."
        & $RustupExecutable run $script:NeuTTSRustToolchain cargo build --locked --release --manifest-path (Join-Path $sourceRoot 'Cargo.toml') --example magichandy_neutts --features $runnerFeatures | Out-Host
        if ($LASTEXITCODE -ne 0) {
            throw "Persistent NeuTTS runner build failed (exit $LASTEXITCODE)."
        }
        $runnerCandidate = Join-Path $targetRoot 'release\examples\magichandy_neutts.exe'
        if (-not (Test-Path -LiteralPath $runnerCandidate)) {
            throw "NeuTTS build did not produce '$runnerCandidate'."
        }
        if (-not (Test-MagicHandyNativeProbe -Executable $runnerCandidate -ArgumentList @('--help'))) {
            throw 'The built NeuTTS stream_pcm runner did not pass its help probe.'
        }
        $phonemes = (& $runnerCandidate --espeak $ESpeakExecutable --phonemize 'clearly naturally completely misspoken' 2>&1) -join ' '
        if ($LASTEXITCODE -ne 0 -or
            $phonemes -notmatch 'klˈɪɹli' -or
            $phonemes -notmatch 'nˈætʃɚɹəli' -or
            $phonemes -notmatch 'kəmplˈiːtli' -or
            $phonemes -notmatch 'mɪsspˈoʊkən') {
            throw "The built NeuTTS runner failed its eSpeak NG phonemizer quality probe: $phonemes"
        }
        Copy-Item -LiteralPath $runnerCandidate -Destination (Join-Path $runtimeStage 'stream_pcm.exe') -Force
        $nativeDependencyHashes = [ordered]@{}
        if ($Backend -eq 'cuda') {
            foreach ($name in @('ggml-base.dll', 'ggml-cpu.dll', 'ggml-cuda.dll', 'ggml.dll', 'llama.dll')) {
                $candidate = Join-Path (Split-Path -Parent $runnerCandidate) $name
                if (-not (Test-Path -LiteralPath $candidate -PathType Leaf)) {
                    throw "The CUDA NeuTTS build did not produce its required $name dependency."
                }
                $destination = Join-Path $runtimeStage $name
                Copy-Item -LiteralPath $candidate -Destination $destination -Force
                $nativeDependencyHashes[$name] = Get-MagicHandySHA256 -Path $destination
            }
        }

        Write-Host 'Building the pinned MagicHandy NeuCodec ONNX reference encoder...'
        & $RustupExecutable run $script:NeuTTSRustToolchain cargo build --locked --release --manifest-path $encoderManifest | Out-Host
        if ($LASTEXITCODE -ne 0) {
            throw "NeuCodec reference encoder build failed (exit $LASTEXITCODE)."
        }
        $encoderCandidate = Join-Path $targetRoot 'release\magichandy-neucodec-encoder.exe'
        $directMLCandidate = Join-Path $targetRoot 'release\DirectML.dll'
        if (-not (Test-Path -LiteralPath $encoderCandidate -PathType Leaf) -or -not (Test-Path -LiteralPath $directMLCandidate -PathType Leaf)) {
            throw 'The NeuCodec encoder build did not produce its executable and DirectML runtime.'
        }
        if (-not (Test-MagicHandyNativeProbe -Executable $encoderCandidate -ArgumentList @('--help'))) {
            throw 'The built NeuCodec reference encoder did not pass its help probe.'
        }
        Copy-Item -LiteralPath $encoderCandidate -Destination (Join-Path $runtimeStage 'magichandy-neucodec-encoder.exe') -Force
        Copy-Item -LiteralPath $directMLCandidate -Destination (Join-Path $runtimeStage 'DirectML.dll') -Force

        $metadataDir = Join-Path $runtimeStage 'source-metadata'
        New-Item -ItemType Directory -Force -Path $metadataDir | Out-Null
        Copy-Item -LiteralPath (Join-Path $sourceRoot 'Cargo.toml') -Destination (Join-Path $metadataDir 'neutts-Cargo.toml')
        Copy-Item -LiteralPath (Join-Path $sourceRoot 'Cargo.lock') -Destination (Join-Path $metadataDir 'neutts-Cargo.lock')
        Copy-Item -LiteralPath (Join-Path $repositoryRoot 'workers\neutts-runner\main.rs') -Destination (Join-Path $metadataDir 'magichandy-neutts-runner.rs')
        Copy-Item -LiteralPath (Join-Path $repositoryRoot 'workers\neutts-runner\neutts-rs-v0.1.1-cuda.patch') -Destination (Join-Path $metadataDir 'neutts-rs-v0.1.1-cuda.patch')
        Copy-Item -LiteralPath $encoderManifest -Destination (Join-Path $metadataDir 'encoder-Cargo.toml')
        Copy-Item -LiteralPath $encoderLock -Destination (Join-Path $metadataDir 'encoder-Cargo.lock')
        $licenseDir = Join-Path $runtimeStage 'licenses'
        New-Item -ItemType Directory -Force -Path $licenseDir | Out-Null
        $licenseIndex = New-Object System.Collections.Generic.List[string]
        $licenseNumber = 0
        foreach ($license in @(Get-ChildItem -LiteralPath $cargoHome -File -Recurse -ErrorAction SilentlyContinue | Where-Object { $_.Name -match '^(?i:LICENSE|COPYING|NOTICE)' })) {
            $licenseNumber++
            $package = $license.Directory.Name -replace '[^A-Za-z0-9_.-]', '_'
            $name = ('{0:D3}-{1}-{2}' -f $licenseNumber, $package, $license.Name)
            Copy-Item -LiteralPath $license.FullName -Destination (Join-Path $licenseDir $name)
            $licenseIndex.Add("$name - Cargo source package $package")
        }
        Write-MagicHandyUTF8 -Path (Join-Path $licenseDir 'INDEX.txt') -Content ($licenseIndex -join "`r`n")

        $notices = @"
NeuTTS runtime installed by MagicHandy

neutts-rs $($script:NeuTTSSourceTag) at $($script:NeuTTSSourceCommit)
Source: $($script:NeuTTSSourceURL)
Cargo package metadata license: MIT
The build includes llama.cpp through llama-cpp-4. NeuTTS phonemization invokes the separately
installed eSpeak NG $($script:NeuTTSPhonemizerVersion)+ package (GPL-3.0-or-later).
Cargo dependency license files available in the downloaded sources are retained under licenses\.
MagicHandy's persistent NeuTTS runner and CUDA offload patch are GPL-3.0-only.
Execution backend: $Backend. CUDA builds use all-layer llama.cpp offload and the Burn WGPU codec.

NeuTTS Air Q4 and NeuCodec model repositories declare Apache-2.0.
Backbone revision: $($script:NeuTTSBackboneRevision)
Codec revision: $($script:NeuTTSCodecRevision)

MagicHandy NeuCodec reference encoder worker: GPL-3.0-only.
Its ort, rubato, audioadapter, and hound dependencies declare MIT and/or Apache-2.0 licenses.
ONNX Runtime and Microsoft DirectML declare the MIT license.
DistillNeuCodec ONNX model: Apache-2.0.
Model source: https://huggingface.co/KevinAHM/distill-neucodec-onnx
Encoder model revision: $($script:NeuTTSEncoderRevision)
"@
        Write-MagicHandyUTF8 -Path (Join-Path $runtimeStage 'THIRD_PARTY_NOTICES.txt') -Content $notices
        $rustVersion = (& $RustupExecutable run $script:NeuTTSRustToolchain rustc -vV) -join "`n"
        if ($LASTEXITCODE -ne 0) {
            throw 'Could not record the pinned Rust compiler identity.'
        }
        $runnerHash = Get-MagicHandySHA256 -Path (Join-Path $runtimeStage 'stream_pcm.exe')
        $decoderHash = Get-MagicHandySHA256 -Path $decoderStage
        $encoderHash = Get-MagicHandySHA256 -Path (Join-Path $runtimeStage 'magichandy-neucodec-encoder.exe')
        $directMLHash = Get-MagicHandySHA256 -Path (Join-Path $runtimeStage 'DirectML.dll')
        $manifest = [pscustomobject][ordered]@{
            schema_version = 4
            installed_at = [DateTimeOffset]::UtcNow.ToString('o')
            source_tag = $script:NeuTTSSourceTag
            source_commit = $script:NeuTTSSourceCommit
            rust_toolchain = $script:NeuTTSRustToolchain
            rust_version = $rustVersion
            backend = $Backend
            runner_protocol = $script:NeuTTSRunnerProtocol
            phonemizer = $script:NeuTTSPhonemizer
            phonemizer_version = $script:NeuTTSPhonemizerVersion
            backbone_acceleration = if ($Backend -eq 'cuda') { 'cuda_all_layers' } else { 'cpu' }
            codec_acceleration = if ($Backend -eq 'cuda') { 'wgpu' } else { 'cpu' }
            native_dependencies = $nativeDependencyHashes
            backbone_revision = $script:NeuTTSBackboneRevision
            backbone_sha256 = $script:NeuTTSBackboneSHA256
            codec_revision = $script:NeuTTSCodecRevision
            codec_checkpoint_sha256 = $script:NeuTTSCodecSHA256
            runner_sha256 = $runnerHash
            decoder_sha256 = $decoderHash
            encoder_revision = $script:NeuTTSEncoderRevision
            encoder_sha256 = $encoderHash
            encoder_model_sha256 = $script:NeuTTSEncoderModelSHA256
            encoder_model_data_sha256 = $script:NeuTTSEncoderWeightsSHA256
            directml_sha256 = $directMLHash
        }
        Write-MagicHandyUTF8 -Path (Join-Path $runtimeStage 'runtime.json') -Content ($manifest | ConvertTo-Json -Depth 3)
        Remove-Item -LiteralPath $codecRepo -Recurse -Force

        if (-not (Test-MagicHandyNeuTTSInstallRoot -InstallRoot $activeStage -Backend $Backend)) {
            throw 'The staged NeuTTS runtime did not pass checksum and manifest verification.'
        }
        if (Test-Path -LiteralPath $active) {
            Move-Item -LiteralPath $active -Destination $activeBackup
        }
        try {
            Move-Item -LiteralPath $activeStage -Destination $active
            if (-not (Test-MagicHandyNeuTTSInstall -DataDir $DataDir -Backend $Backend)) {
                throw 'The activated NeuTTS runtime did not pass final verification.'
            }
        } catch {
            if (Test-Path -LiteralPath $active) {
                Remove-Item -LiteralPath $active -Recurse -Force
            }
            if (Test-Path -LiteralPath $activeBackup) {
                if (Test-Path -LiteralPath $active) {
                    throw "NeuTTS rollback cannot restore '$activeBackup' because '$active' still exists. The backup was preserved."
                }
                Move-Item -LiteralPath $activeBackup -Destination $active -Force
            }
            throw
        }
        foreach ($staleBackup in @(Get-ChildItem -LiteralPath $root -Directory -Filter 'active.backup-*' -ErrorAction SilentlyContinue)) {
            try {
                Remove-Item -LiteralPath $staleBackup.FullName -Recurse -Force
            } catch {
                Write-Warning "The verified NeuTTS runtime is active, but old rollback data could not be removed from '$($staleBackup.FullName)': $_"
            }
        }
    } finally {
        $env:CARGO_HOME = $previousCargoHome
        $env:CARGO_TARGET_DIR = $previousCargoTarget
        $env:HF_HOME = $previousHFHome
        $env:HF_HUB_OFFLINE = $previousOffline
        $env:LIBCLANG_PATH = $previousLibClang
        $env:LOCALAPPDATA = $previousLocalAppData
        $env:CMAKE = $previousCMake
        $env:CUDA_PATH = $previousCUDAPath
        $env:CudaToolkitDir = $previousCUDAToolkitDir
        $env:CUDACXX = $previousCUDACXX
        $env:Path = $previousPath
        foreach ($path in @($activeStage, $buildRoot)) {
            if (Test-Path -LiteralPath $path) {
                Remove-Item -LiteralPath $path -Recurse -Force -ErrorAction SilentlyContinue
            }
        }
    }
    $runtime = Join-Path $active 'runtime'
    $installedBackbone = Join-Path $active "hf\hub\models--neuphonic--neutts-air-q4-gguf\snapshots\$($script:NeuTTSBackboneRevision)\neutts-air-Q4_0.gguf"
    Write-Host "NeuTTS runner:  $(Join-Path $runtime 'stream_pcm.exe')" -ForegroundColor Green
    Write-Host "NeuTTS decoder: $(Join-Path $runtime 'models\neucodec_decoder.safetensors')" -ForegroundColor Green
    Write-Host "NeuCodec encoder: $(Join-Path $runtime 'magichandy-neucodec-encoder.exe')" -ForegroundColor Green
    Write-Host "NeuTTS backbone cache: $installedBackbone" -ForegroundColor Green
    Write-Host 'In Settings > Voice, select NeuTTS Air and generate a reference voice from a WAV plus its exact transcript. Leave the runner override blank.' -ForegroundColor Cyan
}

function Test-MagicHandyHTTPReady {
    param([Parameter(Mandatory = $true)][ValidateRange(1, 65535)][int]$Port)

    try {
        $state = Invoke-RestMethod -Uri "http://127.0.0.1:$Port/api/state" -TimeoutSec 1
        return $null -ne $state -and
            $state.PSObject.Properties.Name -contains 'version' -and
            $state.PSObject.Properties.Name -contains 'settings' -and
            $state.PSObject.Properties.Name -contains 'motion'
    } catch {
        return $false
    }
}

function Get-MagicHandyLoopbackListeners {
    param([Parameter(Mandatory = $true)][ValidateRange(1, 65535)][int]$Port)

    return @(Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue | Where-Object {
        [string]$_.LocalAddress -in @('127.0.0.1', '0.0.0.0', '::', '::0')
    })
}

function Test-MagicHandyProcessReady {
    param(
        [Parameter(Mandatory = $true)][ValidateRange(1, 65535)][int]$Port,
        [Parameter(Mandatory = $true)][int]$TargetProcessId
    )

    if (-not (Test-MagicHandyHTTPReady -Port $Port)) {
        return $false
    }
    $listeners = @(Get-MagicHandyLoopbackListeners -Port $Port)
    return [bool]($listeners | Where-Object { [int]$_.OwningProcess -eq $TargetProcessId })
}

function ConvertTo-MagicHandyNativeArgument {
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string]$Value)

    if ($Value.Contains('"')) {
        throw 'Native process arguments cannot contain a double quote.'
    }
    $trailingSeparators = 0
    for ($index = $Value.Length - 1; $index -ge 0 -and $Value[$index] -eq '\'; $index--) {
        $trailingSeparators++
    }
    $escapedTail = (('\' * $trailingSeparators) -join '')
    return '"' + $Value + $escapedTail + '"'
}

function New-MagicHandyAppArgumentLine {
    param(
        [Parameter(Mandatory = $true)][string]$Address,
        [Parameter(Mandatory = $true)][string]$DataDir
    )

    return '-addr {0} -data-dir {1}' -f `
        (ConvertTo-MagicHandyNativeArgument -Value $Address), `
        (ConvertTo-MagicHandyNativeArgument -Value $DataDir)
}

function Get-MagicHandyCheckoutProcesses {
    param([Parameter(Mandatory = $true)][string]$RepositoryPath)

    $executable = [System.IO.Path]::GetFullPath((Join-Path $RepositoryPath 'magichandy.exe'))
    $ownedPaths = @($executable, "$executable~")
    return @(Get-CimInstance Win32_Process -ErrorAction Stop | Where-Object {
        -not [string]::IsNullOrWhiteSpace([string]$_.ExecutablePath) -and
            $ownedPaths -contains ([System.IO.Path]::GetFullPath([string]$_.ExecutablePath))
    })
}

function Remove-MagicHandyBuildBackups {
    param([Parameter(Mandatory = $true)][string]$RepositoryPath)

    foreach ($name in @('magichandy.exe~', 'voice-parakeet-worker.exe~', 'voice-neutts-worker.exe~', 'voice-elevenlabs-worker.exe~')) {
        $path = Join-Path $RepositoryPath $name
        if (Test-Path -LiteralPath $path) {
            Assert-MagicHandyChildPath -Root $RepositoryPath -Candidate $path
            Remove-Item -LiteralPath $path -Force
        }
    }
}

function Stop-MagicHandyProcessTree {
    param([Parameter(Mandatory = $true)][int]$TargetProcessId)

    $output = @(& "$env:SystemRoot\System32\taskkill.exe" /PID $TargetProcessId /T /F)
    $exitCode = $LASTEXITCODE
    $output | Out-Host
    if ($exitCode -ne 0) {
        throw "Could not stop MagicHandy process tree $TargetProcessId (exit $exitCode)."
    }
}

function ConvertFrom-MagicHandyStopErrorResponse {
    param([Parameter(Mandatory = $true)][System.Management.Automation.ErrorRecord]$ErrorRecord)

    $body = if ($null -ne $ErrorRecord.ErrorDetails) { [string]$ErrorRecord.ErrorDetails.Message } else { '' }
    if ([string]::IsNullOrWhiteSpace($body)) {
        $httpResponse = if ($null -ne $ErrorRecord.Exception -and $ErrorRecord.Exception.PSObject.Properties.Name -contains 'Response') {
            $ErrorRecord.Exception.Response
        } else {
            $null
        }
        if ($null -ne $httpResponse -and $httpResponse.PSObject.Methods.Name -contains 'GetResponseStream') {
            $stream = $httpResponse.GetResponseStream()
            if ($null -ne $stream) {
                $reader = New-Object System.IO.StreamReader($stream)
                try {
                    $body = $reader.ReadToEnd()
                } finally {
                    $reader.Dispose()
                    $stream.Dispose()
                }
            }
        }
    }
    if ([string]::IsNullOrWhiteSpace($body)) {
        return $null
    }
    try {
        return $body | ConvertFrom-Json
    } catch {
        return $null
    }
}

function Invoke-MagicHandyRebuildStopRequest {
    param([Parameter(Mandatory = $true)][ValidateRange(1, 65535)][int]$Port)

    try {
        return Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:$Port/api/motion/stop" -TimeoutSec 15
    } catch {
        $response = ConvertFrom-MagicHandyStopErrorResponse -ErrorRecord $_
        if ($null -eq $response) {
            throw
        }
        if ($response -isnot [System.Management.Automation.PSCustomObject]) {
            throw
        }
        $response | Add-Member -NotePropertyName '_http_error' -NotePropertyValue $true -Force
        return $response
    }
}

function Test-MagicHandyEngineActive {
    param([AllowNull()][object]$Engine)

    if ($null -eq $Engine) {
        return $false
    }
    foreach ($name in @('running', 'paused', 'completing')) {
        if ($Engine.PSObject.Properties.Name -contains $name -and [bool]$Engine.$name) {
            return $true
        }
    }
    return $false
}

function Assert-MagicHandyRebuildStopResponse {
    param(
        [Parameter(Mandatory = $true)][object]$Response,
        [switch]$AllowPhysicalStopConfirmation,
        [scriptblock]$PhysicalStopConfirmation
    )

    if ($Response -isnot [System.Management.Automation.PSCustomObject]) {
        throw 'Emergency Stop response was not a JSON object.'
    }
    $propertyNames = @($Response.PSObject.Properties.Name)
    if ($propertyNames -notcontains 'available') {
        throw 'Emergency Stop response was missing required field ''available''.'
    }
    foreach ($name in @('available', 'stopped', '_http_error')) {
        if ($propertyNames -contains $name -and $Response.$name -isnot [bool]) {
            throw "Emergency Stop response field '$name' was not boolean."
        }
    }
    $locallyStopped = $propertyNames -contains 'stopped' -and [bool]$Response.stopped
    $hasEngineState = $propertyNames -contains 'engine'
    if ($hasEngineState) {
        if ($null -eq $Response.engine -or $Response.engine -isnot [System.Management.Automation.PSCustomObject]) {
            throw 'Emergency Stop response field ''engine'' was not an object.'
        }
        $engineProperties = @($Response.engine.PSObject.Properties.Name)
        foreach ($name in @('running', 'paused', 'completing')) {
            if ($engineProperties -notcontains $name -or $Response.engine.$name -isnot [bool]) {
                throw "Emergency Stop engine field '$name' was missing or not boolean."
            }
        }
    }
    if ($hasEngineState -and -not (Test-MagicHandyEngineActive -Engine $Response.engine)) {
        $locallyStopped = $true
    }
    $hasTransportResult = $propertyNames -contains 'transport_result'
    if ($hasTransportResult) {
        if ($null -eq $Response.transport_result -or $Response.transport_result -isnot [System.Management.Automation.PSCustomObject]) {
            throw 'Emergency Stop response field ''transport_result'' was not an object.'
        }
        $resultProperties = @($Response.transport_result.PSObject.Properties.Name)
        if ($resultProperties -notcontains 'ok' -or $Response.transport_result.ok -isnot [bool]) {
            throw 'Emergency Stop transport result field ''ok'' was missing or not boolean.'
        }
        if (-not $hasEngineState -and [bool]$Response.transport_result.ok) {
            $locallyStopped = $true
        }
    }
    if ($propertyNames -contains 'error' -and $Response.error -isnot [string]) {
        throw 'Emergency Stop response field ''error'' was not text.'
    }
    $errorMessage = if ($propertyNames -contains 'error') { $Response.error } else { '' }
    $fromHTTPError = $propertyNames -contains '_http_error' -and [bool]$Response._http_error
    if ($fromHTTPError -and [string]::IsNullOrWhiteSpace($errorMessage)) {
        throw 'Emergency Stop HTTP error response did not contain an error message.'
    }
    $reportedTransportFailure = -not [bool]$Response.available -or `
        ($hasTransportResult -and -not [bool]$Response.transport_result.ok)
    if ($reportedTransportFailure -and [string]::IsNullOrWhiteSpace($errorMessage)) {
        throw 'Emergency Stop response reported transport failure without an error message.'
    }
    if ([string]::IsNullOrWhiteSpace($errorMessage)) {
        if (-not $locallyStopped) {
            throw 'Emergency Stop response did not confirm local stopped state.'
        }
        return
    }
    $recognizedLegacyResponse = $propertyNames -contains 'available' -and `
        ($locallyStopped -or ($hasTransportResult -and -not $hasEngineState))
    if ($recognizedLegacyResponse) {
        if ($AllowPhysicalStopConfirmation) {
            Write-Warning "Physical transport Stop delivery was not confirmed: $errorMessage"
            $confirmation = if ($null -ne $PhysicalStopConfirmation) {
                & $PhysicalStopConfirmation
            } else {
                Read-Host 'Verify the device is physically stopped, then type STOPPED to continue'
            }
            if ($confirmation.Trim() -ceq 'STOPPED') {
                return
            }
        }
        throw "Physical Stop delivery was not confirmed. Verify the device is stopped, close the app, and rerun update.ps1. Transport error: $errorMessage"
    }
    throw "Emergency Stop failed before rebuild: $errorMessage"
}

function Stop-MagicHandyAppForRebuild {
    param(
        [Parameter(Mandatory = $true)][string]$RepositoryPath,
        [Parameter(Mandatory = $true)][ValidateRange(1, 65535)][int]$Port,
        [switch]$AllowPhysicalStopConfirmation,
        [scriptblock]$PhysicalStopConfirmation
    )

    $listeners = @(Get-MagicHandyLoopbackListeners -Port $Port)
    $processes = @(Get-MagicHandyCheckoutProcesses -RepositoryPath $RepositoryPath)
    if ($processes.Count -eq 0) {
        if ($listeners.Count -gt 0) {
            throw "Port $Port is owned by another process. The rebuild was not started."
        }
        Remove-MagicHandyBuildBackups -RepositoryPath $RepositoryPath
        return
    }
    if ($processes.Count -ne 1) {
        throw "Multiple MagicHandy instances are running from this checkout. Use Emergency Stop in each instance, close them, and rerun update.ps1."
    }

    $ownedIDs = @($processes | ForEach-Object { [int]$_.ProcessId })
    $foreignListeners = @($listeners | Where-Object { [int]$_.OwningProcess -notin $ownedIDs })
    if ($foreignListeners.Count -gt 0) {
        throw "Port $Port is owned by another process. MagicHandy was left running and the rebuild was not started."
    }
    if (-not [bool]($listeners | Where-Object { [int]$_.OwningProcess -in $ownedIDs })) {
        throw "A MagicHandy process from this checkout is running but is not reachable on configured port $Port. Use Emergency Stop, close it, and rerun update.ps1."
    }

    Write-Host "Stopping the running MagicHandy process before rebuilding..." -ForegroundColor Cyan
    try {
        $stopResponse = Invoke-MagicHandyRebuildStopRequest -Port $Port
        Assert-MagicHandyRebuildStopResponse -Response $stopResponse -AllowPhysicalStopConfirmation:$AllowPhysicalStopConfirmation -PhysicalStopConfirmation $PhysicalStopConfirmation
    } catch {
        throw "Emergency Stop failed before rebuild. The running app was left in place: $($_.Exception.Message)"
    }

    foreach ($process in $processes) {
        Stop-MagicHandyProcessTree -TargetProcessId ([int]$process.ProcessId)
    }
    $deadline = [DateTime]::UtcNow.AddSeconds(5)
    do {
        $remaining = @(Get-MagicHandyCheckoutProcesses -RepositoryPath $RepositoryPath)
        if ($remaining.Count -eq 0) {
            break
        }
        Start-Sleep -Milliseconds 100
    } while ([DateTime]::UtcNow -lt $deadline)
    if ($remaining.Count -ne 0) {
        throw 'MagicHandy did not exit after its process tree was stopped. The rebuild was not started.'
    }
    Remove-MagicHandyBuildBackups -RepositoryPath $RepositoryPath
}

function Ensure-MagicHandyOllamaModel {
    param(
        [Parameter(Mandatory = $true)][string]$OllamaExecutable,
        [string]$Model
    )
    if ([string]::IsNullOrWhiteSpace($Model)) {
        return
    }
    $previousPreference = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        & $OllamaExecutable show $Model *> $null
        $present = $LASTEXITCODE -eq 0
    } finally {
        $ErrorActionPreference = $previousPreference
    }
    if ($present) {
        Write-Host "Ollama model '$Model' is already present." -ForegroundColor Green
        return
    }
    Write-Host "Pulling Ollama model '$Model'. Model downloads may use several GB."
    & $OllamaExecutable pull $Model | Out-Host
    if ($LASTEXITCODE -ne 0) {
        throw "Ollama could not pull '$Model' (exit $LASTEXITCODE)."
    }
}

function ConvertTo-MagicHandyQuotedLiteral([string]$Value) {
    return "'" + $Value.Replace("'", "''") + "'"
}

function Write-MagicHandyLauncher {
    param(
        [Parameter(Mandatory = $true)][string]$RepositoryPath,
        [Parameter(Mandatory = $true)][string]$DataDir,
        [Parameter(Mandatory = $true)][int]$Port
    )
    $launcher = Join-Path $RepositoryPath 'Start-MagicHandy.ps1'
    $support = Join-Path $RepositoryPath 'scripts\installer\InstallerSupport.psm1'
    $supportLiteral = ConvertTo-MagicHandyQuotedLiteral -Value $support
    $repositoryLiteral = ConvertTo-MagicHandyQuotedLiteral -Value $RepositoryPath
    $dataLiteral = ConvertTo-MagicHandyQuotedLiteral -Value $DataDir
    $content = @"
# Generated by MagicHandy install.ps1. Rerun update.ps1 to refresh it.
`$ErrorActionPreference = 'Stop'
Import-Module $supportLiteral -Force -DisableNameChecking
Start-MagicHandyApp -RepositoryPath $repositoryLiteral -DataDir $dataLiteral -Port $Port
"@
    Write-MagicHandyUTF8 -Path $launcher -Content $content
    Write-Host "Created $launcher" -ForegroundColor Green
}

function Invoke-MagicHandyProvision {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)][object]$State,
        [Parameter(Mandatory = $true)][string]$RepositoryPath,
        [int]$RunningPort = 0,
        [switch]$AssumeYes,
        [switch]$PlanOnly
    )

    if ($PlanOnly) {
        Write-InstallerHeading 'Provisioning plan (no changes made)'
        foreach ($item in (Get-MagicHandyProvisionPlan -State $State)) {
            Write-Host "  - $item"
        }
        return
    }
    if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT -or -not [Environment]::Is64BitOperatingSystem) {
        throw 'The source installer currently requires 64-bit Windows.'
    }

    New-Item -ItemType Directory -Force -Path $State.data_dir | Out-Null
    Write-InstallerHeading 'Build MagicHandy and worker adapters'
    if ($RunningPort -eq 0) {
        $RunningPort = [int]$State.port
    }
    Stop-MagicHandyAppForRebuild -RepositoryPath $RepositoryPath -Port $RunningPort -AllowPhysicalStopConfirmation:(-not $AssumeYes)
    $go = Ensure-MagicHandyGo -AssumeYes:$AssumeYes
    Build-MagicHandyBinaries -RepositoryPath $RepositoryPath -GoExecutable $go

    if ([bool]$State.build_managed_llama) {
        Write-InstallerHeading "Managed llama.cpp ($($State.llama_backend))"
        $git = Ensure-MagicHandyGit -AssumeYes:$AssumeYes
        $cmake = Ensure-MagicHandyCMake -AssumeYes:$AssumeYes
        Ensure-MagicHandyVCToolchain -AssumeYes:$AssumeYes
        $cuda = ''
        if ([string]$State.llama_backend -eq 'cuda') {
            $cuda = Ensure-MagicHandyCUDA -AssumeYes:$AssumeYes
        }
        $builder = Join-Path $RepositoryPath 'internal\llm\runtimeassets\build-managed-llama.ps1'
        & $builder -DataDir $State.data_dir -Backend $State.llama_backend
        if ($LASTEXITCODE -ne 0) {
            throw "Managed llama.cpp build failed (exit $LASTEXITCODE)."
        }

        Write-InstallerHeading 'NeuTTS Air runtime (with managed llama.cpp)'
        $espeak = Ensure-MagicHandyESpeak -AssumeYes:$AssumeYes
        Restore-MagicHandyNeuTTSBackup -DataDir $State.data_dir
        if (Test-MagicHandyNeuTTSInstall -DataDir $State.data_dir -Backend $State.llama_backend) {
            Write-Host "NeuTTS runtime is already checksum-verified at $(Join-Path $State.data_dir 'voice\neutts\active\runtime')." -ForegroundColor Green
        } else {
            Confirm-MagicHandyNeuTTSInstall -Backend $State.llama_backend -AssumeYes:$AssumeYes
            $libClang = Ensure-MagicHandyLibClang -AssumeYes:$AssumeYes
            $rustup = Ensure-MagicHandyRustup -AssumeYes:$AssumeYes
            Install-MagicHandyNeuTTS -DataDir $State.data_dir -GitExecutable $git -RustupExecutable $rustup -LibClangPath $libClang -CMakeExecutable $cmake -ESpeakExecutable $espeak -Backend $State.llama_backend -CUDAExecutable $cuda
        }
    }

    if ([bool]$State.ensure_ollama) {
        Write-InstallerHeading 'Ollama provider'
        $ollama = Ensure-MagicHandyOllama -AssumeYes:$AssumeYes
        Ensure-MagicHandyOllamaModel -OllamaExecutable $ollama -Model ([string]$State.ollama_model)
    }

    if ([bool]$State.install_parakeet) {
        Write-InstallerHeading 'Offline Parakeet speech input'
        Install-MagicHandyParakeet -DataDir $State.data_dir
    }

    if ([bool]$State.create_launcher) {
        Write-InstallerHeading 'Launcher'
        Write-MagicHandyLauncher -RepositoryPath $RepositoryPath -DataDir $State.data_dir -Port ([int]$State.port)
    }
}

function Start-MagicHandyApp {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)][string]$RepositoryPath,
        [Parameter(Mandatory = $true)][string]$DataDir,
        [Parameter(Mandatory = $true)][int]$Port,
        [switch]$NoBrowser
    )
    $exe = Join-Path $RepositoryPath 'magichandy.exe'
    if (-not (Test-Path -LiteralPath $exe)) {
        throw "MagicHandy executable not found at '$exe'."
    }
    $url = "http://127.0.0.1:$Port"
    $listeners = @(Get-MagicHandyLoopbackListeners -Port $Port)
    if ($listeners.Count -gt 0) {
        $ownedProcesses = @(Get-MagicHandyCheckoutProcesses -RepositoryPath $RepositoryPath)
        $ownedIDs = @($ownedProcesses | ForEach-Object { [int]$_.ProcessId })
        $ownedListeners = @($listeners | Where-Object { [int]$_.OwningProcess -in $ownedIDs })
        $foreignListeners = @($listeners | Where-Object { [int]$_.OwningProcess -notin $ownedIDs })
        if ($ownedProcesses.Count -eq 1 -and $ownedListeners.Count -gt 0 -and $foreignListeners.Count -eq 0 -and (Test-MagicHandyHTTPReady -Port $Port)) {
            if (-not $NoBrowser) {
                Start-Process $url
            }
            Write-Host "MagicHandy is already running at $url" -ForegroundColor Green
            return
        }
        throw "Port $Port is already in use by a process that cannot be verified as this checkout's MagicHandy app."
    }

    $argumentLine = New-MagicHandyAppArgumentLine -Address "127.0.0.1:$Port" -DataDir $DataDir
    $process = Start-Process -FilePath $exe -ArgumentList $argumentLine -PassThru -WindowStyle Hidden
    $deadline = [DateTime]::UtcNow.AddSeconds(10)
    do {
        $process.Refresh()
        if ($process.HasExited) {
            throw "MagicHandy exited before its local server became ready (exit $($process.ExitCode))."
        }
        if (Test-MagicHandyProcessReady -Port $Port -TargetProcessId $process.Id) {
            if (-not $NoBrowser) {
                Start-Process $url
            }
            Write-Host "MagicHandy is running at $url" -ForegroundColor Green
            return
        }
        Start-Sleep -Milliseconds 250
    } while ([DateTime]::UtcNow -lt $deadline)

    if (-not $process.HasExited) {
        Stop-MagicHandyProcessTree -TargetProcessId $process.Id
    }
    throw "MagicHandy did not become ready at $url within 10 seconds. The failed process was stopped."
}

function Update-MagicHandySource {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)][string]$RepositoryPath,
        [switch]$AssumeYes
    )
    $git = Ensure-MagicHandyGit -AssumeYes:$AssumeYes
    if (-not (Test-Path -LiteralPath (Join-Path $RepositoryPath '.git'))) {
        throw "'$RepositoryPath' is not a Git checkout. Download a current source archive and run install.ps1 instead."
    }
    $status = & $git -C $RepositoryPath status --porcelain --untracked-files=normal
    if ($LASTEXITCODE -ne 0) {
        throw 'Could not inspect the Git worktree.'
    }
    if (-not [string]::IsNullOrWhiteSpace(($status -join "`n"))) {
        throw 'The MagicHandy worktree has local changes. Source update was not attempted. Preserve and reconcile those changes first, or use update.ps1 -NoPull to rebuild the current checkout.'
    }

    $branchOutput = @(& $git -C $RepositoryPath symbolic-ref --quiet --short HEAD 2>$null)
    $branchExit = $LASTEXITCODE
    if ($branchExit -ne 0 -or $branchOutput.Count -ne 1 -or [string]::IsNullOrWhiteSpace([string]$branchOutput[0])) {
        throw 'Source update requires a named branch. Detached HEAD and in-progress rebases are not updated.'
    }
    $currentBranch = ([string]$branchOutput[0]).Trim()
    $releaseRef = 'refs/remotes/origin/main'
    $releaseRefspec = '+refs/heads/main:refs/remotes/origin/main'
    $targetRef = ''
    $targetDisplay = ''

    if ($currentBranch -eq 'main') {
        $fetchOutput = @(& $git -C $RepositoryPath fetch --prune origin $releaseRefspec)
        $fetchExit = $LASTEXITCODE
        $fetchOutput | Out-Host
        if ($fetchExit -ne 0) {
            throw "Fetching origin failed (exit $fetchExit). No branch, index, or working-tree files were changed."
        }
        & $git -C $RepositoryPath show-ref --verify --quiet $releaseRef
        if ($LASTEXITCODE -ne 0) {
            throw 'origin/main is unavailable after fetch. No source files were changed.'
        }
        $targetRef = $releaseRef
        $targetDisplay = 'origin/main'
    } else {
        $localRef = "refs/heads/$currentBranch"
        $upstreamOutput = @(& $git -C $RepositoryPath for-each-ref '--format=%(upstream)' $localRef)
        $upstreamExit = $LASTEXITCODE
        $remoteOutput = @(& $git -C $RepositoryPath for-each-ref '--format=%(upstream:remotename)' $localRef)
        $remoteExit = $LASTEXITCODE
        $remoteRefOutput = @(& $git -C $RepositoryPath for-each-ref '--format=%(upstream:remoteref)' $localRef)
        $remoteRefExit = $LASTEXITCODE
        if ($upstreamExit -ne 0 -or $remoteExit -ne 0 -or $remoteRefExit -ne 0) {
            throw 'Could not inspect the current branch upstream. No source files were changed.'
        }
        $upstreamRef = ($upstreamOutput -join '').Trim()
        $upstreamRemote = ($remoteOutput -join '').Trim()
        $upstreamRemoteRef = ($remoteRefOutput -join '').Trim()
        if ([string]::IsNullOrWhiteSpace($upstreamRef) -or [string]::IsNullOrWhiteSpace($upstreamRemote) -or [string]::IsNullOrWhiteSpace($upstreamRemoteRef)) {
            throw "The current branch '$currentBranch' has no configured upstream. No source files were changed."
        }

        $remoteMatch = @(& $git -C $RepositoryPath ls-remote --exit-code --heads $upstreamRemote $upstreamRemoteRef 2>$null)
        $remoteMatchExit = $LASTEXITCODE
        if ($remoteMatchExit -eq 0) {
            $featureRefspec = '+{0}:{1}' -f $upstreamRemoteRef, $upstreamRef
            $fetchOutput = @(& $git -C $RepositoryPath fetch --prune $upstreamRemote $featureRefspec)
            $fetchExit = $LASTEXITCODE
            $fetchOutput | Out-Host
            if ($fetchExit -ne 0) {
                throw "Fetching '$upstreamRemoteRef' from '$upstreamRemote' failed (exit $fetchExit). No branch, index, or working-tree files were changed."
            }
            & $git -C $RepositoryPath show-ref --verify --quiet $upstreamRef
            if ($LASTEXITCODE -ne 0) {
                throw 'The configured upstream was not available after fetch. No source files were changed.'
            }
            $targetRef = $upstreamRef
            $targetDisplay = $upstreamRef -replace '^refs/remotes/', ''
        } elseif ($remoteMatchExit -eq 2 -and $upstreamRemote -ne '.') {
            $fetchOutput = @(& $git -C $RepositoryPath fetch --prune origin $releaseRefspec)
            $fetchExit = $LASTEXITCODE
            $fetchOutput | Out-Host
            if ($fetchExit -ne 0) {
                throw "The feature upstream was deleted, and fetching origin/main failed (exit $fetchExit). No source files were changed."
            }
            & $git -C $RepositoryPath show-ref --verify --quiet $releaseRef
            if ($LASTEXITCODE -ne 0) {
                throw 'The feature upstream was deleted, but origin/main is unavailable. No source files were changed.'
            }
            & $git -C $RepositoryPath merge-base --is-ancestor HEAD $releaseRef
            $ancestorExit = $LASTEXITCODE
            if ($ancestorExit -eq 1) {
                throw "The deleted feature upstream cannot be replaced by origin/main because '$currentBranch' contains commits not present there. No branch, index, or working-tree files were changed."
            }
            if ($ancestorExit -ne 0) {
                throw 'Could not verify whether the deleted feature branch is contained in origin/main. No source files were changed.'
            }
            Write-Host "The upstream for '$currentBranch' was deleted after merge; fast-forwarding from origin/main without changing branch or upstream settings." -ForegroundColor Cyan
            $targetRef = $releaseRef
            $targetDisplay = 'origin/main'
        } elseif ($remoteMatchExit -eq 2) {
            throw 'The configured local upstream no longer exists. No source files were changed.'
        } else {
            throw "Could not inspect '$upstreamRemoteRef' on '$upstreamRemote' (exit $remoteMatchExit). No source files were changed."
        }
    }

    Write-Host "Fast-forwarding '$currentBranch' from '$targetDisplay'..."
    $mergeOutput = @(& $git -C $RepositoryPath merge --ff-only $targetRef)
    $mergeExit = $LASTEXITCODE
    $mergeOutput | Out-Host
    if ($mergeExit -ne 0) {
        throw "Cannot fast-forward '$currentBranch' to '$targetDisplay'. Remote refs were fetched, but the branch, index, and working-tree files were not changed."
    }
}

Export-ModuleMember -Function @(
    'Write-InstallerHeading',
    'Write-MagicHandyBanner',
    'Write-MagicHandyCompletionArt',
    'Confirm-MagicHandyChoice',
    'Read-MagicHandyValue',
    'Read-MagicHandyOptionalValue',
    'Read-MagicHandyBackend',
    'Resolve-MagicHandyExecutable',
    'Get-MagicHandyInstallStatePath',
    'New-MagicHandyInstallState',
    'Read-MagicHandyInstallState',
    'Write-MagicHandyInstallState',
    'Show-MagicHandyInstallState',
    'Get-MagicHandyProvisionPlan',
    'Ensure-MagicHandyGit',
    'Invoke-MagicHandyProvision',
    'Start-MagicHandyApp',
    'Update-MagicHandySource'
)
