#!/bin/bash
# Pre-commit hook: runs linting and tests before git commit commands

INPUT=$(cat)
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')

# Only check git commit commands
if ! echo "$COMMAND" | grep -qE '\bgit\b.*\bcommit\b'; then
  exit 0
fi

# Run linting
if ! golangci-lint run ./... 2>&1; then
  echo "Lint failed. Fix lint errors before committing." >&2
  exit 2
fi

# Run tests
if ! go test ./... 2>&1; then
  echo "Tests failed. Fix test failures before committing." >&2
  exit 2
fi

exit 0
