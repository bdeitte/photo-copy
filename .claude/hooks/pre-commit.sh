#!/bin/bash
# Pre-commit hook: runs linting and tests before git commit commands.
# Uses simple grep on raw JSON input — may trigger on non-commit commands
# that happen to contain "git" and "commit", which is acceptable.

INPUT=$(cat)

# Check if the raw input contains both "git" and "commit"
if ! echo "$INPUT" | grep -q 'git'; then
  exit 0
fi
if ! echo "$INPUT" | grep -q 'commit'; then
  exit 0
fi

# Run linting
if ! golangci-lint run ./... >&2; then
  echo "Lint failed. Fix lint errors before committing." >&2
  exit 2
fi

# Run tests
if ! go test ./... >&2; then
  echo "Tests failed. Fix test failures before committing." >&2
  exit 2
fi

exit 0
