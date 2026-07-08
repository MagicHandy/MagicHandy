# Run Go tests with cache/temp inside .cache (see kaspersky-go-dev.ps1).
$ErrorActionPreference = "Stop"
$root = Split-Path $PSScriptRoot -Parent
. (Join-Path $PSScriptRoot "kaspersky-go-dev.ps1") -EnvOnly

Push-Location $root
try {
    if ($args.Count -gt 0) {
        go test @args
    } else {
        go test ./...
    }
} finally {
    Pop-Location
}
