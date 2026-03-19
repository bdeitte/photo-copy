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

The `Before` date is inclusive of the full day — implemented as `< start of next day` (i.e., `time.Date(y, m, d+1, 0, 0, 0, 0, loc)`) to avoid missing sub-second timestamps. Validation errors (bad format, start after end) are reported at CLI level before invoking any service.

### Timezone handling

All date parsing and comparison uses **local time**. The `--date-range` flag values are parsed as local time. Dates from different sources (Flickr API, EXIF, MP4) are compared as-is without timezone conversion. This is documented in CLI help text — the intent is "date as the photographer sees it," which aligns with how `date_taken` and EXIF `DateTimeOriginal` work (local time, no timezone). MP4 creation times are stored as UTC but are compared without conversion for simplicity.

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

### Interaction with --limit

`--limit` counts only **transferred** files, not examined files. Date-filtered files that are skipped do not count toward the limit. For example, `--limit 10 --date-range 2020-01-01:` processes photos until 10 in-range files are transferred, regardless of how many out-of-range files were skipped.

This is consistent across all services, including S3 where rclone applies date filtering (`--min-age`/`--max-age`) before the `--files-from` limit.

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

For **S3 uploads and downloads**, pass rclone's `--max-age` and `--min-age` flags to the rclone subprocess. The mapping is:
- `After` date → `--max-age YYYY-MM-DD` (files newer than this date)
- `Before` date → `--min-age YYYY-MM-DD` (files older than this date)

Note the inverted naming: rclone's `--max-age` means "maximum age" (i.e., files modified *after* the date), which maps to our `After` bound. These flags accept absolute dates in ISO8601 format.

For S3 objects, rclone filters by the object's **LastModified** timestamp (the time the object was uploaded/copied to S3), which may differ from the original file's modification time or metadata date.

**Important caveat**: S3 date filtering uses rclone's age-based filtering, not embedded metadata dates. This differs from Flickr/Google where embedded metadata dates (EXIF, MP4 creation time) are used. This must be clearly documented in CLI help text, README, and CLAUDE.md.

**Interaction with `--files-from`**: The existing S3 code uses `--files-from` when `--limit` is set, which disables most rclone filters. When both `--date-range` and `--limit` are used with S3, apply date filtering first by listing files with `--min-age`/`--max-age`, then truncate to the limit for `--files-from`.

### No-op warnings

`--date-range` on `google import-takeout` logs a warning because import-takeout processes zip archives where per-file date filtering would be complex and of low value:

```
--date-range has no effect on import-takeout
```

Both `--no-metadata` and `--date-range` on `config` subcommands (`config flickr`, `config google`, `config s3`) log warnings since these commands only manage credentials.

### Date reading from files

For reading metadata dates from local files during uploads:

- **JPEG**: Use an external library (`rwcarlsen/goexif` or similar) for reading EXIF `DateTimeOriginal`. The current `jpegmeta` package writes XMP APP1 segments, which is structurally different from reading EXIF (TIFF/IFD format in a separate APP1 segment). Add a `ReadDate(filePath string) (time.Time, error)` function in `jpegmeta` that wraps the external library.
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
