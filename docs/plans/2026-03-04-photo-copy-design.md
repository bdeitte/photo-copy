# Photo Copy - Design Document

## Goal

Provide an easy way to copy images and videos between Flickr, Google Photos, S3, and local directories. Album structure and comments are not preserved - just the raw media files.

## Architecture

Two tools with clearly separated responsibilities:

1. **rclone** (external install) - handles all S3 <-> local copies
2. **`photo-copy` Python CLI** (this repo) - handles Flickr (upload/download) and Google Photos (upload via API + import from Google Takeout)

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

## `photo-copy` Python CLI

### Commands

```
photo-copy flickr download --output-dir /path/to/dir
photo-copy flickr upload --input-dir /path/to/dir

photo-copy google-photos upload --input-dir /path/to/dir
photo-copy google-photos import-takeout --takeout-dir /path/to/takeout/zips --output-dir /path/to/dir

photo-copy config flickr    # interactive setup for Flickr API key/secret + OAuth
photo-copy config google    # interactive setup for Google OAuth credentials
```

### Behavior

- **Flickr download:** Pulls all photos from user's account via Flickr API, saving originals to the output directory. Flat file structure.
- **Flickr upload:** Uploads all images/videos in a directory to user's Flickr account.
- **Google Photos upload:** Uploads all media in a directory via the Google Photos API.
- **Google Takeout import:** Extracts photos/videos from Takeout zip files, stripping JSON metadata files, outputting clean media files.
- **Config commands:** Interactive setup for API credentials, stored in `~/.config/photo-copy/`.

### Dependencies

- `click` - CLI framework
- `flickrapi` - Flickr API client
- `google-auth` + `google-api-python-client` - Google Photos API
- `tqdm` - progress bars

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
- **Progress:** tqdm progress bars for all operations.
- **Logging:** Logs written to `~/.config/photo-copy/logs/`.
- **No metadata/album management:** Albums and comments are ignored. Raw media files only.
- **Cross-platform:** Python CLI is pure Python. rclone supports Linux/macOS/Windows.

## Repo Structure

```
photo-copy/
├── CLAUDE.md              # instructions for Claude Code sessions
├── README.md              # user-facing docs
├── setup.sh               # installs rclone + pip install -e .
├── pyproject.toml          # Python project config
├── src/
│   └── photo_copy/
│       ├── __init__.py
│       ├── cli.py          # click CLI entrypoint
│       ├── flickr.py       # Flickr upload/download
│       ├── google.py       # Google Photos upload + Takeout import
│       └── config.py       # credential management
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
