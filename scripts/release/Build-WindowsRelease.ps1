#requires -Version 5.1
<#
.SYNOPSIS
Builds the versioned Windows payload, portable ZIP, and unsigned Inno Setup executable.

.DESCRIPTION
The script never publishes a release. Both distributable artifacts are built
from the same staged payload, which includes the pure-Go app, first-party voice
adapters, licenses, user documentation, and the optional managed voice setup
scripts used by the in-app setup flow.
#>
[CmdletBinding()]
param(
    [string]$Version = '',
    [string]$Commit = '',
    [string]$OutputRoot = '',
    [string]$ISCCPath = '',
    [switch]$SkipFrontendBuild,
    [switch]$SkipInstaller,
    [switch]$KeepStaging,
    [switch]$AllowDirty
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$repository = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
if ([string]::IsNullOrWhiteSpace($OutputRoot)) {
    $OutputRoot = Join-Path $repository 'artifacts'
}
$OutputRoot = [System.IO.Path]::GetFullPath($OutputRoot)

function Invoke-ReleaseCommand {
    param(
        [Parameter(Mandatory = $true)][string]$Executable,
        [Parameter(Mandatory = $true)][string[]]$Arguments,
        [Parameter(Mandatory = $true)][string]$Description,
        [string]$WorkingDirectory = $repository
    )
    Push-Location $WorkingDirectory
    try {
        & $Executable @Arguments
        if ($LASTEXITCODE -ne 0) {
            throw "$Description failed (exit $LASTEXITCODE)."
        }
    } finally {
        Pop-Location
    }
}

function Resolve-ReleaseExecutable {
    param([Parameter(Mandatory = $true)][string[]]$Names)
    foreach ($name in $Names) {
        $command = Get-Command $name -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($command) {
            return [string]$command.Source
        }
    }
    return $null
}

function Resolve-ISCC {
    if (-not [string]::IsNullOrWhiteSpace($ISCCPath)) {
        $resolved = [System.IO.Path]::GetFullPath($ISCCPath)
        if (-not (Test-Path -LiteralPath $resolved -PathType Leaf)) {
            throw "Inno Setup compiler not found at '$resolved'."
        }
        return $resolved
    }
    $command = Resolve-ReleaseExecutable -Names @('ISCC.exe', 'iscc')
    if ($command) {
        return $command
    }
    foreach ($candidate in @(
        (Join-Path $env:ProgramFiles 'Inno Setup 7\ISCC.exe'),
        (Join-Path ${env:ProgramFiles(x86)} 'Inno Setup 7\ISCC.exe'),
        (Join-Path $env:ProgramFiles 'Inno Setup 6\ISCC.exe'),
        (Join-Path ${env:ProgramFiles(x86)} 'Inno Setup 6\ISCC.exe'),
        (Join-Path $env:LOCALAPPDATA 'Programs\Inno Setup 7\ISCC.exe'),
        (Join-Path $env:LOCALAPPDATA 'Programs\Inno Setup 6\ISCC.exe')
    )) {
        if (Test-Path -LiteralPath $candidate -PathType Leaf) {
            return $candidate
        }
    }
    throw 'Inno Setup 6 or 7 is required. Install it or pass -ISCCPath.'
}

function Read-GitValue {
    param([Parameter(Mandatory = $true)][string[]]$Arguments)
    $git = Resolve-ReleaseExecutable -Names @('git.exe', 'git')
    if (-not $git) {
        throw 'Git is required when Version or Commit is not supplied explicitly.'
    }
    $value = @(& $git -C $repository @Arguments 2>$null) -join ''
    if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($value)) {
        throw "Git could not resolve: $($Arguments -join ' ')."
    }
    return $value.Trim()
}

