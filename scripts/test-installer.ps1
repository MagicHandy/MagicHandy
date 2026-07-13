#Requires -Version 5.1
[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$Repo = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$support = Join-Path $Repo 'scripts\installer\InstallerSupport.psm1'
Import-Module $support -Force -DisableNameChecking

function Assert-True([bool]$Condition, [string]$Message) {
    if (-not $Condition) {
        throw "Assertion failed: $Message"
    }
}

function Assert-Equal($Expected, $Actual, [string]$Message) {
    if ($Expected -ne $Actual) {
        throw "Assertion failed: $Message. Expected '$Expected', got '$Actual'."
    }
}

function Assert-PlanContains([string[]]$Plan, [string]$Pattern) {
    Assert-True -Condition ([bool]($Plan | Where-Object { $_ -match $Pattern })) -Message "plan should contain /$Pattern/"
}

function Assert-PlanExcludes([string[]]$Plan, [string]$Pattern) {
    Assert-True -Condition (-not [bool]($Plan | Where-Object { $_ -match $Pattern })) -Message "plan should exclude /$Pattern/"
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("magichandy-installer-test-" + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
try {
    Write-Host 'Checking PowerShell 5.1 syntax...'
    $files = @(
        'install.ps1',
        'update.ps1',
        'scripts\installer\InstallerSupport.psm1',
        'internal\llm\runtimeassets\build-managed-llama.ps1'
    )
    foreach ($file in $files) {
        $tokens = $null
        $errors = $null
        $path = Join-Path $Repo $file
        [System.Management.Automation.Language.Parser]::ParseFile($path, [ref]$tokens, [ref]$errors) | Out-Null
        Assert-Equal -Expected 0 -Actual $errors.Count -Message "$file should parse"
    }

    Write-Host 'Checking installer branding and completion art...'
    $installBanner = Write-MagicHandyBanner -Operation Install 6>&1 | Out-String
    Assert-True -Condition ($installBanner -match 'INSTALL - local-first AI control for The Handy') -Message 'install banner should identify the product and operation'
    Assert-True -Condition ($installBanner -match 'Emergency Stop') -Message 'install banner should retain the safety reminder'
    $updateBanner = Write-MagicHandyBanner -Operation Update 6>&1 | Out-String
    Assert-True -Condition ($updateBanner -match 'UPDATE - local-first AI control for The Handy') -Message 'update banner should identify the product and operation'
    $installCompletion = Write-MagicHandyCompletionArt -Operation Install 6>&1 | Out-String
    Assert-True -Condition ($installCompletion -match 'INSTALL COMPLETE') -Message 'install completion should identify the finished operation'
    Assert-True -Condition ($installCompletion -match 'BUILD VERIFIED - READY') -Message 'install completion should report a verified build'
    Assert-True -Condition ($installCompletion -match 'Congratulations.+select a model, voice provider, and device transport') -Message 'install completion should give relevant next steps'
    Assert-True -Condition ($installCompletion -match '\|\|=+\[\]') -Message 'completion should include the Handy motion-rail text art'
    $updateCompletion = Write-MagicHandyCompletionArt -Operation Update 6>&1 | Out-String
    Assert-True -Condition ($updateCompletion -match 'Congratulations.+Saved installation choices were reapplied') -Message 'update completion should confirm preserved choices'
    $planCompletion = Write-MagicHandyCompletionArt -Operation UpdatePlan 6>&1 | Out-String
    Assert-True -Condition ($planCompletion -match 'NO CHANGES MADE') -Message 'plan completion should not claim that a build ran'

    Write-Host 'Checking installer-state round trip and data hygiene...'
    $statePath = Join-Path $tempRoot 'install-state.json'
    $dataDir = Join-Path $tempRoot 'data'
    $state = New-MagicHandyInstallState `
        -RepositoryPath $Repo `
        -DataDir $dataDir `
        -Port 49800 `
        -SetupLLM $true `
        -BuildManagedLlama $true `
        -LlamaBackend 'cuda' `
        -EnsureOllama $true `
        -OllamaModel 'example/model:latest' `
        -InstallParakeet $true `
        -CreateLauncher $true
    Write-MagicHandyInstallState -State $state -Path $statePath
    $loaded = Read-MagicHandyInstallState -Path $statePath
    Assert-Equal -Expected 1 -Actual ([int]$loaded.schema_version) -Message 'state schema'
    Assert-Equal -Expected 49800 -Actual ([int]$loaded.port) -Message 'saved port'
    Assert-Equal -Expected 'cuda' -Actual ([string]$loaded.llama_backend) -Message 'saved backend'
    Assert-True -Condition ([bool]$loaded.install_parakeet) -Message 'saved Parakeet choice'
    $json = Get-Content -LiteralPath $statePath -Raw
    Assert-True -Condition ($json -notmatch '(?i)api.?key|connection.?key|password|secret') -Message 'state must not define secret fields'
    Assert-True -Condition (-not (Test-Path -LiteralPath "$statePath.partial-$PID")) -Message 'state write must be atomic'

    Write-Host 'Checking generated launcher quoting and syntax...'
    $launcherRoot = Join-Path $tempRoot "launcher root's copy"
    $launcherData = Join-Path $tempRoot "data root's copy"
    New-Item -ItemType Directory -Force -Path $launcherRoot | Out-Null
    $supportModule = Get-Module InstallerSupport
    & $supportModule {
        param($RepositoryPath, $DataDir)
        Write-MagicHandyLauncher -RepositoryPath $RepositoryPath -DataDir $DataDir -Port 49900
    } $launcherRoot $launcherData
    $launcherPath = Join-Path $launcherRoot 'Start-MagicHandy.ps1'
    $tokens = $null
    $errors = $null
    [System.Management.Automation.Language.Parser]::ParseFile($launcherPath, [ref]$tokens, [ref]$errors) | Out-Null
    Assert-Equal -Expected 0 -Actual $errors.Count -Message 'generated launcher should parse'
    $launcherText = Get-Content -LiteralPath $launcherPath -Raw
    Assert-True -Condition ($launcherText -match '-WindowStyle Hidden') -Message 'launcher should hide the background app window'
    Assert-True -Condition ($launcherText -match "root''s copy") -Message 'launcher should escape apostrophes in paths'

    Write-Host 'Checking selected-component plans...'
    $managedPlan = @(Get-MagicHandyProvisionPlan -State $loaded)
    Assert-PlanContains -Plan $managedPlan -Pattern 'Go 1\.25'
    Assert-PlanContains -Plan $managedPlan -Pattern 'Visual Studio C\+\+'
    Assert-PlanContains -Plan $managedPlan -Pattern 'CUDA Toolkit'
    Assert-PlanContains -Plan $managedPlan -Pattern 'Parakeet CPU runner'
    Assert-PlanContains -Plan $managedPlan -Pattern 'NeuTTS Air'

    $ollamaState = New-MagicHandyInstallState `
        -RepositoryPath $Repo `
        -DataDir $dataDir `
        -Port 49717 `
        -SetupLLM $true `
        -BuildManagedLlama $false `
        -LlamaBackend 'cpu' `
        -EnsureOllama $true `
        -InstallParakeet $false `
        -CreateLauncher $false
    $ollamaPlan = @(Get-MagicHandyProvisionPlan -State $ollamaState)
    Assert-PlanContains -Plan $ollamaPlan -Pattern 'Ensure Ollama'
    Assert-PlanExcludes -Plan $ollamaPlan -Pattern 'CMake|Visual Studio|CUDA|Parakeet CPU runner'

    Write-Host 'Checking updater fast-forward and dirty-worktree refusal...'
    $git = Resolve-MagicHandyExecutable -Name 'git'
    Assert-True -Condition (-not [string]::IsNullOrWhiteSpace($git)) -Message 'Git is required by the Windows CI image'
    $remote = Join-Path $tempRoot 'remote.git'
    $seed = Join-Path $tempRoot 'seed'
    $checkout = Join-Path $tempRoot 'checkout'
    & $git init --bare $remote | Out-Null
    & $git init $seed | Out-Null
    & $git -C $seed config user.email 'installer-test@magichandy.invalid'
    & $git -C $seed config user.name 'MagicHandy Installer Test'
    [System.IO.File]::WriteAllText((Join-Path $seed 'version.txt'), 'v1')
    & $git -C $seed add version.txt
    & $git -C $seed commit -m 'initial' | Out-Null
    & $git -C $seed remote add origin $remote
    & $git -C $seed push -u origin HEAD | Out-Null
    & $git clone $remote $checkout | Out-Null
    [System.IO.File]::WriteAllText((Join-Path $seed 'version.txt'), 'v2')
    & $git -C $seed add version.txt
    & $git -C $seed commit -m 'update' | Out-Null
    & $git -C $seed push | Out-Null
    Update-MagicHandySource -RepositoryPath $checkout -AssumeYes
    Assert-Equal -Expected 'v2' -Actual (Get-Content -LiteralPath (Join-Path $checkout 'version.txt') -Raw) -Message 'updater should fast-forward clean checkout'
    [System.IO.File]::WriteAllText((Join-Path $checkout 'version.txt'), 'dirty')
    $dirtyRejected = $false
    try {
        Update-MagicHandySource -RepositoryPath $checkout -AssumeYes
    } catch {
        $dirtyRejected = $_.Exception.Message -match 'local changes'
    }
    Assert-True -Condition $dirtyRejected -Message 'updater should reject a dirty checkout'

    Write-Host 'Checking install.ps1 plan-only behavior...'
    $freshPlanState = Join-Path $tempRoot 'fresh-plan-state.json'
    & (Join-Path $Repo 'install.ps1') `
        -Yes `
        -SkipLlamaBuild `
        -SkipParakeet `
        -NoLauncher `
        -NoLaunch `
        -PlanOnly `
        -StatePath $freshPlanState | Out-Host
    Assert-True -Condition (-not (Test-Path -LiteralPath $freshPlanState)) -Message 'install plan must not persist state'

    Write-Host 'Checking update.ps1 preserved-choice plan...'
    $beforeHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $statePath).Hash
    & (Join-Path $Repo 'update.ps1') `
        -Yes `
        -NoPull `
        -NoLaunch `
        -PlanOnly `
        -StatePath $statePath | Out-Host
    $afterHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $statePath).Hash
    Assert-Equal -Expected $beforeHash -Actual $afterHash -Message 'update plan must not rewrite saved choices'

    Write-Host 'Checking updater runtime reconfiguration prompt...'
    $global:MagicHandyInstallerResponses = New-Object System.Collections.Generic.Queue[string]
    $global:MagicHandyInstallerPrompts = New-Object System.Collections.Generic.List[string]
    foreach ($response in @('y', '', '', 'y', 'n', 'y', '-', 'n', 'y')) {
        $global:MagicHandyInstallerResponses.Enqueue($response)
    }
    function global:Read-Host {
        param([string]$Prompt)
        $global:MagicHandyInstallerPrompts.Add($Prompt)
        if ($global:MagicHandyInstallerResponses.Count -eq 0) {
            throw "No test response remains for prompt '$Prompt'."
        }
        return $global:MagicHandyInstallerResponses.Dequeue()
    }
    try {
        $reconfigureOutput = (& (Join-Path $Repo 'update.ps1') `
            -NoPull `
            -NoLaunch `
            -PlanOnly `
            -StatePath $statePath 6>&1 | Out-String)
    } finally {
        $remainingResponses = $global:MagicHandyInstallerResponses.Count
        $capturedPrompts = @($global:MagicHandyInstallerPrompts)
        Remove-Item Function:\global:Read-Host -ErrorAction SilentlyContinue
        Remove-Variable MagicHandyInstallerResponses -Scope Global -ErrorAction SilentlyContinue
        Remove-Variable MagicHandyInstallerPrompts -Scope Global -ErrorAction SilentlyContinue
    }
    Assert-True -Condition (($capturedPrompts -join "`n") -match 'Modify previous installation choices') -Message 'updater should ask whether to modify choices'
    Assert-True -Condition ($reconfigureOutput -match 'Managed llama\.cpp: no') -Message 'reconfiguration should switch managed llama.cpp off'
    Assert-True -Condition ($reconfigureOutput -match 'Ollama model:\s+\(unchanged\)') -Message 'reconfiguration should clear the optional model'
    Assert-PlanExcludes -Plan @($reconfigureOutput -split "`r?`n") -Pattern 'Ensure Git and CMake|CUDA Toolkit'
    Assert-Equal -Expected $beforeHash -Actual ((Get-FileHash -Algorithm SHA256 -LiteralPath $statePath).Hash) -Message 'reconfiguration plan must not rewrite state'
    Assert-Equal -Expected 0 -Actual $remainingResponses -Message 'all expected prompts should be consumed'

    Write-Host 'Installer tests passed.' -ForegroundColor Green
} finally {
    $resolvedTemp = [System.IO.Path]::GetFullPath([System.IO.Path]::GetTempPath()).TrimEnd('\') + '\'
    $resolvedRoot = [System.IO.Path]::GetFullPath($tempRoot)
    if ($resolvedRoot.StartsWith($resolvedTemp, [StringComparison]::OrdinalIgnoreCase) -and (Test-Path -LiteralPath $resolvedRoot)) {
        Remove-Item -LiteralPath $resolvedRoot -Recurse -Force
    }
}
