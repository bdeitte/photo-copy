# --no-metadata and --date-range Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `--no-metadata` and `--date-range` root-level CLI flags to control metadata embedding and filter transfers by date.

**Architecture:** Two new fields in `rootOpts`, a new `internal/daterange` package for parsing/matching, date-reading functions in `jpegmeta`/`mp4meta`, and plumbing through all service Upload/Download methods. No-op warnings via `PersistentPreRunE`.

**Tech Stack:** Go, cobra, `rwcarlsen/goexif` (EXIF reading), `abema/go-mp4` (MP4 reading — already a dependency)

**Spec:** `plans/2026-03-18-no-metadata-date-range-design.md`

---

### Task 1: Create `internal/daterange` package

**Files:**
- Create: `internal/daterange/daterange.go`
- Create: `internal/daterange/daterange_test.go`

- [ ] **Step 1: Write failing tests for `Parse` and `Contains`**

```go
package daterange

import (
	"testing"
	"time"
)

func TestParseBothBounds(t *testing.T) {
	dr, err := Parse("2020-01-01:2023-12-31")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dr.After == nil || dr.Before == nil {
		t.Fatal("expected both bounds to be set")
	}
	// After should be start of 2020-01-01
	expectedAfter := time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local)
	if !dr.After.Equal(expectedAfter) {
		t.Errorf("After = %v, want %v", *dr.After, expectedAfter)
	}
	// Before should be start of 2024-01-01 (day after end date)
	expectedBefore := time.Date(2024, 1, 1, 0, 0, 0, 0, time.Local)
	if !dr.Before.Equal(expectedBefore) {
		t.Errorf("Before = %v, want %v", *dr.Before, expectedBefore)
	}
}

func TestParseOpenStart(t *testing.T) {
	dr, err := Parse(":2023-12-31")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dr.After != nil {
		t.Error("expected After to be nil")
	}
	if dr.Before == nil {
		t.Fatal("expected Before to be set")
	}
}

func TestParseOpenEnd(t *testing.T) {
	dr, err := Parse("2020-01-01:")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dr.After == nil {
		t.Fatal("expected After to be set")
	}
	if dr.Before != nil {
		t.Error("expected Before to be nil")
	}
}

func TestParseBadFormat(t *testing.T) {
	cases := []string{"", "2020-01-01", "bad:date", "2023-12-31:2020-01-01"}
	for _, c := range cases {
		if _, err := Parse(c); err == nil {
			t.Errorf("Parse(%q) should have returned error", c)
		}
	}
}

func TestParseEmptyBothSides(t *testing.T) {
	_, err := Parse(":")
	if err == nil {
		t.Error("Parse(':') should return error")
	}
}

func TestContains(t *testing.T) {
	dr, _ := Parse("2020-01-01:2023-12-31")
	tests := []struct {
		t    time.Time
		want bool
	}{
		{time.Date(2019, 12, 31, 23, 59, 59, 0, time.Local), false},
		{time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local), true},
		{time.Date(2022, 6, 15, 12, 0, 0, 0, time.Local), true},
		{time.Date(2023, 12, 31, 23, 59, 59, 999999999, time.Local), true},
		{time.Date(2024, 1, 1, 0, 0, 0, 0, time.Local), false},
	}
	for _, tt := range tests {
		if got := dr.Contains(tt.t); got != tt.want {
			t.Errorf("Contains(%v) = %v, want %v", tt.t, got, tt.want)
		}
	}
}

func TestContainsNilRange(t *testing.T) {
	var dr *DateRange
	// nil range should match everything
	if !dr.Contains(time.Now()) {
		t.Error("nil DateRange should contain all times")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/daterange/ -v`
Expected: compilation error — package does not exist

- [ ] **Step 3: Implement `daterange.go`**

