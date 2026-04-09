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

func TestImportTakeout_TakeoutPrefixExtractsMedia(t *testing.T) {
	// Regression: real Google Takeout zips use "Takeout/Google Photos/..." paths.
	// Previously scanOneZip only matched "Google Photos/", silently skipping all entries.
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()

	jpegData := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x02, 0x00, 0x00, 0xFF, 0xD9}

	createTestZip(t, takeoutDir, map[string]string{
		"Takeout/Google Photos/Trip/photo1.jpg":              string(jpegData),
		"Takeout/Google Photos/Trip/photo1.jpg.json":         `{"title":"Beach","photoTakenTime":{"timestamp":"1640000000"}}`,
		"Takeout/Google Photos/Trip/video.mp4":               "mp4data",
		"Takeout/Google Photos/Photos from 2022/other.jpg":   string(jpegData),
		"Takeout/Google Photos/Trip/metadata.json":           `{"albums":[]}`,
	})

	result, err := ImportTakeout(context.Background(), takeoutDir, outputDir, nil, false)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	if result.Succeeded != 3 {
		t.Fatalf("expected 3 files extracted, got %d", result.Succeeded)
	}

	// Album media goes into subdirectory
	if _, err := os.Stat(filepath.Join(outputDir, "Trip", "photo1.jpg")); err != nil {
		t.Error("photo1.jpg not found in Trip/ subdirectory")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "Trip", "video.mp4")); err != nil {
		t.Error("video.mp4 not found in Trip/ subdirectory")
	}

	// Year folder media is flattened to output root
	if _, err := os.Stat(filepath.Join(outputDir, "other.jpg")); err != nil {
		t.Error("other.jpg not found in output root (year folder should be flattened)")
	}

	// Verify metadata was applied from sidecar
	destPath := filepath.Join(outputDir, "Trip", "photo1.jpg")
	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatal("reading photo1.jpg:", err)
	}
	if !strings.Contains(string(data), "Beach") {
		t.Error("XMP metadata should contain title 'Beach' from sidecar")
	}
	info, err := os.Stat(destPath)
	if err != nil {
		t.Fatal(err)
	}
	wantTime := time.Unix(1640000000, 0)
	if !info.ModTime().Equal(wantTime) {
		t.Errorf("ModTime = %v, want %v", info.ModTime(), wantTime)
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

func TestImportTakeout_SkipsAlreadyImported(t *testing.T) {
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()

	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/Album/photo.jpg": "jpegdata",
	})

	// Pre-populate the import log as if the file was already imported.
	logPath := filepath.Join(outputDir, ".photo-copy-import.log")
	if err := os.WriteFile(logPath, []byte("Album/photo.jpg\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ImportTakeout(context.Background(), takeoutDir, outputDir, nil, false)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	if result.Succeeded != 0 {
		t.Errorf("expected 0 succeeded (file should be skipped), got %d", result.Succeeded)
	}
	if result.Skipped != 1 {
		t.Errorf("expected 1 skipped, got %d", result.Skipped)
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

func TestImportTakeout_CollisionLogsActualPath(t *testing.T) {
	// When a collision renames photo.jpg to photo_1.jpg, the import log must
	// record the renamed path so reruns skip correctly.
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()

	// Pre-create a file to force a collision.
	if err := os.MkdirAll(filepath.Join(outputDir, "Album"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "Album", "photo.jpg"), []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}

	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/Album/photo.jpg": "new data",
	})

	result, err := ImportTakeout(context.Background(), takeoutDir, outputDir, nil, true)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if result.Succeeded != 1 {
		t.Fatalf("expected 1 succeeded, got %d", result.Succeeded)
	}

	// Import log should contain the renamed path, not the original.
	logData, err := os.ReadFile(filepath.Join(outputDir, ".photo-copy-import.log"))
	if err != nil {
		t.Fatal("import log not found")
	}
	logContent := string(logData)
	if !strings.Contains(logContent, "Album/photo_1.jpg") {
		t.Errorf("import log should contain renamed path 'Album/photo_1.jpg', got: %q", logContent)
	}
	if strings.Contains(logContent, "Album/photo.jpg\n") {
		t.Error("import log should NOT contain original path 'Album/photo.jpg' for a collision-renamed file")
	}

	// Second run: the source entry (Album/photo.jpg) is not in the log (it was
	// renamed), so it will be re-imported and collision-renamed to photo_2.jpg.
	// But photo_1.jpg is in the log and would be skipped if encountered again.
	_, err = ImportTakeout(context.Background(), takeoutDir, outputDir, nil, true)
	if err != nil {
		t.Fatalf("second import failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "Album", "photo_2.jpg")); err != nil {
		t.Error("photo_2.jpg should exist from second run collision rename")
	}
}

func TestImportTakeout_RerunSkipsAllFiles(t *testing.T) {
	// Uses a valid JPEG with a matching sidecar so metadata embedding actually
	// changes the file size, exercising the path that motivated the log-based skip.
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()

	// Minimal valid JPEG that jpegmeta.SetMetadata can write XMP into.
	jpegData := string([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x02, 0x00, 0x00, 0xFF, 0xD9})

	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/Trip/photo.jpg":                             jpegData,
		"Google Photos/Trip/photo.jpg.supplemental-metadata.json":  `{"title":"Beach","description":"Nice day","photoTakenTime":{"timestamp":"1640000000"}}`,
		"Google Photos/Photos from 2022/other.jpg":                 jpegData,
		"Google Photos/Photos from 2022/other.jpg.supplemental-metadata.json": `{"title":"Park","photoTakenTime":{"timestamp":"1650000000"}}`,
	})

	// First run: extract everything with metadata enabled (the default path).
	result, err := ImportTakeout(context.Background(), takeoutDir, outputDir, nil, false)
	if err != nil {
		t.Fatalf("first import failed: %v", err)
	}
	if result.Succeeded != 2 {
		t.Fatalf("first run: expected 2 succeeded, got %d", result.Succeeded)
	}

	// Verify metadata was actually embedded (file size changed from zip entry).
	tripPhoto := filepath.Join(outputDir, "Trip", "photo.jpg")
	info, err := os.Stat(tripPhoto)
	if err != nil {
		t.Fatalf("photo.jpg not found: %v", err)
	}
	if info.Size() == int64(len(jpegData)) {
		t.Fatal("metadata embedding did not change file size — test is not exercising the right path")
	}

	// Import log should have been created.
	logPath := filepath.Join(outputDir, ".photo-copy-import.log")
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("import log not created: %v", err)
	}

	// Second run: everything should be skipped via import log.
	result, err = ImportTakeout(context.Background(), takeoutDir, outputDir, nil, false)
	if err != nil {
		t.Fatalf("second import failed: %v", err)
	}
	if result.Succeeded != 0 {
		t.Errorf("second run: expected 0 succeeded, got %d", result.Succeeded)
	}
	if result.Skipped != 2 {
		t.Errorf("second run: expected 2 skipped, got %d", result.Skipped)
	}

	// No duplicate files should have been created.
	entries, _ := os.ReadDir(filepath.Join(outputDir, "Trip"))
	for _, e := range entries {
		if strings.Contains(e.Name(), "_1") {
			t.Errorf("unexpected duplicate file: %s", e.Name())
		}
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

func TestImportTakeout_SupplementalMetadataSidecar(t *testing.T) {
	// Regression: newer Google Takeout exports use ".supplemental-metadata.json"
	// instead of ".json" for sidecar files.
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()

	jpegData := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x02, 0x00, 0x00, 0xFF, 0xD9}

	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/Trip/photo.jpg":                                string(jpegData),
		"Google Photos/Trip/photo.jpg.supplemental-metadata.json":    `{"title":"Beach","description":"Sunny","photoTakenTime":{"timestamp":"1640000000"}}`,
	})

	result, err := ImportTakeout(context.Background(), takeoutDir, outputDir, nil, false)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if result.Succeeded != 1 {
		t.Fatalf("expected 1 file, got %d", result.Succeeded)
	}

	destPath := filepath.Join(outputDir, "Trip", "photo.jpg")

	// Verify filesystem timestamp was set from sidecar.
	info, err := os.Stat(destPath)
	if err != nil {
		t.Fatal("photo.jpg not found")
	}
	wantTime := time.Unix(1640000000, 0)
	if !info.ModTime().Equal(wantTime) {
		t.Errorf("ModTime = %v, want %v", info.ModTime(), wantTime)
	}

	// Verify XMP metadata was embedded.
	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatal("reading photo.jpg:", err)
	}
	content := string(data)
	if !strings.Contains(content, "Beach") {
		t.Error("XMP metadata should contain title 'Beach'")
	}
	if !strings.Contains(content, "Sunny") {
		t.Error("XMP metadata should contain description 'Sunny'")
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

	destPath := filepath.Join(outputDir, "Trip", "photo.jpg")

	// Verify filesystem timestamp was set from JSON sidecar
	info, err := os.Stat(destPath)
	if err != nil {
		t.Fatal("photo.jpg not found")
	}
	wantTime := time.Unix(1640000000, 0)
	if !info.ModTime().Equal(wantTime) {
		t.Errorf("ModTime = %v, want %v", info.ModTime(), wantTime)
	}

	// Verify XMP metadata was embedded in the JPEG
	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatal("reading photo.jpg:", err)
	}
	content := string(data)
	if !strings.Contains(content, "Beach") {
		t.Error("XMP metadata should contain title 'Beach'")
	}
	if !strings.Contains(content, "Nice day") {
		t.Error("XMP metadata should contain description 'Nice day'")
	}
}

