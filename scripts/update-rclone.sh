#!/usr/bin/env bash
set -e

RCLONE_VERSION="${1:-v1.68.2}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BIN_DIR="$SCRIPT_DIR/../rclone-bin"

mkdir -p "$BIN_DIR"

TMPDIR=$(mktemp -d)
trap "rm -rf $TMPDIR" EXIT

download_rclone() {
    local platform="$1"
    local binary_name="$2"

    echo "Downloading rclone $RCLONE_VERSION for $platform..."

    local url="https://downloads.rclone.org/${RCLONE_VERSION}/rclone-${RCLONE_VERSION}-${platform}.zip"
    curl -sL "$url" -o "$TMPDIR/rclone.zip"
    unzip -q -o "$TMPDIR/rclone.zip" -d "$TMPDIR"

    if [[ "$platform" == windows-* ]]; then
        cp "$TMPDIR/rclone-${RCLONE_VERSION}-${platform}/rclone.exe" "$BIN_DIR/$binary_name"
    else
        cp "$TMPDIR/rclone-${RCLONE_VERSION}-${platform}/rclone" "$BIN_DIR/$binary_name"
        chmod +x "$BIN_DIR/$binary_name"
    fi

    rm -f "$TMPDIR/rclone.zip"
    rm -rf "$TMPDIR/rclone-${RCLONE_VERSION}-${platform}"
    echo "  -> $binary_name"
}

download_rclone "linux-amd64"   "rclone-linux-amd64"
download_rclone "linux-arm64"   "rclone-linux-arm64"
download_rclone "osx-amd64"     "rclone-darwin-amd64"
download_rclone "osx-arm64"     "rclone-darwin-arm64"
download_rclone "windows-amd64" "rclone-windows-amd64.exe"
download_rclone "windows-arm64" "rclone-windows-arm64.exe"

echo ""
echo "Rclone $RCLONE_VERSION downloaded for all platforms."
echo "Files in $BIN_DIR:"
ls -lh "$BIN_DIR"/rclone-*
