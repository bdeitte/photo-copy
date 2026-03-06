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

if not exist "rclone-bin" (
    echo.
    echo Warning: rclone binaries not found in rclone-bin\
    echo Run: scripts\update-rclone.sh
    echo ^(S3 commands will not work without rclone binaries^)
) else (
    dir /b rclone-bin\rclone-* >nul 2>nul
    if %errorlevel% neq 0 (
        echo.
        echo Warning: rclone binaries not found in rclone-bin\
        echo Run: scripts\update-rclone.sh
        echo ^(S3 commands will not work without rclone binaries^)
    )
)

echo.
echo Setup complete! Next steps:
echo   1. Run 'photo-copy config flickr' to set up Flickr credentials
echo   2. Run 'photo-copy config google' to set up Google credentials
echo   3. Run 'photo-copy config s3' to set up S3 credentials