func TestImportTakeout_MetadataFromCrossZipSidecar(t *testing.T) {
	// Media in one zip, JSON sidecar in another — metadata should still be applied.
	dir := t.TempDir()
	outputDir := t.TempDir()

	// Minimal valid JPEG for XMP embedding
	jpegData := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x02, 0x00, 0x00, 0xFF, 0xD9}

	createTestZip(t, dir, map[string]string{
		"Google Photos/Trip/photo.jpg": string(jpegData),
	})
	_ = os.Rename(filepath.Join(dir, "takeout.zip"), filepath.Join(dir, "takeout-001.zip"))
	createTestZip(t, dir, map[string]string{
		"Google Photos/Trip/photo.jpg.json": `{"title":"Cross-Zip Title","description":"Split archive","photoTakenTime":{"timestamp":"1640000000"}}`,
	})
	_ = os.Rename(filepath.Join(dir, "takeout.zip"), filepath.Join(dir, "takeout-002.zip"))

	result, err := ImportTakeout(context.Background(), dir, outputDir, nil, false)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if result.Succeeded != 1 {
		t.Fatalf("expected 1 file, got %d", result.Succeeded)
	}

	destPath := filepath.Join(outputDir, "Trip", "photo.jpg")

	// Verify filesystem timestamp was set from cross-zip sidecar
	info, err := os.Stat(destPath)
	if err != nil {
		t.Fatal("photo.jpg not found")
	}
	wantTime := time.Unix(1640000000, 0)
	if !info.ModTime().Equal(wantTime) {
		t.Errorf("ModTime = %v, want %v", info.ModTime(), wantTime)
	}

	// Verify XMP metadata was embedded from the cross-zip sidecar
	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatal("reading photo.jpg:", err)
	}
	content := string(data)
	if !strings.Contains(content, "Cross-Zip Title") {
		t.Error("XMP metadata should contain title 'Cross-Zip Title' from cross-zip sidecar")
	}
	if !strings.Contains(content, "Split archive") {
		t.Error("XMP metadata should contain description 'Split archive' from cross-zip sidecar")
	}
}

