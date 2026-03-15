package flickr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/logging"
)

func TestLoadTransferLog_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	log, err := loadTransferLog(filepath.Join(tmpDir, "transfer.log"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(log) != 0 {
		t.Fatalf("expected empty log, got %d entries", len(log))
	}
}

func TestTransferLog_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "transfer.log")

	if err := appendTransferLog(logPath, "111"); err != nil {
		t.Fatalf("append failed: %v", err)
	}
	if err := appendTransferLog(logPath, "222"); err != nil {
		t.Fatalf("append failed: %v", err)
	}

	log, err := loadTransferLog(logPath)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if !log["111"] || !log["222"] {
		t.Fatalf("expected both photo IDs in log, got: %v", log)
	}
}

func TestTransferLog_BackwardCompatibility(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "transfer.log")

	// Old format: "ID_SECRET.jpg"
	if err := appendTransferLog(logPath, "12345_abcdef.jpg"); err != nil {
		t.Fatalf("append failed: %v", err)
	}
	// New format: bare ID
	if err := appendTransferLog(logPath, "67890"); err != nil {
		t.Fatalf("append failed: %v", err)
	}

	log, err := loadTransferLog(logPath)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if !log["12345"] {
		t.Fatalf("expected old-format entry to be parsed as photo ID '12345', got: %v", log)
	}
	if !log["67890"] {
		t.Fatalf("expected new-format entry '67890' in log, got: %v", log)
	}
}

func TestExtensionFromURL(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		defaultExt string
		want       string
	}{
		{"jpg photo", "https://farm1.staticflickr.com/server/12345_secret_o.jpg", ".jpg", ".jpg"},
		{"png photo", "https://farm1.staticflickr.com/server/12345_secret_o.png", ".jpg", ".png"},
		{"mp4 video with ext", "https://www.flickr.com/photos/user/12345/play/orig/abcdef.mp4", ".mp4", ".mp4"},
		{"mov video", "https://example.com/video.mov", ".mp4", ".mov"},
		{"no extension photo", "https://example.com/file", ".jpg", ".jpg"},
		{"no extension video", "https://example.com/file", ".mp4", ".mp4"},
		{"empty url photo", "", ".jpg", ".jpg"},
		{"empty url video", "", ".mp4", ".mp4"},
		{"query params", "https://example.com/photo.png?token=abc", ".jpg", ".png"},
		{"video URL no ext (flickr style)", "https://www.flickr.com/photos/user/12345/play/orig/secret/", ".mp4", ".mp4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extensionFromURL(tt.url, tt.defaultExt)
			if got != tt.want {
				t.Errorf("extensionFromURL(%q, %q) = %q, want %q", tt.url, tt.defaultExt, got, tt.want)
			}
		})
	}
}

func TestDefaultExtForLabel(t *testing.T) {
	tests := []struct {
		label string
		want  string
	}{
		{"Original", ".jpg"},
		{"Large", ".jpg"},
		{"Video Original", ".mp4"},
		{"Video Player", ".mp4"},
		{"Video Large", ".mp4"},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			got := defaultExtForLabel(tt.label)
			if got != tt.want {
				t.Errorf("defaultExtForLabel(%q) = %q, want %q", tt.label, got, tt.want)
			}
		})
	}
}

func TestBuildAPIURL(t *testing.T) {
	url := buildAPIURL("flickr.people.getPhotos", "testkey", map[string]string{
		"user_id": "me",
		"page":    "1",
	})

	if url == "" {
		t.Fatal("expected non-empty URL")
	}
	if !containsSubstr(url, "flickr.people.getPhotos") {
		t.Fatalf("URL missing method: %s", url)
	}
	if !containsSubstr(url, "testkey") {
		t.Fatalf("URL missing API key: %s", url)
	}
}

func containsSubstr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func newTestClient() *Client {
	return &Client{
		cfg: &config.FlickrConfig{
			APIKey:           "test-key",
			APISecret:        "test-secret",
			OAuthToken:       "test-token",
			OAuthTokenSecret: "test-token-secret",
		},
		http: &http.Client{},
		log:  logging.New(false, nil),
	}
}

