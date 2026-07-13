@echo off
REM Download sing-box for Windows
REM This script downloads the latest sing-box release from GitHub

setlocal enabledelayedexpansion

set VERSION=1.13.14
set FILENAME=sing-box-%VERSION%-windows-amd64.zip
set DOWNLOAD_URL=https://github.com/SagerNet/sing-box/releases/download/v%VERSION%/%FILENAME%
set BIN_DIR=bin
set TEMP_ZIP=%TEMP%\%FILENAME%

echo ========================================
echo   sing-box Downloader for OneProxy
echo ========================================
echo.
echo Version: %VERSION%
echo Target: %BIN_DIR%\sing-box.exe
echo.

REM Check if bin directory exists
if not exist "%BIN_DIR%" (
    echo Creating bin directory...
    mkdir "%BIN_DIR%"
)

REM Check if sing-box already exists
if exist "%BIN_DIR%\sing-box.exe" (
    echo sing-box.exe already exists in %BIN_DIR%
    set /p OVERWRITE="Do you want to overwrite it? (y/n): "
    if /i not "!OVERWRITE!"=="y" (
        echo Aborted.
        exit /b 0
    )
)

echo.
echo Downloading sing-box v%VERSION%...
echo From: %DOWNLOAD_URL%
echo.

REM Download using PowerShell
powershell -Command "& {[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12; Invoke-WebRequest -Uri '%DOWNLOAD_URL%' -OutFile '%TEMP_ZIP%'}"

if not exist "%TEMP_ZIP%" (
    echo ERROR: Download failed!
    exit /b 1
)

echo.
echo Extracting...

REM Extract using PowerShell
powershell -Command "& {Expand-Archive -Path '%TEMP_ZIP%' -DestinationPath '%TEMP%\singbox-extract' -Force}"

REM Find and copy sing-box.exe
for /r "%TEMP%\singbox-extract" %%f in (sing-box.exe) do (
    if exist "%%f" (
        echo Copying sing-box.exe to %BIN_DIR%\...
        copy /Y "%%f" "%BIN_DIR%\sing-box.exe" >nul
        goto :cleanup
    )
)

echo ERROR: sing-box.exe not found in archive!
goto :cleanup

:cleanup
REM Cleanup
echo Cleaning up...
if exist "%TEMP_ZIP%" del /Q "%TEMP_ZIP%"
if exist "%TEMP%\singbox-extract" rd /S /Q "%TEMP%\singbox-extract"

if exist "%BIN_DIR%\sing-box.exe" (
    echo.
    echo ========================================
    echo   sing-box downloaded successfully!
    echo ========================================
    echo Location: %BIN_DIR%\sing-box.exe
    echo.
    echo You can now run OneProxy:
    echo   oneproxy.exe
    echo.
) else (
    echo.
    echo ERROR: Installation failed!
    exit /b 1
)

endlocal
