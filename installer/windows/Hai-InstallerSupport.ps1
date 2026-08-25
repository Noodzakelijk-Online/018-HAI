[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"

function Get-HaiInstallRoot {
    return (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot "..\..")).Path
}

function Get-HaiDataRoot {
    $dataRoot = Join-Path $env:LOCALAPPDATA "HAI"
    if (-not (Test-Path -LiteralPath $dataRoot -PathType Container)) {
        New-Item -ItemType Directory -Path $dataRoot -Force | Out-Null
    }
    return $dataRoot
}

function Get-HaiEnvironmentFile {
    return (Join-Path (Get-HaiDataRoot) "hai.env")
}

function Get-HaiEnvironmentValue {
    param([Parameter(Mandatory = $true)][string]$Name)

    $environmentFile = Get-HaiEnvironmentFile
    if (-not (Test-Path -LiteralPath $environmentFile -PathType Leaf)) {
        throw "HAI is not initialized. Use Start HAI before reading local connector settings."
    }

    $pattern = "(?m)^$([Regex]::Escape($Name))=(?<value>.*)$"
    $match = [Regex]::Match([IO.File]::ReadAllText($environmentFile), $pattern)
    if (-not $match.Success) {
        throw "HAI environment file is missing $Name. Start HAI again to recreate the local configuration."
    }

    return $match.Groups["value"].Value.Trim().Trim("'").Trim('"')
}

function Get-HaiComposeFile {
    $composeFile = Join-Path (Get-HaiInstallRoot) "docker-compose.local.yml"
    if (-not (Test-Path -LiteralPath $composeFile -PathType Leaf)) {
        throw "HAI installation is incomplete: missing $composeFile"
    }
    return $composeFile
}

function Get-HaiGatewayPort {
    $environmentFile = Get-HaiEnvironmentFile
    if (-not (Test-Path -LiteralPath $environmentFile -PathType Leaf)) {
        return 8088
    }

    $match = [Regex]::Match(
        [IO.File]::ReadAllText($environmentFile),
        "(?m)^GATEWAY_HOST_PORT=(?<port>[0-9]{1,5})$"
    )
    if (-not $match.Success) {
        return 8088
    }

    $port = [int]$match.Groups["port"].Value
    if ($port -lt 1 -or $port -gt 65535) {
        throw "HAI environment file contains an invalid gateway port."
    }
    return $port
}

function Get-HaiA2ALocalPort {
    $environmentFile = Get-HaiEnvironmentFile
    if (-not (Test-Path -LiteralPath $environmentFile -PathType Leaf)) {
        return 8091
    }

    $match = [Regex]::Match(
        [IO.File]::ReadAllText($environmentFile),
        "(?m)^HAI_A2A_LOCAL_PORT=(?<port>[0-9]{1,5})$"
    )
    if (-not $match.Success) {
        return 8091
    }

    $port = [int]$match.Groups["port"].Value
    if ($port -lt 1 -or $port -gt 65535) {
        throw "HAI environment file contains an invalid local A2A port."
    }
    return $port
}

function Get-HaiUrl {
    return "http://127.0.0.1:$(Get-HaiGatewayPort)"
}

function Get-HaiA2AUrl {
    return "http://127.0.0.1:$(Get-HaiA2ALocalPort)"
}

