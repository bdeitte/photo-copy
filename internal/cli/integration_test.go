//go:build integration

package cli

import (
	"archive/zip"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/testutil/mockserver"
)

// testImageData is fake image content used across tests.
var testImageData = []byte("fake-jpeg-data-for-testing")

// setupFlickrConfig writes a test Flickr config to the given dir.
func setupFlickrConfig(t *testing.T, configDir string) {
	t.Helper()
	err := config.SaveFlickrConfig(configDir, &config.FlickrConfig{
		APIKey:           "test-key",
		APISecret:        "test-secret",
		OAuthToken:       "test-token",
		OAuthTokenSecret: "test-token-secret",
	})
	if err != nil {
		t.Fatalf("saving test flickr config: %v", err)
	}
}

// setupGoogleConfig writes a test Google config to the given dir.
func setupGoogleConfig(t *testing.T, configDir string) {
	t.Helper()
	err := config.SaveGoogleConfig(configDir, &config.GoogleConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
	})
	if err != nil {
		t.Fatalf("saving test google config: %v", err)
	}
}

// setTestEnv sets the common test env vars.
func setTestEnv(t *testing.T, configDir string) {
	t.Helper()
	t.Setenv("PHOTO_COPY_CONFIG_DIR", configDir)
	t.Setenv("PHOTO_COPY_TEST_MODE", "1")
}

// executeCmd creates a new root command, sets args, and executes it.
func executeCmd(t *testing.T, args ...string) error {
	t.Helper()
	cmd := NewRootCmd()
	cmd.SetArgs(args)
	// Suppress cobra's own error printing
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	return cmd.Execute()
}

// readLines reads a file and returns non-empty lines.
func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, strings.TrimSpace(line))
		}
	}
	return lines
}

// flickrPhotosResponse builds a Flickr getPhotos JSON response.
func flickrPhotosResponse(photos []map[string]string, page, pages, total int) map[string]any {
	return map[string]any{
		"photos": map[string]any{
			"page":  page,
			"pages": pages,
			"total": total,
			"photo": photos,
		},
		"stat": "ok",
	}
}

// flickrSizesResponse builds a Flickr getSizes JSON response.
func flickrSizesResponse(sourceURL string) map[string]any {
	return map[string]any{
		"sizes": map[string]any{
			"size": []map[string]string{
				{"label": "Original", "source": sourceURL},
			},
		},
		"stat": "ok",
	}
}

// --- Flickr Download Tests ---

func TestFlickrDownload_HappyPath(t *testing.T) {
	outputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	photos := []map[string]string{
		{"id": "1", "secret": "aaa", "server": "1", "title": "photo1"},
		{"id": "2", "secret": "bbb", "server": "1", "title": "photo2"},
		{"id": "3", "secret": "ccc", "server": "1", "title": "photo3"},
	}

	var mock *mockserver.FlickrMock
	mock = mockserver.NewFlickr(t).
		OnGetPhotos(mockserver.RespondJSON(200, flickrPhotosResponse(photos, 1, 1, 3))).
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

	// Verify 3 files downloaded
	for _, p := range photos {
		filename := fmt.Sprintf("%s_%s.jpg", p["id"], p["secret"])
		data, err := os.ReadFile(filepath.Join(outputDir, filename))
		if err != nil {
			t.Errorf("expected file %s: %v", filename, err)
			continue
		}
		if string(data) != string(testImageData) {
			t.Errorf("file %s content mismatch", filename)
		}
	}

	// Verify transfer log
	logLines := readLines(t, filepath.Join(outputDir, "transfer.log"))
	if len(logLines) != 3 {
		t.Errorf("transfer log has %d entries, want 3", len(logLines))
	}
}

func TestFlickrDownload_Pagination(t *testing.T) {
	outputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	page1Photos := []map[string]string{
		{"id": "1", "secret": "aaa", "server": "1", "title": "p1"},
		{"id": "2", "secret": "bbb", "server": "1", "title": "p2"},
	}
	page2Photos := []map[string]string{
		{"id": "3", "secret": "ccc", "server": "1", "title": "p3"},
	}

	var mock *mockserver.FlickrMock
	mock = mockserver.NewFlickr(t).
		OnGetPhotos(mockserver.RespondSequence(
			mockserver.RespondJSON(200, flickrPhotosResponse(page1Photos, 1, 2, 3)),
			mockserver.RespondJSON(200, flickrPhotosResponse(page2Photos, 2, 2, 3)),
		)).
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

	// Verify all 3 files from 2 pages
	for _, id := range []string{"1_aaa", "2_bbb", "3_ccc"} {
		if _, err := os.Stat(filepath.Join(outputDir, id+".jpg")); err != nil {
			t.Errorf("missing file %s.jpg", id)
		}
	}
}

