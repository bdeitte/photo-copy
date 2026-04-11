## Photo Copy

Copy and backup your photos and videos.<br>
Copy between iCloud Photos, Google Photos, Flickr, AWS S3, and local directories.<br>
<a href="https://github.com/bdeitte/photo-copy/actions/workflows/ci.yml"><img src="https://github.com/bdeitte/photo-copy/actions/workflows/ci.yml/badge.svg" alt="CI"></a>

photo-copy transfers photos and videos between iCloud Photos, Google Photos, Flickr, AWS S3, and local directories. It handles the download, upload, and metadata. Use it to:
- **Back up** your photo library to S3 or a local drive
- **Move photos** between services (Flickr to Google Photos, iCloud to S3, etc.)
- **Consolidate** scattered photos from multiple services into one place

photo-copy transfers files and embedded metadata (dates, titles, descriptions, tags). It does not currently transfer albums, collections, comments, or favorites — just the photos and videos themselves.

## Quick Start

:warning: Google Photos, Flickr, and S3 support are well-tested. iCloud support is still in alpha.

Requires Go 1.25+. Build the binary first:
```bash
# Or setup.bat on Windows
./setup.sh
```

### Back up your photos

Download your library to a local directory or S3 for safekeeping.

```bash
# Download all Flickr photos to a local directory
./photo-copy config flickr
./photo-copy flickr download ~/flickr-backup

# Back up local photos to S3 Glacier Deep Archive
./photo-copy config s3
./photo-copy s3 upload ~/photos my-bucket/photos/
```

### Move photos between services

Download from one service, then upload to another. photo-copy uses a local directory as the intermediate step — there is no direct service-to-service transfer. Configure each service first (see service sections below).

```bash
# Flickr → Google Photos
./photo-copy config flickr
./photo-copy flickr download ~/flickr-photos
./photo-copy config google
./photo-copy google upload ~/flickr-photos

# Google Takeout → S3
./photo-copy google download ~/takeout-zips ~/google-photos
./photo-copy config s3
./photo-copy s3 upload ~/google-photos my-bucket/photos/
```

### Service capabilities

| Service | Download | Upload | Config required | Platform limits |
|---------|----------|--------|-----------------|-----------------|
| Flickr | Yes | Yes | Yes | — |
| Google Photos | Yes, through Takeout import | Yes | Upload only | 10,000 uploads/day |
| S3 | Yes | Yes | Yes | — |
| iCloud | Yes | macOS only | Download only | Upload requires Photos.app |

### Dates and duplicates

