# Flickr Metadata Embedding Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Embed Flickr title, description, and tags as XMP metadata into downloaded JPEG and MP4/MOV files.

**Architecture:** Two metadata packages (`internal/jpegmeta` for JPEG XMP, extended `internal/mp4meta` for MP4/MOV XMP) write XMP constructed as raw XML strings. The Flickr download loop fetches metadata via existing `extras` param, strips HTML from descriptions, and calls the appropriate package per file type.

**Tech Stack:** Go standard library, `golang.org/x/net/html` (HTML tokenizer), `github.com/abema/go-mp4` (existing dependency)

---

## Chunk 1: Flickr Metadata Helpers and HTML Stripping

### Task 1: Add `golang.org/x/net` dependency

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Add the dependency**

Run: `go get golang.org/x/net`

- [ ] **Step 2: Verify it was added**

Run: `grep "golang.org/x/net" go.mod`
Expected: a line like `golang.org/x/net v0.x.x`

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "Add golang.org/x/net dependency for HTML tokenization"
```

---

### Task 2: Create `internal/flickr/metadata.go` with `stripHTML`

**Files:**
- Create: `internal/flickr/metadata.go`
- Create: `internal/flickr/metadata_test.go`

- [ ] **Step 1: Write the failing tests for `stripHTML`**

Create `internal/flickr/metadata_test.go`:

```go
package flickr

import "testing"