function Get-ReleaseSourceState {
    $git = Resolve-ReleaseExecutable -Names @('git.exe', 'git')
    $gitMetadata = Join-Path $repository '.git'
    if (-not $git -or -not (Test-Path -LiteralPath $gitMetadata)) {
        if ($AllowDirty) {
            return 'unverified'
        }
        throw 'Git metadata is required to verify release source provenance. Use -AllowDirty only for a non-release local smoke build.'
    }
    $status = @(& $git -C $repository status --porcelain=v1 --untracked-files=all 2>$null)
    if ($LASTEXITCODE -ne 0) {
        throw 'Git could not verify the release worktree state.'
    }
    if ($status.Count -gt 0) {
        if (-not $AllowDirty) {
            throw 'The release worktree has local changes. Commit them before packaging, or use -AllowDirty for a visibly marked local smoke build.'
        }
        return 'dirty'
    }
    return 'clean'
}

function Resolve-WindowsNumericVersion {
    param([Parameter(Mandatory = $true)][string]$SemanticVersion)

    $match = [regex]::Match(
        $SemanticVersion,
        '^(?<major>0|[1-9]\d*)\.(?<minor>0|[1-9]\d*)\.(?<patch>0|[1-9]\d*)(?:-(?<stage>alpha|beta|rc)\.(?<ordinal>[1-9]\d*))?$'
    )
    if (-not $match.Success) {
        # Pull-request and local acceptance versions are deliberately not
        # represented as ordered Windows release versions.
        return '0.0.0.0'
    }

    $major = [uint32]$match.Groups['major'].Value
    $minor = [uint32]$match.Groups['minor'].Value
    $patch = [uint32]$match.Groups['patch'].Value
    foreach ($component in @($major, $minor, $patch)) {
        if ($component -gt 65535) {
            throw "Version '$SemanticVersion' exceeds the Windows 16-bit version-component limit."
        }
    }

    $build = 65535
    if ($match.Groups['stage'].Success) {
        $ordinal = [uint32]$match.Groups['ordinal'].Value
        if ($ordinal -gt 9999) {
            throw "Version '$SemanticVersion' has a prerelease ordinal above 9999."
        }
        switch ($match.Groups['stage'].Value) {
            'alpha' { $build = $ordinal }
            'beta' { $build = 10000 + $ordinal }
            'rc' { $build = 20000 + $ordinal }
        }
    }

    return '{0}.{1}.{2}.{3}' -f $major, $minor, $patch, $build
}

if ([string]::IsNullOrWhiteSpace($Commit)) {
    $Commit = Read-GitValue -Arguments @('rev-parse', 'HEAD')
}
if ($Commit -notmatch '^[0-9a-fA-F]{7,40}$') {
    throw 'Commit must be a 7-40 character hexadecimal Git revision.'
}
if ([string]::IsNullOrWhiteSpace($Version)) {
    $Version = Read-GitValue -Arguments @('describe', '--tags', '--always')
}
$Version = $Version.Trim()
if ($Version.StartsWith('v') -and $Version.Length -gt 1 -and $Version[1] -match '[0-9]') {
    $Version = $Version.Substring(1)
}
if ($Version -notmatch '^[0-9A-Za-z][0-9A-Za-z.+_-]{0,63}$') {
    throw "Version '$Version' contains unsupported characters."
}
$sourceState = Get-ReleaseSourceState
$artifactVersion = $Version -replace '[^0-9A-Za-z._-]', '-'
$numericVersion = Resolve-WindowsNumericVersion -SemanticVersion $Version

$go = Resolve-ReleaseExecutable -Names @('go.exe', 'go')
if (-not $go) {
    throw 'Go is required to build the Windows release payload.'
}
if (-not $SkipFrontendBuild) {
    $npm = Resolve-ReleaseExecutable -Names @('npm.cmd', 'npm')
    if (-not $npm) {
        throw 'Node.js and npm are required to build the embedded frontend.'
    }
    Invoke-ReleaseCommand -Executable $npm -Arguments @('ci') -Description 'Frontend dependency installation' -WorkingDirectory (Join-Path $repository 'web')
    Invoke-ReleaseCommand -Executable $npm -Arguments @('run', 'build') -Description 'Frontend production build' -WorkingDirectory (Join-Path $repository 'web')
}

