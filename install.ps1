#Requires -Version 5.1
<#
.SYNOPSIS
    Interactive installer for MagicHandy — a local-first AI controller for The Handy.

.DESCRIPTION
    Checks for the Go toolchain (offers to install it), builds MagicHandy, sets up
    a data folder, optionally helps you get a local LLM running (with CUDA
    detection and explicit, consented model downloads), and launches the app.

    The runtime is a single Go binary that serves an embedded browser UI on
    localhost. Nothing here downloads a model or installs software without asking.

    Adults only. MagicHandy controls an intimate device; use it responsibly and at
    your own risk.

.PARAMETER Port
    Local port to serve on (default 49717).

.PARAMETER DataDir
    Where to store settings and data. Default is your Windows profile; pass a path
    (e.g. .\data) for a portable install.

.PARAMETER Yes
    Assume "yes" to prompts (non-interactive).

.PARAMETER NoLaunch
    Build and set up, but do not start the app.

.EXAMPLE
    .\install.ps1

.EXAMPLE
    .\install.ps1 -DataDir .\data -Port 49800
#>
[CmdletBinding()]
param(
    [int]$Port = 49717,
    [string]$DataDir,
    [switch]$Yes,
    [switch]$NoLaunch
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Write-Head([string]$Text) {
    Write-Host ''
    Write-Host $Text -ForegroundColor Cyan
    Write-Host ('-' * $Text.Length) -ForegroundColor DarkGray
}
function Test-Cmd([string]$Name) {
    return [bool](Get-Command $Name -ErrorAction SilentlyContinue)
}
function Confirm-YesNo([string]$Question, [bool]$Default = $true) {
    if ($Yes) { return $true }
    $hint = if ($Default) { 'Y/n' } else { 'y/N' }
    $answer = Read-Host "$Question [$hint]"
    if ([string]::IsNullOrWhiteSpace($answer)) { return $Default }
    return $answer -match '^(y|yes)$'
}

Write-Host ''
Write-Host '  MagicHandy installer' -ForegroundColor White
Write-Host '  Local-first AI control for The Handy. Adults only; use responsibly.' -ForegroundColor DarkGray

# Resolve and validate the repo root (this script lives at the project root).
$Repo = Split-Path -Parent $MyInvocation.MyCommand.Path
if (-not (Test-Path (Join-Path $Repo 'cmd\magichandy'))) {
    throw "This script must run from the MagicHandy project folder (couldn't find cmd\magichandy)."
}
Set-Location $Repo

# --- 1. Go toolchain ---------------------------------------------------------
Write-Head '1. Go toolchain'
if (Test-Cmd 'go') {
    Write-Host "Found $(& go version)"
} else {
    Write-Host 'Go is not installed. MagicHandy is built from source with Go 1.25+.'
    if ((Test-Cmd 'winget') -and (Confirm-YesNo 'Install Go now with winget?')) {
        winget install --id GoLang.Go -e --source winget
        Write-Host 'Go installed. Close and reopen PowerShell so PATH updates, then re-run this script.' -ForegroundColor Yellow
        return
    }
    Write-Host 'Install Go 1.25+ from https://go.dev/dl/ and re-run this script.' -ForegroundColor Yellow
    return
}
$goVersion = & go version
if ($goVersion -match 'go(\d+)\.(\d+)') {
    $maj = [int]$Matches[1]; $min = [int]$Matches[2]
    if ($maj -lt 1 -or ($maj -eq 1 -and $min -lt 25)) {
        Write-Warning "Go $maj.$min detected; 1.25+ is recommended. Continuing anyway."
    }
}

# --- 2. Build ----------------------------------------------------------------
Write-Head '2. Build MagicHandy'
$exe = Join-Path $Repo 'magichandy.exe'
$env:CGO_ENABLED = '0'
Write-Host 'Building (pure Go, no C compiler needed)...'
& go build -o $exe ./cmd/magichandy
if ($LASTEXITCODE -ne 0) { throw 'Build failed. See the output above.' }
Write-Host "Built $exe" -ForegroundColor Green

# --- 3. Data folder ----------------------------------------------------------
Write-Head '3. Data folder'
if (-not $DataDir) {
    if (Confirm-YesNo 'Store data in a portable .\data folder next to the app? (No = your Windows profile)' $false) {
        $DataDir = Join-Path $Repo 'data'
    }
}
if ($DataDir) {
    Write-Host "Data will be stored in: $DataDir"
} else {
    Write-Host 'Data will be stored under your Windows profile (…\AppData\Roaming\MagicHandy).'
}
Write-Host 'Your settings, chat memory, and Handy connection key are stored locally only.'

# --- 4. Local LLM (optional) -------------------------------------------------
Write-Head '4. Local LLM (for chat)'
if (Test-Cmd 'nvidia-smi') {
    Write-Host 'NVIDIA GPU detected — llama.cpp with a CUDA build is the fast path.' -ForegroundColor Green
} else {
    Write-Host 'No NVIDIA GPU detected — Ollama or a CPU build of llama.cpp will work, just slower.'
}
if (Confirm-YesNo 'Set up a local LLM now? (you can always do this later in Settings > Model)') {
    if (Test-Cmd 'ollama') {
        Write-Host 'Ollama is installed.'
        $model = Read-Host 'Model to pull now (blank to skip, e.g. llama3.1:8b)'
        if (-not [string]::IsNullOrWhiteSpace($model)) {
            Write-Host "Heads up: '$model' is a multi-gigabyte download." -ForegroundColor Yellow
            if (Confirm-YesNo "Pull '$model' with Ollama now?") {
                try { & ollama pull $model } catch { Write-Warning "ollama pull failed: $_" }
            }
        }
    } else {
        Write-Host 'Ollama not found. The easiest local option is to install it from https://ollama.com/,'
        Write-Host 'then run: ollama pull llama3.1:8b'
    }
    if (Test-Cmd 'llama-server') {
        Write-Host 'llama-server (llama.cpp) found on PATH — set its path and a GGUF model in Settings > Model.'
    } else {
        Write-Host 'For the llama.cpp path (recommended on NVIDIA): download a CUDA llama.cpp release and a'
        Write-Host 'GGUF model, then set both paths in the app under Settings > Model. See'
        Write-Host 'docs/installation-automation.md for the planned guided flow.'
    }
}

# --- 5. Launch ---------------------------------------------------------------
Write-Head '5. Launch'
$startArgs = @('-addr', "127.0.0.1:$Port")
if ($DataDir) { $startArgs += @('-data-dir', $DataDir) }
$url = "http://127.0.0.1:$Port"

if (Confirm-YesNo 'Create a Start-MagicHandy.ps1 launcher in this folder?') {
    $argLine = ($startArgs | ForEach-Object { if ($_ -match '\s') { '"' + $_ + '"' } else { $_ } }) -join ' '
    $launcher = @"
# Starts MagicHandy and opens it in your browser.
Start-Process -FilePath "$exe" -ArgumentList $argLine
Start-Sleep -Seconds 1
Start-Process "$url"
"@
    Set-Content -Path (Join-Path $Repo 'Start-MagicHandy.ps1') -Value $launcher -Encoding utf8
    Write-Host 'Created Start-MagicHandy.ps1' -ForegroundColor Green
}

if (-not $NoLaunch -and (Confirm-YesNo 'Start MagicHandy now?')) {
    Start-Process -FilePath $exe -ArgumentList $startArgs
    Start-Sleep -Seconds 1
    Start-Process $url
    Write-Host "MagicHandy is starting at $url" -ForegroundColor Green
}

Write-Host ''
Write-Host 'All set. Open Settings to connect your Handy and pick your model.' -ForegroundColor White
Write-Host 'Emergency Stop is always on-screen (or press Esc).' -ForegroundColor DarkGray