func TestStripHTML(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", ""},
		{"plain text", "hello world", "hello world"},
		{"simple tag", "<b>bold</b>", "bold"},
		{"link tag", `<a href="http://example.com">click here</a>`, "click here"},
		{"br tags", "line1<br>line2<br/>line3", "line1 line2 line3"},
		{"nested tags", "<div><p>nested <b>bold</b></p></div>", "nested bold"},
		{"html entities", "rock &amp; roll", "rock & roll"},
		{"numeric entity", "&#39;quoted&#39;", "'quoted'"},
		{"heart expression", "I <3 this", "I <3 this"},
		{"multiple spaces", "hello   world", "hello world"},
		{"newlines and tabs", "hello\n\t  world", "hello world"},
		{"leading trailing space", "  hello  ", "hello"},
		{"mixed html and entities", "<p>Tom &amp; Jerry</p>", "Tom & Jerry"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripHTML(tt.input)
			if got != tt.want {
				t.Errorf("stripHTML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/flickr/ -run TestStripHTML -v`
Expected: FAIL — `stripHTML` not defined

- [ ] **Step 3: Implement `stripHTML`**

Create `internal/flickr/metadata.go`:

```go
package flickr

import (
	"html"
	"io"
	"regexp"
	"strings"

	gohtml "golang.org/x/net/html"
)

// stripHTML removes HTML tags from s, decodes HTML entities, and collapses whitespace.
// Uses the golang.org/x/net/html tokenizer to correctly handle malformed HTML
// and edge cases like "<3" in descriptions.
func stripHTML(s string) string {
	if s == "" {
		return ""
	}

	var b strings.Builder
	z := gohtml.NewTokenizer(strings.NewReader(s))
	for {
		tt := z.Next()
		switch tt {
		case gohtml.ErrorToken:
			if z.Err() == io.EOF {
				return collapseWhitespace(html.UnescapeString(b.String()))
			}
			// On parse error, fall back to the raw string
			return collapseWhitespace(html.UnescapeString(s))
		case gohtml.TextToken:
			b.Write(z.Text())
		}
	}
}

var multiSpace = regexp.MustCompile(`\s+`)

func collapseWhitespace(s string) string {
	return strings.TrimSpace(multiSpace.ReplaceAllString(s, " "))
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/flickr/ -run TestStripHTML -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/flickr/metadata.go internal/flickr/metadata_test.go
git commit -m "Add stripHTML for cleaning Flickr description HTML"
```

---

### Task 3: Add `flickrDescription`, `photoMeta`, `buildPhotoMeta`, and `isEmpty`

**Files:**
- Modify: `internal/flickr/metadata.go`
- Modify: `internal/flickr/metadata_test.go`

- [ ] **Step 1: Write failing tests for `buildPhotoMeta` and `isEmpty`**

Add to `internal/flickr/metadata_test.go`:

```go
import "encoding/json"

func TestBuildPhotoMeta(t *testing.T) {
	tests := []struct {
		name        string
		title       string
		descHTML    string
		tagsStr     string
		wantTitle   string
		wantDesc    string
		wantTags    []string
		wantEmpty   bool
	}{
		{
			name: "all fields",
			title: "My Photo", descHTML: "<b>A great photo</b>", tagsStr: "sunset beach",
			wantTitle: "My Photo", wantDesc: "A great photo", wantTags: []string{"sunset", "beach"},
		},
		{
			name: "empty fields",
			title: "", descHTML: "", tagsStr: "",
			wantEmpty: true,
		},
		{
			name: "title only",
			title: "Just Title", descHTML: "", tagsStr: "",
			wantTitle: "Just Title", wantTags: nil,
		},
		{
			name: "tags with extra spaces",
			title: "", descHTML: "", tagsStr: "  one   two  three  ",
			wantTags: []string{"one", "two", "three"},
		},
		{
			name: "description with html",
			title: "", descHTML: `<a href="http://example.com">link</a> and <b>bold</b>`, tagsStr: "",
			wantDesc: "link and bold",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPhotoMeta(tt.title, tt.descHTML, tt.tagsStr)
			if got.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", got.Title, tt.wantTitle)
			}
			if got.Description != tt.wantDesc {
				t.Errorf("Description = %q, want %q", got.Description, tt.wantDesc)
			}
			if len(got.Tags) != len(tt.wantTags) {
				t.Errorf("Tags = %v, want %v", got.Tags, tt.wantTags)
			} else {
				for i, tag := range got.Tags {
					if tag != tt.wantTags[i] {
						t.Errorf("Tags[%d] = %q, want %q", i, tag, tt.wantTags[i])
					}
				}
			}
			if tt.wantEmpty && !got.isEmpty() {
				t.Error("expected isEmpty() to return true")
			}
			if !tt.wantEmpty && got.isEmpty() {
				t.Error("expected isEmpty() to return false")
			}
		})
	}
}

func TestFlickrDescription_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		want    string
	}{
		{"object with _content", `{"_content": "hello world"}`, "hello world"},
		{"empty content", `{"_content": ""}`, ""},
		{"html content", `{"_content": "<b>bold</b>"}`, "<b>bold</b>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fd flickrDescription
			if err := json.Unmarshal([]byte(tt.json), &fd); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}
			if fd.Content != tt.want {
				t.Errorf("Content = %q, want %q", fd.Content, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/flickr/ -run "TestBuildPhotoMeta|TestFlickrDescription" -v`
Expected: FAIL — types not defined

- [ ] **Step 3: Implement the types and functions**

Add to `internal/flickr/metadata.go`:

```go
// flickrDescription handles the Flickr API's description format: {"_content": "text"}.
type flickrDescription struct {
	Content string `json:"_content"`
}

// photoMeta holds cleaned metadata extracted from the Flickr API response.
type photoMeta struct {
	Title       string
	Description string
	Tags        []string
}

// buildPhotoMeta creates a photoMeta from raw Flickr API fields.
// It strips HTML from the description and splits the space-separated tags string.
func buildPhotoMeta(title, descriptionHTML, tagsStr string) photoMeta {
	m := photoMeta{
		Title:       title,
		Description: stripHTML(descriptionHTML),
	}
	if tagsStr = strings.TrimSpace(tagsStr); tagsStr != "" {
		for _, tag := range strings.Fields(tagsStr) {
			m.Tags = append(m.Tags, tag)
		}
	}
	return m
}

// isEmpty returns true if all metadata fields are empty.
func (m photoMeta) isEmpty() bool {
	return m.Title == "" && m.Description == "" && len(m.Tags) == 0
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/flickr/ -run "TestBuildPhotoMeta|TestFlickrDescription|TestStripHTML" -v`
Expected: PASS

- [ ] **Step 5: Run linter**

Run: `golangci-lint run ./internal/flickr/`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
git add internal/flickr/metadata.go internal/flickr/metadata_test.go
git commit -m "Add Flickr metadata helpers: buildPhotoMeta, flickrDescription, isEmpty"
```

---

## Chunk 2: JPEG XMP Metadata Package

### Task 4: Create `internal/jpegmeta` package with `buildXMPPacket`

**Files:**
- Create: `internal/jpegmeta/jpegmeta.go`
- Create: `internal/jpegmeta/jpegmeta_test.go`

- [ ] **Step 1: Write failing tests for `buildXMPPacket`**

Create `internal/jpegmeta/jpegmeta_test.go`:

```go
package jpegmeta

import (
	"strings"
	"testing"
)

func TestBuildXMPPacket(t *testing.T) {
	meta := Metadata{
		Title:       "My Photo",
		Description: "A beautiful sunset",
		Tags:        []string{"sunset", "beach", "ocean"},
	}
	xmp := buildXMPPacket(meta)

	// Verify XMP structure
	if !strings.Contains(xmp, `<?xpacket begin=`) {
		t.Error("missing xpacket begin")
	}
	if !strings.Contains(xmp, `<?xpacket end="w"?>`) {
		t.Error("missing xpacket end")
	}
	if !strings.Contains(xmp, `xmlns:dc="http://purl.org/dc/elements/1.1/"`) {
		t.Error("missing Dublin Core namespace")
	}

	// Verify metadata values
	if !strings.Contains(xmp, "<dc:title>") || !strings.Contains(xmp, "My Photo") {
		t.Error("missing or incorrect dc:title")
	}
	if !strings.Contains(xmp, "<dc:description>") || !strings.Contains(xmp, "A beautiful sunset") {
		t.Error("missing or incorrect dc:description")
	}
	if !strings.Contains(xmp, "<dc:subject>") {
		t.Error("missing dc:subject")
	}
	for _, tag := range meta.Tags {
		if !strings.Contains(xmp, "<rdf:li>"+tag+"</rdf:li>") {
			t.Errorf("missing tag %q in dc:subject", tag)
		}
	}
}

func TestBuildXMPPacket_EmptyFields(t *testing.T) {
	xmp := buildXMPPacket(Metadata{Title: "Only Title"})
	if !strings.Contains(xmp, "Only Title") {
		t.Error("missing title")
	}
	if strings.Contains(xmp, "<dc:description>") {
		t.Error("empty description should be omitted")
	}
	if strings.Contains(xmp, "<dc:subject>") {
		t.Error("empty tags should be omitted")
	}
}

func TestBuildXMPPacket_XMLEscaping(t *testing.T) {
	meta := Metadata{
		Title:       `Photo & "Friends"`,
		Description: `<not a tag> & stuff`,
		Tags:        []string{"rock&roll"},
	}
	xmp := buildXMPPacket(meta)
	if !strings.Contains(xmp, "Photo &amp; &#34;Friends&#34;") {
		t.Errorf("title not properly escaped in XMP:\n%s", xmp)
	}
	if !strings.Contains(xmp, "&lt;not a tag&gt; &amp; stuff") {
		t.Errorf("description not properly escaped in XMP:\n%s", xmp)
	}
	if !strings.Contains(xmp, "rock&amp;roll") {
		t.Errorf("tag not properly escaped in XMP:\n%s", xmp)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/jpegmeta/ -run TestBuildXMPPacket -v`
Expected: FAIL — package doesn't exist

- [ ] **Step 3: Implement `buildXMPPacket` and `Metadata` type**

Create `internal/jpegmeta/jpegmeta.go`:

```go
// Package jpegmeta provides utilities for writing XMP metadata into JPEG files.
package jpegmeta

import (
	"fmt"
	"html"
	"strings"
)

// Metadata holds the metadata values to embed in a JPEG file.
type Metadata struct {
	Title       string
	Description string
	Tags        []string
}

// buildXMPPacket constructs a complete XMP packet as an XML string.
// Uses Dublin Core (dc) namespace for title, description, and subject (tags).
// Empty fields are omitted from the output.
func buildXMPPacket(meta Metadata) string {
	var b strings.Builder
	b.WriteString("<?xpacket begin=\"\xef\xbb\xbf\" id=\"W5M0MpCehiHzreSzNTczkc9d\"?>\n")
	b.WriteString("<x:xmpmeta xmlns:x=\"adobe:ns:meta/\">\n")
	b.WriteString("<rdf:RDF xmlns:rdf=\"http://www.w3.org/1999/02/22-rdf-syntax-ns#\">\n")
	b.WriteString("<rdf:Description rdf:about=\"\"\n")
	b.WriteString("  xmlns:dc=\"http://purl.org/dc/elements/1.1/\">\n")

	if meta.Title != "" {
		b.WriteString("<dc:title><rdf:Alt><rdf:li xml:lang=\"x-default\">")
		b.WriteString(escapeXML(meta.Title))
		b.WriteString("</rdf:li></rdf:Alt></dc:title>\n")
	}
	if meta.Description != "" {
		b.WriteString("<dc:description><rdf:Alt><rdf:li xml:lang=\"x-default\">")
		b.WriteString(escapeXML(meta.Description))
		b.WriteString("</rdf:li></rdf:Alt></dc:description>\n")
	}
	if len(meta.Tags) > 0 {
		b.WriteString("<dc:subject><rdf:Bag>\n")
		for _, tag := range meta.Tags {
			fmt.Fprintf(&b, "<rdf:li>%s</rdf:li>\n", escapeXML(tag))
		}
		b.WriteString("</rdf:Bag></dc:subject>\n")
	}

	b.WriteString("</rdf:Description>\n")
	b.WriteString("</rdf:RDF>\n")
	b.WriteString("</x:xmpmeta>\n")
	b.WriteString("<?xpacket end=\"w\"?>")
	return b.String()
}

// escapeXML escapes special characters for safe inclusion in XML text content.
func escapeXML(s string) string {
	s = html.EscapeString(s)
	return s
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/jpegmeta/ -run TestBuildXMPPacket -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/jpegmeta/jpegmeta.go internal/jpegmeta/jpegmeta_test.go
git commit -m "Add jpegmeta package with XMP packet construction"
```

---

### Task 5: Implement `jpegmeta.SetMetadata` (JPEG segment manipulation)

**Files:**
- Modify: `internal/jpegmeta/jpegmeta.go`
- Modify: `internal/jpegmeta/jpegmeta_test.go`

- [ ] **Step 1: Write failing tests for `SetMetadata`**

Add to `internal/jpegmeta/jpegmeta_test.go`:

```go
import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
)

// xmpNamespace is the standard XMP APP1 namespace header.
const xmpNamespace = "http://ns.adobe.com/xap/1.0/\x00"

// writeMinimalJPEG creates a minimal valid JPEG file: SOI + APP0 (JFIF) + SOS + EOI.
func writeMinimalJPEG(t *testing.T, path string) {
	t.Helper()
	var buf bytes.Buffer
	// SOI marker
	buf.Write([]byte{0xFF, 0xD8})
	// APP0 marker (JFIF)
	app0Data := []byte("JFIF\x00\x01\x01\x00\x00\x01\x00\x01\x00\x00")
	buf.Write([]byte{0xFF, 0xE0})
	binary.Write(&buf, binary.BigEndian, uint16(len(app0Data)+2))
	buf.Write(app0Data)
	// SOS marker (start of scan — signals image data follows)
	buf.Write([]byte{0xFF, 0xDA})
	binary.Write(&buf, binary.BigEndian, uint16(2))
	// EOI marker
	buf.Write([]byte{0xFF, 0xD9})
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
}

// readXMPFromJPEG reads the XMP APP1 segment from a JPEG file and returns its content.
func readXMPFromJPEG(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Scan for APP1 marker with XMP namespace
	for i := 0; i < len(data)-4; i++ {
		if data[i] == 0xFF && data[i+1] == 0xE1 {
			segLen := int(binary.BigEndian.Uint16(data[i+2 : i+4]))
			segData := data[i+4 : i+2+segLen]
			if bytes.HasPrefix(segData, []byte(xmpNamespace)) {
				return string(segData[len(xmpNamespace):])
			}
		}
	}
	return ""
}

func TestSetMetadata_WritesXMP(t *testing.T) {
	dir := t.TempDir()
	jpgPath := filepath.Join(dir, "test.jpg")
	writeMinimalJPEG(t, jpgPath)

	meta := Metadata{
		Title:       "Test Title",
		Description: "Test Description",
		Tags:        []string{"tag1", "tag2"},
	}
	if err := SetMetadata(jpgPath, meta); err != nil {
		t.Fatalf("SetMetadata failed: %v", err)
	}

	xmp := readXMPFromJPEG(t, jpgPath)
	if xmp == "" {
		t.Fatal("no XMP segment found in output JPEG")
	}
	if !strings.Contains(xmp, "Test Title") {
		t.Error("XMP missing title")
	}
	if !strings.Contains(xmp, "Test Description") {
		t.Error("XMP missing description")
	}
	if !strings.Contains(xmp, "tag1") || !strings.Contains(xmp, "tag2") {
		t.Error("XMP missing tags")
	}
}

func TestSetMetadata_ReplacesExistingXMP(t *testing.T) {
	dir := t.TempDir()
	jpgPath := filepath.Join(dir, "test.jpg")
	writeMinimalJPEG(t, jpgPath)

	// Write first
	if err := SetMetadata(jpgPath, Metadata{Title: "First"}); err != nil {
		t.Fatal(err)
	}
	// Write second (should replace)
	if err := SetMetadata(jpgPath, Metadata{Title: "Second"}); err != nil {
		t.Fatal(err)
	}

	xmp := readXMPFromJPEG(t, jpgPath)
	if strings.Contains(xmp, "First") {
		t.Error("old XMP should have been replaced")
	}
	if !strings.Contains(xmp, "Second") {
		t.Error("new XMP should be present")
	}
}

func TestSetMetadata_NonJPEGError(t *testing.T) {
	dir := t.TempDir()
	txtPath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(txtPath, []byte("not a jpeg"), 0644); err != nil {
		t.Fatal(err)
	}
	origData, _ := os.ReadFile(txtPath)

	err := SetMetadata(txtPath, Metadata{Title: "Test"})
	if err == nil {
		t.Fatal("expected error for non-JPEG file")
	}

	afterData, _ := os.ReadFile(txtPath)
	if !bytes.Equal(origData, afterData) {
		t.Error("original file should be unchanged on error")
	}
}

func TestSetMetadata_PreservesFilePermissions(t *testing.T) {
	dir := t.TempDir()
	jpgPath := filepath.Join(dir, "test.jpg")
	writeMinimalJPEG(t, jpgPath)

	if err := os.Chmod(jpgPath, 0600); err != nil {
		t.Fatal(err)
	}

	if err := SetMetadata(jpgPath, Metadata{Title: "Test"}); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(jpgPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("file permissions = %o, want 0600", info.Mode().Perm())
	}
}

func TestSetMetadata_NonexistentFile(t *testing.T) {
	err := SetMetadata("/nonexistent/path.jpg", Metadata{Title: "Test"})
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/jpegmeta/ -run "TestSetMetadata" -v`
Expected: FAIL — `SetMetadata` not defined

- [ ] **Step 3: Implement `SetMetadata`**

Add to `internal/jpegmeta/jpegmeta.go`:

```go
import (
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
)

const xmpNamespaceHeader = "http://ns.adobe.com/xap/1.0/\x00"

// SetMetadata writes XMP metadata (title, description, tags) into a JPEG file.
// It writes to a temp file and renames over the original on success.
// Preserves the original file's permissions.
func SetMetadata(filePath string, meta Metadata) error {
	origInfo, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("stat %s: %w", filePath, err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filePath, err)
	}

	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		return fmt.Errorf("%s is not a JPEG file", filePath)
	}

	xmpPayload := []byte(xmpNamespaceHeader + buildXMPPacket(meta))

	result, err := insertOrReplaceXMP(data, xmpPayload)
	if err != nil {
		return fmt.Errorf("inserting XMP: %w", err)
	}

	out, err := os.CreateTemp(filepath.Dir(filePath), "*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := out.Name()

	if _, err := out.Write(result); err != nil {
		_ = out.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Chmod(tmpPath, origInfo.Mode().Perm()); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("setting permissions: %w", err)
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("renaming temp file: %w", err)
	}
	return nil
}

// insertOrReplaceXMP scans JPEG marker segments and inserts an XMP APP1 segment
// after the existing APP0/EXIF APP1 segments, or replaces an existing XMP APP1.
func insertOrReplaceXMP(data, xmpPayload []byte) ([]byte, error) {
	if len(xmpPayload)+2 > 65535 {
		return nil, fmt.Errorf("XMP payload too large: %d bytes", len(xmpPayload))
	}

	var result bytes.Buffer
	// Write SOI
	result.Write(data[:2])

	pos := 2
	inserted := false

	for pos < len(data)-1 {
		if data[pos] != 0xFF {
			// Not a marker — write remaining data (image data after SOS)
			result.Write(data[pos:])
			break
		}

		marker := data[pos+1]

		// SOS (0xDA) or EOI (0xD9) — insert XMP before if not yet inserted, then write rest
		if marker == 0xDA || marker == 0xD9 {
			if !inserted {
				writeXMPSegment(&result, xmpPayload)
				inserted = true
			}
			result.Write(data[pos:])
			break
		}

		// Markers without length (standalone markers like RST, TEM)
		if marker == 0x00 || (marker >= 0xD0 && marker <= 0xD7) || marker == 0x01 {
			result.Write(data[pos : pos+2])
			pos += 2
			continue
		}

		// Read segment length
		if pos+4 > len(data) {
			result.Write(data[pos:])
			break
		}
		segLen := int(binary.BigEndian.Uint16(data[pos+2 : pos+4]))
		segEnd := pos + 2 + segLen
		if segEnd > len(data) {
			result.Write(data[pos:])
			break
		}

		// Check if this is an existing XMP APP1 segment — skip it (will be replaced)
		if marker == 0xE1 {
			segData := data[pos+4 : segEnd]
			if bytes.HasPrefix(segData, []byte(xmpNamespaceHeader)) {
				// Skip this segment (we'll insert our new one)
				pos = segEnd
				continue
			}
		}

		// Copy this segment
		result.Write(data[pos:segEnd])

		// Insert XMP after APP0 (0xE0) or EXIF APP1 (0xE1) segments
		if !inserted && (marker == 0xE0 || marker == 0xE1) {
			// Check if next marker is still an APP marker; if so, wait
			if segEnd+1 < len(data) && data[segEnd] == 0xFF {
				nextMarker := data[segEnd+1]
				if nextMarker == 0xE0 || nextMarker == 0xE1 {
					pos = segEnd
					continue
				}
			}
			writeXMPSegment(&result, xmpPayload)
			inserted = true
		}

		pos = segEnd
	}

	return result.Bytes(), nil
}

// writeXMPSegment writes an APP1 XMP marker segment to w.
func writeXMPSegment(w io.Writer, xmpPayload []byte) {
	segLen := uint16(len(xmpPayload) + 2)
	buf := []byte{0xFF, 0xE1, byte(segLen >> 8), byte(segLen)}
	_, _ = w.Write(buf)
	_, _ = w.Write(xmpPayload)
}
```

Note: The `bytes` import needs to be added to the import block at the top of the file.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/jpegmeta/ -v`
Expected: PASS

- [ ] **Step 5: Run linter**

Run: `golangci-lint run ./internal/jpegmeta/`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
git add internal/jpegmeta/jpegmeta.go internal/jpegmeta/jpegmeta_test.go
git commit -m "Implement jpegmeta.SetMetadata for writing XMP into JPEG files"
```

---

## Chunk 3: MP4 XMP Metadata Extension

### Task 6: Add `SetXMPMetadata` to `internal/mp4meta`

**Files:**
- Modify: `internal/mp4meta/mp4meta.go`
- Modify: `internal/mp4meta/mp4meta_test.go`

- [ ] **Step 1: Write failing tests for `SetXMPMetadata`**

Add to `internal/mp4meta/mp4meta_test.go`:

```go
import (
	"encoding/binary"
	"io"
	"strings"
)

// xmpUUIDBytes is the standard UUID for XMP metadata in ISO BMFF containers.
var xmpUUIDBytes = []byte{0xBE, 0x7A, 0xCF, 0xCB, 0x97, 0xA9, 0x42, 0xE8, 0x9C, 0x71, 0x99, 0x94, 0x91, 0xE3, 0xAF, 0xAC}

// readXMPFromMP4 reads the XMP uuid box from an MP4 file and returns the XMP content.
func readXMPFromMP4(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	for {
		// Read box header: 4 bytes size + 4 bytes type
		var header [8]byte
		if _, err := io.ReadFull(f, header[:]); err != nil {
			break
		}
		boxSize := binary.BigEndian.Uint32(header[:4])
		boxType := string(header[4:8])

		if boxSize < 8 {
			break
		}

		if boxType == "uuid" && boxSize > 24 {
			// Read the 16-byte UUID
			var uuid [16]byte
			if _, err := io.ReadFull(f, uuid[:]); err != nil {
				break
			}
			if bytes.Equal(uuid[:], xmpUUIDBytes) {
				xmpData := make([]byte, boxSize-24)
				if _, err := io.ReadFull(f, xmpData); err != nil {
					break
				}
				return string(xmpData)
			}
			// Not XMP UUID, skip remaining
			if _, err := f.Seek(int64(boxSize)-24, io.SeekCurrent); err != nil {
				break
			}
		} else {
			// Skip this box
			if _, err := f.Seek(int64(boxSize)-8, io.SeekCurrent); err != nil {
				break
			}
		}
	}
	return ""
}

func TestSetXMPMetadata_WritesUUIDBox(t *testing.T) {
	dir := t.TempDir()
	mp4Path := filepath.Join(dir, "test.mp4")
	writeMinimalMP4(t, mp4Path, 0)

	meta := XMPMetadata{
		Title:       "My Video",
		Description: "A great video",
		Tags:        []string{"travel", "vlog"},
	}
	if err := SetXMPMetadata(mp4Path, meta); err != nil {
		t.Fatalf("SetXMPMetadata failed: %v", err)
	}

	xmp := readXMPFromMP4(t, mp4Path)
	if xmp == "" {
		t.Fatal("no XMP uuid box found")
	}
	if !strings.Contains(xmp, "My Video") {
		t.Error("XMP missing title")
	}
	if !strings.Contains(xmp, "A great video") {
		t.Error("XMP missing description")
	}
	if !strings.Contains(xmp, "travel") || !strings.Contains(xmp, "vlog") {
		t.Error("XMP missing tags")
	}
}

func TestSetXMPMetadata_AfterSetCreationTime(t *testing.T) {
	dir := t.TempDir()
	mp4Path := filepath.Join(dir, "test.mp4")
	writeMinimalMP4(t, mp4Path, 0)

	targetTime := time.Date(2020, 6, 15, 14, 30, 0, 0, time.UTC)
	if err := SetCreationTime(mp4Path, targetTime); err != nil {
		t.Fatalf("SetCreationTime failed: %v", err)
	}

	meta := XMPMetadata{Title: "After Timestamps"}
	if err := SetXMPMetadata(mp4Path, meta); err != nil {
		t.Fatalf("SetXMPMetadata failed: %v", err)
	}

	// Verify timestamps are still intact
	expectedMP4Time := uint32(uint64(targetTime.Unix()) + mp4Epoch)
	verifyTimestampsV0(t, mp4Path, expectedMP4Time)

	// Verify XMP is present
	xmp := readXMPFromMP4(t, mp4Path)
	if !strings.Contains(xmp, "After Timestamps") {
		t.Error("XMP missing after combined SetCreationTime + SetXMPMetadata")
	}
}

func TestSetXMPMetadata_ReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	mp4Path := filepath.Join(dir, "test.mp4")
	writeMinimalMP4(t, mp4Path, 0)

	if err := SetXMPMetadata(mp4Path, XMPMetadata{Title: "First"}); err != nil {
		t.Fatal(err)
	}
	if err := SetXMPMetadata(mp4Path, XMPMetadata{Title: "Second"}); err != nil {
		t.Fatal(err)
	}

	xmp := readXMPFromMP4(t, mp4Path)
	if strings.Contains(xmp, "First") {
		t.Error("old XMP should have been replaced")
	}
	if !strings.Contains(xmp, "Second") {
		t.Error("new XMP should be present")
	}
}

func TestSetXMPMetadata_NonMP4Error(t *testing.T) {
	dir := t.TempDir()
	txtPath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(txtPath, []byte("not an mp4"), 0644); err != nil {
		t.Fatal(err)
	}
	origData, _ := os.ReadFile(txtPath)

	err := SetXMPMetadata(txtPath, XMPMetadata{Title: "Test"})
	if err == nil {
		t.Fatal("expected error for non-MP4 file")
	}

	afterData, _ := os.ReadFile(txtPath)
	if !bytes.Equal(origData, afterData) {
		t.Error("original file should be unchanged on error")
	}
}

func TestSetXMPMetadata_PreservesFilePermissions(t *testing.T) {
	dir := t.TempDir()
	mp4Path := filepath.Join(dir, "test.mp4")
	writeMinimalMP4(t, mp4Path, 0)

	if err := os.Chmod(mp4Path, 0600); err != nil {
		t.Fatal(err)
	}

	if err := SetXMPMetadata(mp4Path, XMPMetadata{Title: "Test"}); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(mp4Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("file permissions = %o, want 0600", info.Mode().Perm())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/mp4meta/ -run "TestSetXMPMetadata" -v`
Expected: FAIL — `XMPMetadata` and `SetXMPMetadata` not defined

- [ ] **Step 3: Implement `SetXMPMetadata`**

Add to `internal/mp4meta/mp4meta.go`. Uses raw I/O (not `go-mp4`'s `ReadBoxStructure`) because `go-mp4` does not natively support reading/writing UUID extended-type boxes, and its callback model makes it difficult to insert new top-level boxes between existing ones.

```go
import (
	"bytes"
	"encoding/binary"
	"html"
	"io"
	"strings"
)

// XMPMetadata holds the metadata values to embed as XMP in an MP4/MOV file.
type XMPMetadata struct {
	Title       string
	Description string
	Tags        []string
}

// xmpUUID is the standard UUID for XMP metadata in ISO BMFF containers.
var xmpUUID = [16]byte{0xBE, 0x7A, 0xCF, 0xCB, 0x97, 0xA9, 0x42, 0xE8, 0x9C, 0x71, 0x99, 0x94, 0x91, 0xE3, 0xAF, 0xAC}

// SetXMPMetadata writes XMP metadata (title, description, tags) into an MP4 or MOV
// file as a uuid box. Uses raw I/O to read and copy top-level boxes, skipping any
// existing XMP uuid box and appending a new one after moov.
// Writes to a temp file and renames over the original on success.
// Preserves the original file's permissions.
func SetXMPMetadata(filePath string, meta XMPMetadata) error {
	origInfo, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("stat %s: %w", filePath, err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filePath, err)
	}

	// Validate: must have at least one box (8 bytes minimum)
	if len(data) < 8 {
		return fmt.Errorf("%s is too small to be an MP4 file", filePath)
	}

	xmpPayload := buildMP4XMPPacket(meta)
	result, err := insertOrReplaceUUIDBox(data, xmpPayload)
	if err != nil {
		return fmt.Errorf("inserting XMP: %w", err)
	}

	out, err := os.CreateTemp(filepath.Dir(filePath), "*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := out.Name()

	if _, err := out.Write(result); err != nil {
		_ = out.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Chmod(tmpPath, origInfo.Mode().Perm()); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("setting permissions: %w", err)
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("renaming temp file: %w", err)
	}
	return nil
}

// insertOrReplaceUUIDBox walks top-level MP4 boxes, copies them all through
// (skipping any existing XMP uuid box), and appends a new XMP uuid box after moov.
func insertOrReplaceUUIDBox(data []byte, xmpPayload []byte) ([]byte, error) {
	var result bytes.Buffer
	pos := 0
	wroteXMP := false

	for pos < len(data) {
		if pos+8 > len(data) {
			// Remaining bytes too small for a box header — copy as-is
			result.Write(data[pos:])
			break
		}

		boxSize := int(binary.BigEndian.Uint32(data[pos : pos+4]))
		boxType := string(data[pos+4 : pos+8])

		// Handle size==0 (box extends to end of file)
		if boxSize == 0 {
			boxSize = len(data) - pos
		}
		// Handle size==1 (64-bit extended size)
		if boxSize == 1 {
			if pos+16 > len(data) {
				result.Write(data[pos:])
				break
			}
			extSize := binary.BigEndian.Uint64(data[pos+8 : pos+16])
			if extSize > uint64(len(data)) {
				result.Write(data[pos:])
				break
			}
			boxSize = int(extSize)
		}

		if boxSize < 8 || pos+boxSize > len(data) {
			// Invalid box — copy remaining data as-is
			result.Write(data[pos:])
			break
		}

		// Skip existing XMP uuid box
		if boxType == "uuid" && boxSize > 24 {
			if bytes.Equal(data[pos+8:pos+24], xmpUUID[:]) {
				pos += boxSize
				continue
			}
		}

		// Copy this box
		result.Write(data[pos : pos+boxSize])

		// After moov box, insert XMP uuid box
		if boxType == "moov" && !wroteXMP {
			writeXMPUUIDBox(&result, xmpPayload)
			wroteXMP = true
		}

		pos += boxSize
	}

	// If no moov was found (shouldn't happen for valid files), append at end
	if !wroteXMP {
		writeXMPUUIDBox(&result, xmpPayload)
	}

	return result.Bytes(), nil
}

// writeXMPUUIDBox writes a uuid box with XMP payload to w.
// Format: [4-byte big-endian size]["uuid"][16-byte UUID][XMP payload]
func writeXMPUUIDBox(w io.Writer, xmpPayload []byte) {
	boxSize := uint32(8 + 16 + len(xmpPayload))
	_ = binary.Write(w, binary.BigEndian, boxSize)
	_, _ = w.Write([]byte("uuid"))
	_, _ = w.Write(xmpUUID[:])
	_, _ = w.Write(xmpPayload)
}

// buildMP4XMPPacket constructs an XMP packet for MP4/MOV files.
// Duplicates the logic from jpegmeta — both packages are independent.
func buildMP4XMPPacket(meta XMPMetadata) []byte {
	var b strings.Builder
	b.WriteString("<?xpacket begin=\"\xef\xbb\xbf\" id=\"W5M0MpCehiHzreSzNTczkc9d\"?>\n")
	b.WriteString("<x:xmpmeta xmlns:x=\"adobe:ns:meta/\">\n")
	b.WriteString("<rdf:RDF xmlns:rdf=\"http://www.w3.org/1999/02/22-rdf-syntax-ns#\">\n")
	b.WriteString("<rdf:Description rdf:about=\"\"\n")
	b.WriteString("  xmlns:dc=\"http://purl.org/dc/elements/1.1/\">\n")

	if meta.Title != "" {
		b.WriteString("<dc:title><rdf:Alt><rdf:li xml:lang=\"x-default\">")
		b.WriteString(escapeXML(meta.Title))
		b.WriteString("</rdf:li></rdf:Alt></dc:title>\n")
	}
	if meta.Description != "" {
		b.WriteString("<dc:description><rdf:Alt><rdf:li xml:lang=\"x-default\">")
		b.WriteString(escapeXML(meta.Description))
		b.WriteString("</rdf:li></rdf:Alt></dc:description>\n")
	}
	if len(meta.Tags) > 0 {
		b.WriteString("<dc:subject><rdf:Bag>\n")
		for _, tag := range meta.Tags {
			fmt.Fprintf(&b, "<rdf:li>%s</rdf:li>\n", escapeXML(tag))
		}
		b.WriteString("</rdf:Bag></dc:subject>\n")
	}

	b.WriteString("</rdf:Description>\n")
	b.WriteString("</rdf:RDF>\n")
	b.WriteString("</x:xmpmeta>\n")
	b.WriteString("<?xpacket end=\"w\"?>")
	return []byte(b.String())
}

func escapeXML(s string) string {
	return html.EscapeString(s)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/mp4meta/ -v`
Expected: PASS (both old and new tests)

- [ ] **Step 5: Run linter**

Run: `golangci-lint run ./internal/mp4meta/`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
git add internal/mp4meta/mp4meta.go internal/mp4meta/mp4meta_test.go
git commit -m "Add SetXMPMetadata to mp4meta for writing title/description/tags"
```

---

## Chunk 4: Flickr Download Loop Integration

### Task 7: Update Flickr API response struct and extras parameter

**Files:**
- Modify: `internal/flickr/flickr.go`

- [ ] **Step 1: Add `Description` and `Tags` to `photosResponse` and update `extras`**

In `internal/flickr/flickr.go`, update the `photosResponse` struct (around line 214) to add `Description` and `Tags`:

```go
Photo []struct {
	ID          string           `json:"id"`
	Secret      string           `json:"secret"`
	Server      string           `json:"server"`
	Title       string           `json:"title"`
	DateTaken   string           `json:"datetaken"`
	DateUpload  string           `json:"dateupload"`
	Description flickrDescription `json:"description"`
	Tags        string           `json:"tags"`
} `json:"photo"`
```

Update the extras parameter (around line 288) from:
```go
"extras": "date_taken,date_upload",
```
to:
```go
"extras": "date_taken,date_upload,description,tags",
```

- [ ] **Step 2: Run existing tests to verify nothing broke**

Run: `go test ./internal/flickr/ -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/flickr/flickr.go
git commit -m "Add description and tags to Flickr API response struct and extras"
```

---

### Task 8: Integrate metadata writing into the download loop

**Files:**
- Modify: `internal/flickr/flickr.go`

- [ ] **Step 1: Add import for `jpegmeta` package**

Add to the import block in `internal/flickr/flickr.go`:

```go
"github.com/briandeitte/photo-copy/internal/jpegmeta"
```

- [ ] **Step 2: Add metadata writing after the date-setting block**

After the existing date-setting block (around line 386, after the `else` block for "no date available"), add:

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

- [ ] **Step 3: Run all tests**

Run: `go test ./internal/flickr/ -v`
Expected: PASS

Run: `go test ./... `
Expected: PASS

- [ ] **Step 4: Run linter**

Run: `golangci-lint run ./...`
Expected: No errors

- [ ] **Step 5: Commit**

```bash
git add internal/flickr/flickr.go
git commit -m "Integrate metadata writing into Flickr download loop"
```

---

## Chunk 5: Integration Tests and Documentation

### Task 9: Add integration tests for metadata embedding

**Files:**
- Modify: `internal/cli/integration_test.go`

- [ ] **Step 1: Write integration test for JPEG metadata embedding**

Add to `internal/cli/integration_test.go`:

```go
import (
	"bytes"
	"encoding/binary"

	"github.com/briandeitte/photo-copy/internal/jpegmeta"
)
```

Add a minimal JPEG builder and the test:

```go
// minimalJPEGData creates a minimal valid JPEG byte slice (SOI + APP0 + SOS + EOI).
func minimalJPEGData() []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0xFF, 0xD8}) // SOI
	app0Data := []byte("JFIF\x00\x01\x01\x00\x00\x01\x00\x01\x00\x00")
	buf.Write([]byte{0xFF, 0xE0})
	_ = binary.Write(&buf, binary.BigEndian, uint16(len(app0Data)+2))
	buf.Write(app0Data)
	buf.Write([]byte{0xFF, 0xDA}) // SOS
	_ = binary.Write(&buf, binary.BigEndian, uint16(2))
	buf.Write([]byte{0xFF, 0xD9}) // EOI
	return buf.Bytes()
}

func TestFlickrDownload_EmbedsJPEGMetadata(t *testing.T) {
	outputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	photosWithDesc := []map[string]any{
		{
			"id": "1", "secret": "aaa", "server": "1", "title": "Sunset Photo",
			"datetaken": "2020-06-15 14:30:00", "dateupload": "1592234567",
			"tags": "sunset beach ocean",
			"description": map[string]string{"_content": "<b>Beautiful</b> sunset at the beach"},
		},
	}

	var mock *mockserver.FlickrMock
	mock = mockserver.NewFlickr(t).
		OnGetPhotos(mockserver.RespondJSON(200, map[string]any{
			"photos": map[string]any{
				"page": 1, "pages": 1, "total": 1,
				"photo": photosWithDesc,
			},
			"stat": "ok",
		})).
		OnGetSizes(func(w http.ResponseWriter, r *http.Request) {
			photoID := r.URL.Query().Get("photo_id")
			mockserver.RespondJSON(200, flickrSizesResponse(
				mock.Server.URL+"/download/"+photoID+".jpg",
			))(w, r)
		}).
		OnDownload(mockserver.RespondBytes(200, minimalJPEGData())).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_API_URL", mock.APIURL)

	err := executeCmd(t, "flickr", "download", outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Read back the JPEG file and check for XMP metadata
	filePath := filepath.Join(outputDir, "1_aaa.jpg")
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}

	// Find XMP APP1 segment
	xmpNS := []byte("http://ns.adobe.com/xap/1.0/\x00")
	xmpContent := ""
	for i := 0; i < len(data)-4; i++ {
		if data[i] == 0xFF && data[i+1] == 0xE1 {
			segLen := int(binary.BigEndian.Uint16(data[i+2 : i+4]))
			segData := data[i+4 : i+2+segLen]
			if bytes.HasPrefix(segData, xmpNS) {
				xmpContent = string(segData[len(xmpNS):])
				break
			}
		}
	}

	if xmpContent == "" {
		t.Fatal("no XMP metadata found in downloaded JPEG")
	}
	if !strings.Contains(xmpContent, "Sunset Photo") {
		t.Error("XMP missing title")
	}
	if !strings.Contains(xmpContent, "Beautiful sunset at the beach") {
		t.Error("XMP missing description (should be HTML-stripped)")
	}
	if strings.Contains(xmpContent, "<b>") {
		t.Error("XMP should not contain HTML tags in description")
	}
	if !strings.Contains(xmpContent, "sunset") || !strings.Contains(xmpContent, "beach") || !strings.Contains(xmpContent, "ocean") {
		t.Error("XMP missing tags")
	}
}
```

- [ ] **Step 2: Write integration test for MP4 metadata embedding**

Add to `internal/cli/integration_test.go`. This test uses `writeMinimalMP4` from `mp4meta_test.go` — since that's in the `mp4meta` package and not exported, create a minimal MP4 inline using the `go-mp4` writer, or use the raw byte approach. Simplest: write a minimal valid MP4 as raw bytes.

```go
func TestFlickrDownload_EmbedsMP4Metadata(t *testing.T) {
	outputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	// Create a minimal MP4 file to serve as download content.
	// This needs to be a valid MP4 that mp4meta.SetCreationTime and SetXMPMetadata can process.
	// Use a temp file and the go-mp4 writer to create it.
	mp4Content := createMinimalMP4(t)

	photosWithDesc := []map[string]any{
		{
			"id": "1", "secret": "aaa", "server": "1", "title": "Beach Video",
			"datetaken": "2020-06-15 14:30:00", "dateupload": "1592234567",
			"tags": "beach waves",
			"description": map[string]string{"_content": "Video of <b>big</b> waves"},
		},
	}

	var mock *mockserver.FlickrMock
	mock = mockserver.NewFlickr(t).
		OnGetPhotos(mockserver.RespondJSON(200, map[string]any{
			"photos": map[string]any{
				"page": 1, "pages": 1, "total": 1,
				"photo": photosWithDesc,
			},
			"stat": "ok",
		})).
		OnGetSizes(func(w http.ResponseWriter, r *http.Request) {
			mockserver.RespondJSON(200, flickrMultiSizesResponse([]map[string]string{
				{"label": "Video Original", "source": mock.Server.URL + "/download/video.mp4"},
			}))(w, r)
		}).
		OnDownload(mockserver.RespondBytes(200, mp4Content)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_API_URL", mock.APIURL)

	err := executeCmd(t, "flickr", "download", outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	filePath := filepath.Join(outputDir, "1_aaa.mp4")

	// Verify XMP uuid box is present
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}

	xmpUUID := []byte{0xBE, 0x7A, 0xCF, 0xCB, 0x97, 0xA9, 0x42, 0xE8, 0x9C, 0x71, 0x99, 0x94, 0x91, 0xE3, 0xAF, 0xAC}
	xmpContent := ""
	pos := 0
	for pos+8 < len(data) {
		boxSize := int(binary.BigEndian.Uint32(data[pos : pos+4]))
		boxType := string(data[pos+4 : pos+8])
		if boxSize < 8 || pos+boxSize > len(data) {
			break
		}
		if boxType == "uuid" && boxSize > 24 && bytes.Equal(data[pos+8:pos+24], xmpUUID) {
			xmpContent = string(data[pos+24 : pos+boxSize])
			break
		}
		pos += boxSize
	}

	if xmpContent == "" {
		t.Fatal("no XMP uuid box found in downloaded MP4")
	}
	if !strings.Contains(xmpContent, "Beach Video") {
		t.Error("XMP missing title")
	}
	if !strings.Contains(xmpContent, "Video of big waves") {
		t.Error("XMP missing description (should be HTML-stripped)")
	}
	if !strings.Contains(xmpContent, "beach") || !strings.Contains(xmpContent, "waves") {
		t.Error("XMP missing tags")
	}

	// Verify file system timestamp was set
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	expectedTime := time.Date(2020, 6, 15, 14, 30, 0, 0, time.UTC)
	if !info.ModTime().UTC().Equal(expectedTime) {
		t.Errorf("file mod time = %v, want %v", info.ModTime().UTC(), expectedTime)
	}
}
```

Add the `createMinimalMP4` helper that uses `go-mp4` to build a valid MP4:

```go
import (
	gomp4 "github.com/abema/go-mp4"
)

// createMinimalMP4 creates a minimal valid MP4 byte slice (ftyp + moov with mvhd/trak/tkhd/mdia/mdhd).
func createMinimalMP4(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gomp4.NewWriter(&buf)

	ftyp, err := w.StartBox(&gomp4.BoxInfo{Type: gomp4.BoxTypeFtyp()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = gomp4.Marshal(w, &gomp4.Ftyp{MajorBrand: [4]byte{'i', 's', 'o', 'm'}, MinorVersion: 0x200}, ftyp.Context)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.EndBox(); err != nil {
		t.Fatal(err)
	}

	if _, err = w.StartBox(&gomp4.BoxInfo{Type: gomp4.BoxTypeMoov()}); err != nil {
		t.Fatal(err)
	}
	mvhdBI, err := w.StartBox(&gomp4.BoxInfo{Type: gomp4.BoxTypeMvhd()})
	if err != nil {
		t.Fatal(err)
	}
	mvhd := &gomp4.Mvhd{FullBox: gomp4.FullBox{Version: 0}, Rate: 0x00010000, Volume: 0x0100, NextTrackID: 2}
	mvhd.Timescale = 1000
	if _, err = gomp4.Marshal(w, mvhd, mvhdBI.Context); err != nil {
		t.Fatal(err)
	}
	if _, err = w.EndBox(); err != nil {
		t.Fatal(err)
	}

	if _, err = w.StartBox(&gomp4.BoxInfo{Type: gomp4.BoxTypeTrak()}); err != nil {
		t.Fatal(err)
	}
	tkhdBI, err := w.StartBox(&gomp4.BoxInfo{Type: gomp4.BoxTypeTkhd()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = gomp4.Marshal(w, &gomp4.Tkhd{FullBox: gomp4.FullBox{Version: 0, Flags: [3]byte{0, 0, 3}}, TrackID: 1}, tkhdBI.Context); err != nil {
		t.Fatal(err)
	}
	if _, err = w.EndBox(); err != nil {
		t.Fatal(err)
	}

	if _, err = w.StartBox(&gomp4.BoxInfo{Type: gomp4.BoxTypeMdia()}); err != nil {
		t.Fatal(err)
	}
	mdhdBI, err := w.StartBox(&gomp4.BoxInfo{Type: gomp4.BoxTypeMdhd()})
	if err != nil {
		t.Fatal(err)
	}
	mdhd := &gomp4.Mdhd{FullBox: gomp4.FullBox{Version: 0}}
	mdhd.Timescale = 1000
	if _, err = gomp4.Marshal(w, mdhd, mdhdBI.Context); err != nil {
		t.Fatal(err)
	}
	if _, err = w.EndBox(); err != nil {
		t.Fatal(err)
	}

	// Close mdia, trak, moov
	for range 3 {
		if _, err = w.EndBox(); err != nil {
			t.Fatal(err)
		}
	}

	return buf.Bytes()
}
```

- [ ] **Step 3: Run the integration tests**

Run: `go test ./internal/cli/ -tags integration -run "TestFlickrDownload_Embeds" -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/cli/integration_test.go
git commit -m "Add integration tests for Flickr JPEG and MP4 metadata embedding"
```

---

### Task 10: Update README.md and CLAUDE.md

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`

- [ ] **Step 1: Add "Media metadata preservation" section to README.md**

After the "Media date preservation" section (around line 99), add:

```markdown
### Media metadata preservation

Flickr downloads embed original metadata into downloaded files:

- **Title** — Written as XMP `dc:title`
- **Description** — Written as XMP `dc:description` (HTML stripped to plain text)
- **Tags** — Written as XMP `dc:subject` keywords

Supported for JPEG photos and MP4/MOV videos. Other file types retain only file system timestamps (no embedded metadata).
```

- [ ] **Step 2: Update CLAUDE.md package layout**

Add the `jpegmeta` package description after `mp4meta` in the package layout:

```
- **jpegmeta/** - Writes XMP metadata (title, description, tags) into JPEG files. Used by Flickr downloads to preserve original Flickr metadata in photo files.
```

Update the `mp4meta` description to:

```
- **mp4meta/** - Sets creation/modification timestamps and XMP metadata (title, description, tags) in MP4/MOV container metadata (`mvhd`/`tkhd`/`mdhd` boxes and `uuid` XMP box) using `abema/go-mp4`. Used by Flickr downloads to preserve original capture dates and Flickr metadata in video files.
```

Update the Flickr key patterns bullet to:

```
- Flickr downloads preserve original dates and metadata: `date_taken` (preferred) or `date_upload` (fallback) from the API. Title, description, and tags are embedded as XMP metadata. Video files (`.mp4`, `.mov`) get MP4 container metadata updated via the `mp4meta` package; JPEG photos get XMP written via the `jpegmeta` package; all files get file system timestamps set via `os.Chtimes`.
```

- [ ] **Step 3: Run all tests and linter**

Run: `go test ./...`
Expected: PASS

Run: `golangci-lint run ./...`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add README.md CLAUDE.md
git commit -m "Update README and CLAUDE.md with metadata embedding documentation"
```

---

### Task 11: Final verification

- [ ] **Step 1: Run full test suite**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 2: Run integration tests**

Run: `go test ./internal/cli/ -tags integration -v`
Expected: PASS

- [ ] **Step 3: Run linter**

Run: `golangci-lint run ./...`
Expected: No errors

- [ ] **Step 4: Build the binary**

Run: `go build -o photo-copy ./cmd/photo-copy`
Expected: Builds successfully
