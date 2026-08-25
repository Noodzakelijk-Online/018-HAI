[CmdletBinding()]
param(
    [switch]$SkipPayload
)

$ErrorActionPreference = "Stop"

$repositoryRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot "..")).Path
$buildScript = Join-Path $PSScriptRoot "build-windows-installer.ps1"
$installerScript = Join-Path $repositoryRoot "installer\windows\HAI.iss"
$supportScript = Join-Path $repositoryRoot "installer\windows\Hai-InstallerSupport.ps1"
$initializerScript = Join-Path $PSScriptRoot "initialize-windows.ps1"
$runtimeDatabaseRoleScript = Join-Path $PSScriptRoot "provision-runtime-db-role.ps1"
$documentation = Join-Path $repositoryRoot "docs\windows-installer.md"

foreach ($requiredFile in @($buildScript, $installerScript, $supportScript, $initializerScript, $runtimeDatabaseRoleScript, $documentation)) {
    if (-not (Test-Path -LiteralPath $requiredFile -PathType Leaf)) {
        throw "Windows installer contract is missing: $requiredFile"
    }
}

$build = [IO.File]::ReadAllText($buildScript)
$installer = [IO.File]::ReadAllText($installerScript)
$support = [IO.File]::ReadAllText($supportScript)
$initializer = [IO.File]::ReadAllText($initializerScript)
$runtimeDatabaseRole = [IO.File]::ReadAllText($runtimeDatabaseRoleScript)
$startScript = [IO.File]::ReadAllText((Join-Path $repositoryRoot "installer\windows\Start-HAI.ps1"))
$stopScript = [IO.File]::ReadAllText((Join-Path $repositoryRoot "installer\windows\Stop-HAI.ps1"))
$connectorTest = [IO.File]::ReadAllText((Join-Path $repositoryRoot "installer\windows\Test-HAI-LocalConnector.ps1"))
$hostRuntimeWorker = [IO.File]::ReadAllText((Join-Path $repositoryRoot "installer\windows\Run-HAI-DeepSeekBridge.ps1"))
$ngrokStart = [IO.File]::ReadAllText((Join-Path $repositoryRoot "scripts\start-ngrok.ps1"))
$exampleEnvironment = [IO.File]::ReadAllText((Join-Path $repositoryRoot ".env.example"))
$secretGenerator = [IO.File]::ReadAllText((Join-Path $repositoryRoot "scripts\generate-secrets.sh"))
$docs = [IO.File]::ReadAllText($documentation)
$gitignore = [IO.File]::ReadAllText((Join-Path $repositoryRoot ".gitignore"))

if ($gitignore -notmatch [Regex]::Escape("/installer/release/")) {
    throw "Generated installer release artifacts must be ignored by Git."
}

foreach ($script in @(
    $buildScript,
    $supportScript,
    (Join-Path $repositoryRoot "installer\windows\Start-HAI.ps1"),
    (Join-Path $repositoryRoot "installer\windows\Stop-HAI.ps1"),
    (Join-Path $repositoryRoot "installer\windows\HAI-Status.ps1"),
    (Join-Path $repositoryRoot "installer\windows\Test-HAI-LocalConnector.ps1"),
    (Join-Path $repositoryRoot "installer\windows\Run-HAI-DeepSeekBridge.ps1"),
    (Join-Path $repositoryRoot "installer\windows\Open-HAI.ps1"),
    $runtimeDatabaseRoleScript
)) {
    $tokens = $null
    $errors = $null
    [Management.Automation.Language.Parser]::ParseFile($script, [ref]$tokens, [ref]$errors) | Out-Null
    if ($errors.Count -gt 0) {
        throw "PowerShell syntax error in ${script}: $($errors[0].Message)"
    }
}

foreach ($required in @(
    "-ExcludeRelativePath"
)) {
    if ($build -notmatch [Regex]::Escape($required)) {
        throw "Installer build contract is missing '$required'."
    }
}

foreach ($required in @(
    "AllowDirtyWorktree",
    "status --porcelain=v1 --untracked-files=all",
    "Refusing to build a release installer from a dirty worktree"
)) {
    if ($build -notmatch [Regex]::Escape($required)) {
        throw "Installer release-integrity contract is missing '$required'."
    }
}

if ($build -match [Regex]::Escape('ls-files --cached --others --exclude-standard)')) {
    throw "Installer staging must not package arbitrary nonignored files from a developer checkout."
}
foreach ($required in @(
    "installer/windows",
    "scripts/build-windows-installer.ps1",
    "docs/windows-installer.md"
)) {
    if ($build -notmatch [Regex]::Escape($required)) {
        throw "Installer staging allowlist is missing '$required'."
    }
}

