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

	if err := appendTransferLog(logPath, "photo1.jpg"); err != nil {
		t.Fatalf("append failed: %v", err)
	}
	if err := appendTransferLog(logPath, "photo2.jpg"); err != nil {
		t.Fatalf("append failed: %v", err)
	}

	log, err := loadTransferLog(logPath)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if !log["photo1.jpg"] || !log["photo2.jpg"] {
		t.Fatalf("expected both photos in log, got: %v", log)
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
