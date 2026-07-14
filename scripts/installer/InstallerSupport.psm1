Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$script:InstallStateSchema = 1
$script:MinimumGoVersion = [Version]'1.25.0'
$script:ParakeetRunnerURL = 'https://github.com/mudler/parakeet.cpp/releases/download/v0.4.0/parakeet-v0.4.0-bin-win-cpu-x64.zip'
$script:ParakeetRunnerSHA256 = '2880150a1bad2944baed46f2e6bb9f1bc55263a9f2bb85573785a7ec4fa35f27'
$script:ParakeetModelURL = 'https://huggingface.co/mudler/parakeet-cpp-gguf/resolve/main/tdt-0.6b-v3-q4_k.gguf?download=true'
$script:ParakeetModelSHA256 = '993d73feb4206dadda865ab25bd64b50c48dc4d013c3bf6126a721f28b1d5ee8'

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
        'Install' { 'Open Settings to select a model, voice provider, and device transport. NeuTTS external runtime assets are not installed.' }
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
    return -not [string]::IsNullOrWhiteSpace(($installation | Select-Object -First 1))
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
    $ollama = if ([bool]$State.ensure_ollama) { 'yes' } else { 'no' }
    $parakeet = if ([bool]$State.install_parakeet) { 'yes' } else { 'no' }
    $launcher = if ([bool]$State.create_launcher) { 'yes' } else { 'no' }
    Write-Host "  Data directory:   $($State.data_dir)"
    Write-Host "  Local port:       $($State.port)"
    Write-Host "  Managed llama.cpp: $managed"
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
    $plan.Add('Build Parakeet, NeuTTS Air, and ElevenLabs Go protocol adapters (external NeuTTS runtime/assets not included)')
    if ([bool]$State.build_managed_llama) {
        $plan.Add('Ensure Git and CMake are installed')
        $plan.Add('Ensure the Visual Studio C++ Build Tools workload and Windows SDK are installed')
        if ([string]$State.llama_backend -eq 'cuda') {
            $plan.Add('Ensure the NVIDIA CUDA Toolkit is installed')
        }
        $plan.Add("Build and activate pinned managed llama.cpp ($($State.llama_backend))")
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
        Confirm-MagicHandyPackageInstall -Name 'Git for Windows' -Purpose 'updating MagicHandy and fetching pinned llama.cpp source' -License 'GPL-2.0; https://gitforwindows.org/' -AssumeYes:$AssumeYes
        Invoke-MagicHandyWinGetInstall -ID 'Git.Git' -AssumeYes:$AssumeYes
        $git = Resolve-MagicHandyExecutable -Name 'git'
    }
    if (-not $git) {
        throw 'Git installation completed but git.exe is unavailable. Restart PowerShell and rerun the script.'
    }
    return $git
}

function Ensure-MagicHandyCMake {
    [CmdletBinding()]
    param([switch]$AssumeYes)

    $cmake = Resolve-MagicHandyCMake
    if (-not $cmake) {
        Confirm-MagicHandyPackageInstall -Name 'CMake' -Purpose 'configuring the managed llama.cpp source build' -License 'BSD-3-Clause; https://cmake.org/licensing/' -AssumeYes:$AssumeYes
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
    Confirm-MagicHandyPackageInstall -Name 'Visual Studio Build Tools with Desktop C++' -Purpose 'compiling the managed llama.cpp runner' -License 'Microsoft Visual Studio license; https://visualstudio.microsoft.com/license-terms/' -Size 'several GB' -AssumeYes:$AssumeYes
    $override = '--wait --quiet --norestart --nocache --add Microsoft.VisualStudio.Workload.VCTools --includeRecommended'
    Invoke-MagicHandyWinGetInstall -ID 'Microsoft.VisualStudio.BuildTools' -Override $override -AssumeYes:$AssumeYes
    if (-not (Test-MagicHandyVCToolchain)) {
        throw 'The Visual Studio C++ workload is still unavailable. Restart Windows if requested, then rerun install.ps1.'
    }
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

function Install-MagicHandyVerifiedDownload {
    param(
        [Parameter(Mandatory = $true)][string]$Uri,
        [Parameter(Mandatory = $true)][string]$Destination,
        [Parameter(Mandatory = $true)][string]$ExpectedSHA256
    )
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

    $partial = "$Destination.partial"
    if (Test-Path -LiteralPath $partial) {
        Remove-Item -LiteralPath $partial -Recurse -Force
    }
    try {
        Write-Host "Downloading $(Split-Path -Leaf $Destination)..."
        Invoke-WebRequest -UseBasicParsing -Uri $Uri -OutFile $partial
        $actual = Get-MagicHandySHA256 -Path $partial
        if ($actual -ne $expected) {
            throw "SHA-256 verification failed for $(Split-Path -Leaf $Destination)."
        }
        Move-Item -LiteralPath $partial -Destination $Destination -Force
        Write-Host "Verified $(Split-Path -Leaf $Destination)." -ForegroundColor Green
    } finally {
        if (Test-Path -LiteralPath $partial) {
            Remove-Item -LiteralPath $partial -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
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
        Ensure-MagicHandyGit -AssumeYes:$AssumeYes | Out-Null
        Ensure-MagicHandyCMake -AssumeYes:$AssumeYes | Out-Null
        Ensure-MagicHandyVCToolchain -AssumeYes:$AssumeYes
        if ([string]$State.llama_backend -eq 'cuda') {
            Ensure-MagicHandyCUDA -AssumeYes:$AssumeYes | Out-Null
        }
        $builder = Join-Path $RepositoryPath 'internal\llm\runtimeassets\build-managed-llama.ps1'
        & $builder -DataDir $State.data_dir -Backend $State.llama_backend
        if ($LASTEXITCODE -ne 0) {
            throw "Managed llama.cpp build failed (exit $LASTEXITCODE)."
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
