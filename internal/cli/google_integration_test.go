//go:build integration

package cli

import (
	"archive/zip"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/briandeitte/photo-copy/internal/testutil/mockserver"
)

// createTestZip creates a zip file in dir with the given files.
func createTestZip(t *testing.T, dir, name string, files map[string]string) {
	t.Helper()
	zipPath := filepath.Join(dir, name)
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for fname, content := range files {
		fw, err := w.Create(fname)
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
}

// --- Google Upload Tests ---

func TestGoogleUpload_HappyPath(t *testing.T) {
	inputDir := t.TempDir()
	configDir := t.TempDir()
	setupGoogleConfig(t, configDir)

	_ = os.WriteFile(filepath.Join(inputDir, "photo1.jpg"), testImageData, 0644)
	_ = os.WriteFile(filepath.Join(inputDir, "photo2.png"), []byte("png-data"), 0644)

	mock := mockserver.NewGoogle(t).
		OnUploadBytes(func(w http.ResponseWriter, r *http.Request) {
			// Return a unique upload token based on the filename header
			filename := r.Header.Get("X-Goog-Upload-File-Name")
			_, _ = w.Write([]byte("token-" + filename))
		}).
		OnBatchCreate(mockserver.RespondJSON(200, map[string]any{})).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_GOOGLE_API_URL", mock.BaseURL)
	t.Setenv("PHOTO_COPY_GOOGLE_TOKEN", "skip")

	err := executeCmd(t, "google", "upload", inputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify upload log written inside inputDir
	logLines := readLines(t, filepath.Join(inputDir, ".photo-copy-upload.log"))
	if len(logLines) != 2 {
		t.Errorf("upload log has %d entries, want 2", len(logLines))
	}

	// Verify upload token was passed to batchCreate
	requests := mock.Requests()
	batchRequests := 0
	for _, req := range requests {
		if req.Path == "/v1/mediaItems:batchCreate" {
			batchRequests++
			if !strings.Contains(string(req.Body), "token-") {
				t.Error("batchCreate request should contain upload token")
			}
		}
	}
	if batchRequests != 2 {
		t.Errorf("got %d batchCreate requests, want 2", batchRequests)
	}
}

func TestGoogleUpload_ResumesFromLog(t *testing.T) {
	inputDir := t.TempDir()
	configDir := t.TempDir()
	setupGoogleConfig(t, configDir)

	_ = os.WriteFile(filepath.Join(inputDir, "photo1.jpg"), testImageData, 0644)
	_ = os.WriteFile(filepath.Join(inputDir, "photo2.jpg"), []byte("data2"), 0644)
	// Pre-populate upload log — photo1 already uploaded
	_ = os.WriteFile(filepath.Join(inputDir, ".photo-copy-upload.log"), []byte("photo1.jpg\n"), 0644)

	mock := mockserver.NewGoogle(t).
		OnUploadBytes(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("upload-token"))
		}).
		OnBatchCreate(mockserver.RespondJSON(200, map[string]any{})).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_GOOGLE_API_URL", mock.BaseURL)
	t.Setenv("PHOTO_COPY_GOOGLE_TOKEN", "skip")

	err := executeCmd(t, "google", "upload", inputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only 1 upload should have happened (photo2 only)
	uploadRequests := 0
	for _, req := range mock.Requests() {
		if req.Path == "/v1/uploads" {
			uploadRequests++
		}
	}
	if uploadRequests != 1 {
		t.Errorf("got %d upload requests, want 1 (photo1 should be skipped)", uploadRequests)
	}
}

func TestGoogleUpload_RetryOnUpload429(t *testing.T) {
	inputDir := t.TempDir()
	configDir := t.TempDir()
	setupGoogleConfig(t, configDir)

	_ = os.WriteFile(filepath.Join(inputDir, "photo.jpg"), testImageData, 0644)

	mock := mockserver.NewGoogle(t).
		OnUploadBytes(mockserver.RespondSequence(
			mockserver.RespondStatus(429),
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("upload-token-after-retry"))
			},
		)).
		OnBatchCreate(mockserver.RespondJSON(200, map[string]any{})).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_GOOGLE_API_URL", mock.BaseURL)
	t.Setenv("PHOTO_COPY_GOOGLE_TOKEN", "skip")

	err := executeCmd(t, "google", "upload", inputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	logLines := readLines(t, filepath.Join(inputDir, ".photo-copy-upload.log"))
	if len(logLines) != 1 {
		t.Errorf("upload log has %d entries, want 1", len(logLines))
	}
}