```go
// Package daterange provides date range parsing and matching for the --date-range flag.
package daterange

import (
	"fmt"
	"strings"
	"time"
)

const dateFormat = "2006-01-02"

// DateRange represents an optional date range with inclusive bounds.
// Nil fields indicate no bound in that direction.
type DateRange struct {
	After  *time.Time // start of range (inclusive); nil = no lower bound
	Before *time.Time // start of day AFTER end date (exclusive); nil = no upper bound
}

// Parse parses a date range string in the format "YYYY-MM-DD:YYYY-MM-DD".
// Either side may be empty for open-ended ranges: "2020-01-01:", ":2023-12-31".
// Both sides empty (":") is an error.
func Parse(s string) (*DateRange, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid date range %q: expected format YYYY-MM-DD:YYYY-MM-DD", s)
	}

	startStr, endStr := parts[0], parts[1]
	if startStr == "" && endStr == "" {
		return nil, fmt.Errorf("invalid date range %q: at least one date must be specified", s)
	}

	dr := &DateRange{}

	if startStr != "" {
		t, err := time.ParseInLocation(dateFormat, startStr, time.Local)
		if err != nil {
			return nil, fmt.Errorf("invalid start date %q: %w", startStr, err)
		}
		dr.After = &t
	}

	if endStr != "" {
		t, err := time.ParseInLocation(dateFormat, endStr, time.Local)
		if err != nil {
			return nil, fmt.Errorf("invalid end date %q: %w", endStr, err)
		}
		// Make end date inclusive of the full day by using start of next day
		nextDay := t.AddDate(0, 0, 1)
		dr.Before = &nextDay
	}

	if dr.After != nil && dr.Before != nil && !dr.After.Before(*dr.Before) {
		return nil, fmt.Errorf("invalid date range: start %s is not before end %s", startStr, endStr)
	}

	return dr, nil
}

// Contains reports whether t falls within the date range.
// A nil DateRange contains all times.
func (dr *DateRange) Contains(t time.Time) bool {
	if dr == nil {
		return true
	}
	if dr.After != nil && t.Before(*dr.After) {
		return false
	}
	if dr.Before != nil && !t.Before(*dr.Before) {
		return false
	}
	return true
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/daterange/ -v`
Expected: all tests pass

- [ ] **Step 5: Run lint**

Run: `golangci-lint run ./internal/daterange/`
Expected: no issues

- [ ] **Step 6: Commit**

```bash
git add internal/daterange/
git commit -m "Add daterange package for --date-range flag parsing"
```

---

### Task 2: Add root-level flags and `PersistentPreRunE` no-op warnings

**Files:**
- Modify: `internal/cli/root.go`

- [ ] **Step 1: Add `noMetadata` and `dateRange` to `rootOpts` and register flags**

Add to `rootOpts` struct:
```go
noMetadata bool
dateRangeStr string
```

Add to `NewRootCmd` after existing flags:
```go
rootCmd.PersistentFlags().BoolVar(&opts.noMetadata, "no-metadata", false, "Skip metadata embedding during Flickr downloads (XMP, MP4 creation time, timestamps)")
rootCmd.PersistentFlags().StringVar(&opts.dateRangeStr, "date-range", "", "Filter by date range (YYYY-MM-DD:YYYY-MM-DD, either side optional). For S3, filters by file modification time via rclone, not embedded metadata dates.")
```

- [ ] **Step 2: Add `PersistentPreRunE` for no-op warnings and date-range validation**

Add a `PersistentPreRunE` to the root command that:
1. Parses `dateRangeStr` into a `*daterange.DateRange` stored on `rootOpts` (add a `parsedDateRange *daterange.DateRange` field).
2. Checks the subcommand path (`cmd.CommandPath()`) and logs warnings when flags don't apply.

```go
import "github.com/briandeitte/photo-copy/internal/daterange"
```

Add field to `rootOpts`:
```go
parsedDateRange *daterange.DateRange
```

Add `PersistentPreRunE`:
```go
rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
    path := cmd.CommandPath()

    if opts.dateRangeStr != "" {
        dr, err := daterange.Parse(opts.dateRangeStr)
        if err != nil {
            return fmt.Errorf("invalid --date-range: %w", err)
        }
        opts.parsedDateRange = dr
    }

    // No-op warnings
    if opts.noMetadata {
        if !strings.Contains(path, "flickr download") {
            fmt.Fprintln(os.Stderr, "Warning: --no-metadata has no effect on "+cmd.Name()+"; metadata embedding only occurs during Flickr downloads")
        }
    }

    if opts.parsedDateRange != nil {
        if strings.Contains(path, "import-takeout") {
            fmt.Fprintln(os.Stderr, "Warning: --date-range has no effect on import-takeout")
            opts.parsedDateRange = nil
        } else if strings.Contains(path, "config") {
            fmt.Fprintln(os.Stderr, "Warning: --date-range has no effect on config commands")
            opts.parsedDateRange = nil
        }
    }

    return nil
}
```

Note: Add `"strings"` to the import block.

- [ ] **Step 3: Build to verify compilation**

Run: `go build ./cmd/photo-copy`
Expected: compiles successfully

- [ ] **Step 4: Run all tests**

Run: `go test ./...`
Expected: all pass

- [ ] **Step 5: Run lint**

Run: `golangci-lint run ./...`
Expected: no issues

- [ ] **Step 6: Commit**

```bash
git add internal/cli/root.go
git commit -m "Add --no-metadata and --date-range root flags with no-op warnings"
```

---

