[CmdletBinding()]
param(
    [ValidateRange(1, 65535)]
    [int]$GatewayPort = 8088,
    [ValidateRange(30, 900)]
    [int]$HealthTimeoutSeconds = 600,
    [switch]$EnableHostRuntime,
    [switch]$NoBrowser
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "Hai-InstallerSupport.ps1")

Assert-HaiDockerReady
Assert-HaiSingleInstallation
Initialize-HaiLocalEnvironment -GatewayPort $GatewayPort

$composeArguments = Get-HaiComposeArguments
if ($EnableHostRuntime) {
    Assert-HaiHostRuntimeConfigured
    $composeArguments += @("--profile", "local-host-runtime")
}
Write-Host "Starting the local HAI stack. The first run downloads and builds its containers." -ForegroundColor Cyan
& docker @composeArguments --profile local-a2a up -d --build
if ($LASTEXITCODE -ne 0) {
    throw "HAI startup failed. Open Docker Desktop and inspect the 018-hai container logs."
}

Wait-HaiReady -TimeoutSeconds $HealthTimeoutSeconds
if ($EnableHostRuntime) {
    Start-HaiHostRuntimeWorker
    Write-Host "HAI's optional local DeepSeek Harness worker has been started." -ForegroundColor Green
}
$url = Get-HaiUrl
Write-Host "HAI is ready at $url" -ForegroundColor Green
if (-not $NoBrowser) {
    Start-Process $url
}