func TestGoogleUpload_PartialFailure(t *testing.T) {
	inputDir := t.TempDir()
	configDir := t.TempDir()
	setupGoogleConfig(t, configDir)

	// Filenames are alphabetically ordered so os.ReadDir processes a_good before b_bad
	_ = os.WriteFile(filepath.Join(inputDir, "a_good.jpg"), testImageData, 0644)
	_ = os.WriteFile(filepath.Join(inputDir, "b_bad.jpg"), []byte("data2"), 0644)

	var mu sync.Mutex
	fileCount := 0
	mock := mockserver.NewGoogle(t).
		OnUploadBytes(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("token"))
		}).
		OnBatchCreate(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			n := fileCount
			fileCount++
			mu.Unlock()
			if n == 0 {
				// First file succeeds
				mockserver.RespondJSON(200, map[string]any{})(w, r)
			} else {
				// Second file fails persistently
				mockserver.RespondStatus(500)(w, r)
			}
		}).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_GOOGLE_API_URL", mock.BaseURL)
	t.Setenv("PHOTO_COPY_GOOGLE_TOKEN", "skip")

	// Command should not return error — Google upload continues past failures
	err := executeCmd(t, "google", "upload", inputDir)
	if err != nil {
		t.Fatalf("unexpected error (should continue past failures): %v", err)
	}

	// Only the successful file should be in the upload log
	logLines := readLines(t, filepath.Join(inputDir, ".photo-copy-upload.log"))
	if len(logLines) != 1 {
		t.Errorf("upload log has %d entries, want 1 (only successful file)", len(logLines))
	}
	if len(logLines) > 0 && logLines[0] != "a_good.jpg" {
		t.Errorf("upload log entry = %q, want a_good.jpg", logLines[0])
	}
}

func TestGoogleUpload_LimitFlag(t *testing.T) {
	inputDir := t.TempDir()
	configDir := t.TempDir()
	setupGoogleConfig(t, configDir)

	_ = os.WriteFile(filepath.Join(inputDir, "a.jpg"), testImageData, 0644)
	_ = os.WriteFile(filepath.Join(inputDir, "b.jpg"), testImageData, 0644)
	_ = os.WriteFile(filepath.Join(inputDir, "c.jpg"), testImageData, 0644)

	mock := mockserver.NewGoogle(t).
		OnUploadBytes(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("token"))
		}).
		OnBatchCreate(mockserver.RespondJSON(200, map[string]any{})).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_GOOGLE_API_URL", mock.BaseURL)
	t.Setenv("PHOTO_COPY_GOOGLE_TOKEN", "skip")

	err := executeCmd(t, "google", "upload", "--limit", "2", inputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	logLines := readLines(t, filepath.Join(inputDir, ".photo-copy-upload.log"))
	if len(logLines) != 2 {
		t.Errorf("upload log has %d entries, want 2", len(logLines))
	}
}

func TestGoogleUpload_NestedSubdirectories(t *testing.T) {
	inputDir := t.TempDir()
	configDir := t.TempDir()
	setupGoogleConfig(t, configDir)

	// Create files in root and a subdirectory
	_ = os.WriteFile(filepath.Join(inputDir, "root.jpg"), testImageData, 0644)
	_ = os.MkdirAll(filepath.Join(inputDir, "album"), 0755)
	_ = os.WriteFile(filepath.Join(inputDir, "album", "nested.jpg"), testImageData, 0644)

	mock := mockserver.NewGoogle(t).
		OnUploadBytes(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("token"))
		}).
		OnBatchCreate(mockserver.RespondJSON(200, map[string]any{})).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_GOOGLE_API_URL", mock.BaseURL)
	t.Setenv("PHOTO_COPY_GOOGLE_TOKEN", "skip")

	err := executeCmd(t, "google", "upload", inputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both files should be uploaded
	uploadRequests := 0
	for _, req := range mock.Requests() {
		if req.Path == "/v1/uploads" {
			uploadRequests++
		}
	}
	if uploadRequests != 2 {
		t.Errorf("got %d upload requests, want 2 (root + nested)", uploadRequests)
	}

	// Upload log should contain relative path for nested file
	logLines := readLines(t, filepath.Join(inputDir, ".photo-copy-upload.log"))
	hasNested := false
	for _, line := range logLines {
		if line == filepath.Join("album", "nested.jpg") {
			hasNested = true
		}
	}
	if !hasNested {
		t.Errorf("upload log should contain relative path %q, got: %v", filepath.Join("album", "nested.jpg"), logLines)
	}
}

