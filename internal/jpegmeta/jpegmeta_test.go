package jpegmeta

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeMinimalJPEG creates a minimal valid JPEG file with SOI + APP0(JFIF) + SOS + EOI.
func writeMinimalJPEG(t *testing.T, path string) {
	t.Helper()
	// SOI marker
	soi := []byte{0xFF, 0xD8}
	// APP0 JFIF segment: marker + length (2 bytes) + JFIF\x00 identifier + minimal data
	app0Payload := []byte("JFIF\x00\x01\x01\x00\x00\x01\x00\x01\x00\x00")
	app0Len := uint16(len(app0Payload) + 2)
	app0 := []byte{
		0xFF, 0xE0,
		byte(app0Len >> 8), byte(app0Len),
	}
	app0 = append(app0, app0Payload...)
	// SOS marker (start of scan) — minimal, just the marker
	sos := []byte{0xFF, 0xDA, 0x00, 0x0C, 0x03, 0x01, 0x00, 0x02, 0x11, 0x03, 0x11, 0x00, 0x3F, 0x00}
	// EOI marker
	eoi := []byte{0xFF, 0xD9}

	var data []byte
	data = append(data, soi...)
	data = append(data, app0...)
	data = append(data, sos...)
	data = append(data, eoi...)

	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

// readXMPFromJPEG scans the JPEG for an APP1 segment with XMP namespace, returning XMP content.
func readXMPFromJPEG(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const xmpNS = "http://ns.adobe.com/xap/1.0/\x00"
	i := 2 // skip SOI
	for i+4 <= len(data) {
		if data[i] != 0xFF {
			break
		}
		marker := data[i+1]
		if marker == 0xD9 || marker == 0xDA {
			break
		}
		segLen := int(data[i+2])<<8 | int(data[i+3])
		payload := data[i+4 : i+2+segLen]
		if marker == 0xE1 && strings.HasPrefix(string(payload), xmpNS) {
			return string(payload[len(xmpNS):])
		}
		i += 2 + segLen
	}
	t.Fatal("XMP APP1 segment not found in JPEG")
	return ""
}

// --- Tests for buildXMPPacket ---

func TestBuildXMPPacket_AllFields(t *testing.T) {
	meta := Metadata{
		Title:       "My Photo",
		Description: "A nice sunset",
		Tags:        []string{"sunset", "nature"},
	}
	pkt := buildXMPPacket(meta)

	checks := []string{
		`<?xpacket begin=`,
		`id="W5M0MpCehiHzreSzNTczkc9d"`,
		`<?xpacket end="w"?>`,
		`xmlns:dc="http://purl.org/dc/elements/1.1/"`,
		`<dc:title>`,
		`<rdf:Alt>`,
		`xml:lang="x-default"`,
		`My Photo`,
		`</dc:title>`,
		`<dc:description>`,
		`A nice sunset`,
		`</dc:description>`,
		`<dc:subject>`,
		`<rdf:Bag>`,
		`<rdf:li>sunset</rdf:li>`,
		`<rdf:li>nature</rdf:li>`,
		`</rdf:Bag>`,
		`</dc:subject>`,
	}
	for _, want := range checks {
		if !strings.Contains(pkt, want) {
			t.Errorf("buildXMPPacket missing %q", want)
		}
	}
}

func TestBuildXMPPacket_EmptyFields(t *testing.T) {
	meta := Metadata{
		Title: "Only Title",
	}
	pkt := buildXMPPacket(meta)

	if !strings.Contains(pkt, `<dc:title>`) {
		t.Error("expected dc:title to be present")
	}
	if strings.Contains(pkt, `dc:description`) {
		t.Error("dc:description should be omitted when empty")
	}
	if strings.Contains(pkt, `dc:subject`) {
		t.Error("dc:subject should be omitted when empty")
	}
}

func TestBuildXMPPacket_XMLEscaping(t *testing.T) {
	meta := Metadata{
		Title:       `Photo & "Friends"`,
		Description: "",
		Tags:        []string{`<not a tag>`},
	}
	pkt := buildXMPPacket(meta)

	if !strings.Contains(pkt, `Photo &amp; &#34;Friends&#34;`) {
		t.Errorf("title not properly XML-escaped, got packet:\n%s", pkt)
	}
	if !strings.Contains(pkt, `&lt;not a tag&gt;`) {
		t.Errorf("tag not properly XML-escaped, got packet:\n%s", pkt)
	}
}

// --- Tests for SetMetadata ---

func TestSetMetadata_WritesXMP(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jpg")
	writeMinimalJPEG(t, path)

	meta := Metadata{
		Title:       "Sunset Over Mountains",
		Description: "Beautiful golden hour",
		Tags:        []string{"sunset", "mountains", "golden hour"},
	}
	if err := SetMetadata(path, meta); err != nil {
		t.Fatalf("SetMetadata failed: %v", err)
	}

	xmp := readXMPFromJPEG(t, path)
	if !strings.Contains(xmp, "Sunset Over Mountains") {
		t.Errorf("XMP missing title, got: %s", xmp)
	}
	if !strings.Contains(xmp, "Beautiful golden hour") {
		t.Errorf("XMP missing description, got: %s", xmp)
	}
	if !strings.Contains(xmp, "<rdf:li>sunset</rdf:li>") {
		t.Errorf("XMP missing tag 'sunset', got: %s", xmp)
	}
	if !strings.Contains(xmp, "<rdf:li>mountains</rdf:li>") {
		t.Errorf("XMP missing tag 'mountains', got: %s", xmp)
	}
}

func TestSetMetadata_ReplacesExistingXMP(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jpg")
	writeMinimalJPEG(t, path)

	first := Metadata{Title: "First Title"}
	if err := SetMetadata(path, first); err != nil {
		t.Fatalf("first SetMetadata failed: %v", err)
	}

	second := Metadata{Title: "Second Title", Description: "Replaced"}
	if err := SetMetadata(path, second); err != nil {
		t.Fatalf("second SetMetadata failed: %v", err)
	}

	xmp := readXMPFromJPEG(t, path)
	if strings.Contains(xmp, "First") {
		t.Errorf("old XMP not replaced, still contains 'First': %s", xmp)
	}
	if !strings.Contains(xmp, "Second Title") {
		t.Errorf("new XMP not written, missing 'Second Title': %s", xmp)
	}
}

func TestSetMetadata_NonJPEGError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "not-a-jpeg.jpg")
	original := []byte("this is not a jpeg file at all")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}

	err := SetMetadata(path, Metadata{Title: "Test"})
	if err == nil {
		t.Fatal("expected error for non-JPEG file")
	}

	after, _ := os.ReadFile(path)
	if string(after) != string(original) {
		t.Error("non-JPEG file was modified on error")
	}
}