func TestFlickrDownload_ResumesFromLog(t *testing.T) {
	outputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	// Pre-populate transfer log with one file
	_ = os.WriteFile(filepath.Join(outputDir, "transfer.log"), []byte("1_aaa.jpg\n"), 0644)

	photos := []map[string]string{
		{"id": "1", "secret": "aaa", "server": "1", "title": "p1"},
		{"id": "2", "secret": "bbb", "server": "1", "title": "p2"},
	}

	var mock *mockserver.FlickrMock
	mock = mockserver.NewFlickr(t).
		OnGetPhotos(mockserver.RespondJSON(200, flickrPhotosResponse(photos, 1, 1, 2))).
		OnGetSizes(func(w http.ResponseWriter, r *http.Request) {
			photoID := r.URL.Query().Get("photo_id")
			if photoID == "1" {
				t.Error("getSizes should not be called for already-downloaded photo 1")
			}
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

	// Only photo 2 should be downloaded
	if _, err := os.Stat(filepath.Join(outputDir, "2_bbb.jpg")); err != nil {
		t.Error("photo 2 should have been downloaded")
	}
	// Photo 1 file should NOT exist (was in log but not re-downloaded)
	if _, err := os.Stat(filepath.Join(outputDir, "1_aaa.jpg")); err == nil {
		t.Error("photo 1 should not have been re-downloaded")
	}

	// Transfer log should now have both entries
	logLines := readLines(t, filepath.Join(outputDir, "transfer.log"))
	if len(logLines) != 2 {
		t.Errorf("transfer log has %d entries, want 2", len(logLines))
	}
}

func TestFlickrDownload_RetryOn429(t *testing.T) {
	outputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	photos := []map[string]string{
		{"id": "1", "secret": "aaa", "server": "1", "title": "p1"},
	}

	var mock *mockserver.FlickrMock
	mock = mockserver.NewFlickr(t).
		OnGetPhotos(mockserver.RespondJSON(200, flickrPhotosResponse(photos, 1, 1, 1))).
		OnGetSizes(mockserver.RespondSequence(
			mockserver.RespondStatus(429),
			func(w http.ResponseWriter, r *http.Request) {
				photoID := r.URL.Query().Get("photo_id")
				mockserver.RespondJSON(200, flickrSizesResponse(
					mock.Server.URL+"/download/"+photoID+".jpg",
				))(w, r)
			},
		)).
		OnDownload(mockserver.RespondBytes(200, testImageData)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_API_URL", mock.APIURL)

	err := executeCmd(t, "flickr", "download", outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outputDir, "1_aaa.jpg")); err != nil {
		t.Error("photo should have been downloaded after retry")
	}
}

func TestFlickrDownload_RetryOn5xx(t *testing.T) {
	outputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	photos := []map[string]string{
		{"id": "1", "secret": "aaa", "server": "1", "title": "p1"},
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
		OnDownload(mockserver.RespondSequence(
			mockserver.RespondStatus(500),
			mockserver.RespondBytes(200, testImageData),
		)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_API_URL", mock.APIURL)

	err := executeCmd(t, "flickr", "download", outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outputDir, "1_aaa.jpg"))
	if err != nil {
		t.Fatal("photo should have been downloaded after 5xx retry")
	}
	if string(data) != string(testImageData) {
		t.Error("downloaded file content mismatch")
	}
}

func TestFlickrDownload_LimitFlag(t *testing.T) {
	outputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	photos := []map[string]string{
		{"id": "1", "secret": "aaa", "server": "1", "title": "p1"},
		{"id": "2", "secret": "bbb", "server": "1", "title": "p2"},
		{"id": "3", "secret": "ccc", "server": "1", "title": "p3"},
		{"id": "4", "secret": "ddd", "server": "1", "title": "p4"},
		{"id": "5", "secret": "eee", "server": "1", "title": "p5"},
	}

	var mock *mockserver.FlickrMock
	mock = mockserver.NewFlickr(t).
		OnGetPhotos(mockserver.RespondJSON(200, flickrPhotosResponse(photos, 1, 1, 5))).
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

	err := executeCmd(t, "flickr", "download", "--limit", "2", outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Count downloaded files (excluding transfer.log)
	entries, _ := os.ReadDir(outputDir)
	count := 0
	for _, e := range entries {
		if e.Name() != "transfer.log" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("downloaded %d files, want 2", count)
	}
}

// --- Flickr Upload Tests ---

func TestFlickrUpload_HappyPath(t *testing.T) {
	inputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	// Create test media files
	_ = os.WriteFile(filepath.Join(inputDir, "photo1.jpg"), testImageData, 0644)
	_ = os.WriteFile(filepath.Join(inputDir, "photo2.jpg"), []byte("jpeg-data-2"), 0644)

	mock := mockserver.NewFlickr(t).
		OnUpload(mockserver.RespondStatus(200)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_UPLOAD_URL", mock.UploadURL)

	err := executeCmd(t, "flickr", "upload", inputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify 2 upload requests were made
	uploadRequests := 0
	for _, req := range mock.Requests() {
		if strings.HasPrefix(req.Path, "/services/upload/") {
			uploadRequests++
		}
	}
	if uploadRequests != 2 {
		t.Errorf("got %d upload requests, want 2", uploadRequests)
	}
}

func TestFlickrUpload_SkipsNonMedia(t *testing.T) {
	inputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	_ = os.WriteFile(filepath.Join(inputDir, "photo.jpg"), testImageData, 0644)
	_ = os.WriteFile(filepath.Join(inputDir, "readme.txt"), []byte("not media"), 0644)
	_ = os.WriteFile(filepath.Join(inputDir, "data.csv"), []byte("1,2,3"), 0644)

	mock := mockserver.NewFlickr(t).
		OnUpload(mockserver.RespondStatus(200)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_UPLOAD_URL", mock.UploadURL)

	err := executeCmd(t, "flickr", "upload", inputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	uploadRequests := 0
	for _, req := range mock.Requests() {
		if strings.HasPrefix(req.Path, "/services/upload/") {
			uploadRequests++
		}
	}
	if uploadRequests != 1 {
		t.Errorf("got %d upload requests, want 1 (only .jpg)", uploadRequests)
	}
}

func TestFlickrUpload_LimitFlag(t *testing.T) {
	inputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	_ = os.WriteFile(filepath.Join(inputDir, "a.jpg"), testImageData, 0644)
	_ = os.WriteFile(filepath.Join(inputDir, "b.jpg"), testImageData, 0644)
	_ = os.WriteFile(filepath.Join(inputDir, "c.jpg"), testImageData, 0644)

	mock := mockserver.NewFlickr(t).
		OnUpload(mockserver.RespondStatus(200)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_UPLOAD_URL", mock.UploadURL)

	err := executeCmd(t, "flickr", "upload", "--limit", "1", inputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	uploadRequests := 0
	for _, req := range mock.Requests() {
		if strings.HasPrefix(req.Path, "/services/upload/") {
			uploadRequests++
		}
	}
	if uploadRequests != 1 {
		t.Errorf("got %d upload requests, want 1", uploadRequests)
	}
}

func TestFlickrUpload_FailsOnError(t *testing.T) {
	inputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	_ = os.WriteFile(filepath.Join(inputDir, "photo.jpg"), testImageData, 0644)

	mock := mockserver.NewFlickr(t).
		OnUpload(mockserver.RespondStatus(500)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_UPLOAD_URL", mock.UploadURL)

	err := executeCmd(t, "flickr", "upload", inputDir)
	if err == nil {
		t.Fatal("expected error on upload failure")
	}
	if !strings.Contains(err.Error(), "upload failed HTTP 500") {
		t.Errorf("error should mention upload failure, got: %v", err)
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

// --- Google Import Takeout Tests ---

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
		_, _ = fw.Write([]byte(content))
	}
	_ = w.Close()
	_ = f.Close()
}

func TestGoogleImportTakeout_HappyPath(t *testing.T) {
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()

	createTestZip(t, takeoutDir, "takeout.zip", map[string]string{
		"Google Photos/Trip/photo1.jpg": "jpeg-content",
		"Google Photos/Trip/video.mp4":  "mp4-content",
	})

	err := executeCmd(t, "google", "import-takeout", takeoutDir, outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outputDir, "photo1.jpg")); err != nil {
		t.Error("photo1.jpg should have been extracted")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "video.mp4")); err != nil {
		t.Error("video.mp4 should have been extracted")
	}
}

func TestGoogleImportTakeout_FiltersNonMedia(t *testing.T) {
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()

	createTestZip(t, takeoutDir, "takeout.zip", map[string]string{
		"Google Photos/Trip/photo.jpg":      "jpeg-content",
		"Google Photos/Trip/photo.jpg.json": `{"title":"photo"}`,
		"Google Photos/Trip/metadata.json":  `{"albums":[]}`,
	})

	err := executeCmd(t, "google", "import-takeout", takeoutDir, outputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outputDir, "photo.jpg")); err != nil {
		t.Error("photo.jpg should have been extracted")
	}

	// Non-media files should NOT be extracted
	entries, _ := os.ReadDir(outputDir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			t.Errorf("non-media file should not be extracted: %s", e.Name())
		}
	}
}