### Task 3: Implement `--no-metadata` in Flickr download

**Files:**
- Modify: `internal/flickr/flickr.go:267` (Download method signature and metadata block)
- Modify: `internal/cli/flickr.go:47` (pass `noMetadata` to Download)

- [ ] **Step 1: Update `Download` signature to accept `noMetadata bool`**

In `internal/flickr/flickr.go`, change line 267:
```go
func (c *Client) Download(ctx context.Context, outputDir string, limit int, noMetadata bool) (*transfer.Result, error) {
```

- [ ] **Step 2: Wrap metadata block in `if !noMetadata`**

In `internal/flickr/flickr.go`, wrap lines 388-430 (the metadata block starting with `photoDate := resolvePhotoDate(...)` through the `os.Chtimes` block) in:
```go
if !noMetadata {
    // existing metadata code...
}
```

Keep the `photoDate` resolution and the status log line outside the guard — the date is still useful for logging even when metadata is skipped. Move only the actual metadata-writing operations inside the guard:
- `mp4meta.SetCreationTime`
- `jpegmeta.SetMetadata`
- `mp4meta.SetXMPMetadata`
- `os.Chtimes`

Restructure as:
```go
photoDate := resolvePhotoDate(photo.DateTaken, photo.DateUpload)
if !noMetadata {
    if !photoDate.IsZero() {
        if ext == ".mp4" || ext == ".mov" {
            if err := mp4meta.SetCreationTime(filePath, photoDate); err != nil {
                c.log.Error("setting MP4 metadata for %s: %v", filename, err)
            }
        }
    }

    meta := buildPhotoMeta(photo.Title, photo.Description.Content, photo.Tags)
    if !meta.isEmpty() {
        switch ext {
        case ".jpg", ".jpeg":
            if err := jpegmeta.SetMetadata(filePath, jpegmeta.Metadata{
                Title:       meta.Title,
                Description: meta.Description,
                Tags:        meta.Tags,
            }); err != nil {
                c.log.Error("setting JPEG XMP metadata for %s: %v", filename, err)
            }
        case ".mp4", ".mov":
            if err := mp4meta.SetXMPMetadata(filePath, mp4meta.XMPMetadata{
                Title:       meta.Title,
                Description: meta.Description,
                Tags:        meta.Tags,
            }); err != nil {
                c.log.Error("setting MP4 XMP metadata for %s: %v", filename, err)
            }
        }
    }

    if !photoDate.IsZero() {
        if err := os.Chtimes(filePath, photoDate, photoDate); err != nil {
            c.log.Error("setting file time for %s: %v", filename, err)
        }
    } else {
        c.log.Info("no date available for %s, skipping date metadata", filename)
    }
}
```

- [ ] **Step 3: Update CLI to pass `noMetadata`**

In `internal/cli/flickr.go:47`, change:
```go
result, err := client.Download(context.Background(), outputDir, opts.limit)
```
to:
```go
result, err := client.Download(context.Background(), outputDir, opts.limit, opts.noMetadata)
```

- [ ] **Step 4: Build to verify compilation**

Run: `go build ./cmd/photo-copy`
Expected: compiles successfully

- [ ] **Step 5: Run all tests and lint**

Run: `go test ./...`
Run: `golangci-lint run ./...`
Expected: all pass, no lint issues

- [ ] **Step 6: Commit**

```bash
git add internal/flickr/flickr.go internal/cli/flickr.go
git commit -m "Implement --no-metadata flag for Flickr downloads"
```

---

### Task 4: Implement `--date-range` in Flickr download

**Files:**
- Modify: `internal/flickr/flickr.go:267` (Download signature, add date filtering in loop)
- Modify: `internal/cli/flickr.go:47` (pass dateRange)

- [ ] **Step 1: Update `Download` signature**

Change to:
```go
func (c *Client) Download(ctx context.Context, outputDir string, limit int, noMetadata bool, dateRange *daterange.DateRange) (*transfer.Result, error) {
```

Add import:
```go
"github.com/briandeitte/photo-copy/internal/daterange"
```

- [ ] **Step 2: Add date filtering after the transfer log skip check**

In the photo loop (after the `transferred[photo.ID]` skip check at line ~325), add:
```go
// Date range filtering
if dateRange != nil {
    photoDate := resolvePhotoDate(photo.DateTaken, photo.DateUpload)
    if !photoDate.IsZero() && !dateRange.Contains(photoDate) {
        result.RecordSkip(1)
        c.log.Debug("skipping %s: date %s outside range", photo.ID, photoDate.Format("2006-01-02"))
        continue
    }
    // Photos with no resolvable date are included
}
```

