package mp4meta

import (
	"bytes"
	"encoding/binary"
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

		headerSize := 8
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
			headerSize = 16
		}

		if boxSize < headerSize || pos+boxSize > len(data) {
			break
		}

		if boxType == "uuid" && pos+headerSize+16 <= pos+boxSize {
			uuid := data[pos+headerSize : pos+headerSize+16]
			if bytes.Equal(uuid, xmpUUID) {
				return string(data[pos+headerSize+16 : pos+boxSize])
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

// writeMinimalMP4WithMdat creates a minimal valid MP4 with ftyp + moov + mdat.
// The mdat box contains fake media data to simulate a real file.
func writeMinimalMP4WithMdat(t *testing.T, path string, version uint8) {
	t.Helper()

	// First write the normal minimal MP4 (ftyp + moov)
	writeMinimalMP4(t, path, version)

	// Append an mdat box with some payload data.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	// mdat box: 8-byte header + payload
	mdatPayload := []byte("fake video frame data that should survive metadata writes intact")
	mdatSize := uint32(8 + len(mdatPayload))
	var header [8]byte
	header[0] = byte(mdatSize >> 24)
	header[1] = byte(mdatSize >> 16)
	header[2] = byte(mdatSize >> 8)
	header[3] = byte(mdatSize)
	copy(header[4:], "mdat")
	if _, err := f.Write(header[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(mdatPayload); err != nil {
		t.Fatal(err)
	}
}

// TestSetXMPThenCreationTime_PreservesPlayback tests the exact sequence used
// in the Flickr download loop: SetXMPMetadata first, then SetCreationTime.
// This is the opposite of TestSetXMPMetadata_AfterSetCreationTime.
func TestSetXMPThenCreationTime_PreservesPlayback(t *testing.T) {
	dir := t.TempDir()
	mp4Path := filepath.Join(dir, "test.mp4")
	writeMinimalMP4WithMdat(t, mp4Path, 0)

	// Read original mdat content for comparison
	origData, err := os.ReadFile(mp4Path)
	if err != nil {
		t.Fatal(err)
	}
	origMdat := extractMdatPayload(t, origData)
	if len(origMdat) == 0 {
		t.Fatal("test setup: mdat payload not found in original file")
	}

	// Step 1: SetXMPMetadata (as in download loop)
	meta := XMPMetadata{
		Title:       "Beach Sunset",
		Description: "A lovely sunset at the beach",
		Tags:        []string{"sunset", "beach", "ocean"},
	}
	if err := SetXMPMetadata(mp4Path, meta); err != nil {
		t.Fatalf("SetXMPMetadata failed: %v", err)
	}

	// Step 2: SetCreationTime (as in download loop)
	targetTime := time.Date(2020, 6, 15, 14, 30, 0, 0, time.UTC)
	if err := SetCreationTime(mp4Path, targetTime); err != nil {
		t.Fatalf("SetCreationTime failed: %v", err)
	}

	// Verify mdat payload survived both operations
	resultData, err := os.ReadFile(mp4Path)
	if err != nil {
		t.Fatal(err)
	}
	resultMdat := extractMdatPayload(t, resultData)
	if !bytes.Equal(origMdat, resultMdat) {
		t.Errorf("mdat payload corrupted!\n  original: %q\n  result:   %q", string(origMdat), string(resultMdat))
	}

	// Verify timestamps were set
	expectedMP4Time := uint32(uint64(targetTime.Unix()) + mp4Epoch)
	verifyTimestampsV0(t, mp4Path, expectedMP4Time)

	// Verify XMP metadata is present
	xmp := readXMPFromMP4(t, mp4Path)
	if xmp == "" {
		t.Fatal("XMP UUID box not found after both operations")
	}
	if !strings.Contains(xmp, "Beach Sunset") {
		t.Errorf("XMP missing title, got: %s", xmp)
	}
}

// extractMdatPayload finds the mdat box and returns its payload (excluding header).
func extractMdatPayload(t *testing.T, data []byte) []byte {
	t.Helper()
	pos := 0
	for pos+8 <= len(data) {
		boxSize := int(data[pos])<<24 | int(data[pos+1])<<16 | int(data[pos+2])<<8 | int(data[pos+3])
		boxType := string(data[pos+4 : pos+8])
		if boxSize == 0 {
			boxSize = len(data) - pos
		}
		if boxSize < 8 || pos+boxSize > len(data) {
			break
		}
		if boxType == "mdat" {
			return data[pos+8 : pos+boxSize]
		}
		pos += boxSize
	}
	return nil
}

// writeMP4WithStco creates a minimal MP4 with ftyp + moov (containing stco with
// absolute offsets) + mdat. This simulates a real "fast-start" MP4 where moov
// comes before mdat, which is common for Flickr videos.
func writeMP4WithStco(t *testing.T, path string) {
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
	if _, err = w.EndBox(); err != nil {
		t.Fatal(err)
	}

	// moov box
	if _, err = w.StartBox(&gomp4.BoxInfo{Type: gomp4.BoxTypeMoov()}); err != nil {
		t.Fatal(err)
	}

	// mvhd box
	mvhdBI, err := w.StartBox(&gomp4.BoxInfo{Type: gomp4.BoxTypeMvhd()})
	if err != nil {
		t.Fatal(err)
	}
	mvhd := &gomp4.Mvhd{Rate: 0x00010000, Volume: 0x0100, NextTrackID: 2}
	mvhd.Timescale = 1000
	if _, err = gomp4.Marshal(w, mvhd, mvhdBI.Context); err != nil {
		t.Fatal(err)
	}
	if _, err = w.EndBox(); err != nil {
		t.Fatal(err)
	}

	// trak box
	if _, err = w.StartBox(&gomp4.BoxInfo{Type: gomp4.BoxTypeTrak()}); err != nil {
		t.Fatal(err)
	}

	// tkhd box
	tkhdBI, err := w.StartBox(&gomp4.BoxInfo{Type: gomp4.BoxTypeTkhd()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = gomp4.Marshal(w, &gomp4.Tkhd{
		FullBox: gomp4.FullBox{Flags: [3]byte{0, 0, 3}},
		TrackID: 1,
	}, tkhdBI.Context); err != nil {
		t.Fatal(err)
	}
	if _, err = w.EndBox(); err != nil {
		t.Fatal(err)
	}

	// mdia box
	if _, err = w.StartBox(&gomp4.BoxInfo{Type: gomp4.BoxTypeMdia()}); err != nil {
		t.Fatal(err)
	}

	// mdhd box
	mdhdBI, err := w.StartBox(&gomp4.BoxInfo{Type: gomp4.BoxTypeMdhd()})
	if err != nil {
		t.Fatal(err)
	}
	mdhd := &gomp4.Mdhd{}
	mdhd.Timescale = 1000
	if _, err = gomp4.Marshal(w, mdhd, mdhdBI.Context); err != nil {
		t.Fatal(err)
	}
	if _, err = w.EndBox(); err != nil {
		t.Fatal(err)
	}

	// minf → stbl → stco (sample table with chunk offsets)
	if _, err = w.StartBox(&gomp4.BoxInfo{Type: gomp4.BoxTypeMinf()}); err != nil {
		t.Fatal(err)
	}
	if _, err = w.StartBox(&gomp4.BoxInfo{Type: gomp4.BoxTypeStbl()}); err != nil {
		t.Fatal(err)
	}
	stcoBI, err := w.StartBox(&gomp4.BoxInfo{Type: gomp4.BoxTypeStco()})
	if err != nil {
		t.Fatal(err)
	}
	// We'll write a placeholder offset; we'll fix it after we know the mdat position.
	// For now, write a stco with 1 entry pointing to offset 0 (will be fixed below).
	if _, err = gomp4.Marshal(w, &gomp4.Stco{
		EntryCount:  1,
		ChunkOffset: []uint32{0}, // placeholder
	}, stcoBI.Context); err != nil {
		t.Fatal(err)
	}
	if _, err = w.EndBox(); err != nil {
		t.Fatal(err)
	}

	// close stbl, minf, mdia, trak, moov
	for range 5 {
		if _, err = w.EndBox(); err != nil {
			t.Fatal(err)
		}
	}

	// Flush writer and get current file position (= start of mdat)
	if err := f.Sync(); err != nil {
		t.Fatal(err)
	}
	mdatOffset, err := f.Seek(0, 1)
	if err != nil {
		t.Fatal(err)
	}

	// Write mdat box manually
	mdatPayload := []byte("REAL_VIDEO_FRAME_DATA_12345678")
	mdatSize := uint32(8 + len(mdatPayload))
	var header [8]byte
	binary.BigEndian.PutUint32(header[0:4], mdatSize)
	copy(header[4:8], "mdat")
	if _, err = f.Write(header[:]); err != nil {
		t.Fatal(err)
	}
	if _, err = f.Write(mdatPayload); err != nil {
		t.Fatal(err)
	}

	// Now fix the stco offset to point to the actual mdat data (after mdat header)
	stcoChunkOffset := uint32(mdatOffset) + 8 // skip mdat box header

	// Re-read file to find and patch stco
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Find stco box by scanning for "stco" type signature and patch the chunk offset
	patched := false
	for i := 0; i+20 <= len(data); i++ {
		if string(data[i+4:i+8]) == "stco" {
			bSize := int(binary.BigEndian.Uint32(data[i : i+4]))
			if bSize >= 20 && i+bSize <= len(data) {
				// stco layout: header(8) + version/flags(4) + entry_count(4) + offsets(4 each)
				binary.BigEndian.PutUint32(data[i+16:], stcoChunkOffset)
				patched = true
				break
			}
		}
	}
	if !patched {
		t.Fatal("could not find stco box to patch")
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

// TestSetXMPMetadata_PreservesStcoOffsets verifies that inserting an XMP UUID
// box does not corrupt stco chunk offsets. If the UUID box is inserted between
// moov and mdat, it shifts mdat but the stco offsets still point to the old
// position, causing playback failure.
func TestSetXMPMetadata_PreservesStcoOffsets(t *testing.T) {
	dir := t.TempDir()
	mp4Path := filepath.Join(dir, "test.mp4")
	writeMP4WithStco(t, mp4Path)

	// Read original stco offset and mdat payload
	origData, err := os.ReadFile(mp4Path)
	if err != nil {
		t.Fatal(err)
	}
	origOffset := findStcoOffset(t, origData)
	origMdat := extractMdatPayload(t, origData)
	if len(origMdat) == 0 {
		t.Fatal("test setup: no mdat payload found")
	}

	// Verify original offset points to correct data
	if int(origOffset)+len(origMdat) > len(origData) {
		t.Fatalf("original stco offset %d out of range (file size %d)", origOffset, len(origData))
	}
	origChunkData := origData[origOffset : origOffset+uint32(len(origMdat))]
	if !bytes.Equal(origChunkData, origMdat) {
		t.Fatal("test setup: stco offset doesn't point to mdat payload")
	}

	// Apply XMP metadata
	meta := XMPMetadata{Title: "Test Title", Tags: []string{"tag1"}}
	if err := SetXMPMetadata(mp4Path, meta); err != nil {
		t.Fatalf("SetXMPMetadata failed: %v", err)
	}

	// Read result and verify stco offset still points to correct data
	resultData, err := os.ReadFile(mp4Path)
	if err != nil {
		t.Fatal(err)
	}
	resultOffset := findStcoOffset(t, resultData)
	resultMdat := extractMdatPayload(t, resultData)

	if !bytes.Equal(origMdat, resultMdat) {
		t.Error("mdat payload was corrupted")
	}

	// The critical check: does stco still point to the right data?
	if int(resultOffset)+len(resultMdat) > len(resultData) {
		t.Fatalf("stco offset %d out of range after SetXMPMetadata (file size %d)", resultOffset, len(resultData))
	}
	resultChunkData := resultData[resultOffset : resultOffset+uint32(len(resultMdat))]
	if !bytes.Equal(resultChunkData, resultMdat) {
		t.Errorf("stco offset broken after SetXMPMetadata: offset %d no longer points to mdat payload\n"+
			"  expected data at offset: %q\n"+
			"  actual data at offset:   %q\n"+
			"  (original offset was %d, now %d — diff = %d bytes = likely UUID box size)",
			resultOffset, string(resultMdat), string(resultChunkData),
			origOffset, resultOffset, int(resultOffset)-int(origOffset))
	}
}

// findStcoOffset scans for the stco box and returns the first chunk offset value.
func findStcoOffset(t *testing.T, data []byte) uint32 {
	t.Helper()
	// Scan byte-by-byte for "stco" signature
	for i := 0; i+12 <= len(data); i++ {
		if string(data[i+4:i+8]) == "stco" {
			boxSize := int(binary.BigEndian.Uint32(data[i : i+4]))
			if boxSize >= 20 && i+boxSize <= len(data) {
				// stco: header(8) + version/flags(4) + entry_count(4) + offset(4)
				return binary.BigEndian.Uint32(data[i+16 : i+20])
			}
		}
	}
	t.Fatal("stco box not found")
	return 0
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
