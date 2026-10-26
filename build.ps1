# OneProxy Build — MSVC 2022 + Qt6 + Go DLL
param(
    [switch]$Clean,
    [switch]$Installer,
    [string]$QtDir,
    [string]$VcVars,
    [string]$InnoSetup,
    [switch]$Sign,
    [switch]$SigningDryRun,
    [string]$SigningThumbprint,
    [string]$SigningPfxPath,
    [string]$SigningTool,
    [string]$SigningTimestampUrl = 'http://timestamp.digicert.com'
)

$ErrorActionPreference = "Stop"
$root = $PSScriptRoot
$buildDir = "$root\trayapp\build"
. "$root\scripts\sign-windows-command.ps1"
if ($Sign -and $SigningDryRun) { throw 'Sign and SigningDryRun are mutually exclusive.' }
if (($Sign -or $SigningDryRun) -and -not $Installer) { throw 'Signing options require -Installer.' }
$innoArgs = @()
if ($Sign) {
    if (-not $SigningTool) { $SigningTool = (Get-Command signtool.exe -ErrorAction Stop).Source }
    $innoArgs = @(Get-OneProxyInnoSigningArguments -SignTool $SigningTool -CertificateThumbprint $SigningThumbprint -PfxPath $SigningPfxPath -TimestampUrl $SigningTimestampUrl)
}

function Resolve-QtDir {
    if ($QtDir) { return $QtDir }
    if ($env:ONEPROXY_QT_DIR) { return $env:ONEPROXY_QT_DIR }

    $qmake = Get-Command qmake.exe -ErrorAction SilentlyContinue
    if ($qmake) {
        $detected = & $qmake.Source -query QT_INSTALL_PREFIX
        $spec = & $qmake.Source -query QMAKE_XSPEC
        if ($LASTEXITCODE -eq 0 -and $detected -and $spec -match "msvc") {
            return $detected.Trim()
        }
    }

    $fallback = "C:\Qt\6.8.3\msvc2022_64"
    if (Test-Path $fallback) { return $fallback }
    throw "Qt 6 not found. Pass -QtDir, set ONEPROXY_QT_DIR, or add qmake.exe to PATH."
}

function Resolve-VcVars {
    if ($VcVars) { return $VcVars }
    if ($env:ONEPROXY_VCVARS) { return $env:ONEPROXY_VCVARS }

    $vswhere = "${env:ProgramFiles(x86)}\Microsoft Visual Studio\Installer\vswhere.exe"
    if (Test-Path $vswhere) {
        $installPath = & $vswhere -latest -version "[17.0,18.0)" -products * -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
        if ($installPath) {
            $installPath = $installPath.Trim()
            $detected = Join-Path $installPath "VC\Auxiliary\Build\vcvars64.bat"
            if (Test-Path $detected) { return $detected }
        }
    }

    throw "MSVC build tools not found. Pass -VcVars or set ONEPROXY_VCVARS."
}

function Resolve-InnoSetup {
    if ($InnoSetup) { return $InnoSetup }
    if ($env:ONEPROXY_INNO_SETUP) { return $env:ONEPROXY_INNO_SETUP }

    $command = Get-Command ISCC.exe -ErrorAction SilentlyContinue
    if ($command) { return $command.Source }

    $fallback = "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe"
    if (Test-Path $fallback) { return $fallback }
    throw "Inno Setup not found. Pass -InnoSetup or set ONEPROXY_INNO_SETUP."
}

function Clear-BuildDir {
    if (Test-Path $buildDir) {
        Get-ChildItem $buildDir -ErrorAction SilentlyContinue | Remove-Item -Recurse -Force -ErrorAction SilentlyContinue
    } else {
        New-Item -ItemType Directory -Force -Path $buildDir | Out-Null
    }
}

if ($Clean) {
    Clear-BuildDir
    Remove-Item -Force "$root\oneproxy.dll" -ErrorAction SilentlyContinue
    Remove-Item -Force "$root\oneproxy.exe" -ErrorAction SilentlyContinue
    Write-Host "clean done"
    exit 0
}

