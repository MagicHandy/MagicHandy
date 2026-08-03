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

function Test-UvExecutable {
    [CmdletBinding()]
    param([Parameter(Mandatory = $true)][string]$Path)

    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        return $false
    }
    try {
        $reported = @(& $Path --version 2>&1) -join ' '
        return $LASTEXITCODE -eq 0 -and $reported -match '^uv\s+\d+\.\d+'
    } catch {
        Write-Verbose "Ignoring unusable uv candidate '$Path': $($_.Exception.Message)"
        return $false
    }
}

function Resolve-Uv {
    [CmdletBinding()]
    param([scriptblock]$Probe)

    if ($null -eq $Probe) {
        $Probe = { param([string]$Candidate) Test-UvExecutable -Path $Candidate }
    }

    $candidates = @()
    $command = Get-Command 'uv.exe' -ErrorAction SilentlyContinue
    if ($command -and -not [string]::IsNullOrWhiteSpace([string]$command.Source)) {
        $candidates += [string]$command.Source
    }
    $candidates += @(
        (Join-Path $env:USERPROFILE '.local\bin\uv.exe'),
        (Join-Path $env:LOCALAPPDATA 'Programs\uv\uv.exe')
    )

    # WinGet portable links can be visible before Windows can execute them in
    # the current logon session. Prefer the real package binary when present.
    foreach ($packageRoot in @(
        (Join-Path $env:LOCALAPPDATA 'Microsoft\WinGet\Packages'),
        (Join-Path $env:ProgramFiles 'WinGet\Packages')
    )) {
        if (-not (Test-Path -LiteralPath $packageRoot -PathType Container)) {
            continue
        }
        $packageDirectories = @(
            Get-ChildItem -LiteralPath $packageRoot -Directory -Filter 'astral-sh.uv_*' -ErrorAction SilentlyContinue |
                Sort-Object LastWriteTimeUtc -Descending
        )
        foreach ($packageDirectory in $packageDirectories) {
            $candidates += @(
                Get-ChildItem -LiteralPath $packageDirectory.FullName -File -Filter 'uv.exe' -Recurse -ErrorAction SilentlyContinue |
                    Select-Object -ExpandProperty FullName
            )
        }
    }

    $candidates += @(
        (Join-Path $env:LOCALAPPDATA 'Microsoft\WinGet\Links\uv.exe'),
        (Join-Path $env:ProgramFiles 'WinGet\Links\uv.exe')
    )
    foreach ($candidate in @($candidates | Select-Object -Unique)) {
        if (-not [string]::IsNullOrWhiteSpace([string]$candidate) -and (& $Probe ([string]$candidate))) {
            return [System.IO.Path]::GetFullPath([string]$candidate)
        }
    }
    return $null
}

function Ensure-Uv {
    $uv = Resolve-Uv
    if ($uv) {
        return $uv
    }
    Confirm-TTSAction 'uv is required to install app-owned Python and voice packages. Install uv with WinGet?'
    Invoke-MagicHandyWinGetInstall -ID 'astral-sh.uv' -AssumeYes:$Yes
    for ($attempt = 0; $attempt -lt 10 -and -not $uv; $attempt++) {
        $uv = Resolve-Uv
        if (-not $uv) {
            Start-Sleep -Milliseconds 250
        }
    }
    if (-not $uv) {
        throw 'uv was installed, but no runnable uv.exe could be found. Repair astral-sh.uv in WinGet and retry setup.'
    }
    return $uv
}

