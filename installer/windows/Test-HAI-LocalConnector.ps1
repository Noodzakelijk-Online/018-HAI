[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "Hai-InstallerSupport.ps1")

Assert-HaiDockerReady
$environmentFile = Get-HaiEnvironmentFile
if (-not (Test-Path -LiteralPath $environmentFile -PathType Leaf)) {
    throw "HAI is not initialized. Use Start HAI before testing the local connector."
}

$agentCardUrl = "$(Get-HaiA2AUrl)/.well-known/agent-card.json"
try {
    $response = Invoke-WebRequest -UseBasicParsing -Uri $agentCardUrl -TimeoutSec 10
} catch {
    throw "The local A2A connector is unavailable at $agentCardUrl. Start HAI, then try again."
}

if ($response.StatusCode -ne 200) {
    throw "The local A2A connector returned HTTP $($response.StatusCode) at $agentCardUrl."
}

try {
    $agentCard = $response.Content | ConvertFrom-Json -ErrorAction Stop
} catch {
    throw "The local A2A connector returned an invalid Agent Card."
}

if ([string]::IsNullOrWhiteSpace($agentCard.name) -or [string]::IsNullOrWhiteSpace($agentCard.url)) {
    throw "The local A2A connector returned an incomplete Agent Card."
}

Write-Host "Local A2A connector is reachable at $agentCardUrl" -ForegroundColor Green
Write-Host "Agent: $($agentCard.name)"
Write-Host "Planning endpoint: $($agentCard.url)"
