# Integration checks against built PE files; never run or install the products.
param([string]$Root = (Split-Path $PSScriptRoot -Parent))
$ErrorActionPreference = 'Stop'
$Root = (Resolve-Path -LiteralPath $Root).Path
$hashScript = Join-Path $PSScriptRoot 'release-hashes.ps1'
$release = Get-Content -LiteralPath (Join-Path $Root 'dist/release-manifest.json') -Raw | ConvertFrom-Json
$fixture = Join-Path ([IO.Path]::GetTempPath()) ('oneproxy-hash-test-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path "$fixture/trayapp/build", "$fixture/dist" -Force | Out-Null
try {
    foreach ($file in @('oneproxy-tray.exe', 'oneproxy.dll')) {
        Copy-Item -LiteralPath "$Root/trayapp/build/$file" -Destination "$fixture/trayapp/build/$file"
    }
    Set-Content -LiteralPath "$fixture/trayapp/build/config.json" -Value 'test-only-placeholder'
    Set-Content -LiteralPath "$fixture/dist/OneProxy-old-setup.exe" -Value 'stale'
    New-Item -ItemType Directory -Path "$fixture/trayapp/build/CMakeFiles/CompilerIdCXX", "$fixture/trayapp/build/platforms", "$fixture/trayapp/build/bin" -Force | Out-Null
    $compilerProbe = "$fixture/trayapp/build/CMakeFiles/CompilerIdCXX/CMakeCXXCompilerId.exe"
    Set-Content -LiteralPath $compilerProbe -Value 'compiler-probe'
    Set-Content -LiteralPath "$fixture/trayapp/build/build-helper.exe" -Value 'build-helper'
    Set-Content -LiteralPath "$fixture/trayapp/build/oneproxy.exe" -Value 'test-cli'
    Set-Content -LiteralPath "$fixture/trayapp/build/dxcompiler.dll" -Value 'test-dxcompiler'
    Set-Content -LiteralPath "$fixture/trayapp/build/dxil.dll" -Value 'test-dxil'
    Set-Content -LiteralPath "$fixture/trayapp/build/platforms/qwindows.dll" -Value 'test-plugin'
    Set-Content -LiteralPath "$fixture/trayapp/build/bin/sing-box.exe" -Value 'test-dependency'
    $arguments = @{
        Root = $fixture; Version = $release.version; Product = $release.product
        Publisher = $release.publisher; BuildCommit = $release.buildCommit; SourceDirty = $true
    }
    & $hashScript @arguments
    & $hashScript -Root $fixture -Verify
    $actual = Get-Content -LiteralPath "$fixture/dist/release-manifest.json" -Raw | ConvertFrom-Json
    $expectedPaths = @('trayapp/build/oneproxy-tray.exe', 'trayapp/build/oneproxy.exe', 'trayapp/build/oneproxy.dll', 'trayapp/build/dxcompiler.dll', 'trayapp/build/dxil.dll', 'trayapp/build/platforms/qwindows.dll', 'trayapp/build/bin/sing-box.exe')
    if ((Compare-Object $expectedPaths @($actual.artifacts.path)) -or -not $actual.sourceDirty -or $actual.buildCommit -ne $release.buildCommit) {
        throw 'Unexpected artifact selection or provenance'
    }
    Remove-Item -LiteralPath $compilerProbe
    & $hashScript -Root $fixture -Verify
    [IO.File]::AppendAllText("$fixture/trayapp/build/oneproxy.dll", 'tamper')
    $rejected = $false
    try { & $hashScript -Root $fixture -Verify } catch {
        if ($_ -notmatch 'Release hash/size mismatch') { throw }
        $rejected = $true
    }
    if (-not $rejected) { throw 'Tampered artifact was accepted' }
    $arguments.Publisher = 'Incorrect test publisher'
    $rejected = $false
    try { & $hashScript @arguments } catch {
        if ($_ -notmatch 'Release version resource mismatch') { throw }
        $rejected = $true
    }
    if (-not $rejected) { throw 'Incorrect publisher was accepted' }
    Write-Host 'PASS: round-trip, provenance, config/stale-installer/build-helper exclusion, compiler-probe removal, deployment subdirectories, tamper detection, publisher validation'
} finally {
    $tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\', '/')
    $resolvedFixture = [IO.Path]::GetFullPath($fixture)
    if ([IO.Path]::GetDirectoryName($resolvedFixture) -ne $tempRoot -or
        [IO.Path]::GetFileName($resolvedFixture) -notmatch '^oneproxy-hash-test-[0-9a-f]{32}$') {
        throw "Refusing cleanup outside test directory: $resolvedFixture"
    }
    Remove-Item -LiteralPath $resolvedFixture -Recurse -Force
}
