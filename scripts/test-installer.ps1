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

function Get-AvailableLoopbackPort {
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
    try {
        $listener.Start()
        return ([System.Net.IPEndPoint]$listener.LocalEndpoint).Port
    } finally {
        $listener.Stop()
    }
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

    Write-Host 'Checking same-process CUDA environment initialization...'
    $builderPath = Join-Path $Repo 'internal\llm\runtimeassets\build-managed-llama.ps1'
    $builderTokens = $null
    $builderErrors = $null
    $builderAst = [System.Management.Automation.Language.Parser]::ParseFile($builderPath, [ref]$builderTokens, [ref]$builderErrors)
    $initializerAst = $builderAst.Find({
        $args[0] -is [System.Management.Automation.Language.FunctionDefinitionAst] -and
            $args[0].Name -eq 'Initialize-CudaToolkitEnvironment'
    }, $true)
    Assert-True -Condition ($null -ne $initializerAst) -Message 'managed llama.cpp builder should define CUDA environment initialization'
    Invoke-Expression $initializerAst.Extent.Text

    $fakeToolkit = Join-Path $tempRoot 'CUDA\v99.1'
    $fakeNvcc = Join-Path $fakeToolkit 'bin\nvcc.exe'
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $fakeNvcc) | Out-Null
    [System.IO.File]::WriteAllText($fakeNvcc, '')
    $originalCUDAPath = [Environment]::GetEnvironmentVariable('CUDA_PATH', 'Process')
    $originalCudaToolkitDir = [Environment]::GetEnvironmentVariable('CudaToolkitDir', 'Process')
    try {
        $env:CUDA_PATH = 'stale'
        $env:CudaToolkitDir = ''
        Initialize-CudaToolkitEnvironment -Nvcc $fakeNvcc
        Assert-Equal -Expected $fakeToolkit -Actual $env:CUDA_PATH -Message 'CUDA_PATH should use the resolved nvcc toolkit root'
        Assert-Equal -Expected "$fakeToolkit\" -Actual $env:CudaToolkitDir -Message 'CudaToolkitDir should include the trailing separator required by MSBuild'
        $childEnvironment = & powershell.exe -NoProfile -Command '[Console]::Write($env:CUDA_PATH + [char]124 + $env:CudaToolkitDir)'
        Assert-Equal -Expected "$fakeToolkit|$fakeToolkit\" -Actual $childEnvironment -Message 'CUDA environment should reach child build processes'
    } finally {
        [Environment]::SetEnvironmentVariable('CUDA_PATH', $originalCUDAPath, 'Process')
        [Environment]::SetEnvironmentVariable('CudaToolkitDir', $originalCudaToolkitDir, 'Process')
    }

    Write-Host 'Checking installer branding and completion art...'
    $installBanner = Write-MagicHandyBanner -Operation Install 6>&1 | Out-String
    Assert-True -Condition ($installBanner -match 'INSTALL - local-first AI control for The Handy') -Message 'install banner should identify the product and operation'
    Assert-True -Condition ($installBanner -match 'Emergency Stop') -Message 'install banner should retain the safety reminder'
    $updateBanner = Write-MagicHandyBanner -Operation Update 6>&1 | Out-String
    Assert-True -Condition ($updateBanner -match 'UPDATE - local-first AI control for The Handy') -Message 'update banner should identify the product and operation'
    $installCompletion = Write-MagicHandyCompletionArt -Operation Install 6>&1 | Out-String
    Assert-True -Condition ($installCompletion -match 'INSTALL COMPLETE') -Message 'install completion should identify the finished operation'
    Assert-True -Condition ($installCompletion -match 'APP BUILD VERIFIED - CONFIGURATION REQUIRED') -Message 'install completion should distinguish a verified build from configured providers'
    Assert-True -Condition ($installCompletion -match 'Open Settings.+select a model, voice provider, and device transport') -Message 'install completion should give relevant next steps'
    Assert-True -Condition ($installCompletion -match 'Managed NeuTTS still needs reference codes\s+and their exact transcript') -Message 'install completion should disclose the remaining NeuTTS reference boundary'
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

    Write-Host 'Checking pinned NeuTTS Cargo lock correction...'
    $supportModule = Get-Module InstallerSupport
    $lockSource = Join-Path $tempRoot 'neutts-lock-source'
    New-Item -ItemType Directory -Force -Path $lockSource | Out-Null
    $lockPath = Join-Path $lockSource 'Cargo.lock'
    $lockFixture = @'
version = 4

