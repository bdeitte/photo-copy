#!/bin/bash
# Pre-commit hook: runs linting and tests before git commit commands

INPUT=$(cat)

# Extract command, with fallback if jq is not available
if command -v jq >/dev/null 2>&1; then
  COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')
elif command -v python3 >/dev/null 2>&1; then
  if ! COMMAND=$(echo "$INPUT" | python3 -c "import sys, json; print(json.load(sys.stdin).get('tool_input',{}).get('command',''))" 2>/dev/null); then
    echo "Warning: pre-commit hook failed to parse input with python3" >&2
  fi
fi

# If we couldn't extract COMMAND (no jq/python3 or parse failure), fall back to
# checking the raw input for "commit" to avoid silently skipping all checks.
if [[ -z "$COMMAND" ]]; then
  if echo "$INPUT" | grep -q '"commit"'; then
    echo "Warning: pre-commit hook could not parse command (no jq or python3), running checks as a precaution" >&2
  else
    exit 0
  fi
else
  # Only check git commit commands (anchored to start of command or after && / ;)
  if ! echo "$COMMAND" | grep -qE '(^|&&|;)[[:space:]]*git[[:space:]]+commit([[:space:]]|$)'; then
    exit 0
  fi
fi

# Run linting (send all output to stderr so it doesn't interfere with hook result parsing)
if ! golangci-lint run ./... >&2; then
  echo "Lint failed. Fix lint errors before committing." >&2
  exit 2
fi

# Run tests (send all output to stderr so it doesn't interfere with hook result parsing)
if ! go test ./... >&2; then
  echo "Tests failed. Fix test failures before committing." >&2
  exit 2
fi

exit 0
