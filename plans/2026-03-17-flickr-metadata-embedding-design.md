# Flickr Metadata Embedding: Title, Description, and Tags

## Problem

Downloaded files from Flickr lose their title, description, and tags — metadata that only exists on Flickr's servers. If someone downloads their library as a backup, this information is lost unless it's embedded in the files themselves.

## Solution

Fetch title, description, and tags from the Flickr API during download and embed them as XMP metadata in JPEG and MP4/MOV files.

## Decisions

- **XMP only** — no EXIF or IPTC. XMP is the modern standard, works for both JPEG and video, and all major tools read it.
- **Embed directly in files** — no sidecar files. Consumer tools (Lightroom, Apple Photos, etc.) read embedded metadata automatically.
- **JPEG and MP4/MOV only** — PNG, TIFF, and other formats are out of scope for the initial implementation.
- **Two packages** — new `internal/jpegmeta` for JPEG, extend `internal/mp4meta` for video. Each package owns one file format.
- **HTML stripping** — Flickr allows HTML in descriptions; strip to plain text before embedding.
- **No extra API calls** — title, description, and tags are fetched via the existing `getPhotos` `extras` parameter.
- **XMP construction duplicated** — both `jpegmeta` and `mp4meta` build XMP packets independently (~20 lines each) rather than sharing a package.
- **No external XMP library** — the XMP packet is simple enough (just `dc:title`, `dc:description`, `dc:subject`) to construct as a raw XML string. This avoids depending on `trimmer-io/go-xmp` which is unmaintained (last commit 2021). Each package has a small `buildXMPPacket` helper that templates the XML.

## API Changes in Flickr Package

### Extras parameter

Change the `extras` value from `"date_taken,date_upload"` to `"date_taken,date_upload,description,tags"`.

Title is already returned by default in the `getPhotos` response.

### Struct changes in `photosResponse.Photo`

Add `Description` and `Tags` fields:

```go
Photo []struct {
    ID          string           `json:"id"`
    Secret      string           `json:"secret"`
    Server      string           `json:"server"`
    Title       string           `json:"title"`
    DateTaken   string           `json:"datetaken"`
    DateUpload  string           `json:"dateupload"`
    Description flickrDescription `json:"description"` // NEW
    Tags        string           `json:"tags"`          // NEW — space-separated
} `json:"photo"`
```

### Flickr description unmarshaling

The `description` field from `extras` returns as `{"_content": "the text"}`. Define a custom type:

```go
type flickrDescription struct {
    Content string `json:"_content"`
}
```

Access via `photo.Description.Content`.

## New File: `internal/flickr/metadata.go`

Contains helper functions for building metadata from API response fields:

```go
type photoMeta struct {
    Title       string
    Description string
    Tags        []string
}

func buildPhotoMeta(title, descriptionHTML, tagsStr string) photoMeta
func (m photoMeta) isEmpty() bool
func stripHTML(s string) string
```

- `buildPhotoMeta`: strips HTML from description, splits tags by space, returns struct
- `isEmpty`: returns true if all fields are empty/nil
- `stripHTML`: uses Go's `golang.org/x/net/html` tokenizer to properly extract text content from HTML, then decodes entities via `html.UnescapeString`, collapses whitespace, and trims. This handles malformed HTML and edge cases like `<3` in descriptions correctly (a regex approach would incorrectly strip content like `<3`).

## New Package: `internal/jpegmeta`

Single-purpose package for writing XMP metadata into JPEG files.

### Exported API

```go
package jpegmeta

type Metadata struct {
    Title       string
    Description string
    Tags        []string
}

func SetMetadata(filePath string, meta Metadata) error
```

### Implementation

1. Read the JPEG file
2. Build an XMP packet with `dc:title`, `dc:description`, `dc:subject` as a raw XML string via a `buildXMPPacket` helper (no external XMP library)
3. Scan JPEG marker segments to find the insertion point for the XMP `APP1` segment (after existing `APP0`/EXIF `APP1` segments), or replace an existing XMP `APP1` (identified by the `http://ns.adobe.com/xap/1.0/` namespace header)
4. Write to a temp file in the same directory, then rename over original (same safety pattern as `mp4meta.SetCreationTime`)
5. Preserve original file permissions: stat the original before rewriting, chmod the temp file to match before renaming
5. Empty fields in `Metadata` are omitted from the XMP packet

JPEG segment manipulation is done directly with `io.Reader`/`io.Writer` — no JPEG library dependency needed. No external dependencies required — XMP is constructed as raw XML.

## Extending `internal/mp4meta`

### New exported function

```go
type XMPMetadata struct {
    Title       string
    Description string
    Tags        []string
}

func SetXMPMetadata(filePath string, meta XMPMetadata) error
```

### Implementation

1. Same read-copy-rename pattern as `SetCreationTime`
2. Build an XMP packet as raw XML string via `buildXMPPacket` helper with `dc:title`, `dc:description`, `dc:subject` (no external XMP library)
3. Write into a `uuid` atom with the standard XMP UUID (`BE7ACFCB97A942E89C71999491E3AFAC`). The `uuid` box is an extended-type box: write 4-byte size + `"uuid"` type + 16-byte UUID + XMP payload bytes directly via the `io.Writer`, since `go-mp4`'s typed box API doesn't natively support UUID boxes
4. During box-by-box copy, insert the `uuid` atom after `moov` (or replace an existing XMP `uuid` atom, detected by matching the 16-byte UUID prefix)
5. Preserve original file permissions: stat the original before rewriting, chmod the temp file to match before renaming
6. Empty fields are omitted from the XMP packet

