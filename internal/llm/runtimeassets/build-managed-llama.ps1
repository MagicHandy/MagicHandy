#Requires -Version 5.1
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$DataDir,

    [ValidateSet('auto', 'cpu', 'cuda')]
    [string]$Backend = 'auto'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$LlamaVersion = 'b9966'
$LlamaCommit = 'c749cb041706647f460bb918cccc9d91995205ab'
$LlamaRepository = 'https://github.com/ggml-org/llama.cpp.git'

function Resolve-Executable([string]$Name) {
    $command = Get-Command $Name -ErrorAction SilentlyContinue
    if ($null -ne $command) {
        return $command.Source
    }

    $candidates = switch ($Name.ToLowerInvariant()) {
        'git' { @((Join-Path $env:ProgramFiles 'Git\cmd\git.exe')) }
        'cmake' { @((Join-Path $env:ProgramFiles 'CMake\bin\cmake.exe')) }
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

function Resolve-CMake {
    $cmake = Resolve-Executable 'cmake'
    if ($cmake) {
        return $cmake
    }

    $vswhere = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio\Installer\vswhere.exe'
    if (Test-Path -LiteralPath $vswhere) {
        $candidate = & $vswhere -latest -products * -find 'Common7\IDE\CommonExtensions\Microsoft\CMake\CMake\bin\cmake.exe' | Select-Object -First 1
        if ($candidate -and (Test-Path -LiteralPath $candidate)) {
            return $candidate
        }
    }

    foreach ($programFilesRoot in @($env:ProgramFiles, ${env:ProgramFiles(x86)})) {
        $visualStudioRoot = Join-Path $programFilesRoot 'Microsoft Visual Studio'
        if (Test-Path -LiteralPath $visualStudioRoot) {
            $candidate = Get-ChildItem -Path $visualStudioRoot -Filter cmake.exe -File -Recurse -ErrorAction SilentlyContinue |
                Where-Object { $_.FullName -like '*CommonExtensions*Microsoft*CMake*bin*' } |
                Select-Object -First 1
            if ($null -ne $candidate) {
                return $candidate.FullName
            }
        }
    }
    return $null
}

function Test-VCToolchain {
    $vswhere = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio\Installer\vswhere.exe'
    if (-not (Test-Path -LiteralPath $vswhere)) {
        return $false
    }
    $installation = & $vswhere -latest -products * -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
    return -not [string]::IsNullOrWhiteSpace(($installation | Select-Object -First 1))
}

function Invoke-Checked([string]$Executable, [string[]]$Arguments, [string]$Failure) {
    & $Executable @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$Failure (exit $LASTEXITCODE)."
    }
}

function Invoke-Captured([string]$Executable, [string[]]$Arguments, [string]$Failure) {
    $previousPreference = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $output = (& $Executable @Arguments 2>&1 | Out-String)
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousPreference
    }
    if ($exitCode -ne 0) {
        throw "$Failure (exit $exitCode): $($output.Trim())"
    }
    return $output
}

function Write-UTF8NoBOM([string]$Path, [string]$Content) {
    $encoding = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($Path, $Content, $encoding)
}

