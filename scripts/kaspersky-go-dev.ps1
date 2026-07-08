# Redirects Go build/test temp files into the repo and opens Kaspersky so you
# can add a single folder exclusion (consumer Kaspersky has no CLI for this).
param(
    [switch]$EnvOnly
)

$ErrorActionPreference = "Stop"
$root = Split-Path $PSScriptRoot -Parent
$cacheRoot = Join-Path $root ".cache"
$goTmp = Join-Path $cacheRoot "go-tmp"
$goBuild = Join-Path $cacheRoot "go-build"

New-Item -ItemType Directory -Force -Path $goTmp, $goBuild | Out-Null

$env:GOTMPDIR = $goTmp
$env:GOCACHE = $goBuild

Write-Host ""
Write-Host "Go dev cache (use ONE Kaspersky exclusion for the whole repo):" -ForegroundColor Cyan
Write-Host "  $root"
Write-Host ""
Write-Host "GOTMPDIR = $goTmp"
Write-Host "GOCACHE  = $goBuild"
Write-Host ""

if ($EnvOnly) {
    return
}

$kasperskyUi = "${env:ProgramFiles(x86)}\Kaspersky Lab\Kaspersky 21.25\avpui.exe"
$message = @"
O Kaspersky nao deixa o Cursor alterar exclusoes automaticamente.

Adicione esta pasta nas exclusoes:
$root

No Kaspersky:
Configuracoes (engrenagem) -> Seguranca -> Ameacas e exclusoes -> Gerenciar exclusoes -> Adicionar -> Pasta

Marque todos os componentes de protecao e confirme.
Depois rode: go clean -testcache
"@

Add-Type -AssemblyName System.Windows.Forms
[System.Windows.Forms.MessageBox]::Show(
    $message,
    "MagicHandy - exclusao Kaspersky",
    [System.Windows.Forms.MessageBoxButtons]::OK,
    [System.Windows.Forms.MessageBoxIcon]::Information
) | Out-Null

Start-Process explorer.exe -ArgumentList $root
if (Test-Path $kasperskyUi) {
    Start-Process $kasperskyUi
} else {
    Write-Warning "Kaspersky UI not found at $kasperskyUi"
}
