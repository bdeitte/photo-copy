#!/usr/bin/env bash
set -e

# osxphotos is macOS-only — skip on other platforms
if [[ "$(uname)" != "Darwin" ]]; then
    echo "Skipping osxphotos (macOS only, current platform: $(uname))"
    exit 0
fi

OSXPHOTOS_VERSION="${1:-0.75.6}"
OSXPHOTOS_VERSION="${OSXPHOTOS_VERSION#v}"  # Strip leading 'v' if present
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BIN_DIR="$SCRIPT_DIR"

mkdir -p "$BIN_DIR"

# Detect current version from an existing binary
CURRENT_VERSION=""
if [[ -x "$BIN_DIR/osxphotos-darwin-arm64" ]]; then
    ver=$("$BIN_DIR/osxphotos-darwin-arm64" version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1) || true
    CURRENT_VERSION="${ver:-unknown}"
else
    CURRENT_VERSION="unknown"
fi

if [ "$CURRENT_VERSION" = "$OSXPHOTOS_VERSION" ]; then
    echo "Already at osxphotos $OSXPHOTOS_VERSION — nothing to do."
    exit 0
fi

echo "=== osxphotos Update: $CURRENT_VERSION -> $OSXPHOTOS_VERSION ==="
echo ""
echo "Release: https://github.com/RhetTbull/osxphotos/releases/tag/v${OSXPHOTOS_VERSION}"
echo ""
bash "$SCRIPT_DIR/../show-github-release-notes.sh" "RhetTbull/osxphotos" "v${OSXPHOTOS_VERSION}"
echo ""

WORK_DIR=$(mktemp -d)
trap 'rm -rf "$WORK_DIR"' EXIT

ZIP_NAME="osxphotos_MacOS_exe_darwin_arm64_v${OSXPHOTOS_VERSION}.zip"
URL="https://github.com/RhetTbull/osxphotos/releases/download/v${OSXPHOTOS_VERSION}/${ZIP_NAME}"

echo "Downloading osxphotos $OSXPHOTOS_VERSION (darwin-arm64)..."
curl -sfL "$URL" -o "$WORK_DIR/$ZIP_NAME"
unzip -q -o "$WORK_DIR/$ZIP_NAME" -d "$WORK_DIR/osxphotos"

# Find the osxphotos binary in the extracted zip
EXTRACTED_BIN=$(find "$WORK_DIR/osxphotos" -name "osxphotos" -type f | head -1)
if [ -z "$EXTRACTED_BIN" ]; then
    echo "Error: could not find osxphotos binary in zip"
    exit 1
fi

cp "$EXTRACTED_BIN" "$BIN_DIR/osxphotos-darwin-arm64"
chmod +x "$BIN_DIR/osxphotos-darwin-arm64"
echo "  -> osxphotos-darwin-arm64"

echo ""
echo "osxphotos $OSXPHOTOS_VERSION downloaded."
ls -lh "$BIN_DIR"/osxphotos-*
