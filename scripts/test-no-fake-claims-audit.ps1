[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"

$repositoryRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot "..")).Path
$auditScript = Join-Path $PSScriptRoot "no-fake-claims-audit.ps1"

if (-not (Test-Path -LiteralPath $auditScript -PathType Leaf)) {
    throw "Windows no-fake-claims audit is missing: $auditScript"
}

$tokens = $null
$errors = $null
[Management.Automation.Language.Parser]::ParseFile($auditScript, [ref]$tokens, [ref]$errors) | Out-Null
if ($errors.Count -gt 0) {
    throw "PowerShell syntax error in ${auditScript}: $($errors[0].Message)"
}

Push-Location $repositoryRoot
try {
    & $auditScript
    if ($LASTEXITCODE -ne 0) {
        throw "Windows no-fake-claims audit failed with exit code $LASTEXITCODE."
    }
} finally {
    Pop-Location
}

Write-Host "Windows no-fake-claims audit contract passed."