foreach ($required in @(
    "initialize-windows.ps1",
    "docker compose",
    "Docker Desktop",
    "com.docker.compose.project=018-hai",
    "HAI environment initialization failed"
)) {
    if ($support -notmatch [Regex]::Escape($required)) {
        throw "Installer runtime contract is missing '$required'."
    }
}

foreach ($required in @(
    "hai-dsh-bridge.exe",
    "./cmd/hai-dsh-bridge",
    "GOOS=windows",
    "golang:1.25.13"
)) {
    if ($build -notmatch [Regex]::Escape($required)) {
        throw "Installer build contract is missing bundled host-runtime worker support: $required"
    }
}

foreach ($required in @(
    "Start-HaiHostRuntimeWorker",
    "Stop-HaiHostRuntimeWorker",
    "Get-HaiHostRuntimeWorkerStatus",
    "Test-HaiHostRuntimeWorkerProcess",
    "Win32_Process",
    "pid record does not reference the HAI runtime worker",
    "HAI_HOST_RUNTIME_BRIDGE_TOKEN",
    "DEEPSEEK_HARNESS_ENABLED",
    "DEEPSEEK_HARNESS_EXECUTION_ENABLED",
    "HAI_HOST_RUNTIME_BRIDGE_URL",
    "DEEPSEEK_HARNESS_WORKSPACE_KEY",
    'Get-Command $executable'
)) {
    if ($support -notmatch [Regex]::Escape($required)) {
        throw "Installer support contract is missing host-runtime lifecycle control: $required"
    }
}

if ($initializer -notmatch [Regex]::Escape('HAI_A2A_LOCAL_PORT') -or
    $initializer -notmatch [Regex]::Escape('127.0.0.1:$a2aLocalPort/api/v1/a2a')) {
    throw "The first-run initializer does not configure the separate local A2A connector."
}

if ($startScript -notmatch [Regex]::Escape('--profile local-a2a')) {
    throw "The installed start command must activate the local A2A connector profile."
}

if ($stopScript -notmatch [Regex]::Escape('Assert-HaiSingleInstallation')) {
    throw "The installed stop command must refuse to manage another HAI installation."
}

foreach ($required in @(
    "HAI_A2A_BRIDGE_TOKEN",
    "A2A-Version",
    "supportedInterfaces",
    'Planning endpoint: $($planningInterface.url)',
    "SendMessage",
    "TASK_STATE_COMPLETED",
    "hai-controlled-planning-proposal"
)) {
    if ($connectorTest -notmatch [Regex]::Escape($required)) {
        throw "The local connector diagnostic does not verify '$required'."
    }
}

if ($connectorTest -match [Regex]::Escape('Planning endpoint: $($agentCard.url)')) {
    throw "The local connector diagnostic reports the removed Agent Card URL field instead of its advertised planning interface."
}

if ($initializer -notmatch [Regex]::Escape('GATEWAY_HOST_BIND') -or
    $initializer -notmatch [Regex]::Escape('"127.0.0.1"')) {
    throw "The first-run initializer does not enforce a loopback gateway."
}

$initializerEnvironmentKeys = [Regex]::Matches($initializer, 'Set-DotEnvValue\s+\$content\s+"([A-Z0-9_]+)"') |
    ForEach-Object { $_.Groups[1].Value } |
    Select-Object -Unique
foreach ($key in $initializerEnvironmentKeys) {
    if ($exampleEnvironment -notmatch "(?m)^$([Regex]::Escape($key))=") {
        throw "The first-run initializer writes '$key', but .env.example does not define it."
    }
}

foreach ($required in @(
    "BACKEND_DB_USER",
    "BACKEND_DB_PASSWORD",
    "DB_MIGRATIONS_ENABLED"
)) {
    if ($exampleEnvironment -notmatch "(?m)^$([Regex]::Escape($required))=") {
        throw "The least-privilege database contract is missing '$required' from .env.example."
    }
    if ($initializer -notmatch [Regex]::Escape($required)) {
        throw "The Windows initializer does not configure '$required'."
    }
}

foreach ($required in @(
    "BACKEND_DB_USER",
    "BACKEND_DB_PASSWORD",
    "ALTER DEFAULT PRIVILEGES",
    "NOSUPERUSER",
    "NOCREATEDB",
    "NOCREATEROLE",
    "DB_MIGRATIONS_ENABLED=false"
)) {
    if ($runtimeDatabaseRole -notmatch [Regex]::Escape($required)) {
        throw "The runtime database role provisioner is missing '$required'."
    }
}

foreach ($required in @(
    "ComposeProjectName",
    "Assert-HaiComposeProjectAvailable",
    "com.docker.compose.project.working_dir",
    "Another HAI installation already owns Docker project"
)) {
    if ($initializer -notmatch [Regex]::Escape($required)) {
        throw "The first-run initializer does not protect Docker project ownership: '$required'."
    }
}

