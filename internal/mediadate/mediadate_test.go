package mediadate

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	gomp4 "github.com/abema/go-mp4"

	"github.com/briandeitte/photo-copy/internal/mp4meta"
)

func TestResolveDate_FallbackToModTime(t *testing.T) {
	tmp := t.TempDir()
	path := tmp + "/test.png"
	if err := os.WriteFile(path, []byte("fake png"), 0644); err != nil {
		t.Fatal(err)
	}
	want := time.Date(2022, 6, 15, 12, 0, 0, 0, time.Local)
	if err := os.Chtimes(path, want, want); err != nil {
		t.Fatal(err)
	}

	got := ResolveDate(path)
	if got.IsZero() {
		t.Fatal("expected non-zero time")
	}
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestResolveDate_NonExistentFile(t *testing.T) {
	got := ResolveDate("/nonexistent/file.jpg")
	if !got.IsZero() {
		t.Errorf("expected zero time for nonexistent file, got %v", got)
	}
}

func TestResolveDate_JPEG_WithExif(t *testing.T) {
	jpeg := buildJPEGWithExifDate(t, "2020:06:15 14:30:00")
	dir := t.TempDir()
	path := filepath.Join(dir, "photo.jpg")
	if err := os.WriteFile(path, jpeg, 0644); err != nil {
		t.Fatal(err)
	}
	// Set mtime to a different time to confirm EXIF takes priority.
	mtime := time.Date(2000, 1, 1, 0, 0, 0, 0, time.Local)
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}

	got := ResolveDate(path)
	want := time.Date(2020, 6, 15, 14, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("ResolveDate() = %v, want %v", got, want)
	}
}

func TestResolveDate_MP4_WithCreationTime(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "video.mp4")
	writeMinimalMP4(t, path, 0)

	want := time.Date(2021, 3, 20, 10, 0, 0, 0, time.UTC)
	if err := mp4meta.SetCreationTime(path, want); err != nil {
		t.Fatalf("SetCreationTime failed: %v", err)
	}
	// Set mtime to a different time to confirm MP4 metadata takes priority.
	mtime := time.Date(2000, 1, 1, 0, 0, 0, 0, time.Local)
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}

	got := ResolveDate(path)
	if !got.Equal(want) {
		t.Errorf("ResolveDate() = %v, want %v", got, want)
	}
}

func TestResolveDate_JPEG_NoExif_FallsBackToMtime(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bare.jpg")
	// Minimal JPEG: SOI + EOI, no EXIF.
	if err := os.WriteFile(path, []byte{0xFF, 0xD8, 0xFF, 0xD9}, 0644); err != nil {
		t.Fatal(err)
	}
	want := time.Date(2019, 8, 5, 16, 45, 0, 0, time.Local)
	if err := os.Chtimes(path, want, want); err != nil {
		t.Fatal(err)
	}

	got := ResolveDate(path)
	if !got.Equal(want) {
		t.Errorf("ResolveDate() = %v, want %v (expected mtime fallback)", got, want)
	}
}

func TestResolveDate_UnknownExtension_UsesMtime(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "image.webp")
	if err := os.WriteFile(path, []byte("fake webp data"), 0644); err != nil {
		t.Fatal(err)
	}
	want := time.Date(2023, 12, 25, 8, 0, 0, 0, time.Local)
	if err := os.Chtimes(path, want, want); err != nil {
		t.Fatal(err)
	}

	got := ResolveDate(path)
	if !got.Equal(want) {
		t.Errorf("ResolveDate() = %v, want %v (expected mtime for .webp)", got, want)
	}
}

// --- Test helpers ---

