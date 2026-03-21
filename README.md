<p align="center">
  <img src="photocopy.png" alt="photo copy logo"><br><br>
  <b>Copy and backup your photos and videos.</b><br>
  <b>Copy between iCloud Photos, Google Photos, Flickr, AWS S3, and local directories.</b><br>
  <a href="https://github.com/bdeitte/photo-copy/actions/workflows/ci.yml"><img src="https://github.com/bdeitte/photo-copy/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
</p>

## Setup

> **Note:** This project is in active development. Google Photos and Flickr support has been well-tested and is in good shape. S3 and iCloud support is still in an alpha state.

Requires Go 1.21+

```bash
# macOS / Linux
./setup.sh

# Windows
setup.bat
```

## Usage

### Configure credentials

Each command will tell you what you need to do. Credentials are saved to `~/.config/photo-copy/`.

```bash
./photo-copy config flickr    # Flickr API key + OAuth
./photo-copy config google    # Google OAuth credentials for upload
./photo-copy config s3        # AWS credentials for S3
./photo-copy config icloud   # iCloud download authentication (not needed for upload)
```

### Flickr

```bash
# Download all photos from Flickr
./photo-copy flickr download ../flickr-photos

# Upload local photos to Flickr
./photo-copy flickr upload ../photos
```

### Google Photos

The Google Photos API only allows access to photos the app itself uploaded, so downloading your full library requires Google Takeout (a manual zip export from Google).

```bash
# Upload local photos to Google Photos (requires 'config google' setup)
./photo-copy google upload ../photos

# Download: export your library via Google Takeout, then extract the zips
./photo-copy google import-takeout ../takeout-zips ../google-photos
```

### S3

```bash
# Upload local photos to S3
./photo-copy s3 upload ../photos --bucket my-bucket --prefix photos/

# Download photos from S3
./photo-copy s3 download ../photos --bucket my-bucket --prefix photos/
```

### iCloud Photos

Download works on all platforms. Upload requires macOS with Photos.app and iCloud Photos sync enabled.

icloudpd and osxphotos are bundled with photo-copy for most platforms. No separate installation is needed on supported platforms.

- **icloudpd** (for downloads): bundled for Linux amd64/arm64, macOS amd64 (runs via Rosetta on Apple Silicon), and Windows amd64. Other platforms: `pipx install icloudpd`.
- **osxphotos** (for uploads): bundled for macOS ARM64 only. Intel Macs: `pipx install osxphotos`.

```bash
# Download all photos from iCloud
./photo-copy icloud download ../icloud-photos

# Upload local photos to iCloud (macOS only — imports into Photos.app)
./photo-copy icloud upload ../photos
```

**Notes:**
- Download requires Apple ID with 2FA. Run `photo-copy config icloud` to authenticate.
- Session cookies expire approximately every 2 months — re-run `config icloud` to re-authenticate.
- Advanced Data Protection must be disabled for downloads.
- Upload does not require `config icloud` — it imports files directly into Photos.app via osxphotos. If iCloud Photos sync is enabled in System Settings, they automatically upload to iCloud.
- `--no-metadata` has no effect on iCloud commands.

## Features

### Resumable transfers

All transfers are resumable — if a download or upload is interrupted, re-running the same command picks up where it left off:

- **Flickr downloads** — A `transfer.log` file in the output directory tracks each successfully downloaded file. Already-downloaded files are skipped on restart.
- **Google Photos uploads** — An upload log file tracks completed uploads the same way.
- **S3 uploads/downloads** — Handled by rclone, which compares source and destination and only transfers changed or missing files.
- **iCloud downloads** — Handled by icloudpd, which skips files that already exist in the output directory by filename matching.
- **iCloud uploads** — Each run imports all files; Photos.app deduplicates automatically.

Files that fail during transfer are not marked as completed in the log, so re-running the same command will automatically retry them while skipping files that already succeeded.

### Rate limiting & retry

- **Flickr** — Requests are throttled to 1/second (staying under Flickr's 3,600 requests/hour API limit). HTTP 429 and 5xx errors are retried up to 5 times with exponential backoff (2s, 4s, 8s, 16s, 32s), honoring the `Retry-After` header when present. This applies to both API calls and photo downloads.
- **Google Photos** — Subject to a 10,000 uploads/day limit, enforced in code.

### Filtering Options

- `--date-range YYYY-MM-DD:YYYY-MM-DD` — Only process files within the specified date range. Either side can be omitted for open-ended ranges (e.g., `2020-01-01:` for everything from 2020 onward, `:2023-12-31` for everything up to end of 2023).

**Date sources by command:**
- **Flickr download**: Uses `date_taken` (preferred) or `date_upload` from the Flickr API.
- **Flickr/Google upload**: Reads EXIF DateTimeOriginal (JPEG) or MP4 creation time, falling back to file modification time.
- **S3 upload/download**: Uses rclone's `--min-age`/`--max-age` flags, which filter by **file modification time** (or S3 object LastModified timestamp), not embedded metadata dates. This is a limitation of delegating to rclone.
- **iCloud download**: Uses icloudpd's date filtering (photo creation date from iCloud). Note: `--limit` maps to icloudpd's `--recent` flag, which selects the N most recently uploaded photos.
- **iCloud upload**: Reads EXIF DateTimeOriginal (JPEG) or MP4 creation time, falling back to file modification time (same as Flickr/Google upload).

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

- `--no-metadata` — Skip metadata embedding during Flickr downloads (XMP metadata, MP4 creation time, filesystem timestamps). The raw file is downloaded without modification.

### Debug mode

Add `--debug` to any command for verbose logging:

```bash
./photo-copy flickr download ../photos --debug
```

### Supported file types

JPEG, PNG, TIFF, GIF, HEIC, WebP, MP4, MOV, AVI, MKV

## Development

See [CLAUDE.md](CLAUDE.md#architecture) for some details on the project.

### Linting & Testing

Install golangci-lint:

```bash
brew install golangci-lint
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

photo-copy relies on these excellent open-source tools:

- **[rclone](https://rclone.org/)** — Used for S3 uploads and downloads
- **[icloudpd](https://github.com/icloud-photos-downloader/icloud_photos_downloader)** — Used for iCloud Photos downloads
- **[osxphotos](https://github.com/RhetTbull/osxphotos)** — Used for iCloud Photos uploads on macOS
