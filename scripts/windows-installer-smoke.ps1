param(
    [Parameter(Mandatory = $true)][string]$Installer,
    [Parameter(Mandatory = $true)][string]$Version
)

$ErrorActionPreference = 'Stop'
if ($env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or [Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) {
    throw 'Installer smoke tests require a disposable GitHub-hosted Windows runner'
}

$installerPath = (Resolve-Path -LiteralPath $Installer).Path
$workspace = Join-Path $env:RUNNER_TEMP "infinity-iac-smoke-$([guid]::NewGuid())"
New-Item -ItemType Directory -Path $workspace | Out-Null
$environmentKey = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $true)
$originalPath = $environmentKey.GetValue('Path', $null, [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
$originalKind = if ($null -ne $originalPath) { $environmentKey.GetValueKind('Path') } else { [Microsoft.Win32.RegistryValueKind]::ExpandString }
$originalConfig = $env:INFINITY_IAC_CONFIG
$env:INFINITY_IAC_CONFIG = Join-Path $workspace 'offline-config.json'

try {
    foreach ($scenario in @('existing-path', 'empty-segments', 'no-path-task', 'preexisting-entry')) {
        $installDir = Join-Path $workspace $scenario
        $baseline = [string]$originalPath
        if ($scenario -eq 'empty-segments') { $baseline = ';%USERPROFILE%\test-only;;C:\Windows;' }
        if ($scenario -eq 'preexisting-entry') { $baseline += ";$installDir" }
        $environmentKey.SetValue('Path', $baseline, $originalKind)
        $tasks = if ($scenario -eq 'no-path-task') { '/TASKS=' } else { '/TASKS=addtopath' }
        $expected = if ($scenario -in @('no-path-task', 'preexisting-entry')) { $baseline } else { "$baseline;$installDir" }
        $uninstaller = Join-Path $installDir 'unins000.exe'
        try {
            $setup = Start-Process -FilePath $installerPath -ArgumentList @('/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', '/NOICONS', $tasks, "/DIR=`"$installDir`"", "/LOG=`"$(Join-Path $workspace "$scenario-install.log")`"") -Wait -PassThru
            if ($setup.ExitCode -ne 0) { throw "$scenario installer failed with exit code $($setup.ExitCode)" }
            $installedPath = $environmentKey.GetValue('Path', $null, [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
            if ($installedPath -cne $expected) { throw "$scenario changed the user PATH unexpectedly" }
            $binary = Join-Path $installDir 'infinity-iac.exe'
            $reported = & $binary --version
            if ($LASTEXITCODE -ne 0 -or $reported.Trim() -ne $Version) { throw "$scenario installed version does not match $Version" }
            $catalog = & $binary list recipes --output json
            if ($LASTEXITCODE -ne 0) { throw "$scenario installed catalog failed" }
            $items = ($catalog | ConvertFrom-Json).recipes
            if (@($items).Count -ne 137) { throw "$scenario installed catalog is incomplete" }
            $rendered = & $binary recipe render --name oc-hunter
            if ($LASTEXITCODE -ne 0 -or -not ($rendered -match 'oc-hunter')) { throw "$scenario installed recipe rendering failed" }
            if (Test-Path -LiteralPath $env:INFINITY_IAC_CONFIG) { throw 'Offline smoke wrote configuration' }
            if (-not (Test-Path -LiteralPath $uninstaller)) { throw "$scenario did not install an uninstaller" }
        } finally {
            if (Test-Path -LiteralPath $uninstaller) {
                $uninstall = Start-Process -FilePath $uninstaller -ArgumentList @('/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', "/LOG=`"$(Join-Path $workspace "$scenario-uninstall.log")`"") -Wait -PassThru
                if ($uninstall.ExitCode -ne 0) { throw "$scenario uninstaller failed with exit code $($uninstall.ExitCode)" }
            }
            if (Test-Path -LiteralPath (Join-Path $installDir 'infinity-iac.exe')) { throw "$scenario uninstall left the binary behind" }
            $restoredPath = $environmentKey.GetValue('Path', $null, [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
            if ($restoredPath -cne $baseline) {
                throw "$scenario uninstall changed the user PATH: expected '$baseline', got '$restoredPath'"
            }
        }
        Write-Output "$scenario passed: install, version, catalog, rendering, uninstall, and exact PATH restoration"
    }
} finally {
    if ($null -eq $originalPath) {
        $environmentKey.DeleteValue('Path', $false)
    } else {
        $environmentKey.SetValue('Path', $originalPath, $originalKind)
    }
    $environmentKey.Dispose()
    $env:INFINITY_IAC_CONFIG = $originalConfig
}
