#Requires -Version 5.1

[CmdletBinding()]
param(
    [ValidateNotNullOrEmpty()]
    [string]$BaseUrl = 'http://127.0.0.1:49717',

    [ValidateRange(5, 300)]
    [int]$TimeoutSeconds = 120
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$reviewBaseUrl = $BaseUrl.TrimEnd('/')

function Get-OptionalProperty {
    param(
        [Parameter(Mandatory = $true)]
        [object]$InputObject,

        [Parameter(Mandatory = $true)]
        [string]$Name
    )

    $property = $InputObject.PSObject.Properties[$Name]
    if ($null -eq $property) {
        return $null
    }
    return $property.Value
}

function Assert-ReviewCondition {
    param(
        [Parameter(Mandatory = $true)]
        [bool]$Condition,

        [Parameter(Mandatory = $true)]
        [string]$Message
    )

    if (-not $Condition) {
        throw $Message
    }
}

try {
    $health = Invoke-RestMethod -Method Get -Uri "$reviewBaseUrl/healthz" -TimeoutSec 10
    Assert-ReviewCondition ([string](Get-OptionalProperty $health 'status') -eq 'ok') `
        "The review app at '$reviewBaseUrl' is not healthy."

    $status = Invoke-RestMethod -Method Get -Uri "$reviewBaseUrl/api/llm/status" -TimeoutSec 15
    $provider = [string](Get-OptionalProperty $status 'provider')
    $providerBaseUrl = ([string](Get-OptionalProperty $status 'base_url')).TrimEnd('/')
    $model = [string](Get-OptionalProperty $status 'model')
    $message = [string](Get-OptionalProperty $status 'message')

    Assert-ReviewCondition ([bool](Get-OptionalProperty $status 'available')) `
        "The review LLM provider '$provider' is unavailable: $message"
    Assert-ReviewCondition ([bool](Get-OptionalProperty $status 'model_available')) `
        "The configured review model '$model' is unavailable: $message"
    Assert-ReviewCondition ([bool](Get-OptionalProperty $status 'loaded')) `
        "The configured review model '$model' is not loaded: $message"
    Assert-ReviewCondition (-not [string]::IsNullOrWhiteSpace($providerBaseUrl)) `
        'The review app did not report an LLM base URL.'
    Assert-ReviewCondition (-not [string]::IsNullOrWhiteSpace($model)) `
        'The review app did not report a selected LLM model.'

    $completionUrl = if ($providerBaseUrl.EndsWith('/v1', [System.StringComparison]::OrdinalIgnoreCase)) {
        "$providerBaseUrl/chat/completions"
    } else {
        "$providerBaseUrl/v1/chat/completions"
    }
    $completionRequest = @{
        model = $model
        messages = @(
            @{
                role = 'system'
                content = 'Reply with one short plain-text sentence confirming readiness.'
            },
            @{
                role = 'user'
                content = 'Run the review readiness check now.'
            }
        )
        max_tokens = 64
        temperature = 0
        stream = $false
    }
    # Match MagicHandy's llama.cpp provider when saved reasoning is off. Some
    # thinking-capable local templates otherwise spend this deliberately small
    # readiness budget entirely in reasoning_content and return empty visible
    # content even though ordinary app generation is healthy.
    if ($provider -eq 'llama_cpp') {
        $completionRequest['chat_template_kwargs'] = @{ enable_thinking = $false }
    }
    $completionBody = $completionRequest | ConvertTo-Json -Depth 6 -Compress

    # This is a direct provider generation probe. It does not acquire a
    # controller lease, enter MagicHandy chat, or touch the motion engine.
    $completion = Invoke-RestMethod -Method Post -Uri $completionUrl `
        -ContentType 'application/json' -Body $completionBody -TimeoutSec $TimeoutSeconds
    $choices = @(Get-OptionalProperty $completion 'choices')
    Assert-ReviewCondition ($choices.Count -gt 0 -and $null -ne $choices[0]) `
        "The configured review model '$model' returned no completion choice."
    $completionMessage = Get-OptionalProperty $choices[0] 'message'
    Assert-ReviewCondition ($null -ne $completionMessage) `
        "The configured review model '$model' returned no completion message."
    $reply = [string](Get-OptionalProperty $completionMessage 'content')
    Assert-ReviewCondition (-not [string]::IsNullOrWhiteSpace($reply)) `
        "The configured review model '$model' returned an empty completion."

    [pscustomobject]@{
        ready = $true
        app_url = $reviewBaseUrl
        provider = $provider
        model = $model
        reply = $reply.Trim()
    } | ConvertTo-Json -Depth 4
} catch {
    Write-Error "Review LLM readiness failed: $($_.Exception.Message)"
    exit 1
}
