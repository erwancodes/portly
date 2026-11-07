[CmdletBinding()]
param(
    [string]$SourceRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path,
    [string]$InstallRoot = (Join-Path $env:LOCALAPPDATA 'Portly'),
    [switch]$SkipPath,
    [switch]$SkipSkill
)

$ErrorActionPreference = 'Stop'

function Copy-RequiredFile([string]$Source, [string]$Destination) {
    if (-not (Test-Path -LiteralPath $Source -PathType Leaf)) {
        throw "Missing release file: $Source"
    }
    Copy-Item -LiteralPath $Source -Destination $Destination -Force
}

$windowsBinary = Join-Path $SourceRoot 'windows\Portly.exe'
$cliBinary = Join-Path $SourceRoot 'cli\portly.exe'
$binRoot = Join-Path $InstallRoot 'bin'
$dataRoot = Join-Path $InstallRoot 'data'

New-Item -ItemType Directory -Force -Path $InstallRoot, $binRoot, $dataRoot | Out-Null
Copy-RequiredFile $windowsBinary (Join-Path $InstallRoot 'Portly.exe')
Copy-RequiredFile $cliBinary (Join-Path $binRoot 'portly.exe')
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'Uninstall-Portly.ps1') -Destination (Join-Path $InstallRoot 'Uninstall-Portly.ps1') -Force

$portlyHome = Join-Path $env:USERPROFILE '.config\portly'
New-Item -ItemType Directory -Force -Path (Join-Path $portlyHome 'logs') | Out-Null

$managedRule = @'
<!-- portly:managed-rule:start -->
## Development servers

- Always use Portly (`portly ...`) to start, stop, restart, inspect, or keep local development servers running.
- Start with `portly status`. Use `portly status --details` only for the full inventory and metrics, and `--json` only for machine-readable fields. Reuse a healthy managed server; if an in-scope server is running outside Portly, register it and use `portly take-over <project/server> --json`.
- For long-lived or reusable work, create a project and server.
- For builds, tests, code generation, previews, demos, and other bounded one-off work, run `portly temp '<command>' --path <folder> --timeout 30m`, then `portly wait <job-id>`. `temp` returns immediately with an ID; `wait` prints captured logs and exits with the command's real code. A timeout kills the whole process group and exits with code `124`.
- Never launch persistent development servers directly, in the background, or through another supervisor.
<!-- portly:managed-rule:end -->
'@

function Install-GlobalRules {
    $startMarker = '<!-- portly:managed-rule:start -->'
    $endMarker = '<!-- portly:managed-rule:end -->'
    $ruleFiles = @(
        (Join-Path $env:USERPROFILE '.agents\AGENTS.md'),
        (Join-Path $env:USERPROFILE '.claude\CLAUDE.md')
    )
    foreach ($file in $ruleFiles) {
        New-Item -ItemType Directory -Force -Path (Split-Path -Parent $file) | Out-Null
        $contents = if (Test-Path -LiteralPath $file) { Get-Content -LiteralPath $file -Raw } else { '' }
        $start = $contents.IndexOf($startMarker, [StringComparison]::Ordinal)
        $end = if ($start -ge 0) { $contents.IndexOf($endMarker, $start, [StringComparison]::Ordinal) } else { -1 }
        if ($start -ge 0 -and $end -ge 0) {
            $end += $endMarker.Length
            $updated = $contents.Substring(0, $start) + $managedRule.Trim() + $contents.Substring($end)
        } else {
            $prefix = if ([String]::IsNullOrWhiteSpace($contents)) { '' } else { $contents.TrimEnd() + "`r`n`r`n" }
            $updated = $prefix + $managedRule.Trim() + "`r`n"
        }
        if ($updated -cne $contents) {
            Set-Content -LiteralPath $file -Value $updated -Encoding utf8 -NoNewline
        }
    }
}

if (-not $SkipPath) {
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $pathEntries = @($userPath -split ';' | Where-Object { $_ -and $_.Trim() })
    if (-not ($pathEntries | Where-Object { $_.TrimEnd('\') -ieq $binRoot.TrimEnd('\') })) {
        [Environment]::SetEnvironmentVariable('Path', (($pathEntries + $binRoot) -join ';'), 'User')
        Write-Output "Added $binRoot to the user PATH."
    }
}

if (-not $SkipSkill) {
    $sourceSkill = Join-Path $SourceRoot 'skills\portly'
    $skillRoot = Join-Path $env:USERPROFILE '.agents\skills\portly'
    if (Test-Path -LiteralPath $sourceSkill -PathType Container) {
        if (Test-Path -LiteralPath $skillRoot) {
            Write-Warning "Kept existing agent skill at $skillRoot; installer will not overwrite personal instructions."
        } else {
            New-Item -ItemType Directory -Force -Path (Split-Path -Parent $skillRoot) | Out-Null
            Copy-Item -LiteralPath $sourceSkill -Destination $skillRoot -Recurse
        }
    }
}

Install-GlobalRules

$startMenu = Join-Path ([Environment]::GetFolderPath('StartMenu')) 'Programs\Portly'
New-Item -ItemType Directory -Force -Path $startMenu | Out-Null
$shortcutPath = Join-Path $startMenu 'Portly.lnk'
$shell = New-Object -ComObject WScript.Shell
$shortcut = $shell.CreateShortcut($shortcutPath)
$shortcut.TargetPath = Join-Path $InstallRoot 'Portly.exe'
$shortcut.WorkingDirectory = $InstallRoot
$shortcut.Description = 'Portly local development supervisor'
$shortcut.Save()

Write-Output "Portly installed to $InstallRoot"
Write-Output "CLI: $binRoot\portly.exe"
Write-Output "Config: $portlyHome"