func TestGoogleUpload_NoSubdirLogWhenAllUploaded(t *testing.T) {
	inputDir := t.TempDir()
	configDir := t.TempDir()
	setupGoogleConfig(t, configDir)

	// Create a nested file
	_ = os.MkdirAll(filepath.Join(inputDir, "album"), 0755)
	_ = os.WriteFile(filepath.Join(inputDir, "album", "nested.jpg"), testImageData, 0644)

	// Pre-populate upload log — nested file already uploaded
	_ = os.WriteFile(filepath.Join(inputDir, ".photo-copy-upload.log"),
		[]byte(filepath.Join("album", "nested.jpg")+"\n"), 0644)

	mock := mockserver.NewGoogle(t).
		OnUploadBytes(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("token"))
		}).
		OnBatchCreate(mockserver.RespondJSON(200, map[string]any{})).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_GOOGLE_API_URL", mock.BaseURL)
	t.Setenv("PHOTO_COPY_GOOGLE_TOKEN", "skip")

	err := executeCmd(t, "google", "upload", inputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No upload requests should be made — all files were already uploaded
	uploadRequests := 0
	for _, req := range mock.Requests() {
		if req.Path == "/v1/uploads" {
			uploadRequests++
		}
	}
	if uploadRequests != 0 {
		t.Errorf("got %d upload requests, want 0 (all files already uploaded)", uploadRequests)
	}
}

func TestGoogleUpload_DateRange(t *testing.T) {
	inputDir := t.TempDir()
	configDir := t.TempDir()
	setupGoogleConfig(t, configDir)

	// Create files with known modification times
	inRange := filepath.Join(inputDir, "in_range.jpg")
	outRange := filepath.Join(inputDir, "out_range.jpg")
	_ = os.WriteFile(inRange, testImageData, 0644)
	_ = os.WriteFile(outRange, testImageData, 0644)

	// Set mod times: in_range = 2022, out_range = 2019
	_ = os.Chtimes(inRange, time.Now(), time.Date(2022, 6, 15, 10, 0, 0, 0, time.Local))
	_ = os.Chtimes(outRange, time.Now(), time.Date(2019, 1, 1, 10, 0, 0, 0, time.Local))

	mock := mockserver.NewGoogle(t).
		OnUploadBytes(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("token"))
		}).
		OnBatchCreate(mockserver.RespondJSON(200, map[string]any{})).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_GOOGLE_API_URL", mock.BaseURL)
	t.Setenv("PHOTO_COPY_GOOGLE_TOKEN", "skip")

	err := executeCmd(t, "google", "upload", "--date-range", "2021-01-01:2023-12-31", inputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only in_range.jpg should have been uploaded
	logLines := readLines(t, filepath.Join(inputDir, ".photo-copy-upload.log"))
	if len(logLines) != 1 {
		t.Errorf("upload log has %d entries, want 1", len(logLines))
	}
	if len(logLines) > 0 && logLines[0] != "in_range.jpg" {
		t.Errorf("upload log entry = %q, want in_range.jpg", logLines[0])
	}

	// Verify only 1 upload request was made
	uploadRequests := 0
	for _, req := range mock.Requests() {
		if req.Path == "/v1/uploads" {
			uploadRequests++
		}
	}
	if uploadRequests != 1 {
		t.Errorf("got %d upload requests, want 1", uploadRequests)
	}
}

// --- Google Download Tests ---

func TestGoogleDownload_HappyPath(t *testing.T) {
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()

	createTestZip(t, takeoutDir, "takeout.zip", map[string]string{
		"Google Photos/Trip/photo1.jpg": "jpeg-content",
		"Google Photos/Trip/video.mp4":  "mp4-content",
	})

	err := executeCmd(t, "google", "download", takeoutDir, outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outputDir, "Trip", "photo1.jpg")); err != nil {
		t.Error("photo1.jpg should have been extracted in Trip/ subdirectory")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "Trip", "video.mp4")); err != nil {
		t.Error("video.mp4 should have been extracted in Trip/ subdirectory")
	}
}