Note: this `photoDate` is computed early for filtering; the later `photoDate` in the metadata block is separate (only reached for non-filtered photos).

- [ ] **Step 3: Update CLI call**

In `internal/cli/flickr.go:47`, change to:
```go
result, err := client.Download(context.Background(), outputDir, opts.limit, opts.noMetadata, opts.parsedDateRange)
```

- [ ] **Step 4: Build and run tests**

Run: `go build ./cmd/photo-copy`
Run: `go test ./...`
Run: `golangci-lint run ./...`
Expected: all pass

- [ ] **Step 5: Commit**

```bash
git add internal/flickr/flickr.go internal/cli/flickr.go
git commit -m "Implement --date-range filtering for Flickr downloads"
```

---

### Task 5: Add JPEG date reading to `jpegmeta`

**Files:**
- Modify: `go.mod` (add `rwcarlsen/goexif` dependency)
- Create: `internal/jpegmeta/readdate.go`
- Create: `internal/jpegmeta/readdate_test.go`

- [ ] **Step 1: Add the goexif dependency**

Run: `go get github.com/rwcarlsen/goexif/exif`

- [ ] **Step 2: Write failing test**

Create `internal/jpegmeta/readdate_test.go`. For test fixtures, create a minimal JPEG with EXIF data programmatically, or use a known test JPEG. Simplest approach — test with a file that has no EXIF (returns zero time) and test the function signature:

```go
package jpegmeta

import (
	"os"
	"testing"
	"time"
)

func TestReadDateNoExif(t *testing.T) {
	// Create a minimal JPEG without EXIF
	tmp := t.TempDir()
	path := tmp + "/test.jpg"
	// Minimal JPEG: SOI + EOI
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

func TestReadDateWithExif(t *testing.T) {
	// This test requires a JPEG file with EXIF DateTimeOriginal.
	// Create one using the goexif test utilities or use an embedded fixture.
	// For now, skip if no fixture available — the integration tests will cover this.
	t.Skip("requires JPEG fixture with EXIF DateTimeOriginal")
	_ = time.Now() // placeholder
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/jpegmeta/ -v -run TestReadDate`
Expected: compilation error — ReadDate not defined

- [ ] **Step 4: Implement `ReadDate`**

Create `internal/jpegmeta/readdate.go`:

```go
package jpegmeta

import (
	"os"
	"time"

	"github.com/rwcarlsen/goexif/exif"
)

// ReadDate reads the EXIF DateTimeOriginal from a JPEG file.
// Falls back to DateTime if DateTimeOriginal is not present.
// Returns zero time if the EXIF data is missing or unparseable.
// Returns an error only for file I/O failures.
func ReadDate(filePath string) (time.Time, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return time.Time{}, err
	}
	defer func() { _ = f.Close() }()

	x, err := exif.Decode(f)
	if err != nil {
		// No EXIF data — not an error, just no date
		return time.Time{}, nil
	}

	// Try DateTimeOriginal first (camera capture date), then DateTime (last modified)
	tag, err := x.Get(exif.DateTimeOriginal)
	if err == nil {
		if t, err := time.Parse("2006:01:02 15:04:05", tag.StringVal()); err == nil {
			return t, nil
		}
	}

	// Fallback to DateTime
	t, err := x.DateTime()
	if err != nil {
		return time.Time{}, nil
	}

	return t, nil
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/jpegmeta/ -v -run TestReadDate`
Expected: tests pass (except skipped fixture test)

- [ ] **Step 6: Run lint**

Run: `golangci-lint run ./internal/jpegmeta/`
Expected: no issues

- [ ] **Step 7: Commit**

```bash
git add internal/jpegmeta/readdate.go internal/jpegmeta/readdate_test.go go.mod go.sum
git commit -m "Add JPEG EXIF date reading via goexif"
```

---

### Task 6: Add MP4 creation time reading to `mp4meta`

**Files:**
- Create: `internal/mp4meta/readdate.go`
- Create: `internal/mp4meta/readdate_test.go`

- [ ] **Step 1: Write failing test**

```go
package mp4meta

import (
	"os"
	"testing"
)

func TestReadCreationTimeInvalidFile(t *testing.T) {
	tmp := t.TempDir()
	path := tmp + "/test.mp4"
	if err := os.WriteFile(path, []byte("not an mp4"), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadCreationTime(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.IsZero() {
		t.Errorf("expected zero time for invalid MP4, got %v", got)
	}
}

func TestReadCreationTimeNonExistent(t *testing.T) {
	_, err := ReadCreationTime("/nonexistent/file.mp4")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/mp4meta/ -v -run TestReadCreationTime`
Expected: compilation error — ReadCreationTime not defined

