# CLAUDE.md

## Always commit changes

After any loop of work, always commit the changes. This allows usage of roborev to review the changes.

## Build & Run

Go version: 1.25 (see `go.mod`).

```bash
go build -o photo-copy ./cmd/photo-copy     # build binary
go test ./...                                 # run all tests
go test ./internal/flickr/                    # run tests for a single package
go test ./internal/flickr/ -run TestBuildAPI  # run a single test
```

Setup: `./setup.sh` (builds binary and verifies tool binaries exist in `tools-bin/`).

## Linting & Testing

```bash
golangci-lint run ./...                       # run all linters
go test ./...                                 # run all tests
go test ./internal/cli/ -tags integration    # run integration tests
```

Always run `golangci-lint run ./...` and `go test ./...` after making code changes, before committing. Fix any lint errors or test failures before proceeding.

A Claude Code PreToolUse hook (`.claude/settings.json` + `.claude/hooks/pre-commit.sh`) runs lint, tests, and integration tests before any `git commit` Bash command — commits will be blocked if any fail.

## Go Coding Standards

Follow [Effective Go](https://go.dev/doc/effective_go) conventions:

- **Formatting:** `gofmt` is the standard. The `golangci-lint` config (`.golangci.yml`) enables: govet, staticcheck, errcheck, unused, ineffassign, gocritic.
- **Naming:** MixedCaps/mixedCaps only, no underscores. No `Get` prefix on getters. One-method interfaces use `-er` suffix.
- **Errors:** Always check errors immediately. Return `(value, error)` pairs. Never silently ignore errors.
- **Comments:** Doc comments on all exported identifiers. No intervening blank line between comment and declaration.
- **Control flow:** Eliminate error cases early (guard clauses), keep the success path unindented. Omit `else` when the `if` body ends in `return`.
- **Concurrency:** Share memory by communicating (channels), not by communicating via shared memory.
- **Zero values:** Design types so zero values are useful without further initialization.

## Architecture

Go CLI app using [cobra](https://github.com/spf13/cobra) for command structure. Entry point: `cmd/photo-copy/main.go` calls `cli.Execute()`.

### Package layout (`internal/`)

- **cli/** - Cobra command definitions. `root.go` wires subcommands (`flickr`, `google`, `s3`, `icloud`, `config`) and uses `signal.NotifyContext` + `ExecuteContext` for cancellation. Each subcommand file defines its own flags and invokes the corresponding service client. `config_interactive.go` handles interactive credential prompts. Integration tests are split per service (`flickr_integration_test.go`, `google_integration_test.go`, `integration_test.go`).
- **config/** - JSON-based credential storage in `~/.config/photo-copy/`. Separate files per service (`flickr.json`, `google_credentials.json`, `google_token.json`, `s3.json`).
- **flickr/** - Flickr API client with OAuth 1.0a signing (`oauth.go`), HTTP helpers with retry/throttle (`http.go`), download logic (`download.go`), upload logic (`upload.go`), date resolution (`dates.go`), and metadata building (`metadata.go`). Uses transfer log files (`transfer.log`) for resumable downloads.
- **google/** - Google Photos API client with OAuth2 flow. `takeout.go` handles extracting media from Google Takeout zip archives with context-aware cancellation. Uses upload log files for resumable uploads.
- **s3/** - S3 operations via bundled rclone binary subprocess. `rclone.go` handles binary resolution (checks next to executable, then cwd) and temp config generation. `s3.go` builds rclone command args and runs them. Upload defaults to Glacier Deep Archive (`--s3-storage-class DEEP_ARCHIVE`), configurable via `--storage-class` flag.
- **icloud/** — iCloud Photos client. Downloads via icloudpd subprocess (cross-platform). Uploads via osxphotos subprocess (macOS only, imports into Photos.app which syncs to iCloud). No direct Apple API — both operations delegate to bundled binaries: icloudpd binaries are in `tools-bin/icloudpd/` and osxphotos binaries are in `tools-bin/osxphotos/`, with PATH fallback for unsupported platforms.
- **xmp/** - Shared XMP metadata types and Dublin Core packet builder. `Metadata` struct (Title, Description, Tags) with `IsEmpty()` check and `BuildDublinCorePacket()` for generating xpacket-wrapped XML. Used by both `jpegmeta` and `mp4meta`.
- **jpegmeta/** - Writes XMP metadata into JPEG files as APP1 segments via the shared `xmp` package. Also reads EXIF DateTimeOriginal via `rwcarlsen/goexif` for date-range filtering of uploads.
- **mp4meta/** - Sets creation/modification timestamps in MP4/MOV container metadata (`mvhd`/`tkhd`/`mdhd` boxes) using `abema/go-mp4`, and writes XMP metadata as UUID boxes via the shared `xmp` package. Also reads creation time from the `mvhd` box for date-range filtering of uploads.
- **daterange/** - Parses `--date-range YYYY-MM-DD:YYYY-MM-DD` flag values into a `DateRange` struct with optional `After`/`Before` bounds. `Contains()` checks if a time falls within the range.
- **mediadate/** - Resolves the best available date from a media file: EXIF `DateTimeOriginal` for JPEGs (via `rwcarlsen/goexif`), `mvhd` creation time for MP4/MOV (via `abema/go-mp4`), falling back to file modification time.
- **media/** - Shared `IsSupportedFile()` filter for supported photo/video extensions.
- **transfer/** - Shared `Result` struct for tracking transfer statistics (succeeded/failed/skipped counts, bytes, errors). `Validate()` checks for count mismatches and zero-size files (suppressed when `Limited` is true). `PrintSummary()` and `WriteReport()` use `ScanLabel` to customize output labels. `HandleResult()` is the standard CLI handler that runs validation, summary, and report. `Estimator` provides time-remaining estimates.
- **logging/** - Simple leveled logger (Debug/Info/Error) writing to stderr with timestamps.
- **testutil/mockserver/** - Configurable mock HTTP servers for Flickr and Google Photos, used by integration tests. Builder API with `OnGetPhotos()`, `OnGetSizes()`, etc. Shared handler factories (`RespondJSON`, `RespondSequence`).

### Key patterns

- All service clients follow the same pattern: `NewClient(config, logger)` returning a `*Client` with `Upload`/`Download` methods taking `context.Context`. Google's `NewClient` additionally takes `ctx` and `configDir` parameters and returns `(*Client, error)` because it performs OAuth2 token exchange during construction.
- Resumable transfers: Flickr uses an append-only `transfer.log` for resumable downloads; Google Photos uses `.photo-copy-upload.log` for resumable uploads. Failed files are not logged, so re-running retries them automatically. S3 relies on rclone's built-in diffing.
- Transfer results: All Download/Upload methods return `*transfer.Result`. The CLI calls `transfer.HandleResult(result, log, dir)` which runs validation, prints a summary, and writes a report file. S3 uses `ScanDir()` after rclone completes since it can't track per-file results. When `--limit` stops a transfer early, `result.Limited` suppresses the expected-vs-actual count warning.
- Flickr rate limiting: Requests are throttled to 1/second (3,600/hour API limit) with adaptive throttle that increases on 429 responses and gradually decreases on success. HTTP 429 and 5xx responses trigger exponential backoff retry (up to 8 attempts), honoring `Retry-After` headers. Implemented in `retryableGet()` and `throttle()`/`onRateLimited()`/`onRequestSuccess()` in `flickr.go`.
- Uploads continue past failures: Both Flickr and Google uploads continue on per-file errors (logging them) rather than failing fast, with an abort threshold of 10 consecutive failures for Flickr.
- S3 delegates to rclone subprocess rather than using the AWS SDK directly. Platform-specific rclone binaries live in `tools-bin/rclone/` (Git LFS, downloaded via `tools-bin/rclone/update.sh`). 6 platforms: linux/darwin/windows x amd64/arm64.
- The `--debug` flag on the root command enables verbose logging across all subcommands. CLI flags (`debug`, `limit`) are owned by a `rootOpts` struct (not package-level vars) for test isolation.
- The `--no-metadata` flag on `flickr download` skips all metadata operations (XMP, MP4 creation time, filesystem timestamps).
- The `--date-range` flag filters by date: Flickr download uses API dates, uploads use `mediadate.ResolveDate()`, S3 uses rclone `--min-age`/`--max-age` (file mod time, not metadata).
- S3 commands accept a positional destination argument: plain bucket name (`my-bucket/prefix/`) or full S3 URL (`https://bucket.s3.region.amazonaws.com/prefix/`). URL format overrides the configured region. Parsing is in `cli/s3_destination.go`.
- Root-level flags `--no-metadata` and `--date-range` live in `rootOpts` with no-op warnings via `PersistentPreRunE` when used with inapplicable commands.
- Integration tests use env var overrides (`PHOTO_COPY_CONFIG_DIR`, `PHOTO_COPY_FLICKR_API_URL`, `PHOTO_COPY_GOOGLE_API_URL`, `PHOTO_COPY_TEST_MODE`, etc.) to redirect service URLs to mock servers and disable throttling.
- Flickr downloads preserve original dates: `date_taken` (preferred) or `date_upload` (fallback) from the API. Video files (`.mp4`, `.mov`) get MP4 container metadata updated via the `mp4meta` package; all files get file system timestamps set via `os.Chtimes`.
- Flickr downloads embed XMP metadata: title, description (HTML-stripped), and tags from the Flickr API are written as XMP (Dublin Core) into JPEG files (via `jpegmeta`) and MP4/MOV files (via `mp4meta`). Both use the shared `xmp` package for packet generation. HTML in descriptions is stripped using `golang.org/x/net/html` tokenizer. Metadata building is in `flickr/metadata.go`.
- HTTP timeouts use transport-level settings (DialContext, TLS, ResponseHeader) via `defaultTimeoutTransport()` cloned from `http.DefaultTransport` to preserve proxy/HTTP2/keepalive. No client-wide `Timeout` since large media transfers need unlimited body read time.
- OAuth token refresh is timeout-protected by injecting a timeout-configured `http.Client` via the `oauth2.HTTPClient` context key.
- Flickr OAuth nonce generation and signing return errors instead of panicking.
- Cobra command `Annotations` map declares feature support (e.g., `"supportsMetadata": "true"`) for declarative flag warnings.
- Google Takeout extraction is context-aware: `extractFile` uses `copyWithContext` for cancellable large file copies, and `extractMediaFromZip` checks `ctx.Err()` after extraction failures to return immediately on cancellation rather than continuing to the next file.
- No album management — raw media files with embedded metadata only.

### Design constraints

- **Google Photos download:** The API only allows access to photos the app itself uploaded (since March 2025). Full library export requires Google Takeout (manual zip export), hence the `google download` command requires Google Takeout zip files.
- **Google Photos upload limit:** 10,000 uploads/day, enforced in code.
- **Cross-service copies** (e.g., Flickr -> S3) go through a local directory as an intermediate step — there is no direct service-to-service transfer.
- **`config s3`** can import credentials from `~/.aws/credentials` (reads the `[default]` profile).
- **iCloud Photos upload:** macOS only — imports into Photos.app via osxphotos, relies on iCloud Photos sync to upload to cloud. No cross-platform upload API exists.
- **iCloud Photos authentication:** Requires Apple ID with 2FA. Session cookies expire ~2 months. Advanced Data Protection must be disabled.

## Feature overview

Details on the features can be read in README.md

## Reminders

- Require exact regression coverage for bug fixes. If a change fixes a branch-specific bug, add or
update a test that exercises the exact failing branch through the public entrypoint. Do not accept
tests that only cover adjacent branches or internal helpers when the bug is user-visible at the
exported API.
- Avoid timing-based tests for cancellation, retries, or concurrency. Prefer deterministic hooks,
synchronization points, or injected fakes over sleep/polling-based tests.
- Propagate context end-to-end for long-running commands. When a CLI command uses cmd.Context() or
root signal handling, all downstream loops, file-copy paths, and subprocesses must observe that
context and return cancellation promptly.
- Do not use a fixed http.Client.Timeout on clients that upload or download media bodies. Use http.
DefaultTransport.Clone() and configure connect/TLS/response-header timeouts, or operation-specific
contexts, so large transfers are not aborted mid-stream.
- Treat retry/backoff arithmetic as boundary-sensitive. Clamp before overflow-prone multiplication,
distinguish retryable vs permanent API failures, and add tests at the real boundary values and
beyond them.
- Keep transfer reporting semantically accurate. Do not report scanned/existing files as successful
transfers, and clearly label limit-truncated runs in logs, summaries, and reports.
- Preserve CLI compatibility and precise user messaging. Renamed commands should keep deprecated
aliases when practical, nested-command warnings should use the full command path, and argument
validators must distinguish missing from extra positional arguments.
- Keep docs and architecture notes implementation-accurate. README, CLAUDE/AGENTS notes, and command
examples must reflect actual command names, argument counts, service-specific exceptions, and
runtime behavior.
- Harden tool update scripts for real failure modes. Use curl -sfL (or equivalent), guard platform-
specific installers, and keep tool resolution behavior consistent across commands and setup flows.
- Do not surface non-blocking, out-of-scope nits as failing findings. Performance observations, pre-
existing cleanup, and optional platform enhancements should only fail review when the diff
introduces a concrete user-visible risk.

## API documents that can be helpful

rclone documentation: https://rclone.org/
Flickr API documentation: https://www.flickr.com/services/api/
Google API documentation: https://developers.google.com/photos/library/reference/rest

