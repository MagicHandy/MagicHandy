# Motion calibration battery: disables safety lock and runs unit + device tests.
$ErrorActionPreference = "Stop"
$root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$url = "http://127.0.0.1:49717"
$logFile = Join-Path (Split-Path $root -Parent) "debug-d9c091.log"
$clientId = "calibrate-battery-" + [guid]::NewGuid().ToString("N").Substring(0, 8)

if (Test-Path $logFile) { Remove-Item $logFile -Force }

function Invoke-MagicHandy {
    param([string]$Method, [string]$Path, [string]$Body = $null)
    $params = @{
        Uri = $url + $Path
        Method = $Method
        Headers = @{
            "Content-Type" = "application/json"
            "X-MagicHandy-UI" = "lso"
            "X-MagicHandy-Client-ID" = $clientId
        }
        TimeoutSec = 15
    }
    if ($Body) { $params.Body = $Body }
    return Invoke-RestMethod @params
}

Write-Host "Claiming controller as $clientId..."
try {
    Invoke-MagicHandy -Method GET -Path "/api/controller" | Out-Null
} catch {
    Write-Warning "MagicHandy not reachable at $url — device tests may skip if Handy offline"
}

Write-Host "Disabling hardware_safety_lock..."
$body = @{
    updates = @{
        motion = @{
            hardware_safety_lock = $false
        }
    }
} | ConvertTo-Json -Depth 5
try {
    Invoke-MagicHandy -Method PUT -Path "/api/settings" -Body $body | Out-Null
    Write-Host "hardware_safety_lock=false saved"
} catch {
    Write-Warning "Could not update settings (close other browser tabs or restart stack): $($_.Exception.Message)"
}

Push-Location $root
Write-Host "Unit calibration matrix (all regions x tipos x velocities)..."
& go test ./internal/motion -run TestMotionCalibrateBattery -count=1 -timeout 120s
if ($LASTEXITCODE -ne 0) { Pop-Location; exit $LASTEXITCODE }

Write-Host "Device calibration battery (gentle/balanced/intense + sweeps)..."
& go test -tags=integration ./internal/httpapi -run TestMotionCalibrateBatteryDevice -count=1 -timeout 600s -v
$deviceCode = $LASTEXITCODE
Pop-Location

if (Test-Path $logFile) {
    Write-Host "`nDebug log: $logFile"
    Get-Content $logFile -Tail 15
}

exit $deviceCode
