# Ensure managed llama.cpp runner + GGUF model for MagicHandy (separate from LSO/Ollama).
$ErrorActionPreference = "Stop"

$root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$dataDir = if ($env:MAGICHANDY_DATA_DIR) {
    $env:MAGICHANDY_DATA_DIR.Trim()
} else {
    Join-Path $root ".local-data"
}

$llamaDir = Join-Path $dataDir "llama"
$runnerPath = Join-Path $llamaDir "llama-server.exe"
$modelDir = Join-Path $dataDir "models\gguf\qwen2.5-7b-instruct-q4_k_m"
$modelPath = Join-Path $modelDir "model.gguf"
$legacyDolphinDir = Join-Path $dataDir "models\gguf\dolphin-2.9.3-mistral-nemo-12b-q4_k_m"
$settingsPath = Join-Path $dataDir "settings.json"
$llamaPort = if ($env:MAGICHANDY_LLAMA_PORT) { [int]$env:MAGICHANDY_LLAMA_PORT.Trim() } else { 18080 }
$llamaBaseUrl = "http://127.0.0.1:${llamaPort}"
$modelName = "Qwen2.5-7B-Instruct-Q4_K_M"
$modelUrl = "https://huggingface.co/bartowski/Qwen2.5-7B-Instruct-GGUF/resolve/main/Qwen2.5-7B-Instruct-Q4_K_M.gguf"
$modelMinBytes = 4.2GB
$llamaRelease = "b9886"

function Ensure-Directory([string]$Path) {
    New-Item -ItemType Directory -Force -Path $Path | Out-Null
}

function Download-FileIfMissing {
    param(
        [string]$Url,
        [string]$Destination,
        [string]$Label
    )
    if (Test-Path $Destination) {
        $size = (Get-Item $Destination).Length
        if ($size -gt 1MB) {
            Write-Host "$Label OK (cached)."
            return
        }
        Remove-Item $Destination -Force
    }
    Ensure-Directory (Split-Path $Destination -Parent)
    Write-Host "Downloading $Label..."
    Write-Host "  $Url"
    Invoke-WebRequest -Uri $Url -OutFile $Destination -UseBasicParsing
}

function Expand-ZipInto {
    param(
        [string]$ZipPath,
        [string]$Destination
    )
    Ensure-Directory $Destination
    Expand-Archive -Path $ZipPath -DestinationPath $Destination -Force
}

function Find-LlamaServer([string]$SearchRoot) {
    Get-ChildItem -Path $SearchRoot -Filter "llama-server.exe" -Recurse -ErrorAction SilentlyContinue |
        Select-Object -First 1 -ExpandProperty FullName
}

if (-not (Test-Path $runnerPath)) {
    $cacheDir = Join-Path $dataDir "downloads"
    Ensure-Directory $cacheDir
    $binZip = Join-Path $cacheDir "llama-$llamaRelease-bin-win-cuda-12.4-x64.zip"
    $cudaZip = Join-Path $cacheDir "cudart-llama-$llamaRelease-bin-win-cuda-12.4-x64.zip"
    $extractDir = Join-Path $cacheDir "llama-$llamaRelease"

    Download-FileIfMissing `
        -Url "https://github.com/ggml-org/llama.cpp/releases/download/$llamaRelease/llama-$llamaRelease-bin-win-cuda-12.4-x64.zip" `
        -Destination $binZip `
        -Label "llama.cpp CUDA binaries"

    Download-FileIfMissing `
        -Url "https://github.com/ggml-org/llama.cpp/releases/download/$llamaRelease/cudart-llama-bin-win-cuda-12.4-x64.zip" `
        -Destination $cudaZip `
        -Label "llama.cpp CUDA runtime DLLs"

    if (Test-Path $extractDir) { Remove-Item $extractDir -Recurse -Force }
    Expand-ZipInto -ZipPath $binZip -Destination $extractDir
    Expand-ZipInto -ZipPath $cudaZip -Destination $extractDir

    $found = Find-LlamaServer $extractDir
    if (-not $found) {
        throw "llama-server.exe not found inside downloaded llama.cpp package"
    }

    Ensure-Directory $llamaDir
    Copy-Item -Path (Join-Path (Split-Path $found -Parent) "*") -Destination $llamaDir -Recurse -Force
    if (-not (Test-Path $runnerPath)) {
        throw "Failed to install llama-server.exe to $llamaDir"
    }
    Write-Host "llama-server installed: $runnerPath"
} else {
    Write-Host "llama-server OK: $runnerPath"
}

