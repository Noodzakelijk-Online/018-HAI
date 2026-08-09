[CmdletBinding(DefaultParameterSetName = 'Start')]
param(
    [Parameter(ParameterSetName = 'Start')]
    [switch]$ValidateOnly,

    [Parameter(ParameterSetName = 'Stop', Mandatory = $true)]
    [switch]$Stop,

    [string]$EnvFile = '.env.local',
    [string]$ComposeFile = 'docker-compose.local.yml'
)

$ErrorActionPreference = 'Stop'
$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))

function Resolve-RepoFile([string]$PathValue) {
    $candidate = if ([IO.Path]::IsPathRooted($PathValue)) {
        $PathValue
    } else {
        Join-Path $repoRoot $PathValue
    }
    $resolved = [IO.Path]::GetFullPath($candidate)
    if (-not (Test-Path -LiteralPath $resolved -PathType Leaf)) {
        throw "Required file not found: $resolved"
    }
    return $resolved
}

function Read-DotEnv([string]$PathValue) {
    $values = @{}
    foreach ($line in Get-Content -LiteralPath $PathValue) {
        if ($line -notmatch '^\s*([A-Za-z_][A-Za-z0-9_]*)=(.*)$') {
            continue
        }
        $name = $Matches[1]
        $value = $Matches[2].Trim()
        if (($value.StartsWith('"') -and $value.EndsWith('"')) -or
            ($value.StartsWith("'") -and $value.EndsWith("'"))) {
            $value = $value.Substring(1, $value.Length - 2)
        }
        $values[$name] = $value
    }
    return $values
}

function Get-Setting([hashtable]$Values, [string]$Name, [string]$Default = '') {
    if ($Values.ContainsKey($Name)) {
        return [string]$Values[$Name]
    }
    return $Default
}

function Require-Secret([hashtable]$Values, [string]$Name, [int]$MinimumLength) {
    $value = Get-Setting $Values $Name
    if ($value.Length -lt $MinimumLength -or $value -match '(?i)change[-_ ]?this|changeme|example|placeholder') {
        throw "$Name must be a non-placeholder secret of at least $MinimumLength characters before public access."
    }
}

function Test-NgrokHostname([string]$HostName) {
    $normalized = $HostName.Trim().ToLowerInvariant()
    foreach ($suffix in @('.ngrok.app', '.ngrok.dev', '.ngrok-free.app', '.ngrok-free.dev')) {
        if ($normalized.Length -gt $suffix.Length -and $normalized.EndsWith($suffix)) {
            return $true
        }
    }
    return $false
}

$envPath = Resolve-RepoFile $EnvFile
$composePath = Resolve-RepoFile $ComposeFile

if ($Stop) {
    & docker compose --env-file $envPath --profile cloud-tunnel -f $composePath stop ngrok
    if ($LASTEXITCODE -ne 0) {
        throw 'Failed to stop the ngrok service.'
    }
    return
}

$settings = Read-DotEnv $envPath
if ((Get-Setting $settings 'RUN_MODE' 'production').ToLowerInvariant() -ne 'production') {
    throw 'RUN_MODE must be production before public access is enabled.'
}
if ((Get-Setting $settings 'LOCAL_LOGIN_BYPASS_ENABLED' 'false').ToLowerInvariant() -ne 'false') {
    throw 'LOCAL_LOGIN_BYPASS_ENABLED must be false before public access is enabled.'
}
if ((Get-Setting $settings 'IDP_COOKIE_SECURE' 'false').ToLowerInvariant() -ne 'true') {
    throw 'IDP_COOKIE_SECURE must be true before public access is enabled.'
}
if ((Get-Setting $settings 'GATEWAY_HOST_BIND' '127.0.0.1') -ne '127.0.0.1') {
    throw 'GATEWAY_HOST_BIND must remain 127.0.0.1; ngrok reaches nginx on the private Docker network.'
}
$rateLimit = 0
if (-not [int]::TryParse((Get-Setting $settings 'RATE_LIMIT_PER_MINUTE' '0'), [ref]$rateLimit) -or $rateLimit -le 0) {
    throw 'RATE_LIMIT_PER_MINUTE must be a positive integer before public access is enabled.'
}

Require-Secret $settings 'NGROK_AUTHTOKEN' 20
Require-Secret $settings 'JWT_SECRET' 32
Require-Secret $settings 'BACKEND_API_SHARED_KEY' 32
Require-Secret $settings 'HAI_MEMORY_ENCRYPTION_KEY' 32
Require-Secret $settings 'HAI_APPROVAL_PROOF_SIGNING_KEY' 32
Require-Secret $settings 'DB_PASSWORD' 32
Require-Secret $settings 'DB_RUNTIME_PASSWORD' 32

