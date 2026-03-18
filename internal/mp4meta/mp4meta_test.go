package mp4meta

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gomp4 "github.com/abema/go-mp4"
)

// writeMinimalMP4 creates a minimal valid MP4 file with ftyp + moov(mvhd + trak(tkhd + mdia(mdhd))).
// version controls whether V0 (32-bit) or V1 (64-bit) timestamp boxes are used.
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

func TestSetCreationTime_Version0(t *testing.T) {
	dir := t.TempDir()
	mp4Path := filepath.Join(dir, "test.mp4")
	writeMinimalMP4(t, mp4Path, 0)

	targetTime := time.Date(2020, 6, 15, 14, 30, 0, 0, time.UTC)
	if err := SetCreationTime(mp4Path, targetTime); err != nil {
		t.Fatalf("SetCreationTime failed: %v", err)
	}

	expectedMP4Time := uint32(uint64(targetTime.Unix()) + mp4Epoch)
	verifyTimestampsV0(t, mp4Path, expectedMP4Time)
}

func TestSetCreationTime_Version1(t *testing.T) {
	dir := t.TempDir()
	mp4Path := filepath.Join(dir, "test.mp4")
	writeMinimalMP4(t, mp4Path, 1)

	targetTime := time.Date(2020, 6, 15, 14, 30, 0, 0, time.UTC)
	if err := SetCreationTime(mp4Path, targetTime); err != nil {
		t.Fatalf("SetCreationTime failed: %v", err)
	}

	expectedMP4Time := uint64(targetTime.Unix()) + mp4Epoch
	verifyTimestampsV1(t, mp4Path, expectedMP4Time)
}

func TestSetCreationTime_NonMP4(t *testing.T) {
	dir := t.TempDir()
	txtPath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(txtPath, []byte("not an mp4"), 0644); err != nil {
		t.Fatal(err)
	}

	originalData, _ := os.ReadFile(txtPath)

	err := SetCreationTime(txtPath, time.Now())
	if err == nil {
		t.Fatal("expected error for non-MP4 file")
	}

	afterData, _ := os.ReadFile(txtPath)
	if !bytes.Equal(originalData, afterData) {
		t.Error("original file was modified on error")
	}
}

