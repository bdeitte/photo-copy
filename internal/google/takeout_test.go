package google

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func createTestZip(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	zipPath := filepath.Join(dir, "takeout.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for name, content := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return zipPath
}

func TestImportTakeout_ExtractsMediaOnly(t *testing.T) {
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()

	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/Trip/photo1.jpg":               "jpegdata",
		"Google Photos/Trip/photo1.jpg.json":           `{"title":"photo1"}`,
		"Google Photos/Trip/video.mp4":                 "mp4data",
		"Google Photos/Trip/metadata.json":             `{"albums":[]}`,
		"Google Photos/Trip/print-subscriptions.json":  `{}`,
	})

	result, err := ImportTakeout(context.Background(), takeoutDir, outputDir, nil, false)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	if result.Succeeded != 2 {
		t.Fatalf("expected 2 files extracted, got %d", result.Succeeded)
	}

	if _, err := os.Stat(filepath.Join(outputDir, "Trip", "photo1.jpg")); err != nil {
		t.Fatal("photo1.jpg not found in Trip/ subdirectory")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "Trip", "video.mp4")); err != nil {
		t.Fatal("video.mp4 not found in Trip/ subdirectory")
	}
}

func TestImportTakeout_SkipsNonMedia(t *testing.T) {
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()

	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/readme.html": "<html>hi</html>",
		"Google Photos/data.json":   "{}",
	})

	result, err := ImportTakeout(context.Background(), takeoutDir, outputDir, nil, false)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	if result.Succeeded != 0 {
		t.Fatalf("expected 0 files extracted, got %d", result.Succeeded)
	}
}

func TestImportTakeout_MultipleZips(t *testing.T) {
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()

	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/Album1/a.jpg": "data1",
	})
	_ = os.Rename(filepath.Join(takeoutDir, "takeout.zip"), filepath.Join(takeoutDir, "takeout-001.zip"))

	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/Album2/b.png": "data2",
	})
	_ = os.Rename(filepath.Join(takeoutDir, "takeout.zip"), filepath.Join(takeoutDir, "takeout-002.zip"))

	result, err := ImportTakeout(context.Background(), takeoutDir, outputDir, nil, false)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	if result.Succeeded != 2 {
		t.Fatalf("expected 2 files, got %d", result.Succeeded)
	}
}

func TestImportTakeout_DuplicateFilenameRenames(t *testing.T) {
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()

	// Pre-create a file that will collide with the zip's Album/photo.jpg
	if err := os.MkdirAll(filepath.Join(outputDir, "Album"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "Album", "photo.jpg"), []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}

	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/Album/photo.jpg": "new data",
	})

	result, err := ImportTakeout(context.Background(), takeoutDir, outputDir, nil, false)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	if result.Succeeded != 1 {
		t.Fatalf("expected 1 file extracted, got %d", result.Succeeded)
	}

	// Original file should be untouched
	data, err := os.ReadFile(filepath.Join(outputDir, "Album", "photo.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "existing" {
		t.Errorf("original file was overwritten, got %q", string(data))
	}

	// New file should be renamed to photo_1.jpg in the Album subdirectory
	data, err = os.ReadFile(filepath.Join(outputDir, "Album", "photo_1.jpg"))
	if err != nil {
		t.Fatal("Album/photo_1.jpg not found — duplicate rename did not work")
	}
	if string(data) != "new data" {
		t.Errorf("renamed file has wrong content: %q", string(data))
	}
}

