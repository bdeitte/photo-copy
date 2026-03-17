# MP4 Creation Date Metadata Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Preserve original capture dates on Flickr-downloaded files by setting MP4 container metadata and file system timestamps.

**Architecture:** New `internal/mp4meta/` package handles MP4/MOV box rewriting using `abema/go-mp4`. Flickr package adds `date_taken`/`date_upload` to API requests and calls mp4meta + os.Chtimes after each download. Date parsing is an exported helper for testability.

**Tech Stack:** Go, `github.com/abema/go-mp4`

---

## File Structure

| Action | File | Responsibility |
|--------|------|----------------|
| Create | `internal/mp4meta/mp4meta.go` | `SetCreationTime()` — rewrite MP4/MOV boxes to set creation dates |
| Create | `internal/mp4meta/mp4meta_test.go` | Unit tests for SetCreationTime with minimal MP4 fixtures |
| Modify | `internal/flickr/flickr.go` | Add date fields to API request/response, call mp4meta + Chtimes after download |
| Create | `internal/flickr/dates.go` | `resolvePhotoDate()` — parse date_taken/date_upload into time.Time |
| Create | `internal/flickr/dates_test.go` | Unit tests for date parsing and fallback logic |
| Modify | `internal/cli/integration_test.go` | Add integration test for date preservation |

---

## Chunk 1: mp4meta package

### Task 1: Add go-mp4 dependency

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Add dependency**

```bash
go get github.com/abema/go-mp4
```

- [ ] **Step 2: Verify it resolves**

```bash
go mod tidy
```

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "Add abema/go-mp4 dependency for MP4 metadata editing"
```

### Task 2: Implement SetCreationTime with tests

**Files:**
- Create: `internal/mp4meta/mp4meta.go`
- Create: `internal/mp4meta/mp4meta_test.go`

- [ ] **Step 1: Write the implementation**

```go
// Package mp4meta provides utilities for editing MP4/MOV container metadata.
package mp4meta

import (
	"fmt"
	"os"
	"time"

	gomp4 "github.com/abema/go-mp4"
)

// mp4Epoch is the offset in seconds between Unix epoch (1970-01-01) and
// the MP4/QuickTime epoch (1904-01-01).
const mp4Epoch = 2082844800

// SetCreationTime sets the creation and modification timestamps in the
// mvhd, tkhd, and mdhd boxes of an MP4 or MOV file. It writes to a temp
// file and renames over the original on success.
func SetCreationTime(filePath string, t time.Time) error {
	mp4Time := uint64(t.Unix()) + mp4Epoch

	in, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("opening %s: %w", filePath, err)
	}
	defer in.Close()

	tmpPath := filePath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}

	w := gomp4.NewWriter(out)

	_, err = gomp4.ReadBoxStructure(in, func(h *gomp4.ReadHandle) (any, error) {
		switch h.BoxInfo.Type {
		case gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia():
			_, err := w.StartBox(&h.BoxInfo)
			if err != nil {
				return nil, err
			}
			val, err := h.Expand()
			if err != nil {
				return nil, err
			}
			_, err = w.EndBox()
			return val, err

		case gomp4.BoxTypeMvhd():
			return nil, rewriteMvhd(h, w, mp4Time)

		case gomp4.BoxTypeTkhd():
			return nil, rewriteTkhd(h, w, mp4Time)

		case gomp4.BoxTypeMdhd():
			return nil, rewriteMdhd(h, w, mp4Time)

		default:
			_, err := w.CopyBox(in, &h.BoxInfo)
			return nil, err
		}
	})

	closeErr := out.Close()
	in.Close()

	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("processing MP4: %w", err)
	}
	if closeErr != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", closeErr)
	}

	return os.Rename(tmpPath, filePath)
}

func rewriteMvhd(h *gomp4.ReadHandle, w *gomp4.Writer, mp4Time uint64) error {
	box, _, err := h.ReadPayload()
	if err != nil {
		return err
	}
	mvhd := box.(*gomp4.Mvhd)

	if mvhd.GetVersion() == 0 {
		mvhd.CreationTimeV0 = uint32(mp4Time)
		mvhd.ModificationTimeV0 = uint32(mp4Time)
	} else {
		mvhd.CreationTimeV1 = mp4Time
		mvhd.ModificationTimeV1 = mp4Time
	}

	bi, err := w.StartBox(&h.BoxInfo)
	if err != nil {
		return err
	}
	if _, err := gomp4.Marshal(w, mvhd, bi.Context); err != nil {
		return err
	}
	_, err = w.EndBox()
	return err
}

