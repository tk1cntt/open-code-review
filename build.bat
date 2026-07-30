@echo off
setlocal

echo ============================================
echo   Open Code Review - Build
echo ============================================
echo.

set GO=C:\Program Files\Go\bin\go.exe
if not exist "%GO%" set GO=go

echo Building ocr.exe...
echo.
"%GO%" build -ldflags="-s -w" -o ocr.exe ./cmd/opencodereview/
if %errorlevel% neq 0 (
    echo.
    echo BUILD FAILED!
    pause
    exit /b 1
)

echo.
for %%A in (ocr.exe) do echo   ocr.exe — %%~zA bytes
echo.
echo Build successful.
echo.
echo Usage: ocr.exe review --from main --to HEAD
echo        ocr.exe scan
echo        ocr.exe viewer
echo.

endlocal