function Assert-HaiHostRuntimeConfigured {
    $enabled = Get-HaiEnvironmentValue -Name "HAI_HOST_RUNTIME_BRIDGE_ENABLED"
    $token = Get-HaiEnvironmentValue -Name "HAI_HOST_RUNTIME_BRIDGE_TOKEN"
    $bridgeUrl = Get-HaiEnvironmentValue -Name "HAI_HOST_RUNTIME_BRIDGE_URL"
    $harnessEnabled = Get-HaiEnvironmentValue -Name "DEEPSEEK_HARNESS_ENABLED"
    $executionEnabled = Get-HaiEnvironmentValue -Name "DEEPSEEK_HARNESS_EXECUTION_ENABLED"
    $executable = Get-HaiEnvironmentValue -Name "DEEPSEEK_HARNESS_EXECUTABLE"
    $version = Get-HaiEnvironmentValue -Name "DEEPSEEK_HARNESS_VERSION"
    $workspace = Get-HaiEnvironmentValue -Name "DEEPSEEK_HARNESS_WORKSPACE"
    $stateDirectory = Get-HaiEnvironmentValue -Name "DEEPSEEK_HARNESS_STATE_DIR"
    $workspaceKey = Get-HaiEnvironmentValue -Name "DEEPSEEK_HARNESS_WORKSPACE_KEY"

    if ($enabled -ine "true") {
        throw "Host runtime is disabled in the local HAI environment. Enable it explicitly only after reviewing the DeepSeek Harness worker setup."
    }
    if ($harnessEnabled -ine "true" -or $executionEnabled -ine "true") {
        throw "Host runtime requires both DEEPSEEK_HARNESS_ENABLED=true and DEEPSEEK_HARNESS_EXECUTION_ENABLED=true. HAI will not start a worker that the backend is configured to block."
    }
    if ($token.Length -lt 32) {
        throw "HAI_HOST_RUNTIME_BRIDGE_TOKEN must be a dedicated random value with at least 32 characters."
    }
    $bridgeUri = $null
    if (-not [Uri]::TryCreate($bridgeUrl, [UriKind]::Absolute, [ref]$bridgeUri) -or
        $bridgeUri.Scheme -ne "http" -or
        $bridgeUri.Host -notin @("127.0.0.1", "localhost") -or
        -not [string]::IsNullOrWhiteSpace($bridgeUri.UserInfo) -or
        -not [string]::IsNullOrWhiteSpace($bridgeUri.Query) -or
        -not [string]::IsNullOrWhiteSpace($bridgeUri.Fragment)) {
        throw "HAI_HOST_RUNTIME_BRIDGE_URL must be an http loopback URL without credentials, query, or fragment."
    }
    if ([string]::IsNullOrWhiteSpace($version) -or
        [string]::IsNullOrWhiteSpace($workspace) -or
        [string]::IsNullOrWhiteSpace($stateDirectory) -or
        [string]::IsNullOrWhiteSpace($workspaceKey) -or
        [string]::IsNullOrWhiteSpace($executable)) {
        throw "Host runtime requires a pinned DeepSeek Harness version, executable, workspace key, and dedicated Windows workspace and state-directory settings."
    }
    if (-not (Test-Path -LiteralPath $workspace -PathType Container)) {
        throw "DEEPSEEK_HARNESS_WORKSPACE must be an existing dedicated Windows directory."
    }
    $resolvedWorkspace = [IO.Path]::GetFullPath($workspace)
    $resolvedStateDirectory = [IO.Path]::GetFullPath($stateDirectory)
    if (-not $resolvedStateDirectory.StartsWith($resolvedWorkspace.TrimEnd([IO.Path]::DirectorySeparatorChar) + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
        throw "DEEPSEEK_HARNESS_STATE_DIR must stay inside DEEPSEEK_HARNESS_WORKSPACE."
    }
    $command = Get-Command $executable -ErrorAction SilentlyContinue
    if (-not $command) {
        throw "DEEPSEEK_HARNESS_EXECUTABLE is not available on this Windows machine. Install or configure the pinned DeepSeek Harness CLI first."
    }
    $reportedVersion = (& $command.Source --version 2>$null | Out-String).Trim()
    if ($LASTEXITCODE -ne 0 -or $reportedVersion -notlike "*$version*") {
        throw "DeepSeek Harness version mismatch. Expected '$version'; the configured executable reported '$reportedVersion'."
    }
}

function Start-HaiHostRuntimeWorker {
    $workerScript = Join-Path $PSScriptRoot "Run-HAI-DeepSeekBridge.ps1"
    $bridgeBinary = Join-Path $PSScriptRoot "hai-dsh-bridge.exe"
    $pidFile = Join-Path (Get-HaiDataRoot) "hai-dsh-bridge.pid"
    $logDirectory = Join-Path (Get-HaiDataRoot) "logs"
    if (-not (Test-Path -LiteralPath $workerScript -PathType Leaf) -or -not (Test-Path -LiteralPath $bridgeBinary -PathType Leaf)) {
        throw "The HAI host runtime worker is missing from this installation. Reinstall HAI from a complete installer payload."
    }

    if (Test-Path -LiteralPath $pidFile -PathType Leaf) {
        $existingPid = 0
        if ([int]::TryParse([IO.File]::ReadAllText($pidFile).Trim(), [ref]$existingPid)) {
            if (Test-HaiHostRuntimeWorkerProcess -ProcessId $existingPid) {
                return
            }
        }
        Remove-Item -LiteralPath $pidFile -Force -ErrorAction SilentlyContinue
    }

    New-Item -ItemType Directory -Path $logDirectory -Force | Out-Null
    $stdout = Join-Path $logDirectory "hai-dsh-bridge.out.log"
    $stderr = Join-Path $logDirectory "hai-dsh-bridge.err.log"
    $arguments = "-NoProfile -ExecutionPolicy Bypass -File `"$workerScript`" -EnvFile `"$(Get-HaiEnvironmentFile)`""
    $process = Start-Process -FilePath "$env:WINDIR\System32\WindowsPowerShell\v1.0\powershell.exe" `
        -ArgumentList $arguments -WorkingDirectory (Get-HaiInstallRoot) -WindowStyle Hidden -PassThru `
        -RedirectStandardOutput $stdout -RedirectStandardError $stderr
    [IO.File]::WriteAllText($pidFile, $process.Id.ToString(), (New-Object Text.UTF8Encoding($false)))
    Start-Sleep -Milliseconds 750
    if (-not (Get-Process -Id $process.Id -ErrorAction SilentlyContinue)) {
        Remove-Item -LiteralPath $pidFile -Force -ErrorAction SilentlyContinue
        throw "HAI host runtime worker exited during startup. Inspect %LOCALAPPDATA%\HAI\logs\hai-dsh-bridge.err.log for a local diagnostic."
    }
}

function Test-HaiHostRuntimeWorkerProcess {
    param(
        [Parameter(Mandatory = $true)]
        [int]$ProcessId
    )

    $process = Get-CimInstance -ClassName Win32_Process -Filter "ProcessId = $ProcessId" -ErrorAction SilentlyContinue
    if (-not $process) {
        return $false
    }

    $expectedScript = Join-Path $PSScriptRoot "Run-HAI-DeepSeekBridge.ps1"
    return $process.Name -ieq "powershell.exe" -and
        -not [string]::IsNullOrWhiteSpace($process.CommandLine) -and
        $process.CommandLine.IndexOf($expectedScript, [StringComparison]::OrdinalIgnoreCase) -ge 0
}

function Get-HaiHostRuntimeWorkerStatus {
    $pidFile = Join-Path (Get-HaiDataRoot) "hai-dsh-bridge.pid"
    if (-not (Test-Path -LiteralPath $pidFile -PathType Leaf)) {
        return "not started"
    }
    $workerPid = 0
    if (-not [int]::TryParse([IO.File]::ReadAllText($pidFile).Trim(), [ref]$workerPid)) {
        return "pid record invalid"
    }
    if (Test-HaiHostRuntimeWorkerProcess -ProcessId $workerPid) {
        return "running (PID $workerPid)"
    }
    if (Get-Process -Id $workerPid -ErrorAction SilentlyContinue) {
        return "pid record does not reference the HAI runtime worker"
    }
    return "stopped (inspect local bridge logs)"
}

function Stop-HaiHostRuntimeWorker {
    $pidFile = Join-Path (Get-HaiDataRoot) "hai-dsh-bridge.pid"
    if (-not (Test-Path -LiteralPath $pidFile -PathType Leaf)) {
        return
    }
    $workerPid = 0
    if (-not [int]::TryParse([IO.File]::ReadAllText($pidFile).Trim(), [ref]$workerPid)) {
        Remove-Item -LiteralPath $pidFile -Force
        return
    }
    if (Test-HaiHostRuntimeWorkerProcess -ProcessId $workerPid) {
        Stop-Process -Id $workerPid -ErrorAction Stop
    }
    Remove-Item -LiteralPath $pidFile -Force
}

function Assert-HaiDockerReady {
    if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
        throw "Docker Desktop is required. Install and start Docker Desktop, then run Start HAI again."
    }

    $serverVersion = & docker version --format '{{.Server.Version}}' 2>$null
    if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($serverVersion)) {
        throw "Docker Desktop is installed but its Linux engine is not ready. Start Docker Desktop and wait for it to finish starting."
    }

    & docker compose version *> $null
    if ($LASTEXITCODE -ne 0) {
        throw "Docker Compose v2 is required. Update Docker Desktop, then run Start HAI again."
    }
}