- [ ] **Step 3: Implement `ReadCreationTime`**

Create `internal/mp4meta/readdate.go`:

```go
package mp4meta

import (
	"os"
	"time"

	gomp4 "github.com/abema/go-mp4"
)

// ReadCreationTime reads the creation time from an MP4/MOV file's mvhd box.
// Returns zero time if the file is not a valid MP4 or has no creation time.
// Returns an error only for file I/O failures.
func ReadCreationTime(filePath string) (time.Time, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return time.Time{}, err
	}
	defer func() { _ = f.Close() }()

	var creationTime uint64
	found := false

	_, err = gomp4.ReadBoxStructure(f, func(h *gomp4.ReadHandle) (interface{}, error) {
		switch h.BoxInfo.Type {
		case gomp4.BoxTypeMoov():
			// Must expand container boxes to reach mvhd inside
			_, err := h.Expand()
			return nil, err

		case gomp4.BoxTypeMvhd():
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			if mvhd, ok := box.(*gomp4.Mvhd); ok {
				if mvhd.GetVersion() == 0 {
					creationTime = uint64(mvhd.CreationTimeV0)
				} else {
					creationTime = mvhd.CreationTimeV1
				}
				found = true
			}
			return nil, nil

		default:
			return nil, nil
		}
	})
	if err != nil {
		// Not a valid MP4 — return zero time
		return time.Time{}, nil
	}

	if !found || creationTime == 0 {
		return time.Time{}, nil
	}

	// Reuse the existing mp4Epoch constant (seconds offset from Unix epoch)
	return time.Unix(int64(creationTime)-mp4Epoch, 0), nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/mp4meta/ -v -run TestReadCreationTime`
Expected: tests pass

- [ ] **Step 5: Run lint**

Run: `golangci-lint run ./internal/mp4meta/`
Expected: no issues

- [ ] **Step 6: Commit**

```bash
git add internal/mp4meta/readdate.go internal/mp4meta/readdate_test.go
git commit -m "Add MP4 creation time reading from mvhd box"
```

---

### Task 7: Create shared `mediadate` helper for upload date resolution

**Files:**
- Create: `internal/mediadate/mediadate.go`
- Create: `internal/mediadate/mediadate_test.go`

This is a thin wrapper that picks the right reader based on file extension and falls back to modtime.

- [ ] **Step 1: Write failing tests**

```go
package mediadate

import (
	"os"
	"testing"
	"time"
)

func TestResolveDate_FallbackToModTime(t *testing.T) {
	// Create a plain text file — no EXIF or MP4 metadata
	tmp := t.TempDir()
	path := tmp + "/test.png"
	if err := os.WriteFile(path, []byte("fake png"), 0644); err != nil {
		t.Fatal(err)
	}
	// Set a known modtime
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/mediadate/ -v`
Expected: compilation error

- [ ] **Step 3: Implement `mediadate.go`**

```go
// Package mediadate resolves the best available date from a media file's
// embedded metadata, falling back to file modification time.
package mediadate

import (
	"os"
	"strings"
	"time"

	"github.com/briandeitte/photo-copy/internal/jpegmeta"
	"github.com/briandeitte/photo-copy/internal/mp4meta"
)

// ResolveDate returns the best available date for a media file.
// For JPEGs, reads EXIF DateTimeOriginal. For MP4/MOV, reads creation time
// from the mvhd box. Falls back to file modification time.
// Returns zero time if the file cannot be read.
func ResolveDate(filePath string) time.Time {
	ext := strings.ToLower(filePath)

	switch {
	case strings.HasSuffix(ext, ".jpg") || strings.HasSuffix(ext, ".jpeg"):
		if t, err := jpegmeta.ReadDate(filePath); err == nil && !t.IsZero() {
			return t
		}
	case strings.HasSuffix(ext, ".mp4") || strings.HasSuffix(ext, ".mov"):
		if t, err := mp4meta.ReadCreationTime(filePath); err == nil && !t.IsZero() {
			return t
		}
	}

	// Fallback to file modification time
	info, err := os.Stat(filePath)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/mediadate/ -v`
Expected: tests pass

- [ ] **Step 5: Run lint**

Run: `golangci-lint run ./internal/mediadate/`
Expected: no issues

- [ ] **Step 6: Commit**

```bash
git add internal/mediadate/
git commit -m "Add mediadate package for resolving file dates from metadata"
```

---

### Task 8: Implement `--date-range` in Flickr upload

**Files:**
- Modify: `internal/flickr/flickr.go:552` (Upload method)
- Modify: `internal/cli/flickr.go:94` (pass dateRange)

- [ ] **Step 1: Update `Upload` signature**