if (Test-Path $modelPath) {
    $size = (Get-Item $modelPath).Length
    if ($size -lt $modelMinBytes) {
        Write-Host "Incomplete GGUF ($([math]::Round($size / 1GB, 2)) GB) - re-downloading..."
        Remove-Item $modelPath -Force
    }
}
if (-not (Test-Path $modelPath)) {
    Ensure-Directory $modelDir
    Download-FileIfMissing `
        -Url $modelUrl `
        -Destination $modelPath `
        -Label "Qwen2.5-7B-Instruct Q4_K_M GGUF (~4.7 GB, one-time)"
} else {
    Write-Host "GGUF model OK: $modelPath"
}

if (Test-Path $legacyDolphinDir) {
    Remove-Item $legacyDolphinDir -Recurse -Force
    Write-Host "Removed legacy Dolphin model (~7.5 GB freed)"
}

$llmBlock = @{
    provider = "llama_cpp"
    llama_cpp_mode = "managed"
    llama_cpp_base_url = $llamaBaseUrl
    llama_cpp_runner_path = $runnerPath
    llama_cpp_model_path = $modelPath
    ollama_base_url = "http://127.0.0.1:11434"
    model = $modelName
    prompt_set = "magichandy_motion_v1_pt_br"
    request_timeout_ms = 120000
}

if (Test-Path $settingsPath) {
    $settings = Get-Content -Path $settingsPath -Raw -Encoding utf8 | ConvertFrom-Json
    if (-not $settings.llm) { $settings | Add-Member -NotePropertyName llm -NotePropertyValue ([pscustomobject]@{}) }
    foreach ($key in $llmBlock.Keys) {
        $settings.llm | Add-Member -NotePropertyName $key -NotePropertyValue $llmBlock[$key] -Force
    }
    if (-not $settings.device -or -not $settings.device.hsp_dispatch_owner) {
        if (-not $settings.device) { $settings | Add-Member -NotePropertyName device -NotePropertyValue ([pscustomobject]@{}) }
        $settings.device | Add-Member -NotePropertyName hsp_dispatch_owner -NotePropertyValue "intiface" -Force
    } elseif ($settings.device.hsp_dispatch_owner -eq "cloud_rest") {
        $settings.device | Add-Member -NotePropertyName hsp_dispatch_owner -NotePropertyValue "intiface" -Force
    }
    if (-not $settings.device.intiface_url) {
        $settings.device | Add-Member -NotePropertyName intiface_url -NotePropertyValue "ws://127.0.0.1:12345" -Force
    }
} else {
    $settings = @{
        version = 1
        server = @{ port = 49717 }
        device = @{
            hsp_dispatch_owner = "intiface"
            intiface_url = "ws://127.0.0.1:12345"
            firmware_api_requirement = "firmware_v4_api_v3_required"
            api_application_id_source = "bundled_app_id"
        }
        motion = @{
            speed_min_percent = 20
            speed_max_percent = 80
            stroke_min_percent = 0
            stroke_max_percent = 100
            reverse_direction = $false
            style = "balanced"
        }
        llm = $llmBlock
        diagnostics = @{ verbosity = "normal" }
    }
}

$settings | ConvertTo-Json -Depth 6 | Set-Content -Path $settingsPath -Encoding utf8
Write-Host "MagicHandy LLM settings: $settingsPath"
Write-Host "  provider: llama_cpp (managed)"
Write-Host "  llama:    $llamaBaseUrl"
Write-Host "  model:    $modelName"
Write-Host "  (LSO/Ollama continua em :11434; MagicHandy usa llama.cpp em :$llamaPort)"
