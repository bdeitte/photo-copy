# CLAUDE.md

## Always commit changes

After any loop of work, always commit the changes. This allows usage of roborev to review the changes.

## Build & Run

```bash
go build -o photo-copy ./cmd/photo-copy     # build binary
go test ./...                                 # run all tests
go test ./internal/flickr/                    # run tests for a single package
go test ./internal/flickr/ -run TestBuildAPI  # run a single test
```

Setup: `./setup.sh` (builds binary and verifies rclone binaries exist in `rclone-bin/`).

## Linting & Testing

```bash
golangci-lint run ./...                       # run all linters
go test ./...                                 # run all tests
go test ./internal/cli/ -tags integration    # run integration tests
```

Always run `golangci-lint run ./...` and `go test ./...` after making code changes, before committing. Fix any lint errors or test failures before proceeding.

A Claude Code pre-commit hook (`.claude/settings.json`) enforces this — commits will be blocked if lint or tests fail.

## Architecture

Go CLI app using [cobra](https://github.com/spf13/cobra) for command structure. Entry point: `cmd/photo-copy/main.go` calls `cli.Execute()`.

### Package layout (`internal/`)

- **cli/** - Cobra command definitions. `root.go` wires subcommands (`flickr`, `google-photos`, `s3`, `config`). Each subcommand file defines its own flags and invokes the corresponding service client.
- **config/** - JSON-based credential storage in `~/.config/photo-copy/`. Separate files per service (`flickr.json`, `google_credentials.json`, `google_token.json`, `s3.json`).
- **flickr/** - Flickr API client with OAuth 1.0a signing (`oauth.go`). Uses transfer log files (`transfer.log`) for resumable downloads.
- **google/** - Google Photos API client with OAuth2 flow. `takeout.go` handles extracting media from Google Takeout zip archives. Uses upload log files for resumable uploads.
- **s3/** - S3 operations via bundled rclone binary subprocess. `rclone.go` handles binary resolution (checks next to executable, then cwd) and temp config generation. `s3.go` builds rclone command args and runs them.
- **media/** - Shared `IsSupportedFile()` filter for supported photo/video extensions.
- **transfer/** - Shared `Result` struct for tracking transfer statistics (succeeded/failed/skipped counts, bytes, errors). `Validate()` checks for count mismatches, zero-size files, and transfer log consistency. `PrintSummary()` writes a human-readable summary to stderr. `WriteReport()` writes a detailed report file. `HandleResult()` is the standard CLI handler that runs all three.
- **logging/** - Simple leveled logger (Debug/Info/Error) writing to stderr with timestamps.
- **testutil/mockserver/** - Configurable mock HTTP servers for Flickr and Google Photos, used by integration tests. Builder API with `OnGetPhotos()`, `OnGetSizes()`, etc. Shared handler factories (`RespondJSON`, `RespondSequence`).

### Key patterns

- All service clients follow the same pattern: `NewClient(config, logger)` returning a `*Client` with `Upload`/`Download` methods taking `context.Context`.
- Resumable transfers: Flickr and Google Photos use append-only log files (`transfer.log`) to track completed files, skipping them on restart. Failed files are not logged, so re-running retries them automatically. S3 relies on rclone's built-in diffing.
- Transfer results: All Download/Upload methods return `*transfer.Result`. The CLI calls `transfer.HandleResult(result, log, dir)` which runs validation, prints a summary, and writes a report file. S3 uses `ScanDir()` after rclone completes since it can't track per-file results.
- Flickr rate limiting: Requests are throttled to 1/second (3,600/hour API limit). HTTP 429 and 5xx responses trigger exponential backoff retry (up to 5 attempts), honoring `Retry-After` headers. Implemented in `retryableGet()` and `throttle()` in `flickr.go`.
- Uploads continue past failures: Both Flickr and Google uploads continue on per-file errors (logging them) rather than failing fast, with an abort threshold of 10 consecutive failures for Flickr.
- S3 delegates to rclone subprocess rather than using the AWS SDK directly. Platform-specific rclone binaries live in `rclone-bin/` (Git LFS, downloaded via `rclone-bin/update-rclone.sh`). 6 platforms: linux/darwin/windows x amd64/arm64.
- The `--debug` flag on the root command enables verbose logging across all subcommands. CLI flags (`debug`, `limit`) are owned by a `rootOpts` struct (not package-level vars) for test isolation.
- Integration tests use env var overrides (`PHOTO_COPY_CONFIG_DIR`, `PHOTO_COPY_FLICKR_API_URL`, `PHOTO_COPY_GOOGLE_API_URL`, `PHOTO_COPY_TEST_MODE`, etc.) to redirect service URLs to mock servers and disable throttling.
- No album/metadata management — raw media files only.

### Design constraints

- **Google Photos download:** The API only allows access to photos the app itself uploaded (since March 2025). Full library export requires Google Takeout (manual zip export), hence the `import-takeout` command.
- **Google Photos upload limit:** 10,000 uploads/day, enforced in code.
- **Cross-service copies** (e.g., Flickr -> S3) go through a local directory as an intermediate step — there is no direct service-to-service transfer.
- **`config s3`** can import credentials from `~/.aws/credentials` (reads the `[default]` profile).

### Design docs

Detailed design and implementation plans live in `plans/`.

## Feature overview

Details on the features can be read in README.md

## API documents that can be helpful

rclone documentation: https://rclone.org/
Flickr API documentation: https://www.flickr.com/services/api/
Google API documentation: https://developers.google.com/photos/library/reference/rest

