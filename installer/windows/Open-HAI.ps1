[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "Hai-InstallerSupport.ps1")
Assert-HaiDockerReady
Assert-HaiSingleInstallation
Start-Process (Get-HaiUrl)