Change line 552:
```go
func (c *Client) Upload(ctx context.Context, inputDir string, limit int, dateRange *daterange.DateRange) (*transfer.Result, error) {
```

Add imports:
```go
"github.com/briandeitte/photo-copy/internal/daterange"
"github.com/briandeitte/photo-copy/internal/mediadate"
```

- [ ] **Step 2: Add date filtering in the upload loop**

The current code at lines 574-577 truncates files by limit upfront. With date range filtering, we can't pre-truncate since we need to check each file's date. Restructure the upload loop:

After building the `files` slice (line 566), instead of the existing limit truncation, add date filtering that builds a filtered list:

```go
// Filter by date range (before applying limit)
if dateRange != nil {
    var filtered []string
    for _, name := range files {
        filePath := filepath.Join(inputDir, name)
        fileDate := mediadate.ResolveDate(filePath)
        if fileDate.IsZero() || dateRange.Contains(fileDate) {
            filtered = append(filtered, name)
        } else {
            result.RecordSkip(1)
            c.log.Debug("skipping %s: date %s outside range", name, fileDate.Format("2006-01-02"))
        }
    }
    files = filtered
}

if len(files) == 0 {
    c.log.Info("no supported media files found in %s (after filtering)", inputDir)
    result.Finish()
    return result, nil
}

if limit > 0 && len(files) > limit {
    c.log.Info("limiting upload to %d of %d files", limit, len(files))
    files = files[:limit]
}
```

- [ ] **Step 3: Update CLI call**

In `internal/cli/flickr.go:94`, change to:
```go
result, err := client.Upload(context.Background(), inputDir, opts.limit, opts.parsedDateRange)
```

- [ ] **Step 4: Build and test**

Run: `go build ./cmd/photo-copy`
Run: `go test ./...`
Run: `golangci-lint run ./...`
Expected: all pass

- [ ] **Step 5: Commit**

```bash
git add internal/flickr/flickr.go internal/cli/flickr.go
git commit -m "Implement --date-range filtering for Flickr uploads"
```

---

### Task 9: Implement `--date-range` in Google upload

**Files:**
- Modify: `internal/google/google.go:104` (Upload method)
- Modify: `internal/cli/google.go:49` (pass dateRange)

- [ ] **Step 1: Update `Upload` signature**

Change line 104:
```go
func (c *Client) Upload(ctx context.Context, inputDir string, limit int, dateRange *daterange.DateRange) (*transfer.Result, error) {
```

Add imports:
```go
"github.com/briandeitte/photo-copy/internal/daterange"
"github.com/briandeitte/photo-copy/internal/mediadate"
```

- [ ] **Step 2: Add date filtering after collecting files and filtering uploaded**

After the `toUpload` list is built (line ~126), add:

```go
// Filter by date range
if dateRange != nil {
    var filtered []string
    for _, filePath := range toUpload {
        fileDate := mediadate.ResolveDate(filePath)
        if fileDate.IsZero() || dateRange.Contains(fileDate) {
            filtered = append(filtered, filePath)
        } else {
            result.RecordSkip(1)
            c.log.Debug("skipping %s: date %s outside range", filepath.Base(filePath), fileDate.Format("2006-01-02"))
        }
    }
    toUpload = filtered
}
```

Place this before the dailyLimit and `--limit` truncation checks so date filtering is applied first.

- [ ] **Step 3: Update CLI call**

In `internal/cli/google.go:49`, change to:
```go
result, err := client.Upload(ctx, inputDir, opts.limit, opts.parsedDateRange)
```

- [ ] **Step 4: Build and test**

Run: `go build ./cmd/photo-copy`
Run: `go test ./...`
Run: `golangci-lint run ./...`
Expected: all pass

- [ ] **Step 5: Commit**

```bash
git add internal/google/google.go internal/cli/google.go
git commit -m "Implement --date-range filtering for Google Photos uploads"
```

---

### Task 10: Implement `--date-range` in S3 upload and download

**Files:**
- Modify: `internal/s3/s3.go:28,77` (Upload/Download methods)
- Modify: `internal/cli/s3.go:44,74` (pass dateRange)

- [ ] **Step 1: Update S3 `Upload` and `Download` signatures**

In `internal/s3/s3.go`, change Upload (line 28):
```go
func (c *Client) Upload(ctx context.Context, inputDir, bucket, prefix string, mediaOnly bool, limit int, dateRange *daterange.DateRange) (*transfer.Result, error) {
```

Change Download (line 77):
```go
func (c *Client) Download(ctx context.Context, bucket, prefix, outputDir string, mediaOnly bool, limit int, dateRange *daterange.DateRange) (*transfer.Result, error) {
```

