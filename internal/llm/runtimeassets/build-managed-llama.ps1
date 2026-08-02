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
$LlamaReleaseURL = 'https://github.com/ggml-org/llama.cpp/releases/tag/b9966'
$LlamaAssets = @{
    cpu = @(
        [pscustomobject]@{
            Name = 'llama-b9966-bin-win-cpu-x64.zip'
            URL = 'https://github.com/ggml-org/llama.cpp/releases/download/b9966/llama-b9966-bin-win-cpu-x64.zip'
            SHA256 = 'a2e791df47c8abd09e23f85a00699d6d6552445f6bba21e810263eaeefbf672a'
            Bytes = 18211851L
        }
    )
    cuda = @(
        [pscustomobject]@{
            Name = 'llama-b9966-bin-win-cuda-12.4-x64.zip'
            URL = 'https://github.com/ggml-org/llama.cpp/releases/download/b9966/llama-b9966-bin-win-cuda-12.4-x64.zip'
            SHA256 = 'bd95fbe38267b41ba109f922b978985e3ce982fef47040f90534a291617fcee9'
            Bytes = 267340684L
        },
        [pscustomobject]@{
            Name = 'cudart-llama-bin-win-cuda-12.4-x64.zip'
            URL = 'https://github.com/ggml-org/llama.cpp/releases/download/b9966/cudart-llama-bin-win-cuda-12.4-x64.zip'
            SHA256 = '8c79a9b226de4b3cacfd1f83d24f962d0773be79f1e7b75c6af4ded7e32ae1d6'
            Bytes = 391443627L
        }
    )
}

function Resolve-Executable([string]$Name) {
    $command = Get-Command $Name -ErrorAction SilentlyContinue
    if ($null -ne $command) {
        return $command.Source
    }

    if ($Name -eq 'nvidia-smi') {
        foreach ($candidate in @(
            (Join-Path $env:SystemRoot 'System32\nvidia-smi.exe'),
            (Join-Path $env:ProgramFiles 'NVIDIA Corporation\NVSMI\nvidia-smi.exe')
        )) {
            if (Test-Path -LiteralPath $candidate -PathType Leaf) {
                return $candidate
            }
        }
    }
    return $null
}

function Test-NVIDIAGPU {
    $nvidiaSMI = Resolve-Executable 'nvidia-smi'
    if (-not $nvidiaSMI) {
        return $false
    }
    try {
        & $nvidiaSMI '--query-gpu=name' '--format=csv,noheader' 2>$null | Out-Null
        return $LASTEXITCODE -eq 0
    } catch {
        return $false
    }
}