func TestImportTakeout_CancelledBeforeStart(t *testing.T) {
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()

	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/Trip/photo1.jpg": "jpegdata",
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := ImportTakeout(ctx, takeoutDir, outputDir, nil, false)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

func TestImportTakeout_CancelledDuringExtraction(t *testing.T) {
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()

	const fileCount = 20
	files := make(map[string]string, fileCount)
	for i := range fileCount {
		files[fmt.Sprintf("Google Photos/Album/photo%d.jpg", i)] = strings.Repeat("x", 1024)
	}
	createTestZip(t, takeoutDir, files)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel deterministically after the first file is extracted.
	extracted := 0
	afterExtract := func() {
		extracted++
		if extracted == 1 {
			cancel()
		}
	}

	result, err := ImportTakeout(ctx, takeoutDir, outputDir, nil, false, withAfterExtract(afterExtract))
	if err == nil {
		t.Fatal("expected error from cancelled context during extraction")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
	// Verify partial extraction — some files should exist in Album/ but not all
	albumDir := filepath.Join(outputDir, "Album")
	entries, _ := os.ReadDir(albumDir)
	if len(entries) == 0 {
		t.Error("expected at least one extracted file before cancellation")
	}
	if result != nil && result.Succeeded >= fileCount {
		t.Error("expected partial extraction, but all files were extracted")
	}
}

func TestImportTakeout_CancelledDuringExtractFile(t *testing.T) {
	// Exercises the ctx.Err() check after extractFile fails due to cancellation.
	// Uses beforeExtract to cancel the context right before the second file's
	// extractFile call (after the top-of-loop ctx.Err() check has already passed).
	// extractFile then fails inside copyWithContext, and the post-failure
	// ctx.Err() check returns context.Canceled immediately.
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()

	files := make(map[string]string, 5)
	for i := range 5 {
		files[fmt.Sprintf("Google Photos/Album/photo%d.jpg", i)] = strings.Repeat("x", 1024)
	}
	createTestZip(t, takeoutDir, files)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel right before the second extractFile call, after the top-of-loop
	// ctx.Err() check has passed. This ensures extractFile runs with a
	// cancelled context and fails, hitting the post-failure ctx.Err() branch.
	extractCount := 0
	beforeExtract := func() {
		extractCount++
		if extractCount == 2 {
			cancel()
		}
	}

	result, err := ImportTakeout(ctx, takeoutDir, outputDir, nil, false, withBeforeExtract(beforeExtract))
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Should have exactly 1 successful extraction before cancellation stopped the rest
	if result.Succeeded != 1 {
		t.Errorf("expected 1 succeeded, got %d", result.Succeeded)
	}
	// The cancelled extractFile should NOT be recorded as a per-file error
	if result.Failed > 0 {
		t.Errorf("expected 0 failed (cancellation should not record file errors), got %d", result.Failed)
	}
}

func TestImportTakeout_AlbumSubdirectory(t *testing.T) {
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()
	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/Trip to Paris/photo.jpg": "jpegdata",
		"Google Photos/Trip to Paris/video.mp4": "mp4data",
	})
	result, err := ImportTakeout(context.Background(), takeoutDir, outputDir, nil, false)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if result.Succeeded != 2 {
		t.Fatalf("expected 2, got %d", result.Succeeded)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "Trip to Paris", "photo.jpg")); err != nil {
		t.Error("expected photo.jpg in Trip to Paris/ subdirectory")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "Trip to Paris", "video.mp4")); err != nil {
		t.Error("expected video.mp4 in Trip to Paris/ subdirectory")
	}
}

func TestImportTakeout_YearFolderFlattened(t *testing.T) {
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()
	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/Photos from 2022/photo.jpg": "jpegdata",
	})
	result, err := ImportTakeout(context.Background(), takeoutDir, outputDir, nil, false)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if result.Succeeded != 1 {
		t.Fatalf("expected 1, got %d", result.Succeeded)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "photo.jpg")); err != nil {
		t.Error("expected photo.jpg in output root")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "Photos from 2022")); err == nil {
		t.Error("year folder subdirectory should not be created")
	}
}

func TestImportTakeout_DedupYearVsAlbum(t *testing.T) {
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()
	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/Trip/photo.jpg":             "jpegdata1234",
		"Google Photos/Photos from 2022/photo.jpg": "jpegdata1234",
	})
	result, err := ImportTakeout(context.Background(), takeoutDir, outputDir, nil, false)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if result.Succeeded != 1 {
		t.Fatalf("expected 1 (deduped), got %d", result.Succeeded)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "Trip", "photo.jpg")); err != nil {
		t.Error("expected photo.jpg in Trip/")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "photo.jpg")); err == nil {
		t.Error("year folder photo.jpg should have been deduped away")
	}
}

func TestImportTakeout_DedupDifferentSizeKeptBoth(t *testing.T) {
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()
	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/Trip/photo.jpg":             "short",
		"Google Photos/Photos from 2022/photo.jpg": "muchlongerdata",
	})
	result, err := ImportTakeout(context.Background(), takeoutDir, outputDir, nil, false)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if result.Succeeded != 2 {
		t.Fatalf("expected 2, got %d", result.Succeeded)
	}
}

