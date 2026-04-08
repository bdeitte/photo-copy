package google

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/logging"
	"golang.org/x/oauth2"
)

func TestLoadUploadLog_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	log, err := loadUploadLog(filepath.Join(tmpDir, "upload.log"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(log) != 0 {
		t.Fatalf("expected empty log, got %d entries", len(log))
	}
}

func TestUploadLog_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "upload.log")

	if err := appendUploadLog(logPath, "photo1.jpg"); err != nil {
		t.Fatalf("append failed: %v", err)
	}
	if err := appendUploadLog(logPath, "photo2.jpg"); err != nil {
		t.Fatalf("append failed: %v", err)
	}

	log, err := loadUploadLog(logPath)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if !log["photo1.jpg"] || !log["photo2.jpg"] {
		t.Fatalf("expected both photos in log, got: %v", log)
	}
}

func TestCollectMediaFiles_IncludesSubdirectories(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "photo.jpg"), []byte("fake"), 0644)
	_ = os.MkdirAll(filepath.Join(tmpDir, "album"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, "album", "nested.jpg"), []byte("fake"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("fake"), 0644)

	files, err := collectMediaFiles(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 media files (including nested), got %d: %v", len(files), files)
	}
}

func TestCollectMediaFiles_ReturnsRelativePaths(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, "sub"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, "sub", "pic.jpg"), []byte("fake"), 0644)

	files, err := collectMediaFiles(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0] != filepath.Join("sub", "pic.jpg") {
		t.Errorf("expected relative path %q, got %q", filepath.Join("sub", "pic.jpg"), files[0])
	}
}

func newTestGoogleClient() *Client {
	return &Client{
		httpClient: &http.Client{},
		log:        logging.New(false, nil),
	}
}

func TestUploadURL_Default(t *testing.T) {
	c := newTestGoogleClient()
	got := c.uploadURL()
	if got != "https://photoslibrary.googleapis.com/v1/uploads" {
		t.Errorf("uploadURL() = %q, want default", got)
	}
}

func TestUploadURL_EnvOverride(t *testing.T) {
	t.Setenv("PHOTO_COPY_GOOGLE_API_URL", "http://localhost:9999")
	c := newTestGoogleClient()
	got := c.uploadURL()
	if got != "http://localhost:9999/v1/uploads" {
		t.Errorf("uploadURL() = %q, want override", got)
	}
}

func TestUploadURL_FieldOverride(t *testing.T) {
	c := newTestGoogleClient()
	c.apiBaseURL = "http://localhost:9999"
	got := c.uploadURL()
	if got != "http://localhost:9999/v1/uploads" {
		t.Errorf("uploadURL() = %q, want field override", got)
	}
}

func TestBatchCreateURL_Default(t *testing.T) {
	c := newTestGoogleClient()
	got := c.batchCreateURL()
	if got != "https://photoslibrary.googleapis.com/v1/mediaItems:batchCreate" {
		t.Errorf("batchCreateURL() = %q, want default", got)
	}
}

func TestBatchCreateURL_EnvOverride(t *testing.T) {
	t.Setenv("PHOTO_COPY_GOOGLE_API_URL", "http://localhost:9999")
	c := newTestGoogleClient()
	got := c.batchCreateURL()
	if got != "http://localhost:9999/v1/mediaItems:batchCreate" {
		t.Errorf("batchCreateURL() = %q, want override", got)
	}
}

func TestBatchCreateURL_FieldOverride(t *testing.T) {
	c := newTestGoogleClient()
	c.apiBaseURL = "http://localhost:9999"
	got := c.batchCreateURL()
	if got != "http://localhost:9999/v1/mediaItems:batchCreate" {
		t.Errorf("batchCreateURL() = %q, want field override", got)
	}
}

