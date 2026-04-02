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
		"Google Photos/Trip/photo1.jpg":                "jpegdata",
		"Google Photos/Trip/photo1.jpg.json":           `{"title":"photo1"}`,
		"Google Photos/Trip/video.mp4":                 "mp4data",
		"Google Photos/Trip/metadata.json":             `{"albums":[]}`,
		"Google Photos/Trip/print-subscriptions.json":  `{}`,
	})

	result, err := ImportTakeout(context.Background(), takeoutDir, outputDir, nil)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	if result.Succeeded != 2 {
		t.Fatalf("expected 2 files extracted, got %d", result.Succeeded)
	}

	if _, err := os.Stat(filepath.Join(outputDir, "photo1.jpg")); err != nil {
		t.Fatal("photo1.jpg not found in output")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "video.mp4")); err != nil {
		t.Fatal("video.mp4 not found in output")
	}
}

func TestImportTakeout_SkipsNonMedia(t *testing.T) {
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()

	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/readme.html": "<html>hi</html>",
		"Google Photos/data.json":   "{}",
	})

	result, err := ImportTakeout(context.Background(), takeoutDir, outputDir, nil)
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
		"Google Photos/a.jpg": "data1",
	})
	_ = os.Rename(filepath.Join(takeoutDir, "takeout.zip"), filepath.Join(takeoutDir, "takeout-001.zip"))

	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/b.png": "data2",
	})
	_ = os.Rename(filepath.Join(takeoutDir, "takeout.zip"), filepath.Join(takeoutDir, "takeout-002.zip"))

	result, err := ImportTakeout(context.Background(), takeoutDir, outputDir, nil)
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

	// Pre-create a file that will collide with the zip's photo.jpg
	if err := os.WriteFile(filepath.Join(outputDir, "photo.jpg"), []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}

	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/Album/photo.jpg": "new data",
	})

	result, err := ImportTakeout(context.Background(), takeoutDir, outputDir, nil)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	if result.Succeeded != 1 {
		t.Fatalf("expected 1 file extracted, got %d", result.Succeeded)
	}

	// Original file should be untouched
	data, err := os.ReadFile(filepath.Join(outputDir, "photo.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "existing" {
		t.Errorf("original file was overwritten, got %q", string(data))
	}

	// New file should be renamed to photo_1.jpg
	data, err = os.ReadFile(filepath.Join(outputDir, "photo_1.jpg"))
	if err != nil {
		t.Fatal("photo_1.jpg not found — duplicate rename did not work")
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

	_, err := ImportTakeout(ctx, takeoutDir, outputDir, nil)
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

	result, err := ImportTakeout(ctx, takeoutDir, outputDir, nil, withAfterExtract(afterExtract))
	if err == nil {
		t.Fatal("expected error from cancelled context during extraction")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
	// Verify partial extraction — some files should exist but not all
	entries, _ := os.ReadDir(outputDir)
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

	result, err := ImportTakeout(ctx, takeoutDir, outputDir, nil, withBeforeExtract(beforeExtract))
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
