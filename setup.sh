#!/bin/bash
set -e

echo "=== photo-copy setup ==="

# Build photo-copy
echo "Building photo-copy..."
if ! command -v go &> /dev/null; then
    echo "Error: Go is not installed. Install from https://go.dev/dl/"
    exit 1
fi

go build -o photo-copy ./cmd/photo-copy
echo "Built ./photo-copy"

# Verify tool binaries are present
if [ ! -d "tools-bin/rclone" ] || [ -z "$(ls tools-bin/rclone/rclone-* 2>/dev/null)" ]; then
    echo ""
    echo "Warning: rclone binaries not found in tools-bin/rclone/"
    echo "(S3 commands will not work without rclone binaries)"
fi

if [ ! -d "tools-bin/icloudpd" ] || [ -z "$(ls tools-bin/icloudpd/icloudpd-* 2>/dev/null)" ]; then
    echo ""
    echo "Warning: icloudpd binaries not found in tools-bin/icloudpd/"
    echo "(iCloud download will fall back to system-installed icloudpd)"
fi

if [ ! -d "tools-bin/osxphotos" ] || [ -z "$(ls tools-bin/osxphotos/osxphotos-* 2>/dev/null)" ]; then
    echo ""
    echo "Warning: osxphotos binary not found in tools-bin/osxphotos/"
    echo "(iCloud upload will fall back to system-installed osxphotos)"
fi

echo ""
echo "To download all tool binaries: ./tools-bin/update.sh"

echo ""
echo "Setup complete! Next step is to configure what you need:"
echo "  - Run './photo-copy config flickr' to set up Flickr credentials"
echo "  - Run './photo-copy config google' to set up Google credentials (only needed for upload)"
echo "  - Run './photo-copy config s3' to set up S3 credentials"
