# Hash only release binaries/assets, never local user configuration.
[CmdletBinding(DefaultParameterSetName = 'Generate')]
param(
    [string]$Root = (Split-Path $PSScriptRoot -Parent),
    [Parameter(Mandatory, ParameterSetName = 'Generate')][string]$Version,
    [Parameter(Mandatory, ParameterSetName = 'Generate')][string]$Product,
    [Parameter(Mandatory, ParameterSetName = 'Generate')][string]$Publisher,
    [Parameter(Mandatory, ParameterSetName = 'Generate')][string]$BuildCommit,
    [Parameter(Mandatory, ParameterSetName = 'Generate')][bool]$SourceDirty,
    [Parameter(ParameterSetName = 'Generate')][switch]$Installer,
    [Parameter(Mandatory, ParameterSetName = 'Verify')][switch]$Verify,
    [string]$Manifest = 'dist/release-manifest.json'
)
$ErrorActionPreference = 'Stop'
$Root = (Resolve-Path -LiteralPath $Root).Path
$manifestPath = Join-Path $Root $Manifest

function Resolve-Artifact([string]$RelativePath) {
    $path = [IO.Path]::GetFullPath((Join-Path $Root $RelativePath))
    if (-not $path.StartsWith($Root.TrimEnd('\', '/') + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Artifact outside release root: $RelativePath"
    }
    return $path
}

function Test-ProductResource([string]$Path, [string]$ExpectedVersion, [string]$ExpectedProduct, [string]$ExpectedPublisher) {
    $info = [Diagnostics.FileVersionInfo]::GetVersionInfo($Path)
    # Inno Setup pads version strings with spaces in its executable stub.
    if (([string]$info.ProductName).TrimEnd() -ne $ExpectedProduct -or ([string]$info.CompanyName).TrimEnd() -ne $ExpectedPublisher -or
        ([string]$info.ProductVersion).TrimEnd() -ne $ExpectedVersion -or ([string]$info.FileVersion).TrimEnd() -ne "$ExpectedVersion.0") {
        throw "Release version resource mismatch: $Path"
    }
}

if ($Verify) {
    $release = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
    if ($release.schemaVersion -ne 1 -or $release.buildCommit -notmatch '^[0-9a-f]{40}$' -or
        $release.version -notmatch '^\d+\.\d+\.\d+$' -or @($release.artifacts).Count -eq 0) {
        throw 'Invalid release manifest'
    }
    foreach ($artifact in $release.artifacts) {
        $path = Resolve-Artifact $artifact.path
        if ((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash -ne $artifact.sha256 -or
            (Get-Item -LiteralPath $path).Length -ne $artifact.size) {
            throw "Release hash/size mismatch: $($artifact.path)"
        }
        if ($artifact.productOwned) {
            Test-ProductResource $path $release.version $release.product $release.publisher
        }
    }
    Write-Host "Verified $(@($release.artifacts).Count) release artifacts: $manifestPath"
    return
}

if ($BuildCommit -notmatch '^[0-9a-f]{40}$') { throw 'BuildCommit must be a full Git commit ID.' }
$ownedPaths = @('trayapp/build/oneproxy-tray.exe', 'trayapp/build/oneproxy.dll')
if ($Installer) { $ownedPaths += "dist/OneProxy-$Version-setup.exe" }
foreach ($relative in $ownedPaths) {
    Test-ProductResource (Resolve-Artifact $relative) $Version $Product $Publisher
}
# Match the installer's payload locations, not CMake's recursive build tree.
$payloadRoot = Join-Path $Root 'trayapp/build'
$rootPatterns = @('oneproxy-tray.exe', 'oneproxy.exe', 'oneproxy.dll', 'oneproxy-upgrade.dll', 'dxcompiler.dll', 'dxil.dll', 'vcruntime140*.dll', 'msvcp140*.dll',
    'concrt140.dll', 'vccorlib140.dll', 'Qt6Core.dll', 'Qt6Gui.dll', 'Qt6Widgets.dll',
    'Qt6Network.dll', 'Qt6Svg.dll', 'green.ico', 'yellow.ico', 'red.ico')
$payloadFiles = @(foreach ($pattern in $rootPatterns) {
    Get-ChildItem -LiteralPath $payloadRoot -Filter $pattern -File
})
foreach ($directory in @('bin', 'platforms', 'styles', 'imageformats', 'iconengines', 'tls', 'generic', 'networkinformation')) {
    $path = Join-Path $payloadRoot $directory
    if (Test-Path -LiteralPath $path -PathType Container) {
        $payloadFiles += @(Get-ChildItem -LiteralPath $path -File -Recurse |
            Where-Object { $_.Extension -in @('.exe', '.dll', '.srs', '.ico') })
    }
}
$paths = @($payloadFiles |
    ForEach-Object { $_.FullName.Substring($Root.Length + 1).Replace('\', '/') })
if ($Installer) { $paths += "dist/OneProxy-$Version-setup.exe" }
$artifacts = @(foreach ($relative in ($paths | Sort-Object -Unique)) {
    $path = Resolve-Artifact $relative
    [ordered]@{
        path = $relative
        sha256 = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
        size = (Get-Item -LiteralPath $path).Length
        productOwned = $relative -in $ownedPaths
    }
})
$release = [ordered]@{
    schemaVersion = 1
    product = $Product
    publisher = $Publisher
    version = $Version
    buildCommit = $BuildCommit
    sourceDirty = $SourceDirty
    generatedAtUtc = [DateTime]::UtcNow.ToString('o')
    artifacts = $artifacts
}
New-Item -ItemType Directory -Force -Path (Split-Path $manifestPath -Parent) | Out-Null
$release | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $manifestPath -Encoding UTF8
Write-Host "Release SHA256 manifest: $manifestPath"