New-Item -ItemType Directory -Force -Path $OutputRoot | Out-Null
$stageRoot = Join-Path $OutputRoot ".staging-$artifactVersion-$([Guid]::NewGuid().ToString('N'))"
$payloadRoot = Join-Path $stageRoot 'MagicHandy'
New-Item -ItemType Directory -Force -Path $payloadRoot | Out-Null

try {
    $previousCGO = $env:CGO_ENABLED
    $previousGOOS = $env:GOOS
    $previousGOARCH = $env:GOARCH
    $env:CGO_ENABLED = '0'
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    try {
        $ldflags = "-s -w -X main.version=$Version -X main.commit=$Commit"
        $targets = @(
            @{ Output = 'magichandy.exe'; Package = './cmd/magichandy' },
            @{ Output = 'voice-parakeet-worker.exe'; Package = './cmd/voice-parakeet-worker' },
            @{ Output = 'voice-openai-tts-worker.exe'; Package = './cmd/voice-openai-tts-worker' },
            @{ Output = 'voice-elevenlabs-worker.exe'; Package = './cmd/voice-elevenlabs-worker' }
        )
        foreach ($target in $targets) {
            $destination = Join-Path $payloadRoot $target.Output
            Invoke-ReleaseCommand -Executable $go -Arguments @('build', '-trimpath', '-ldflags', $ldflags, '-o', $destination, $target.Package) -Description "Building $($target.Output)"
            if (-not (Test-Path -LiteralPath $destination -PathType Leaf)) {
                throw "Build did not produce '$destination'."
            }
        }
    } finally {
        $env:CGO_ENABLED = $previousCGO
        $env:GOOS = $previousGOOS
        $env:GOARCH = $previousGOARCH
    }

    foreach ($file in @('LICENSE', 'README.md')) {
        Copy-Item -LiteralPath (Join-Path $repository $file) -Destination (Join-Path $payloadRoot $file)
    }
    $sourceNotice = @(
        'MagicHandy corresponding source'
        ''
        "Version: $Version"
        "Commit: $($Commit.ToLowerInvariant())"
        "Source state: $sourceState"
        'License: GPL-3.0-only'
        "Source: https://github.com/MagicHandy/MagicHandy/tree/$($Commit.ToLowerInvariant())"
        ''
        $(if ($sourceState -eq 'clean') {
            'The release manifest records every file included in this binary distribution.'
        } else {
            'LOCAL ACCEPTANCE BUILD: files differ from or could not be verified against the named commit. Do not publish.'
        })
    ) -join "`r`n"
    [System.IO.File]::WriteAllText(
        (Join-Path $payloadRoot 'SOURCE.txt'),
        $sourceNotice,
        (New-Object System.Text.UTF8Encoding($false))
    )
    Copy-Item -LiteralPath (Join-Path $repository 'docs') -Destination $payloadRoot -Recurse
    $scriptsRoot = Join-Path $payloadRoot 'scripts'
    New-Item -ItemType Directory -Force -Path $scriptsRoot | Out-Null
    foreach ($file in @(
        'install-llama-runtime.ps1',
        'install-parakeet-module.ps1',
        'install-tts-module.ps1',
        'update-tts-module.ps1'
    )) {
        Copy-Item -LiteralPath (Join-Path $repository "scripts\$file") -Destination (Join-Path $scriptsRoot $file)
    }
    Copy-Item `
        -LiteralPath (Join-Path $repository 'internal\llm\runtimeassets\build-managed-llama.ps1') `
        -Destination (Join-Path $scriptsRoot 'build-managed-llama.ps1')
    Copy-Item -LiteralPath (Join-Path $repository 'scripts\installer') -Destination $scriptsRoot -Recurse
    $ttsScriptsRoot = Join-Path $scriptsRoot 'tts'
    New-Item -ItemType Directory -Force -Path $ttsScriptsRoot | Out-Null
    Get-ChildItem -LiteralPath (Join-Path $repository 'scripts\tts') -File -Filter '*.py' | ForEach-Object {
        Copy-Item -LiteralPath $_.FullName -Destination (Join-Path $ttsScriptsRoot $_.Name)
    }

    $manifestFiles = @(Get-ChildItem -LiteralPath $payloadRoot -File -Recurse | Sort-Object FullName | ForEach-Object {
        [ordered]@{
            path = $_.FullName.Substring($payloadRoot.Length + 1).Replace('\', '/')
            size = $_.Length
            sha256 = (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        }
    })
    $manifest = [ordered]@{
        schema_version = 1
        product = 'MagicHandy'
        version = $Version
        commit = $Commit.ToLowerInvariant()
        source_state = $sourceState
        platform = 'windows-amd64'
        license = 'GPL-3.0-only'
        source_url = "https://github.com/MagicHandy/MagicHandy/tree/$($Commit.ToLowerInvariant())"
        generated_at = [DateTime]::UtcNow.ToString('o')
        files = $manifestFiles
    }
    $manifestPath = Join-Path $payloadRoot 'release-manifest.json'
    [System.IO.File]::WriteAllText($manifestPath, ($manifest | ConvertTo-Json -Depth 5), (New-Object System.Text.UTF8Encoding($false)))

    $portablePath = Join-Path $OutputRoot "MagicHandy-$artifactVersion-windows-amd64-portable.zip"
    if (Test-Path -LiteralPath $portablePath) {
        Remove-Item -LiteralPath $portablePath -Force
    }
    Compress-Archive -LiteralPath $payloadRoot -DestinationPath $portablePath -CompressionLevel Optimal

    $checksums = New-Object System.Collections.Generic.List[string]
    $portableHash = (Get-FileHash -LiteralPath $portablePath -Algorithm SHA256).Hash.ToLowerInvariant()
    $checksums.Add("$portableHash  $([System.IO.Path]::GetFileName($portablePath))")

    $setupPath = $null
    if (-not $SkipInstaller) {
        $iscc = Resolve-ISCC
        $innoScript = Join-Path $repository 'installer\magichandy.iss'
        Invoke-ReleaseCommand -Executable $iscc -Arguments @(
            "/DSourceDir=$payloadRoot",
            "/DOutputDir=$OutputRoot",
            "/DAppVersion=$Version",
            "/DArtifactVersion=$artifactVersion",
            "/DNumericVersion=$numericVersion",
            $innoScript
        ) -Description 'Inno Setup compilation'
        $setupPath = Join-Path $OutputRoot "MagicHandy-$artifactVersion-windows-amd64-setup.exe"
        if (-not (Test-Path -LiteralPath $setupPath -PathType Leaf)) {
            throw "Inno Setup did not produce '$setupPath'."
        }
        $setupHash = (Get-FileHash -LiteralPath $setupPath -Algorithm SHA256).Hash.ToLowerInvariant()
        $checksums.Add("$setupHash  $([System.IO.Path]::GetFileName($setupPath))")
    }

    $checksumPath = Join-Path $OutputRoot "MagicHandy-$artifactVersion-windows-amd64-SHA256SUMS.txt"
    [System.IO.File]::WriteAllLines($checksumPath, $checksums, (New-Object System.Text.UTF8Encoding($false)))
} finally {
    if (-not $KeepStaging -and (Test-Path -LiteralPath $stageRoot)) {
        Remove-Item -LiteralPath $stageRoot -Recurse -Force
    }
}

Write-Host "Portable ZIP: $portablePath" -ForegroundColor Green
if ($setupPath) {
    Write-Host "Setup binary: $setupPath" -ForegroundColor Green
}
Write-Host "Checksums:    $checksumPath" -ForegroundColor Green
