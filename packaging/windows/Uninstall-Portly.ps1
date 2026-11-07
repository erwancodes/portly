[CmdletBinding()]
param(
    [string]$InstallRoot = (Join-Path $env:LOCALAPPDATA 'Portly'),
    [switch]$KeepConfig
)

$ErrorActionPreference = 'Stop'
$binRoot = Join-Path $InstallRoot 'bin'
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$pathEntries = @($userPath -split ';' | Where-Object { $_ -and $_.Trim() })
$remaining = @($pathEntries | Where-Object { $_.TrimEnd('\') -ine $binRoot.TrimEnd('\') })
[Environment]::SetEnvironmentVariable('Path', ($remaining -join ';'), 'User')

$shortcutPath = Join-Path ([Environment]::GetFolderPath('StartMenu')) 'Programs\Portly\Portly.lnk'
if (Test-Path -LiteralPath $shortcutPath) {
    Remove-Item -LiteralPath $shortcutPath -Force
}
$shortcutDirectory = Split-Path -Parent $shortcutPath
if (Test-Path -LiteralPath $shortcutDirectory) {
    Remove-Item -LiteralPath $shortcutDirectory -Force -Recurse
}

$skillRoot = Join-Path $env:USERPROFILE '.agents\skills\portly'
if (Test-Path -LiteralPath $skillRoot) {
    Write-Warning "Kept agent skill at $skillRoot. Remove it manually if it was installed by Portly."
}

if (Test-Path -LiteralPath $InstallRoot) {
    Remove-Item -LiteralPath $InstallRoot -Force -Recurse
}
if (-not $KeepConfig) {
    $portlyHome = Join-Path $env:USERPROFILE '.config\portly'
    if (Test-Path -LiteralPath $portlyHome) {
        Write-Warning "Kept Portly config and logs at $portlyHome."
    }
}

Write-Output 'Portly uninstalled.'