### Two-pass rewrite

For MP4/MOV files, `SetCreationTime` runs first (existing code), then `SetXMPMetadata` runs second. Both independently rewrite the file via temp-file-then-rename. The second pass copies all boxes verbatim (including the already-modified timestamp boxes from the first pass), so the first pass's changes are preserved. This two-pass approach is acceptable — merging both into one pass would complicate `SetCreationTime`'s clean API, and the I/O cost of a second pass is negligible compared to the network download.

## Integration into Flickr Download Loop

In `flickr.go` `Download()`, after the existing date-setting block, add metadata writing:

```go
// Set metadata (title, description, tags) on downloaded file.
meta := buildPhotoMeta(photo.Title, photo.Description.Content, photo.Tags)
if !meta.isEmpty() {
    filePath := filepath.Join(outputDir, filename)
    if ext == ".mp4" || ext == ".mov" {
        if err := mp4meta.SetXMPMetadata(filePath, mp4meta.XMPMetadata{
            Title: meta.Title, Description: meta.Description, Tags: meta.Tags,
        }); err != nil {
            c.log.Error("setting MP4 XMP metadata for %s: %v", filename, err)
        }
    } else if ext == ".jpg" || ext == ".jpeg" {
        if err := jpegmeta.SetMetadata(filePath, jpegmeta.Metadata{
            Title: meta.Title, Description: meta.Description, Tags: meta.Tags,
        }); err != nil {
            c.log.Error("setting JPEG metadata for %s: %v", filename, err)
        }
    }
}
```

- Metadata errors are logged but don't fail the download (same pattern as date-setting)
- Only JPEG and MP4/MOV are handled; other formats are silently skipped
- Note: `ext` at this point reflects the final resolved extension after any 404 fallback in the download loop, which is the desired behavior

## Documentation Updates

### README.md

Add a new subsection under "Features" after "Media date preservation":

```markdown
### Media metadata preservation

Flickr downloads embed original metadata into downloaded files:

- **Title** — Written as XMP `dc:title`
- **Description** — Written as XMP `dc:description` (HTML stripped to plain text)
- **Tags** — Written as XMP `dc:subject` keywords

Supported for JPEG photos and MP4/MOV videos. Other file types retain only file system timestamps (no embedded metadata).
```

### CLAUDE.md

Update the package layout to include `jpegmeta`:

```
- **jpegmeta/** - Writes XMP metadata (title, description, tags) into JPEG files. Used by Flickr downloads to preserve original Flickr metadata in photo files.
```

Update the `mp4meta` description:

```
- **mp4meta/** - Sets creation/modification timestamps and XMP metadata (title, description, tags) in MP4/MOV container metadata. Used by Flickr downloads to preserve original capture dates and Flickr metadata in video files.
```

Update the "Flickr downloads preserve original dates" bullet in "Key patterns" to mention metadata:

```
- Flickr downloads preserve original dates and metadata: `date_taken` (preferred) or `date_upload` (fallback) from the API. Title, description, and tags are embedded as XMP metadata. Video files (`.mp4`, `.mov`) get MP4 container metadata updated via the `mp4meta` package; JPEG photos get XMP written via the `jpegmeta` package; all files get file system timestamps set via `os.Chtimes`.
```

## Testing

### Unit tests for `internal/jpegmeta`

- Create a minimal valid JPEG in-memory (SOI + APP0 + minimal image data + EOI)
- Call `SetMetadata` with title/description/tags, read back and verify XMP `APP1` segment contains expected `dc:title`, `dc:description`, `dc:subject`
- Test with empty metadata fields — verify they're omitted from XMP
- Test replacing existing XMP — write metadata twice, verify second replaces first
- Test error case: non-JPEG file returns error, original file unchanged

### Unit tests for `internal/mp4meta` (new XMP function)

- Reuse existing minimal MP4 test file construction
- Call `SetXMPMetadata`, read back and verify `uuid` atom contains expected XMP
- Test with empty metadata — verify no `uuid` atom written
- Test `SetCreationTime` followed by `SetXMPMetadata` on same file — verify both creation dates and XMP are intact

### Unit tests for flickr metadata helpers

- `stripHTML`: links, `<br>`, `<b>`, `&amp;` entities, nested tags, plain text, empty string, and `<3` (heart expression that regex would incorrectly strip)
- `buildPhotoMeta`: HTML stripping of description, space-splitting of tags, empty field handling
- `flickrDescription` JSON unmarshaling: verify `{"_content": "text"}` parses correctly

### Integration tests

- Update mock server's `getPhotos` response to include `description` and `tags` fields
- After download, verify JPEG files contain XMP with expected title/description/tags
- After download, verify MP4 files contain both creation time metadata and XMP metadata

## Dependencies

New dependency: `golang.org/x/net/html` (for HTML tokenization in `stripHTML`). Run `go get golang.org/x/net` to add to `go.mod`.

No external XMP library needed — XMP packets are constructed as raw XML strings.
