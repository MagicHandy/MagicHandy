# Stop MagicHandy background server on :49717 (or MAGICHANDY_PORT).
$ErrorActionPreference = "SilentlyContinue"

$root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$dataDir = if ($env:MAGICHANDY_DATA_DIR) {
    $env:MAGICHANDY_DATA_DIR.Trim()
} else {
    Join-Path $root ".local-data"
}
$pidFile = Join-Path $dataDir "stack.pids.json"
$port = if ($env:MAGICHANDY_PORT) { [int]$env:MAGICHANDY_PORT.Trim() } else { 49717 }
$llamaPort = if ($env:MAGICHANDY_LLAMA_PORT) { [int]$env:MAGICHANDY_LLAMA_PORT.Trim() } else { 18080 }
$killed = [System.Collections.Generic.HashSet[int]]::new()
$protectPids = [System.Collections.Generic.HashSet[int]]::new()
[void]$protectPids.Add($PID)
try {
    $parent = Get-CimInstance Win32_Process -Filter "ProcessId=$PID" -ErrorAction SilentlyContinue
    if ($parent -and $parent.ParentProcessId -gt 0) {
        [void]$protectPids.Add([int]$parent.ParentProcessId)
    }
} catch { }

function Test-LauncherCommandLine([string]$CommandLine) {
    if (-not $CommandLine) { return $false }
    return $CommandLine -match '(?i)(start_stack|stop_stack)\.ps1|Iniciar-MagicHandy\.bat|Parar-MagicHandy\.bat|start\.bat'
}

function Register-Killed([int]$ProcessId) {
    if ($ProcessId -gt 0) { [void]$killed.Add($ProcessId) }
}

function Stop-ProcessTree([int]$ProcessId) {
    if ($ProcessId -le 0 -or $killed.Contains($ProcessId)) { return }
    Get-CimInstance Win32_Process -ErrorAction SilentlyContinue |
        Where-Object { $_.ParentProcessId -eq $ProcessId } |
        ForEach-Object { Stop-ProcessTree([int]$_.ProcessId) }
    Stop-Process -Id $ProcessId -Force -ErrorAction SilentlyContinue
    Register-Killed $ProcessId
}

function Stop-PortOwners([int]$TargetPort) {
    $conns = Get-NetTCPConnection -LocalPort $TargetPort -State Listen -ErrorAction SilentlyContinue
    foreach ($c in $conns) {
        Stop-ProcessTree([int]$c.OwningProcess)
    }
}

function Stop-ByCommandLine([string]$Pattern) {
    Get-CimInstance Win32_Process -ErrorAction SilentlyContinue |
        Where-Object {
            $_.CommandLine -and
            ($_.CommandLine -match $Pattern) -and
            -not $protectPids.Contains([int]$_.ProcessId) -and
            -not (Test-LauncherCommandLine $_.CommandLine)
        } |
        ForEach-Object { Stop-ProcessTree([int]$_.ProcessId) }
}

function Stop-MagicHandyPowerShellWindows() {
    Get-Process powershell -ErrorAction SilentlyContinue | ForEach-Object {
        $title = $_.MainWindowTitle
        if ($title -like "MagicHandy -*") {
            Stop-ProcessTree($_.Id)
        }
    }
}

if (Test-Path $pidFile) {
    try {
        $saved = Get-Content $pidFile -Raw | ConvertFrom-Json
        foreach ($key in @("magichandy", "shell")) {
            $val = $saved.$key
            if ($null -ne $val) { Stop-ProcessTree([int]$val) }
        }
    } catch { }
    Remove-Item $pidFile -Force -ErrorAction SilentlyContinue
}

Stop-PortOwners $port
Stop-PortOwners $llamaPort

$rootEsc = [regex]::Escape($root)
Stop-ByCommandLine "$rootEsc\\bin\\magichandy\.exe"
Stop-ByCommandLine "magichandy(\.exe)?"
Stop-ByCommandLine "go run.*cmd/magichandy"
Stop-ByCommandLine "go run.*cmd\\magichandy"
Stop-ByCommandLine "llama-server(\.exe)?"

Stop-MagicHandyPowerShellWindows
Stop-PortOwners $port
Stop-PortOwners $llamaPort

Write-Host "MagicHandy encerrado ($($killed.Count) processo(s)). Portas $port e $llamaPort liberadas."
