# Design: --no-metadata and --date-range Options

## Overview

Two new root-level CLI flags:

1. **`--no-metadata`** — Skip all metadata embedding operations during Flickr downloads (XMP, MP4 creation time, filesystem timestamps).
2. **`--date-range START:END`** — Filter uploads/downloads to only process files within a date range.

Both flags are root-level (like `--debug` and `--limit`). When a flag has no effect on the current command, a clear warning is logged.

## Feature 1: --no-metadata

### Scope

Currently, metadata embedding only occurs during **Flickr downloads**. The following operations are skipped when `--no-metadata` is set:

- JPEG XMP metadata writing (`jpegmeta.SetMetadata`) — title, description, tags
- MP4/MOV creation time setting (`mp4meta.SetCreationTime`)
- MP4/MOV XMP metadata writing (`mp4meta.SetXMPMetadata`) — title, description, tags
- Filesystem timestamp setting (`os.Chtimes`)

The file is downloaded but left completely untouched.

### No-op warnings

When `--no-metadata` is used with any command other than `flickr download`, log a warning:

```
--no-metadata has no effect on [command]; metadata embedding only occurs during Flickr downloads
```

## Feature 2: --date-range

### Flag format

```
--date-range YYYY-MM-DD:YYYY-MM-DD
```

Either side is optional for open-ended ranges:
- `--date-range 2020-01-01:2023-12-31` — both bounds
- `--date-range 2020-01-01:` — everything from 2020 onward
- `--date-range :2023-12-31` — everything up to end of 2023

The `Before` date is inclusive of the full day (set to 23:59:59). Validation errors (bad format, start after end) are reported at CLI level before invoking any service.

### Date range struct

New package `internal/daterange/daterange.go`:

```go
type DateRange struct {
    After  *time.Time // nil = no lower bound
    Before *time.Time // nil = no upper bound
}

func Parse(s string) (*DateRange, error)  // parses "YYYY-MM-DD:YYYY-MM-DD"
func (dr *DateRange) Contains(t time.Time) bool
```

Shared across all commands.

### Flickr download

Uses the existing `resolvePhotoDate()` logic (`date_taken` preferred, `date_upload` fallback) from the Flickr API response. After resolving the date, check against the range. If outside, skip the photo (increment `result.Skipped`, log at info level with the photo's date and title).

Photos with no resolvable date (both fields empty/unparseable) are **included** — we don't silently drop files with missing dates.

### Uploads (Flickr, Google, S3)

For Flickr and Google uploads, before uploading each file, read the file's embedded metadata date:

1. **JPEG files**: Read EXIF `DateTimeOriginal`.
2. **MP4/MOV files**: Read creation time from the `mvhd` box.
3. **Other supported formats**: Fall back to file modification time (`os.Stat().ModTime()`).
4. **Fallback**: If metadata date reading fails or returns zero, use file modification time.

Check the resolved date against the range. If outside, skip (increment `result.Skipped`, log with filename and date).

For **S3 uploads and downloads**, pass `--min-age` and/or `--max-age` flags to the rclone subprocess. These filter by **file modification time**, not embedded metadata date.

**Important caveat**: S3 date filtering uses file modification time because rclone handles file iteration directly. This differs from Flickr/Google where embedded metadata dates are used. This must be clearly documented in CLI help text, README, and CLAUDE.md.

### No-op warnings

`--date-range` on `google import-takeout` logs:

```
--date-range has no effect on import-takeout
```

### Date reading from files

For reading metadata dates from local files during uploads:

- **JPEG**: If extending `jpegmeta` to read EXIF `DateTimeOriginal` is straightforward, add `ReadDate(filePath string) (time.Time, error)` there. Otherwise, use an external library like `rwcarlsen/goexif`.
- **MP4/MOV**: Add `ReadCreationTime(filePath string) (time.Time, error)` to `mp4meta` using the existing `abema/go-mp4` dependency to read the `mvhd` box.

Both return `time.Time` and `error`. Zero time means no date found, triggering fallback to file modification time.

## Method signature changes

```go
// Flickr
func (c *Client) Download(ctx context.Context, outputDir string, limit int, noMetadata bool, dateRange *daterange.DateRange) (*transfer.Result, error)
func (c *Client) Upload(ctx context.Context, inputDir string, limit int, dateRange *daterange.DateRange) (*transfer.Result, error)

// Google
func (c *Client) Upload(ctx context.Context, inputDir string, limit int, dateRange *daterange.DateRange) (*transfer.Result, error)

// S3
func (c *Client) Upload(ctx context.Context, inputDir, bucket, prefix string, mediaOnly bool, limit int, dateRange *daterange.DateRange) (*transfer.Result, error)
func (c *Client) Download(ctx context.Context, bucket, prefix, outputDir string, mediaOnly bool, limit int, dateRange *daterange.DateRange) (*transfer.Result, error)
```

`noMetadata` only goes to Flickr `Download` since that's the only place metadata is embedded.

## Testing

- **Unit tests for `daterange.Parse`**: Valid ranges, open-ended, bad format, start after end.
- **Unit tests for date reading**: JPEG EXIF and MP4 creation time reading with test fixtures.
- **Integration tests**: Update existing mock server tests to verify:
  - `--no-metadata` skips metadata embedding (check files aren't modified after download)
  - `--date-range` filters photos correctly (mock API returns photos with various dates, verify only in-range ones are downloaded)
  - Upload date filtering with test files that have known EXIF/MP4 dates
- **No-op warning tests**: Verify warning messages when flags don't apply.

## Documentation

- Update **README.md** with new flags, including clear callout that S3 `--date-range` uses file modification time (not embedded metadata date) because it delegates to rclone.
- Update **CLAUDE.md** to document the new flags, the `daterange` package, date reading capabilities in `jpegmeta`/`mp4meta`, and updated method signatures.
- CLI `--help` text for both flags, with the S3 caveat in the `--date-range` description.
