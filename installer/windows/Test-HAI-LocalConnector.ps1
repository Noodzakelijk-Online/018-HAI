[CmdletBinding()]
param(
    [ValidateRange(1, 60)]
    [int]$TimeoutSeconds = 10,
    [ValidateLength(1, 1000)]
    [string]$Request = "Prepare a source-backed review checklist without sending or changing anything."
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "Hai-InstallerSupport.ps1")

Assert-HaiDockerReady
Assert-HaiSingleInstallation
Assert-HaiLocalEnvironment

$environmentContent = [IO.File]::ReadAllText((Get-HaiEnvironmentFile))
if ((Get-HaiEnvironmentValue -Content $environmentContent -Name "HAI_A2A_BRIDGE_ENABLED").ToLowerInvariant() -ne "true") {
    throw "The local HAI connector is disabled. Start HAI again to restore the installed local planning connector."
}
$token = Get-HaiEnvironmentValue -Content $environmentContent -Name "HAI_A2A_BRIDGE_TOKEN"
if ($token.Length -lt 32) {
    throw "The local HAI connector token is missing or invalid. Start HAI again to regenerate the installed local environment."
}

$baseUrl = (Get-HaiUrl).TrimEnd("/")
try {
    $cardResponse = Invoke-RestMethod -UseBasicParsing -Uri "$baseUrl/.well-known/agent-card.json" -TimeoutSec $TimeoutSeconds
} catch {
    throw "The local HAI connector Agent Card is unavailable at $baseUrl. Start HAI and wait for readiness before retrying."
}
if ($cardResponse.name -ne "HAI controlled planning" -or
    $cardResponse.skills[0].id -ne "hai_controlled_planning") {
    throw "The local HAI connector returned an unexpected Agent Card. Do not connect another tool until the HAI installation has been repaired."
}

$body = @{
    jsonrpc = "2.0"
    id      = "hai-local-connector-check"
    method  = "SendMessage"
    params  = @{
        message = @{
            messageId = "hai-local-connector-check"
            role      = "ROLE_USER"
            parts     = @(@{ text = $Request; mediaType = "text/plain" })
        }
    }
} | ConvertTo-Json -Depth 8 -Compress

try {
    $result = Invoke-RestMethod -UseBasicParsing -Method Post -Uri "$baseUrl/api/v1/a2a" `
        -Headers @{ Authorization = "Bearer $token"; "A2A-Version" = "1.0" } `
        -ContentType "application/json" -Body $body -TimeoutSec $TimeoutSeconds
} catch {
    throw "The local HAI connector rejected its controlled planning check. Verify that HAI is ready and that no other local gateway is using this port."
}

$task = $result.result.task
$artifact = $task.artifacts | Select-Object -First 1
if ($task.status.state -ne "TASK_STATE_COMPLETED" -or
    $artifact.name -ne "hai-controlled-planning-proposal" -or
    $artifact.description -notmatch "Non-executable planning") {
    throw "The local HAI connector returned an unexpected result. No external work was performed, but the connector should be repaired before use."
}

Write-Host "Local HAI connector passed: the Agent Card and a non-executable planning draft are available at $baseUrl." -ForegroundColor Green
