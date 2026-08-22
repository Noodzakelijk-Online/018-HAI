[CmdletBinding()]
param(
    [ValidateRange(1, 65535)]
    [int]$GatewayPort = 8088,
    [ValidateRange(30, 900)]
    [int]$HealthTimeoutSeconds = 600,
    [switch]$EnableEventBus,
    [switch]$NoBrowser
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "Hai-InstallerSupport.ps1")

Assert-HaiDockerReady
Assert-HaiSingleInstallation
Initialize-HaiLocalEnvironment -GatewayPort $GatewayPort
if ($EnableEventBus) {
    Set-HaiEventBusEnabled
} else {
    Set-HaiEventBusDisabled
}

$composeArguments = Get-HaiComposeArguments
$profileArguments = @()
if ($EnableEventBus) {
    $profileArguments = @("--profile", "event-bus")
} else {
    # A prior event-bus start leaves the optional containers present. Stop them
    # explicitly so normal startup returns the machine to the low-resource mode.
    & docker @composeArguments --profile event-bus stop zookeeper kafka nginxconfigmanager
    if ($LASTEXITCODE -ne 0) {
        throw "HAI could not stop the optional Kafka event-bus services. Open Docker Desktop and inspect the 018-hai Kafka containers."
    }
}
Write-Host "Starting the local HAI stack. The first run downloads and builds its containers." -ForegroundColor Cyan
& docker @composeArguments @profileArguments up -d --build
if ($LASTEXITCODE -ne 0) {
    throw "HAI startup failed. Open Docker Desktop and inspect the 018-hai container logs."
}

Wait-HaiReady -TimeoutSeconds $HealthTimeoutSeconds
$url = Get-HaiUrl
Write-Host "HAI is ready at $url" -ForegroundColor Green
if (-not $NoBrowser) {
    Start-Process $url
}
