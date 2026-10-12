param(
    [Parameter(Mandatory)][string]$LegacyBuild,
    [Parameter(Mandatory)][string]$BaselineBuild,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [string]$VcVars = 'C:\Program Files\Microsoft Visual Studio\2022\Community\VC\Auxiliary\Build\vcvars64.bat',
    [string]$ISCC = 'C:\Program Files (x86)\Inno Setup 6\ISCC.exe'
)
$ErrorActionPreference = 'Stop'
$repo = Split-Path $PSScriptRoot -Parent
$out = [IO.Path]::GetFullPath($OutputDirectory)
if (Test-Path -LiteralPath $out) { throw 'Use a new evidence directory; previous runs are immutable.' }
New-Item -ItemType Directory -Path $out | Out-Null
Start-Transcript -Path (Join-Path $out 'verification.log')
try {
    $before = @(Get-CimInstance Win32_Process | Where-Object Name -in @('oneproxy-tray.exe','oneproxy.exe','sing-box.exe') |
        Select-Object ProcessId, CreationDate, ExecutablePath)
    $before | ConvertTo-Json | Set-Content (Join-Path $out 'unrelated-before.json') -Encoding UTF8
    $buildBat = Join-Path $out 'build-test.bat'
    Set-Content -LiteralPath $buildBat -Encoding ASCII @"
@echo off
call "$VcVars" >nul 2>&1
if %ERRORLEVEL% neq 0 exit /b 1
cd /d "$out"
cl /nologo /std:c++17 /EHsc /MT /O2 "$repo\trayapp\tests\upgrade_windows_test.cpp" "$repo\trayapp\platform\upgrade_windows.cpp" /Fe:upgrade-test.exe /link user32.lib
if %ERRORLEVEL% neq 0 exit /b 2
cl /nologo /std:c++17 /EHsc /MT /O2 /LD "$repo\trayapp\tests\upgrade_profile_shim.cpp" /Fe:upgrade-profile-shim.dll
exit /b %ERRORLEVEL%
"@
    & cmd.exe /c $buildBat
    if ($LASTEXITCODE) { throw 'Native test build failed' }
    $harness = Join-Path $out 'upgrade-test.exe'
    function Copy-Payload([string]$Source, [string]$Target, [int]$Port) {
        New-Item -ItemType Directory -Force -Path $Target | Out-Null
        Get-ChildItem -LiteralPath $Source -File | Where-Object Extension -in @('.exe','.dll','.ico') |
            Copy-Item -Destination $Target
        foreach ($folder in @('bin','platforms','styles','imageformats','iconengines','tls','generic','networkinformation')) {
            $path = Join-Path $Source $folder
            if (Test-Path $path) { Copy-Item -LiteralPath $path -Destination $Target -Recurse }
        }
        # The released core hardcodes Clash 127.0.0.1:9090, already owned by
        # the user's proxy. Keep the real tray/core but use a non-networking
        # long-lived child to test their actual Job and shutdown lifecycle.
        Copy-Item -LiteralPath $harness -Destination (Join-Path $Target 'bin\sing-box.exe') -Force
        # No external subscription or DNS/health traffic, no system proxy/TUN.
        $config = Get-Content (Join-Path $repo 'config-placeholder.json') -Raw | ConvertFrom-Json
        $config.unified.port = $Port
        $config.health_check.enabled = $false
        $config.dns.flush_on_failure = $false
        $config.proxies[0].server = '127.0.0.1'
        $config.proxies[0].port = $Port + 1
        $config.proxies[0].local_port = $Port + 2
        $config | ConvertTo-Json -Depth 20 | Set-Content (Join-Path $Target 'config.json') -Encoding UTF8
        $profileData = Join-Path $Target 'profile\.oneproxy'
        New-Item -ItemType Directory -Force $profileData | Out-Null
        Copy-Item (Join-Path $Target 'config.json') (Join-Path $profileData 'config.json')
        # The tray reads the CWD fixture. Keep a distinct saved subscription in
        # the user-profile fixture to verify Setup preserves it without fetching.
        $saved = Get-Content (Join-Path $profileData 'config.json') -Raw | ConvertFrom-Json
        $saved | Add-Member -NotePropertyName subscription_url -NotePropertyValue 'https://subscription.invalid/preserve-me'
        $saved | ConvertTo-Json -Depth 20 | Set-Content (Join-Path $profileData 'config.json') -Encoding UTF8
        Set-Content (Join-Path $profileData 'subscription-sentinel.txt') 'keep-subscription-state' -Encoding UTF8
    }
    $sources = [ordered]@{ legacy = $LegacyBuild; baseline = $BaselineBuild; current = (Join-Path $repo 'trayapp\build') }
    $port = 28180
    foreach ($name in $sources.Keys) {
        $target = Join-Path $out $name
        Copy-Payload $sources[$name] $target $port
        $port += 10
        $sourceHashes = @(Get-FileHash (Join-Path $sources[$name] 'oneproxy-tray.exe'), (Join-Path $sources[$name] 'oneproxy.dll'))
        $sourceHashes | ConvertTo-Json | Set-Content (Join-Path $out "$name-source-hashes.json") -Encoding UTF8
        $configBefore = (Get-FileHash (Join-Path $target 'config.json')).Hash
        $mode = if ($name -eq 'current') { '--new' } else { '--old' }
        & $harness $mode $target
        if ($LASTEXITCODE) { throw "$name integration failed" }
        if ((Get-FileHash (Join-Path $target 'config.json')).Hash -ne $configBefore) { throw 'Config changed during stop' }
        foreach ($sourceHash in $sourceHashes) {
            if ((Get-FileHash $sourceHash.Path).Hash -ne $sourceHash.Hash) { throw 'Source artifact changed' }
        }
    }
    $fixtureRoot = Join-Path $out 'fixtures'
    foreach ($name in @('target','other')) {
        $directory = Join-Path $fixtureRoot $name
        New-Item -ItemType Directory -Force (Join-Path $directory 'bin') | Out-Null
        Copy-Item $harness (Join-Path $directory 'oneproxy-tray.exe')
        Copy-Item $harness (Join-Path $directory 'bin\sing-box.exe')
    }
    & $harness --fixture-test $fixtureRoot
    if ($LASTEXITCODE) { throw 'Fallback/identity/locking tests failed' }

    # Real uninstall generation/registration is mandatory for setup coverage.
    foreach ($setupName in @('legacy','baseline')) {
        & (Join-Path $PSScriptRoot 'test-windows-uninstall-upgrade.ps1') `
            -LegacyBuild $sources[$setupName] `
            -OutputDirectory (Join-Path $out "setup-$setupName") -VcVars $VcVars -ISCC $ISCC
    }
    foreach ($process in $before) {
        $after = Get-CimInstance Win32_Process -Filter "ProcessId=$($process.ProcessId)"
        if (!$after -or $after.CreationDate -ne $process.CreationDate -or $after.ExecutablePath -ne $process.ExecutablePath) {
            throw "Unrelated process changed: $($process.ProcessId)"
        }
    }
    Write-Host 'PASS unrelated existing processes preserved (PID, creation time, executable path)'
} finally { Stop-Transcript }