func rewriteTkhd(h *gomp4.ReadHandle, w *gomp4.Writer, mp4Time uint64) error {
	box, _, err := h.ReadPayload()
	if err != nil {
		return err
	}
	tkhd := box.(*gomp4.Tkhd)

	if tkhd.GetVersion() == 0 {
		tkhd.CreationTimeV0 = uint32(mp4Time)
		tkhd.ModificationTimeV0 = uint32(mp4Time)
	} else {
		tkhd.CreationTimeV1 = mp4Time
		tkhd.ModificationTimeV1 = mp4Time
	}

	bi, err := w.StartBox(&h.BoxInfo)
	if err != nil {
		return err
	}
	if _, err := gomp4.Marshal(w, tkhd, bi.Context); err != nil {
		return err
	}
	_, err = w.EndBox()
	return err
}

func rewriteMdhd(h *gomp4.ReadHandle, w *gomp4.Writer, mp4Time uint64) error {
	box, _, err := h.ReadPayload()
	if err != nil {
		return err
	}
	mdhd := box.(*gomp4.Mdhd)

	if mdhd.GetVersion() == 0 {
		mdhd.CreationTimeV0 = uint32(mp4Time)
		mdhd.ModificationTimeV0 = uint32(mp4Time)
	} else {
		mdhd.CreationTimeV1 = mp4Time
		mdhd.ModificationTimeV1 = mp4Time
	}

	bi, err := w.StartBox(&h.BoxInfo)
	if err != nil {
		return err
	}
	if _, err := gomp4.Marshal(w, mdhd, bi.Context); err != nil {
		return err
	}
	_, err = w.EndBox()
	return err
}
```

- [ ] **Step 2: Write test helpers and tests**

```go
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
	var buf bytes.Buffer
	w := gomp4.NewWriter(&buf)

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
	moov, err := w.StartBox(&gomp4.BoxInfo{Type: gomp4.BoxTypeMoov()})
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
	if version == 0 {
		mvhd.TimescaleV0 = 1000
	} else {
		mvhd.TimescaleV1 = 1000
	}
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
	if version == 0 {
		mdhd.TimescaleV0 = 1000
	} else {
		mdhd.TimescaleV1 = 1000
	}
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

	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
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
	defer f.Close()

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
	defer f.Close()

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
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/mp4meta/ -v
```

Expected: PASS

- [ ] **Step 4: Run linter**

```bash
golangci-lint run ./internal/mp4meta/
```

- [ ] **Step 5: Commit**

```bash
git add internal/mp4meta/
git commit -m "Add mp4meta package for setting MP4/MOV creation timestamps"
```

**Files:**
- Create: `internal/mp4meta/mp4meta.go`

- [ ] **Step 1: Implement SetCreationTime**

```go
// Package mp4meta provides utilities for editing MP4/MOV container metadata.
package mp4meta

import (
	"fmt"
	"io"
	"os"
	"time"

	gomp4 "github.com/abema/go-mp4"
)

// mp4Epoch is the offset in seconds between Unix epoch (1970-01-01) and
// the MP4/QuickTime epoch (1904-01-01).
const mp4Epoch = 2082844800

// SetCreationTime sets the creation and modification timestamps in the
// mvhd, tkhd, and mdhd boxes of an MP4 or MOV file. It writes to a temp
// file and renames over the original on success.
func SetCreationTime(filePath string, t time.Time) error {
	mp4Time := uint64(t.Unix()) + mp4Epoch

	in, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("opening %s: %w", filePath, err)
	}
	defer in.Close()

	tmpPath := filePath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}

	w := gomp4.NewWriter(out)
	var writeErr error

	_, err = gomp4.ReadBoxStructure(in, func(h *gomp4.ReadHandle) (any, error) {
		switch h.BoxInfo.Type {
		case gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia():
			_, err := w.StartBox(&h.BoxInfo)
			if err != nil {
				return nil, err
			}
			val, err := h.Expand()
			if err != nil {
				return nil, err
			}
			_, err = w.EndBox()
			return val, err

		case gomp4.BoxTypeMvhd():
			return nil, rewriteMvhd(h, w, mp4Time)

		case gomp4.BoxTypeTkhd():
			return nil, rewriteTkhd(h, w, mp4Time)

		case gomp4.BoxTypeMdhd():
			return nil, rewriteMdhd(h, w, mp4Time)

		default:
			_, err := w.CopyBox(in, &h.BoxInfo)
			return nil, err
		}
	})

	closeErr := out.Close()
	in.Close()

	if err != nil || writeErr != nil {
		os.Remove(tmpPath)
		if err != nil {
			return fmt.Errorf("processing MP4: %w", err)
		}
		return fmt.Errorf("writing MP4: %w", writeErr)
	}
	if closeErr != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", closeErr)
	}

	return os.Rename(tmpPath, filePath)
}

