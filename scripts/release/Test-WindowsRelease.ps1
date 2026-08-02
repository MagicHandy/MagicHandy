#requires -Version 5.1
<#
.SYNOPSIS
Verifies a built MagicHandy Windows payload and its install lifecycle.

.DESCRIPTION
Portable payload and checksum checks are always read-only. -ExerciseInstaller
uses an isolated current-user install directory. -ExerciseDefaultInstall also
tests the real Program Files default and clean user-data removal, but refuses to
run when an existing packaged install or default MagicHandy data directory is
present.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$Version,
    [Parameter(Mandatory = $true)][string]$Commit,
    [string]$ArtifactsRoot = '',
    [string]$RepositoryRoot = '',
    [ValidateSet('clean', 'dirty', 'unverified')][string]$ExpectedSourceState = 'clean',
    [switch]$ExerciseInstaller,
    [switch]$ExerciseDefaultInstall
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if ([string]::IsNullOrWhiteSpace($RepositoryRoot)) {
    $RepositoryRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
} else {
    $RepositoryRoot = [System.IO.Path]::GetFullPath($RepositoryRoot)
}
if ([string]::IsNullOrWhiteSpace($ArtifactsRoot)) {
    $ArtifactsRoot = Join-Path $RepositoryRoot 'artifacts'
}
$ArtifactsRoot = [System.IO.Path]::GetFullPath($ArtifactsRoot)
$Commit = $Commit.Trim().ToLowerInvariant()
$artifactVersion = $Version -replace '[^0-9A-Za-z._-]', '-'
$appID = '{A9859C5A-AD69-4D2E-91DA-809D109984DA}_is1'

function Assert-Release {
    param(
        [Parameter(Mandatory = $true)][bool]$Condition,
        [Parameter(Mandatory = $true)][string]$Message
    )
    if (-not $Condition) {
        throw "Release acceptance failed: $Message"
    }
}

function Invoke-ReleaseProcess {
    param(
        [Parameter(Mandatory = $true)][string]$FilePath,
        [Parameter(Mandatory = $true)][string[]]$ArgumentList,
        [Parameter(Mandatory = $true)][string]$Description
    )
    $process = Start-Process -FilePath $FilePath -ArgumentList $ArgumentList -Wait -PassThru
    if ($process.ExitCode -ne 0) {
        throw "$Description failed with exit $($process.ExitCode)."
    }
}

function Assert-Version {
    param([Parameter(Mandatory = $true)][string]$Executable)
    $reported = @(& $Executable -version 2>&1) -join "`n"
    if ($LASTEXITCODE -ne 0) {
        throw "Version probe failed for '$Executable'."
    }
    $expected = "magichandy $Version ($Commit)"
    Assert-Release -Condition ($reported.Trim() -eq $expected) -Message "version output '$($reported.Trim())' should equal '$expected'"
}

function Wait-MagicHandyReady {
    param([Parameter(Mandatory = $true)][int]$Port)
    for ($attempt = 0; $attempt -lt 100; $attempt++) {
        try {
            Invoke-RestMethod -Uri "http://127.0.0.1:$Port/healthz" -TimeoutSec 1 | Out-Null
            return
        } catch {
            Start-Sleep -Milliseconds 125
        }
    }
    throw "MagicHandy did not become ready on port $Port."
}

function Get-AvailableLoopbackPort {
    $listener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)
    try {
        $listener.Start()
        return ([Net.IPEndPoint]$listener.LocalEndpoint).Port
    } finally {
        $listener.Stop()
    }
}

function Get-Uninstaller {
    param([Parameter(Mandatory = $true)][string]$InstallDirectory)
    $uninstallers = @(Get-ChildItem -LiteralPath $InstallDirectory -Filter 'unins*.exe' -File)
    Assert-Release -Condition ($uninstallers.Count -eq 1) -Message "one uninstaller should exist in '$InstallDirectory'"
    return $uninstallers[0].FullName
}

