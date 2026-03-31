package flickr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/logging"
	"github.com/briandeitte/photo-copy/internal/transfer"
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
	if !strings.Contains(url, "flickr.people.getPhotos") {
		t.Fatalf("URL missing method: %s", url)
	}
	if !strings.Contains(url, "testkey") {
		t.Fatalf("URL missing API key: %s", url)
	}
}

func newTestClient() *Client {
	return &Client{
		cfg: &config.FlickrConfig{
			APIKey:           "test-key",
			APISecret:        "test-secret",
			OAuthToken:       "test-token",
			OAuthTokenSecret: "test-token-secret",
		},
		http:             &http.Client{},
		log:              logging.New(false, nil),
		throttleInterval: minRequestInterval,
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

func TestRetryDelay_CapsAt5MinutesForHighAttempts(t *testing.T) {
	t.Setenv("PHOTO_COPY_TEST_MODE", "")
	c := newTestClient()
	resp := &http.Response{Header: http.Header{}}

	// Attempt 7: 2s * 2^7 = 256s (under 5m cap, should NOT be capped)
	delay7 := c.retryDelay(7, resp)
	if delay7 != 256*time.Second {
		t.Errorf("attempt 7: expected 256s, got %v", delay7)
	}

	// Attempt 8: 2s * 2^8 = 512s (over 5m cap, should be capped)
	delay8 := c.retryDelay(8, resp)
	if delay8 != maxRateLimitBackoff {
		t.Errorf("attempt 8: expected %v, got %v", maxRateLimitBackoff, delay8)
	}

	// High attempts including values around and beyond the int64 overflow
	// boundary (~attempt 33) must all return the cap.
	for _, attempt := range []int{20, 32, 33, 40, 100, 1000} {
		delay := c.retryDelay(attempt, resp)
		if delay != maxRateLimitBackoff {
			t.Errorf("attempt %d: expected %v, got %v", attempt, maxRateLimitBackoff, delay)
		}
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
	if c.throttleInterval <= minRequestInterval {
		t.Errorf("expected throttleInterval to increase after 429s, got %v", c.throttleInterval)
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

func TestRetryableGet_RetriesOn502(t *testing.T) {
	t.Setenv("PHOTO_COPY_TEST_MODE", "1")
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 1 {
			w.WriteHeader(http.StatusBadGateway)
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

func TestRetryableGet_RetriesOn503(t *testing.T) {
	t.Setenv("PHOTO_COPY_TEST_MODE", "1")
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
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

func TestRetryableGet_NoRetryOn400(t *testing.T) {
	t.Setenv("PHOTO_COPY_TEST_MODE", "1")
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	c := newTestClient()
	c.http = server.Client()

	resp, err := c.retryableGet(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if attempts != 1 {
		t.Errorf("expected 1 attempt (no retry), got %d", attempts)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}
}

func TestRetryableGet_NoRetryOn401(t *testing.T) {
	t.Setenv("PHOTO_COPY_TEST_MODE", "1")
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	c := newTestClient()
	c.http = server.Client()

	resp, err := c.retryableGet(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if attempts != 1 {
		t.Errorf("expected 1 attempt (no retry), got %d", attempts)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", resp.StatusCode)
	}
}

func TestRetryableGet_NoRetryOn403(t *testing.T) {
	t.Setenv("PHOTO_COPY_TEST_MODE", "1")
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	c := newTestClient()
	c.http = server.Client()

	resp, err := c.retryableGet(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if attempts != 1 {
		t.Errorf("expected 1 attempt (no retry), got %d", attempts)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", resp.StatusCode)
	}
}

func TestRetryableGet_ExhaustsRetries(t *testing.T) {
	t.Setenv("PHOTO_COPY_TEST_MODE", "1")
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := newTestClient()
	c.http = server.Client()

	_, err := c.retryableGet(context.Background(), server.URL)
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if attempts != 8 {
		t.Errorf("expected 8 attempts (1 initial + 7 retries), got %d", attempts)
	}
}

func TestRetryableGet_429RetriesIndefinitely(t *testing.T) {
	t.Setenv("PHOTO_COPY_TEST_MODE", "1")
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 10 {
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
		t.Fatalf("expected success after 429s clear, got error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if attempts != 11 {
		t.Errorf("expected 11 attempts (10 x 429 + 1 success), got %d", attempts)
	}
}

func TestRetryableGet_5xxStillLimitedTo7Retries(t *testing.T) {
	t.Setenv("PHOTO_COPY_TEST_MODE", "1")
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := newTestClient()
	c.http = server.Client()

	_, err := c.retryableGet(context.Background(), server.URL)
	if err == nil {
		t.Fatal("expected error after exhausting 5xx retries")
	}
	// 1 initial + 7 retries = 8 total attempts
	if attempts != 8 {
		t.Errorf("expected 8 attempts for 5xx, got %d", attempts)
	}
}

func TestEstimateRemaining(t *testing.T) {
	tests := []struct {
		name      string
		limit     int
		result    *transfer.Result
		want      int
	}{
		{
			name:  "no limit uses expected minus processed",
			limit: 0,
			result: &transfer.Result{Expected: 100, Succeeded: 30, Skipped: 10, Failed: 5},
			want:  55,
		},
		{
			name:  "limit with remaining",
			limit: 10,
			result: &transfer.Result{Succeeded: 3, Failed: 1},
			want:  6,
		},
		{
			name:  "limit fully consumed",
			limit: 10,
			result: &transfer.Result{Succeeded: 9, Failed: 1},
			want:  0,
		},
		{
			name:  "limit exceeded clamps to zero",
			limit: 10,
			result: &transfer.Result{Succeeded: 11, Failed: 2},
			want:  0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateRemaining(tt.limit, tt.result)
			if got != tt.want {
				t.Errorf("estimateRemaining(%d, ...) = %d, want %d", tt.limit, got, tt.want)
			}
		})
	}
}

func TestOnRateLimited_DoublesInterval(t *testing.T) {
	c := newTestClient()
	c.throttleInterval = minRequestInterval

	c.onRateLimited()
	if c.throttleInterval != 2*minRequestInterval {
		t.Errorf("expected %v, got %v", 2*minRequestInterval, c.throttleInterval)
	}

	c.onRateLimited()
	if c.throttleInterval != 4*minRequestInterval {
		t.Errorf("expected %v, got %v", 4*minRequestInterval, c.throttleInterval)
	}
}

func TestOnRateLimited_CapsAtMax(t *testing.T) {
	c := newTestClient()
	c.throttleInterval = maxThrottleInterval

	c.onRateLimited()
	if c.throttleInterval != maxThrottleInterval {
		t.Errorf("expected interval capped at %v, got %v", maxThrottleInterval, c.throttleInterval)
	}
}

func TestOnRequestSuccess_ReducesInterval(t *testing.T) {
	c := newTestClient()
	c.throttleInterval = 4 * time.Second

	c.onRequestSuccess()
	expected := 3 * time.Second // 4s * 3/4 = 3s
	if c.throttleInterval != expected {
		t.Errorf("expected %v, got %v", expected, c.throttleInterval)
	}
}

func TestOnRequestSuccess_ClampsToMin(t *testing.T) {
	c := newTestClient()
	c.throttleInterval = minRequestInterval

	c.onRequestSuccess()
	if c.throttleInterval != minRequestInterval {
		t.Errorf("expected interval to stay at %v, got %v", minRequestInterval, c.throttleInterval)
	}
}

func TestOnRequestSuccess_DoesNotGoBelowMin(t *testing.T) {
	c := newTestClient()
	// Set to a value where 3/4 would go below minRequestInterval
	c.throttleInterval = minRequestInterval + 100*time.Millisecond

	c.onRequestSuccess()
	if c.throttleInterval < minRequestInterval {
		t.Errorf("expected interval >= %v, got %v", minRequestInterval, c.throttleInterval)
	}
}

func TestDownload_404FallbackToAlternativeSize(t *testing.T) {
	t.Setenv("PHOTO_COPY_TEST_MODE", "1")

	originalHits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		method := q.Get("method")

		switch {
		case method == "flickr.people.getPhotos":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"photos":{"page":1,"pages":1,"total":1,"photo":[{"id":"111","secret":"abc","title":"test","datetaken":"2024-01-01 00:00:00","dateupload":"1704067200","description":{"_content":""},"tags":""}]},"stat":"ok"}`))

		case method == "flickr.photos.getSizes":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"sizes":{"size":[{"label":"Original","source":"` + "http://" + r.Host + `/download/original.jpg"},{"label":"Large","source":"` + "http://" + r.Host + `/download/large.jpg"}]},"stat":"ok"}`))

		case r.URL.Path == "/download/original.jpg":
			originalHits++
			w.WriteHeader(http.StatusNotFound)

		case r.URL.Path == "/download/large.jpg":
			_, _ = w.Write([]byte("fallback image data"))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	t.Setenv("PHOTO_COPY_FLICKR_API_URL", server.URL)
	outputDir := t.TempDir()

	c := newTestClient()
	c.cfg.APIKey = "test-key"
	c.http = server.Client()

	result, err := c.Download(context.Background(), outputDir, 0, true, nil)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	if result.Succeeded != 1 {
		t.Errorf("expected 1 succeeded, got %d", result.Succeeded)
	}
	if result.Failed != 0 {
		t.Errorf("expected 0 failed, got %d", result.Failed)
	}
	if originalHits != 1 {
		t.Errorf("expected original URL to be hit once, got %d", originalHits)
	}
}

func TestDownload_Non404ErrorDoesNotFallback(t *testing.T) {
	t.Setenv("PHOTO_COPY_TEST_MODE", "1")

	fallbackHits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		method := q.Get("method")

		switch {
		case method == "flickr.people.getPhotos":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"photos":{"page":1,"pages":1,"total":1,"photo":[{"id":"222","secret":"def","title":"test2","datetaken":"2024-01-01 00:00:00","dateupload":"1704067200","description":{"_content":""},"tags":""}]},"stat":"ok"}`))

		case method == "flickr.photos.getSizes":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"sizes":{"size":[{"label":"Original","source":"` + "http://" + r.Host + `/download/original.jpg"},{"label":"Large","source":"` + "http://" + r.Host + `/download/large.jpg"}]},"stat":"ok"}`))

		case r.URL.Path == "/download/original.jpg":
			w.WriteHeader(http.StatusForbidden)

		case r.URL.Path == "/download/large.jpg":
			fallbackHits++
			_, _ = w.Write([]byte("fallback data"))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	t.Setenv("PHOTO_COPY_FLICKR_API_URL", server.URL)
	outputDir := t.TempDir()

	c := newTestClient()
	c.cfg.APIKey = "test-key"
	c.http = server.Client()

	result, err := c.Download(context.Background(), outputDir, 0, true, nil)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	if result.Failed != 1 {
		t.Errorf("expected 1 failed (403 should not fallback), got %d", result.Failed)
	}
	if result.Succeeded != 0 {
		t.Errorf("expected 0 succeeded, got %d", result.Succeeded)
	}
	if fallbackHits != 0 {
		t.Errorf("expected fallback URL to not be hit, got %d hits", fallbackHits)
	}
}

func TestDownload_XMLPermissionDeniedNoRetry(t *testing.T) {
	t.Setenv("PHOTO_COPY_TEST_MODE", "1")

	getSizesHits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		method := q.Get("method")

		switch method {
		case "flickr.people.getPhotos":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"photos":{"page":1,"pages":1,"total":1,"photo":[{"id":"333","secret":"ghi","title":"private photo","datetaken":"2024-01-01 00:00:00","dateupload":"1704067200","description":{"_content":""},"tags":""}]},"stat":"ok"}`))

		case "flickr.photos.getSizes":
			getSizesHits++
			w.Header().Set("Content-Type", "text/xml; charset=utf-8")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8" ?>
<rsp stat="fail">
    <err code="2" msg="Permission denied" />
</rsp>`))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	t.Setenv("PHOTO_COPY_FLICKR_API_URL", server.URL)
	outputDir := t.TempDir()

	c := newTestClient()
	c.cfg.APIKey = "test-key"
	c.http = server.Client()

	result, err := c.Download(context.Background(), outputDir, 0, true, nil)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	if result.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", result.Failed)
	}
	if result.Succeeded != 0 {
		t.Errorf("expected 0 succeeded, got %d", result.Succeeded)
	}
	// The key assertion: getSizes should only be called once, not retried
	if getSizesHits != 1 {
		t.Errorf("expected getSizes to be called exactly once (no retries for permanent XML errors), got %d", getSizesHits)
	}
}