function Assert-HaiSingleInstallation {
    $installRoot = Get-HaiInstallRoot
    $containerIds = @(& docker ps -aq --filter "label=com.docker.compose.project=018-hai")
    if ($LASTEXITCODE -ne 0 -or $containerIds.Count -eq 0) {
        return
    }

    $workingDirectories = @(
        $containerIds |
            ForEach-Object { & docker inspect --format '{{index .Config.Labels "com.docker.compose.project.working_dir"}}' $_ 2>$null } |
            Where-Object { -not [string]::IsNullOrWhiteSpace($_) } |
            Select-Object -Unique
    )
    foreach ($workingDirectory in $workingDirectories) {
        if (-not [string]::Equals(
            [IO.Path]::GetFullPath($workingDirectory),
            [IO.Path]::GetFullPath($installRoot),
            [StringComparison]::OrdinalIgnoreCase
        )) {
            throw "HAI is already running from '$workingDirectory'. Stop that installation before starting this one so HAI keeps one canonical local stack and one set of Docker volumes."
        }
    }
}

function Get-HaiComposeArguments {
    return @("compose", "--env-file", (Get-HaiEnvironmentFile), "-f", (Get-HaiComposeFile))
}

function Initialize-HaiLocalEnvironment {
    param([ValidateRange(1, 65535)][int]$GatewayPort = 8088)

    $environmentFile = Get-HaiEnvironmentFile
    if (Test-Path -LiteralPath $environmentFile -PathType Leaf) {
        return
    }

    $initializer = Join-Path (Get-HaiInstallRoot) "scripts\initialize-windows.ps1"
    & $initializer -EnvFile $environmentFile -GatewayPort $GatewayPort
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $environmentFile -PathType Leaf)) {
        throw "HAI environment initialization failed. Check the setup prompts and try Start HAI again."
    }
}

function Wait-HaiReady {
    param([ValidateRange(30, 900)][int]$TimeoutSeconds = 600)

    $uri = "$(Get-HaiUrl)/readyz"
    $deadline = [DateTimeOffset]::UtcNow.AddSeconds($TimeoutSeconds)
    do {
        try {
            $response = Invoke-WebRequest -UseBasicParsing -Uri $uri -TimeoutSec 5
            if ($response.StatusCode -eq 200) {
                return
            }
        } catch {
            # The stack is still building or waiting for dependencies.
        }
        Start-Sleep -Seconds 3
    } while ([DateTimeOffset]::UtcNow -lt $deadline)

    throw "HAI did not become ready within $TimeoutSeconds seconds. Open Docker Desktop and inspect the 018-hai containers."
}