[[package]]
name = "neutts"
version = "0.1.0"
dependencies = [
 "fixture",
]

[[package]]
name = "fixture"
version = "0.1.0"
'@
    [System.IO.File]::WriteAllText($lockPath, $lockFixture)
    & $supportModule { param($SourceRoot) Repair-MagicHandyNeuTTSCargoLock -SourceRoot $SourceRoot } $lockSource
    $correctedLock = [System.IO.File]::ReadAllText($lockPath)
    Assert-True -Condition ($correctedLock -match '(?m)^name = "neutts"\r?\nversion = "0\.1\.1"\r?$') -Message 'known upstream root package version should be corrected'
    Assert-True -Condition ($correctedLock -match '(?m)^name = "fixture"\r?\nversion = "0\.1\.0"\r?$') -Message 'dependency versions should remain unchanged'

    $unexpectedLockSource = Join-Path $tempRoot 'unexpected-neutts-lock-source'
    New-Item -ItemType Directory -Force -Path $unexpectedLockSource | Out-Null
    $unexpectedLockPath = Join-Path $unexpectedLockSource 'Cargo.lock'
    [System.IO.File]::WriteAllText($unexpectedLockPath, ($lockFixture -replace 'version = "0\.1\.0"', 'version = "0.1.2"'))
    $unexpectedLockRejected = $false
    try {
        & $supportModule { param($SourceRoot) Repair-MagicHandyNeuTTSCargoLock -SourceRoot $SourceRoot } $unexpectedLockSource
    } catch {
        $unexpectedLockRejected = $true
    }
    Assert-True -Condition $unexpectedLockRejected -Message 'unexpected upstream lock content should fail closed'

    Write-Host 'Checking native executable probe classification...'
    $probeExecutable = (Get-Process -Id $PID).Path
    $probeResults = & $supportModule {
        param($ProbeExecutable, $MissingProbe)
        $ErrorActionPreference = 'Stop'
        [pscustomobject]@{
            StderrSuccess = Test-MagicHandyNativeProbe -Executable $ProbeExecutable -ArgumentList @('-NoProfile', '-Command', '[Console]::Error.WriteLine(''usage''); exit 0')
            Nonzero = Test-MagicHandyNativeProbe -Executable $ProbeExecutable -ArgumentList @('-NoProfile', '-Command', 'exit 7')
            Missing = Test-MagicHandyNativeProbe -Executable $MissingProbe
            RestoredErrorAction = $ErrorActionPreference
        }
    } $probeExecutable (Join-Path $tempRoot 'missing-probe.exe')
    Assert-True -Condition ([bool]$probeResults.StderrSuccess) -Message 'stderr with exit zero should pass a native probe'
    Assert-True -Condition (-not [bool]$probeResults.Nonzero) -Message 'nonzero exit should fail a native probe'
    Assert-True -Condition (-not [bool]$probeResults.Missing) -Message 'an executable launch failure should fail closed'
    Assert-Equal -Expected 'Stop' -Actual ([string]$probeResults.RestoredErrorAction) -Message 'native probe should restore ErrorActionPreference'

    Write-Host 'Checking app-managed NeuTTS runtime manifest discovery...'
    $neuttsData = Join-Path $tempRoot 'neutts-data'
    $neuttsRuntimeResult = & $supportModule {
        param($DataDir)
        $root = Join-Path $DataDir 'voice\neutts\active'
        $runtime = Join-Path $root 'runtime'
        $runner = Join-Path $runtime 'stream_pcm.exe'
        $decoder = Join-Path $runtime 'models\neucodec_decoder.safetensors'
        $gguf = Join-Path $root "hf\hub\models--neuphonic--neutts-air-q4-gguf\snapshots\$script:NeuTTSBackboneRevision\neutts-air-Q4_0.gguf"
        foreach ($path in @($runner, $decoder, $gguf)) {
            New-Item -ItemType Directory -Force -Path (Split-Path -Parent $path) | Out-Null
            [System.IO.File]::WriteAllText($path, 'fixture')
        }
        $backboneRef = Join-Path $root 'hf\hub\models--neuphonic--neutts-air-q4-gguf\refs\main'
        New-Item -ItemType Directory -Force -Path (Split-Path -Parent $backboneRef) | Out-Null
        [System.IO.File]::WriteAllText($backboneRef, $script:NeuTTSBackboneRevision)
        $fixtureBackboneHash = Get-MagicHandySHA256 -Path $gguf
        $originalBackboneHash = $script:NeuTTSBackboneSHA256
        $script:NeuTTSBackboneSHA256 = $fixtureBackboneHash
        $manifest = [pscustomobject]@{
            schema_version = 1
            source_commit = $script:NeuTTSSourceCommit
            rust_toolchain = $script:NeuTTSRustToolchain
            backbone_revision = $script:NeuTTSBackboneRevision
            backbone_sha256 = $fixtureBackboneHash
            codec_revision = $script:NeuTTSCodecRevision
            codec_checkpoint_sha256 = $script:NeuTTSCodecSHA256
            runner_sha256 = (Get-MagicHandySHA256 -Path $runner)
            decoder_sha256 = (Get-MagicHandySHA256 -Path $decoder)
        }
        [System.IO.File]::WriteAllText((Join-Path $runtime 'runtime.json'), ($manifest | ConvertTo-Json))
        try {
            $valid = Test-MagicHandyNeuTTSInstall -DataDir $DataDir
            [System.IO.File]::AppendAllText($runner, 'tampered')
            $tampered = Test-MagicHandyNeuTTSInstall -DataDir $DataDir
            [System.IO.File]::WriteAllText($runner, 'fixture')
            $malformedJSON = ($manifest | ConvertTo-Json) -replace '"schema_version":\s+1', '"schema_version":"invalid"'
            [System.IO.File]::WriteAllText((Join-Path $runtime 'runtime.json'), $malformedJSON)
            $malformed = Test-MagicHandyNeuTTSInstall -DataDir $DataDir
            [pscustomobject]@{ Valid = $valid; Tampered = $tampered; Malformed = $malformed }
        } finally {
            $script:NeuTTSBackboneSHA256 = $originalBackboneHash
        }
    } $neuttsData
    Assert-True -Condition ([bool]$neuttsRuntimeResult.Valid) -Message 'matching NeuTTS manifest and runtime files should be reusable'
    Assert-True -Condition (-not [bool]$neuttsRuntimeResult.Tampered) -Message 'changed NeuTTS runtime bytes should require repair'
    Assert-True -Condition (-not [bool]$neuttsRuntimeResult.Malformed) -Message 'a malformed NeuTTS manifest should request repair rather than abort setup'

    Write-Host 'Checking interrupted NeuTTS swap recovery...'
    $recoveryData = Join-Path $tempRoot 'neutts-recovery-data'
    $backupRuntime = Join-Path $recoveryData 'voice\neutts\active.backup-fixture\runtime'
    New-Item -ItemType Directory -Force -Path $backupRuntime | Out-Null
    [System.IO.File]::WriteAllText((Join-Path $backupRuntime 'marker.txt'), 'previous runtime')
    & $supportModule { param($DataDir) Restore-MagicHandyNeuTTSBackup -DataDir $DataDir } $recoveryData
    Assert-True -Condition (Test-Path -LiteralPath (Join-Path $recoveryData 'voice\neutts\active\runtime\marker.txt')) -Message 'an interrupted swap should restore its previous active runtime'

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
    Assert-True -Condition ($launcherText -match 'Start-MagicHandyApp') -Message 'launcher should reuse the verified app startup path'
    Assert-True -Condition ($launcherText -match 'InstallerSupport\.psm1') -Message 'launcher should import shared installer support'
    Assert-True -Condition ($launcherText -match "root''s copy") -Message 'launcher should escape apostrophes in paths'

    Write-Host 'Checking running app Stop and process-tree teardown before rebuild...'
    $runtimeRepo = Join-Path $tempRoot 'running-app-repo'
    $runtimeData = Join-Path $tempRoot 'running app data with spaces'
    $runtimePort = Get-AvailableLoopbackPort
    New-Item -ItemType Directory -Force -Path $runtimeRepo | Out-Null
    $runtimeExe = Join-Path $runtimeRepo 'magichandy.exe'
    $go = Resolve-MagicHandyExecutable -Name 'go'
    Assert-True -Condition (-not [string]::IsNullOrWhiteSpace($go)) -Message 'Go is required by the Windows CI image'
    $previousCGO = $env:CGO_ENABLED
    try {
        $env:CGO_ENABLED = '0'
        & $go -C $Repo build -o $runtimeExe ./cmd/magichandy
        Assert-Equal -Expected 0 -Actual $LASTEXITCODE -Message 'test app build should succeed'
    } finally {
        $env:CGO_ENABLED = $previousCGO
    }
    $runtimeArguments = & $supportModule {
        param($Address, $DataDir)
        New-MagicHandyAppArgumentLine -Address $Address -DataDir $DataDir
    } "127.0.0.1:$runtimePort" $runtimeData
    $runtimeProcess = Start-Process -FilePath $runtimeExe -ArgumentList $runtimeArguments -PassThru -WindowStyle Hidden
    try {
        $ready = $false
        $readyDeadline = [DateTime]::UtcNow.AddSeconds(10)
        do {
            try {
                Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:$runtimePort/api/state" -TimeoutSec 1 | Out-Null
                $ready = $true
                break
            } catch {
                Start-Sleep -Milliseconds 100
            }
        } while ([DateTime]::UtcNow -lt $readyDeadline)
        Assert-True -Condition $ready -Message 'test app should become ready'
        Assert-True -Condition (Test-Path -LiteralPath (Join-Path $runtimeData 'magichandy.db')) -Message 'quoted startup should keep the database under the intended spaced data path'

        $foreignRepo = Join-Path $tempRoot 'foreign-app-repo'
        New-Item -ItemType Directory -Force -Path $foreignRepo | Out-Null
        Copy-Item -LiteralPath $runtimeExe -Destination (Join-Path $foreignRepo 'magichandy.exe')
        $foreignRejected = $false
        try {
            & $supportModule {
                param($RepositoryPath, $Port)
                Stop-MagicHandyAppForRebuild -RepositoryPath $RepositoryPath -Port $Port
            } $foreignRepo $runtimePort
        } catch {
            $foreignRejected = $_.Exception.Message -match 'owned by another process'
        }
        Assert-True -Condition $foreignRejected -Message 'rebuild preparation should refuse a listener from another checkout'
        $runtimeProcess.Refresh()
        Assert-True -Condition (-not $runtimeProcess.HasExited) -Message 'foreign-checkout refusal must leave the running app alive'

        [System.IO.File]::WriteAllText("$runtimeExe~", 'stale build backup')
        & $supportModule {
            param($RepositoryPath, $Port)
            Stop-MagicHandyAppForRebuild -RepositoryPath $RepositoryPath -Port $Port -AllowPhysicalStopConfirmation -PhysicalStopConfirmation { 'STOPPED' }
        } $runtimeRepo $runtimePort
        $runtimeProcess.Refresh()
        Assert-True -Condition $runtimeProcess.HasExited -Message 'rebuild preparation should stop the running app tree'
        Assert-True -Condition (-not (Test-Path -LiteralPath "$runtimeExe~")) -Message 'rebuild preparation should remove stale Go executable backups'
    } finally {
        $runtimeProcess.Refresh()
        if (-not $runtimeProcess.HasExited) {
            & "$env:SystemRoot\System32\taskkill.exe" /PID $runtimeProcess.Id /T /F | Out-Null
        }
    }

    Write-Host 'Checking staged binary replacement and verified relaunch...'
    [System.IO.File]::WriteAllText($runtimeExe, 'stale executable sentinel')
    $staleHash = (Get-FileHash -LiteralPath $runtimeExe -Algorithm SHA256).Hash
    & $supportModule {
        param($RepositoryPath, $GoExecutable)
        Build-MagicHandyBinaries -RepositoryPath $RepositoryPath -GoExecutable $GoExecutable
    } $runtimeRepo $go
    $rebuiltHash = (Get-FileHash -LiteralPath $runtimeExe -Algorithm SHA256).Hash
    Assert-True -Condition ($rebuiltHash -ne $staleHash) -Message 'staged build should replace the stale executable only after a successful compile'
    Assert-True -Condition (-not [bool](Get-ChildItem -LiteralPath $runtimeRepo -Filter '*.partial-*' -File)) -Message 'successful staged build should leave no partial executable'
    Assert-True -Condition (-not (Test-Path -LiteralPath "$runtimeExe~")) -Message 'staged replacement should not create a Go executable backup'
    $relaunchPort = Get-AvailableLoopbackPort
    $relaunchData = Join-Path $tempRoot 'verified relaunch data with spaces'
    Start-MagicHandyApp -RepositoryPath $runtimeRepo -DataDir $relaunchData -Port $relaunchPort -NoBrowser
    try {
        $index = (Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:$relaunchPort/" -TimeoutSec 5).Content
        $asset = [regex]::Match($index, '/assets/[^"'']+\.js').Value
        $javascript = (Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:$relaunchPort$asset" -TimeoutSec 5).Content
        Assert-True -Condition ($javascript -match 'Maximum output') -Message 'verified relaunch should serve the current embedded UI'
    } finally {
        & $supportModule { param($RepositoryPath, $Port) Stop-MagicHandyAppForRebuild -RepositoryPath $RepositoryPath -Port $Port -AllowPhysicalStopConfirmation -PhysicalStopConfirmation { 'STOPPED' } } $runtimeRepo $relaunchPort
    }

    Write-Host 'Checking rebuild Stop response classification...'
    $stopErrorJSON = '{"available":true,"engine":{"running":false,"paused":false,"completing":false},"error":"Intiface connection is stale"}'
    $stopErrorRecord = [System.Management.Automation.ErrorRecord]::new(
        [System.Net.WebException]::new('The remote server returned an error: (502) Bad Gateway.'),
        'MagicHandyStop502',
        [System.Management.Automation.ErrorCategory]::InvalidOperation,
        $null
    )
    $stopErrorRecord.ErrorDetails = [System.Management.Automation.ErrorDetails]::new($stopErrorJSON)
    $parsedStopError = & $supportModule {
        param($ErrorRecord)
        ConvertFrom-MagicHandyStopErrorResponse -ErrorRecord $ErrorRecord
    } $stopErrorRecord
    Assert-Equal -Expected 'Intiface connection is stale' -Actual ([string]$parsedStopError.error) -Message '502 Stop JSON should remain available for classification'
    $emptyStopErrorRecord = [System.Management.Automation.ErrorRecord]::new(
        [System.Net.WebException]::new('connection failed'),
        'MagicHandyStopNoBody',
        [System.Management.Automation.ErrorCategory]::ConnectionError,
        $null
    )
    $emptyStopError = & $supportModule {
        param($ErrorRecord)
        ConvertFrom-MagicHandyStopErrorResponse -ErrorRecord $ErrorRecord
    } $emptyStopErrorRecord
    Assert-True -Condition ($null -eq $emptyStopError) -Message 'a missing HTTP error body should fail parsing without a strict-mode exception'

    & $supportModule {
        Assert-MagicHandyRebuildStopResponse -AllowPhysicalStopConfirmation -PhysicalStopConfirmation { 'STOPPED' } -Response ([pscustomobject]@{
            available = $false
            stopped = $true
            error = 'configured transport unavailable'
        })
    } 3>$null
    & $supportModule {
        Assert-MagicHandyRebuildStopResponse -AllowPhysicalStopConfirmation -PhysicalStopConfirmation { 'STOPPED' } -Response ([pscustomobject]@{
            available = $true
            stopped = $true
            transport_result = [pscustomobject]@{ ok = $false }
            error = 'Intiface connection is stale'
        })
    } 3>$null
    & $supportModule {
        Assert-MagicHandyRebuildStopResponse -Response ([pscustomobject]@{
            available = $true
            transport_result = [pscustomobject]@{ ok = $true }
        })
    }
    $legacyTransportFailureRejected = $false
    try {
        & $supportModule {
            Assert-MagicHandyRebuildStopResponse -Response ([pscustomobject]@{
                available = $true
                engine = [pscustomobject]@{
                    running = $false
                    paused = $false
                    completing = $false
                }
                error = 'older endpoint transport failure'
            })
        }
    } catch {
        $legacyTransportFailureRejected = $_.Exception.Message -match 'Physical Stop delivery was not confirmed'
    }
    Assert-True -Condition $legacyTransportFailureRejected -Message 'an older endpoint failure must require explicit physical-stop confirmation'
    $wrongConfirmationRejected = $false
    try {
        & $supportModule {
            Assert-MagicHandyRebuildStopResponse -AllowPhysicalStopConfirmation -PhysicalStopConfirmation { 'stopped' } -Response ([pscustomobject]@{
                available = $false
                stopped = $true
                error = 'configured transport unavailable'
            })
        }
    } catch {
        $wrongConfirmationRejected = $_.Exception.Message -match 'Physical Stop delivery was not confirmed'
    }
    Assert-True -Condition $wrongConfirmationRejected -Message 'physical-stop confirmation must require exact STOPPED text'
    $malformedStopRejected = $false
    try {
        & $supportModule {
            Assert-MagicHandyRebuildStopResponse -Response ([pscustomobject]@{ message = 'gateway failure' })
        }
    } catch {
        $malformedStopRejected = $_.Exception.Message -match 'missing required field'
    }
    Assert-True -Condition $malformedStopRejected -Message 'an unexpected JSON response must fail closed'
    $runningStopRejected = $false
    try {
        & $supportModule {
            Assert-MagicHandyRebuildStopResponse -Response ([pscustomobject]@{
                available = $true
                engine = [pscustomobject]@{
                    running = $true
                    paused = $false
                    completing = $false
                }
            })
        }
    } catch {
        $runningStopRejected = $_.Exception.Message -match 'did not confirm local stopped state'
    }
    Assert-True -Condition $runningStopRejected -Message 'a response that still reports running motion must fail closed'
    $invalidBooleanRejected = $false
    try {
        & $supportModule {
            Assert-MagicHandyRebuildStopResponse -Response ([pscustomobject]@{
                available = $true
                stopped = 'false'
            })
        }
    } catch {
        $invalidBooleanRejected = $_.Exception.Message -match 'not boolean'
    }
    Assert-True -Condition $invalidBooleanRejected -Message 'string boolean fields must fail closed'
    $nullErrorRejected = $false
    try {
        & $supportModule {
            Assert-MagicHandyRebuildStopResponse -Response ([pscustomobject]@{
                available = $true
                stopped = $true
                error = $null
            })
        }
    } catch {
        $nullErrorRejected = $_.Exception.Message -match 'was not text'
    }
    Assert-True -Condition $nullErrorRejected -Message 'null error fields must fail closed'
    $incompleteEngineRejected = $false
    try {
        & $supportModule {
            Assert-MagicHandyRebuildStopResponse -Response ([pscustomobject]@{
                available = $true
                engine = [pscustomobject]@{ running = $false }
            })
        }
    } catch {
        $incompleteEngineRejected = $_.Exception.Message -match 'missing or not boolean'
    }
    Assert-True -Condition $incompleteEngineRejected -Message 'incomplete engine state must fail closed'
    $httpErrorWithoutMessageRejected = $false
    try {
        & $supportModule {
            Assert-MagicHandyRebuildStopResponse -Response ([pscustomobject]@{
                available = $true
                stopped = $true
                _http_error = $true
            })
        }
    } catch {
        $httpErrorWithoutMessageRejected = $_.Exception.Message -match 'did not contain an error message'
    }
    Assert-True -Condition $httpErrorWithoutMessageRejected -Message 'an HTTP error without a backend error message must fail closed'
    $unavailableWithoutMessageRejected = $false
    try {
        & $supportModule {
            Assert-MagicHandyRebuildStopResponse -Response ([pscustomobject]@{
                available = $false
                stopped = $true
            })
        }
    } catch {
        $unavailableWithoutMessageRejected = $_.Exception.Message -match 'transport failure without an error message'
    }
    Assert-True -Condition $unavailableWithoutMessageRejected -Message 'unavailable transport without an error message must fail closed'
    $failedResultWithoutMessageRejected = $false
    try {
        & $supportModule {
            Assert-MagicHandyRebuildStopResponse -Response ([pscustomobject]@{
                available = $true
                stopped = $true
                transport_result = [pscustomobject]@{ ok = $false }
            })
        }
    } catch {
        $failedResultWithoutMessageRejected = $_.Exception.Message -match 'transport failure without an error message'
    }
    Assert-True -Condition $failedResultWithoutMessageRejected -Message 'failed transport result without an error message must fail closed'
    $stopFailureRejected = $false
    try {
        & $supportModule {
            Assert-MagicHandyRebuildStopResponse -Response ([pscustomobject]@{
                available = $true
                stopped = $false
                error = 'active Stop failed'
            })
        }
    } catch {
        $stopFailureRejected = $_.Exception.Message -match 'active Stop failed'
    }
    Assert-True -Condition $stopFailureRejected -Message 'a failed active Stop must abort rebuild preparation'

    Write-Host 'Checking multiple checkout instances are refused before any forced teardown...'
    $multiPortA = Get-AvailableLoopbackPort
    do { $multiPortB = Get-AvailableLoopbackPort } while ($multiPortB -eq $multiPortA)
    $multiArgsA = & $supportModule { param($Address, $DataDir) New-MagicHandyAppArgumentLine -Address $Address -DataDir $DataDir } "127.0.0.1:$multiPortA" (Join-Path $tempRoot 'multi-data-a')
    $multiArgsB = & $supportModule { param($Address, $DataDir) New-MagicHandyAppArgumentLine -Address $Address -DataDir $DataDir } "127.0.0.1:$multiPortB" (Join-Path $tempRoot 'multi-data-b')
    $multiProcessA = Start-Process -FilePath $runtimeExe -ArgumentList $multiArgsA -PassThru -WindowStyle Hidden
    $multiProcessB = Start-Process -FilePath $runtimeExe -ArgumentList $multiArgsB -PassThru -WindowStyle Hidden
    try {
        $multiDeadline = [DateTime]::UtcNow.AddSeconds(10)
        do {
            $multiReadyA = $false
            $multiReadyB = $false
            try { Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:$multiPortA/api/state" -TimeoutSec 1 | Out-Null; $multiReadyA = $true } catch {}
            try { Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:$multiPortB/api/state" -TimeoutSec 1 | Out-Null; $multiReadyB = $true } catch {}
            if (-not ($multiReadyA -and $multiReadyB)) { Start-Sleep -Milliseconds 100 }
        } while (-not ($multiReadyA -and $multiReadyB) -and [DateTime]::UtcNow -lt $multiDeadline)
        Assert-True -Condition ($multiReadyA -and $multiReadyB) -Message 'both test app instances should become ready'
        $multipleRejected = $false
        try {
            & $supportModule { param($RepositoryPath, $Port) Stop-MagicHandyAppForRebuild -RepositoryPath $RepositoryPath -Port $Port } $runtimeRepo $multiPortA
        } catch {
            $multipleRejected = $_.Exception.Message -match 'Multiple MagicHandy instances'
        }
        Assert-True -Condition $multipleRejected -Message 'multiple checkout instances must be refused before teardown'
        $multiProcessA.Refresh()
        $multiProcessB.Refresh()
        Assert-True -Condition (-not $multiProcessA.HasExited -and -not $multiProcessB.HasExited) -Message 'multiple-instance refusal must leave every app alive'
    } finally {
        foreach ($process in @($multiProcessA, $multiProcessB)) {
            $process.Refresh()
            if (-not $process.HasExited) {
                & "$env:SystemRoot\System32\taskkill.exe" /PID $process.Id /T /F | Out-Null
            }
        }
    }

    Write-Host 'Checking selected-component plans...'
    $managedPlan = @(Get-MagicHandyProvisionPlan -State $loaded)
    Assert-PlanContains -Plan $managedPlan -Pattern 'Go 1\.25'
    Assert-PlanContains -Plan $managedPlan -Pattern 'Visual Studio C\+\+'
    Assert-PlanContains -Plan $managedPlan -Pattern 'CUDA Toolkit'
    Assert-PlanContains -Plan $managedPlan -Pattern 'Parakeet CPU runner'
    Assert-PlanContains -Plan $managedPlan -Pattern 'NeuTTS Air.*protocol adapters'
    Assert-PlanContains -Plan $managedPlan -Pattern 'LLVM/libclang, Rustup.*Rust 1\.94\.0.*MSVC toolchain'
    Assert-PlanContains -Plan $managedPlan -Pattern 'Build pinned neutts-rs stream_pcm.*llama\.cpp binding'
    Assert-PlanContains -Plan $managedPlan -Pattern 'checksum-verified NeuTTS Air Q4.*NeuCodec decoder'

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
    Assert-PlanContains -Plan $ollamaPlan -Pattern 'Skip NeuTTS runtime build.*managed llama\.cpp is not selected'
    Assert-PlanExcludes -Plan $ollamaPlan -Pattern 'CMake|Visual Studio|CUDA|LLVM/libclang|Rustup|Build pinned neutts-rs|checksum-verified NeuTTS|Parakeet CPU runner'

    Write-Host 'Checking updater fast-forward and dirty-worktree refusal...'
    $git = Resolve-MagicHandyExecutable -Name 'git'
    Assert-True -Condition (-not [string]::IsNullOrWhiteSpace($git)) -Message 'Git is required by the Windows CI image'
    $remote = Join-Path $tempRoot 'remote.git'
    $seed = Join-Path $tempRoot 'seed'
    $checkout = Join-Path $tempRoot 'checkout'
    & $git init --bare --initial-branch=main $remote | Out-Null
    & $git init --initial-branch=main $seed | Out-Null
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

    Write-Host 'Checking updater follows a live feature upstream...'
    $activeCheckout = Join-Path $tempRoot 'active-feature-checkout'
    & $git -C $seed switch -c active-feature | Out-Null
    [System.IO.File]::WriteAllText((Join-Path $seed 'active.txt'), 'feature v1')
    & $git -C $seed add active.txt
    & $git -C $seed commit -m 'active feature' | Out-Null
    & $git -C $seed push -u origin active-feature | Out-Null
    & $git clone --branch active-feature $remote $activeCheckout | Out-Null
    [System.IO.File]::WriteAllText((Join-Path $seed 'active.txt'), 'feature v2')
    & $git -C $seed add active.txt
    & $git -C $seed commit -m 'advance active feature' | Out-Null
    & $git -C $seed push | Out-Null
    Update-MagicHandySource -RepositoryPath $activeCheckout -AssumeYes
    Assert-Equal -Expected 'active-feature' -Actual (& $git -C $activeCheckout branch --show-current) -Message 'live feature update should retain its branch'
    Assert-Equal -Expected 'feature v2' -Actual (Get-Content -LiteralPath (Join-Path $activeCheckout 'active.txt') -Raw) -Message 'live feature should follow its own upstream'
    & $git -C $seed switch main | Out-Null
    & $git -C $seed push origin --delete active-feature | Out-Null

    Write-Host 'Checking updater fallback for a merged and deleted feature branch...'
    $mergedCheckout = Join-Path $tempRoot 'merged-feature-checkout'
    & $git -C $seed switch -c merged-feature | Out-Null
    [System.IO.File]::WriteAllText((Join-Path $seed 'feature.txt'), 'merged feature')
    & $git -C $seed add feature.txt
    & $git -C $seed commit -m 'merged feature' | Out-Null
    & $git -C $seed push -u origin merged-feature | Out-Null
    & $git clone --single-branch --branch merged-feature $remote $mergedCheckout | Out-Null
    & $git -C $seed switch main | Out-Null
    & $git -C $seed merge --no-ff merged-feature -m 'merge feature' | Out-Null
    & $git -C $seed push origin main | Out-Null
    & $git -C $seed push origin --delete merged-feature | Out-Null
    Update-MagicHandySource -RepositoryPath $mergedCheckout -AssumeYes
    Assert-Equal -Expected 'merged-feature' -Actual (& $git -C $mergedCheckout branch --show-current) -Message 'deleted-feature fallback should retain the local branch name'
    Assert-Equal -Expected (& $git -C $mergedCheckout rev-parse refs/remotes/origin/main) -Actual (& $git -C $mergedCheckout rev-parse HEAD) -Message 'merged deleted feature should fast-forward to origin/main'
    Assert-Equal -Expected 'refs/heads/merged-feature' -Actual (& $git -C $mergedCheckout config --get branch.merged-feature.merge) -Message 'fallback should not rewrite upstream configuration'

    Write-Host 'Checking updater refusal for an unmerged deleted feature branch...'
    $unmergedCheckout = Join-Path $tempRoot 'unmerged-feature-checkout'
    & $git -C $seed switch -c unmerged-feature | Out-Null
    [System.IO.File]::WriteAllText((Join-Path $seed 'unmerged.txt'), 'local feature work')
    & $git -C $seed add unmerged.txt
    & $git -C $seed commit -m 'unmerged feature' | Out-Null
    & $git -C $seed push -u origin unmerged-feature | Out-Null
    & $git clone --branch unmerged-feature $remote $unmergedCheckout | Out-Null
    $unmergedHead = & $git -C $unmergedCheckout rev-parse HEAD
    & $git -C $seed switch main | Out-Null
    [System.IO.File]::WriteAllText((Join-Path $seed 'main-only.txt'), 'new release work')
    & $git -C $seed add main-only.txt
    & $git -C $seed commit -m 'advance main' | Out-Null
    & $git -C $seed push origin main | Out-Null
    & $git -C $seed push origin --delete unmerged-feature | Out-Null
    $unmergedRejected = $false
    try {
        Update-MagicHandySource -RepositoryPath $unmergedCheckout -AssumeYes
    } catch {
        $unmergedRejected = $_.Exception.Message -match 'contains commits not present'
    }
    Assert-True -Condition $unmergedRejected -Message 'updater should reject an unmerged feature whose upstream was deleted'
    Assert-Equal -Expected $unmergedHead -Actual (& $git -C $unmergedCheckout rev-parse HEAD) -Message 'unmerged deleted feature should keep its HEAD'
    Assert-Equal -Expected 'unmerged-feature' -Actual (& $git -C $unmergedCheckout branch --show-current) -Message 'unmerged deleted feature should keep its branch'

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
    Assert-PlanContains -Plan @($reconfigureOutput -split "`r?`n") -Pattern 'Skip NeuTTS runtime build.*managed llama\.cpp is not selected'
    Assert-PlanExcludes -Plan @($reconfigureOutput -split "`r?`n") -Pattern 'Ensure Git and CMake|CUDA Toolkit|LLVM/libclang|Rustup|Build pinned neutts-rs'
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