func TestSetCreationTime_NonexistentFile(t *testing.T) {
	err := SetCreationTime("/nonexistent/path/test.mp4", time.Now())
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestSetCreationTime_PreEpochDate(t *testing.T) {
	preEpoch := time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	mp4Path := filepath.Join(dir, "test.mp4")
	writeMinimalMP4(t, mp4Path, 0)

	err := SetCreationTime(mp4Path, preEpoch)
	if err == nil {
		t.Fatal("expected error for pre-epoch date")
	}
}

func TestSetCreationTime_V0Overflow(t *testing.T) {
	// A date far enough in the future to overflow uint32 MP4 time.
	// uint32 max = 4294967295 seconds from 1904 ≈ year 2040.
	futureDate := time.Date(2050, 1, 1, 0, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	mp4Path := filepath.Join(dir, "test.mp4")
	writeMinimalMP4(t, mp4Path, 0)

	err := SetCreationTime(mp4Path, futureDate)
	if err == nil {
		t.Fatal("expected error for V0 overflow")
	}
}

func TestSetCreationTime_V1NoOverflow(t *testing.T) {
	// Same future date should succeed with V1 (64-bit) boxes.
	futureDate := time.Date(2050, 1, 1, 0, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	mp4Path := filepath.Join(dir, "test.mp4")
	writeMinimalMP4(t, mp4Path, 1)

	if err := SetCreationTime(mp4Path, futureDate); err != nil {
		t.Fatalf("V1 should handle future dates: %v", err)
	}

	expectedMP4Time := uint64(futureDate.Unix()) + mp4Epoch
	verifyTimestampsV1(t, mp4Path, expectedMP4Time)
}

// readXMPFromMP4 walks top-level boxes looking for a UUID box with the XMP UUID.
// Returns the XMP content string, or empty string if not found.
func readXMPFromMP4(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readXMPFromMP4: reading file: %v", err)
	}

	xmpUUID := []byte{0xBE, 0x7A, 0xCF, 0xCB, 0x97, 0xA9, 0x42, 0xE8, 0x9C, 0x71, 0x99, 0x94, 0x91, 0xE3, 0xAF, 0xAC}
	pos := 0
	for pos+8 <= len(data) {
		boxSize := int(data[pos])<<24 | int(data[pos+1])<<16 | int(data[pos+2])<<8 | int(data[pos+3])
		boxType := string(data[pos+4 : pos+8])

		if boxSize == 0 {
			// extends to EOF
			boxSize = len(data) - pos
		} else if boxSize == 1 {
			// 64-bit extended size
			if pos+16 > len(data) {
				break
			}
			extSize := int(data[pos+8])<<56 | int(data[pos+9])<<48 | int(data[pos+10])<<40 | int(data[pos+11])<<32 |
				int(data[pos+12])<<24 | int(data[pos+13])<<16 | int(data[pos+14])<<8 | int(data[pos+15])
			boxSize = extSize
		}

		if boxSize < 8 || pos+boxSize > len(data) {
			break
		}

		if boxType == "uuid" && pos+8+16 <= len(data) {
			uuid := data[pos+8 : pos+24]
			if bytes.Equal(uuid, xmpUUID) {
				return string(data[pos+24 : pos+boxSize])
			}
		}

		pos += boxSize
	}
	return ""
}

func TestSetXMPMetadata_WritesUUIDBox(t *testing.T) {
	dir := t.TempDir()
	mp4Path := filepath.Join(dir, "test.mp4")
	writeMinimalMP4(t, mp4Path, 0)

	meta := XMPMetadata{
		Title:       "My Title",
		Description: "My Description",
		Tags:        []string{"tag1", "tag2"},
	}
	if err := SetXMPMetadata(mp4Path, meta); err != nil {
		t.Fatalf("SetXMPMetadata failed: %v", err)
	}

	xmp := readXMPFromMP4(t, mp4Path)
	if xmp == "" {
		t.Fatal("XMP UUID box not found in MP4")
	}
	if !strings.Contains(xmp, "My Title") {
		t.Errorf("XMP missing title, got: %s", xmp)
	}
	if !strings.Contains(xmp, "My Description") {
		t.Errorf("XMP missing description, got: %s", xmp)
	}
	if !strings.Contains(xmp, "tag1") {
		t.Errorf("XMP missing tag1, got: %s", xmp)
	}
	if !strings.Contains(xmp, "tag2") {
		t.Errorf("XMP missing tag2, got: %s", xmp)
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

	// Verify timestamps are still correct.
	expectedMP4Time := uint32(uint64(targetTime.Unix()) + mp4Epoch)
	verifyTimestampsV0(t, mp4Path, expectedMP4Time)

	// Verify XMP is present.
	xmp := readXMPFromMP4(t, mp4Path)
	if xmp == "" {
		t.Fatal("XMP UUID box not found after SetCreationTime+SetXMPMetadata")
	}
	if !strings.Contains(xmp, "After Timestamps") {
		t.Errorf("XMP missing title, got: %s", xmp)
	}
}

func TestSetXMPMetadata_ReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	mp4Path := filepath.Join(dir, "test.mp4")
	writeMinimalMP4(t, mp4Path, 0)

	first := XMPMetadata{Title: "First Title"}
	if err := SetXMPMetadata(mp4Path, first); err != nil {
		t.Fatalf("first SetXMPMetadata failed: %v", err)
	}

	second := XMPMetadata{Title: "Second Title"}
	if err := SetXMPMetadata(mp4Path, second); err != nil {
		t.Fatalf("second SetXMPMetadata failed: %v", err)
	}

	xmp := readXMPFromMP4(t, mp4Path)
	if !strings.Contains(xmp, "Second Title") {
		t.Errorf("XMP should contain second title, got: %s", xmp)
	}
	if strings.Contains(xmp, "First Title") {
		t.Errorf("XMP should not contain first title after replacement, got: %s", xmp)
	}
}

func TestSetXMPMetadata_NonMP4Error(t *testing.T) {
	dir := t.TempDir()
	txtPath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(txtPath, []byte("not an mp4"), 0644); err != nil {
		t.Fatal(err)
	}

	originalData, _ := os.ReadFile(txtPath)

	err := SetXMPMetadata(txtPath, XMPMetadata{Title: "Test"})
	if err == nil {
		t.Fatal("expected error for non-MP4 file")
	}

	afterData, _ := os.ReadFile(txtPath)
	if !bytes.Equal(originalData, afterData) {
		t.Error("original file was modified on error")
	}
}

func TestSetXMPMetadata_PreservesFilePermissions(t *testing.T) {
	dir := t.TempDir()
	mp4Path := filepath.Join(dir, "test.mp4")
	writeMinimalMP4(t, mp4Path, 0)

	if err := os.Chmod(mp4Path, 0600); err != nil {
		t.Fatal(err)
	}

	if err := SetXMPMetadata(mp4Path, XMPMetadata{Title: "Perms Test"}); err != nil {
		t.Fatalf("SetXMPMetadata failed: %v", err)
	}

	info, err := os.Stat(mp4Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("permissions = %o, want 0600", info.Mode().Perm())
	}
}

// verifyTimestampsV0 reads the MP4 and checks all version 0 (32-bit) timestamps.
func verifyTimestampsV0(t *testing.T, path string, expected uint32) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	var foundMvhd, foundTkhd, foundMdhd bool
	_, err = gomp4.ReadBoxStructure(f, func(h *gomp4.ReadHandle) (any, error) {
		switch h.BoxInfo.Type {
		case gomp4.BoxTypeMvhd():
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			mvhd := box.(*gomp4.Mvhd)
			if mvhd.CreationTimeV0 != expected {
				t.Errorf("mvhd CreationTimeV0 = %d, want %d", mvhd.CreationTimeV0, expected)
			}
			if mvhd.ModificationTimeV0 != expected {
				t.Errorf("mvhd ModificationTimeV0 = %d, want %d", mvhd.ModificationTimeV0, expected)
			}
			foundMvhd = true
		case gomp4.BoxTypeTkhd():
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			tkhd := box.(*gomp4.Tkhd)
			if tkhd.CreationTimeV0 != expected {
				t.Errorf("tkhd CreationTimeV0 = %d, want %d", tkhd.CreationTimeV0, expected)
			}
			if tkhd.ModificationTimeV0 != expected {
				t.Errorf("tkhd ModificationTimeV0 = %d, want %d", tkhd.ModificationTimeV0, expected)
			}
			foundTkhd = true
		case gomp4.BoxTypeMdhd():
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			mdhd := box.(*gomp4.Mdhd)
			if mdhd.CreationTimeV0 != expected {
				t.Errorf("mdhd CreationTimeV0 = %d, want %d", mdhd.CreationTimeV0, expected)
			}
			if mdhd.ModificationTimeV0 != expected {
				t.Errorf("mdhd ModificationTimeV0 = %d, want %d", mdhd.ModificationTimeV0, expected)
			}
			foundMdhd = true
		}
		return h.Expand()
	})
	// Tolerate errors from go-mp4 failing to parse unknown box types (e.g. uuid).
	// The important thing is whether we found the timestamp boxes we're looking for.
	if err != nil && !foundMvhd {
		t.Fatal(err)
	}
	if !foundMvhd {
		t.Error("mvhd box not found")
	}
	if !foundTkhd {
		t.Error("tkhd box not found")
	}
	if !foundMdhd {
		t.Error("mdhd box not found")
	}
}

// verifyTimestampsV1 reads the MP4 and checks all version 1 (64-bit) timestamps.
func verifyTimestampsV1(t *testing.T, path string, expected uint64) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	var foundMvhd, foundTkhd, foundMdhd bool
	_, err = gomp4.ReadBoxStructure(f, func(h *gomp4.ReadHandle) (any, error) {
		switch h.BoxInfo.Type {
		case gomp4.BoxTypeMvhd():
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			mvhd := box.(*gomp4.Mvhd)
			if mvhd.CreationTimeV1 != expected {
				t.Errorf("mvhd CreationTimeV1 = %d, want %d", mvhd.CreationTimeV1, expected)
			}
			if mvhd.ModificationTimeV1 != expected {
				t.Errorf("mvhd ModificationTimeV1 = %d, want %d", mvhd.ModificationTimeV1, expected)
			}
			foundMvhd = true
		case gomp4.BoxTypeTkhd():
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			tkhd := box.(*gomp4.Tkhd)
			if tkhd.CreationTimeV1 != expected {
				t.Errorf("tkhd CreationTimeV1 = %d, want %d", tkhd.CreationTimeV1, expected)
			}
			if tkhd.ModificationTimeV1 != expected {
				t.Errorf("tkhd ModificationTimeV1 = %d, want %d", tkhd.ModificationTimeV1, expected)
			}
			foundTkhd = true
		case gomp4.BoxTypeMdhd():
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			mdhd := box.(*gomp4.Mdhd)
			if mdhd.CreationTimeV1 != expected {
				t.Errorf("mdhd CreationTimeV1 = %d, want %d", mdhd.CreationTimeV1, expected)
			}
			if mdhd.ModificationTimeV1 != expected {
				t.Errorf("mdhd ModificationTimeV1 = %d, want %d", mdhd.ModificationTimeV1, expected)
			}
			foundMdhd = true
		}
		return h.Expand()
	})
	// Tolerate errors from go-mp4 failing to parse unknown box types (e.g. uuid).
	if err != nil && !foundMvhd {
		t.Fatal(err)
	}
	if !foundMvhd {
		t.Error("mvhd box not found")
	}
	if !foundTkhd {
		t.Error("tkhd box not found")
	}
	if !foundMdhd {
		t.Error("mdhd box not found")
	}
}