func TestAPIURL_Default(t *testing.T) {
	got := apiURL()
	if got != "https://api.flickr.com/services/rest/" {
		t.Errorf("apiURL() = %q, want default", got)
	}
}

func TestAPIURL_EnvOverride(t *testing.T) {
	t.Setenv("PHOTO_COPY_FLICKR_API_URL", "http://localhost:9999/api/")
	got := apiURL()
	if got != "http://localhost:9999/api/" {
		t.Errorf("apiURL() = %q, want override", got)
	}
}

func TestFlickrUploadURL_Default(t *testing.T) {
	got := flickrUploadURL()
	if got != "https://up.flickr.com/services/upload/" {
		t.Errorf("flickrUploadURL() = %q, want default", got)
	}
}

func TestFlickrUploadURL_EnvOverride(t *testing.T) {
	t.Setenv("PHOTO_COPY_FLICKR_UPLOAD_URL", "http://localhost:9999/upload/")
	got := flickrUploadURL()
	if got != "http://localhost:9999/upload/" {
		t.Errorf("flickrUploadURL() = %q, want override", got)
	}
}

func TestThrottle_TestMode(t *testing.T) {
	t.Setenv("PHOTO_COPY_TEST_MODE", "1")
	c := newTestClient()
	start := time.Now()
	c.throttle()
	c.throttle()
	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Errorf("throttle in test mode took %v, expected near-zero", elapsed)
	}
}

func TestRetryDelay_TestMode(t *testing.T) {
	t.Setenv("PHOTO_COPY_TEST_MODE", "1")
	c := newTestClient()
	resp := &http.Response{Header: http.Header{}}
	delay := c.retryDelay(3, resp)
	if delay != time.Millisecond {
		t.Errorf("retryDelay in test mode = %v, want 1ms", delay)
	}
}

func TestRetryDelay_ExponentialBackoff(t *testing.T) {
	c := newTestClient()
	resp := &http.Response{Header: http.Header{}}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 2 * time.Second},
		{1, 4 * time.Second},
		{2, 8 * time.Second},
		{3, 16 * time.Second},
		{4, 32 * time.Second},
	}

	for _, tt := range tests {
		got := c.retryDelay(tt.attempt, resp)
		if got != tt.expected {
			t.Errorf("retryDelay(%d) = %v, want %v", tt.attempt, got, tt.expected)
		}
	}
}

func TestRetryDelay_HonorsRetryAfterHeader(t *testing.T) {
	c := newTestClient()
	resp := &http.Response{
		Header: http.Header{
			"Retry-After": []string{"10"},
		},
	}

	got := c.retryDelay(0, resp)
	if got != 10*time.Second {
		t.Errorf("retryDelay with Retry-After=10 got %v, want 10s", got)
	}
}

func TestRetryDelay_InvalidRetryAfterFallsBackToExponential(t *testing.T) {
	c := newTestClient()
	resp := &http.Response{
		Header: http.Header{
			"Retry-After": []string{"not-a-number"},
		},
	}

	got := c.retryDelay(1, resp)
	if got != 4*time.Second {
		t.Errorf("retryDelay with invalid Retry-After got %v, want 4s", got)
	}
}

func TestRetryableGet_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	c := newTestClient()
	c.http = server.Client()

	resp, err := c.retryableGet(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestRetryableGet_RetriesOn429(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 2 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := newTestClient()
	c.http = server.Client()

	resp, err := c.retryableGet(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetryableGet_RetriesOn500(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := newTestClient()
	c.http = server.Client()

	resp, err := c.retryableGet(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestRetryableGet_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	c := newTestClient()
	c.http = server.Client()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := c.retryableGet(ctx, server.URL)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestRetryableGet_ExhaustsRetries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	c := newTestClient()
	c.http = server.Client()

	_, err := c.retryableGet(context.Background(), server.URL)
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
}
