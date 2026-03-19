package jpegmeta

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadDateNoExif(t *testing.T) {
	tmp := t.TempDir()
	path := tmp + "/test.jpg"
	if err := os.WriteFile(path, []byte{0xFF, 0xD8, 0xFF, 0xD9}, 0644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadDate(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.IsZero() {
		t.Errorf("expected zero time for JPEG without EXIF, got %v", got)
	}
}

func TestReadDateNonExistentFile(t *testing.T) {
	_, err := ReadDate("/nonexistent/file.jpg")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestReadDateWithDateTimeOriginal(t *testing.T) {
	jpeg := buildJPEGWithExifDate(t, exifDateTimeOriginal, "2020:06:15 14:30:00")
	path := filepath.Join(t.TempDir(), "test.jpg")
	if err := os.WriteFile(path, jpeg, 0644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadDate(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2020, 6, 15, 14, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("ReadDate() = %v, want %v", got, want)
	}
}

func TestReadDateWithDateTimeFallback(t *testing.T) {
	jpeg := buildJPEGWithExifDate(t, exifDateTime, "2019:03:10 08:15:00")
	path := filepath.Join(t.TempDir(), "test.jpg")
	if err := os.WriteFile(path, jpeg, 0644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadDate(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// goexif's DateTime() returns local time; compare date/time components
	if got.Year() != 2019 || got.Month() != 3 || got.Day() != 10 ||
		got.Hour() != 8 || got.Minute() != 15 || got.Second() != 0 {
		t.Errorf("ReadDate() = %v, want 2019-03-10 08:15:00", got)
	}
}

func TestReadDate_MalformedExif(t *testing.T) {
	// Build a JPEG with an APP1 segment that has the Exif header but garbage TIFF data.
	var jpeg []byte
	jpeg = append(jpeg, 0xFF, 0xD8) // SOI

	// APP1 marker with Exif header + garbage
	payload := []byte("Exif\x00\x00")
	payload = append(payload, []byte("THIS_IS_NOT_VALID_TIFF_DATA_AT_ALL_GARBAGE")...)
	segLen := uint16(len(payload) + 2)
	jpeg = append(jpeg, 0xFF, 0xE1)
	jpeg = append(jpeg, byte(segLen>>8), byte(segLen))
	jpeg = append(jpeg, payload...)
	jpeg = append(jpeg, 0xFF, 0xD9) // EOI

	path := filepath.Join(t.TempDir(), "malformed.jpg")
	if err := os.WriteFile(path, jpeg, 0644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadDate(path)
	if err != nil {
		t.Fatalf("expected no error for malformed EXIF, got: %v", err)
	}
	if !got.IsZero() {
		t.Errorf("expected zero time for malformed EXIF, got %v", got)
	}
}

// EXIF tag IDs
const (
	exifDateTimeOriginal = 0x9003
	exifDateTime         = 0x0132
)

// buildJPEGWithExifDate constructs a minimal valid JPEG with an EXIF APP1
// segment containing a single date tag. The date string must be in EXIF
// format: "YYYY:MM:DD HH:MM:SS" (19 chars + null terminator).
func buildJPEGWithExifDate(t *testing.T, tagID uint16, dateStr string) []byte {
	t.Helper()

	// EXIF date values are 20 bytes (19 chars + null terminator)
	dateVal := make([]byte, 20)
	copy(dateVal, dateStr)

	// For DateTimeOriginal (0x9003), it lives in the Exif IFD, not IFD0.
	// For DateTime (0x0132), it lives in IFD0.
	// Build the appropriate TIFF structure.

	if tagID == exifDateTime {
		return buildJPEGWithIFD0Date(t, tagID, dateVal)
	}
	return buildJPEGWithExifSubIFDDate(t, tagID, dateVal)
}

// buildJPEGWithIFD0Date builds a JPEG with a date tag in IFD0.
func buildJPEGWithIFD0Date(t *testing.T, tagID uint16, dateVal []byte) []byte {
	t.Helper()

	// TIFF header (little-endian) + IFD0 with one entry
	// Layout: TIFF header (8) + IFD0 entry count (2) + IFD entry (12) + next IFD offset (4) + date value (20)
	tiff := make([]byte, 0, 128)

	// TIFF header: byte order (II = little-endian), magic 42, offset to IFD0
	tiff = append(tiff, 'I', 'I')                         // byte order
	tiff = binary.LittleEndian.AppendUint16(tiff, 0x002A) // magic
	tiff = binary.LittleEndian.AppendUint32(tiff, 8)      // offset to IFD0

	// IFD0: 1 entry
	tiff = binary.LittleEndian.AppendUint16(tiff, 1) // entry count

	// IFD entry: tag, type=ASCII(2), count=20, value offset
	valueOffset := uint32(8 + 2 + 12 + 4) // after IFD0
	tiff = binary.LittleEndian.AppendUint16(tiff, tagID)
	tiff = binary.LittleEndian.AppendUint16(tiff, 2)  // ASCII type
	tiff = binary.LittleEndian.AppendUint32(tiff, 20)  // count (bytes)
	tiff = binary.LittleEndian.AppendUint32(tiff, valueOffset)

	// Next IFD offset (0 = no more IFDs)
	tiff = binary.LittleEndian.AppendUint32(tiff, 0)

	// Date value
	tiff = append(tiff, dateVal...)

	return wrapTIFFInJPEG(tiff)
}

// buildJPEGWithExifSubIFDDate builds a JPEG with a date tag in the Exif sub-IFD.
func buildJPEGWithExifSubIFDDate(t *testing.T, tagID uint16, dateVal []byte) []byte {
	t.Helper()

	// Layout:
	// TIFF header (8)
	// IFD0: 1 entry pointing to Exif sub-IFD (2 + 12 + 4 = 18)
	// Exif sub-IFD: 1 entry with the date tag (2 + 12 + 4 = 18)
	// Date value (20)

	tiff := make([]byte, 0, 128)

	// TIFF header
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

	// Exif sub-IFD at offset 26
	dateValueOffset := exifSubIFDOffset + 2 + 12 + 4 // = 44
	tiff = binary.LittleEndian.AppendUint16(tiff, 1)
	tiff = binary.LittleEndian.AppendUint16(tiff, tagID)
	tiff = binary.LittleEndian.AppendUint16(tiff, 2)  // ASCII type
	tiff = binary.LittleEndian.AppendUint32(tiff, 20)  // count
	tiff = binary.LittleEndian.AppendUint32(tiff, dateValueOffset)

	// Next IFD offset
	tiff = binary.LittleEndian.AppendUint32(tiff, 0)

	// Date value at offset 44
	tiff = append(tiff, dateVal...)

	return wrapTIFFInJPEG(tiff)
}

// wrapTIFFInJPEG wraps TIFF data in a JPEG APP1 (Exif) segment.
func wrapTIFFInJPEG(tiff []byte) []byte {
	var payload []byte
	payload = append(payload, "Exif\x00\x00"...)
	payload = append(payload, tiff...)

	segLen := uint16(len(payload) + 2)

	var jpeg []byte
	jpeg = append(jpeg, 0xFF, 0xD8)                       // SOI
	jpeg = append(jpeg, 0xFF, 0xE1)                       // APP1 marker
	jpeg = append(jpeg, byte(segLen>>8), byte(segLen))    // segment length
	jpeg = append(jpeg, payload...)
	jpeg = append(jpeg, 0xFF, 0xD9)                       // EOI
	return jpeg
}
