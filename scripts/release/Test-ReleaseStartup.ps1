#Requires -Version 5.1
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$Executable,
    [Parameter(Mandatory = $true)][string]$TestRoot
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# Load just the harness functions, without installing or removing anything.
$tokens = $null
$parseErrors = $null
$ast = [Management.Automation.Language.Parser]::ParseFile(
    (Join-Path $PSScriptRoot 'Test-WindowsRelease.ps1'), [ref]$tokens, [ref]$parseErrors)
if ($parseErrors.Count -ne 0) { throw 'Release acceptance script must parse.' }
foreach ($name in @('Get-AvailableLoopbackPort', 'Start-ReleaseTestApp', 'Wait-MagicHandyReady')) {
    $definition = $ast.Find({
        $args[0] -is [Management.Automation.Language.FunctionDefinitionAst] -and $args[0].Name -eq $name
    }, $true)
    if ($null -eq $definition) { throw "Missing release helper $name." }
    Invoke-Expression $definition.Extent.Text
}

$logs = Join-Path $TestRoot 'logs'
$port = Get-AvailableLoopbackPort
$occupiedRemote = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, $port + 1)
$app = $null
try {
    $occupiedRemote.Start()
    $app = Start-ReleaseTestApp -Executable $Executable -Port $port -LogDirectory $logs `
        -Stage 'occupied-remote' -DataDirectory (Join-Path $TestRoot 'blocked data')
    $watch = [Diagnostics.Stopwatch]::StartNew()
    $failure = ''
    try { Wait-MagicHandyReady -Port $port -Process $app } catch { $failure = $_.Exception.Message }
    if ($failure -notmatch 'remote listener' -or $failure -notmatch 'exited with code') {
        throw "An occupied remote port must report the actual startup failure: $failure"
    }
    if ($watch.Elapsed.TotalSeconds -gt 20) { throw 'Exited app should fail promptly, not wait for the health timeout.' }
    if (-not (Test-Path -LiteralPath $app.StartupStderr)) { throw 'Startup stderr was not retained.' }
} finally {
    if ($null -ne $app -and -not $app.HasExited) { Stop-Process -Id $app.Id; $app.WaitForExit() }
    $occupiedRemote.Stop()
}

for ($attempt = 1; $attempt -le 3; $attempt++) {
    $port = Get-AvailableLoopbackPort
    $app = Start-ReleaseTestApp -Executable $Executable -Port $port -LogDirectory $logs `
        -Stage "ready-$attempt" -DataDirectory (Join-Path $TestRoot "ready data $attempt")
    try {
        # Readiness verifies both listeners; a main-only server cannot pass.
        Wait-MagicHandyReady -Port $port -Process $app
    } finally {
        if (-not $app.HasExited) { Stop-Process -Id $app.Id; $app.WaitForExit() }
    }
}
Write-Output 'Release startup regression passed: occupied remote port diagnosed; three dual-listener starts passed.'
