$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $repoRoot

function Invoke-Required {
    param(
        [scriptblock] $Command
    )

    & $Command
    if ($LASTEXITCODE -ne 0) {
        throw "Command failed with exit code $LASTEXITCODE"
    }
}

Invoke-Required { go build -o .\run-weaver.exe .\cmd\run-weaver }

$doctor = .\run-weaver.exe doctor --target windows --json
$doctor | Write-Output

$stateDir = Join-Path $env:LOCALAPPDATA "run-weaver"
New-Item -ItemType Directory -Force -Path $stateDir | Out-Null

$runningPid = $PID
$missingPid = 999999
$statePath = Join-Path $stateDir "state.json"

function Write-RunWeaverState {
    param(
        [int] $Pid
    )

    $state = [ordered]@{
        schemaVersion = 1
        target = "windows"
        updatedAt = (Get-Date).ToUniversalTime().ToString("o")
        daemon = [ordered]@{
            pid = $Pid
            service = "run-weaver"
        }
        job = $null
    }

    $state | ConvertTo-Json -Depth 10 | Set-Content -Path $statePath -Encoding UTF8
}

Write-RunWeaverState -Pid $runningPid
Invoke-Required { .\run-weaver.exe status --json | Write-Output }

Write-RunWeaverState -Pid $missingPid
Invoke-Required { .\run-weaver.exe status --json | Write-Output }