function Get-LlamaReleaseAssets([string]$SelectedBackend) {
    if ($SelectedBackend -notin @('cpu', 'cuda')) {
        throw "Unsupported managed llama.cpp backend '$SelectedBackend'."
    }
    return @($LlamaAssets[$SelectedBackend])
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

function Get-SHA256([string]$Path) {
    return (Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash.ToLowerInvariant()
}

function Test-VerifiedFile([string]$Path, [string]$ExpectedSHA256, [long]$ExpectedBytes) {
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        return $false
    }
    try {
        $file = Get-Item -LiteralPath $Path
        return $file.Length -eq $ExpectedBytes -and (Get-SHA256 $Path) -eq $ExpectedSHA256
    } catch {
        return $false
    }
}

function Format-ByteCount([long]$Bytes) {
    if ($Bytes -ge 1GB) { return ('{0:N2} GiB' -f ($Bytes / 1GB)) }
    if ($Bytes -ge 1MB) { return ('{0:N1} MiB' -f ($Bytes / 1MB)) }
    if ($Bytes -ge 1KB) { return ('{0:N1} KiB' -f ($Bytes / 1KB)) }
    return "$Bytes B"
}

function Install-VerifiedDownload(
    [string]$URL,
    [string]$Destination,
    [string]$ExpectedSHA256,
    [long]$ExpectedBytes
) {
    $downloadURI = $null
    if (-not [Uri]::TryCreate($URL, [UriKind]::Absolute, [ref]$downloadURI) -or $downloadURI.Scheme -ne 'https') {
        throw 'Managed llama.cpp assets must use an absolute HTTPS URL.'
    }
    if ($ExpectedSHA256 -notmatch '^[0-9a-f]{64}$' -or $ExpectedBytes -le 0) {
        throw 'Managed llama.cpp asset metadata is invalid.'
    }

    $parent = Split-Path -Parent $Destination
    New-Item -ItemType Directory -Force -Path $parent | Out-Null
    if (Test-VerifiedFile $Destination $ExpectedSHA256 $ExpectedBytes) {
        Write-Host "Verified cached $(Split-Path -Leaf $Destination)." -ForegroundColor Green
        return
    }
    if (Test-Path -LiteralPath $Destination) {
        Remove-Item -LiteralPath $Destination -Force
    }

    $partial = "$Destination.partial"
    $name = Split-Path -Leaf $Destination
    [Net.ServicePointManager]::SecurityProtocol =
        [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    $lastFailure = ''

    for ($attempt = 1; $attempt -le 6; $attempt++) {
        $offset = if (Test-Path -LiteralPath $partial -PathType Leaf) {
            (Get-Item -LiteralPath $partial).Length
        } else {
            0L
        }
        if ($offset -eq $ExpectedBytes) {
            if (Test-VerifiedFile $partial $ExpectedSHA256 $ExpectedBytes) {
                Move-Item -LiteralPath $partial -Destination $Destination -Force
                Write-Host "Verified completed partial $name." -ForegroundColor Green
                return
            }
            Remove-Item -LiteralPath $partial -Force
            $offset = 0L
        }
        if ($offset -gt $ExpectedBytes) {
            Remove-Item -LiteralPath $partial -Force
            $offset = 0L
        }

        if ($offset -gt 0) {
            Write-Host "Resuming $name from $(Format-ByteCount $offset)..."
        } else {
            Write-Host "Downloading $name ($(Format-ByteCount $ExpectedBytes))..."
        }

        $response = $null
        $input = $null
        $output = $null
        try {
            $request = [System.Net.HttpWebRequest][System.Net.WebRequest]::Create($downloadURI)
            $request.Method = 'GET'
            $request.UserAgent = 'MagicHandy-Installer/1.0'
            $request.AllowAutoRedirect = $true
            $request.MaximumAutomaticRedirections = 10
            $request.AutomaticDecompression = [System.Net.DecompressionMethods]::None
            $request.Headers['Accept-Encoding'] = 'identity'
            $request.Timeout = 60000
            $request.ReadWriteTimeout = 60000
            $request.CachePolicy = [System.Net.Cache.RequestCachePolicy]::new(
                [System.Net.Cache.RequestCacheLevel]::NoCacheNoStore
            )
            if ($offset -gt 0) {
                $request.AddRange([long]$offset)
            }

            $response = [System.Net.HttpWebResponse]$request.GetResponse()
            if ($response.ResponseUri.Scheme -ne 'https') {
                throw 'Managed llama.cpp download refused an HTTPS-to-HTTP redirect.'
            }
            if (-not [string]::IsNullOrWhiteSpace($response.ContentEncoding) -and $response.ContentEncoding -ne 'identity') {
                throw "Download server returned unsupported content encoding '$($response.ContentEncoding)'."
            }

            $append = $false
            if ([int]$response.StatusCode -eq 206) {
                $contentRange = $response.Headers['Content-Range']
                if ($contentRange -notmatch '^bytes\s+(\d+)-(\d+)/(\d+)$' -or
                    [long]$Matches[1] -ne $offset -or [long]$Matches[3] -ne $ExpectedBytes) {
                    throw "Download server returned an unexpected byte range: '$contentRange'."
                }
                $append = $offset -gt 0
            } elseif ([int]$response.StatusCode -eq 200) {
                if ($offset -gt 0) {
                    Write-Warning 'The download server did not resume the saved partial; restarting this asset from zero.'
                    $offset = 0L
                }
            } else {
                throw "Download server returned HTTP $([int]$response.StatusCode)."
            }

            $mode = if ($append) { [System.IO.FileMode]::OpenOrCreate } else { [System.IO.FileMode]::Create }
            $output = [System.IO.File]::Open(
                $partial,
                $mode,
                [System.IO.FileAccess]::Write,
                [System.IO.FileShare]::None
            )
            if ($append) {
                if ($output.Length -ne $offset) {
                    throw 'Download partial changed while it was being resumed.'
                }
                $output.Position = $offset
            }

            $input = $response.GetResponseStream()
            $buffer = New-Object byte[] (1MB)
            $nextProgress = [DateTime]::UtcNow.AddSeconds(5)
            while ($true) {
                $count = $input.Read($buffer, 0, $buffer.Length)
                if ($count -eq 0) {
                    break
                }
                $output.Write($buffer, 0, $count)
                if ($output.Position -gt $ExpectedBytes) {
                    throw 'Downloaded bytes exceeded the pinned asset size.'
                }
                if ([DateTime]::UtcNow -ge $nextProgress) {
                    Write-Host "Downloaded $(Format-ByteCount $output.Position) / $(Format-ByteCount $ExpectedBytes) of $name..."
                    $nextProgress = [DateTime]::UtcNow.AddSeconds(5)
                }
            }
            $output.Flush()
            if ($output.Length -ne $ExpectedBytes) {
                throw "Download ended at $(Format-ByteCount $output.Length); expected $(Format-ByteCount $ExpectedBytes)."
            }
            $lastFailure = ''
        } catch {
            $lastFailure = $_.Exception.Message
        } finally {
            if ($null -ne $input) { $input.Dispose() }
            if ($null -ne $output) { $output.Dispose() }
            if ($null -ne $response) { $response.Dispose() }
        }

        if ([string]::IsNullOrWhiteSpace($lastFailure)) {
            Write-Host "Verifying $name SHA-256..."
            if (-not (Test-VerifiedFile $partial $ExpectedSHA256 $ExpectedBytes)) {
                Remove-Item -LiteralPath $partial -Force -ErrorAction SilentlyContinue
                throw "SHA-256 verification failed for $name. The untrusted download was removed."
            }
            Move-Item -LiteralPath $partial -Destination $Destination -Force
            Write-Host "Verified $name." -ForegroundColor Green
            return
        }

        if ($attempt -eq 6) {
            throw "Downloading $name failed after 6 attempts: $lastFailure. Partial data was retained for retry."
        }
        Write-Warning "Downloading $name was interrupted: $lastFailure. Retrying ($($attempt + 1)/6)..."
        Start-Sleep -Seconds ([Math]::Min(20, [Math]::Pow(2, $attempt - 1)))
    }
}

function Expand-VerifiedZip([string]$Archive, [string]$Destination) {
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    New-Item -ItemType Directory -Force -Path $Destination | Out-Null
    $destinationPrefix = [System.IO.Path]::GetFullPath($Destination).TrimEnd('\') + '\'
    $seen = New-Object 'System.Collections.Generic.HashSet[string]' ([StringComparer]::OrdinalIgnoreCase)
    $zip = [System.IO.Compression.ZipFile]::OpenRead($Archive)
    try {
        foreach ($entry in $zip.Entries) {
            $relative = $entry.FullName.Replace('/', '\')
            if ([string]::IsNullOrWhiteSpace($relative)) {
                throw 'Verified llama.cpp archive contains an empty path.'
            }
            $segments = @($relative.Split('\') | Where-Object { $_ -ne '' })
            if ([System.IO.Path]::IsPathRooted($relative) -or $relative.Contains(':') -or
                $segments -contains '..' -or $segments -contains '.') {
                throw "Verified llama.cpp archive contains an unsafe path: '$($entry.FullName)'."
            }
            $target = [System.IO.Path]::GetFullPath((Join-Path $Destination $relative))
            if (-not $target.StartsWith($destinationPrefix, [StringComparison]::OrdinalIgnoreCase)) {
                throw "Verified llama.cpp archive path escapes its staging directory: '$($entry.FullName)'."
            }
            $unixType = (($entry.ExternalAttributes -shr 16) -band 0xF000)
            if ($unixType -eq 0xA000) {
                throw "Verified llama.cpp archive contains a symbolic link: '$($entry.FullName)'."
            }
            if (-not $entry.FullName.EndsWith('/') -and -not $seen.Add($target)) {
                throw "Verified llama.cpp archive contains a duplicate path: '$($entry.FullName)'."
            }
        }
    } finally {
        $zip.Dispose()
    }
    [System.IO.Compression.ZipFile]::ExtractToDirectory($Archive, $Destination)
}

function Copy-ArchiveFiles([string]$Source, [string]$Destination) {
    foreach ($file in (Get-ChildItem -LiteralPath $Source -File -Recurse)) {
        $target = Join-Path $Destination $file.Name
        if (Test-Path -LiteralPath $target -PathType Leaf) {
            if ((Get-SHA256 $target) -ne (Get-SHA256 $file.FullName)) {
                throw "Verified llama.cpp assets contain conflicting files named '$($file.Name)'."
            }
            continue
        }
        Copy-Item -LiteralPath $file.FullName -Destination $target
    }
}

function Test-ExistingRuntime(
    [string]$InstallManifest,
    [string]$Runner,
    [string]$SelectedBackend,
    [string]$RunnerRelativePath
) {
    if (-not (Test-Path -LiteralPath $InstallManifest -PathType Leaf) -or
        -not (Test-Path -LiteralPath $Runner -PathType Leaf)) {
        return $false
    }
    try {
        $manifest = Get-Content -LiteralPath $InstallManifest -Raw | ConvertFrom-Json
        if ($manifest.schema_version -ne 1 -or $manifest.runtime -ne 'llama.cpp' -or
            $manifest.version -ne $LlamaVersion -or $manifest.commit -ne $LlamaCommit -or
            $manifest.backend -ne $SelectedBackend -or $manifest.runner -ne $RunnerRelativePath -or
            $manifest.source -notin @('built_from_source', 'verified_upstream_release')) {
            return $false
        }
        [DateTimeOffset]::Parse([string]$manifest.built_at) | Out-Null
        $versionOutput = Invoke-Captured $Runner @('--version') 'Probe existing llama-server'
        return $versionOutput -match $LlamaCommit.Substring(0, 7)
    } catch {
        Write-Warning "The existing managed runtime failed validation: $_"
        return $false
    }
}

function Activate-ExistingManifest([string]$InstallManifest, [string]$RuntimeRoot) {
    $json = Get-Content -LiteralPath $InstallManifest -Raw
    $active = Join-Path $RuntimeRoot 'active.json'
    $partial = "$active.partial"
    Write-UTF8NoBOM $partial $json
    Move-Item -LiteralPath $partial -Destination $active -Force
}

function Write-RuntimeMetadata(
    [string]$RuntimeRoot,
    [string]$InstallID,
    [string]$SelectedBackend,
    [string]$RunnerRelativePath,
    [object[]]$Assets
) {
    $installedAt = [DateTimeOffset]::UtcNow.ToString('o')
    $installManifest = Join-Path $RuntimeRoot "installs\$InstallID\runtime.json"
    $manifest = [ordered]@{
        schema_version = 1
        runtime = 'llama.cpp'
        version = $LlamaVersion
        commit = $LlamaCommit
        backend = $SelectedBackend
        runner = $RunnerRelativePath
        source = 'verified_upstream_release'
        built_at = $installedAt
    }
    $json = $manifest | ConvertTo-Json
    Write-UTF8NoBOM $installManifest $json

    $provenance = [ordered]@{
        schema_version = 1
        runtime = 'llama.cpp'
        version = $LlamaVersion
        commit = $LlamaCommit
        backend = $SelectedBackend
        release = $LlamaReleaseURL
        verified_at = $installedAt
        assets = @($Assets | ForEach-Object {
            [ordered]@{
                name = $_.Name
                url = $_.URL
                sha256 = $_.SHA256
                bytes = $_.Bytes
            }
        })
    }
    Write-UTF8NoBOM (Join-Path $RuntimeRoot "installs\$InstallID\provenance.json") ($provenance | ConvertTo-Json -Depth 5)

    $active = Join-Path $RuntimeRoot 'active.json'
    $partial = "$active.partial"
    Write-UTF8NoBOM $partial $json
    Move-Item -LiteralPath $partial -Destination $active -Force
}

if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) {
    throw 'Managed llama.cpp runtime installation is currently supported on Windows only.'
}
if (-not [Environment]::Is64BitOperatingSystem) {
    throw 'Managed llama.cpp requires 64-bit Windows.'
}

$selectedBackend = $Backend
$hasNVIDIA = Test-NVIDIAGPU
if ($selectedBackend -eq 'auto') {
    $selectedBackend = if ($hasNVIDIA) { 'cuda' } else { 'cpu' }
}
if ($selectedBackend -eq 'cuda' -and -not $hasNVIDIA) {
    throw 'The CUDA backend requires a working NVIDIA driver and GPU. Choose the CPU backend on this machine.'
}
$assets = @(Get-LlamaReleaseAssets $selectedBackend)

$resolvedDataDir = [System.IO.Path]::GetFullPath($DataDir)
$runtimeRoot = Join-Path $resolvedDataDir 'runtimes\llama.cpp'
$installID = "$LlamaVersion-$selectedBackend-$($LlamaCommit.Substring(0, 7))"
$installDir = Join-Path $runtimeRoot "installs\$installID"
$installManifest = Join-Path $installDir 'runtime.json'
$runner = Join-Path $installDir 'bin\llama-server.exe'
$runnerRelative = "installs/$installID/bin/llama-server.exe"
$downloadDir = Join-Path $runtimeRoot 'downloads'
$license = Join-Path $PSScriptRoot 'LICENSE-llama.cpp'
New-Item -ItemType Directory -Force -Path (Join-Path $runtimeRoot 'installs') | Out-Null
New-Item -ItemType Directory -Force -Path $downloadDir | Out-Null

if (-not (Test-Path -LiteralPath $license -PathType Leaf)) {
    throw 'The pinned llama.cpp MIT license file is missing. Repair MagicHandy and retry.'
}

$lockPath = Join-Path $runtimeRoot '.install.lock'
$lock = $null
try {
    try {
        $lock = [System.IO.File]::Open(
            $lockPath,
            [System.IO.FileMode]::OpenOrCreate,
            [System.IO.FileAccess]::ReadWrite,
            [System.IO.FileShare]::None
        )
    } catch [System.IO.IOException] {
        throw 'Another managed llama.cpp installation is already running.'
    }

    if (Test-ExistingRuntime $installManifest $runner $selectedBackend $runnerRelative) {
        Activate-ExistingManifest $installManifest $runtimeRoot
        Write-Host "Managed llama.cpp $LlamaVersion ($selectedBackend) is already installed." -ForegroundColor Green
        return
    }
    if (Test-Path -LiteralPath $installDir) {
        Assert-ChildPath $runtimeRoot $installDir
        Write-Warning 'Replacing an incomplete or mismatched app-owned llama.cpp install.'
        Remove-Item -LiteralPath $installDir -Recurse -Force
    }

    foreach ($asset in $assets) {
        Install-VerifiedDownload `
            -URL $asset.URL `
            -Destination (Join-Path $downloadDir $asset.Name) `
            -ExpectedSHA256 $asset.SHA256 `
            -ExpectedBytes $asset.Bytes
    }

    $token = [Guid]::NewGuid().ToString('N')
    $workspace = Join-Path $runtimeRoot ".extract-$token"
    $installStage = Join-Path $runtimeRoot "installs\$installID.partial-$token"
    foreach ($path in @($workspace, $installStage)) {
        Assert-ChildPath $runtimeRoot $path
    }
    try {
        $stageBin = Join-Path $installStage 'bin'
        New-Item -ItemType Directory -Force -Path $stageBin | Out-Null
        for ($index = 0; $index -lt $assets.Count; $index++) {
            $asset = $assets[$index]
            $extractDir = Join-Path $workspace "asset-$index"
            Write-Host "Extracting $($asset.Name)..."
            Expand-VerifiedZip (Join-Path $downloadDir $asset.Name) $extractDir
            Copy-ArchiveFiles $extractDir $stageBin
        }

        $stagedRunner = Join-Path $stageBin 'llama-server.exe'
        if (-not (Test-Path -LiteralPath $stagedRunner -PathType Leaf)) {
            throw 'The verified llama.cpp release did not contain llama-server.exe.'
        }
        Copy-Item -LiteralPath $license -Destination (Join-Path $installStage 'LICENSE-llama.cpp')

        $savedPath = $env:Path
        try {
            $env:Path = "$stageBin;$env:SystemRoot\System32;$env:SystemRoot"
            $versionOutput = Invoke-Captured $stagedRunner @('--version') 'Probe verified llama-server'
            if ($versionOutput -notmatch $LlamaCommit.Substring(0, 7)) {
                throw 'The verified llama-server failed its pinned-version probe.'
            }
            if ($selectedBackend -eq 'cuda') {
                $devices = Invoke-Captured $stagedRunner @('--list-devices') 'Probe verified CUDA llama-server'
                if ($devices -notmatch '(?im)^\s*CUDA\d*:') {
                    throw 'The verified CUDA llama-server could not detect an NVIDIA CUDA device. Update the NVIDIA driver or choose CPU.'
                }
            }
        } finally {
            $env:Path = $savedPath
        }

        Move-Item -LiteralPath $installStage -Destination $installDir
        Write-RuntimeMetadata $runtimeRoot $installID $selectedBackend $runnerRelative $assets
        Write-Host "Installed verified managed llama.cpp $LlamaVersion ($selectedBackend)." -ForegroundColor Green

        foreach ($asset in $assets) {
            Remove-Item -LiteralPath (Join-Path $downloadDir $asset.Name) -Force -ErrorAction SilentlyContinue
            Remove-Item -LiteralPath (Join-Path $downloadDir "$($asset.Name).partial") -Force -ErrorAction SilentlyContinue
        }
    } finally {
        if (Test-Path -LiteralPath $workspace) {
            Remove-Item -LiteralPath $workspace -Recurse -Force -ErrorAction SilentlyContinue
        }
        if (Test-Path -LiteralPath $installStage) {
            Remove-Item -LiteralPath $installStage -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
} finally {
    if ($null -ne $lock) {
        $lock.Dispose()
    }
    Remove-Item -LiteralPath $lockPath -Force -ErrorAction SilentlyContinue
}