func TestGoogleDownload_FiltersNonMedia(t *testing.T) {
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()

	createTestZip(t, takeoutDir, "takeout.zip", map[string]string{
		"Google Photos/Trip/photo.jpg":      "jpeg-content",
		"Google Photos/Trip/photo.jpg.json": `{"title":"photo"}`,
		"Google Photos/Trip/metadata.json":  `{"albums":[]}`,
	})

	err := executeCmd(t, "google", "download", takeoutDir, outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outputDir, "Trip", "photo.jpg")); err != nil {
		t.Error("photo.jpg should have been extracted in Trip/ subdirectory")
	}

	// Non-media files should NOT be extracted
	entries, _ := os.ReadDir(filepath.Join(outputDir, "Trip"))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			t.Errorf("non-media file should not be extracted: %s", e.Name())
		}
	}
}

func TestGoogleDownload_DuplicateFilenames(t *testing.T) {
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()

	createTestZip(t, takeoutDir, "takeout.zip", map[string]string{
		"Google Photos/Album1/sunset.jpg": "jpeg-from-album1",
		"Google Photos/Album2/sunset.jpg": "jpeg-from-album2",
	})

	err := executeCmd(t, "google", "download", takeoutDir, outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both files should exist in their respective album subdirectories — no collision
	if _, err := os.Stat(filepath.Join(outputDir, "Album1", "sunset.jpg")); err != nil {
		t.Error("sunset.jpg should have been extracted in Album1/ subdirectory")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "Album2", "sunset.jpg")); err != nil {
		t.Error("sunset.jpg should have been extracted in Album2/ subdirectory")
	}
}

func TestGoogleDownload_MultipleZips(t *testing.T) {
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()

	createTestZip(t, takeoutDir, "takeout-001.zip", map[string]string{
		"Google Photos/Trip/photo1.jpg": "jpeg-from-zip1",
	})
	createTestZip(t, takeoutDir, "takeout-002.zip", map[string]string{
		"Google Photos/Vacation/photo2.jpg": "jpeg-from-zip2",
	})

	err := executeCmd(t, "google", "download", takeoutDir, outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outputDir, "Trip", "photo1.jpg")); err != nil {
		t.Error("photo1.jpg from first zip should have been extracted in Trip/ subdirectory")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "Vacation", "photo2.jpg")); err != nil {
		t.Error("photo2.jpg from second zip should have been extracted in Vacation/ subdirectory")
	}
}

func TestGoogleDownload_NoMetadataFlag(t *testing.T) {
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()

	// Use a minimal valid JPEG so metadata embedding would succeed if attempted
	jpegData := buildMinimalJPEG()
	createTestZip(t, takeoutDir, "takeout.zip", map[string]string{
		"Google Photos/Trip/photo.jpg":      string(jpegData),
		"Google Photos/Trip/photo.jpg.json": `{"title":"Beach","description":"Nice day","photoTakenTime":{"timestamp":"1640000000"}}`,
	})

	err := executeCmd(t, "google", "download", "--no-metadata", takeoutDir, outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outputDir, "Trip", "photo.jpg")); err != nil {
		t.Error("photo.jpg should have been extracted in Trip/ subdirectory")
	}

	// Verify metadata was NOT applied — mtime should not match sidecar timestamp
	info, err := os.Stat(filepath.Join(outputDir, "Trip", "photo.jpg"))
	if err != nil {
		t.Fatal("photo.jpg not found")
	}
	sidecarTime := time.Date(2021, 12, 20, 17, 46, 40, 0, time.UTC)
	if info.ModTime().Equal(sidecarTime) {
		t.Error("file mtime should not match sidecar timestamp when --no-metadata is set")
	}

	// Verify no XMP metadata was embedded
	xmpContent := readXMPFromJPEG(t, filepath.Join(outputDir, "Trip", "photo.jpg"))
	if xmpContent != "" {
		t.Error("XMP metadata should not be embedded when --no-metadata is set")
	}
}

// --- Fatal Error Summary Suppression Tests (Google) ---

func TestGoogleUpload_FatalError_NoSummary(t *testing.T) {
	configDir := t.TempDir()
	setupGoogleConfig(t, configDir)

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_GOOGLE_TOKEN", "skip")

	// Non-existent directory causes Upload to return an error from collectMediaFiles.
	// Before the fix, HandleResult would still be called and could panic or produce
	// a misleading summary. Now the error returns directly.
	nonexistent := filepath.Join(t.TempDir(), "does-not-exist")
	err := executeCmd(t, "google", "upload", nonexistent)
	if err == nil {
		t.Fatal("expected error from non-existent directory")
	}
	if !strings.Contains(err.Error(), "collecting media files") {
		t.Errorf("expected 'collecting media files' error, got: %v", err)
	}
}