func rewriteMvhd(h *gomp4.ReadHandle, w *gomp4.Writer, mp4Time uint64) error {
	box, _, err := h.ReadPayload()
	if err != nil {
		return err
	}
	mvhd := box.(*gomp4.Mvhd)

	if mvhd.GetVersion() == 0 {
		mvhd.CreationTimeV0 = uint32(mp4Time)
		mvhd.ModificationTimeV0 = uint32(mp4Time)
	} else {
		mvhd.CreationTimeV1 = mp4Time
		mvhd.ModificationTimeV1 = mp4Time
	}

	bi, err := w.StartBox(&h.BoxInfo)
	if err != nil {
		return err
	}
	if _, err := gomp4.Marshal(w, mvhd, bi.Context); err != nil {
		return err
	}
	_, err = w.EndBox()
	return err
}

func rewriteTkhd(h *gomp4.ReadHandle, w *gomp4.Writer, mp4Time uint64) error {
	box, _, err := h.ReadPayload()
	if err != nil {
		return err
	}
	tkhd := box.(*gomp4.Tkhd)

	if tkhd.GetVersion() == 0 {
		tkhd.CreationTimeV0 = uint32(mp4Time)
		tkhd.ModificationTimeV0 = uint32(mp4Time)
	} else {
		tkhd.CreationTimeV1 = mp4Time
		tkhd.ModificationTimeV1 = mp4Time
	}

	bi, err := w.StartBox(&h.BoxInfo)
	if err != nil {
		return err
	}
	if _, err := gomp4.Marshal(w, tkhd, bi.Context); err != nil {
		return err
	}
	_, err = w.EndBox()
	return err
}

func rewriteMdhd(h *gomp4.ReadHandle, w *gomp4.Writer, mp4Time uint64) error {
	box, _, err := h.ReadPayload()
	if err != nil {
		return err
	}
	mdhd := box.(*gomp4.Mdhd)

	if mdhd.GetVersion() == 0 {
		mdhd.CreationTimeV0 = uint32(mp4Time)
		mdhd.ModificationTimeV0 = uint32(mp4Time)
	} else {
		mdhd.CreationTimeV1 = mp4Time
		mdhd.ModificationTimeV1 = mp4Time
	}

	bi, err := w.StartBox(&h.BoxInfo)
	if err != nil {
		return err
	}
	if _, err := gomp4.Marshal(w, mdhd, bi.Context); err != nil {
		return err
	}
	_, err = w.EndBox()
	return err
}
```

- [ ] **Step 2: Run tests to verify they pass**

```bash
go test ./internal/mp4meta/ -v
```

Expected: PASS

- [ ] **Step 3: Run linter**

```bash
golangci-lint run ./internal/mp4meta/
```

- [ ] **Step 4: Commit**

```bash
git add internal/mp4meta/
git commit -m "Implement mp4meta.SetCreationTime for MP4/MOV date metadata"
```

---

## Chunk 2: Flickr date parsing

### Task 3: Implement date resolution with tests

**Files:**
- Create: `internal/flickr/dates.go`
- Create: `internal/flickr/dates_test.go`

- [ ] **Step 1: Write the implementation**

```go
package flickr

import (
	"strconv"
	"time"
)

const flickrDateFormat = "2006-01-02 15:04:05"

// resolvePhotoDate parses date_taken and date_upload from the Flickr API,
// returning the best available time. Prefers date_taken; falls back to
// date_upload. Returns zero time if both are unusable.
func resolvePhotoDate(dateTaken, dateUpload string) time.Time {
	if dateTaken != "" && dateTaken != "0000-00-00 00:00:00" {
		if t, err := time.Parse(flickrDateFormat, dateTaken); err == nil {
			return t
		}
	}

	if dateUpload != "" {
		if epoch, err := strconv.ParseInt(dateUpload, 10, 64); err == nil && epoch > 0 {
			return time.Unix(epoch, 0)
		}
	}

	return time.Time{}
}
```

- [ ] **Step 2: Write the tests**

```go
package flickr

