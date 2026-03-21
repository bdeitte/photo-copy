@echo off
echo === photo-copy setup ===

echo Building photo-copy...
where go >nul 2>nul
if %errorlevel% neq 0 (
    echo Error: Go is not installed. Install from https://go.dev/dl/
    exit /b 1
)

go build -o photo-copy.exe ./cmd/photo-copy
if %errorlevel% neq 0 (
    echo Build failed.
    exit /b 1
)
echo Built photo-copy.exe

if not exist "tools-bin\rclone" (
    echo.
    echo Warning: rclone binaries not found in tools-bin\rclone\
    echo ^(S3 commands will not work without rclone binaries^)
) else (
    dir /b tools-bin\rclone\rclone-* >nul 2>nul
    if %errorlevel% neq 0 (
        echo.
        echo Warning: rclone binaries not found in tools-bin\rclone\
        echo ^(S3 commands will not work without rclone binaries^)
    )
)

if not exist "tools-bin\icloudpd" (
    echo.
    echo Warning: icloudpd binaries not found in tools-bin\icloudpd\
    echo ^(iCloud download will fall back to system-installed icloudpd^)
) else (
    dir /b tools-bin\icloudpd\icloudpd-* >nul 2>nul
    if %errorlevel% neq 0 (
        echo.
        echo Warning: icloudpd binaries not found in tools-bin\icloudpd\
        echo ^(iCloud download will fall back to system-installed icloudpd^)
    )
)

if not exist "tools-bin\osxphotos" (
    echo.
    echo Warning: osxphotos binary not found in tools-bin\osxphotos\
    echo ^(iCloud upload will fall back to system-installed osxphotos^)
) else (
    dir /b tools-bin\osxphotos\osxphotos-* >nul 2>nul
    if %errorlevel% neq 0 (
        echo.
        echo Warning: osxphotos binary not found in tools-bin\osxphotos\
        echo ^(iCloud upload will fall back to system-installed osxphotos^)
    )
)

echo.
echo To download all tool binaries: bash tools-bin\update.sh

echo.
echo Setup complete! Next steps:
echo   1. Run 'photo-copy config flickr' to set up Flickr credentials
echo   2. Run 'photo-copy config google' to set up Google credentials
echo   3. Run 'photo-copy config s3' to set up S3 credentials
