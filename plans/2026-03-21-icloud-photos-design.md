# iCloud Photos Integration Design

## Overview

Add iCloud Photos download and upload support to photo-copy by wrapping two external Python tools:
- **Download (cross-platform):** [icloudpd](https://github.com/icloud-photos-downloader/icloud_photos_downloader) — actively maintained, 11.7k stars, uses reverse-engineered iCloud web APIs
- **Upload (macOS only):** [osxphotos](https://github.com/RhetTbull/osxphotos) — imports into Photos.app, iCloud sync handles cloud upload

This follows the same pattern as S3, which wraps rclone as an external subprocess.

## Why External Tools

There is no official Apple API for remote iCloud Photos access. All cross-platform tools use reverse-engineered private web APIs. Wrapping battle-tested tools (icloudpd, osxphotos) avoids maintaining compatibility with Apple's undocumented APIs — that burden stays with the upstream communities.

## Command Structure

```
photo-copy
├── icloud
│   ├── download <output-dir>   # via icloudpd (cross-platform)
│   └── upload <input-dir>      # via osxphotos (macOS only)
├── config
│   └── icloud                  # verify tools installed, run icloudpd auth
```

## Config

### ICloudConfig struct (`internal/config/config.go`)

```go
type ICloudConfig struct {
    AppleID   string `json:"apple_id"`
    CookieDir string `json:"cookie_dir"` // icloudpd session cookie directory
}
```

Stored in `~/.config/photo-copy/icloud.json`. `CookieDir` defaults to `~/.config/photo-copy/icloud-cookies/`.

Add `LoadICloudConfig()` and `SaveICloudConfig()` following existing patterns.

### Config command (`photo-copy config icloud`)

1. Check `icloudpd` is installed via `exec.LookPath("icloudpd")`. If missing, print install instructions (`pip install icloudpd` or `pipx install icloudpd`) and fail.
2. Check `osxphotos` is installed via `exec.LookPath("osxphotos")`. If missing, warn that upload won't be available (don't fail — download-only is valid).
3. Prompt for Apple ID email.
4. Save `icloud.json` with apple_id and cookie_dir.
5. Run icloudpd authentication: `icloudpd --username <email> --cookie-directory <cookie-dir> --auth-only`. This triggers icloudpd's interactive 2FA flow in the terminal. Stdin/stdout/stderr are connected directly so the user can interact.
6. On success, print confirmation.

## Download (`photo-copy icloud download <output-dir>`)

### External command

```
icloudpd --directory <output-dir> --username <apple_id> --cookie-directory <cookie-dir>
```

### Flag mapping

| photo-copy flag | icloudpd flag |
|---|---|
| `--limit N` | `--recent N` (most recent N photos — note: this selects the N most recently uploaded, which differs from other services' `--limit` that caps processed count; document this) |
| `--date-range YYYY-MM-DD:YYYY-MM-DD` | TBD — verify exact flag names against target icloudpd version (`--from-date`/`--to-date` or similar). Pin minimum icloudpd version. |
| `--debug` | `--log-level debug` |

**Implementation note:** The exact icloudpd CLI flags for date filtering must be verified against the installed version during implementation. Pin and document a minimum required icloudpd version. Run `icloudpd --version` during `config icloud` to verify compatibility.

### Progress tracking

icloudpd outputs lines like `Downloading <filename>` to stderr. The Go wrapper:

1. Pre-counts total photos if possible (icloudpd logs a count at startup like `Found N items`).
2. Creates a `transfer.NewEstimator()`.
3. Parses stderr line by line. On each actual download completion line (not skips), calls `estimator.Tick()` and logs:
   ```
   [X/Y] [Estimated Z left] downloaded <filename>
   ```
   This matches the existing format used by Flickr, Google, and S3.
4. If total is unknown, falls back to `[X] downloaded <filename>` (same as S3 fallback).

### Transfer result

After icloudpd completes, call `result.ScanDir()` to populate the `transfer.Result` with file counts and sizes (same approach as S3, since icloudpd doesn't provide structured per-file output). Then `transfer.HandleResult()` for summary/validation/reporting.

### Resumability

icloudpd handles this natively — it skips files that already exist in the output directory by filename matching. No separate transfer log is needed.

### Session expiry

icloudpd exits with an error when cookies expire (~2 months). Detect this from the exit code or stderr output (exact error pattern TBD — determine during implementation) and tell the user to run `photo-copy config icloud` to re-authenticate.

## Upload (`photo-copy icloud upload <input-dir>`)

### Platform check

```go
if runtime.GOOS != "darwin" {
    return fmt.Errorf("iCloud upload requires macOS with Photos.app and iCloud Photos sync enabled")
}
```

Fail immediately with a clear error on non-macOS platforms.

### Tool check

`exec.LookPath("osxphotos")` — fail with install instructions if not found.

### External command

```
osxphotos import <input-dir> --walk --glob "*.jpg" --glob "*.jpeg" ...
```

Using `--walk` to recurse subdirectories, and `--glob` patterns matching `media.SupportedExtensions()`.

### Flag mapping

| photo-copy flag | osxphotos behavior |
|---|---|
| `--limit N` | Pre-scan directory with `media.IsSupportedFile()`, collect first N files, pass as explicit file args instead of directory |
| `--date-range` | Pre-filter files using `mediadate.ResolveDate()` (existing package), pass only matching files as args |
| `--debug` | `--verbose` flag on osxphotos |

### Progress tracking

Parse osxphotos import output line by line. Pre-count files to determine total. Use `transfer.NewEstimator()` and log:
```
[X/Y] [Estimated Z left] uploaded <filename>
```

### Transfer result

Pre-count files before import. After osxphotos completes, populate `transfer.Result` by parsing osxphotos stdout for per-file import results. osxphotos reports per-file status during import — exact output format TBD during implementation. If structured per-file output is not reliably parseable, fall back to `result.ScanDir()` (same as S3). Then `transfer.HandleResult()`.

### iCloud sync note

Importing into Photos.app means photos are local first. If iCloud Photos sync is enabled in System Settings, they automatically upload to iCloud. The CLI and documentation must make this clear — the upload is to Photos.app, not directly to iCloud.

## Package Layout

### New package: `internal/icloud/`

```
internal/icloud/
├── icloud.go       # Client struct, NewClient(), tool discovery (LookPath)
├── download.go     # Download() method — wraps icloudpd subprocess
├── upload.go       # Upload() method — wraps osxphotos subprocess, platform check
```

`NewClient` signature: `NewClient(cfg *config.ICloudConfig, log *logging.Logger) *Client`. Tool paths (icloudpd, osxphotos) are resolved via `exec.LookPath()` inside `Download()` and `Upload()` respectively, not at construction time. This allows the client to be created even if only one tool is installed (e.g., download-only without osxphotos). Env var overrides (`PHOTO_COPY_ICLOUDPD_PATH`, `PHOTO_COPY_OSXPHOTOS_PATH`) are checked first for testing.

### Modified files

- `internal/config/config.go` — add `ICloudConfig`, `LoadICloudConfig()`, `SaveICloudConfig()`, `icloudFile` constant
- `internal/cli/icloud.go` — new file: `newICloudCmd(opts)`, `newICloudDownloadCmd(opts)`, `newICloudUploadCmd(opts)`
- `internal/cli/config.go` — add `newConfigICloudCmd()`, register in `newConfigCmd()`
- `internal/cli/root.go` — register `newICloudCmd(opts)` in `rootCmd`; update `PersistentPreRunE` to warn when `--no-metadata` is used with iCloud commands (no-op, same as S3) and include `config icloud` in the `--date-range` no-op warning
- `README.md` — add iCloud section documenting commands, setup, macOS-only upload limitation; update tagline to include iCloud Photos
- `CLAUDE.md` — add `icloud/` package description in Architecture section

## README.md Updates

Add a new section under the existing service documentation:

- **iCloud Photos** heading with download and upload subcommands
- Setup instructions: install icloudpd (`pipx install icloudpd`), install osxphotos (`pipx install osxphotos`, macOS only), run `photo-copy config icloud`
- Document that download works cross-platform, upload requires macOS with Photos.app and iCloud Photos sync enabled
- Document 2FA requirement and session cookie expiry (~2 months)
- Document that Advanced Data Protection must be disabled for downloads
- Note that `--no-metadata` has no effect on iCloud commands
- Update the project tagline to include iCloud Photos

## CLAUDE.md Updates

Add to the package layout section:
- **icloud/** — iCloud Photos client. Downloads via icloudpd subprocess (cross-platform). Uploads via osxphotos subprocess (macOS only, imports into Photos.app which syncs to iCloud). No direct Apple API — both operations delegate to external Python tools, similar to how S3 delegates to rclone.

Add to design constraints:
- **iCloud Photos upload:** macOS only — imports into Photos.app via osxphotos, relies on iCloud Photos sync to upload to cloud. No cross-platform upload API exists.
- **iCloud Photos authentication:** Requires Apple ID with 2FA. Session cookies expire ~2 months. Advanced Data Protection must be disabled.

## Error Handling

| Scenario | Behavior |
|---|---|
| icloudpd not installed | Error with `pip install icloudpd` / `pipx install icloudpd` instructions |
| osxphotos not installed | Error with `pip install osxphotos` / `pipx install osxphotos` instructions |
| Session expired (download) | Detect from icloudpd output, tell user to run `photo-copy config icloud` |
| Non-macOS upload | Immediate error explaining macOS + Photos.app requirement |
| iCloud sync disabled | Document in README; CLI can't detect this programmatically |
| 2FA required during download | icloudpd handles this interactively if session is fresh; otherwise re-auth needed |

## Testing

### Unit tests (`internal/icloud/`)

- Test arg-building functions: `buildDownloadArgs()`, `buildUploadArgs()`
- Test platform detection for upload
- Test config loading/saving
- Test output parsing (progress line extraction from icloudpd/osxphotos output)

### Integration tests (`internal/cli/`)

Use env var overrides following existing patterns:
- `PHOTO_COPY_ICLOUDPD_PATH` — override icloudpd binary path, point to a mock script
- `PHOTO_COPY_OSXPHOTOS_PATH` — override osxphotos binary path, point to a mock script

Mock scripts are simple shell scripts that echo expected output patterns and create expected files in the output directory. This follows the same approach that could be used for rclone testing.

## What's NOT Included

- No album management (consistent with rest of project)
- No metadata embedding on download (icloudpd preserves original files)
- No `--no-metadata` flag effect (no metadata operations to skip)
- No bundled binaries (unlike rclone for S3 — icloudpd/osxphotos are pip-installed)
- No direct iCloud API implementation
- No shared album support
- No Shared Library support
