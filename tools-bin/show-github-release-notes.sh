#!/usr/bin/env bash
# Fetches and displays GitHub release notes for a given repo and tag.
# Usage: show-github-release-notes.sh <github-repo> <tag> [max-lines]
# Example: show-github-release-notes.sh rclone/rclone v1.73.2 50
#
# Exits silently on any failure (network, missing python3, bad JSON, etc.)

REPO="$1"
TAG="$2"
MAX_LINES="${3:-50}"

if [ -z "$REPO" ] || [ -z "$TAG" ]; then
    exit 0
fi

NOTES=$(curl -sL "https://api.github.com/repos/${REPO}/releases/tags/${TAG}" 2>/dev/null | \
    python3 -c "
import sys, json
try:
    body = json.load(sys.stdin).get('body', '')
    print(body)
except Exception:
    pass
" 2>/dev/null) || exit 0

if [ -z "$NOTES" ]; then
    exit 0
fi

TOTAL_LINES=$(echo "$NOTES" | wc -l | tr -d ' ')

echo "Release notes:"
if [ "$TOTAL_LINES" -gt "$MAX_LINES" ]; then
    echo "$NOTES" | head -n "$MAX_LINES"
    echo "  ... ($((TOTAL_LINES - MAX_LINES)) more lines — see release URL above)"
else
    echo "$NOTES"
fi
