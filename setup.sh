#!/bin/bash
set -e

echo "=== photo-copy setup ==="

# Install rclone if not present
if ! command -v rclone &> /dev/null; then
    echo "Installing rclone..."
    curl https://rclone.org/install.sh | sudo bash
else
    echo "rclone already installed: $(rclone version | head -1)"
fi

# Build photo-copy
echo "Building photo-copy..."
if ! command -v go &> /dev/null; then
    echo "Error: Go is not installed. Install from https://go.dev/dl/"
    exit 1
fi

go build -o photo-copy ./cmd/photo-copy
echo "Built ./photo-copy"

echo ""
echo "Setup complete! Next steps:"
echo "  1. Run './photo-copy config flickr' to set up Flickr credentials"
echo "  2. Run './photo-copy config google' to set up Google credentials"
echo "  3. Run 'rclone config' to set up S3 remote"