func TestImportTakeout_NoMetadataFlag(t *testing.T) {
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()
	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/Trip/photo.jpg":      "jpegdata",
		"Google Photos/Trip/photo.jpg.json": `{"title":"My Photo","photoTakenTime":{"timestamp":"1640000000"}}`,
	})
	result, err := ImportTakeout(context.Background(), takeoutDir, outputDir, nil, true)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if result.Succeeded != 1 {
		t.Fatalf("expected 1, got %d", result.Succeeded)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "Trip", "photo.jpg")); err != nil {
		t.Error("expected photo.jpg to be extracted")
	}

	// Verify metadata was NOT applied — mtime should not match sidecar timestamp.
	info, err := os.Stat(filepath.Join(outputDir, "Trip", "photo.jpg"))
	if err != nil {
		t.Fatal("photo.jpg not found")
	}
	sidecarTime := time.Unix(1640000000, 0)
	if info.ModTime().Equal(sidecarTime) {
		t.Error("file mtime should not match sidecar timestamp when --no-metadata is set")
	}
}

func TestImportTakeout_MetadataFromJSON(t *testing.T) {
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()

	// Create a minimal valid JPEG with SOI marker so jpegmeta.SetMetadata works
	jpegData := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x02, 0x00, 0x00, 0xFF, 0xD9}

	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/Trip/photo.jpg":      string(jpegData),
		"Google Photos/Trip/photo.jpg.json": `{"title":"Beach","description":"Nice day","photoTakenTime":{"timestamp":"1640000000"}}`,
	})

	result, err := ImportTakeout(context.Background(), takeoutDir, outputDir, nil, false)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if result.Succeeded != 1 {
		t.Fatalf("expected 1 file, got %d", result.Succeeded)
	}

	// Verify filesystem timestamp was set from JSON sidecar
	info, err := os.Stat(filepath.Join(outputDir, "Trip", "photo.jpg"))
	if err != nil {
		t.Fatal("photo.jpg not found")
	}
	wantTime := time.Unix(1640000000, 0)
	if !info.ModTime().Equal(wantTime) {
		t.Errorf("ModTime = %v, want %v", info.ModTime(), wantTime)
	}
}

func TestImportTakeout_NoSidecarNoMetadata(t *testing.T) {
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()

	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/Trip/photo.jpg": "jpegdata",
		// No JSON sidecar
	})

	result, err := ImportTakeout(context.Background(), takeoutDir, outputDir, nil, false)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if result.Succeeded != 1 {
		t.Fatalf("expected 1 file, got %d", result.Succeeded)
	}

	// File should still be extracted even without sidecar
	if _, err := os.Stat(filepath.Join(outputDir, "Trip", "photo.jpg")); err != nil {
		t.Error("photo.jpg not found")
	}
}

func TestImportTakeout_ZeroTimestampUsesZipModTime(t *testing.T) {
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()

	// Create a zip with custom modification times using zip.CreateHeader
	zipPath := filepath.Join(takeoutDir, "takeout.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)

	customTime := time.Date(2020, 6, 15, 12, 0, 0, 0, time.UTC)

	// Add media file with custom mod time
	header := &zip.FileHeader{
		Name:     "Google Photos/Trip/photo.jpg",
		Method:   zip.Deflate,
		Modified: customTime,
	}
	fw, err := w.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte("jpegdata")); err != nil {
		t.Fatal(err)
	}

	// Add JSON sidecar with zero timestamp
	header2 := &zip.FileHeader{
		Name:     "Google Photos/Trip/photo.jpg.json",
		Method:   zip.Deflate,
		Modified: customTime,
	}
	fw2, err := w.CreateHeader(header2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw2.Write([]byte(`{"title":"Photo","photoTakenTime":{"timestamp":"0"}}`)); err != nil {
		t.Fatal(err)
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	result, err := ImportTakeout(context.Background(), takeoutDir, outputDir, nil, false)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if result.Succeeded != 1 {
		t.Fatalf("expected 1 file, got %d", result.Succeeded)
	}

	// With zero timestamp in sidecar, should fall back to zip entry mod time
	info, err := os.Stat(filepath.Join(outputDir, "Trip", "photo.jpg"))
	if err != nil {
		t.Fatal("photo.jpg not found")
	}
	// The file's mod time should be set to the zip entry's modification time
	if !info.ModTime().Equal(customTime) {
		t.Errorf("ModTime = %v, want %v (zip entry mod time)", info.ModTime(), customTime)
	}
}