foreach ($forbidden in @(
    "Copy-Item -Path (Join-Path $repositoryRoot '.env.local')",
    "GATEWAY_HOST_BIND=0.0.0.0",
    "LOCAL_LOGIN_BYPASS_ENABLED=true",
    "-match '(^|/)\\.env($|\\.)'"
)) {
    if ($build -match [Regex]::Escape($forbidden)) {
        throw "Installer build script contains unsafe value '$forbidden'."
    }
}

foreach ($required in @(
    "[Setup]",
    "[Files]",
    "[Icons]",
    "[Run]",
    "HAI Local",
    "Start-HAI.ps1",
    "Stop-HAI.ps1",
    "Open local dashboard"
)) {
    if ($installer -notmatch [Regex]::Escape($required)) {
        throw "Inno Setup contract is missing '$required'."
    }
}

if ($build -notmatch [Regex]::Escape('HAI_INSTALLER_OUTPUT_DIR') -or
    $installer -notmatch [Regex]::Escape('HAI_INSTALLER_OUTPUT_DIR')) {
    throw "Installer output-directory configuration is not wired through the build and Inno Setup scripts."
}

foreach ($forbidden in @(
    ".env.local",
    "db_data_automation",
    "db_data_idp",
    "connected-sources\\*"
)) {
    if ($installer -match [Regex]::Escape($forbidden)) {
        throw "Inno Setup script must not directly package local data: '$forbidden'."
    }
}

foreach ($required in @(
    "Docker Desktop",
    "127.0.0.1",
    "%LOCALAPPDATA%\HAI",
    "Uninstall",
    "does not delete"
)) {
    if ($docs -notmatch [Regex]::Escape($required)) {
        throw "Installer documentation is missing '$required'."
    }
}

foreach ($required in @(
    '{{json .Config.Labels}}',
    'ConvertFrom-Json',
    'COMPOSE_PROJECT_NAME',
    'Assert-HaiComposeOwnership -ProjectName $composeProjectName',
    'Refusing to manage cloud access until ownership can be verified'
)) {
    if ($ngrokStart -notmatch [Regex]::Escape($required)) {
        throw "The cloud-tunnel ownership gate is missing '$required'."
    }
}

foreach ($required in @(
    'DB_PASSWORD=$(secret)',
    'FIRST_RUN_ADMIN_PASSWORD=$(secret)'
)) {
    if ($secretGenerator -notmatch [Regex]::Escape($required)) {
        throw "The cross-platform secret generator is missing '$required'."
    }
}
if ($initializer -notmatch [Regex]::Escape('FIRST_RUN_ADMIN_PASSWORD')) {
    throw "The Windows initializer must configure the first-run owner password."
}

if ($exampleEnvironment -match [Regex]::Escape('DB_PASSWORD=postgres') -or
    $exampleEnvironment -match [Regex]::Escape('FIRST_RUN_ADMIN_PASSWORD=ChangeMe123!')) {
    throw "The safe environment template must not ship a known database or owner password."
}

foreach ($required in @(
    "'DB_PASSWORD'",
    "'FIRST_RUN_ADMIN_PASSWORD'"
)) {
    if ($ngrokStart -notmatch [Regex]::Escape($required)) {
        throw "The cloud-tunnel secret gate is missing '$required'."
    }
}

if ($SkipPayload) {
    Write-Host "Windows installer source contracts passed without regenerating the release payload."
    exit 0
}

& $buildScript -SkipCompile
if ($LASTEXITCODE -ne 0) {
    throw "Installer payload preparation failed."
}
$payloadRoot = Join-Path $repositoryRoot "installer\release\payload"
foreach ($forbiddenPayloadPath in @(
    ".env.local",
    ".env-backend",
    ".env-gateway",
    ".env-idp",
    "connected-sources\private.txt"
)) {
    if (Test-Path -LiteralPath (Join-Path $payloadRoot $forbiddenPayloadPath)) {
        throw "Installer payload contains excluded local data: $forbiddenPayloadPath"
    }
}
if (-not (Test-Path -LiteralPath (Join-Path $payloadRoot ".env.example") -PathType Leaf)) {
    throw "Installer payload is missing the safe environment template."
}
foreach ($requiredPayloadPath in @(
    "docker-compose.local.yml",
    "nginx-config\a2a-local.conf.template",
    "installer\windows\Start-HAI.ps1",
    "installer\windows\Stop-HAI.ps1",
    "installer\windows\HAI-Status.ps1",
    "installer\windows\Test-HAI-LocalConnector.ps1"
)) {
    if (-not (Test-Path -LiteralPath (Join-Path $payloadRoot $requiredPayloadPath) -PathType Leaf)) {
        throw "Installer payload is missing required product file: $requiredPayloadPath"
    }
}

Write-Host "Windows installer behavioral contracts passed."