Add import:
```go
"github.com/briandeitte/photo-copy/internal/daterange"
```

- [ ] **Step 2: Add helper to build rclone age filter args**

Add a helper function:
```go
// buildDateRangeFlags converts a DateRange into rclone --min-age/--max-age flags.
func buildDateRangeFlags(dr *daterange.DateRange) []string {
	if dr == nil {
		return nil
	}
	var flags []string
	if dr.After != nil {
		// --max-age = "files no older than" = our After bound
		flags = append(flags, "--max-age", dr.After.Format("2006-01-02"))
	}
	if dr.Before != nil {
		// --min-age = "files at least this old" = our Before bound
		// dr.Before is start of next day, so use the original end date
		endDate := dr.Before.AddDate(0, 0, -1)
		flags = append(flags, "--min-age", endDate.Format("2006-01-02"))
	}
	return flags
}
```

- [ ] **Step 3: Apply date range flags in Upload**

In `Upload`, add a `dateFlags` variable after the `filterFlags`:
```go
dateFlags := buildDateRangeFlags(dateRange)
```

**Important:** The existing code rebuilds `args` from scratch inside the `limit > 0` branch (line 65: `args = buildUploadArgs(...)`), which discards any flags appended earlier. The date range flags must be appended to `args` in **both** branches:

1. Before the `limit` check, append to `filterFlags` so `buildFilesFrom` picks them up:
```go
if len(dateFlags) > 0 {
    filterFlags = append(filterFlags, dateFlags...)
}
```

2. After the `limit` branch (both paths), append date flags to `args`:
```go
if len(dateFlags) > 0 {
    args = append(args, dateFlags...)
}
```

Or alternatively, move the date flag append to after the entire `if limit > 0 { ... }` block so it applies to the final `args` regardless of which branch was taken.

- [ ] **Step 4: Apply date range flags in Download**

Same pattern in `Download` — the `limit > 0` branch also rebuilds `args` (line 119), so apply the same fix: append date flags to `filterFlags` before the limit check, and append to `args` after the limit branch.

- [ ] **Step 5: Update `buildFilesFrom` to carry over date range flags**

In `buildFilesFrom` (line ~273), the code already carries over `--include` flags. Add similar handling for `--min-age` and `--max-age`:

```go
for i := 0; i < len(copyArgs); i++ {
    switch copyArgs[i] {
    case "--include":
        if i+1 < len(copyArgs) {
            lsfArgs = append(lsfArgs, "--include", copyArgs[i+1])
            i++
        }
    case "--min-age", "--max-age":
        if i+1 < len(copyArgs) {
            lsfArgs = append(lsfArgs, copyArgs[i], copyArgs[i+1])
            i++
        }
    }
}
```

- [ ] **Step 6: Update CLI calls**

In `internal/cli/s3.go:44`:
```go
result, err := client.Upload(context.Background(), args[0], bucket, prefix, true, opts.limit, opts.parsedDateRange)
```

In `internal/cli/s3.go:74`:
```go
result, err := client.Download(context.Background(), bucket, prefix, args[0], true, opts.limit, opts.parsedDateRange)
```

- [ ] **Step 7: Build and test**

Run: `go build ./cmd/photo-copy`
Run: `go test ./...`
Run: `golangci-lint run ./...`
Expected: all pass

- [ ] **Step 8: Commit**

```bash
git add internal/s3/s3.go internal/cli/s3.go
git commit -m "Implement --date-range filtering for S3 via rclone age flags"
```

---

### Task 11: Integration tests

**Files:**
- Modify: `internal/cli/integration_test.go`

- [ ] **Step 1: Add `--no-metadata` Flickr download test**

Add a test that downloads with `--no-metadata` and verifies:
- Files are downloaded successfully
- Downloaded files are not modified (no XMP, no timestamp changes)
- Transfer log is written correctly

