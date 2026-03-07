<div align="center">
  <img src="photocopy.png" alt="photo copy logo"></h1>
</div>

# photo copy

Copy photos and videos between Google Photos, Flickr, AWS S3, and local directories.<p>

**DANGER: THIS REPO IS ACTIVELY BEING CLAUDE CODE DEVELOPED. USE WITH CAUTION UNTIL IT IS FURTHER ALONG**

## Setup

Requires Go 1.21+

```bash
# macOS / Linux
./setup.sh

# Windows
setup.bat
```

## Usage

### Configure credentials

```bash
./photo-copy config flickr    # Flickr API key + OAuth
./photo-copy config google    # Google OAuth credentials
./photo-copy config s3        # AWS credentials for S3
```

### Flickr

```bash
# Download all photos from Flickr
./photo-copy flickr download ../flickr-photos

# Upload local photos to Flickr
./photo-copy flickr upload ../photos
```

### Google Photos

```bash
# Upload local photos to Google Photos
./photo-copy google-photos upload ../photos

# Extract media from Google Takeout zips
./photo-copy google-photos import-takeout ../takeout-zips ../google-photos
```

### S3

```bash
# Upload local photos to S3
./photo-copy s3 upload ../photos --bucket my-bucket --prefix photos/

# Download photos from S3
./photo-copy s3 download ../photos --bucket my-bucket --prefix photos/
```

## Resumable transfers

All transfers are resumable — if a download or upload is interrupted, re-running the same command picks up where it left off:

- **Flickr downloads** — A `transfer.log` file in the output directory tracks each successfully downloaded file. Already-downloaded files are skipped on restart.
- **Google Photos uploads** — An upload log file tracks completed uploads the same way.
- **S3 uploads/downloads** — Handled by rclone, which compares source and destination and only transfers changed or missing files.

## Rate limiting & retry

- **Flickr** — Requests are throttled to 1/second (staying under Flickr's 3,600 requests/hour API limit). HTTP 429 and 5xx errors are retried up to 5 times with exponential backoff (2s, 4s, 8s, 16s, 32s), honoring the `Retry-After` header when present. This applies to both API calls and photo downloads.
- **Google Photos** — Subject to a 10,000 uploads/day limit, enforced in code.

### Debug mode

Add `--debug` to any command for verbose logging:

```bash
./photo-copy flickr download ../photos --debug
```

## Development

See [CLAUDE.md](CLAUDE.md) for some details on the project.

### Updating rclone

To update the bundled rclone binaries:

```bash
./scripts/update-rclone.sh v1.68.2
```

## Supported file types

JPEG, PNG, TIFF, GIF, HEIC, WebP, MP4, MOV, AVI, MKV
