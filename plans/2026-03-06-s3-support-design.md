# Unified S3 Support via Embedded Rclone - Design Document

## Goal

Add `photo-copy s3 upload/download` commands so all copy operations go through a single CLI. Rclone is bundled in the repo (Git LFS) and invoked as a subprocess — the user never installs or configures rclone directly.

## Architecture

`photo-copy` embeds platform-specific rclone binaries in the repo. At runtime, it selects the correct binary for the current OS/architecture, writes a temporary rclone config with the user's AWS credentials, and runs rclone as a subprocess.

## New Commands

```
photo-copy s3 upload --input-dir /path/to/dir --bucket my-bucket [--prefix photos/] [--debug]
photo-copy s3 download --bucket my-bucket --output-dir /path/to/dir [--prefix photos/] [--debug]
photo-copy config s3    # interactive AWS credential setup
```

## Rclone Binary Management

- 6 binaries stored in `rclone-bin/` via Git LFS:
  - `rclone-linux-amd64`
  - `rclone-linux-arm64`
  - `rclone-darwin-amd64`
  - `rclone-darwin-arm64`
  - `rclone-windows-amd64.exe`
  - `rclone-windows-arm64.exe`
- `scripts/update-rclone.sh` downloads a pinned rclone version for all 6 platforms
- Version is pinned in the script (e.g., `RCLONE_VERSION=v1.68.2`)

## S3 Credential Flow (`photo-copy config s3`)

1. Check for `~/.aws/credentials`
2. If found: "Found existing AWS credentials. Use these? (y/n)"
3. If yes: read access key, secret key, region from AWS config
4. If no or not found: prompt for access key, secret key, region interactively
5. Save to `~/.config/photo-copy/s3.json`

## How Rclone Is Invoked

1. Load S3 credentials from `~/.config/photo-copy/s3.json`
2. Write a temporary rclone config file defining an S3 remote:
   ```
   [s3]
   type = s3
   provider = AWS
   access_key_id = <key>
   secret_access_key = <secret>
   region = <region>
   ```
3. Resolve the rclone binary path: `rclone-bin/rclone-{os}-{arch}[.exe]`
4. Execute: `<rclone-binary> copy <src> <dst> --progress --config <tmpfile>`
   - Upload: `rclone copy <input-dir> s3:<bucket>/<prefix> --progress`
   - Download: `rclone copy s3:<bucket>/<prefix> <output-dir> --progress`
5. Stream rclone stdout/stderr to the user
6. In `--debug` mode: log the full rclone command and temp config path
7. Clean up temp config on exit
8. Media file filtering: for uploads, only copy supported file types (use a temp dir or rclone `--include` flags)

## Updated Copy Matrix

Everything goes through `photo-copy`:

| From \ To | Local | S3 | Flickr | Google Photos |
|---|---|---|---|---|
| **Local** | cp | `photo-copy s3 upload` | `photo-copy flickr upload` | `photo-copy google-photos upload` |
| **S3** | `photo-copy s3 download` | n/a | s3 download, then flickr upload | s3 download, then google upload |
| **Flickr** | `photo-copy flickr download` | flickr download, then s3 upload | n/a | flickr download, then google upload |
| **Google Takeout** | `photo-copy google-photos import-takeout` | import-takeout, then s3 upload | import-takeout, then flickr upload | n/a |

## S3 Config Storage

| Credential | Stored at |
|---|---|
| AWS access key, secret key, region | `~/.config/photo-copy/s3.json` |

## Repo Structure Changes

```
photo-copy/
├── rclone-bin/                    # Git LFS tracked
│   ├── rclone-linux-amd64
│   ├── rclone-linux-arm64
│   ├── rclone-darwin-amd64
│   ├── rclone-darwin-arm64
│   ├── rclone-windows-amd64.exe
│   └── rclone-windows-arm64.exe
├── scripts/
│   └── update-rclone.sh          # downloads pinned rclone version for all platforms
├── internal/
│   ├── s3/                        # S3 upload/download via rclone subprocess
│   └── cli/                       # new s3 + config s3 commands
└── ...
```
