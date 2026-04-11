#!/usr/bin/env bash
set -e

ICLOUDPD_VERSION="${1:-1.32.2}"
ICLOUDPD_VERSION="${ICLOUDPD_VERSION#v}"  # Strip leading 'v' if present
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BIN_DIR="$SCRIPT_DIR"

mkdir -p "$BIN_DIR"

# Detect current version from an existing binary
CURRENT_VERSION=""
for bin in "$BIN_DIR"/icloudpd-*; do
    [[ -x "$bin" ]] || continue
    ver=$("$bin" --version 2>/dev/null | head -1 | grep -oE '[0-9]+\.[0-9]+\.[0-9]+') || continue
    CURRENT_VERSION="$ver"
    break
done
CURRENT_VERSION="${CURRENT_VERSION:-unknown}"

if [ "$CURRENT_VERSION" = "$ICLOUDPD_VERSION" ]; then
    echo "Already at icloudpd $ICLOUDPD_VERSION — nothing to do."
    exit 0
fi

echo "=== icloudpd Update: $CURRENT_VERSION -> $ICLOUDPD_VERSION ==="
echo ""
echo "Release: https://github.com/icloud-photos-downloader/icloud_photos_downloader/releases/tag/v${ICLOUDPD_VERSION}"
echo ""
bash "$SCRIPT_DIR/../show-github-release-notes.sh" "icloud-photos-downloader/icloud_photos_downloader" "v${ICLOUDPD_VERSION}"
echo ""

WORK_DIR=$(mktemp -d)
trap 'rm -rf "$WORK_DIR"' EXIT

BASE_URL="https://github.com/icloud-photos-downloader/icloud_photos_downloader/releases/download/v${ICLOUDPD_VERSION}"

download_icloudpd() {
    local upstream_name="$1"
    local binary_name="$2"

    echo "Downloading icloudpd $ICLOUDPD_VERSION ($upstream_name)..."
    curl -sfL "${BASE_URL}/icloudpd-${ICLOUDPD_VERSION}-${upstream_name}" -o "$BIN_DIR/$binary_name"
    chmod +x "$BIN_DIR/$binary_name"
    echo "  -> $binary_name"
}

# Upstream uses "macos" not "darwin", and no arm64 macOS binary exists
download_icloudpd "linux-amd64"     "icloudpd-linux-amd64"
download_icloudpd "linux-arm64"     "icloudpd-linux-arm64"
download_icloudpd "macos-amd64"     "icloudpd-darwin-amd64"

# Windows binary has .exe suffix upstream
echo "Downloading icloudpd $ICLOUDPD_VERSION (windows-amd64)..."
curl -sfL "${BASE_URL}/icloudpd-${ICLOUDPD_VERSION}-windows-amd64.exe" -o "$BIN_DIR/icloudpd-windows-amd64.exe"
echo "  -> icloudpd-windows-amd64.exe"

echo ""
echo "icloudpd $ICLOUDPD_VERSION downloaded."
echo "Files in $BIN_DIR:"
ls -lh "$BIN_DIR"/icloudpd-*