import (
	"testing"
	"time"
)

func TestResolvePhotoDate_DateTaken(t *testing.T) {
	got := resolvePhotoDate("2020-06-15 14:30:00", "1592234567")
	want := time.Date(2020, 6, 15, 14, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("resolvePhotoDate = %v, want %v", got, want)
	}
}

func TestResolvePhotoDate_FallbackToDateUpload(t *testing.T) {
	got := resolvePhotoDate("", "1592234567")
	want := time.Unix(1592234567, 0)
	if !got.Equal(want) {
		t.Errorf("resolvePhotoDate = %v, want %v", got, want)
	}
}

func TestResolvePhotoDate_ZeroDateTakenFallback(t *testing.T) {
	got := resolvePhotoDate("0000-00-00 00:00:00", "1592234567")
	want := time.Unix(1592234567, 0)
	if !got.Equal(want) {
		t.Errorf("resolvePhotoDate = %v, want %v", got, want)
	}
}

func TestResolvePhotoDate_BothUnusable(t *testing.T) {
	got := resolvePhotoDate("", "")
	if !got.IsZero() {
		t.Errorf("resolvePhotoDate = %v, want zero time", got)
	}
}

func TestResolvePhotoDate_BothUnusable_BadValues(t *testing.T) {
	got := resolvePhotoDate("0000-00-00 00:00:00", "not-a-number")
	if !got.IsZero() {
		t.Errorf("resolvePhotoDate = %v, want zero time", got)
	}
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/flickr/ -run TestResolvePhotoDate -v
```

Expected: PASS

- [ ] **Step 4: Run linter**

```bash
golangci-lint run ./internal/flickr/
```

- [ ] **Step 5: Commit**

```bash
git add internal/flickr/dates.go internal/flickr/dates_test.go
git commit -m "Add Flickr date resolution with date_taken/date_upload fallback"
```

---

## Chunk 3: Flickr download integration

### Task 4: Update photosResponse and API call

**Files:**
- Modify: `internal/flickr/flickr.go:207-221` (photosResponse struct)
- Modify: `internal/flickr/flickr.go:281-285` (signedAPIGet call)

- [ ] **Step 1: Add date fields to photo struct in photosResponse**

In `internal/flickr/flickr.go`, update the `photosResponse` struct to add `DateTaken` and `DateUpload` fields:

```go
// photosResponse represents the Flickr getPhotos API response.
type photosResponse struct {
	Photos struct {
		Page    int `json:"page"`
		Pages   int `json:"pages"`
		Total   int `json:"total"`
		Photo   []struct {
			ID         string `json:"id"`
			Secret     string `json:"secret"`
			Server     string `json:"server"`
			Title      string `json:"title"`
			DateTaken  string `json:"datetaken"`
			DateUpload string `json:"dateupload"`
		} `json:"photo"`
	} `json:"photos"`
	Stat string `json:"stat"`
}
```

- [ ] **Step 2: Add extras parameter to API call**

In the `Download` method, update the `signedAPIGet` call (around line 281) to request date extras:

```go
		resp, err := c.signedAPIGet(ctx, "flickr.people.getPhotos", map[string]string{
			"user_id":  "me",
			"page":     strconv.Itoa(page),
			"per_page": "500",
			"extras":   "date_taken,date_upload",
		})
```

- [ ] **Step 3: Run existing tests to make sure nothing breaks**

```bash
go test ./internal/flickr/ -v
```

Expected: PASS (adding fields to struct and an extra query param shouldn't break existing tests)

- [ ] **Step 4: Commit**

```bash
git add internal/flickr/flickr.go
git commit -m "Request date_taken and date_upload extras from Flickr API"
```

### Task 5: Set dates after download

**Files:**
- Modify: `internal/flickr/flickr.go` (imports, and after successful download in Download loop)

- [ ] **Step 1: Add mp4meta import and date-setting logic after download**

Add to imports in `flickr.go`:

```go
	"github.com/briandeitte/photo-copy/internal/mp4meta"
```

Insert date-setting logic **after** the `appendTransferLog` call and `os.Stat` / `RecordSuccess` block (after line 366), but **before** `estimator.Tick()`. This placement keeps transfer log recording and stats separate from metadata operations. The code to insert:

```go
			// Set original dates on downloaded file.
			photoDate := resolvePhotoDate(photo.DateTaken, photo.DateUpload)
			if !photoDate.IsZero() {
				filePath := filepath.Join(outputDir, filename)
				if ext == ".mp4" || ext == ".mov" {
					if err := mp4meta.SetCreationTime(filePath, photoDate); err != nil {
						c.log.Error("setting MP4 metadata for %s: %v", filename, err)
					}
				}
				if err := os.Chtimes(filePath, photoDate, photoDate); err != nil {
					c.log.Error("setting file time for %s: %v", filename, err)
				}
			} else {
				c.log.Info("no date available for %s, skipping date metadata", photo.ID)
			}
```

For context, the surrounding code looks like this (showing where to insert):
```go
			// ... existing code ...
			info, statErr := os.Stat(filepath.Join(outputDir, filename))
			if statErr != nil {
				c.log.Error("stat after download for %s: %v", filename, statErr)
				result.RecordSuccess(filename, 0)
			} else {
				result.RecordSuccess(filename, info.Size())
			}

			// >>> INSERT DATE-SETTING CODE HERE <<<

			estimator.Tick()
			// ... rest of loop ...
```

Note: `ext` is already the final resolved extension after the 404 fallback loop, and `filename` is already the final resolved filename.

- [ ] **Step 2: Run all tests**

```bash
go test ./internal/flickr/ -v
```

```bash
go test ./internal/mp4meta/ -v
```

- [ ] **Step 3: Run linter**

```bash
golangci-lint run ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/flickr/flickr.go
git commit -m "Set original capture dates on Flickr downloads (MP4 metadata + file times)"
```

---

## Chunk 4: Integration test

### Task 6: Add integration test for date preservation

**Files:**
- Modify: `internal/cli/integration_test.go`

- [ ] **Step 1: Add integration test**

Add `"time"` to the imports in `internal/cli/integration_test.go`, then add this test. It verifies that file system timestamps are set for photos:

```go
func TestFlickrDownload_PreservesOriginalDates(t *testing.T) {
	outputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	photos := []map[string]string{
		{
			"id": "1", "secret": "aaa", "server": "1", "title": "photo1",
			"datetaken": "2020-06-15 14:30:00", "dateupload": "1592234567",
		},
	}

	var mock *mockserver.FlickrMock
	mock = mockserver.NewFlickr(t).
		OnGetPhotos(mockserver.RespondJSON(200, flickrPhotosResponse(photos, 1, 1, 1))).
		OnGetSizes(func(w http.ResponseWriter, r *http.Request) {
			photoID := r.URL.Query().Get("photo_id")
			mockserver.RespondJSON(200, flickrSizesResponse(
				mock.Server.URL+"/download/"+photoID+".jpg",
			))(w, r)
		}).
		OnDownload(mockserver.RespondBytes(200, testImageData)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_API_URL", mock.APIURL)

	err := executeCmd(t, "flickr", "download", outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify file system timestamp was set to the date_taken value
	filePath := filepath.Join(outputDir, "1_aaa.jpg")
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("file not found: %v", err)
	}
	expectedTime := time.Date(2020, 6, 15, 14, 30, 0, 0, time.UTC)
	modTime := info.ModTime().UTC()
	if !modTime.Equal(expectedTime) {
		t.Errorf("file mod time = %v, want %v", modTime, expectedTime)
	}
}
```

- [ ] **Step 2: Run integration test**

```bash
go test ./internal/cli/ -tags integration -run TestFlickrDownload_PreservesOriginalDates -v
```

Expected: PASS

- [ ] **Step 3: Run all tests**

```bash
go test ./...
```

```bash
go test ./internal/cli/ -tags integration
```

```bash
golangci-lint run ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/cli/integration_test.go
git commit -m "Add integration test for Flickr download date preservation"
```

### Task 7: Final verification

- [ ] **Step 1: Run full test suite**

```bash
go test ./...
```

- [ ] **Step 2: Run integration tests**

```bash
go test ./internal/cli/ -tags integration
```

- [ ] **Step 3: Run linter**

```bash
golangci-lint run ./...
```

- [ ] **Step 4: Build binary**

```bash
go build -o photo-copy ./cmd/photo-copy
```