function Initialize-TTSGit {
    [CmdletBinding()]
    param([Parameter(Mandatory = $true)][string]$Git)

    try {
        $reported = @(& $Git --version 2>&1) -join ' '
    } catch {
        throw "Git was found at '$Git' but could not run. Repair Git for Windows and retry setup: $($_.Exception.Message)"
    }
    if ($LASTEXITCODE -ne 0 -or $reported -notmatch '^git version\s+\d+') {
        throw "Git was found at '$Git' but failed its version probe (reported '$reported'). Repair Git for Windows and retry setup."
    }

    $resolved = [System.IO.Path]::GetFullPath($Git)
    $directory = Split-Path -Parent $resolved
    $pathEntries = @($env:Path -split ';' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
    if (-not [bool]($pathEntries | Where-Object { $_.TrimEnd('\') -eq $directory.TrimEnd('\') })) {
        $env:Path = if ([string]::IsNullOrWhiteSpace($env:Path)) { $directory } else { "$directory;$env:Path" }
    }
    return $resolved
}

function Get-TTSNvidiaGPUName {
    $nvidia = Get-Command 'nvidia-smi.exe' -ErrorAction SilentlyContinue
    if (-not $nvidia -or [string]::IsNullOrWhiteSpace([string]$nvidia.Source)) {
        throw 'Faster Qwen3-TTS requires a working NVIDIA driver, but nvidia-smi.exe was not found.'
    }

    $previousErrorAction = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $gpuNames = @(& $nvidia.Source --query-gpu=name --format=csv,noheader 2>&1)
        $exitCode = $LASTEXITCODE
    } catch {
        throw "The NVIDIA driver probe could not start. Repair the NVIDIA driver before installing Faster Qwen3-TTS: $($_.Exception.Message)"
    } finally {
        $ErrorActionPreference = $previousErrorAction
    }
    $reported = @($gpuNames | ForEach-Object { [string]$_ }) -join '; '
    $usableNames = @($gpuNames | ForEach-Object { [string]$_ } | Where-Object {
        -not [string]::IsNullOrWhiteSpace($_) -and $_ -notmatch '(?i)no devices were found'
    })
    if ($exitCode -ne 0 -or $usableNames.Count -eq 0) {
        throw "The NVIDIA driver probe failed (exit $exitCode; reported '$reported'). Repair the driver before installing Faster Qwen3-TTS."
    }
    return $usableNames -join ', '
}

function Get-TTSPythonVersion {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$PythonVersion
    )

    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        return $null
    }

    $previousErrorAction = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $reported = @(& $Path --version 2>&1) -join ' '
        $exitCode = $LASTEXITCODE
    } catch {
        Write-Verbose "Ignoring unusable Python candidate '$Path': $($_.Exception.Message)"
        return $null
    } finally {
        $ErrorActionPreference = $previousErrorAction
    }
    if ($exitCode -ne 0 -or $reported -notmatch "^Python $([regex]::Escape($PythonVersion))\.") {
        return $null
    }
    return $reported
}

function Get-TTSVirtualEnvironmentVersion {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)][string]$Root,
        [Parameter(Mandatory = $true)][string]$ManagedPythonRoot,
        [Parameter(Mandatory = $true)][string]$PythonVersion
    )

    $python = Join-Path $Root 'Scripts\python.exe'
    $configPath = Join-Path $Root 'pyvenv.cfg'
    if (-not (Test-Path -LiteralPath $python -PathType Leaf) -or
        -not (Test-Path -LiteralPath $configPath -PathType Leaf)) {
        return $null
    }

    try {
        $config = [System.IO.File]::ReadAllText($configPath)
        $homeMatch = [regex]::Match($config, '(?im)^home\s*=\s*(?<path>[^\r\n]+?)\s*$')
        if (-not $homeMatch.Success -or
            $config -notmatch '(?im)^include-system-site-packages\s*=\s*false\s*$') {
            return $null
        }
        $pythonHome = [System.IO.Path]::GetFullPath($homeMatch.Groups['path'].Value.Trim())
        $managedPrefix = [System.IO.Path]::GetFullPath($ManagedPythonRoot).TrimEnd('\') + '\'
        if (-not $pythonHome.StartsWith($managedPrefix, [StringComparison]::OrdinalIgnoreCase)) {
            return $null
        }
        $homeName = Split-Path -Leaf $pythonHome
        if ($homeName -notmatch "^cpython-$([regex]::Escape($PythonVersion))\.\d+-windows-x86_64-none$") {
            return $null
        }
        $basePython = Join-Path $pythonHome 'python.exe'
        if ([string]::IsNullOrWhiteSpace((Get-TTSPythonVersion -Path $basePython -PythonVersion $PythonVersion))) {
            return $null
        }
    } catch {
        Write-Verbose "Ignoring invalid virtual environment configuration '$configPath': $($_.Exception.Message)"
        return $null
    }

    return Get-TTSPythonVersion -Path $python -PythonVersion $PythonVersion
}

