# Linting, Testing, and Claude Enforcement Design

## Goal

Add linting to the project, improve unit test coverage, and ensure Claude Code always runs lint and tests before committing.

## Success Criteria

- All lint checks pass with zero warnings on existing code
- All existing and new tests pass
- The pre-commit hook blocks Claude's commits when either lint or tests fail

## 1. Linting Setup

### Tool: golangci-lint

Add `.golangci.yml` at project root:

```yaml
run:
  timeout: 3m

linters:
  enable:
    - govet
    - staticcheck
    - errcheck
    - unused
    - gosimple
    - ineffassign
    - gocritic

issues:
  exclude-dirs:
    - rclone-bin
```

**Enabled linters:**
- govet — suspicious constructs
- staticcheck — comprehensive static analysis
- errcheck — unchecked error returns
- unused — dead code
- gosimple — simplifications
- ineffassign — useless assignments
- gocritic — style and performance

Note: `typecheck` is enabled by default in golangci-lint and does not need explicit configuration.

**Installation:** `brew install golangci-lint` (recommended by the golangci-lint project; `go install` is discouraged due to potential dependency version issues).

**Run command:** `golangci-lint run ./...`

## 2. Claude Code Enforcement

### Pre-commit hook

Create `.claude/settings.json` (project-level, checked into the repo) with a `PreCommit` hook:

```json
{
  "hooks": {
    "PreCommit": [
      {
        "command": "golangci-lint run ./... && go test ./..."
      }
    ]
  }
}
```

This blocks Claude from committing unless both lint and tests pass. Claude sees failure output and can fix before retrying. This only affects Claude's commits, not the user's manual git workflow.

Note: `.claude/settings.local.json` remains separate for user-specific permissions. The hook goes in the shared `settings.json` so it applies to all Claude Code users of the repo.

### CLAUDE.md updates

Add a section documenting:
- Lint command: `golangci-lint run ./...`
- Instruction to always run lint + tests after code changes, before committing
- Note about the pre-commit hook

## 3. Test Coverage Improvements

### Current state

8 test files across 6 packages, all passing. `cli/` and `cmd/` have no tests.

### Priority areas

**flickr/** (currently: transfer log + URL building tests)
- OAuth signature generation (oauth.go)
- Retry logic / exponential backoff (retryableGet)
- Rate limiting / throttle behavior
- HTTP error status code handling

**google/** (currently: takeout + upload log tests)
- Upload log deduplication
- Daily upload limit enforcement (10K cap)
- Error handling paths

**s3/** (currently: rclone arg-building tests)
- --limit flag behavior
- Error output parsing
- Config file generation edge cases

**cli/** (currently: no tests)
- Flag parsing and validation
- Argument validation (missing required args)
- Help text smoke tests

**config/** (currently: basic save/load tests)
- Malformed JSON handling
- Permission errors on config directory

### Testing patterns

- Table-driven tests (existing pattern in media_test.go)
- httptest for HTTP-level mocking in flickr/google packages
- No mocking internal interfaces — test at boundaries
- Tests should be fast and not require external services

## 4. Files to Create/Modify

| File | Action |
|------|--------|
| `.golangci.yml` | Create — linter configuration |
| `.claude/settings.json` | Create — PreCommit hook (project-level, shared) |
| `CLAUDE.md` | Modify — add lint/test instructions |
| `internal/flickr/flickr_test.go` | Modify — add retry, throttle, error tests |
| `internal/flickr/oauth_test.go` | Create — OAuth signature tests |
| `internal/google/google_test.go` | Modify — add upload limit, error tests |
| `internal/s3/s3_test.go` | Modify — add limit, error parsing tests |
| `internal/s3/rclone_test.go` | Modify — add config generation edge cases |
| `internal/cli/cli_test.go` | Create — flag/arg validation tests |
| `internal/config/config_test.go` | Modify — add edge case tests |

## 5. Implementation Order

1. Add `.golangci.yml` and fix any existing lint errors
2. Update CLAUDE.md with lint/test instructions
3. Create `.claude/settings.json` with pre-commit hook
4. Write new tests per package (flickr -> google -> s3 -> cli -> config)