$publicUrlText = (Get-Setting $settings 'HAI_NGROK_URL').TrimEnd('/')
$publicUri = $null
if (-not [Uri]::TryCreate($publicUrlText, [UriKind]::Absolute, [ref]$publicUri) -or
    $publicUri.Scheme -ne 'https' -or
    -not [string]::IsNullOrEmpty($publicUri.UserInfo) -or
    $publicUri.AbsolutePath -ne '/' -or
    -not [string]::IsNullOrEmpty($publicUri.Query) -or
    -not [string]::IsNullOrEmpty($publicUri.Fragment) -or
    -not $publicUri.IsDefaultPort -or
    -not (Test-NgrokHostname $publicUri.DnsSafeHost) -or
    $publicUri.Host -eq 'your-reserved-domain.ngrok.app') {
    throw 'HAI_NGROK_URL must be a fixed ngrok HTTPS origin without credentials, port, path, query, or fragment.'
}

$publicA2A = (Get-Setting $settings 'HAI_A2A_BRIDGE_PUBLIC_NGROK_ENABLED' 'false').ToLowerInvariant() -eq 'true'
if ($publicA2A) {
    if ((Get-Setting $settings 'HAI_A2A_BRIDGE_ENABLED' 'false').ToLowerInvariant() -ne 'true') {
        throw 'HAI_A2A_BRIDGE_ENABLED must be true when the public ngrok bridge is enabled.'
    }
    Require-Secret $settings 'HAI_A2A_BRIDGE_TOKEN' 32
    if ([string]::IsNullOrWhiteSpace((Get-Setting $settings 'HAI_A2A_BRIDGE_OWNER_ID'))) {
        throw 'HAI_A2A_BRIDGE_OWNER_ID is required when the public ngrok bridge is enabled.'
    }
    if ((Get-Setting $settings 'HAI_A2A_BRIDGE_URL').TrimEnd('/') -ne ($publicUrlText + '/api/v1/a2a')) {
        throw "HAI_A2A_BRIDGE_URL must equal $publicUrlText/api/v1/a2a when the public ngrok bridge is enabled."
    }
}

$callbackPaths = @{
    GOOGLE_LOGIN_REDIRECT_URL = '/api/v1/auth/google/callback'
    GOOGLE_OAUTH_REDIRECT_URL = '/api/v1/sources/oauth/google/callback'
}
foreach ($entry in $callbackPaths.GetEnumerator()) {
    $configured = Get-Setting $settings $entry.Key
    if ($configured -and $configured -ne ($publicUrlText + $entry.Value)) {
        throw "$($entry.Key) must equal $publicUrlText$($entry.Value) when configured."
    }
}

& docker compose --env-file $envPath --profile cloud-tunnel -f $composePath config --quiet
if ($LASTEXITCODE -ne 0) {
    throw 'Docker Compose validation failed.'
}

Write-Host "Cloud-tunnel preflight passed for $publicUrlText"
if ($ValidateOnly) {
    return
}

function Wait-ForHealthyContainer([string]$ContainerName, [int]$Attempts = 45) {
    for ($attempt = 0; $attempt -lt $Attempts; $attempt++) {
        $state = & docker inspect --format '{{.State.Status}}|{{if .State.Health}}{{.State.Health.Status}}{{end}}' $ContainerName 2>$null
        if ($LASTEXITCODE -eq 0) {
            $parts = $state -split '\|', 2
            if ($parts[0] -eq 'running' -and $parts[1] -eq 'healthy') {
                return
            }
            if ($parts[0] -eq 'exited' -or $parts[1] -eq 'unhealthy') {
                & docker logs --tail 40 $ContainerName
                throw "$ContainerName entered state $state."
            }
        }
        Start-Sleep -Seconds 2
    }
    & docker logs --tail 40 $ContainerName
    throw "$ContainerName did not become healthy within $($Attempts * 2) seconds."
}

# Reconcile the security-sensitive base services before creating any public
# endpoint. This applies secure-cookie and OAuth callback changes to the actual
# running IDP rather than trusting only the env file.
& docker compose --env-file $envPath --profile cloud-tunnel -f $composePath up -d --no-build idp backend frontend nginx
if ($LASTEXITCODE -ne 0) {
    throw 'Failed to reconcile the secured HAI gateway services. Build the base stack first.'
}
foreach ($container in @('018-hai-idp', '018-hai-backend', '018-hai-frontend', 'gateway')) {
    Wait-ForHealthyContainer $container
}

$gatewayPort = Get-Setting $settings 'GATEWAY_HOST_PORT' '8088'
try {
    $ready = Invoke-WebRequest -Uri "http://127.0.0.1:$gatewayPort/readyz" -UseBasicParsing -TimeoutSec 5
    if ($ready.StatusCode -ne 200) {
        throw "unexpected HTTP $($ready.StatusCode)"
    }
} catch {
    throw "The reconciled local HAI gateway is not ready on port $gatewayPort. $($_.Exception.Message)"
}

& docker compose --env-file $envPath --profile cloud-tunnel -f $composePath up -d --no-build ngrok
if ($LASTEXITCODE -ne 0) {
    throw 'Failed to start the ngrok service.'
}
Wait-ForHealthyContainer '018-hai-ngrok' 30
Write-Host "HAI is available through $publicUrlText"
