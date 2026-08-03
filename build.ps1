# OneProxy Build — MSVC 2022 + Qt6 + Go DLL
param(
    [switch]$Clean,
    [switch]$Installer,
    [string]$QtDir,
    [string]$VcVars,
    [string]$InnoSetup
)

$ErrorActionPreference = "Stop"
$root = $PSScriptRoot
$buildDir = "$root\trayapp\build"

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
    Write-Host "clean done"
    exit 0
}

$qtDir = Resolve-QtDir
$vcvars = Resolve-VcVars
$innoSetup = if ($Installer) { Resolve-InnoSetup } else { $null }

$singBox = "$root\bin\sing-box.exe"
if (-not (Test-Path $singBox)) {
    throw "sing-box not found: $singBox"
}

foreach ($name in @("geoip-cn", "geosite-cn")) {
    $source = "$root\bin\$name.json"
    $output = "$root\bin\$name.srs"
    if (-not (Test-Path $source)) { throw "Rule-set source not found: $source" }

    Write-Host "Compiling $name.srs..."
    & $singBox rule-set compile $source -o $output
    if ($LASTEXITCODE -ne 0) { throw "Rule-set compile failed: $name" }
}

Push-Location $root

# 1. Go DLL
Write-Host "[1/2] Building oneproxy.dll..."
go build -buildvcs=false -buildmode=c-shared -o oneproxy.dll ./cmd/oneproxy-dll
if ($LASTEXITCODE -ne 0) { Pop-Location; throw "DLL build failed" }
Write-Host "  oneproxy.dll ($([math]::Round((Get-Item oneproxy.dll).Length / 1MB, 1)) MB)"

# 2. MSVC 2022 + Qt6 tray
Write-Host "[2/2] Building oneproxy-tray.exe (MSVC 2022 + Qt6)..."
Clear-BuildDir

$buildBat = "$env:TEMP\oneproxy_build.bat"
$buildLog = "$env:TEMP\oneproxy_build.log"
Set-Content -Path $buildBat -Encoding ASCII @"
@echo off
call "$vcvars" >nul 2>&1
cd /d "$buildDir"
cmake "$root\trayapp" -G "NMake Makefiles" -DCMAKE_BUILD_TYPE=Release -DCMAKE_PREFIX_PATH="$qtDir" >> "$buildLog" 2>&1
if %ERRORLEVEL% neq 0 exit /b 2
nmake >> "$buildLog" 2>&1
if %ERRORLEVEL% neq 0 exit /b 3
"$qtDir\bin\windeployqt.exe" oneproxy-tray.exe --no-translations --no-system-d3d-compiler --no-opengl-sw >> "$buildLog" 2>&1
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
foreach ($f in @("oneproxy-tray.exe","oneproxy.dll","Qt6Gui.dll","Qt6Widgets.dll","platforms\qwindows.dll","bin\sing-box.exe","bin\geoip-cn.srs","bin\geosite-cn.srs")) {
    if (-not (Test-Path "$buildDir\$f")) { $missing += $f }
}
if ($missing.Count -gt 0) {
    Write-Host "WARNING: missing: $missing" -ForegroundColor Yellow
    Pop-Location; throw "Deployment incomplete"
}

if ($Installer) {
    Copy-Item -Force "$root\trayapp\installer.iss" "$buildDir\installer.iss"
    & $innoSetup "$buildDir\installer.iss"
    if ($LASTEXITCODE -ne 0) {
        Pop-Location
        throw "Installer build failed"
    }
}

Pop-Location
Write-Host "Build complete. Run: $buildDir\oneproxy-tray.exe" -ForegroundColor Green