// buildJPEGWithExifDate constructs a minimal JPEG with EXIF DateTimeOriginal.
func buildJPEGWithExifDate(t *testing.T, dateStr string) []byte {
	t.Helper()

	dateVal := make([]byte, 20)
	copy(dateVal, dateStr)

	// Build TIFF with IFD0 -> ExifSubIFD containing DateTimeOriginal (0x9003).
	tiff := make([]byte, 0, 128)

	// TIFF header (little-endian)
	tiff = append(tiff, 'I', 'I')
	tiff = binary.LittleEndian.AppendUint16(tiff, 0x002A)
	tiff = binary.LittleEndian.AppendUint32(tiff, 8) // offset to IFD0

	exifSubIFDOffset := uint32(8 + 2 + 12 + 4) // = 26

	// IFD0: 1 entry (ExifIFDPointer tag 0x8769)
	tiff = binary.LittleEndian.AppendUint16(tiff, 1)
	tiff = binary.LittleEndian.AppendUint16(tiff, 0x8769) // ExifIFD tag
	tiff = binary.LittleEndian.AppendUint16(tiff, 4)      // LONG type
	tiff = binary.LittleEndian.AppendUint32(tiff, 1)      // count
	tiff = binary.LittleEndian.AppendUint32(tiff, exifSubIFDOffset)

	// Next IFD offset
	tiff = binary.LittleEndian.AppendUint32(tiff, 0)

	// Exif sub-IFD
	dateValueOffset := exifSubIFDOffset + 2 + 12 + 4 // = 44
	tiff = binary.LittleEndian.AppendUint16(tiff, 1)
	tiff = binary.LittleEndian.AppendUint16(tiff, 0x9003) // DateTimeOriginal
	tiff = binary.LittleEndian.AppendUint16(tiff, 2)      // ASCII type
	tiff = binary.LittleEndian.AppendUint32(tiff, 20)     // count
	tiff = binary.LittleEndian.AppendUint32(tiff, dateValueOffset)

	// Next IFD offset
	tiff = binary.LittleEndian.AppendUint32(tiff, 0)

	// Date value
	tiff = append(tiff, dateVal...)

	// Wrap in JPEG APP1
	var payload []byte
	payload = append(payload, "Exif\x00\x00"...)
	payload = append(payload, tiff...)

	segLen := uint16(len(payload) + 2)

	var jpeg []byte
	jpeg = append(jpeg, 0xFF, 0xD8)                    // SOI
	jpeg = append(jpeg, 0xFF, 0xE1)                    // APP1 marker
	jpeg = append(jpeg, byte(segLen>>8), byte(segLen)) // segment length
	jpeg = append(jpeg, payload...)
	jpeg = append(jpeg, 0xFF, 0xD9) // EOI
	return jpeg
}

// writeMinimalMP4 creates a minimal valid MP4 file with ftyp + moov(mvhd + trak(tkhd + mdia(mdhd))).
func writeMinimalMP4(t *testing.T, path string, version uint8) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	w := gomp4.NewWriter(f)

	// ftyp box
	ftyp, err := w.StartBox(&gomp4.BoxInfo{Type: gomp4.BoxTypeFtyp()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = gomp4.Marshal(w, &gomp4.Ftyp{
		MajorBrand:   [4]byte{'i', 's', 'o', 'm'},
		MinorVersion: 0x200,
	}, ftyp.Context)
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.EndBox()
	if err != nil {
		t.Fatal(err)
	}

	// moov box
	_, err = w.StartBox(&gomp4.BoxInfo{Type: gomp4.BoxTypeMoov()})
	if err != nil {
		t.Fatal(err)
	}

	// mvhd box
	mvhdBI, err := w.StartBox(&gomp4.BoxInfo{Type: gomp4.BoxTypeMvhd()})
	if err != nil {
		t.Fatal(err)
	}
	mvhd := &gomp4.Mvhd{
		FullBox:     gomp4.FullBox{Version: version},
		Rate:        0x00010000,
		Volume:      0x0100,
		NextTrackID: 2,
	}
	mvhd.Timescale = 1000
	_, err = gomp4.Marshal(w, mvhd, mvhdBI.Context)
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.EndBox()
	if err != nil {
		t.Fatal(err)
	}

	// trak box
	_, err = w.StartBox(&gomp4.BoxInfo{Type: gomp4.BoxTypeTrak()})
	if err != nil {
		t.Fatal(err)
	}

	// tkhd box
	tkhdBI, err := w.StartBox(&gomp4.BoxInfo{Type: gomp4.BoxTypeTkhd()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = gomp4.Marshal(w, &gomp4.Tkhd{
		FullBox: gomp4.FullBox{Version: version, Flags: [3]byte{0, 0, 3}},
		TrackID: 1,
	}, tkhdBI.Context)
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.EndBox()
	if err != nil {
		t.Fatal(err)
	}

	// mdia box
	_, err = w.StartBox(&gomp4.BoxInfo{Type: gomp4.BoxTypeMdia()})
	if err != nil {
		t.Fatal(err)
	}

	// mdhd box
	mdhdBI, err := w.StartBox(&gomp4.BoxInfo{Type: gomp4.BoxTypeMdhd()})
	if err != nil {
		t.Fatal(err)
	}
	mdhd := &gomp4.Mdhd{
		FullBox: gomp4.FullBox{Version: version},
	}
	mdhd.Timescale = 1000
	_, err = gomp4.Marshal(w, mdhd, mdhdBI.Context)
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.EndBox()
	if err != nil {
		t.Fatal(err)
	}

	// close mdia, trak, moov
	for range 3 {
		_, err = w.EndBox()
		if err != nil {
			t.Fatal(err)
		}
	}
}
