<#
.SYNOPSIS
    Bootstraps a MagicHandy source checkout on a clean Windows machine.

.DESCRIPTION
    Requires only Windows PowerShell 5.1 and internet access. The script repairs
    Windows Package Manager when needed, installs Git after consent, clones
    MagicHandy, and delegates all setup choices to install.ps1.

.PARAMETER InstallDir
    Destination checkout. Default: .\MagicHandy.

.PARAMETER Yes
    Accept the Git package prompt and pass unattended consent to install.ps1.

.PARAMETER NoLaunch
    Complete setup without starting MagicHandy.
#>
#Requires -Version 5.1
[CmdletBinding()]
param(
    [string]$InstallDir = (Join-Path ([string](Get-Location)) 'MagicHandy'),
    [switch]$Yes,
    [switch]$NoLaunch
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$repositoryURL = 'https://github.com/MagicHandy/MagicHandy.git'
$InstallDir = [System.IO.Path]::GetFullPath($InstallDir)

function Refresh-BootstrapPath {
    $parts = New-Object System.Collections.Generic.List[string]
    foreach ($value in @(
        $env:Path,
        [Environment]::GetEnvironmentVariable('Path', 'Machine'),
        [Environment]::GetEnvironmentVariable('Path', 'User')
    )) {
        if ([string]::IsNullOrWhiteSpace($value)) {
            continue
        }
        foreach ($part in ($value -split ';')) {
            if (-not [string]::IsNullOrWhiteSpace($part) -and $parts -notcontains $part) {
                $parts.Add($part)
            }
        }
    }
    $env:Path = $parts -join ';'
}

function Resolve-BootstrapExecutable {
    param(
        [Parameter(Mandatory = $true)][string]$Name,
        [string[]]$Candidates = @()
    )

    $command = Get-Command "$Name.exe" -ErrorAction SilentlyContinue
    if ($command) {
        return [string]$command.Source
    }
    foreach ($candidate in $Candidates) {
        if (-not [string]::IsNullOrWhiteSpace($candidate) -and
            (Test-Path -LiteralPath $candidate -PathType Leaf)) {
            return $candidate
        }
    }
    return $null
}

function Ensure-BootstrapWinGet {
    $winget = Resolve-BootstrapExecutable -Name 'winget' -Candidates @(
        (Join-Path $env:LOCALAPPDATA 'Microsoft\WindowsApps\winget.exe')
    )
    if ($winget) {
        return $winget
    }

    Write-Host 'Windows Package Manager is missing.' -ForegroundColor Yellow
    Write-Host 'Repairing it through Microsoft.WinGet.Client for the current user.' -ForegroundColor DarkGray
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    Install-PackageProvider -Name NuGet -Force -Scope CurrentUser | Out-Null
    Install-Module -Name Microsoft.WinGet.Client -Force -Repository PSGallery -Scope CurrentUser
    Import-Module Microsoft.WinGet.Client -Force
    Repair-WinGetPackageManager -Force -Latest | Out-Host
    Refresh-BootstrapPath

    $winget = Resolve-BootstrapExecutable -Name 'winget' -Candidates @(
        (Join-Path $env:LOCALAPPDATA 'Microsoft\WindowsApps\winget.exe')
    )
    if (-not $winget) {
        throw 'Windows Package Manager repair completed, but winget.exe is unavailable. Restart PowerShell and rerun the bootstrap.'
    }
    return $winget
}

function Ensure-BootstrapGit {
    $git = Resolve-BootstrapExecutable -Name 'git' -Candidates @(
        (Join-Path $env:ProgramFiles 'Git\cmd\git.exe'),
        $(if (${env:ProgramFiles(x86)}) { Join-Path ${env:ProgramFiles(x86)} 'Git\cmd\git.exe' }),
        (Join-Path $env:LOCALAPPDATA 'Programs\Git\cmd\git.exe')
    )
    if ($git) {
        return $git
    }

    Write-Host ''
    Write-Host 'Git for Windows is required to download and update MagicHandy.'
    Write-Host 'License: GPL-2.0; https://gitforwindows.org/' -ForegroundColor DarkGray
    if (-not $Yes) {
        $answer = Read-Host 'Install Git for Windows now? [Y/n]'
        if (-not [string]::IsNullOrWhiteSpace($answer) -and
            $answer.Trim() -notmatch '^(?i:y|yes)$') {
            throw 'Git installation was declined.'
        }
    }

    $winget = Ensure-BootstrapWinGet
    & $winget install --id Git.Git --exact --source winget --accept-source-agreements --accept-package-agreements --disable-interactivity
    if ($LASTEXITCODE -ne 0) {
        throw "Git installation failed (exit $LASTEXITCODE)."
    }
    Refresh-BootstrapPath
    $git = Resolve-BootstrapExecutable -Name 'git' -Candidates @(
        (Join-Path $env:ProgramFiles 'Git\cmd\git.exe'),
        $(if (${env:ProgramFiles(x86)}) { Join-Path ${env:ProgramFiles(x86)} 'Git\cmd\git.exe' }),
        (Join-Path $env:LOCALAPPDATA 'Programs\Git\cmd\git.exe')
    )
    if (-not $git) {
        throw 'Git was installed but could not be resolved in this PowerShell session.'
    }
    return $git
}

Write-Host ''
Write-Host 'MagicHandy clean-machine bootstrap' -ForegroundColor Cyan
Write-Host 'Windows PowerShell and internet access are the only prerequisites.' -ForegroundColor DarkGray

$git = Ensure-BootstrapGit
$gitDir = Join-Path $InstallDir '.git'
if (Test-Path -LiteralPath $gitDir -PathType Container) {
    Write-Host "Using existing checkout: $InstallDir" -ForegroundColor Green
} else {
    if (Test-Path -LiteralPath $InstallDir) {
        $entries = @(Get-ChildItem -LiteralPath $InstallDir -Force)
        if ($entries.Count -gt 0) {
            throw "InstallDir exists and is not an empty Git checkout: '$InstallDir'."
        }
    } else {
        New-Item -ItemType Directory -Path (Split-Path -Parent $InstallDir) -Force | Out-Null
    }
    Write-Host "Cloning MagicHandy into $InstallDir..."
    & $git clone $repositoryURL $InstallDir
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $gitDir -PathType Container)) {
        throw "MagicHandy clone failed (exit $LASTEXITCODE)."
    }
}

$installer = Join-Path $InstallDir 'install.ps1'
if (-not (Test-Path -LiteralPath $installer -PathType Leaf)) {
    throw "The checkout does not contain install.ps1: '$InstallDir'."
}
$installerArguments = @{}
if ($Yes) {
    $installerArguments.Yes = $true
}
if ($NoLaunch) {
    $installerArguments.NoLaunch = $true
}
& $installer @installerArguments
