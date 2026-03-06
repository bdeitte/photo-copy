# photo-copy

Copy photos and videos between Flickr, Google Photos, S3, and local directories.

## Setup

```bash
./setup.sh
```

Requires Go 1.21+. Rclone binaries for S3 support are bundled in the repo.

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
./photo-copy flickr download --output-dir ./flickr-photos

# Upload local photos to Flickr
./photo-copy flickr upload --input-dir ./photos
```

### Google Photos

```bash
# Upload local photos to Google Photos
./photo-copy google-photos upload --input-dir ./photos

# Extract media from Google Takeout zips
./photo-copy google-photos import-takeout --takeout-dir ./takeout-zips --output-dir ./google-photos
```

### S3

```bash
# Upload local photos to S3
./photo-copy s3 upload --input-dir ./photos --bucket my-bucket --prefix photos/

# Download photos from S3
./photo-copy s3 download --bucket my-bucket --prefix photos/ --output-dir ./photos
```

### Debug mode

Add `--debug` to any command for verbose logging:

```bash
./photo-copy flickr download --output-dir ./photos --debug
```

### Updating rclone

To update the bundled rclone binaries:

```bash
./scripts/update-rclone.sh v1.68.2
```

## Supported file types

JPEG, PNG, TIFF, GIF, HEIC, WebP, MP4, MOV, AVI, MKV
