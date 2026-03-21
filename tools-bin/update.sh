#!/usr/bin/env bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

if [ -n "$1" ]; then
    # Update a specific tool
    TOOL_SCRIPT="$SCRIPT_DIR/$1/update.sh"
    if [ ! -f "$TOOL_SCRIPT" ]; then
        echo "Error: unknown tool '$1'. Available: rclone, icloudpd, osxphotos"
        exit 1
    fi
    shift
    bash "$TOOL_SCRIPT" "$@"
else
    # Update all tools
    echo "=== Updating all tools ==="
    echo ""
    bash "$SCRIPT_DIR/rclone/update.sh"
    echo ""
    bash "$SCRIPT_DIR/icloudpd/update.sh"
    echo ""
    bash "$SCRIPT_DIR/osxphotos/update.sh"
    echo ""
    echo "=== All tools updated ==="
fi