function Assert-UninstallEntry {
    param(
        [Parameter(Mandatory = $true)][string]$RegistryPath,
        [Parameter(Mandatory = $true)][string]$InstallDirectory
    )
    Assert-Release -Condition (Test-Path -LiteralPath $RegistryPath) -Message "Add/Remove Programs entry '$RegistryPath' should exist"
    $entry = Get-ItemProperty -LiteralPath $RegistryPath
    Assert-Release -Condition ([string]$entry.DisplayName -eq 'MagicHandy') -Message 'Add/Remove Programs display name should be MagicHandy'
    Assert-Release -Condition ([string]$entry.DisplayVersion -eq $Version) -Message 'Add/Remove Programs version should match the package'
    Assert-Release -Condition ([System.IO.Path]::GetFullPath([string]$entry.InstallLocation).TrimEnd('\') -eq $InstallDirectory.TrimEnd('\')) -Message 'Add/Remove Programs install location should match the selected directory'
    Assert-Release -Condition (-not [string]::IsNullOrWhiteSpace([string]$entry.UninstallString)) -Message 'Add/Remove Programs should expose an uninstaller command'
}

function Get-ExpectedWindowsNumericVersion {
    $match = [regex]::Match(
        $Version,
        '^(?<major>0|[1-9]\d*)\.(?<minor>0|[1-9]\d*)\.(?<patch>0|[1-9]\d*)(?:-(?<stage>alpha|beta|rc)\.(?<ordinal>[1-9]\d*))?$'
    )
    if (-not $match.Success) {
        return '0.0.0.0'
    }
    $build = 65535
    if ($match.Groups['stage'].Success) {
        $ordinal = [uint32]$match.Groups['ordinal'].Value
        switch ($match.Groups['stage'].Value) {
            'alpha' { $build = $ordinal }
            'beta' { $build = 10000 + $ordinal }
            'rc' { $build = 20000 + $ordinal }
        }
    }
    return '{0}.{1}.{2}.{3}' -f `
        $match.Groups['major'].Value, `
        $match.Groups['minor'].Value, `
        $match.Groups['patch'].Value, `
        $build
}

Assert-Release -Condition (Test-Path -LiteralPath $ArtifactsRoot -PathType Container) -Message "artifact directory '$ArtifactsRoot' should exist"
$portablePath = Join-Path $ArtifactsRoot "MagicHandy-$artifactVersion-windows-amd64-portable.zip"
$setupPath = Join-Path $ArtifactsRoot "MagicHandy-$artifactVersion-windows-amd64-setup.exe"
$checksumPath = Join-Path $ArtifactsRoot "MagicHandy-$artifactVersion-windows-amd64-SHA256SUMS.txt"
foreach ($path in @($portablePath, $setupPath, $checksumPath)) {
    Assert-Release -Condition (Test-Path -LiteralPath $path -PathType Leaf) -Message "expected artifact '$path' should exist"
}
$releaseFiles = @(Get-ChildItem -LiteralPath $ArtifactsRoot -File | Where-Object { $_.Name -like "MagicHandy-$artifactVersion-windows-amd64-*" })
Assert-Release -Condition ($releaseFiles.Count -eq 3) -Message 'release output should contain exactly setup, portable, and checksum artifacts for this version'
$setupVersion = (Get-Item -LiteralPath $setupPath).VersionInfo
$expectedNumericVersion = Get-ExpectedWindowsNumericVersion
Assert-Release -Condition ($setupVersion.FileVersion.Trim() -eq $expectedNumericVersion) -Message "setup file version should be $expectedNumericVersion"
Assert-Release -Condition ($setupVersion.ProductVersion.Trim() -eq $expectedNumericVersion) -Message "setup product version should be $expectedNumericVersion"
$signature = Get-AuthenticodeSignature -LiteralPath $setupPath
Assert-Release -Condition ($signature.Status -eq [System.Management.Automation.SignatureStatus]::NotSigned) -Message 'setup should be explicitly unsigned until the signing process exists'

$testRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("magichandy-release-test-" + [Guid]::NewGuid().ToString('N'))
$app = $null
$customInstall = ''
$defaultInstall = ''
$ownsCustomInstall = $false
$ownsDefaultInstall = $false
New-Item -ItemType Directory -Force -Path $testRoot | Out-Null
try {
    Write-Host 'Verifying portable payload and provenance...'
    $expanded = Join-Path $testRoot 'portable'
    Expand-Archive -LiteralPath $portablePath -DestinationPath $expanded
    $executables = @(Get-ChildItem -LiteralPath $expanded -Filter 'magichandy.exe' -File -Recurse)
    Assert-Release -Condition ($executables.Count -eq 1) -Message 'portable archive should contain one core executable'
    $portableRoot = $executables[0].Directory.FullName
    Assert-Version -Executable $executables[0].FullName

    foreach ($required in @(
        'scripts\install-tts-module.ps1',
        'scripts\update-tts-module.ps1',
        'scripts\install-llama-runtime.ps1',
        'scripts\build-managed-llama.ps1',
        'scripts\LICENSE-llama.cpp',
        'scripts\install-parakeet-module.ps1',
        'scripts\tts\faster-qwen-server.py',
        'scripts\tts\faster-qwen-constraints.txt',
        'scripts\tts\chatterbox-server.py',
        'scripts\tts\chatterbox-constraints.txt',
        'SOURCE.txt',
        'docs\update-checks.md',
        'docs\versioning-and-releases.md',
        'release-manifest.json'
    )) {
        Assert-Release -Condition (Test-Path -LiteralPath (Join-Path $portableRoot $required) -PathType Leaf) -Message "portable payload should contain $required"
    }

    $manifest = Get-Content -LiteralPath (Join-Path $portableRoot 'release-manifest.json') -Raw | ConvertFrom-Json
    Assert-Release -Condition ([string]$manifest.product -eq 'MagicHandy') -Message 'manifest product should be MagicHandy'
    Assert-Release -Condition ([string]$manifest.version -eq $Version) -Message 'manifest version should match the package'
    Assert-Release -Condition ([string]$manifest.commit -eq $Commit) -Message 'manifest commit should match the package'
    Assert-Release -Condition ([string]$manifest.source_state -eq $ExpectedSourceState) -Message "manifest source state should be $ExpectedSourceState"
    Assert-Release -Condition ([string]$manifest.license -eq 'GPL-3.0-only') -Message 'manifest should identify the project license'
    foreach ($file in $manifest.files) {
        $path = Join-Path $portableRoot ([string]$file.path).Replace('/', '\')
        Assert-Release -Condition (Test-Path -LiteralPath $path -PathType Leaf) -Message "manifest file '$($file.path)' should exist"
        $actual = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
        Assert-Release -Condition ($actual -eq [string]$file.sha256) -Message "manifest hash for '$($file.path)' should match"
    }

    Write-Host 'Verifying outer artifact checksums...'
    $checksumLines = @(Get-Content -LiteralPath $checksumPath | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
    Assert-Release -Condition ($checksumLines.Count -eq 2) -Message 'checksum file should cover the setup EXE and portable ZIP'
    foreach ($line in $checksumLines) {
        Assert-Release -Condition ($line -match '^([0-9a-f]{64})  (.+)$') -Message "checksum line '$line' should use SHA256SUMS format"
        $artifact = Join-Path $ArtifactsRoot $Matches[2]
        Assert-Release -Condition (Test-Path -LiteralPath $artifact -PathType Leaf) -Message "checksummed artifact '$($Matches[2])' should exist"
        $actual = (Get-FileHash -LiteralPath $artifact -Algorithm SHA256).Hash.ToLowerInvariant()
        Assert-Release -Condition ($actual -eq $Matches[1]) -Message "outer checksum for '$($Matches[2])' should match"
    }

    if (-not $ExerciseInstaller) {
        Write-Host 'Windows release payload verification passed.' -ForegroundColor Green
        return
    }

    Write-Host 'Exercising custom install path, shortcuts, upgrade, and retained-data uninstall...'
    $customInstall = Join-Path $testRoot 'custom install'
    $customData = Join-Path $testRoot 'upgrade data'
    $userDesktopShortcut = Join-Path ([Environment]::GetFolderPath('Desktop')) 'MagicHandy.lnk'
    $userStartShortcut = Join-Path ([Environment]::GetFolderPath('Programs')) 'MagicHandy\MagicHandy.lnk'
    $currentUserUninstall = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\$appID"
    foreach ($path in @($userDesktopShortcut, $userStartShortcut, $currentUserUninstall)) {
        Assert-Release -Condition (-not (Test-Path -LiteralPath $path)) -Message "installer smoke requires no pre-existing MagicHandy entry at '$path'"
    }
    $ownsCustomInstall = $true
    Invoke-ReleaseProcess -FilePath $setupPath -Description 'Current-user custom-path setup' -ArgumentList @(
        '/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', '/CURRENTUSER', '/TASKS="desktopicon"', ('/DIR="{0}"' -f $customInstall)
    )
    $customExecutable = Join-Path $customInstall 'magichandy.exe'
    Assert-Release -Condition (Test-Path -LiteralPath $customExecutable -PathType Leaf) -Message 'custom path should contain the app executable'
    Assert-Version -Executable $customExecutable
    Assert-Release -Condition (Test-Path -LiteralPath $userDesktopShortcut -PathType Leaf) -Message 'selected desktop shortcut should be created'
    Assert-Release -Condition (Test-Path -LiteralPath $userStartShortcut -PathType Leaf) -Message 'Start Menu shortcut should be created'
    Assert-UninstallEntry -RegistryPath $currentUserUninstall -InstallDirectory $customInstall

    & $customExecutable -data-dir $customData -set-ui-locale ja -set-chat-locale ja -complete-setup
    if ($LASTEXITCODE -ne 0) {
        throw 'Could not seed upgrade-persistence settings.'
    }
    $port = Get-AvailableLoopbackPort
    $quotedCustomData = '"{0}"' -f $customData
    $app = Start-Process -FilePath $customExecutable -ArgumentList @('-addr', "127.0.0.1:$port", '-data-dir', $quotedCustomData) -PassThru -WindowStyle Hidden
    try {
        Wait-MagicHandyReady -Port $port
        Invoke-ReleaseProcess -FilePath $setupPath -Description 'Active-process over-install' -ArgumentList @(
            '/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', '/CURRENTUSER', '/CLOSEAPPLICATIONS', '/TASKS="desktopicon"', ('/DIR="{0}"' -f $customInstall)
        )
        Assert-Release -Condition ($app.WaitForExit(15000)) -Message 'over-install should close the running app'
        $app = Start-Process -FilePath $customExecutable -ArgumentList @('-addr', "127.0.0.1:$port", '-data-dir', $quotedCustomData) -PassThru -WindowStyle Hidden
        Wait-MagicHandyReady -Port $port
        $state = Invoke-RestMethod -Uri "http://127.0.0.1:$port/api/state" -TimeoutSec 5
        Assert-Release -Condition ([string]$state.settings.ui.locale -eq 'ja' -and [bool]$state.settings.ui.setup_completed) -Message 'over-install should preserve app-owned settings'
        $customUninstaller = Get-Uninstaller -InstallDirectory $customInstall
        Invoke-ReleaseProcess -FilePath $customUninstaller -Description 'Keep-data uninstall' -ArgumentList @(
            '/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', '/KEEPUSERDATA'
        )
        Assert-Release -Condition ($app.WaitForExit(30000)) -Message 'uninstall should gracefully stop the running custom-path app'
    } finally {
        if ($null -ne $app -and -not $app.HasExited) {
            Stop-Process -Id $app.Id -Force
            $app.WaitForExit()
        }
    }
    Assert-Release -Condition (-not (Test-Path -LiteralPath $customInstall)) -Message 'uninstall should remove the selected install directory'
    Assert-Release -Condition (Test-Path -LiteralPath (Join-Path $customData 'magichandy.db') -PathType Leaf) -Message 'explicit keep-data uninstall should preserve app data'
    Assert-Release -Condition (-not (Test-Path -LiteralPath $userDesktopShortcut)) -Message 'uninstall should remove the desktop shortcut'
    Assert-Release -Condition (-not (Test-Path -LiteralPath $userStartShortcut)) -Message 'uninstall should remove the Start Menu shortcut'
    Assert-Release -Condition (-not (Test-Path -LiteralPath $currentUserUninstall)) -Message 'uninstall should remove the current-user Add/Remove Programs entry'
    $ownsCustomInstall = $false

    if (-not $ExerciseDefaultInstall) {
        Write-Host 'Windows installer lifecycle verification passed.' -ForegroundColor Green
        return
    }

    Write-Host 'Exercising Program Files default, clean uninstall, and clean reinstall...'
    $defaultInstall = Join-Path $env:ProgramFiles 'MagicHandy'
    $defaultData = Join-Path ([Environment]::GetFolderPath('ApplicationData')) 'MagicHandy'
    $commonDesktopShortcut = Join-Path ([Environment]::GetFolderPath('CommonDesktopDirectory')) 'MagicHandy.lnk'
    $commonStartShortcut = Join-Path ([Environment]::GetFolderPath('CommonPrograms')) 'MagicHandy\MagicHandy.lnk'
    $machineUninstall = "HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\$appID"
    foreach ($path in @($defaultInstall, $defaultData, $commonDesktopShortcut, $commonStartShortcut, $machineUninstall)) {
        Assert-Release -Condition (-not (Test-Path -LiteralPath $path)) -Message "clean-install smoke refuses to overwrite existing state at '$path'"
    }

    $ownsDefaultInstall = $true
    Invoke-ReleaseProcess -FilePath $setupPath -Description 'Default Program Files setup' -ArgumentList @(
        '/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', '/ALLUSERS', '/TASKS="desktopicon"'
    )
    $defaultExecutable = Join-Path $defaultInstall 'magichandy.exe'
    Assert-Release -Condition (Test-Path -LiteralPath $defaultExecutable -PathType Leaf) -Message 'default setup should install under Program Files'
    Assert-Version -Executable $defaultExecutable
    Assert-Release -Condition (Test-Path -LiteralPath $commonDesktopShortcut -PathType Leaf) -Message 'all-users desktop shortcut should be created when selected'
    Assert-Release -Condition (Test-Path -LiteralPath $commonStartShortcut -PathType Leaf) -Message 'all-users Start Menu shortcut should be created'
    Assert-UninstallEntry -RegistryPath $machineUninstall -InstallDirectory $defaultInstall

    & $defaultExecutable -set-ui-locale ja -set-chat-locale ja -complete-setup
    if ($LASTEXITCODE -ne 0) {
        throw 'Could not seed default app data for clean-uninstall acceptance.'
    }
    [System.IO.File]::WriteAllText((Join-Path $defaultData 'stale-reinstall-marker.txt'), 'must be removed')
    $externalMarker = Join-Path $testRoot 'external-user-owned-media.txt'
    [System.IO.File]::WriteAllText($externalMarker, 'must be preserved')
    $port = Get-AvailableLoopbackPort
    $app = $null
    try {
        $app = Start-Process -FilePath $defaultExecutable -ArgumentList @('-addr', "127.0.0.1:$port") -PassThru -WindowStyle Hidden
        Wait-MagicHandyReady -Port $port
        $defaultUninstaller = Get-Uninstaller -InstallDirectory $defaultInstall
        Invoke-ReleaseProcess -FilePath $defaultUninstaller -Description 'Clean uninstall' -ArgumentList @(
            '/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', '/PURGEUSERDATA'
        )
        Assert-Release -Condition ($app.WaitForExit(30000)) -Message 'uninstall should gracefully stop the running app and its managed workers'
    } finally {
        if ($null -ne $app -and -not $app.HasExited) {
            Stop-Process -Id $app.Id -Force
            $app.WaitForExit()
        }
    }
    foreach ($path in @($defaultInstall, $defaultData, $commonDesktopShortcut, $commonStartShortcut, $machineUninstall)) {
        Assert-Release -Condition (-not (Test-Path -LiteralPath $path)) -Message "clean uninstall should remove '$path'"
    }
    Assert-Release -Condition (Test-Path -LiteralPath $externalMarker -PathType Leaf) -Message 'clean uninstall should not remove user-owned paths outside app data'
    $ownsDefaultInstall = $false

    $ownsDefaultInstall = $true
    Invoke-ReleaseProcess -FilePath $setupPath -Description 'Clean reinstall' -ArgumentList @(
        '/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', '/ALLUSERS', '/NOICONS'
    )
    $port = Get-AvailableLoopbackPort
    $app = Start-Process -FilePath $defaultExecutable -ArgumentList @('-addr', "127.0.0.1:$port") -PassThru -WindowStyle Hidden
    try {
        Wait-MagicHandyReady -Port $port
        $state = Invoke-RestMethod -Uri "http://127.0.0.1:$port/api/state" -TimeoutSec 5
        Assert-Release -Condition (-not [bool]$state.settings.ui.setup_completed) -Message 'reinstall after purge should start with guided setup incomplete'
        Assert-Release -Condition ([string]$state.settings.ui.locale -eq 'en') -Message 'reinstall after purge should use default settings'
    } finally {
        if ($null -ne $app -and -not $app.HasExited) {
            Stop-Process -Id $app.Id -Force
            $app.WaitForExit()
        }
    }
    $defaultUninstaller = Get-Uninstaller -InstallDirectory $defaultInstall
    Invoke-ReleaseProcess -FilePath $defaultUninstaller -Description 'Final clean uninstall' -ArgumentList @(
        '/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', '/PURGEUSERDATA'
    )
    foreach ($path in @($defaultInstall, $defaultData, $commonDesktopShortcut, $commonStartShortcut, $machineUninstall)) {
        Assert-Release -Condition (-not (Test-Path -LiteralPath $path)) -Message "final cleanup should remove '$path'"
    }
    Assert-Release -Condition (Test-Path -LiteralPath $externalMarker -PathType Leaf) -Message 'final cleanup should preserve user-owned paths outside app data'
    $ownsDefaultInstall = $false

    Write-Host 'Windows release install, uninstall, and clean-reinstall acceptance passed.' -ForegroundColor Green
} finally {
    if ($null -ne $app -and -not $app.HasExited) {
        Stop-Process -Id $app.Id -Force -ErrorAction SilentlyContinue
        $app.WaitForExit()
    }
    foreach ($cleanup in @(
        [pscustomobject]@{ Owned = $ownsCustomInstall; Directory = $customInstall; DataSwitch = '/KEEPUSERDATA' },
        [pscustomobject]@{ Owned = $ownsDefaultInstall; Directory = $defaultInstall; DataSwitch = '/PURGEUSERDATA' }
    )) {
        if (-not $cleanup.Owned -or [string]::IsNullOrWhiteSpace([string]$cleanup.Directory)) {
            continue
        }
        $uninstaller = Get-ChildItem -LiteralPath $cleanup.Directory -Filter 'unins*.exe' -File -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($null -ne $uninstaller) {
            try {
                Start-Process -FilePath $uninstaller.FullName -ArgumentList @(
                    '/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', [string]$cleanup.DataSwitch
                ) -Wait -ErrorAction Stop | Out-Null
            } catch {
                Write-Warning "Release-test cleanup could not run '$($uninstaller.FullName)': $_"
            }
        }
    }
    if (Test-Path -LiteralPath $testRoot) {
        Remove-Item -LiteralPath $testRoot -Recurse -Force
    }
}
