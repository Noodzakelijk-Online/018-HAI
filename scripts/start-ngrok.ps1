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
$repoRoot = Split-Path -Parent $PSScriptRoot

function Resolve-RepoFile([string]$Path) {
    if ([System.IO.Path]::IsPathRooted($Path)) { return $Path }
    return Join-Path $repoRoot $Path
}

function Read-DotEnv([string]$Path) {
    $values = @{}
    foreach ($line in Get-Content -LiteralPath $Path) {
        $trimmed = $line.Trim()
        if (!$trimmed -or $trimmed.StartsWith('#') -or !$trimmed.Contains('=')) { continue }
        $key, $value = $trimmed.Split('=', 2)
        $values[$key.Trim()] = $value.Trim().Trim('"').Trim("'")
    }
    return $values
}

function Require-Setting($Values, [string]$Name, [int]$MinimumLength = 1) {
    $value = [string]$Values[$Name]
    if ($value.Length -lt $MinimumLength -or $value -match '(?i)change-this|changeme|example|placeholder') {
        throw "$Name must be configured with a non-placeholder value."
    }
    return $value
}

function Require-Value($Values, [string]$Name, [string]$Expected) {
    if ([string]$Values[$Name] -ne $Expected) { throw "$Name must be '$Expected' before public access can start." }
}

function Assert-HaiComposeOwnership {
    $ids = @(docker ps -aq --filter 'label=com.docker.compose.project=018-hai')
    if (!$ids.Count) { return }
    $workingDirs = @(docker inspect $ids --format '{{ index .Config.Labels "com.docker.compose.project.working_dir" }}' | Where-Object { $_ }) | Select-Object -Unique
    foreach ($workingDir in $workingDirs) {
        if (([System.IO.Path]::GetFullPath($workingDir)).TrimEnd('\\') -ne $repoRoot.TrimEnd('\\')) {
            throw "Refusing to manage 018-hai containers owned by $workingDir. Use that checkout instead."
        }
    }
}

$envPath = Resolve-RepoFile $EnvFile
$composePath = Resolve-RepoFile $ComposeFile
if (!(Test-Path -LiteralPath $envPath)) { throw "Environment file not found: $envPath" }
if (!(Test-Path -LiteralPath $composePath)) { throw "Compose file not found: $composePath" }

Assert-HaiComposeOwnership
if ($Stop) {
    docker compose --env-file $envPath --profile cloud-tunnel -f $composePath stop ngrok
    exit $LASTEXITCODE
}

$settings = Read-DotEnv $envPath
Require-Value $settings 'RUN_MODE' 'production'
Require-Value $settings 'LOCAL_LOGIN_BYPASS_ENABLED' 'false'
Require-Value $settings 'IDP_COOKIE_SECURE' 'true'
Require-Value $settings 'GATEWAY_HOST_BIND' '127.0.0.1'
Require-Value $settings 'HAI_A2A_BRIDGE_PUBLIC_NGROK_ENABLED' 'false'
$ngrokUrl = Require-Setting $settings 'HAI_NGROK_URL'
Require-Setting $settings 'NGROK_AUTHTOKEN' 20 | Out-Null
foreach ($secret in 'JWT_SECRET', 'BACKEND_API_SHARED_KEY', 'HAI_MEMORY_ENCRYPTION_KEY', 'HAI_APPROVAL_PROOF_SIGNING_KEY') {
    Require-Setting $settings $secret 32 | Out-Null
}
if ($ngrokUrl -notmatch '^https://[^/@?#]+\.(ngrok\.app|ngrok\.dev|ngrok-free\.app|ngrok-free\.dev)$') {
    throw 'HAI_NGROK_URL must be a reserved HTTPS ngrok origin without path, query, fragment, or credentials.'
}

docker compose --env-file $envPath --profile cloud-tunnel -f $composePath config --quiet
if ($LASTEXITCODE -ne 0 -or $ValidateOnly) { exit $LASTEXITCODE }
docker compose --env-file $envPath --profile cloud-tunnel -f $composePath up -d ngrok
exit $LASTEXITCODE
