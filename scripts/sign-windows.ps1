<#
.SYNOPSIS
  Sign and verify OneProxy Windows release artifacts.

  The default mode is a dry-run and never modifies files. Signing requires an
  explicit certificate thumbprint or PFX path and a trusted timestamp URL.
  sing-box.exe is deliberately excluded because it is a third-party artifact.
#>
[CmdletBinding()]
param(
    [string]$BuildDir = (Join-Path $PSScriptRoot '..\trayapp\build'),
    [string]$Installer = (Join-Path $PSScriptRoot '..\dist\OneProxy-0.8.1-setup.exe'),
    [string]$Uninstaller,
    [string]$SignTool,
    [string]$CertificateThumbprint,
    [string]$PfxPath,
    [string]$PfxPassword,
    [string]$TimestampUrl = 'http://timestamp.digicert.com',
    [string]$SingBox = (Join-Path $PSScriptRoot '..\bin\sing-box.exe'),
    [string]$SingBoxSourceUrl = 'https://github.com/SagerNet/sing-box/releases/tag/v1.13.14',
    [string]$SingBoxVersion = '1.13.14',
    [switch]$Sign,
    [switch]$VerifyOnly,
    [switch]$PrePackage,
    [switch]$StrictVerify,
    [switch]$SelfTest,
    [string]$Manifest = (Join-Path $PSScriptRoot '..\dist\windows-signing-sha256.txt')
)

$ErrorActionPreference = 'Stop'
$targets = @(
    (Join-Path $BuildDir 'oneproxy-tray.exe'),
    (Join-Path $BuildDir 'oneproxy.dll'),
    (Join-Path $BuildDir 'oneproxy-upgrade.dll'),
    $Installer
)
if ($PrePackage) { $targets = @($targets | Select-Object -First 3) }
if ($Uninstaller) { $targets += $Uninstaller }
$targets = @($targets | Where-Object { $_ } | ForEach-Object { [IO.Path]::GetFullPath($_) })

function Get-SignToolPath {
    if ($SignTool) { return $SignTool }
    $found = Get-Command signtool.exe -ErrorAction SilentlyContinue
    if ($found) { return $found.Source }
    throw 'signtool.exe not found; pass -SignTool or add Windows SDK to PATH.'
}

function Show-Status([string]$Path) {
    if (-not (Test-Path -LiteralPath $Path)) {
        [PSCustomObject]@{ Path = $Path; Exists = $false; Status = 'Missing'; Timestamp = 'Unknown'; SHA256 = '' }
        return
    }
    $sig = Get-AuthenticodeSignature -LiteralPath $Path
    $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash
    $timestampPresent = [bool]$sig.TimeStamperCertificate
    $timestamp = if ($timestampPresent) { $sig.TimeStamperCertificate.Subject } else { 'Missing' }
    [PSCustomObject]@{ Path = $Path; Exists = $true; Status = $sig.Status; Timestamp = $timestamp; TimestampValid = $timestampPresent; SHA256 = $hash }
}

function Test-SigningDecision($Item) {
    return -not $Item.Exists -or $Item.Status -ne 'Valid' -or -not $Item.TimestampValid
}

