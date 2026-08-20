[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"

$repositoryRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot "..")).Path
$requiredFiles = @(
    "scripts\\build-windows-installer.ps1",
    "scripts\\initialize-windows.ps1",
    "installer\\windows\\HAI.iss",
    "installer\\windows\\Hai-InstallerSupport.ps1",
    "installer\\windows\\Start-HAI.ps1",
    "installer\\windows\\Stop-HAI.ps1",
    "installer\\windows\\Open-HAI.ps1",
    "installer\\windows\\HAI-Status.ps1",
    "docs\\windows-installer.md"
)

foreach ($relativePath in $requiredFiles) {
    if (-not (Test-Path -LiteralPath (Join-Path $repositoryRoot $relativePath) -PathType Leaf)) {
        throw "Windows installer contract is missing: $relativePath"
    }
}

$build = [IO.File]::ReadAllText((Join-Path $repositoryRoot "scripts\\build-windows-installer.ps1"))
$support = [IO.File]::ReadAllText((Join-Path $repositoryRoot "installer\\windows\\Hai-InstallerSupport.ps1"))
$initializer = [IO.File]::ReadAllText((Join-Path $repositoryRoot "scripts\\initialize-windows.ps1"))
$installer = [IO.File]::ReadAllText((Join-Path $repositoryRoot "installer\\windows\\HAI.iss"))

foreach ($required in @("Docker Desktop", "docker compose", "127.0.0.1", "Wait-HaiReady", "Uninstall")) {
    if (($build + $support + $initializer + $installer) -notmatch [Regex]::Escape($required)) {
        throw "Windows installer contract is missing '$required'."
    }
}

foreach ($forbidden in @(".env.local", "db_data_automation", "db_data_idp", "GATEWAY_HOST_BIND=0.0.0.0")) {
    if ($installer -match [Regex]::Escape($forbidden)) {
        throw "Installer must not package local data or expose the gateway: $forbidden"
    }
}

Write-Host "Windows installer behavioral contract passed."