function Find-TTSManagedPython {
    param(
        [Parameter(Mandatory = $true)][string]$InstallDirectory,
        [Parameter(Mandatory = $true)][string]$PythonVersion
    )

    if (-not (Test-Path -LiteralPath $InstallDirectory -PathType Container)) {
        return $null
    }

    $requested = [Version]"$PythonVersion.0"
    $candidates = foreach ($directory in (Get-ChildItem -LiteralPath $InstallDirectory -Directory -ErrorAction SilentlyContinue)) {
        if ($directory.Name -notmatch '^cpython-(?<version>\d+\.\d+\.\d+)-windows-x86_64-none$') {
            continue
        }
        $candidateVersion = [Version]$Matches.version
        if ($candidateVersion.Major -ne $requested.Major -or $candidateVersion.Minor -ne $requested.Minor) {
            continue
        }
        $candidate = Join-Path $directory.FullName 'python.exe'
        if (-not (Test-Path -LiteralPath $candidate -PathType Leaf)) {
            continue
        }
        $reported = Get-TTSPythonVersion -Path $candidate -PythonVersion $PythonVersion
        if (-not [string]::IsNullOrWhiteSpace($reported)) {
            [pscustomobject]@{
                Path = [System.IO.Path]::GetFullPath($candidate)
                Version = $candidateVersion
            }
        }
    }

    $selected = $candidates | Sort-Object Version -Descending | Select-Object -First 1
    if ($null -eq $selected) {
        return $null
    }
    return [string]$selected.Path
}