func TestNewClient_SkipOAuth(t *testing.T) {
	t.Setenv("PHOTO_COPY_GOOGLE_TOKEN", "skip")
	cfg := &config.GoogleConfig{ClientID: "test", ClientSecret: "test"}
	client, err := NewClient(context.Background(), cfg, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("NewClient with skip token failed: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestGoogleThrottle_TestMode(t *testing.T) {
	t.Setenv("PHOTO_COPY_TEST_MODE", "1")
	c := newTestGoogleClient()
	start := time.Now()
	c.throttle()
	c.throttle()
	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Errorf("throttle in test mode took %v, expected near-zero", elapsed)
	}
}

func TestGoogleRetryDelay_TestMode(t *testing.T) {
	t.Setenv("PHOTO_COPY_TEST_MODE", "1")
	c := newTestGoogleClient()
	delay := c.retryDelay(3, nil)
	if delay != time.Millisecond {
		t.Errorf("retryDelay in test mode = %v, want 1ms", delay)
	}
}

func TestGoogleRetryDelay_ExponentialBackoff(t *testing.T) {
	c := newTestGoogleClient()

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 2 * time.Second},
		{1, 4 * time.Second},
		{2, 8 * time.Second},
		{3, 16 * time.Second},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt_%d", tt.attempt), func(t *testing.T) {
			got := c.retryDelay(tt.attempt, nil)
			if got != tt.expected {
				t.Errorf("retryDelay(%d) = %v, want %v", tt.attempt, got, tt.expected)
			}
		})
	}
}

func TestGoogleRetryDelay_HonorsRetryAfter(t *testing.T) {
	c := newTestGoogleClient()
	resp := &http.Response{
		Header: http.Header{
			"Retry-After": []string{"5"},
		},
	}

	got := c.retryDelay(0, resp)
	if got != 5*time.Second {
		t.Errorf("got %v, want 5s", got)
	}
}

func TestGoogleRetryDelay_NilResponse(t *testing.T) {
	c := newTestGoogleClient()
	got := c.retryDelay(2, nil)
	if got != 8*time.Second {
		t.Errorf("got %v, want 8s", got)
	}
}

func TestUploadLog_Deduplication(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "upload.log")

	_ = appendUploadLog(logPath, "photo1.jpg")
	_ = appendUploadLog(logPath, "photo1.jpg")
	_ = appendUploadLog(logPath, "photo2.jpg")

	log, err := loadUploadLog(logPath)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if len(log) != 2 {
		t.Errorf("expected 2 unique entries, got %d", len(log))
	}
}

func TestCollectMediaFiles_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	files, err := collectMediaFiles(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected 0 files, got %d", len(files))
	}
}

// roundTripFunc allows using a function as an http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRetryableDo_InvalidGrantFailsImmediately(t *testing.T) {
	t.Setenv("PHOTO_COPY_TEST_MODE", "1")
	calls := 0
	c := &Client{
		httpClient: &http.Client{
			Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return nil, &oauth2.RetrieveError{
					Body:      []byte(`{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`),
					ErrorCode: "invalid_grant",
				}
			}),
		},
		log: logging.New(false, nil),
	}

	_, err := c.retryableDo(context.Background(), func() (*http.Request, error) {
		return http.NewRequest("GET", "http://example.com", nil)
	})

	if !errors.Is(err, errTokenExpired) {
		t.Errorf("expected errTokenExpired, got: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call (no retries), got %d", calls)
	}
}

func TestRetryableDo_WrappedInvalidGrantFailsImmediately(t *testing.T) {
	t.Setenv("PHOTO_COPY_TEST_MODE", "1")
	calls := 0
	c := &Client{
		httpClient: &http.Client{
			Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				innerErr := &oauth2.RetrieveError{
					ErrorCode: "invalid_grant",
				}
				return nil, fmt.Errorf("oauth2: token refresh failed: %w", innerErr)
			}),
		},
		log: logging.New(false, nil),
	}

	_, err := c.retryableDo(context.Background(), func() (*http.Request, error) {
		return http.NewRequest("GET", "http://example.com", nil)
	})

	if !errors.Is(err, errTokenExpired) {
		t.Errorf("expected errTokenExpired for wrapped RetrieveError, got: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call (no retries), got %d", calls)
	}
}

func TestRetryableDo_NonInvalidGrantRetries(t *testing.T) {
	t.Setenv("PHOTO_COPY_TEST_MODE", "1")
	calls := 0
	c := &Client{
		httpClient: &http.Client{
			Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return nil, &oauth2.RetrieveError{
					ErrorCode: "invalid_client",
				}
			}),
		},
		log: logging.New(false, nil),
	}

	_, err := c.retryableDo(context.Background(), func() (*http.Request, error) {
		return http.NewRequest("GET", "http://example.com", nil)
	})

	if errors.Is(err, errTokenExpired) {
		t.Error("non-invalid_grant error should not be treated as token expiry")
	}
	if calls != maxRetries+1 {
		t.Errorf("expected %d calls (full retries), got %d", maxRetries+1, calls)
	}
}

