//go:build integration

package cli

import (
	"os"
	"strings"
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

func TestNoOpWarnings_NoMetadataOnUpload(t *testing.T) {
	inputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	mock := mockserver.NewFlickr(t).
		OnUpload(mockserver.RespondStatus(200)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_UPLOAD_URL", mock.UploadURL)

	// Run upload with --no-metadata on an empty dir; should not crash
	err := executeCmd(t, "flickr", "upload", "--no-metadata", inputDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