func TestSetMetadata_PreservesFilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jpg")
	writeMinimalJPEG(t, path)

	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}

	if err := SetMetadata(path, Metadata{Title: "Test"}); err != nil {
		t.Fatalf("SetMetadata failed: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	got := info.Mode().Perm()
	if got != 0600 {
		t.Errorf("file permissions = %o, want 0600", got)
	}
}

func TestSetMetadata_UnicodeContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jpg")
	writeMinimalJPEG(t, path)

	meta := Metadata{
		Title:       "日本語タイトル",
		Description: "中文描述 & émojis: café",
		Tags:        []string{"日本", "中国", "한국"},
	}
	if err := SetMetadata(path, meta); err != nil {
		t.Fatalf("SetMetadata failed: %v", err)
	}

	xmp := readXMPFromJPEG(t, path)
	if !strings.Contains(xmp, "日本語タイトル") {
		t.Errorf("XMP missing CJK title, got: %s", xmp)
	}
	if !strings.Contains(xmp, "中文描述") {
		t.Errorf("XMP missing Chinese description, got: %s", xmp)
	}
	if !strings.Contains(xmp, "<rdf:li>한국</rdf:li>") {
		t.Errorf("XMP missing Korean tag, got: %s", xmp)
	}
}

func TestSetMetadata_VeryLargeMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jpg")
	writeMinimalJPEG(t, path)

	// Build metadata that approaches the 65533-byte APP1 limit.
	// The XMP namespace header is 29 bytes + null = 30 bytes, plus 2-byte length field.
	// So usable XMP payload must be under 65533 - 30 = 65503 bytes.
	// Build a large title that keeps the total payload just under the limit.
	largeTitle := strings.Repeat("A", 60000)
	meta := Metadata{
		Title: largeTitle,
	}
	if err := SetMetadata(path, meta); err != nil {
		t.Fatalf("SetMetadata failed for large metadata: %v", err)
	}

	xmp := readXMPFromJPEG(t, path)
	if !strings.Contains(xmp, largeTitle) {
		t.Error("XMP missing large title content")
	}
}

func TestSetMetadata_MetadataExceedsLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jpg")
	writeMinimalJPEG(t, path)

	// Build metadata that exceeds the 65533-byte APP1 limit.
	// XMP namespace is 30 bytes, XMP boilerplate is ~300 bytes, so a 66000-char title
	// will push the total well over 65535 bytes.
	hugeTitle := strings.Repeat("X", 66000)
	meta := Metadata{
		Title: hugeTitle,
	}
	err := SetMetadata(path, meta)
	if err == nil {
		t.Fatal("expected error for metadata exceeding APP1 size limit")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("error should mention 'too large', got: %v", err)
	}
}

func TestSetMetadata_NonexistentFile(t *testing.T) {
	err := SetMetadata("/nonexistent/path/test.jpg", Metadata{Title: "Test"})
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}
