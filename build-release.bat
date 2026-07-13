@echo off
REM OneProxy Release Package Builder for Windows
REM Creates release package with all necessary files

echo ========================================
echo OneProxy Release Package Builder
echo ========================================
echo.

REM Check if oneproxy.exe exists
if not exist "oneproxy.exe" (
    echo [ERROR] oneproxy.exe not found!
    echo Please run: go build -o oneproxy.exe ./cmd/oneproxy
    pause
    exit /b 1
)

REM Create release directory
set VERSION=v0.2.0
set RELEASE_DIR=release\oneproxy-%VERSION%-windows-amd64

echo [1/6] Creating release directory...
if exist "release" rmdir /s /q "release"
mkdir "%RELEASE_DIR%"
mkdir "%RELEASE_DIR%\bin"
mkdir "%RELEASE_DIR%\logs"
mkdir "%RELEASE_DIR%\configs"

echo [2/6] Copying executable...
copy oneproxy.exe "%RELEASE_DIR%\" > nul

echo [3/6] Copying documentation...
copy README.md "%RELEASE_DIR%\" > nul
copy QUICKSTART.md "%RELEASE_DIR%\" > nul
copy LICENSE "%RELEASE_DIR%\" > nul
copy RELEASE_NOTES.md "%RELEASE_DIR%\" > nul

echo [4/6] Copying configuration...
copy configs\config.example.json "%RELEASE_DIR%\configs\" > nul

echo [5/6] Copying tools...
copy download-singbox.bat "%RELEASE_DIR%\" > nul
copy Makefile "%RELEASE_DIR%\" > nul

echo [6/6] Creating README for release...
(
echo # OneProxy %VERSION%
echo.
echo ## Quick Start
echo.
echo 1. Download sing-box:
echo    ```
echo    download-singbox.bat
echo    ```
echo.
echo 2. Configure:
echo    ```
echo    copy configs\config.example.json config.json
echo    # Edit config.json with your proxy information
echo    ```
echo.
echo 3. Run:
echo    ```
echo    oneproxy.exe
echo    ```
echo.
echo 4. Start proxy from system tray
echo.
echo See QUICKSTART.md for detailed instructions.
echo See RELEASE_NOTES.md for full feature list.
) > "%RELEASE_DIR%\README.txt"

echo.
echo ========================================
echo Release package created successfully!
echo ========================================
echo.
echo Location: %RELEASE_DIR%
echo.
echo Next steps:
echo   1. Test the package
echo   2. Create ZIP:
echo      cd release
echo      tar -a -c -f oneproxy-%VERSION%-windows-amd64.zip oneproxy-%VERSION%-windows-amd64
echo   3. Upload to GitHub Release
echo.
pause
