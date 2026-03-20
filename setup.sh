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

# Verify rclone binaries are present
if [ ! -d "rclone-bin" ] || [ -z "$(ls rclone-bin/rclone-* 2>/dev/null)" ]; then
    echo ""
    echo "Warning: rclone binaries not found in rclone-bin/"
    echo "Run: ./rclone-bin/update-rclone.sh"
    echo "(S3 commands will not work without rclone binaries)"
fi

echo ""
echo "Setup complete! Next step is to configure what you need:"
echo "  - Run './photo-copy config flickr' to set up Flickr credentials"
echo "  - Run './photo-copy config google' to set up Google credentials (only needed for upload)"
echo "  - Run './photo-copy config s3' to set up S3 credentials"
