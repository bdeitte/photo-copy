<img src="photocopy.png" alt="photo copy logo">

Copy and backup your photos.
Copy photos and videos between Google Photos, Flickr, AWS S3, and local directories.

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

Each command will tell you what you need to do.

```bash
./photo-copy config flickr    # Flickr API key + OAuth
./photo-copy config google    # Google OAuth credentials for upload
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

## Features

### Resumable transfers

All transfers are resumable — if a download or upload is interrupted, re-running the same command picks up where it left off:

- **Flickr downloads** — A `transfer.log` file in the output directory tracks each successfully downloaded file. Already-downloaded files are skipped on restart.
- **Google Photos uploads** — An upload log file tracks completed uploads the same way.
- **S3 uploads/downloads** — Handled by rclone, which compares source and destination and only transfers changed or missing files.

### Rate limiting & retry

- **Flickr** — Requests are throttled to 1/second (staying under Flickr's 3,600 requests/hour API limit). HTTP 429 and 5xx errors are retried up to 5 times with exponential backoff (2s, 4s, 8s, 16s, 32s), honoring the `Retry-After` header when present. This applies to both API calls and photo downloads.
- **Google Photos** — Subject to a 10,000 uploads/day limit, enforced in code.

### Debug mode

Add `--debug` to any command for verbose logging:

```bash
./photo-copy flickr download ../photos --debug
```

### Supported file types

JPEG, PNG, TIFF, GIF, HEIC, WebP, MP4, MOV, AVI, MKV

## Development

See [CLAUDE.md](CLAUDE.md) for some details on the project.

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

### Updating rclone

To update the bundled rclone binaries:

```bash
./rclone-bin/update-rclone.sh v1.68.2
```