Dates on old photos are not always accurate across services. photo-copy reads EXIF data, MP4 container metadata, and service API dates to set the best available timestamp, but some photos — especially older ones — may still have wrong dates. Check your files after transfer with a tool like [Immich](https://immich.app/), [PhotoPrism](https://www.photoprism.app/), [digiKam](https://www.digikam.org/), or [Darktable](https://www.darktable.org/) before uploading to another service if date accuracy matters to you.

Duplicates can happen when downloading from one service and uploading to another, even with identical files. Each photo service handles deduplication differently, and their behavior changes over time. Use `--date-range` to transfer in smaller batches and reduce the chance of duplicates building up across repeated runs.

All credentials are saved to `~/.config/photo-copy/` (override with `PHOTO_COPY_CONFIG_DIR`).

## Flickr

Configure credentials:

```bash
./photo-copy config flickr    # Flickr API key + OAuth
```

Download and upload:

```bash
# Download all photos
./photo-copy flickr download ~/flickr-photos

# Download only photos taken in 2023
./photo-copy flickr download --date-range 2023-01-01:2023-12-31 ~/flickr-photos

# Download the first 100 photos
./photo-copy flickr download --limit 100 ~/flickr-photos

# Upload local photos
./photo-copy flickr upload ~/photos

# Upload only photos from 2020 onward, limit to 500
./photo-copy flickr upload --date-range 2020-01-01: --limit 500 ~/photos
```

## Google Photos

The Google Photos API only allows access to photos the app itself uploaded. Downloading your full library requires Google Takeout — a manual zip export from Google.

Configure credentials (needed for upload only, not Takeout download):

```bash
./photo-copy config google
```

Upload:

```bash
# Upload local photos
./photo-copy google upload ~/photos

# Upload only photos taken before 2024, limit to 1000
./photo-copy google upload --date-range :2023-12-31 --limit 1000 ~/photos
```

Download via Google Takeout:

```bash
# Export your library via Google Takeout, then point at the zip directory
./photo-copy google download ~/takeout-zips ~/google-photos
```

**Google Takeout details:**
- **Album preservation** — Files in album folders are extracted into subdirectories (e.g., `Trip to Paris/photo.jpg`). Files in year folders (`Photos from 2022`) go to the output root.
- **Deduplication** — Photos in both an album folder and a year folder are extracted once (album copy kept). Photos in multiple albums are kept in all.
- **Metadata embedding** — Title, description, and creation date from JSON sidecar files are embedded as XMP metadata into JPEG and MP4/MOV files. File system timestamps are set from the photo's taken date.
- **`--no-metadata`** — Leave extracted files unmodified: skip XMP embedding, container timestamp updates, and filesystem timestamp restoration.

## S3

Uploads default to Glacier Deep Archive — inexpensive long-term storage, but restoring files takes hours. Use `--storage-class` to change (e.g., `STANDARD`, `GLACIER`).

Configure credentials:

```bash
./photo-copy config s3
```

Upload and download:

```bash
# Upload to S3
./photo-copy s3 upload ~/photos my-bucket/photos/

# Upload using full URL
./photo-copy s3 upload ~/photos https://my-bucket.s3.us-west-2.amazonaws.com/photos/

# Upload with standard storage class
./photo-copy s3 upload ~/photos my-bucket/photos/ --storage-class STANDARD

# Upload only files modified in 2023, limit to 200
./photo-copy s3 upload --date-range 2023-01-01:2023-12-31 --limit 200 ~/photos my-bucket/photos/

# Download from S3
./photo-copy s3 download my-bucket/photos/ ~/photos

# Download using full S3 URL
./photo-copy s3 download https://my-bucket.s3.us-west-2.amazonaws.com/photos/ ~/photos

# Download only files modified since 2024
./photo-copy s3 download --date-range 2024-01-01: my-bucket/photos/ ~/photos
```

**Glacier/Deep Archive downloads:** Files in Glacier or Deep Archive require two steps. The first run initiates a Bulk restore (5-12 hours). Re-run the same command after restore completes to download. Files already restored or in Standard storage download immediately. Already-downloaded files are skipped.

```bash
# First run initiates restore
./photo-copy s3 download my-bucket/photos/ ~/photos

# Re-run after a few hours to download restored files
./photo-copy s3 download my-bucket/photos/ ~/photos
```

## iCloud Photos

Download works on all platforms. Upload requires macOS with Photos.app and iCloud Photos sync enabled.

Configure credentials (needed for download only):

```bash
./photo-copy config icloud
```

Download and upload:

```bash
# Download all photos
./photo-copy icloud download ~/icloud-photos

# Download the 50 most recently uploaded photos
./photo-copy icloud download --limit 50 ~/icloud-photos

# Upload local photos (macOS only — imports into Photos.app)
./photo-copy icloud upload ~/photos

# Upload only photos from 2022-2023
./photo-copy icloud upload --date-range 2022-01-01:2023-12-31 ~/photos
```

**iCloud download notes:**
- Requires Apple ID with 2FA. Run `photo-copy config icloud` to authenticate.
- Session cookies expire approximately every 2 months — re-run config to re-authenticate.
- Advanced Data Protection must be disabled.
- icloudpd bundled for Linux amd64/arm64, macOS amd64 (Apple Silicon runs via Rosetta 2), and Windows amd64. Other platforms: `pipx install icloudpd`.

**iCloud upload notes:**
- No config needed — imports directly into Photos.app via osxphotos. If iCloud Photos sync is enabled, files upload to iCloud automatically.
- `--no-metadata` has no effect on iCloud commands.
- osxphotos bundled for macOS ARM64 only. Intel Macs: `pipx install osxphotos`.

## Features

### Resumable transfers

All transfers are resumable — re-running the same command picks up where it left off:

- **Flickr downloads** — A `transfer.log` in the output directory tracks downloaded files. Already-downloaded files are skipped.
- **Google Photos uploads** — An upload log tracks completed uploads. Files in subdirectories are tracked by relative path.
- **S3** — Handled by rclone, which compares source and destination and only transfers changed or missing files.
- **iCloud downloads** — Handled by icloudpd, which skips files already in the output directory by filename.
- **iCloud uploads** — Each run imports all files; Photos.app deduplicates automatically.

Failed files are not marked as completed, so re-running retries them automatically.

**Duplicates in resumable transfers:** Resumable transfers skip files completed in the current run. Flickr and Google Photos uploads do not check whether a file already exists in the service — re-uploading the same files creates duplicates. S3 avoids this via rclone's file comparison. iCloud uploads rely on Photos.app deduplication. Google Takeout import renames on filename collision (e.g., `photo_1.jpg`).

### Rate limiting and retry

- **Flickr** — Requests throttled to stay under 3,600/hour. The interval adapts: doubles on HTTP 429, gradually decreases on success. 429 responses retry indefinitely with exponential backoff capped at 5 minutes. 5xx errors retry up to 7 times. Both honor `Retry-After` headers.
- **Flickr uploads** — Continue past individual failures. 10 consecutive failures abort the transfer.
- **Google Photos** — 10,000 uploads/day limit. Transfers are capped at 10,000 with a log message — re-run the next day.

### Filtering options

- `--date-range YYYY-MM-DD:YYYY-MM-DD` — Only process files in the date range. Either side can be omitted (e.g., `2020-01-01:` for 2020 onward).
- `--limit N` — Only process the first N files. For iCloud downloads, this maps to icloudpd's `--recent` flag (N most recently uploaded).

**Date sources by command:**
- **Flickr download**: `date_taken` (preferred) or `date_upload` from the API.
- **Flickr/Google/iCloud upload**: EXIF DateTimeOriginal (JPEG) or MP4 creation time, falling back to file modification time.
- **S3**: rclone `--min-age`/`--max-age` — filters by file modification time, not embedded metadata.
- **iCloud download**: icloudpd's date filtering (photo creation date from iCloud). `--limit` selects the N most recently uploaded.

### Media metadata preservation

Flickr and Google Takeout downloads preserve original dates and embed metadata.

**Flickr downloads:**
- Video files get dates written into MP4/QuickTime container metadata plus file system modification time. Photo files get file system modification time set. Uses `date_taken` when available, falls back to `date_upload`.
- Title, description, and tags embedded as XMP (Dublin Core) into JPEG and MP4/MOV files. HTML in descriptions is stripped to plain text.

**Google Takeout downloads:**
- Dates from JSON sidecar `photoTakenTime` set as file system modification times. MP4/MOV files also get container metadata updated. Falls back to zip entry timestamp.
- Title and description from JSON sidecars embedded as XMP into JPEG and MP4/MOV files.

`--no-metadata` leaves downloaded files unmodified during Flickr and Google Takeout downloads — skips XMP embedding, container timestamp updates, and filesystem timestamp restoration.

### Subdirectory support

All upload commands recursively scan subdirectories. S3 preserves the subdirectory structure in the upload path. Google Takeout downloads preserve album folder structure as subdirectories.

### Transfer summary and validation

Every transfer prints a summary and writes a report file: file counts (succeeded, skipped, failed), total size, elapsed time, and any errors.

Validation checks after transfer:
- **Count verification** — Expected vs actual file counts.
- **Zero-size detection** — Scans for empty files after downloads.
- **Log consistency** (Flickr) — Verifies transfer.log entries have corresponding files on disk.

Report files are written to the transfer directory as `photo-copy-report-{service}-{operation}-{timestamp}.txt`.

### Debug mode

Add `--debug` to any command for verbose logging:

```bash
./photo-copy flickr download ~/photos --debug
```

## Organizing your photos

photo-copy handles transfers. For browsing, organizing, and verifying dates on your local files, try out these tools:

- **[PhotoPrism](https://www.photoprism.app/)** — Self-hosted photo management
- **[Immich](https://immich.app/)** — Self-hosted photo management
- **[digiKam](https://www.digikam.org/)** — Desktop photo management
- **[Darktable](https://www.darktable.org/)** — Photo workflow and raw processing

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, linting, testing, and integration test instructions.

## Acknowledgments

photo-copy relies on these open-source tools for parts of upload and download:

- **[rclone](https://rclone.org/)** — S3 uploads and downloads
- **[icloudpd](https://github.com/icloud-photos-downloader/icloud_photos_downloader)** — iCloud Photos downloads
- **[osxphotos](https://github.com/RhetTbull/osxphotos)** — iCloud Photos uploads on macOS

