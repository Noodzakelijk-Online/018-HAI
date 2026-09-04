[CmdletBinding()]
param(
    [string]$EnvFile = ".env.local"
)

$ErrorActionPreference = "Stop"

$repositoryRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot "..")).Path
if (-not [IO.Path]::IsPathRooted($EnvFile)) {
    $EnvFile = Join-Path $repositoryRoot $EnvFile
}
$EnvFile = [IO.Path]::GetFullPath($EnvFile)
if (-not (Test-Path -LiteralPath $EnvFile -PathType Leaf)) {
    throw "Environment file not found: $EnvFile"
}

function Get-DotEnvValue {
    param([string]$Content, [string]$Name)
    $match = [Regex]::Match($Content, "(?m)^" + [Regex]::Escape($Name) + "=(.*)$")
    if (-not $match.Success) { throw "Missing $Name in $EnvFile" }
    return $match.Groups[1].Value.Trim().Trim("'").Trim('"')
}

$content = [IO.File]::ReadAllText($EnvFile)
$owner = Get-DotEnvValue $content "DB_USER"
$role = Get-DotEnvValue $content "BACKEND_DB_USER"
$password = Get-DotEnvValue $content "BACKEND_DB_PASSWORD"
$database = Get-DotEnvValue $content "AUTOMATION_DB_NAME"

if ($owner -notmatch '^[A-Za-z_][A-Za-z0-9_]{0,62}$' -or
    $role -notmatch '^[A-Za-z_][A-Za-z0-9_]{0,62}$' -or
    $database -notmatch '^[A-Za-z_][A-Za-z0-9_]{0,62}$') {
    throw "DB_USER, BACKEND_DB_USER, and AUTOMATION_DB_NAME must be safe PostgreSQL identifiers."
}
if ([string]::IsNullOrWhiteSpace($password) -or $password -like 'change-this-*') {
    throw "BACKEND_DB_PASSWORD must be a real distinct secret before provisioning the runtime role."
}
if ($role -eq $owner) {
    throw "BACKEND_DB_USER must differ from DB_USER before provisioning a least-privilege runtime role."
}

$roleIdentifier = '"' + $role.Replace('"', '""') + '"'
$databaseIdentifier = '"' + $database.Replace('"', '""') + '"'
$ownerIdentifier = '"' + $owner.Replace('"', '""') + '"'
$passwordLiteral = $password.Replace("'", "''")
$roleLiteral = $role.Replace("'", "''")

$sql = @"
DO `$role`$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '$roleLiteral') THEN
    CREATE ROLE $roleIdentifier LOGIN PASSWORD '$passwordLiteral' NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT;
  ELSE
    ALTER ROLE $roleIdentifier LOGIN PASSWORD '$passwordLiteral' NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT;
  END IF;
END
`$role`$;
GRANT CONNECT ON DATABASE $databaseIdentifier TO $roleIdentifier;
GRANT USAGE ON SCHEMA public TO $roleIdentifier;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO $roleIdentifier;
GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA public TO $roleIdentifier;
ALTER DEFAULT PRIVILEGES FOR ROLE $ownerIdentifier IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO $roleIdentifier;
ALTER DEFAULT PRIVILEGES FOR ROLE $ownerIdentifier IN SCHEMA public GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO $roleIdentifier;
"@

Push-Location $repositoryRoot
try {
    & docker compose --env-file $EnvFile -f docker-compose.local.yml up -d postgres-automation | Out-Host
    if ($LASTEXITCODE -ne 0) { throw "Could not start postgres-automation." }
    $sql | & docker compose --env-file $EnvFile -f docker-compose.local.yml exec -T postgres-automation psql -v ON_ERROR_STOP=1 -U $owner -d $database
    if ($LASTEXITCODE -ne 0) { throw "Could not provision the runtime database role." }
} finally {
    Pop-Location
}

Write-Host "Provisioned DML-only runtime role '$role'. Set DB_MIGRATIONS_ENABLED=false before restarting the backend."
