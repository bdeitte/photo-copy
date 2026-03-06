# Photo Copy - Design Document

## Goal

Provide an easy way to copy images and videos between Flickr, Google Photos, S3, and local directories. Album structure and comments are not preserved - just the raw media files.

## Architecture

A single Go CLI (`photo-copy`) handles all operations. S3 support uses bundled rclone binaries (stored in the repo via Git LFS) invoked as a subprocess — the user never installs or configures rclone directly.

For cross-service copies (e.g., Flickr -> S3), files go through a local temp directory as an intermediate step.

### Copy Matrix

| From \ To | Local | S3 | Flickr | Google Photos |
|---|---|---|---|---|
| **Local** | cp | `photo-copy s3 upload` | `photo-copy flickr upload` | `photo-copy google-photos upload` |
| **S3** | `photo-copy s3 download` | n/a | s3 download, then flickr upload | s3 download, then google upload |
| **Flickr** | `photo-copy flickr download` | flickr download, then s3 upload | n/a | flickr download, then google upload |
| **Google Takeout** | `photo-copy google-photos import-takeout` | import-takeout, then s3 upload | import-takeout, then flickr upload | n/a |

### Google Photos API Constraints

- **Upload:** Works via the Google Photos Library API. All uploads are stored at original quality and count toward storage quota. Rate limited to 10,000 uploads/day.
- **Download:** Since March 31, 2025, the API only allows access to photos the app itself uploaded. To export a full Google Photos library, the only option is Google Takeout (manual export from Google).

## `photo-copy` Go CLI

### Commands

```
photo-copy flickr download --output-dir /path/to/dir [--debug]
photo-copy flickr upload --input-dir /path/to/dir [--debug]

photo-copy google-photos upload --input-dir /path/to/dir [--debug]
photo-copy google-photos import-takeout --takeout-dir /path/to/takeout/zips --output-dir /path/to/dir [--debug]

photo-copy s3 upload --input-dir /path/to/dir --bucket my-bucket [--prefix photos/] [--debug]
photo-copy s3 download --bucket my-bucket --output-dir /path/to/dir [--prefix photos/] [--debug]

photo-copy config flickr    # interactive setup for Flickr API key/secret + OAuth
photo-copy config google    # interactive setup for Google OAuth credentials
photo-copy config s3        # interactive setup for AWS credentials (can import from ~/.aws/credentials)
```

### Debug Mode

All commands accept a `--debug` flag that enables verbose logging to stderr. When enabled, it logs:

- Every file discovered and whether it's being processed or skipped (with reason)
- API calls being made (endpoint, key parameters)
- Files being copied/downloaded/uploaded (source, destination, size)
- Rate limit status and any throttling pauses
- Resume/skip decisions (file already transferred)
- Rclone commands being executed (for S3 operations)
- Errors and retries with full detail

Without `--debug`, only progress bars and errors are shown.

### Behavior

- **Flickr download:** Pulls all photos from user's account via Flickr API, saving originals to the output directory. Flat file structure.
- **Flickr upload:** Uploads all images/videos in a directory to user's Flickr account.
- **Google Photos upload:** Uploads all media in a directory via the Google Photos API.
- **Google Takeout import:** Extracts photos/videos from Takeout zip files, stripping JSON metadata files, outputting clean media files.
- **S3 upload:** Copies media files from a local directory to an S3 bucket via bundled rclone.
- **S3 download:** Copies media files from an S3 bucket to a local directory via bundled rclone.
- **Config commands:** Interactive setup for API credentials, stored in `~/.config/photo-copy/`.

### Dependencies

- `cobra` - CLI framework
- `golang.org/x/oauth2` - OAuth2 for Flickr and Google auth
- `schollz/progressbar` - progress bars
- `rclone` (bundled binaries) - S3 operations

## Bundled Rclone

Rclone binaries for 6 platforms are stored in `rclone-bin/` via Git LFS:

- `rclone-linux-amd64`, `rclone-linux-arm64`
- `rclone-darwin-amd64`, `rclone-darwin-arm64`
- `rclone-windows-amd64.exe`, `rclone-windows-arm64.exe`

At runtime, `photo-copy` selects the correct binary for the current OS/architecture, writes a temporary rclone config with the user's AWS credentials, and invokes rclone as a subprocess.

To update rclone: `./scripts/update-rclone.sh v1.68.2`

## Practical Details

- **Resumability:** Flickr download and Google Photos upload track what's been transferred (by filename) to avoid re-processing on re-runs.
- **Rate limits:** Google Photos 10,000 uploads/day limit is respected with clear logging when hit.
- **File types:** JPEG, PNG, TIFF, GIF, HEIC, WebP, MP4, MOV, AVI, MKV. Non-media files skipped silently.
- **Progress:** Progress bars for all operations (schollz/progressbar for API operations, rclone --progress for S3).
- **Debug mode:** `--debug` flag on all commands for verbose logging to stderr.
- **No metadata/album management:** Albums and comments are ignored. Raw media files only.
- **Cross-platform:** Go CLI compiles to a single binary. Rclone binaries bundled for Linux/macOS/Windows (amd64 + arm64).

## Repo Structure

```
photo-copy/
├── README.md              # user-facing docs
├── setup.sh               # go build + verify rclone binaries
├── go.mod                 # Go module config
├── go.sum
├── .gitattributes         # Git LFS tracking for rclone binaries
├── cmd/
│   └── photo-copy/
│       └── main.go        # entrypoint
├── internal/
│   ├── cli/               # cobra command definitions
│   ├── flickr/            # Flickr upload/download + OAuth
│   ├── google/            # Google Photos upload + Takeout import
│   ├── s3/                # S3 upload/download via rclone subprocess
│   ├── config/            # credential management
│   ├── media/             # media file type detection
│   └── logging/           # debug/verbose logging support
├── rclone-bin/            # bundled rclone binaries (Git LFS)
├── scripts/
│   └── update-rclone.sh   # download/update rclone for all platforms
└── docs/
    └── plans/
```

## Credential Setup

| Credential | How to obtain | Stored at |
|------------|---------------|-----------|
| Flickr API key + secret | Create app at flickr.com/services/apps/create/ | `~/.config/photo-copy/flickr.json` |
| Google OAuth client ID | Create in Google Cloud Console, enable Photos Library API | `~/.config/photo-copy/google_credentials.json` |
| AWS credentials | `photo-copy config s3` (can import from `~/.aws/credentials`) | `~/.config/photo-copy/s3.json` |