func TestImportTakeout_CrossZipSidecarWithDuplicatePaths(t *testing.T) {
	// Both zips contain the same sidecar path but with different content.
	// The scanner records which zip the sidecar was matched from; extraction
	// must read from the recorded zip, not from whichever zip happens to
	// contain the same entry name.
	dir := t.TempDir()
	outputDir := t.TempDir()

	jpegData := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x02, 0x00, 0x00, 0xFF, 0xD9}

	// Zip 1: has both the media and a sidecar with title "Zip1 Title"
	createTestZip(t, dir, map[string]string{
		"Google Photos/Trip/photo.jpg":      string(jpegData),
		"Google Photos/Trip/photo.jpg.json": `{"title":"Zip1 Title","photoTakenTime":{"timestamp":"1640000000"}}`,
	})
	_ = os.Rename(filepath.Join(dir, "takeout.zip"), filepath.Join(dir, "takeout-001.zip"))

	// Zip 2: has the same sidecar path but different content
	createTestZip(t, dir, map[string]string{
		"Google Photos/Trip/photo.jpg.json": `{"title":"Zip2 Title","photoTakenTime":{"timestamp":"1650000000"}}`,
	})
	_ = os.Rename(filepath.Join(dir, "takeout.zip"), filepath.Join(dir, "takeout-002.zip"))

	result, err := ImportTakeout(context.Background(), dir, outputDir, nil, false)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if result.Succeeded != 1 {
		t.Fatalf("expected 1 file, got %d", result.Succeeded)
	}

	// The media is in zip 1, and the scanner matches the sidecar from zip 1
	// (same zip as the media, scanned first). Verify zip 1's metadata is used.
	destPath := filepath.Join(outputDir, "Trip", "photo.jpg")
	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatal("reading photo.jpg:", err)
	}
	content := string(data)
	if !strings.Contains(content, "Zip1 Title") {
		t.Error("should use sidecar from the same zip as the media (Zip1 Title)")
	}
	if strings.Contains(content, "Zip2 Title") {
		t.Error("should NOT use sidecar from a different zip (Zip2 Title)")
	}
}

