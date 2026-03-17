package mp4meta

import (
	"bytes"
	"os"
	"path/filepath"
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
	if err != nil {
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
	if err != nil {
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
