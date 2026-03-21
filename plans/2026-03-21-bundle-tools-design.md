# Bundle icloudpd and osxphotos Like rclone

## Overview

Move from requiring users to `pipx install` icloudpd and osxphotos to bundling their pre-built binaries in the repo, following the existing rclone pattern. Also consolidate all three tools into a shared `tools-bin/` directory.

## Directory Structure

```
tools-bin/
├── update.sh                        # Top-level: calls all sub-updates
├── rclone/
│   ├── update.sh                    # Adapted from rclone-bin/update-rclone.sh
│   ├── rclone-linux-amd64
│   ├── rclone-linux-arm64
│   ├── rclone-darwin-amd64
│   ├── rclone-darwin-arm64
│   ├── rclone-windows-amd64.exe
│   └── rclone-windows-arm64.exe
├── icloudpd/
│   ├── update.sh
│   ├── icloudpd-linux-amd64
│   ├── icloudpd-linux-arm64
│   ├── icloudpd-darwin-amd64        # No arm64 — runs via Rosetta on Apple Silicon
│   └── icloudpd-windows-amd64.exe
└── osxphotos/
    ├── update.sh
    └── osxphotos-darwin-arm64       # macOS ARM64 only
```

Binary naming: `{tool}-{GOOS}-{GOARCH}[.exe]`. icloudpd upstream uses `macos` — we rename to `darwin` for consistency.

All binaries tracked via Git LFS (`.gitattributes`).

## Update Scripts

### `tools-bin/update.sh`

Calls each tool's update script. Accepts optional tool name to update just one (e.g., `./tools-bin/update.sh rclone`), otherwise updates all.

### `tools-bin/rclone/update.sh`

Current `rclone-bin/update-rclone.sh` adapted for new location. Same logic: version detection, download from `downloads.rclone.org`, extract, rename to `rclone-{os}-{arch}`.

### `tools-bin/icloudpd/update.sh`

Downloads standalone binaries from GitHub releases (`icloud-photos-downloader/icloud_photos_downloader`). 4 platforms: linux-amd64, linux-arm64, darwin-amd64, windows-amd64. Renames from upstream naming (`icloudpd-{ver}-macos-amd64`) to project convention (`icloudpd-darwin-amd64`). Sets executable permissions. Default version configurable at top of script.

### `tools-bin/osxphotos/update.sh`

Downloads zip from GitHub releases (`RhetTbull/osxphotos`). Extracts the binary, renames to `osxphotos-darwin-arm64`. One platform only. Default version configurable at top of script.

All three per-tool scripts: accept optional version arg, detect current version, skip if already up to date.

### Upstream naming conventions (for update scripts)

- **rclone**: Downloads from `https://downloads.rclone.org/v{VER}/rclone-v{VER}-{OS}-{ARCH}.zip`. OS uses `osx` not `darwin` — must rename after extraction.
- **icloudpd**: Downloads from GitHub releases. Asset names: `icloudpd-{VER}-{OS}-{ARCH}` (e.g., `icloudpd-1.32.2-macos-amd64`). OS uses `macos` not `darwin` — must rename. Only download the `icloudpd` binary, not the separate `icloud` binary.
- **osxphotos**: Downloads from GitHub releases. Asset names: `osxphotos_MacOS_exe_darwin_arm64_v{VER}.zip` (underscore-separated, `v` prefix on version). Extract binary from zip and rename to `osxphotos-darwin-arm64`.

## Runtime Binary Resolution

### rclone (internal/s3/rclone.go)

Change `rcloneBinDir()` to look for `tools-bin/rclone/` instead of `rclone-bin/`. Same two-stage search:

1. Next to executable
2. Current working directory
3. Error if not found

### icloudpd / osxphotos (internal/icloud/icloud.go)

Replace current `findTool()` (env var + `exec.LookPath`) with:

