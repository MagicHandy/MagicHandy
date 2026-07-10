# Start MagicHandy HTTP server in background; open http://127.0.0.1:49717
$ErrorActionPreference = "Stop"

$root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$dataDir = if ($env:MAGICHANDY_DATA_DIR) {
    $env:MAGICHANDY_DATA_DIR.Trim()
} else {
    Join-Path $root ".local-data"
}
$logDir = Join-Path $dataDir "logs"
$pidFile = Join-Path $dataDir "stack.pids.json"
$binDir = Join-Path $root "bin"
$exePath = Join-Path $binDir "magichandy.exe"
$port = if ($env:MAGICHANDY_PORT) { [int]$env:MAGICHANDY_PORT.Trim() } else { 49717 }
$bindHost = if ($env:MAGICHANDY_HOST) { $env:MAGICHANDY_HOST.Trim() } else { "127.0.0.1" }
$addr = "${bindHost}:${port}"
$openHost = if ($bindHost -in @("0.0.0.0", "::")) { "127.0.0.1" } else { $bindHost }
$url = "http://${openHost}:${port}"

New-Item -ItemType Directory -Force -Path $dataDir, $logDir, $binDir | Out-Null

$ensureLlama = Join-Path $PSScriptRoot "ensure_llama_cpp.ps1"
if (Test-Path $ensureLlama) {
    & $ensureLlama
}

$lsoPort = 8080
$lsoListener = Get-NetTCPConnection -LocalPort $lsoPort -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
if ($lsoListener) {
    Write-Warning "LSO ainda esta rodando na porta $lsoPort (PID $($lsoListener.OwningProcess))."
    Write-Warning "O Intiface so aceita um cliente de controle - pare o LSO com Parar-LSO.bat antes de usar o Handy."
}

$stopScript = Join-Path $PSScriptRoot "stop_stack.ps1"
if (Test-Path $stopScript) {
    & $stopScript | Out-Null
    Start-Sleep -Seconds 1
}

function Find-GoExecutable {
    $candidates = @(
        (Get-Command go -ErrorAction SilentlyContinue | Select-Object -ExpandProperty Source),
        "C:\Program Files\Go\bin\go.exe",
        "$env:LOCALAPPDATA\Programs\Go\bin\go.exe"
    ) | Where-Object { $_ -and (Test-Path $_) }
    if (-not $candidates) {
        throw "Go nao encontrado. Instale com: winget install GoLang.Go"
    }
    return [string]($candidates | Select-Object -First 1)
}

function Wait-PortListener {
    param([int]$TargetPort, [int]$TimeoutSec = 60)
    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    while ((Get-Date) -lt $deadline) {
        $conn = Get-NetTCPConnection -LocalPort $TargetPort -State Listen -ErrorAction SilentlyContinue |
            Select-Object -First 1
        if ($conn) { return [int]$conn.OwningProcess }
        Start-Sleep -Milliseconds 400
    }
    return 0
}

function Start-BackgroundProcess {
    param(
        [string]$Name,
        [string]$FilePath,
        [string[]]$ArgumentList,
        [string]$WorkingDirectory = $root
    )
    $outLog = Join-Path $logDir ($Name + ".out.log")
    $errLog = Join-Path $logDir ($Name + ".err.log")
    Set-Content -Path $outLog -Value "" -Encoding utf8
    Set-Content -Path $errLog -Value "" -Encoding utf8

    $proc = Start-Process `
        -FilePath $FilePath `
        -ArgumentList $ArgumentList `
        -WorkingDirectory $WorkingDirectory `
        -WindowStyle Hidden `
        -PassThru `
        -RedirectStandardOutput $outLog `
        -RedirectStandardError $errLog
    return [int]$proc.Id
}

$go = Find-GoExecutable

$feDir = Join-Path $root "frontend"
$uiDist = Join-Path $root "uibuild\dist"
if (Test-Path (Join-Path $feDir "package.json")) {
    Write-Host "npm install (frontend UI)..."
    Push-Location $feDir
    & npm install --no-fund --no-audit
    if ($LASTEXITCODE -ne 0) { throw "npm install failed" }
    Write-Host "npm run build (frontend)..."
    & npm run build
    if ($LASTEXITCODE -ne 0) { throw "npm run build failed" }
    Pop-Location
    if (Test-Path (Join-Path $feDir "dist\index.html")) {
        New-Item -ItemType Directory -Force -Path $uiDist | Out-Null
        robocopy (Join-Path $feDir "dist") $uiDist /E /NFL /NDL /NJH /NJS /nc /ns /np | Out-Null
        Write-Host "React UI (frontend) copied to uibuild/dist"
    }
}

Write-Host "go build (magichandy)..."
Push-Location $root
& "$go" build -o $exePath ./cmd/magichandy
if ($LASTEXITCODE -ne 0) { throw "go build failed" }
Pop-Location

if (-not (Test-Path $exePath)) {
    throw "Missing bin/magichandy.exe after build"
}

Write-Host "Starting MagicHandy (background) bind $addr -> $url ..."
$shellPid = Start-BackgroundProcess `
    -Name "magichandy" `
    -FilePath $exePath `
    -ArgumentList @("-addr", $addr, "-data-dir", $dataDir)

$listenerPid = Wait-PortListener -TargetPort $port
$appPid = if ($listenerPid -gt 0) { $listenerPid } else { $shellPid }

$healthy = $false
for ($i = 0; $i -lt 30; $i++) {
    try {
        $r = Invoke-WebRequest -Uri ($url + "/healthz") -UseBasicParsing -TimeoutSec 2
        if ($r.StatusCode -eq 200) { $healthy = $true; break }
    } catch { }
    Start-Sleep -Seconds 1
}

if (-not $healthy) {
    Write-Warning "MagicHandy /healthz not ready yet - check $logDir\magichandy.err.log"
}

Write-Host "Loading managed llama.cpp model (first run may take ~30s)..."
try {
    $llmStatus = Invoke-RestMethod -Uri ($url + "/api/llm/load") -Method POST -TimeoutSec 240
    if ($llmStatus.available) {
        Write-Host "LLM ready: $($llmStatus.message)"
    } else {
        Write-Warning "LLM not ready: $($llmStatus.message)"
        Write-Warning "Check $logDir\magichandy.err.log and .local-data\llama\"
    }
} catch {
    Write-Warning "LLM load failed: $($_.Exception.Message)"
}

$pids = @{
    mode       = "background"
    url        = $url
    port       = $port
    magichandy = $appPid
    shell      = $shellPid
    started_at = (Get-Date).ToString("o")
}
$pids | ConvertTo-Json | Set-Content -Encoding utf8 $pidFile

Write-Host ""
Write-Host "MagicHandy running in background."
Write-Host "  URL:   $url"
Write-Host "  Data:  $dataDir"
Write-Host "  Logs:  $logDir"
Write-Host "  Stop:  Parar-MagicHandy.bat"
Write-Host ""

try {
    Start-Process $url
} catch {
    Start-Process "cmd.exe" -ArgumentList @("/c", "start", $url) -WindowStyle Hidden
}
