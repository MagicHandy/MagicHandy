# Seeds Clarissa persona and Synsual motion mode via the running MagicHandy API.
$ErrorActionPreference = "Stop"

$root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$port = if ($env:MAGICHANDY_PORT) { [int]$env:MAGICHANDY_PORT.Trim() } else { 49717 }
$bindHost = if ($env:MAGICHANDY_HOST) { $env:MAGICHANDY_HOST.Trim() } else { "127.0.0.1" }
$openHost = if ($bindHost -in @("0.0.0.0", "::")) { "127.0.0.1" } else { $bindHost }
$baseUrl = "http://${openHost}:${port}/api"

function Invoke-MagicHandyApi {
    param(
        [string]$Method,
        [string]$Path,
        [object]$Body = $null
    )
    $uri = "$baseUrl$Path"
    $params = @{
        Uri         = $uri
        Method      = $Method
        ContentType = "application/json"
        Headers     = @{
            "X-MagicHandy-UI"        = "lso"
            "X-MagicHandy-Client-ID" = "seed-clarissa"
        }
    }
    if ($null -ne $Body) {
        $params.Body = ($Body | ConvertTo-Json -Depth 12 -Compress)
    }
    return Invoke-RestMethod @params
}

try {
    Invoke-RestMethod -Uri ($baseUrl.Replace("/api", "") + "/healthz") -TimeoutSec 3 | Out-Null
} catch {
    throw "MagicHandy is not running at $baseUrl — start it with scripts/start_stack.ps1 first"
}

$settingsBody = @{
    updates = @{
        motion = @{
            motion_generation_mode = "synsual"
        }
        llm = @{
            prompt_set = "clarissa_synsual_v1"
        }
    }
}
Invoke-MagicHandyApi -Method PUT -Path "/settings" -Body $settingsBody | Out-Null
Write-Host "Settings updated: motion_generation_mode=synsual, prompt_set=clarissa_synsual_v1"

$personas = Invoke-MagicHandyApi -Method GET -Path "/personas"
$clarissa = $personas.personas | Where-Object { $_.id -eq "persona_clarissa_synsual" -or $_.name -eq "Clarissa" } | Select-Object -First 1
if (-not $clarissa) {
    Write-Warning "Clarissa persona not found — restart MagicHandy to run DB migration v8, then re-run this script."
} else {
  Write-Host "Clarissa persona present: $($clarissa.id)"
  Invoke-MagicHandyApi -Method POST -Path "/personas/$($clarissa.id)/activate" | Out-Null
  Write-Host "Activated persona: $($clarissa.name)"
}

Write-Host ""
Write-Host "Done. Open Config > Personas to edit Clarissa or Config > Full JSON to verify settings."
