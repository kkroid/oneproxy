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

    # Execute a real Inno installer with the production shutdown and payload code.
    # Only integration side effects are redirected/disabled: uninstall registration,
    # shell shortcuts, previous-directory lookup, and the user-config destination.
    # This is deliberately identified as an isolation variant, not the release EXE.
    $script = Get-Content (Join-Path $repo 'trayapp\build\installer.iss') -Raw
    $script = $script -replace '(?m)^PrivilegesRequired=admin', 'PrivilegesRequired=lowest'
    $script = $script -replace '(?m)^AppId=.*', ('AppId=OneProxy-Upgrade-Test-' + [guid]::NewGuid())
    $script = $script.Replace('[Setup]', "[Setup]`nUninstallable=no`nCreateUninstallRegKey=no`nUsePreviousAppDir=no`nUsePreviousGroup=no`nUsePreviousTasks=no")
    $script = $script -replace '(?m)^OutputDir=.*', ('OutputDir=' + $out)
    $script = $script -replace '(?m)^OutputBaseFilename=.*', 'OutputBaseFilename=isolated-setup'
    $script = $script -replace '(?s)\[Icons\].*?(?=\[Tasks\])', ''
    $script = $script.Replace('{%USERPROFILE}\.oneproxy', '{app}\profile\.oneproxy')
    # Absolute SourceDir keeps the staged release payload identical.
    $script = $script.Replace('[Setup]', "[Setup]`nSourceDir=$repo\trayapp\build")
    $isolatedScript = Join-Path $out 'isolated-installer.iss'
    Set-Content -LiteralPath $isolatedScript -Value $script -Encoding UTF8
    & $ISCC $isolatedScript
    if ($LASTEXITCODE) { throw 'Isolated installer compilation failed' }
    foreach ($setupName in @('legacy','baseline')) {
        $installTarget = Join-Path $out "setup-$setupName"
        Copy-Payload $sources[$setupName] $installTarget 28280
        $configPaths = @('config.json','profile\.oneproxy\config.json','profile\.oneproxy\subscription-sentinel.txt')
        $hashes = @($configPaths | ForEach-Object { (Get-FileHash (Join-Path $installTarget $_)).Hash })
        $payloadPaths = @('oneproxy-tray.exe','oneproxy.exe','oneproxy.dll','bin\sing-box.exe')
        $oldPayload = @($payloadPaths | ForEach-Object { (Get-FileHash (Join-Path $installTarget $_)).Hash })
        & $harness --setup-locked $installTarget (Join-Path $out 'isolated-setup.exe')
        if ($LASTEXITCODE) { throw 'Locked full setup did not fail safely' }
        for ($i = 0; $i -lt $payloadPaths.Count; $i++) {
            if ((Get-FileHash (Join-Path $installTarget $payloadPaths[$i])).Hash -ne $oldPayload[$i]) { throw 'Blocked setup replaced payload' }
        }
        Write-Host 'PASS blocked full setup leaves old payload unchanged; retry after removing lock'
        & $harness --setup $installTarget (Join-Path $out 'isolated-setup.exe')
        if ($LASTEXITCODE) { throw 'Full isolated setup upgrade failed' }
        for ($i = 0; $i -lt $configPaths.Count; $i++) {
            if ((Get-FileHash (Join-Path $installTarget $configPaths[$i])).Hash -ne $hashes[$i]) { throw 'User state changed during setup' }
        }
        foreach ($name in @('oneproxy-tray.exe','oneproxy.exe','oneproxy.dll','bin\sing-box.exe')) {
            if ((Get-FileHash (Join-Path $installTarget $name)).Hash -ne
                (Get-FileHash (Join-Path $repo "trayapp\build\$name")).Hash) { throw "Installed payload mismatch: $name" }
        }
        Write-Host 'PASS full isolated setup replaced payload and preserved configuration/subscription state'
    }
    foreach ($process in $before) {
        $after = Get-CimInstance Win32_Process -Filter "ProcessId=$($process.ProcessId)"
        if (!$after -or $after.CreationDate -ne $process.CreationDate -or $after.ExecutablePath -ne $process.ExecutablePath) {
            throw "Unrelated process changed: $($process.ProcessId)"
        }
    }
    Write-Host 'PASS unrelated existing processes preserved (PID, creation time, executable path)'
} finally { Stop-Transcript }