function Initialize-TTSPythonEnvironment {
    param(
        [Parameter(Mandatory = $true)][string]$Uv,
        [Parameter(Mandatory = $true)][string]$Root,
        [Parameter(Mandatory = $true)][string]$PythonVersion
    )

    $venv = Join-Path $Root '.venv'
    $python = Join-Path $venv 'Scripts\python.exe'
    $managedPythonRoot = Join-Path $Root 'managed-python'
    $uvCacheRoot = Join-Path $Root 'uv-cache'
    $uvCredentialsRoot = Join-Path $Root 'uv-credentials'
    Assert-MagicHandyChildPath -Root $Root -Candidate $venv
    Assert-MagicHandyChildPath -Root $Root -Candidate $managedPythonRoot
    Assert-MagicHandyChildPath -Root $Root -Candidate $uvCacheRoot
    Assert-MagicHandyChildPath -Root $Root -Candidate $uvCredentialsRoot
    New-Item -ItemType Directory -Path $Root -Force | Out-Null

    # Keep uv's runtime, cache, and credential lock files inside the TTS module.
    # The process-scoped settings also prevent later dependency commands from
    # touching uv's global profile directories during this installer run.
    $env:UV_PYTHON_INSTALL_DIR = $managedPythonRoot
    $env:UV_PYTHON_INSTALL_BIN = '0'
    $env:UV_PYTHON_INSTALL_REGISTRY = '0'
    $env:UV_CACHE_DIR = $uvCacheRoot
    $env:UV_CREDENTIALS_DIR = $uvCredentialsRoot

    if (Test-Path -LiteralPath $python -PathType Leaf) {
        $reportedVersion = Get-TTSVirtualEnvironmentVersion `
            -Root $venv `
            -ManagedPythonRoot $managedPythonRoot `
            -PythonVersion $PythonVersion
        if (-not [string]::IsNullOrWhiteSpace($reportedVersion)) {
            Write-Host "Reusing the existing $reportedVersion environment; dependency checks will repair only changed packages."
            return [pscustomobject]@{
                Root = $venv
                Python = $python
                Version = $reportedVersion
            }
        }
        Write-Warning "Replacing the module environment because it is not a runnable, app-owned Python $PythonVersion environment with an exact patch home."
        Remove-Item -LiteralPath $venv -Recurse -Force
    } elseif (Test-Path -LiteralPath $venv) {
        Write-Warning 'Replacing an incomplete managed Python environment from an interrupted setup.'
        Remove-Item -LiteralPath $venv -Recurse -Force
    }

    $managedPython = Find-TTSManagedPython -InstallDirectory $managedPythonRoot -PythonVersion $PythonVersion
    if ([string]::IsNullOrWhiteSpace($managedPython)) {
        $previousErrorAction = $ErrorActionPreference
        Push-Location -LiteralPath $Root
        try {
            $ErrorActionPreference = 'Continue'
            try {
                $installOutput = @(& $Uv python install --install-dir 'managed-python' --no-bin --no-registry $PythonVersion 2>&1)
                $installExit = $LASTEXITCODE
            } finally {
                $ErrorActionPreference = $previousErrorAction
            }
        } finally {
            Pop-Location
        }
        $installOutput | ForEach-Object { Write-Host $_ }
        $installText = @($installOutput | ForEach-Object { [string]$_ }) -join "`n"
        $managedPython = Find-TTSManagedPython -InstallDirectory $managedPythonRoot -PythonVersion $PythonVersion
        if ([string]::IsNullOrWhiteSpace($managedPython)) {
            throw "Python $PythonVersion installation did not produce a runnable patch-specific interpreter (uv exit $installExit)."
        }
        if ($installExit -ne 0) {
            $isJunctionOnlyFailure =
                $installText -match '(?i)Failed to create Python minor version link directory' -and
                $installText -match '(?i)untrusted mount point|os error 448'
            if (-not $isJunctionOnlyFailure) {
                throw "Python $PythonVersion installation failed after extraction (uv exit $installExit); refusing to mask an unrelated uv failure."
            }
            Write-Warning 'uv installed a usable app-owned Python but Windows refused its optional minor-version junction; continuing with the patch-specific interpreter.'
        }
    } else {
        Write-Host "Reusing app-owned managed Python at '$managedPython'."
    }

    Assert-MagicHandyChildPath -Root $managedPythonRoot -Candidate $managedPython
    # uv's Windows venv launcher records the optional minor-version junction as
    # its home even when an exact patch interpreter is supplied. On redirected
    # profiles that junction can be rejected after extraction, leaving a
    # trampoline that cannot start. CPython's own venv writes the real
    # patch-specific home and standard Windows launchers instead.
    Invoke-Checked `
        -Executable $managedPython `
        -Arguments @('-m', 'venv', '--without-pip', '.venv') `
        -WorkingDirectory $Root `
        -Description 'Python environment creation'
    if (-not (Test-Path -LiteralPath $python -PathType Leaf)) {
        throw "The isolated Python executable was not created at '$python'."
    }
    $verifiedVersion = Get-TTSVirtualEnvironmentVersion `
        -Root $venv `
        -ManagedPythonRoot $managedPythonRoot `
        -PythonVersion $PythonVersion
    if ([string]::IsNullOrWhiteSpace($verifiedVersion)) {
        throw "The isolated environment did not produce a runnable Python $PythonVersion interpreter."
    }
    return [pscustomobject]@{
        Root = $venv
        Python = $python
        Version = $verifiedVersion
    }
}

function Test-TTSPythonRuntime {
    param(
        [Parameter(Mandatory = $true)][string]$Python,
        [Parameter(Mandatory = $true)][string]$Module,
        [Parameter(Mandatory = $true)][string]$RuntimeDevice,
        [Parameter(Mandatory = $true)][string]$Probe,
        [string]$NumbaCacheDirectory = ''
    )

    if (-not (Test-Path -LiteralPath $Probe -PathType Leaf)) {
        throw "The Python voice runtime probe is unavailable: '$Probe'."
    }
    $previousNumbaCache = [System.Environment]::GetEnvironmentVariable(
        'NUMBA_CACHE_DIR',
        [System.EnvironmentVariableTarget]::Process
    )
    try {
        if (-not [string]::IsNullOrWhiteSpace($NumbaCacheDirectory)) {
            New-Item -ItemType Directory -Path $NumbaCacheDirectory -Force | Out-Null
            [System.Environment]::SetEnvironmentVariable(
                'NUMBA_CACHE_DIR',
                $NumbaCacheDirectory,
                [System.EnvironmentVariableTarget]::Process
            )
        }
        Invoke-Checked `
            -Executable $Python `
            -Arguments @($Probe, $Module, $RuntimeDevice) `
            -Description 'Python voice runtime verification'
    } finally {
        [System.Environment]::SetEnvironmentVariable(
            'NUMBA_CACHE_DIR',
            $previousNumbaCache,
            [System.EnvironmentVariableTarget]::Process
        )
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
        [Parameter(Mandatory = $true)][string]$Description,
        [string]$WorkingDirectory = ''
    )

    $pushedLocation = $false
    if (-not [string]::IsNullOrWhiteSpace($WorkingDirectory)) {
        $WorkingDirectory = [System.IO.Path]::GetFullPath($WorkingDirectory)
        if (-not (Test-Path -LiteralPath $WorkingDirectory -PathType Container)) {
            throw "$Description working directory is unavailable: '$WorkingDirectory'."
        }
        Push-Location -LiteralPath $WorkingDirectory
        $pushedLocation = $true
    }

    $previousErrorAction = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    $exitCode = 1
    try {
        try {
            & $Executable @Arguments
            $exitCode = $LASTEXITCODE
        } catch {
            throw "$Description could not start '$Executable': $($_.Exception.Message)"
        }
    } finally {
        $ErrorActionPreference = $previousErrorAction
        if ($pushedLocation) {
            Pop-Location
        }
    }
    if ($exitCode -ne 0) {
        throw "$Description failed (exit $exitCode)."
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
            $previousPreference = $ErrorActionPreference
            $ErrorActionPreference = 'Continue'
            try {
                & $Executable @arguments
                $exitCode = $LASTEXITCODE
            } catch {
                $exitCode = -1
                Write-Warning "Model download attempt $attempt could not start '$Executable': $($_.Exception.Message)"
            } finally {
                $ErrorActionPreference = $previousPreference
            }
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
    $freshClone = $false
    if (Test-Path -LiteralPath $Destination) {
        if (-not (Test-Path -LiteralPath (Join-Path $Destination '.git') -PathType Container)) {
            throw "The module source path exists but is not a Git checkout: '$Destination'."
        }
    } else {
        $parent = Split-Path -Parent $Destination
        New-Item -ItemType Directory -Path $parent -Force | Out-Null
        $cloneStage = "$Destination.partial-$PID-$([Guid]::NewGuid().ToString('N'))"
        try {
            Invoke-Checked -Executable $Git -Arguments @('clone', '--filter=blob:none', '--no-checkout', $URL, $cloneStage) -Description 'Source clone'
            if (Test-Path -LiteralPath $Destination) {
                throw "The module source path appeared while cloning: '$Destination'. Retry after the other installation finishes."
            }
            Move-Item -LiteralPath $cloneStage -Destination $Destination
            $freshClone = $true
        } finally {
            if (Test-Path -LiteralPath $cloneStage) {
                Remove-Item -LiteralPath $cloneStage -Recurse -Force -ErrorAction SilentlyContinue
            }
        }
    }

    $origin = (& $Git -C $Destination remote get-url origin 2>$null | Out-String).Trim()
    if ($LASTEXITCODE -ne 0 -or
        -not [string]::Equals($origin.TrimEnd('/'), $URL.TrimEnd('/'), [StringComparison]::OrdinalIgnoreCase)) {
        throw "The managed module source remote is not the pinned repository. Preserve or remove '$Destination' before updating."
    }

    InstallerSupport\Add-MagicHandyGitInfoExclusions -RepositoryPath $Destination -RelativePaths $InstallerGeneratedPaths
    $dirty = @(& $Git -C $Destination status --porcelain)
    if ($LASTEXITCODE -ne 0) {
        throw "The managed module source could not be inspected: '$Destination'."
    }
    $worktreeEntries = @(Get-ChildItem -LiteralPath $Destination -Force | Where-Object { $_.Name -ne '.git' })
    $recoverIncompleteCheckout = $freshClone -or $worktreeEntries.Count -eq 0
    if ($dirty.Count -gt 0 -and -not $recoverIncompleteCheckout) {
        throw "The managed module source has local changes. Preserve or remove '$Destination' before updating."
    }
    if ($dirty.Count -gt 0 -and -not $freshClone) {
        Write-Warning "Recovering the incomplete managed source checkout at '$Destination'."
    }
    Invoke-Checked -Executable $Git -Arguments @('-C', $Destination, 'fetch', '--depth', '1', 'origin', $Revision) -Description 'Pinned source fetch'
    Invoke-Checked -Executable $Git -Arguments @('-C', $Destination, 'checkout', '--detach', '--force', 'FETCH_HEAD') -Description 'Pinned source checkout'
    $actual = (& $Git -C $Destination rev-parse HEAD).Trim()
    if ($LASTEXITCODE -ne 0 -or $actual -ne $Revision) {
        throw "Pinned source verification failed. Expected $Revision, got '$actual'."
    }
    $remainingChanges = @(& $Git -C $Destination status --porcelain)
    if ($LASTEXITCODE -ne 0 -or $remainingChanges.Count -gt 0) {
        throw "The pinned module source checkout did not finish cleanly: '$Destination'."
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

if ($Module -eq 'faster-qwen3-tts') {
    $gpuName = Get-TTSNvidiaGPUName
    Write-Host "NVIDIA runtime: $gpuName" -ForegroundColor Green
}

Confirm-TTSAction 'Proceed with the optional TTS module download and installation?'
$git = Initialize-TTSGit -Git (InstallerSupport\Ensure-MagicHandyGit -AssumeYes:$Yes)
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
$relativePython = '.venv\Scripts\python.exe'

$constraintsSource = if ($Module -eq 'faster-qwen3-tts') {
    Join-Path $PSScriptRoot 'tts\faster-qwen-constraints.txt'
} else {
    Join-Path $PSScriptRoot 'tts\chatterbox-constraints.txt'
}
if (-not (Test-Path -LiteralPath $constraintsSource -PathType Leaf)) {
    throw "The $Module dependency constraints are unavailable: '$constraintsSource'."
}
$stagedConstraintsName = '.magichandy-install-constraints.txt'
$stagedConstraints = Join-Path $InstallRoot $stagedConstraintsName
Assert-MagicHandyChildPath -Root $InstallRoot -Candidate $stagedConstraints
Copy-Item -LiteralPath $constraintsSource -Destination $stagedConstraints -Force

# uv parses requirements-file arguments separately on Windows. Absolute paths
# containing spaces (notably Program Files) can be truncated at the first space,
# so every local uv path is relative to the app-owned module directory.
try {
    if ($Module -eq 'faster-qwen3-tts') {
        Invoke-Checked `
            -Executable $uv `
            -Arguments @('pip', 'install', '--python', $relativePython, '--torch-backend', 'cu128', '--constraint', $stagedConstraintsName, '--editable', 'source[demo]') `
            -WorkingDirectory $InstallRoot `
            -Description 'Faster Qwen3-TTS dependency installation'
    } else {
        $requirements = Get-ChatterboxRequirements -RuntimeDevice $Device
        $requirementsPath = Join-Path $sourceRoot $requirements
        if (-not (Test-Path -LiteralPath $requirementsPath -PathType Leaf)) {
            throw "The pinned Chatterbox dependency set is unavailable: '$requirementsPath'."
        }
        $relativeRequirements = Join-Path 'source' $requirements
        Write-Host "Chatterbox dependency set: $requirements"
        Invoke-Checked -Executable $uv -Arguments @('pip', 'install', '--python', $relativePython, '--constraint', $stagedConstraintsName, '-r', $relativeRequirements) -WorkingDirectory $InstallRoot -Description 'Chatterbox dependency installation'
        Invoke-Checked -Executable $uv -Arguments @('pip', 'install', '--python', $relativePython, '--no-deps', $chatterboxEngine, 's3tokenizer==0.3.0', 'onnx==1.16.0') -WorkingDirectory $InstallRoot -Description 'Pinned Chatterbox engine installation'
        # The pinned server intentionally overrides descript-audiotools' obsolete
        # protobuf metadata bound; ONNX 1.16 requires a newer runtime and the
        # Chatterbox maintainers validate that combination at runtime.
        Invoke-Checked -Executable $uv -Arguments @('pip', 'install', '--python', $relativePython, '--no-deps', 'protobuf==4.25.8') -WorkingDirectory $InstallRoot -Description 'Chatterbox ONNX protobuf compatibility installation'
    }
} finally {
    Remove-Item -LiteralPath $stagedConstraints -Force -ErrorAction SilentlyContinue
}

$hf = Join-Path $venv 'Scripts\hf.exe'
if (-not (Test-Path -LiteralPath $hf -PathType Leaf)) {
    Invoke-Checked -Executable $uv -Arguments @('pip', 'install', '--python', $relativePython, 'huggingface-hub>=0.36.0,<1.0') -WorkingDirectory $InstallRoot -Description 'Hugging Face client installation'
}
if (-not (Test-Path -LiteralPath $hf -PathType Leaf)) {
    throw "The Hugging Face client was not installed at '$hf'."
}
if ($Module -eq 'faster-qwen3-tts') {
    Invoke-Checked -Executable $uv -Arguments @('pip', 'check', '--python', $relativePython) -WorkingDirectory $InstallRoot -Description 'Python dependency compatibility check'
} else {
    Write-Host 'Validating Chatterbox through its runtime imports; its pinned ONNX protobuf override intentionally conflicts with obsolete package metadata.'
}
Invoke-Checked -Executable $hf -Arguments @('version') -Description 'Hugging Face client verification'
$numbaCacheDirectory = if ($Module -eq 'faster-qwen3-tts') {
    Join-Path $InstallRoot 'runtime-cache\numba'
} else {
    ''
}
Test-TTSPythonRuntime `
    -Python $python `
    -Module $Module `
    -RuntimeDevice $Device `
    -Probe (Join-Path $PSScriptRoot 'tts\runtime-probe.py') `
    -NumbaCacheDirectory $numbaCacheDirectory
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
