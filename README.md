## Photo Copy
Copy and backup your photos and videos.<br>
Copy between iCloud Photos, Google Photos, Flickr, AWS S3, and local directories.<br>
<a href="https://github.com/bdeitte/photo-copy/actions/workflows/ci.yml"><img src="https://github.com/bdeitte/photo-copy/actions/workflows/ci.yml/badge.svg" alt="CI"></a>

## Overview

photo-copy copies photos and videos between cloud services and local directories. Each service has `download` and `upload` commands that transfer between a local directory and that service (`google download` takes a Takeout zip directory and output directory — see [Google Photos](#google-photos)). To copy between two services (e.g., Flickr to S3), download to a local directory first, then upload from that directory.

## Quick Start

> **Note:** This project is in active development. Google Photos and Flickr support has been well-tested and is in good shape. S3 and iCloud support is still in an alpha state.

Requires Go 1.25+

1. Run `./setup.sh` (or `setup.bat` on Windows)
2. Configure credentials for the service you want to use (e.g., `./photo-copy config flickr`). Not all commands require config — see the table below.
3. Download or upload: `./photo-copy <service> download <dir>` or `./photo-copy <service> upload <dir>`. (Google Photos download takes two directories: `./photo-copy google download <takeout-dir> <output-dir>` — see [Google Photos](#google-photos).)

### Service capabilities

| Service | Download | Upload | Config required | Platform limits |
|---------|----------|--------|-----------------|-----------------|
| Flickr | Yes | Yes | Yes | — |
| Google Photos | Yes, through Takeout import | Yes | Upload only | 10,000 uploads/day |
| S3 | Yes | Yes | Yes | — |
| iCloud | Yes | macOS only | Download only | Upload requires Photos.app |

## Usage Details

### Configure credentials

Each command will tell you what you need to do. Credentials are saved to `~/.config/photo-copy/` (override with `PHOTO_COPY_CONFIG_DIR`).

```bash
./photo-copy config flickr    # Flickr API key + OAuth
./photo-copy config google    # Google OAuth credentials for upload (not needed for download)
./photo-copy config s3        # AWS credentials for S3
./photo-copy config icloud    # iCloud download authentication (not needed for upload)
```

### Flickr

```bash
# Download all photos from Flickr
./photo-copy flickr download ../flickr-photos

# Download only photos taken in 2023
./photo-copy flickr download --date-range 2023-01-01:2023-12-31 ../flickr-photos

# Download the first 100 photos
./photo-copy flickr download --limit 100 ../flickr-photos

# Upload local photos to Flickr
./photo-copy flickr upload ../photos

# Upload only photos from 2020 onward, limit to 500
./photo-copy flickr upload --date-range 2020-01-01: --limit 500 ../photos
```

### Google Photos

The Google Photos API only allows access to photos the app itself uploaded, so downloading your full library requires Google Takeout (a manual zip export from Google).

```bash
# Upload local photos to Google Photos
./photo-copy google upload ../photos

# Upload only photos taken before 2024, limit to 1000
./photo-copy google upload --date-range :2023-12-31 --limit 1000 ../photos

# Download: export your library via Google Takeout, then point at the directory of zips
./photo-copy google download ../takeout-zips ../google-photos
```

**Google Takeout download details:**
- **Album preservation** — Files in album folders are extracted into subdirectories (e.g., `Trip to Paris/photo.jpg`). Files in Google's year folders (`Photos from 2022`) are extracted flat to the output root.
- **Deduplication** — Photos that appear in both an album folder and a year folder are only extracted once (the album copy is kept). Photos in multiple albums are kept in all albums.
- **Metadata embedding** — Title, description, and creation date from Google Takeout JSON sidecar files are embedded as XMP metadata into JPEG and MP4/MOV files. File system timestamps are set from the photo's taken date.
- **`--no-metadata`** — Skip metadata embedding during Google Takeout import.

### S3

Uploads default to Glacier Deep Archive storage class. This will cause delays in retrieving the archives but is an inexpensive option to use for long-term storage. Use `--storage-class` to change (e.g. `STANDARD`, `GLACIER`).

```bash
# Upload local photos to S3
./photo-copy s3 upload ../photos my-bucket/photos/

# Upload to S3 using full URL
./photo-copy s3 upload ../photos https://deitte-backup-things.s3.us-west-2.amazonaws.com/deitte-com/

# Upload with standard storage class
./photo-copy s3 upload ../photos my-bucket/photos/ --storage-class STANDARD

# Upload only files modified in 2023, limit to 200
./photo-copy s3 upload --date-range 2023-01-01:2023-12-31 --limit 200 ../photos my-bucket/photos/

# Download photos from S3
./photo-copy s3 download my-bucket/photos/ ../photos

# Download using full S3 URL
./photo-copy s3 download https://deitte-backup-things.s3.us-west-2.amazonaws.com/deitte-com/ ../photos

# Download only files modified since 2024
./photo-copy s3 download --date-range 2024-01-01: my-bucket/photos/ ../photos

# Download from Glacier/Deep Archive (first run initiates restore)
./photo-copy s3 download my-bucket/photos/ ../photos
# Output: "Initiating Glacier restore for 150 files (Bulk tier, ~5-12 hours)..."
# Output: "150 files still restoring from Glacier — re-run this command in a few hours"

# Re-run after a few hours to download restored files
./photo-copy s3 download my-bucket/photos/ ../photos
```

**Glacier/Deep Archive downloads:** Files stored in Glacier or Deep Archive storage classes require a two-step process. The first run automatically initiates a Bulk restore (5-12 hours). Re-run the same command after the restore completes to download the files. Any files already restored or in Standard storage class are downloaded immediately. You can re-run as many times as needed — already-downloaded files are skipped.

### iCloud Photos

Download works on all platforms. Upload requires macOS with Photos.app and iCloud Photos sync enabled.

```bash
# Download all photos from iCloud
./photo-copy icloud download ../icloud-photos

# Download the 50 most recently uploaded photos
./photo-copy icloud download --limit 50 ../icloud-photos

# Upload local photos to iCloud (macOS only — imports into Photos.app)
./photo-copy icloud upload ../photos

# Upload only photos from 2022-2023
./photo-copy icloud upload --date-range 2022-01-01:2023-12-31 ../photos
```

**Notes on iCloud download:**
- Download requires Apple ID with 2FA. Run `photo-copy config icloud` to authenticate.
- Session cookies expire approximately every 2 months — re-run config to re-authenticate.
- Advanced Data Protection must be disabled for downloads.
- icloudpd for downloads bundled for Linux amd64/arm64, macOS amd64 (Apple Silicon runs via Rosetta 2), and Windows amd64. Other platforms: `pipx install icloudpd`.

**Notes on iCloud upload:**
- Upload does not require `config icloud` — it imports files directly into Photos.app via osxphotos. If iCloud Photos sync is enabled in System Settings, they automatically upload to iCloud.
- `--no-metadata` has no effect on iCloud commands.
- osxphotos for uploads bundled for macOS ARM64 only. Intel Macs: `pipx install osxphotos`.

## Features

### Resumable transfers

All transfers are resumable — if a download or upload is interrupted, re-running the same command picks up where it left off:

- **Flickr downloads** — A `transfer.log` file in the output directory tracks each successfully downloaded file. Already-downloaded files are skipped on restart.
- **Google Photos uploads** — An upload log file tracks completed uploads. Files in subdirectories are tracked by relative path.
- **S3 uploads/downloads** — Handled by rclone, which compares source and destination and only transfers changed or missing files.
- **iCloud downloads** — Handled by icloudpd, which skips files that already exist in the output directory by filename matching.
- **iCloud uploads** — Each run imports all files; Photos.app deduplicates automatically.

Files that fail during transfer are not marked as completed in the log, so re-running the same command will automatically retry them while skipping files that already succeeded.

**Note on duplicates:** Resumable transfers skip files already completed in the current transfer run. However, Flickr and Google Photos uploads do not check whether a file already exists in the service — re-uploading the same files to a new directory will create duplicates. S3 avoids this via rclone's file comparison. iCloud uploads rely on Photos.app's built-in deduplication. Google Takeout import renames files on filename collision (e.g., `photo_1.jpg`).

### Rate limiting & retry

- **Flickr** — Requests are throttled to stay under Flickr's 3,600 requests/hour API limit, starting at 1 request/second. The interval adapts automatically: on HTTP 429 (rate limit) responses, the interval doubles (up to 30s between requests), then gradually decreases back to 1/second as requests succeed. HTTP 429 responses are retried indefinitely with exponential backoff capped at 5 minutes between attempts — large downloads will pause and resume automatically when Flickr's rate limit window resets. HTTP 5xx server errors are retried up to 7 times with exponential backoff. Both honor the `Retry-After` header when present. This applies to both API calls and photo downloads.
- **Flickr uploads** — Uploads continue past individual file failures (logging each error) rather than failing fast. If 10 uploads fail consecutively, the transfer aborts to avoid wasting time on a systemic issue (e.g., expired auth token).
- **Google Photos** — Subject to a 10,000 uploads/day limit. If more files are queued, the upload is automatically capped at 10,000 with a log message — re-run the next day to continue.

### Filtering Options

- `--date-range YYYY-MM-DD:YYYY-MM-DD` — Only process files within the specified date range. Either side can be omitted for open-ended ranges (e.g., `2020-01-01:` for everything from 2020 onward, `:2023-12-31` for everything up to end of 2023).
- `--limit N` — Only process the first N files. Useful for testing a transfer before running the full batch, or for processing files incrementally. For iCloud downloads, this maps to icloudpd's `--recent` flag, which selects the N most recently uploaded photos.

**Date sources by command:**
- **Flickr download**: Uses `date_taken` (preferred) or `date_upload` from the Flickr API.
- **Flickr/Google upload**: Reads EXIF DateTimeOriginal (JPEG) or MP4 creation time, falling back to file modification time.
- **S3 upload/download**: Uses rclone's `--min-age`/`--max-age` flags, which filter by **file modification time** (or S3 object LastModified timestamp), not embedded metadata dates. This is a limitation of delegating to rclone.
- **iCloud download**: Uses icloudpd's date filtering (photo creation date from iCloud). Note: `--limit` maps to icloudpd's `--recent` flag, which selects the N most recently uploaded photos.
- **iCloud upload**: Reads EXIF DateTimeOriginal (JPEG) or MP4 creation time, falling back to file modification time (same as Flickr/Google upload).

### Subdirectory support

All upload commands recursively scan subdirectories for media files. Files found in subdirectories are logged and uploaded. For S3, the subdirectory structure is preserved in the upload path. Google Takeout downloads also preserve album folder structure as subdirectories.

### Transfer summary & validation

Every transfer automatically prints a summary and writes a detailed report file when it finishes. The summary includes file counts (succeeded, skipped, failed), total size transferred, and elapsed time. Any errors are re-listed so they're easy to spot.

Post-transfer validation checks for potential issues:

- **Count verification** — Compares the expected file count (from the service API) against the actual number of files succeeded, skipped, and failed. A mismatch means some files were unaccounted for.
- **Zero-size file detection** — Scans the output directory after downloads for empty files that may indicate incomplete transfers.
- **Transfer log consistency** (Flickr) — Verifies that every entry in `transfer.log` has a corresponding file on disk, catching orphaned log entries from interrupted downloads.

A report file (`photo-copy-report-{service}-{operation}-{timestamp}.txt`) is written to the transfer directory with the full breakdown of counts, errors, and validation warnings.

### Media metadata preservation

Flickr downloads automatically preserve original capture dates and embed Flickr metadata:

- **Date preservation** — Original capture dates are set on all downloaded files:
  - Video files (`.mp4`, `.mov`) get the date written into MP4/QuickTime container metadata (`mvhd`/`tkhd`/`mdhd` creation times) plus file system modification time.
  - Photo files get the file system modification time set. Photos typically already contain EXIF dates.
  - Uses Flickr's `date_taken` (original camera date) when available, falling back to `date_upload`.
- **XMP metadata embedding** — Title, description, and tags from Flickr are embedded as XMP metadata (Dublin Core namespace) into downloaded files:
  - JPEG files (`.jpg`, `.jpeg`) get an XMP APP1 segment inserted.
  - Video files (`.mp4`, `.mov`) get an XMP UUID box inserted in the MP4 container.
  - HTML in Flickr descriptions is stripped to plain text. Tags are split into individual keywords.

Google Takeout downloads similarly preserve metadata from JSON sidecar files:

- **Date preservation** — Original photo dates from the JSON sidecar's `photoTakenTime` field are set as file system modification times. For MP4/MOV files, dates are also written into container metadata. Falls back to the zip entry timestamp when the sidecar date is missing.
- **XMP metadata embedding** — Title, description, and creation date from the JSON sidecar are embedded as XMP metadata (Dublin Core namespace) into JPEG and MP4/MOV files.

- `--no-metadata` — Skip metadata embedding during downloads (Flickr and Google Takeout). The raw file is downloaded/extracted without modification.

### Debug mode

Add `--debug` to any command for verbose logging:

```bash
./photo-copy flickr download ../photos --debug
```

### Supported file types

**Uploads and S3 downloads:** JPEG, PNG, TIFF, GIF, HEIC, WebP, MP4, MOV, AVI, MKV — files with other extensions are skipped.

**Flickr and iCloud downloads:** All file types are downloaded as-is from the service, regardless of extension.

## Development

You can read [architecture details](CLAUDE.md#architecture) on the project.

### Linting & Testing

Install golangci-lint ([installation options](https://golangci-lint.run/welcome/install/)):

```bash
# macOS
brew install golangci-lint

# or cross-platform
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

Run linting and unit tests:

```bash
golangci-lint run ./...
go test ./...
```

### Integration Tests

Integration tests exercise CLI commands end-to-end against mock HTTP servers
for Flickr and Google Photos. They use a build tag and don't run with
`go test ./...`:

```bash
go test ./internal/cli/ -tags integration
```

S3 integration testing is not included — S3 operations delegate to a rclone
subprocess, and rclone's own test coverage handles that layer. S3 unit tests
cover command arg building, config generation, and binary resolution.

### Updating bundled tools


To update all bundled tool binaries:

```bash
./tools-bin/update.sh
```

To update a specific tool:

```bash
./tools-bin/update.sh rclone v1.73.2
./tools-bin/update.sh icloudpd 1.32.2
./tools-bin/update.sh osxphotos 0.75.6
```

## Acknowledgments

photo-copy relies on these excellent open-source tools for parts of upload/download.
You can check them out directly if you'd like more options for these cases:

- **[rclone](https://rclone.org/)** — Used for S3 uploads and downloads
- **[icloudpd](https://github.com/icloud-photos-downloader/icloud_photos_downloader)** — Used for iCloud Photos downloads
- **[osxphotos](https://github.com/RhetTbull/osxphotos)** — Used for iCloud Photos uploads on macOS
