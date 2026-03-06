# photo-copy

Copy photos and videos between Flickr, Google Photos, S3, and local directories.

## Setup

```bash
./setup.sh
```

Requires Go 1.21+ and (optionally) rclone for S3 operations.

## Usage

### Configure credentials

```bash
./photo-copy config flickr    # Flickr API key + OAuth
./photo-copy config google    # Google OAuth credentials
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

### S3 (via rclone)

```bash
# Copy to S3
rclone copy ./photos s3remote:my-bucket/photos/ --progress

# Copy from S3
rclone copy s3remote:my-bucket/photos/ ./photos/ --progress
```

### Debug mode

Add `--debug` to any command for verbose logging:

```bash
./photo-copy flickr download --output-dir ./photos --debug
```

## Supported file types

JPEG, PNG, TIFF, GIF, HEIC, WebP, MP4, MOV, AVI, MKV
