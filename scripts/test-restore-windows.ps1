[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$BackupDirectory,
    [string]$EnvFile = ".env.local",
    [switch]$ValidateOnly
)

$ErrorActionPreference = "Stop"
$root = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot "..")).Path
$compose = Join-Path $root "docker-compose.local.yml"

function Resolve-RepoPath([string]$Path) {
    if ([IO.Path]::IsPathRooted($Path)) { return [IO.Path]::GetFullPath($Path) }
    return [IO.Path]::GetFullPath((Join-Path $root $Path))
}

function Read-DotEnv([string]$Path) {
    $values = @{}
    foreach ($line in [IO.File]::ReadAllLines($Path)) {
        if ($line -match '^\s*#' -or [string]::IsNullOrWhiteSpace($line)) { continue }
        $separator = $line.IndexOf('=')
        if ($separator -lt 1) { continue }
        $name = $line.Substring(0, $separator).Trim()
        $value = $line.Substring($separator + 1).Trim()
        if (($value.StartsWith("'") -and $value.EndsWith("'")) -or
            ($value.StartsWith('"') -and $value.EndsWith('"'))) {
            $value = $value.Substring(1, $value.Length - 2)
        }
        $values[$name] = $value
    }
    return $values
}

function Require-Setting($Settings, [string]$Name) {
    $value = [string]$Settings[$Name]
    if ([string]::IsNullOrWhiteSpace($value)) { throw "$Name is required in the environment file." }
    return $value
}

$envPath = Resolve-RepoPath $EnvFile
$bundle = Resolve-RepoPath $BackupDirectory
if (-not (Test-Path -LiteralPath $envPath -PathType Leaf)) { throw "Environment file not found: $envPath" }
if (-not (Test-Path -LiteralPath $bundle -PathType Container)) { throw "Backup directory not found: $bundle" }
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) { throw "Docker Desktop is required." }

$manifestPath = Join-Path $bundle "manifest.json"
if (-not (Test-Path -LiteralPath $manifestPath -PathType Leaf)) { throw "Backup manifest is missing." }
$manifest = Get-Content -LiteralPath $manifestPath -Raw -Encoding utf8 | ConvertFrom-Json
if ($manifest.formatVersion -ne 1) { throw "Unsupported backup format version: $($manifest.formatVersion)" }

$requiredFiles = @("automation.dump", "identity.dump", "media.zip")
foreach ($name in $requiredFiles) {
    $record = @($manifest.files | Where-Object { $_.name -eq $name })
    if ($record.Count -ne 1) { throw "Manifest must contain exactly one $name record." }
    $path = Join-Path $bundle $name
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "Backup file is missing: $name" }
    $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $path).Hash.ToLowerInvariant()
    if ($actual -ne ([string]$record[0].sha256).ToLowerInvariant()) { throw "Checksum mismatch: $name" }
}

Add-Type -AssemblyName System.IO.Compression.FileSystem
$media = [IO.Compression.ZipFile]::OpenRead((Join-Path $bundle "media.zip"))
try { $null = $media.Entries.Count } finally { $media.Dispose() }

$settings = Read-DotEnv $envPath
$dbUser = Require-Setting $settings "DB_USER"
$liveAutomation = Require-Setting $settings "AUTOMATION_DB_NAME"
$liveIdentity = Require-Setting $settings "IDP_DB_NAME"
& docker compose --env-file $envPath -f $compose config --quiet
if ($LASTEXITCODE -ne 0) { throw "Docker Compose validation failed." }

if ($ValidateOnly) {
    Write-Host "Restore preflight passed: manifest, three checksums, media archive, and Compose are valid."
    exit 0
}

$suffix = (Get-Date).ToUniversalTime().ToString("yyyyMMddHHmmss") + (Get-Random -Minimum 1000 -Maximum 9999)
$scratchAutomation = "hai_restore_automation_$suffix".ToLowerInvariant()
$scratchIdentity = "hai_restore_identity_$suffix".ToLowerInvariant()
if ($scratchAutomation -eq $liveAutomation -or $scratchIdentity -eq $liveIdentity) {
    throw "Scratch database name collides with a configured live database."
}

$automationTemp = "/tmp/$scratchAutomation.dump"
$identityTemp = "/tmp/$scratchIdentity.dump"
try {
    & docker cp (Join-Path $bundle "automation.dump") "018-hai-postgres-automation:$automationTemp"
    if ($LASTEXITCODE -ne 0) { throw "Could not stage the automation dump." }
    & docker cp (Join-Path $bundle "identity.dump") "018-hai-postgres-idp:$identityTemp"
    if ($LASTEXITCODE -ne 0) { throw "Could not stage the identity dump." }

    & docker exec 018-hai-postgres-automation createdb -U $dbUser $scratchAutomation
    if ($LASTEXITCODE -ne 0) { throw "Could not create the automation scratch database." }
    & docker exec 018-hai-postgres-automation pg_restore -U $dbUser --exit-on-error --no-owner --no-privileges -d $scratchAutomation $automationTemp
    if ($LASTEXITCODE -ne 0) { throw "Automation restore drill failed." }

    & docker exec 018-hai-postgres-idp createdb -U $dbUser $scratchIdentity
    if ($LASTEXITCODE -ne 0) { throw "Could not create the identity scratch database." }
    & docker exec 018-hai-postgres-idp pg_restore -U $dbUser --exit-on-error --no-owner --no-privileges -d $scratchIdentity $identityTemp
    if ($LASTEXITCODE -ne 0) { throw "Identity restore drill failed." }

    $automationTables = [int]((& docker exec 018-hai-postgres-automation psql -U $dbUser -d $scratchAutomation -Atqc "SELECT count(*) FROM pg_catalog.pg_tables WHERE schemaname='public'" | Out-String).Trim())
    $identityTables = [int]((& docker exec 018-hai-postgres-idp psql -U $dbUser -d $scratchIdentity -Atqc "SELECT count(*) FROM pg_catalog.pg_tables WHERE schemaname='public'" | Out-String).Trim())
    if ($automationTables -lt 1) { throw "Automation restore contains no public tables." }
    if ($identityTables -lt 1) { throw "Identity restore contains no public tables." }
    Write-Host "Restore drill passed: automation tables=$automationTables; identity tables=$identityTables; live databases untouched."
} finally {
    & docker exec 018-hai-postgres-automation dropdb -U $dbUser --if-exists $scratchAutomation 2>$null | Out-Null
    & docker exec 018-hai-postgres-idp dropdb -U $dbUser --if-exists $scratchIdentity 2>$null | Out-Null
    & docker exec 018-hai-postgres-automation rm -f $automationTemp 2>$null | Out-Null
    & docker exec 018-hai-postgres-idp rm -f $identityTemp 2>$null | Out-Null
}