1. Check env var override (keep for testing: `PHOTO_COPY_ICLOUDPD_PATH`, `PHOTO_COPY_OSXPHOTOS_PATH`)
2. Check `tools-bin/{tool}/` next to executable (try `{tool}-{GOOS}-{GOARCH}` first; for icloudpd on darwin/arm64, also try `icloudpd-darwin-amd64` as Rosetta fallback)
3. Check `tools-bin/{tool}/` in cwd (same Rosetta fallback logic)
4. Fall back to `exec.LookPath()` (system PATH) — for platforms where we don't bundle a binary
5. Error with install guidance if all fail

PATH fallback is the key difference from rclone — since we don't have binaries for every platform, users on unsupported platforms can still `pipx install`.

Binary name: `{tool}-{GOOS}-{GOARCH}[.exe]`.

### config icloud (internal/cli/config.go)

Auth check currently uses `exec.LookPath("icloudpd")`. Should use the same resolution logic from `icloud.go`. Export `FindTool` from the `icloud` package so `config.go` can call it directly.

## Error Messages

- icloudpd not found: `"icloudpd not found in tools-bin/ or system PATH. Run ./tools-bin/icloudpd/update.sh to download, or install manually: pipx install icloudpd"`
- osxphotos not found: `"osxphotos not found in tools-bin/ or system PATH. Run ./tools-bin/osxphotos/update.sh to download (macOS ARM64 only), or install manually: pipx install osxphotos"`
- rclone not found: Similar, referencing `tools-bin/rclone/`

## Files to Modify

- `internal/s3/rclone.go` — Change `rclone-bin/` to `tools-bin/rclone/`
- `internal/s3/rclone_test.go` — Update tests for new directory path
- `internal/icloud/icloud.go` — Replace `findTool()` with bundled binary resolution
- `internal/icloud/icloud_test.go` — Update tests for new resolution logic
- `internal/cli/config.go` — Use same resolution logic for auth check
- `.gitattributes` — Replace `rclone-bin/rclone-*` with specific LFS patterns per tool (e.g., `tools-bin/*/rclone-*`, `tools-bin/*/icloudpd-*`, `tools-bin/*/osxphotos-*`) to avoid LFS-tracking shell scripts
- `setup.sh` — Update to check `tools-bin/` instead of `rclone-bin/`, verify all three tools
- `setup.bat` — Update `rclone-bin` references to `tools-bin/rclone`
- `.golangci.yml` — Update `rclone-bin` exclusion to `tools-bin`
- `README.md` — Update setup instructions, remove `pipx install` prerequisites, note platform gaps, add Acknowledgments section
- `CLAUDE.md` — Update architecture docs referencing `rclone-bin/`

## Files to Create

- `tools-bin/update.sh`
- `tools-bin/rclone/update.sh` (moved/adapted from `rclone-bin/update-rclone.sh`)
- `tools-bin/icloudpd/update.sh`
- `tools-bin/osxphotos/update.sh`

## Files/Dirs to Remove

- `rclone-bin/` directory (after moving contents to `tools-bin/rclone/`)

## No Changes Needed

- `icloud/download.go`, `icloud/upload.go` — already receive tool path, just execute it
- S3 upload/download logic — only binary resolution path changes

## Platform Coverage

| Tool | linux amd64 | linux arm64 | darwin amd64 | darwin arm64 | windows amd64 | windows arm64 |
|------|:-----------:|:-----------:|:------------:|:------------:|:-------------:|:-------------:|
| rclone | bundled | bundled | bundled | bundled | bundled | bundled |
| icloudpd | bundled | bundled | bundled | via Rosetta | bundled | PATH fallback |
| osxphotos | n/a | n/a | PATH fallback | bundled | n/a | n/a |

## README Acknowledgments Section

Add a new section at the end of README.md:

```markdown
## Acknowledgments

photo-copy relies on these excellent open-source tools:

- **[rclone](https://rclone.org/)** — Used for S3 uploads and downloads
- **[icloudpd](https://github.com/icloud-photos-downloader/icloud_photos_downloader)** — Used for iCloud Photos downloads
- **[osxphotos](https://github.com/RhetTbull/osxphotos)** — Used for iCloud Photos uploads on macOS
```
