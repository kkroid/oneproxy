# OneProxy Build — MSVC 2022 + Qt6 + Go DLL
param([switch]$Clean)

$ErrorActionPreference = "Stop"
$root = $PSScriptRoot
$buildDir = "$root\trayapp\build"
$qtDir = "C:\Qt\6.8.3\msvc2022_64"
$vcvars = "C:\Program Files\Microsoft Visual Studio\2022\Community\VC\Auxiliary\Build\vcvars64.bat"

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

Get-Process oneproxy-tray -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Get-Process sing-box -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Start-Sleep -Milliseconds 500

Push-Location $root

# 1. Go DLL
Write-Host "[1/2] Building oneproxy.dll..."
go build -buildmode=c-shared -o oneproxy.dll ./cmd/oneproxy-dll
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
cmake "$root\trayapp" -G "NMake Makefiles" -DCMAKE_BUILD_TYPE=Release -DCMAKE_PREFIX_PATH=$qtDir >> "$buildLog" 2>&1
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
if (Test-Path "$root\bin") {
    New-Item -ItemType Directory -Force -Path "$buildDir\bin" | Out-Null
    Copy-Item -Recurse -Force "$root\bin\*" "$buildDir\bin\"
}
if (-not (Test-Path "$buildDir\config.json")) {
    if (Test-Path "$root\config.json") { Copy-Item -Force "$root\config.json" "$buildDir\" }
}

# 4. Verify
$missing = @()
foreach ($f in @("oneproxy-tray.exe","oneproxy.dll","Qt6Gui.dll","Qt6Widgets.dll","platforms\qwindows.dll")) {
    if (-not (Test-Path "$buildDir\$f")) { $missing += $f }
}
if ($missing.Count -gt 0) {
    Write-Host "WARNING: missing: $missing" -ForegroundColor Yellow
    Pop-Location; throw "Deployment incomplete"
}

Pop-Location
Write-Host "Build complete. Run: $buildDir\oneproxy-tray.exe" -ForegroundColor Green
