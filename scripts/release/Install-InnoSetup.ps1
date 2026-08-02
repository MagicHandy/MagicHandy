#requires -Version 5.1
<#
.SYNOPSIS
Installs the checksum-pinned Inno Setup 7 compiler used by Windows packaging.
#>
[CmdletBinding()]
param(
    [string]$InstallRoot = '',
    [string]$DownloadRoot = ''
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$version = '7.0.2'
$assetName = 'innosetup-7.0.2-x64.exe'
$downloadURL = "https://github.com/jrsoftware/issrc/releases/download/is-7_0_2/$assetName"
$expectedSHA256 = '5ad54ca3def786f8f4212552e54cc6d8d61329e2d24a1cfee0571d42c2684ff1'

if ([string]::IsNullOrWhiteSpace($InstallRoot)) {
    $InstallRoot = Join-Path $env:LOCALAPPDATA 'Programs\Inno Setup 7'
}
if ([string]::IsNullOrWhiteSpace($DownloadRoot)) {
    $DownloadRoot = Join-Path ([System.IO.Path]::GetTempPath()) 'MagicHandy-release-tools'
}
$InstallRoot = [System.IO.Path]::GetFullPath($InstallRoot)
$DownloadRoot = [System.IO.Path]::GetFullPath($DownloadRoot)
$iscc = Join-Path $InstallRoot 'ISCC.exe'

function Test-InnoSetup7 {
    param([Parameter(Mandatory = $true)][string]$Path)

    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        return $false
    }

    # ISCC reports help with exit code 1. Invoke it through Process so that
    # probing a valid compiler cannot leak that native status to the caller.
    $startInfo = New-Object System.Diagnostics.ProcessStartInfo
    $startInfo.FileName = $Path
    $startInfo.Arguments = '/?'
    $startInfo.UseShellExecute = $false
    $startInfo.CreateNoWindow = $true
    $startInfo.RedirectStandardOutput = $true
    $startInfo.RedirectStandardError = $true
    $process = New-Object System.Diagnostics.Process
    $process.StartInfo = $startInfo
    try {
        if (-not $process.Start()) {
            return $false
        }
        $stdout = $process.StandardOutput.ReadToEnd()
        $stderr = $process.StandardError.ReadToEnd()
        $process.WaitForExit()
    } finally {
        $process.Dispose()
    }
    $help = "$stdout`n$stderr"
    return $help.Contains('Inno Setup 7 Command-Line Compiler')
}

if (Test-InnoSetup7 -Path $iscc) {
    Write-Host "Reusing Inno Setup $version at '$iscc'."
    Write-Output $iscc
    return
}

New-Item -ItemType Directory -Force -Path $DownloadRoot | Out-Null
$downloadPath = Join-Path $DownloadRoot $assetName
Write-Host "Downloading verified Inno Setup $version x64..."
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
Invoke-WebRequest -UseBasicParsing -Uri $downloadURL -OutFile $downloadPath
$actualSHA256 = (Get-FileHash -LiteralPath $downloadPath -Algorithm SHA256).Hash.ToLowerInvariant()
if ($actualSHA256 -ne $expectedSHA256) {
    throw "Inno Setup download checksum mismatch: expected $expectedSHA256, got $actualSHA256."
}

New-Item -ItemType Directory -Force -Path $InstallRoot | Out-Null
$arguments = @(
    '/VERYSILENT',
    '/SUPPRESSMSGBOXES',
    '/NORESTART',
    '/SP-',
    '/ALLUSERS',
    '/NOICONS',
    "/DIR=`"$InstallRoot`""
)
$process = Start-Process -FilePath $downloadPath -ArgumentList $arguments -Wait -PassThru
if ($process.ExitCode -ne 0) {
    throw "Inno Setup $version installation failed with exit $($process.ExitCode)."
}
if (-not (Test-InnoSetup7 -Path $iscc)) {
    throw "Inno Setup $version did not install a usable compiler at '$iscc'."
}

Write-Host "Installed verified Inno Setup $version at '$iscc'."
Write-Output $iscc