function Test-SingBoxConflict([string]$Path) {
    if ([IO.Path]::GetFileName($Path).Equals('sing-box.exe', [StringComparison]::OrdinalIgnoreCase)) { return $true }
    if (-not (Test-Path -LiteralPath $SingBox)) { return $false }
    $target = [IO.Path]::GetFullPath($Path)
    $sing = [IO.Path]::GetFullPath($SingBox)
    if ($target.Equals($sing, [StringComparison]::OrdinalIgnoreCase)) { return $true }
    if (-not (Test-Path -LiteralPath $Path)) { return $false }
    return (Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash -eq (Get-FileHash -Algorithm SHA256 -LiteralPath $SingBox).Hash
}

if ($SelfTest) {
    $missingTimestamp = [PSCustomObject]@{ Exists = $true; Status = 'Valid'; TimestampValid = $false }
    if (-not (Test-SigningDecision $missingTimestamp)) { throw 'Self-test failed: missing timestamp was accepted.' }
    if (Test-SingBoxConflict $SingBox) { Write-Host 'Self-test passed: sing-box identity is excluded from signing.' }
    Write-Host 'Self-test passed: a valid signature without a trusted timestamp is rejected.'
    exit 0
}

if ($Sign -and $VerifyOnly) { throw 'Use either -Sign or -VerifyOnly, not both.' }
if ($StrictVerify -and $Sign) { throw 'StrictVerify is read-only; do not combine it with Sign.' }
if ($Sign) {
    if ($CertificateThumbprint -and $PfxPath) { throw 'Select exactly one certificate source.' }
    if (-not $CertificateThumbprint -and -not $PfxPath) { throw '-Sign requires -CertificateThumbprint or -PfxPath.' }
    if ([string]::IsNullOrWhiteSpace($TimestampUrl)) { throw '-Sign requires a trusted -TimestampUrl.' }
    $conflicts = @($targets | Where-Object { Test-SingBoxConflict $_ })
    if ($conflicts.Count -gt 0) { throw ('Refusing to sign third-party sing-box identity before any signtool call: ' + ($conflicts -join ', ')) }
    foreach ($path in $targets) {
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "Signing target is missing: $path" }
    }
    $tool = Get-SignToolPath
    foreach ($path in $targets) {
        $args = @('sign', '/fd', 'SHA256', '/tr', $TimestampUrl, '/td', 'SHA256')
        if ($CertificateThumbprint) { $args += @('/sha1', $CertificateThumbprint) }
        if ($PfxPath) {
            $args += @('/f', $PfxPath)
            if ($PfxPassword) { $args += @('/p', $PfxPassword) }
        }
        $args += $path
        & $tool @args
        if ($LASTEXITCODE -ne 0) { throw "signtool failed for $path (exit $LASTEXITCODE)" }
    }
}

$status = @($targets | ForEach-Object { Show-Status $_ })
$status | Format-List
if ($status.Exists -contains $false) { Write-Warning 'One or more release targets are missing; build or installer output is required.' }

$manifestDir = Split-Path -Parent $Manifest
New-Item -ItemType Directory -Force -Path $manifestDir | Out-Null
$manifestLines = @($status | Where-Object Exists | ForEach-Object { "$($_.SHA256)  $($_.Path)" })
$manifestLines += "SING_BOX_VERSION_DECLARED=$SingBoxVersion"
if (Test-Path -LiteralPath $SingBox) {
    $singHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $SingBox).Hash
    $manifestLines += "SING_BOX_SOURCE=$SingBoxSourceUrl"
    $manifestLines += "SING_BOX_SHA256=$singHash  $SingBox"
} else {
    $manifestLines += "SING_BOX_SOURCE=$SingBoxSourceUrl"
    $manifestLines += 'SING_BOX_SHA256=Missing (third-party artifact not bundled or signed by this script)'
}
$manifestLines | Set-Content -Encoding UTF8 -Path $Manifest
Write-Host "SHA256 manifest: $Manifest"

if ($Sign -or $StrictVerify) {
    $invalid = @($status | Where-Object { Test-SigningDecision $_ })
    if ($invalid.Count -gt 0) { throw ('Signature or trusted timestamp verification failed: ' + (($invalid | ForEach-Object Path) -join ', ')) }
    $tool = Get-SignToolPath
    foreach ($path in $targets) {
        & $tool verify /pa /all /tw $path
        if ($LASTEXITCODE -ne 0) { throw "signtool trust/timestamp verification failed: $path" }
    }
    Write-Host 'All requested targets passed Authenticode and signtool trust/timestamp verification.'
} else {
    Write-Host 'Inspection complete; no files were modified by signing. This is not a signed-release gate.'
}