func TestImportTakeout_CrossZipSidecarMediaInLaterZip(t *testing.T) {
	// Media in zip 2, both zips have the same sidecar path.
	// The sidecar from zip 2 (same zip as media) should be used.
	dir := t.TempDir()
	outputDir := t.TempDir()

	jpegData := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x02, 0x00, 0x00, 0xFF, 0xD9}

	// Zip 1: only has a sidecar (different content than zip 2)
	createTestZip(t, dir, map[string]string{
		"Google Photos/Trip/photo.jpg.json": `{"title":"Zip1 Title","photoTakenTime":{"timestamp":"1640000000"}}`,
	})
	_ = os.Rename(filepath.Join(dir, "takeout.zip"), filepath.Join(dir, "takeout-001.zip"))

	// Zip 2: has both the media and a sidecar
	createTestZip(t, dir, map[string]string{
		"Google Photos/Trip/photo.jpg":      string(jpegData),
		"Google Photos/Trip/photo.jpg.json": `{"title":"Zip2 Title","photoTakenTime":{"timestamp":"1650000000"}}`,
	})
	_ = os.Rename(filepath.Join(dir, "takeout.zip"), filepath.Join(dir, "takeout-002.zip"))

	result, err := ImportTakeout(context.Background(), dir, outputDir, nil, false)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if result.Succeeded != 1 {
		t.Fatalf("expected 1 file, got %d", result.Succeeded)
	}

	destPath := filepath.Join(outputDir, "Trip", "photo.jpg")
	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatal("reading photo.jpg:", err)
	}
	content := string(data)
	if !strings.Contains(content, "Zip2 Title") {
		t.Error("should use sidecar from the same zip as the media (Zip2 Title)")
	}
	if strings.Contains(content, "Zip1 Title") {
		t.Error("should NOT use sidecar from a different zip (Zip1 Title)")
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