```go
func TestFlickrDownloadNoMetadata(t *testing.T) {
	configDir := t.TempDir()
	outputDir := t.TempDir()
	setupFlickrConfig(t, configDir)
	setTestEnv(t, configDir)

	mock := mockserver.NewFlickrBuilder().
		OnGetPhotos(mockserver.RespondJSON(http.StatusOK, map[string]interface{}{
			"photos": map[string]interface{}{
				"page":    1,
				"pages":   1,
				"perpage": 500,
				"total":   1,
				"photo": []map[string]interface{}{
					{
						"id":          "123",
						"secret":      "abc",
						"title":       "Test Photo",
						"description": map[string]string{"_content": "A test"},
						"tags":        "tag1 tag2",
						"datetaken":   "2020-06-15 10:30:00",
						"dateupload":  "1592217000",
					},
				},
			},
			"stat": "ok",
		})).
		OnGetSizes("123", mockserver.RespondJSON(http.StatusOK, map[string]interface{}{
			"sizes": map[string]interface{}{
				"size": []map[string]interface{}{
					{"label": "Original", "source": "PHOTO_URL"},
				},
			},
			"stat": "ok",
		})).
		OnDownload(mockserver.RespondBytes(http.StatusOK, testImageData)).
		Build(t)
	defer mock.Close()
	t.Setenv("PHOTO_COPY_FLICKR_API_URL", mock.URL)

	err := executeCmd(t, "flickr", "download", "--no-metadata", outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify file was downloaded
	files, _ := os.ReadDir(outputDir)
	found := false
	for _, f := range files {
		if strings.HasPrefix(f.Name(), "123_") {
			found = true
			// File content should be the raw test data (no XMP modification)
			data, _ := os.ReadFile(filepath.Join(outputDir, f.Name()))
			if !bytes.Equal(data, testImageData) {
				t.Errorf("file was modified despite --no-metadata")
			}
		}
	}
	if !found {
		t.Error("downloaded file not found")
	}
}
```

Note: May need to add `"bytes"` import. Adapt mock builder calls to match the actual `mockserver` API.

- [ ] **Step 2: Add `--date-range` Flickr download filtering test**

Add a test with multiple photos having different dates, use `--date-range` to filter, and verify only in-range photos are downloaded.

- [ ] **Step 3: Add no-op warning tests**

Test that `--no-metadata` with `flickr upload` doesn't error (just warns) and that `--date-range` with `google import-takeout` doesn't error.

- [ ] **Step 4: Run integration tests**

Run: `go test ./internal/cli/ -tags integration -v`
Expected: all pass

- [ ] **Step 5: Run lint**

Run: `golangci-lint run ./...`
Expected: no issues

- [ ] **Step 6: Commit**

```bash
git add internal/cli/integration_test.go
git commit -m "Add integration tests for --no-metadata and --date-range flags"
```

---

### Task 12: Documentation updates

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update README.md**

Add a section documenting the new flags:

```markdown
### Filtering Options

- `--no-metadata` — Skip metadata embedding during Flickr downloads (XMP metadata, MP4 creation time, filesystem timestamps). The raw file is downloaded without modification.
- `--date-range YYYY-MM-DD:YYYY-MM-DD` — Only process files within the specified date range. Either side can be omitted for open-ended ranges (e.g., `2020-01-01:` for everything from 2020 onward, `:2023-12-31` for everything up to end of 2023).

**Date sources by command:**
- **Flickr download**: Uses `date_taken` (preferred) or `date_upload` from the Flickr API.
- **Flickr/Google upload**: Reads EXIF DateTimeOriginal (JPEG) or MP4 creation time, falling back to file modification time.
- **S3 upload/download**: Uses rclone's `--min-age`/`--max-age` flags, which filter by **file modification time** (or S3 object LastModified timestamp), not embedded metadata dates. This is a limitation of delegating to rclone.
```

- [ ] **Step 2: Update CLAUDE.md**

Add to the Architecture / Package layout section:
- **daterange/** — Parses `--date-range YYYY-MM-DD:YYYY-MM-DD` flag values into a `DateRange` struct with optional `After`/`Before` bounds. `Contains()` checks if a time falls within the range.
- **mediadate/** — Resolves the best available date from a media file: EXIF `DateTimeOriginal` for JPEGs (via `rwcarlsen/goexif`), `mvhd` creation time for MP4/MOV (via `abema/go-mp4`), falling back to file modification time.

Update the Key patterns section:
- Add note about `--no-metadata` skipping all metadata operations in Flickr download.
- Add note about `--date-range` filtering: Flickr download uses API dates, uploads use `mediadate.ResolveDate()`, S3 uses rclone `--min-age`/`--max-age` (file mod time, not metadata).

Update the `jpegmeta` and `mp4meta` package descriptions to mention the read capabilities:
- **jpegmeta/** — Writes XMP metadata ... **Also reads EXIF DateTimeOriginal** via `rwcarlsen/goexif` for date-range filtering of uploads.
- **mp4meta/** — Sets creation/modification timestamps ... **Also reads creation time from mvhd box** for date-range filtering of uploads.

- [ ] **Step 3: Run lint on the full project**

Run: `go test ./...`
Run: `golangci-lint run ./...`
Expected: all pass

- [ ] **Step 4: Commit**

```bash
git add README.md CLAUDE.md
git commit -m "Document --no-metadata and --date-range flags in README and CLAUDE.md"
```
