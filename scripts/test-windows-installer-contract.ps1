[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"

$repositoryRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot "..")).Path
$buildScript = Join-Path $PSScriptRoot "build-windows-installer.ps1"
$installerScript = Join-Path $repositoryRoot "installer\windows\HAI.iss"
$supportScript = Join-Path $repositoryRoot "installer\windows\Hai-InstallerSupport.ps1"
$initializerScript = Join-Path $PSScriptRoot "initialize-windows.ps1"
$documentation = Join-Path $repositoryRoot "docs\windows-installer.md"

foreach ($requiredFile in @($buildScript, $installerScript, $supportScript, $initializerScript, $documentation)) {
    if (-not (Test-Path -LiteralPath $requiredFile -PathType Leaf)) {
        throw "Windows installer contract is missing: $requiredFile"
    }
}

$build = [IO.File]::ReadAllText($buildScript)
$installer = [IO.File]::ReadAllText($installerScript)
$support = [IO.File]::ReadAllText($supportScript)
$initializer = [IO.File]::ReadAllText($initializerScript)
$startScript = [IO.File]::ReadAllText((Join-Path $repositoryRoot "installer\windows\Start-HAI.ps1"))
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
    (Join-Path $repositoryRoot "installer\windows\Open-HAI.ps1")
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

if ($initializer -notmatch [Regex]::Escape('HAI_A2A_LOCAL_PORT') -or
    $initializer -notmatch [Regex]::Escape('127.0.0.1:$a2aLocalPort/api/v1/a2a')) {
    throw "The first-run initializer does not configure the separate local A2A connector."
}

if ($startScript -notmatch [Regex]::Escape('--profile local-a2a')) {
    throw "The installed start command must activate the local A2A connector profile."
}

if ($initializer -notmatch [Regex]::Escape('GATEWAY_HOST_BIND') -or
    $initializer -notmatch [Regex]::Escape('"127.0.0.1"')) {
    throw "The first-run initializer does not enforce a loopback gateway."
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
