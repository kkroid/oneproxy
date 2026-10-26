# Static release wiring regression; does not build, install, sign or run products.
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$installer = Get-Content -LiteralPath "$root/trayapp/installer.iss" -Raw
$dll = Get-Content -LiteralPath "$root/cmd/oneproxy-dll/main.go" -Raw
$signing = Get-Content -LiteralPath "$root/scripts/sign-windows.ps1" -Raw
$build = Get-Content -LiteralPath "$root/build.ps1" -Raw
$upgrade = Get-Content -LiteralPath "$root/scripts/test-windows-uninstall-upgrade.ps1" -Raw
if ($installer -notmatch '(?m)^#define AppVersion "([0-9.]+)"') { throw 'Missing installer version' }
$version = $Matches[1]
if ($version -ne '0.8.3') { throw "Unexpected current version: $version" }
if ($installer -notmatch "(?m)^VersionInfoVersion=$([regex]::Escape($version)).0\r?$" -or
    $dll -notmatch ('var appVersion = "' + [regex]::Escape($version) + '"') -or
    -not $signing.Contains("OneProxy-$version-setup.exe") -or
    -not $upgrade.Contains("'fixed-setup' '$version'") -or
    -not $upgrade.Contains(".DisplayVersion -ne '$version'")) {
    throw 'Installer, DLL, signing default or upgrade expectations diverged'
}
# Build remains metadata-driven for Qt/DLL PE resources and installer output.
foreach ($hook in @('APP_VERSION = $metadata.AppVersion',
    "RESOURCE_VERSION = (`$metadata.AppVersion.Replace('.', ',') + ',0')",
    '-X main.appVersion=$($metadata.AppVersion)',
    'VersionInfoVersion=$($metadata.AppVersion).0',
    'dist\OneProxy-$($metadata.AppVersion)-setup.exe')) {
    if (-not $build.Contains($hook)) { throw "Missing build metadata wiring: $hook" }
}
if ($dll -notmatch 'logger.New\(filepath.Join\(dataDir, "logs", "oneproxy.log"\), 0, logger.DefaultBackups\)') {
    throw 'Application logger must use shared default size and volume count'
}
$writer = Get-Content -LiteralPath "$root/internal/logger/writer.go" -Raw
if ($writer -notmatch 'DefaultMaxSize int64 = 64 \* 1024 \* 1024' -or
    $writer -notmatch 'DefaultBackups = 3') { throw 'Unexpected shared log defaults' }
Write-Host "PASS: $version/$version.0 installer/DLL/build/signing/upgrade metadata; shared 64MiB logger default"
