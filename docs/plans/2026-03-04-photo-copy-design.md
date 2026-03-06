# Photo Copy - Design Document

## Goal

Provide an easy way to copy images and videos between Flickr, Google Photos, S3, and local directories. Album structure and comments are not preserved - just the raw media files.

## Architecture

Two tools with clearly separated responsibilities:

1. **rclone** (external install) - handles all S3 <-> local copies
2. **`photo-copy` Go CLI** (this repo) - handles Flickr (upload/download) and Google Photos (upload via API + import from Google Takeout)

For cross-service copies (e.g., Flickr -> S3), files go through a local temp directory as an intermediate step.

### Copy Matrix

| From \ To         | Local       | S3                              | Flickr                            | Google Photos                     |
|-------------------|-------------|----------------------------------|------------------------------------|------------------------------------|
| **Local**         | cp          | rclone                          | photo-copy                        | photo-copy                        |
| **S3**            | rclone      | rclone                          | rclone -> local, then photo-copy  | rclone -> local, then photo-copy  |
| **Flickr**        | photo-copy  | photo-copy -> local, then rclone | n/a                               | photo-copy -> local, then photo-copy |
| **Google Takeout** | unzip       | unzip, then rclone              | unzip, then photo-copy            | n/a                               |

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

photo-copy config flickr    # interactive setup for Flickr API key/secret + OAuth
photo-copy config google    # interactive setup for Google OAuth credentials
```

### Debug Mode

All commands accept a `--debug` flag that enables verbose logging to stderr. When enabled, it logs:

- Every file discovered and whether it's being processed or skipped (with reason)
- API calls being made (endpoint, key parameters)
- Files being copied/downloaded/uploaded (source, destination, size)
- Rate limit status and any throttling pauses
- Resume/skip decisions (file already transferred)
- Errors and retries with full detail

Without `--debug`, only progress bars and errors are shown.

### Behavior

- **Flickr download:** Pulls all photos from user's account via Flickr API, saving originals to the output directory. Flat file structure.
- **Flickr upload:** Uploads all images/videos in a directory to user's Flickr account.
- **Google Photos upload:** Uploads all media in a directory via the Google Photos API.
- **Google Takeout import:** Extracts photos/videos from Takeout zip files, stripping JSON metadata files, outputting clean media files.
- **Config commands:** Interactive setup for API credentials, stored in `~/.config/photo-copy/`.

### Dependencies

- `cobra` - CLI framework
- `golang.org/x/oauth2` - OAuth2 for Flickr and Google auth
- `google.golang.org/api` - Google Photos Library API
- `schollz/progressbar` - progress bars

## Rclone for S3

Rclone is installed externally and configured with an S3 remote. Claude calls rclone directly for S3 operations:

```bash
# Local -> S3
rclone copy /path/to/photos s3remote:my-bucket/photos/ --progress

# S3 -> Local
rclone copy s3remote:my-bucket/photos/ /path/to/local/ --progress
```

### Installation

- **Linux/macOS:** `setup.sh` runs rclone's official install script (`curl https://rclone.org/install.sh | sudo bash`)
- **Windows:** Manual install via `winget install Rclone.Rclone` or download from rclone.org

## Practical Details

- **Resumability:** Flickr download and Google Photos upload track what's been transferred (by filename) to avoid re-processing on re-runs.
- **Rate limits:** Google Photos 10,000 uploads/day limit is respected with clear logging when hit.
- **File types:** JPEG, PNG, TIFF, GIF, HEIC, WebP, MP4, MOV, AVI, MKV. Non-media files skipped silently.
- **Progress:** Progress bars for all operations (schollz/progressbar).
- **Debug mode:** `--debug` flag on all commands for verbose logging to stderr.
- **Logging:** Logs written to `~/.config/photo-copy/logs/`.
- **No metadata/album management:** Albums and comments are ignored. Raw media files only.
- **Cross-platform:** Go CLI compiles to a single binary for Linux/macOS/Windows. rclone supports all three.

## Repo Structure

```
photo-copy/
├── CLAUDE.md              # instructions for Claude Code sessions
├── README.md              # user-facing docs
├── setup.sh               # installs rclone + go build
├── go.mod                 # Go module config
├── go.sum
├── cmd/
│   └── photo-copy/
│       └── main.go        # entrypoint
├── internal/
│   ├── cli/               # cobra command definitions
│   ├── flickr/            # Flickr upload/download
│   ├── google/            # Google Photos upload + Takeout import
│   ├── config/            # credential management
│   └── logging/           # debug/verbose logging support
└── docs/
    └── plans/
```

## Credential Setup

| Credential | How to obtain | Stored at |
|------------|---------------|-----------|
| Flickr API key + secret | Create app at flickr.com/services/apps/create/ | `~/.config/photo-copy/flickr.json` |
| Google OAuth client ID | Create in Google Cloud Console, enable Photos Library API | `~/.config/photo-copy/google_credentials.json` |
| AWS credentials | Already set up via AWS CLI or env vars | Standard AWS config (`~/.aws/`) |
| rclone S3 remote | `rclone config` to create named remote | `~/.config/rclone/rclone.conf` |
