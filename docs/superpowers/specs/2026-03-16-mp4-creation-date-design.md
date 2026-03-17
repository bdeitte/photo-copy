# MP4 Creation Date Metadata for Flickr Downloads

## Problem

Downloaded MP4 video files from Flickr have their metadata dates set to the download time rather than the original capture date. Photos also get current file system timestamps.

## Solution

Fetch original dates from the Flickr API during download, then:
- Set MP4 container metadata (`mvhd`/`tkhd` creation times) for video files
- Set file system modification times for all downloaded files (photos and videos)

## API Changes in Flickr Package

Add `date_taken` and `date_upload` to the `flickr.people.getPhotos` API call:
- Add `"extras": "date_taken,date_upload"` to the `signedAPIGet` params
- Add `DateTaken string` and `DateUpload string` fields to the photo struct in `photosResponse`
- Parse `DateTaken` (format: `"2024-06-15 14:30:00"`) into `time.Time`; if empty or `"0000-00-00 00:00:00"`, fall back to `DateUpload` (Unix timestamp string)
- Pass the resolved `time.Time` through the download loop so it's available after each file is written

## New Package: `internal/mp4meta/`

Single-purpose package with one exported function:

```go
func SetCreationTime(filePath string, t time.Time) error
```

Implementation details:
- Uses `github.com/abema/go-mp4` to read the file box-by-box via `ReadBoxStructure`
- Copies all boxes to a temp file in the same directory (e.g., `filename.mp4.tmp`)
- When encountering `mvhd` or `tkhd` boxes, reads the payload, sets `CreationTimeV0`/`ModificationTimeV0` (or V1 for version 1 boxes) to the target date converted to the MP4 epoch (seconds since 1904-01-01)
- All other boxes are copied unchanged via `CopyBox`
- After successful write, renames temp file over original
- On error, cleans up the temp file

Edge cases:
- If the file has no `moov` box (corrupt/truncated), returns an error but the original file is untouched
- MP4 epoch offset: `unixTime + 2082844800`

## Integration into Download Loop

In `flickr.go` `Download()`, after each successful file download:

1. If the file is an MP4 (check extension), call `mp4meta.SetCreationTime(filePath, dateTaken)`
2. For all files (photos and videos), call `os.Chtimes(filePath, dateTaken, dateTaken)` to set file system times
3. If either call fails, log a warning but don't treat it as a download failure -- the file was successfully downloaded, the metadata is a best-effort enhancement
4. The date resolution logic (prefer `date_taken`, fall back to `date_upload`) happens once per photo when parsing the API response, before entering the download/fallback loop

## Testing

### Unit tests for `internal/mp4meta/`
- Create a minimal valid MP4 file in the test (small `ftyp` + `moov` with `mvhd` and one `trak`/`tkhd`)
- Call `SetCreationTime`, then read back the file and verify `mvhd`/`tkhd` creation times are set correctly
- Test error case: non-MP4 file returns error, original file unchanged
- Test edge case: version 0 vs version 1 `mvhd` boxes (32-bit vs 64-bit timestamps)

### Unit tests for Flickr date parsing
- Test `date_taken` parsing with the Flickr format (`"2024-06-15 14:30:00"`)
- Test fallback to `date_upload` when `date_taken` is empty
- Test fallback to `date_upload` when `date_taken` is the Flickr unknown value (`"0000-00-00 00:00:00"`)

### Integration tests (existing mock server pattern)
- Update mock server's `getPhotos` response to include `datetaken`/`dateupload` fields
- After download, verify file system modification times match the expected date
- For MP4 files, verify MP4 metadata was set (read back the `mvhd` box)

## Dependencies

New dependency: `github.com/abema/go-mp4`