$qtDir = Resolve-QtDir
$vcvars = Resolve-VcVars
$vcvars32 = Join-Path (Split-Path $vcvars) 'vcvars32.bat'
if ($Installer -and -not (Test-Path $vcvars32)) { throw "x86 compiler environment not found: $vcvars32" }
$innoSetup = if ($Installer) { Resolve-InnoSetup } else { $null }

$singBox = "$root\bin\sing-box.exe"
if (-not (Test-Path $singBox)) {
    throw "sing-box not found: $singBox"
}

# Keep the existing installer definitions as the single release metadata source.
$installerSource = Get-Content -LiteralPath "$root\trayapp\installer.iss" -Raw
$metadata = @{}
foreach ($name in @('AppName', 'AppVersion', 'AppPublisher')) {
    if ($installerSource -notmatch "(?m)^#define $name `"([^`"]+)`"") {
        throw "Installer metadata missing: $name"
    }
    $metadata[$name] = $Matches[1]
}
if ($metadata.AppVersion -notmatch '^\d+\.\d+\.\d+$' -or
    @($metadata.AppVersion.Split('.') | Where-Object { [int]$_ -gt 65535 }).Count) {
    throw 'AppVersion must contain three Windows version components (0..65535).'
}
$buildCommit = (& git -C $root rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0) { throw 'Cannot identify the build commit.' }
$sourceDirty = [bool](& git -C $root status --porcelain --untracked-files=normal)
if ($LASTEXITCODE -ne 0) { throw 'Cannot determine source tree state.' }

Clear-BuildDir
$resourceTemplate = Get-Content -LiteralPath "$root\trayapp\version.rc.in" -Raw
function Write-VersionResource([string]$Path, [string]$FileName, [string]$Description, [string]$Type) {
    $values = @{
        APP_NAME = $metadata.AppName; APP_PUBLISHER = $metadata.AppPublisher
        APP_VERSION = $metadata.AppVersion; RESOURCE_VERSION = ($metadata.AppVersion.Replace('.', ',') + ',0')
        FILE_NAME = $FileName; FILE_DESCRIPTION = $Description; RESOURCE_TYPE = $Type
    }
    $resource = $resourceTemplate
    foreach ($key in $values.Keys) {
        $resource = $resource.Replace("@$key@", $values[$key].Replace('\', '\\').Replace('"', '\"'))
    }
    Set-Content -LiteralPath $Path -Value $resource -Encoding UTF8
}
Write-VersionResource "$buildDir\tray-version.rc" 'oneproxy-tray.exe' 'OneProxy Tray' 'VFT_APP'
Write-VersionResource "$buildDir\dll-version.rc" 'oneproxy.dll' 'OneProxy Core' 'VFT_DLL'

foreach ($name in @("geoip-cn", "geosite-cn")) {
    $source = "$root\bin\$name.json"
    $output = "$root\bin\$name.srs"
    if (-not (Test-Path $source)) { throw "Rule-set source not found: $source" }

    Write-Host "Compiling $name.srs..."
    & $singBox rule-set compile $source -o $output
    if ($LASTEXITCODE -ne 0) { throw "Rule-set compile failed: $name" }
}

Push-Location $root

# 1. Go CLI and DLL (the CLI is bundled beside the tray for Unix supervisor use)
Write-Host "[1/2] Building oneproxy.exe and oneproxy.dll..."
go build -buildvcs=false -o oneproxy.exe ./cmd/oneproxy
if ($LASTEXITCODE -ne 0) { Pop-Location; throw "CLI build failed" }
$resourceObject = "$root\cmd\oneproxy-dll\release_windows_amd64.syso"
if (Test-Path -LiteralPath $resourceObject) { throw "Resource output already exists: $resourceObject" }
try {
    # windres is supplied by the same MinGW toolchain used for Go c-shared builds.
    & windres --input "$buildDir\dll-version.rc" --input-format rc --output $resourceObject --output-format coff --target pe-x86-64
    if ($LASTEXITCODE -ne 0) { throw 'DLL version resource compilation failed' }
    go build -buildvcs=false -ldflags "-X main.appVersion=$($metadata.AppVersion)" -buildmode=c-shared -o oneproxy.dll ./cmd/oneproxy-dll
    if ($LASTEXITCODE -ne 0) { throw 'DLL build failed' }
} finally {
    Remove-Item -LiteralPath $resourceObject -Force -ErrorAction SilentlyContinue
}
if ($LASTEXITCODE -ne 0) { Pop-Location; throw "DLL build failed" }
Write-Host "  oneproxy.dll ($([math]::Round((Get-Item oneproxy.dll).Length / 1MB, 1)) MB)"

# 2. MSVC 2022 + Qt6 tray
Write-Host "[2/2] Building oneproxy-tray.exe (MSVC 2022 + Qt6)..."

$buildBat = "$buildDir\oneproxy_build.bat"
$buildLog = "$buildDir\oneproxy_build.log"
Set-Content -Path $buildBat -Encoding ASCII @"
@echo off
call "$vcvars" >nul 2>&1
if %ERRORLEVEL% neq 0 exit /b 1
cd /d "$buildDir"
cmake "$root\trayapp" -G "NMake Makefiles" -DCMAKE_BUILD_TYPE=Release -DBUILD_TESTING=OFF -DCMAKE_PREFIX_PATH="$qtDir" -DCMAKE_PROJECT_OneProxyTray_INCLUDE="$root\trayapp\version.cmake" -DONEPROXY_VERSION_RESOURCE="$buildDir\tray-version.rc" -DONEPROXY_CLI_PATH="$root\oneproxy.exe" >> "$buildLog" 2>&1
if %ERRORLEVEL% neq 0 exit /b 2
nmake >> "$buildLog" 2>&1
if %ERRORLEVEL% neq 0 exit /b 3
"$qtDir\bin\windeployqt.exe" oneproxy-tray.exe --no-compiler-runtime --no-translations --no-opengl-sw >> "$buildLog" 2>&1
if %ERRORLEVEL% neq 0 exit /b 4
rem Deploy the x64 release CRT beside the app; users need no separate installer.
if not exist "%VCToolsRedistDir%x64\Microsoft.VC143.CRT\vcruntime140_1.dll" exit /b 5
copy /y "%VCToolsRedistDir%x64\Microsoft.VC143.CRT\*.dll" . >> "$buildLog" 2>&1
if %ERRORLEVEL% neq 0 exit /b 6
exit /b 0
"@

Write-Host "  Compiling..."
$psi = New-Object System.Diagnostics.ProcessStartInfo
$psi.FileName = "cmd.exe"
$psi.Arguments = "/c `"$buildBat`""
$psi.UseShellExecute = $false
$psi.CreateNoWindow = $true
$p = [System.Diagnostics.Process]::Start($psi)
$p.WaitForExit()
$exitCode = $p.ExitCode
if ($exitCode -ne 0) {
    Write-Host "  BUILD FAILED" -ForegroundColor Red
    Get-Content $buildLog -ErrorAction SilentlyContinue | Select-String "error|Error|fatal" | ForEach-Object { Write-Host "  $_" }
    Remove-Item $buildBat, $buildLog -ErrorAction SilentlyContinue
    Pop-Location; throw "MSVC build failed"
}
Remove-Item $buildBat, $buildLog -ErrorAction SilentlyContinue
Write-Host "  OK"

# 3. Assets
Copy-Item -Force "$root\oneproxy.dll" "$buildDir\"
Get-ChildItem "$root\trayapp\*.ico" -ErrorAction SilentlyContinue | ForEach-Object { Copy-Item -Force $_.FullName "$buildDir\" }
New-Item -ItemType Directory -Force -Path "$buildDir\bin" | Out-Null
foreach ($name in @("sing-box.exe", "geoip-cn.srs", "geosite-cn.srs")) {
    Copy-Item -Force "$root\bin\$name" "$buildDir\bin\"
}
if (-not (Test-Path "$buildDir\config.json")) {
    if (Test-Path "$root\config.json") { Copy-Item -Force "$root\config.json" "$buildDir\" }
}
# Always copy placeholder so DLL can auto-recover if user deletes config.json
if (Test-Path "$root\config-placeholder.json") { Copy-Item -Force "$root\config-placeholder.json" "$buildDir\" }

# 4. Verify
$missing = @()
foreach ($f in @("oneproxy-tray.exe","oneproxy.exe","oneproxy.dll","Qt6Core.dll","Qt6Gui.dll","Qt6Widgets.dll","dxcompiler.dll","dxil.dll","vcruntime140.dll","vcruntime140_1.dll","msvcp140.dll","msvcp140_1.dll","msvcp140_2.dll","msvcp140_atomic_wait.dll","msvcp140_codecvt_ids.dll","concrt140.dll","platforms\qwindows.dll","bin\sing-box.exe","bin\geoip-cn.srs","bin\geosite-cn.srs")) {
    if (-not (Test-Path "$buildDir\$f")) { $missing += $f }
}
if ($missing.Count -gt 0) {
    Write-Host "WARNING: missing: $missing" -ForegroundColor Yellow
    Pop-Location; throw "Deployment incomplete"
}

if ($Installer) {
    # Inno Setup's Pascal host is 32-bit even for an x64 application payload.
    # Use the static CRT so the temporary helper needs no installed runtime.
    $upgradeBat = Join-Path $buildDir 'build-upgrade-helper.bat'
    Set-Content -LiteralPath $upgradeBat -Encoding ASCII @"
@echo off
call "$vcvars32" >nul 2>&1
if %ERRORLEVEL% neq 0 exit /b 1
cd /d "$buildDir"
cl /nologo /std:c++17 /EHsc /MT /O2 /LD "$root\trayapp\platform\upgrade_windows.cpp" /Fe:oneproxy-upgrade.dll /link user32.lib
exit /b %ERRORLEVEL%
"@
    & cmd.exe /c $upgradeBat
    if ($LASTEXITCODE -ne 0) { throw 'Upgrade helper build failed' }
    Remove-Item -LiteralPath $upgradeBat
    $signScript = Join-Path $root 'scripts\sign-windows.ps1'
    if ($Sign -or $SigningDryRun) {
        $signArgs = @('-NoProfile','-File',$signScript,'-PrePackage')
        if ($Sign) { $signArgs += @('-Sign','-TimestampUrl',$SigningTimestampUrl,'-SignTool',$SigningTool) }
        else { $signArgs += '-VerifyOnly' }
        if ($SigningThumbprint) { $signArgs += @('-CertificateThumbprint',$SigningThumbprint) }
        if ($SigningPfxPath) { $signArgs += @('-PfxPath',$SigningPfxPath) }
        & pwsh @signArgs
        if ($LASTEXITCODE -ne 0) { Pop-Location; throw 'Pre-package signing/verification failed' }
    }
    # Update only the staged script; preserve the source installer and other tasks' changes.
    $stagedInstaller = $installerSource -replace '(?m)^VersionInfoVersion=.*', "VersionInfoVersion=$($metadata.AppVersion).0"
    $stagedInstaller = $stagedInstaller.Replace('[Setup]', @"
[Setup]
VersionInfoProductName=$($metadata.AppName)
VersionInfoCompany=$($metadata.AppPublisher)
VersionInfoProductVersion=$($metadata.AppVersion)
VersionInfoTextVersion=$($metadata.AppVersion).0
"@)
    Set-Content -LiteralPath "$buildDir\installer.iss" -Value $stagedInstaller -Encoding UTF8
    & $innoSetup $innoArgs "$buildDir\installer.iss"
    if ($LASTEXITCODE -ne 0) {
        Pop-Location
        throw "Installer build failed"
    }
    if ($Sign -or $SigningDryRun) {
        $postArgs = @('-NoProfile','-File',$signScript,'-Installer',(Join-Path $root "dist\OneProxy-$($metadata.AppVersion)-setup.exe"))
        if ($Sign) { $postArgs += @('-VerifyOnly','-StrictVerify','-SignTool',$SigningTool) }
        else { $postArgs += '-VerifyOnly' }
        & pwsh @postArgs
        if ($LASTEXITCODE -ne 0) { Pop-Location; throw 'Post-package signature verification failed' }
    }
}

& "$root\scripts\release-hashes.ps1" -Root $root -Version $metadata.AppVersion -Product $metadata.AppName -Publisher $metadata.AppPublisher -BuildCommit $buildCommit -SourceDirty $sourceDirty -Installer:$Installer

Pop-Location
Write-Host "Build complete. Run: $buildDir\oneproxy-tray.exe" -ForegroundColor Green