function Assert-ChildPath([string]$Root, [string]$Candidate) {
    $rootPrefix = [System.IO.Path]::GetFullPath($Root).TrimEnd('\') + '\'
    $resolvedCandidate = [System.IO.Path]::GetFullPath($Candidate)
    if (-not $resolvedCandidate.StartsWith($rootPrefix, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing to modify a path outside the managed llama.cpp runtime root: $resolvedCandidate"
    }
}

function Write-RuntimeManifest(
    [string]$RuntimeRoot,
    [string]$InstallID,
    [string]$SelectedBackend,
    [string]$RunnerRelativePath
) {
    $builtAt = [DateTimeOffset]::UtcNow.ToString('o')
    $installManifest = Join-Path $RuntimeRoot "installs\$InstallID\runtime.json"
    if (Test-Path -LiteralPath $installManifest) {
        try {
            $installed = Get-Content -LiteralPath $installManifest -Raw | ConvertFrom-Json
            if ($installed.version -eq $LlamaVersion -and $installed.commit -eq $LlamaCommit -and $installed.backend -eq $SelectedBackend -and $installed.built_at) {
                $builtAt = [string]$installed.built_at
            }
        } catch {
            Write-Warning 'Existing managed runtime metadata was unreadable; replacing it.'
        }
    }
    $manifest = [ordered]@{
        schema_version = 1
        runtime = 'llama.cpp'
        version = $LlamaVersion
        commit = $LlamaCommit
        backend = $SelectedBackend
        runner = $RunnerRelativePath
        source = 'built_from_source'
        built_at = $builtAt
    }
    $json = $manifest | ConvertTo-Json
    Write-UTF8NoBOM $installManifest $json

    $active = Join-Path $RuntimeRoot 'active.json'
    $partial = "$active.partial"
    Write-UTF8NoBOM $partial $json
    Move-Item -LiteralPath $partial -Destination $active -Force
}

if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) {
    throw 'Managed llama.cpp source builds are currently supported on Windows only.'
}
if (-not [Environment]::Is64BitOperatingSystem) {
    throw 'Managed llama.cpp requires 64-bit Windows.'
}

$git = Resolve-Executable 'git'
if (-not $git) {
    throw 'Git is required to fetch the pinned llama.cpp source. Install Git, then retry.'
}
$cmake = Resolve-CMake
if (-not $cmake) {
    throw 'CMake is required to build llama.cpp. Install CMake or the Visual Studio CMake tools, then retry.'
}
if (-not (Test-VCToolchain)) {
    throw 'The Visual Studio Desktop C++ Build Tools workload is required to build llama.cpp. Run install.ps1 to provision it, then retry.'
}

$selectedBackend = $Backend
if ($selectedBackend -eq 'auto') {
    $selectedBackend = if ((Resolve-Executable 'nvidia-smi') -and (Resolve-Executable 'nvcc')) { 'cuda' } else { 'cpu' }
}
if ($selectedBackend -eq 'cuda' -and -not (Resolve-Executable 'nvcc')) {
    throw 'The CUDA backend requires the NVIDIA CUDA Toolkit (nvcc). Install it or choose the CPU backend.'
}

$resolvedDataDir = [System.IO.Path]::GetFullPath($DataDir)
$runtimeRoot = Join-Path $resolvedDataDir 'runtimes\llama.cpp'
$installID = "$LlamaVersion-$selectedBackend-$($LlamaCommit.Substring(0, 7))"
$installDir = Join-Path $runtimeRoot "installs\$InstallID"
$runner = Join-Path $installDir 'bin\llama-server.exe'
$runnerRelative = "installs/$installID/bin/llama-server.exe"
New-Item -ItemType Directory -Force -Path (Join-Path $runtimeRoot 'installs') | Out-Null

if (Test-Path -LiteralPath $installDir) {
    $existingMatches = $false
    if (Test-Path -LiteralPath $runner) {
        try {
            $versionOutput = Invoke-Captured $runner @('--version') 'Probe existing llama-server'
            $existingMatches = $versionOutput -match $LlamaCommit.Substring(0, 7)
        } catch {
            Write-Warning "The existing managed runtime failed validation: $_"
        }
    }
    if ($existingMatches) {
        Write-RuntimeManifest $runtimeRoot $installID $selectedBackend $runnerRelative
        Write-Host "Managed llama.cpp $LlamaVersion ($selectedBackend) is already built." -ForegroundColor Green
        return
    }
    Assert-ChildPath $runtimeRoot $installDir
    Write-Warning 'Replacing an incomplete or mismatched app-owned llama.cpp install.'
    Remove-Item -LiteralPath $installDir -Recurse -Force
}

$token = [Guid]::NewGuid().ToString('N')
$workspace = Join-Path $runtimeRoot ".build-$token"
$sourceDir = Join-Path $workspace 'source'
$buildDir = Join-Path $workspace 'build'
$installStage = Join-Path $runtimeRoot "installs\$InstallID.partial-$token"

try {
    New-Item -ItemType Directory -Force -Path $sourceDir | Out-Null
    Write-Host "Fetching pinned llama.cpp $LlamaVersion source..."
    Invoke-Checked $git @('-C', $sourceDir, 'init', '--quiet') 'Initialize llama.cpp source checkout'
    Invoke-Checked $git @('-C', $sourceDir, 'config', 'core.longpaths', 'true') 'Enable long paths for llama.cpp source'
    Invoke-Checked $git @('-C', $sourceDir, 'remote', 'add', 'origin', $LlamaRepository) 'Configure llama.cpp source remote'
    Invoke-Checked $git @('-C', $sourceDir, 'fetch', '--quiet', '--depth', '1', 'origin', 'tag', $LlamaVersion) 'Fetch pinned llama.cpp source'
    Invoke-Checked $git @('-C', $sourceDir, 'checkout', '--quiet', '--detach', $LlamaVersion) 'Check out pinned llama.cpp source'
    $actualCommit = (& $git -C $sourceDir rev-parse HEAD).Trim()
    if ($LASTEXITCODE -ne 0 -or $actualCommit -ne $LlamaCommit) {
        throw "Pinned llama.cpp source verification failed: got '$actualCommit'."
    }

    Write-Host "Configuring llama.cpp $LlamaVersion ($selectedBackend)..."
    $configure = @(
        '-S', $sourceDir,
        '-B', $buildDir,
        '-A', 'x64',
        '-DLLAMA_BUILD_SERVER=ON',
        '-DLLAMA_BUILD_TOOLS=ON',
        '-DLLAMA_BUILD_TESTS=OFF',
        '-DLLAMA_BUILD_EXAMPLES=OFF',
        '-DLLAMA_BUILD_APP=OFF',
        '-DLLAMA_BUILD_UI=OFF',
        '-DLLAMA_USE_PREBUILT_UI=OFF',
        '-DLLAMA_CURL=OFF',
        '-DLLAMA_OPENSSL=OFF',
        '-DGGML_CCACHE=OFF',
        "-DGGML_CUDA=$(if ($selectedBackend -eq 'cuda') { 'ON' } else { 'OFF' })"
    )
    Invoke-Checked $cmake $configure 'Configure llama.cpp'

    Write-Host 'Building llama-server from source. This can take several minutes...'
    Invoke-Checked $cmake @('--build', $buildDir, '--config', 'Release', '--target', 'llama-server', '--parallel') 'Build llama-server'

    $builtRunner = Join-Path $buildDir 'bin\Release\llama-server.exe'
    if (-not (Test-Path -LiteralPath $builtRunner)) {
        $builtRunner = Join-Path $buildDir 'bin\llama-server.exe'
    }
    if (-not (Test-Path -LiteralPath $builtRunner)) {
        throw 'The llama.cpp build completed without producing llama-server.exe.'
    }
    $versionOutput = Invoke-Captured $builtRunner @('--version') 'Probe built llama-server'
    if ($versionOutput -notmatch $LlamaCommit.Substring(0, 7)) {
        throw 'The built llama-server failed its pinned-version probe.'
    }

    $builtBin = Split-Path -Parent $builtRunner
    $stageBin = Join-Path $installStage 'bin'
    New-Item -ItemType Directory -Force -Path $stageBin | Out-Null
    Get-ChildItem -LiteralPath $builtBin -File | Copy-Item -Destination $stageBin -Force
    Copy-Item -LiteralPath (Join-Path $sourceDir 'LICENSE') -Destination (Join-Path $installStage 'LICENSE-llama.cpp') -Force
    Move-Item -LiteralPath $installStage -Destination $installDir
    Write-RuntimeManifest $runtimeRoot $installID $selectedBackend $runnerRelative
    Write-Host "Built and installed managed llama.cpp $LlamaVersion ($selectedBackend)." -ForegroundColor Green
} finally {
    if (Test-Path -LiteralPath $workspace) {
        Remove-Item -LiteralPath $workspace -Recurse -Force -ErrorAction SilentlyContinue
    }
    if (Test-Path -LiteralPath $installStage) {
        Remove-Item -LiteralPath $installStage -Recurse -Force -ErrorAction SilentlyContinue
    }
}
