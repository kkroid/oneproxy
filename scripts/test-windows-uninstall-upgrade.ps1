param(
    [Parameter(Mandatory)][string]$LegacyBuild,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [string]$VcVars = 'C:\Program Files\Microsoft Visual Studio\2022\Community\VC\Auxiliary\Build\vcvars64.bat',
    [string]$ISCC = 'C:\Program Files (x86)\Inno Setup 6\ISCC.exe'
)
$ErrorActionPreference = 'Stop'
$repo = Split-Path $PSScriptRoot -Parent
$out = [IO.Path]::GetFullPath($OutputDirectory)
if (Test-Path -LiteralPath $out) { throw 'Use a new evidence directory.' }
New-Item -ItemType Directory -Path $out | Out-Null
Start-Transcript (Join-Path $out 'verification.log')
$appId = 'OP-Uninstall-' + [guid]::NewGuid().ToString('N')
$regPath = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\${appId}_is1"
$target = Join-Path $out 'target'
function Run-Setup([string]$Exe, [string]$Log, [int]$Expected = 0) {
    $process = Start-Process -FilePath $Exe -ArgumentList "/VERYSILENT /SUPPRESSMSGBOXES /NORESTART /SP- /DIR=`"$target`" /LOG=`"$out\$Log.log`"" -PassThru -WindowStyle Hidden
    if (!$process.WaitForExit(120000)) { throw "Setup timeout; inspect this isolated process: $($process.Id)" }
    if ($process.ExitCode -ne $Expected) { throw "$Log exit $($process.ExitCode), expected $Expected" }
    Write-Host "PASS $Log exit=$Expected"
}
function Snapshot {
    $snapshot = [ordered]@{}
    foreach ($file in @('unins000.exe','unins000.dat','oneproxy-tray.exe','oneproxy.dll','config.json','profile\.oneproxy\config.json','profile\.oneproxy\subscription-sentinel.txt')) {
        $snapshot[$file] = (Get-FileHash (Join-Path $target $file)).Hash
    }
    $snapshot['registry'] = (Get-ItemProperty $regPath | Select-Object DisplayName,DisplayVersion,InstallLocation,UninstallString,QuietUninstallString | ConvertTo-Json -Compress)
    return ($snapshot | ConvertTo-Json -Compress)
}
try {
    $before = @(Get-CimInstance Win32_Process | Where-Object Name -in @('oneproxy-tray.exe','oneproxy.exe','sing-box.exe') |
        Select-Object ProcessId, CreationDate, ExecutablePath)
    $before | ConvertTo-Json | Set-Content "$out\unrelated-before.json" -Encoding UTF8
    Set-Content "$out\identity.json" (@{ appId=$appId; registry=$regPath; target=$target } | ConvertTo-Json) -Encoding UTF8
    $vc32 = Join-Path (Split-Path $VcVars) 'vcvars32.bat'
    # Freeze the faulty source for an A/B reproduction with a real uninstall log.
    & git -C $repo show '965cc5d:trayapp/platform/upgrade_windows.cpp' | Set-Content "$out\baseline.cpp" -Encoding UTF8
    if ($LASTEXITCODE) { throw 'Cannot obtain the regression baseline' }
    $baselineScript = (& git -C $repo show '965cc5d:trayapp/installer.iss') -join "`n"
    if ($LASTEXITCODE) { throw 'Cannot obtain the baseline installer' }
    @"
@echo off
call "$VcVars" >nul 2>&1
if %ERRORLEVEL% neq 0 exit /b 1
cd /d "$out"
cl /nologo /std:c++17 /EHsc /MT /O2 "$repo\trayapp\tests\upgrade_windows_test.cpp" "$repo\trayapp\platform\upgrade_windows.cpp" /Fe:upgrade-test.exe /link user32.lib
if %ERRORLEVEL% neq 0 exit /b 2
cl /nologo /std:c++17 /EHsc /MT /O2 /LD "$repo\trayapp\tests\upgrade_profile_shim.cpp" /Fe:upgrade-profile-shim.dll
if %ERRORLEVEL% neq 0 exit /b 3
call "$vc32" >nul 2>&1
if %ERRORLEVEL% neq 0 exit /b 4
cl /nologo /std:c++17 /EHsc /MT /O2 /LD baseline.cpp /Fe:baseline-helper.dll /link user32.lib
exit /b %ERRORLEVEL%
"@ | Set-Content "$out\build.bat" -Encoding ASCII
    & cmd.exe /d /c "$out\build.bat"
    if ($LASTEXITCODE) { throw 'Test harness compilation failed' }
    $harness = Join-Path $out 'upgrade-test.exe'
    & $harness --handle-test "$out\handle-fixture"
    if ($LASTEXITCODE) { throw 'Handle ownership reproduction/guard lifetime regression failed' }
    $old = Join-Path $out 'old-payload'
    Copy-Item -LiteralPath $LegacyBuild -Destination $old -Recurse
    Get-FileHash "$LegacyBuild\oneproxy-tray.exe", "$LegacyBuild\oneproxy.dll" |
        ConvertTo-Json | Set-Content "$out\legacy-source-hashes.json" -Encoding UTF8
    Copy-Item "$out\baseline-helper.dll" "$old\oneproxy-upgrade.dll" -Force
    # No real proxy or networking is started. Real legacy Qt/Go owns this child.
    Copy-Item $harness "$old\bin\sing-box.exe" -Force
    $config = Get-Content "$repo\config-placeholder.json" -Raw | ConvertFrom-Json
    $config.unified.port = 28480
    $config.health_check.enabled = $false
    $config.dns.flush_on_failure = $false
    $config.proxies[0].server = '127.0.0.1'
    $config.proxies[0].port = 28481
    $config.proxies[0].local_port = 28482
    $config | ConvertTo-Json -Depth 20 | Set-Content "$old\config-placeholder.json" -Encoding UTF8
    function Compile-Setup([string]$Script, [string]$Payload, [string]$Name, [string]$Version) {
        $Script = $Script -replace '(?m)^PrivilegesRequired=admin', 'PrivilegesRequired=lowest'
        $Script = $Script -replace '(?m)^AppId=.*', "AppId=$appId"
        $Script = $Script -replace '(?m)^AppVersion=.*', "AppVersion=$Version"
        $Script = $Script -replace '(?m)^OutputDir=.*', "OutputDir=$out"
        $Script = $Script -replace '(?m)^OutputBaseFilename=.*', "OutputBaseFilename=$Name"
        $Script = $Script -replace '(?s)\[Icons\].*?(?=\[Tasks\])', ''
        $Script = $Script -replace '(?s)\[Run\].*?(?=\[Code\])', ''
        $Script = $Script.Replace('{%USERPROFILE}\.oneproxy', '{app}\profile\.oneproxy')
        $Script = $Script.Replace('[Setup]', "[Setup]`nSourceDir=$Payload`nUsePreviousAppDir=no`nUsePreviousGroup=no`nUsePreviousTasks=no")
        # Uninstallable/CreateUninstallRegKey stay enabled: this is the regression.
        Set-Content "$out\$Name.iss" $Script -Encoding UTF8
        & $ISCC "$out\$Name.iss" > "$out\$Name-compile.log"
        if ($LASTEXITCODE) { throw "$Name compilation failed" }
    }
    Compile-Setup $baselineScript $old 'old-setup' '0.8.0'
    Compile-Setup (Get-Content "$repo\trayapp\installer.iss" -Raw) "$repo\trayapp\build" 'fixed-setup' '0.8.2'
    Run-Setup "$out\old-setup.exe" 'initial-install'
    if (!(Test-Path $regPath) -or !(Test-Path "$target\unins000.dat")) { throw 'Real uninstall lifecycle missing' }
    # Freeze a profile subscription without ever fetching it.
    $saved = Get-Content "$target\profile\.oneproxy\config.json" -Raw | ConvertFrom-Json
    $saved | Add-Member subscription_url 'https://subscription.invalid/preserve-me'
    $saved | ConvertTo-Json -Depth 20 | Set-Content "$target\profile\.oneproxy\config.json" -Encoding UTF8
    Set-Content "$target\profile\.oneproxy\subscription-sentinel.txt" 'preserve subscription state' -Encoding UTF8
    Copy-Item "$old\config-placeholder.json" "$target\config.json"
    Run-Setup "$out\old-setup.exe" 'baseline-reproduction' 5
    $log = Get-Content "$out\baseline-reproduction.log" -Raw
    if ($log -notmatch '(?s)Saving uninstall information.*MoveFileEx failed; code 5') { throw 'Original failure was not reproduced' }
    Write-Host 'PASS original Saving uninstall information / MoveFileEx code 5 reproduced'
    $snapshot = Snapshot
    $snapshot | Set-Content "$out\before-lock-tests.json" -Encoding UTF8
    foreach ($mode in @('--uninstall-exe','--uninstall-dat','--uninstall-readonly','--uninstall-running','--uninstall-cancel')) {
        & $harness $mode $target "$out\fixed-setup.exe"
        if ($LASTEXITCODE) { throw "$mode failed" }
        if ((Snapshot) -ne $snapshot) { throw "$mode changed uninstall records, payload or profile" }
        $log = Get-Content "$target\$mode.log" -Raw
        if ($log -match 'Saving uninstall information' -or $log -notmatch 'Uninstall information is in use or not writable') {
            throw "$mode did not fail at the intended preparation gate"
        }
        Write-Host "PASS $mode preserved EXE/DAT/registration/payload/configuration"
    }
    $uninstallFile = "$target\unins000.dat"
    $originalAcl = Get-Acl $uninstallFile
    try {
        $deniedAcl = Get-Acl $uninstallFile
        $rule = [Security.AccessControl.FileSystemAccessRule]::new(
            [Security.Principal.WindowsIdentity]::GetCurrent().User,
            [Security.AccessControl.FileSystemRights]'Write,Delete',
            [Security.AccessControl.AccessControlType]::Deny)
        $deniedAcl.AddAccessRule($rule)
        Set-Acl $uninstallFile $deniedAcl
        & $harness --uninstall-permission $target "$out\fixed-setup.exe"
        if ($LASTEXITCODE) { throw 'Denied uninstall DAT ACL was not rejected' }
    } finally { Set-Acl $uninstallFile $originalAcl }
    if ((Snapshot) -ne $snapshot) { throw 'Permission rejection changed installation state' }
    Write-Host 'PASS explicit DAT write/delete ACL denial preserved installation state'
    $configHash = (Get-FileHash "$target\profile\.oneproxy\config.json").Hash
    $localConfigHash = (Get-FileHash "$target\config.json").Hash
    $sentinelHash = (Get-FileHash "$target\profile\.oneproxy\subscription-sentinel.txt").Hash
    & $harness --setup-locked $target "$out\fixed-setup.exe"
    if ($LASTEXITCODE -or (Snapshot) -ne $snapshot) { throw 'Locked application DLL setup did not preserve installation state' }
    & $harness --setup $target "$out\fixed-setup.exe"
    if ($LASTEXITCODE) { throw 'Unlocked retry with running legacy Qt failed' }
    if ((Get-ItemProperty $regPath).DisplayVersion -ne '0.8.2') { throw 'Uninstall registration not upgraded' }
    foreach ($name in @('oneproxy-tray.exe','oneproxy.exe','oneproxy.dll','bin\sing-box.exe')) {
        if ((Get-FileHash "$target\$name").Hash -ne (Get-FileHash "$repo\trayapp\build\$name").Hash) { throw "Payload mismatch: $name" }
    }
    if ((Get-FileHash "$target\config.json").Hash -ne $localConfigHash -or
        (Get-FileHash "$target\profile\.oneproxy\config.json").Hash -ne $configHash -or
        (Get-FileHash "$target\profile\.oneproxy\subscription-sentinel.txt").Hash -ne $sentinelHash) { throw 'Saved profile changed' }
    Write-Host 'PASS released locks, running legacy upgrade, payload and profile preservation'
    & $harness --uninstall-retry $target "$out\fixed-setup.exe"
    if ($LASTEXITCODE) { throw 'Same-wizard unlock and Retry failed' }
    Run-Setup "$out\fixed-setup.exe" 'repeat-fixed-upgrade'
    $successfulSnapshot = Snapshot
    & $harness --uninstall-late $target "$out\fixed-setup.exe"
    if ($LASTEXITCODE -or (Snapshot) -ne $successfulSnapshot) { throw 'Late EXE lock left inconsistent uninstall records or payload' }
    Write-Host 'PASS late external EXE lock cancels at save and preserves prior EXE/DAT/registration'
    Run-Setup "$out\fixed-setup.exe" 'late-lock-retry'
    Copy-Item "$target\unins000.dat" "$out\successful-unins000.dat"
    Get-ItemProperty $regPath | Select-Object DisplayName,DisplayVersion,InstallLocation,UninstallString,QuietUninstallString |
        ConvertTo-Json | Set-Content "$out\successful-registration.json" -Encoding UTF8
    Run-Setup "$target\unins000.exe" 'final-uninstall'
    # Inno's first phase can exit before deferred self-deletion completes.
    $deadline = [DateTime]::UtcNow.AddSeconds(15)
    while ((Test-Path $regPath) -and [DateTime]::UtcNow -lt $deadline) { Start-Sleep -Milliseconds 100 }
    if ((Test-Path $regPath) -or (Test-Path "$target\oneproxy-tray.exe") -or (Test-Path "$target\unins000.dat")) {
        throw 'Upgraded uninstall failed to remove registered payload/record'
    }
    if ((Get-FileHash "$target\profile\.oneproxy\subscription-sentinel.txt").Hash -ne $sentinelHash) { throw 'Uninstall removed unowned subscription state' }
    foreach ($process in $before) {
        $after = Get-CimInstance Win32_Process -Filter "ProcessId=$($process.ProcessId)"
        if (!$after -or $after.CreationDate -ne $process.CreationDate -or $after.ExecutablePath -ne $process.ExecutablePath) {
            throw "Unrelated process changed: $($process.ProcessId)"
        }
    }
    Write-Host 'PASS upgraded uninstaller removed its payload and independent registration; unrelated processes preserved'
} finally {
    # Clean only the exact test registration via its own real uninstaller.
    if ((Test-Path $regPath) -and (Test-Path "$target\unins000.exe")) {
        Set-ItemProperty "$target\unins000.exe" -Name IsReadOnly -Value $false
        Run-Setup "$target\unins000.exe" 'cleanup-uninstall'
    }
    Stop-Transcript
}
