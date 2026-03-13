# Integration Testing Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add CLI-level integration tests that execute cobra commands against configurable mock HTTP servers for Flickr and Google Photos.

**Architecture:** Environment variable overrides redirect service URLs to local `httptest` servers. A `mockserver` package provides configurable mock Flickr/Google servers with a builder API. Integration tests use `//go:build integration` tag and live in `internal/cli/integration_test.go`.

**Tech Stack:** Go stdlib `net/http/httptest`, cobra, existing project packages. No new dependencies.

**Spec:** `plans/2026-03-13-integration-testing-design.md`

---

## Chunk 1: Production Code Changes

### Task 1: Refactor package-level CLI flags into rootOpts struct

**Files:**
- Modify: `internal/cli/root.go`
- Modify: `internal/cli/flickr.go`
- Modify: `internal/cli/google.go`
- Modify: `internal/cli/s3.go`
- Test: `internal/cli/cli_test.go` (existing, verify still passes)

- [ ] **Step 1: Create rootOpts struct and refactor root.go**

Replace the package-level `debug` and `limit` vars with a struct, and thread it through subcommand constructors.

```go
// internal/cli/root.go
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

type rootOpts struct {
	debug bool
	limit int
}

func NewRootCmd() *cobra.Command {
	opts := &rootOpts{}

	rootCmd := &cobra.Command{
		Use:   "photo-copy",
		Short: "Copy photos and videos between Flickr, Google Photos, S3, and local directories",
	}

	rootCmd.PersistentFlags().BoolVar(&opts.debug, "debug", false, "Enable verbose debug logging to stderr")
	rootCmd.PersistentFlags().IntVar(&opts.limit, "limit", 0, "Maximum number of files to upload/download (0 = no limit)")
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	cobra.EnableCommandSorting = false
	rootCmd.AddCommand(newConfigCmd())
	rootCmd.AddCommand(newFlickrCmd(opts))
	rootCmd.AddCommand(newGooglePhotosCmd(opts))
	rootCmd.AddCommand(newS3Cmd(opts))
	rootCmd.InitDefaultHelpCmd()
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "help" {
			rootCmd.RemoveCommand(cmd)
			rootCmd.AddCommand(cmd)
			break
		}
	}

	return rootCmd
}

func Execute() {
	rootCmd := NewRootCmd()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

Note: `newConfigCmd()` does NOT receive `opts` because config commands don't use `debug` or `limit`.

- [ ] **Step 2: Update flickr.go to accept opts**

Change function signatures to accept `*rootOpts`:

```go
func newFlickrCmd(opts *rootOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "flickr",
		Short: "Flickr upload and download commands",
	}

	cmd.AddCommand(newFlickrDownloadCmd(opts))
	cmd.AddCommand(newFlickrUploadCmd(opts))
	return cmd
}

func newFlickrDownloadCmd(opts *rootOpts) *cobra.Command {
	// ... same body but replace `debug` with `opts.debug` and `limit` with `opts.limit`
}

func newFlickrUploadCmd(opts *rootOpts) *cobra.Command {
	// ... same body but replace `debug` with `opts.debug` and `limit` with `opts.limit`
}
```

In each `RunE` closure, replace:
- `logging.New(debug, nil)` → `logging.New(opts.debug, nil)`
- `client.Download(context.Background(), outputDir, limit)` → `client.Download(context.Background(), outputDir, opts.limit)`
- `client.Upload(context.Background(), inputDir, limit)` → `client.Upload(context.Background(), inputDir, opts.limit)`

- [ ] **Step 3: Update google.go to accept opts**

Same pattern:

```go
func newGooglePhotosCmd(opts *rootOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "google",
		Short: "Google Photos commands (upload via API, download via Takeout import)",
	}

	cmd.AddCommand(newGoogleUploadCmd(opts))
	cmd.AddCommand(newGoogleImportTakeoutCmd(opts))
	return cmd
}
```

In `newGoogleUploadCmd`: replace `debug` → `opts.debug`, `limit` → `opts.limit`.
In `newGoogleImportTakeoutCmd`: replace `debug` → `opts.debug` (no `limit` used).

- [ ] **Step 4: Update s3.go to accept opts**

Same pattern:

```go
func newS3Cmd(opts *rootOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "s3",
		Short: "S3 upload and download commands",
	}

	cmd.AddCommand(newS3UploadCmd(opts))
	cmd.AddCommand(newS3DownloadCmd(opts))
	return cmd
}
```

In `newS3UploadCmd`: replace `debug` → `opts.debug`, `limit` → `opts.limit`.
In `newS3DownloadCmd`: replace `debug` → `opts.debug`, `limit` → `opts.limit`.

- [ ] **Step 5: Verify existing tests pass**

Run: `go test ./internal/cli/ && golangci-lint run ./internal/cli/`

Expected: All 9 existing CLI tests pass. Lint clean. The package-level `var debug bool` and `var limit int` lines should be removed — if lint reports unused vars, that confirms the refactor is complete.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/root.go internal/cli/flickr.go internal/cli/google.go internal/cli/s3.go
git commit -m "Refactor CLI flags from package-level vars into rootOpts struct"
```

---

### Task 2: Add PHOTO_COPY_CONFIG_DIR env var override

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/config/config_test.go`:

```go
func TestDefaultDir_EnvOverride(t *testing.T) {
	t.Setenv("PHOTO_COPY_CONFIG_DIR", "/tmp/test-config")
	if got := DefaultDir(); got != "/tmp/test-config" {
		t.Errorf("DefaultDir() = %q, want %q", got, "/tmp/test-config")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestDefaultDir_EnvOverride -v`

Expected: FAIL — `DefaultDir()` returns `~/.config/photo-copy`, not `/tmp/test-config`.

- [ ] **Step 3: Implement the env var check**

In `internal/config/config.go`, modify `DefaultDir()`:

```go
func DefaultDir() string {
	if dir := os.Getenv("PHOTO_COPY_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "photo-copy")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -v && golangci-lint run ./internal/config/`

Expected: All config tests PASS including the new one. Lint clean.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "Add PHOTO_COPY_CONFIG_DIR env var override for DefaultDir()"
```

---

### Task 3: Add Flickr env var overrides and test mode

**Files:**
- Modify: `internal/flickr/flickr.go`
- Test: `internal/flickr/flickr_test.go`

- [ ] **Step 1: Write failing tests for URL overrides and test mode**

Add to `internal/flickr/flickr_test.go`:

```go
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
	if delay != 0 {
		t.Errorf("retryDelay in test mode = %v, want 0", delay)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/flickr/ -run "TestAPIURL|TestFlickrUploadURL|TestThrottle_TestMode|TestRetryDelay_TestMode" -v`

Expected: FAIL — functions don't exist yet.

- [ ] **Step 3: Implement URL helpers and test mode**

In `internal/flickr/flickr.go`:

Replace the constants:
```go
const (
	defaultAPIBaseURL     = "https://api.flickr.com/services/rest/"
	defaultUploadURL      = "https://up.flickr.com/services/upload/"
	transferLogFile       = "transfer.log"
)

func apiURL() string {
	if u := os.Getenv("PHOTO_COPY_FLICKR_API_URL"); u != "" {
		return u
	}
	return defaultAPIBaseURL
}

func flickrUploadURL() string {
	if u := os.Getenv("PHOTO_COPY_FLICKR_UPLOAD_URL"); u != "" {
		return u
	}
	return defaultUploadURL
}

func isTestMode() bool {
	return os.Getenv("PHOTO_COPY_TEST_MODE") != ""
}
```

Update `throttle()`:
```go
func (c *Client) throttle() {
	if isTestMode() {
		return
	}
	if !c.lastRequest.IsZero() {
		elapsed := time.Since(c.lastRequest)
		if elapsed < minRequestInterval {
			time.Sleep(minRequestInterval - elapsed)
		}
	}
	c.lastRequest = time.Now()
}
```

Update `retryDelay()`:
```go
func (c *Client) retryDelay(attempt int, resp *http.Response) time.Duration {
	if isTestMode() {
		return 0
	}
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		if seconds, err := strconv.Atoi(ra); err == nil {
			return time.Duration(seconds) * time.Second
		}
	}
	return baseRetryDelay * time.Duration(math.Pow(2, float64(attempt)))
}
```

**IMPORTANT — also update `buildAPIURL()`** (this callsite is easy to miss since it's a standalone function, not a method on Client). Replace the old constant with `apiURL()`:
```go
func buildAPIURL(method, apiKey string, params map[string]string) string {
	u, _ := url.Parse(apiURL())
	// ... rest unchanged
}
```
If this callsite is missed, the package won't compile because the old `apiBaseURL` constant no longer exists.

Update `signedAPIGet()` — capture URL once, use for both signing and request:
```go
func (c *Client) signedAPIGet(ctx context.Context, method string, extra map[string]string) (*http.Response, error) {
	baseURL := apiURL()
	params := map[string]string{
		"method":         method,
		"format":         "json",
		"nojsoncallback": "1",
	}
	for k, v := range extra {
		params[k] = v
	}

	oauthSign("GET", baseURL, params, c.cfg)

	v := url.Values{}
	for k, val := range params {
		v.Set(k, val)
	}
	return c.retryableGet(ctx, baseURL+"?"+v.Encode())
}
```

Update `uploadFile()` — capture URL once, use for both signing and request:
```go
func (c *Client) uploadFile(ctx context.Context, filePath string) error {
	c.throttle()
	upURL := flickrUploadURL()
	// ... (file reading and multipart construction unchanged) ...

	params := map[string]string{}
	oauthSign("POST", upURL, params, c.cfg)

	// ... (form field writing unchanged) ...

	req, err := http.NewRequestWithContext(ctx, "POST", upURL, body)
	// ... rest unchanged
}
```

- [ ] **Step 4: Run all flickr tests to verify they pass**

Run: `go test ./internal/flickr/ -v && golangci-lint run ./internal/flickr/`

Expected: All tests PASS. Lint clean.

- [ ] **Step 5: Commit**

```bash
git add internal/flickr/flickr.go internal/flickr/flickr_test.go
git commit -m "Add Flickr URL env var overrides and test mode for throttle/retry"
```

---

### Task 4: Add Google env var overrides, OAuth bypass, and test mode

**Files:**
- Modify: `internal/google/google.go`
- Test: `internal/google/google_test.go`

- [ ] **Step 1: Write failing tests**

Add to `internal/google/google_test.go`:

```go
func TestGetUploadURL_Default(t *testing.T) {
	got := getUploadURL()
	if got != "https://photoslibrary.googleapis.com/v1/uploads" {
		t.Errorf("getUploadURL() = %q, want default", got)
	}
}

func TestGetUploadURL_EnvOverride(t *testing.T) {
	t.Setenv("PHOTO_COPY_GOOGLE_API_URL", "http://localhost:9999")
	got := getUploadURL()
	if got != "http://localhost:9999/v1/uploads" {
		t.Errorf("getUploadURL() = %q, want override", got)
	}
}

func TestGetBatchCreateURL_Default(t *testing.T) {
	got := getBatchCreateURL()
	if got != "https://photoslibrary.googleapis.com/v1/mediaItems:batchCreate" {
		t.Errorf("getBatchCreateURL() = %q, want default", got)
	}
}

func TestGetBatchCreateURL_EnvOverride(t *testing.T) {
	t.Setenv("PHOTO_COPY_GOOGLE_API_URL", "http://localhost:9999")
	got := getBatchCreateURL()
	if got != "http://localhost:9999/v1/mediaItems:batchCreate" {
		t.Errorf("getBatchCreateURL() = %q, want override", got)
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
	if delay != 0 {
		t.Errorf("retryDelay in test mode = %v, want 0", delay)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/google/ -run "TestGetUploadURL|TestGetBatchCreateURL|TestNewClient_SkipOAuth|TestGoogleThrottle_TestMode|TestGoogleRetryDelay_TestMode" -v`

Expected: FAIL — functions don't exist yet.

- [ ] **Step 3: Implement URL helpers, OAuth bypass, and test mode**

In `internal/google/google.go`:

Replace the URL constants:
```go
const (
	defaultUploadURL     = "https://photoslibrary.googleapis.com/v1/uploads"
	defaultBatchCreateURL = "https://photoslibrary.googleapis.com/v1/mediaItems:batchCreate"
	defaultAPIBase       = "https://photoslibrary.googleapis.com"
	dailyLimit           = 10000
	maxRetries           = 5
	baseRetryDelay       = 2 * time.Second
	minUploadInterval    = 2 * time.Second
)

func getUploadURL() string {
	if base := os.Getenv("PHOTO_COPY_GOOGLE_API_URL"); base != "" {
		return base + "/v1/uploads"
	}
	return defaultUploadURL
}

func getBatchCreateURL() string {
	if base := os.Getenv("PHOTO_COPY_GOOGLE_API_URL"); base != "" {
		return base + "/v1/mediaItems:batchCreate"
	}
	return defaultBatchCreateURL
}

func isTestMode() bool {
	return os.Getenv("PHOTO_COPY_TEST_MODE") != ""
}
```

Update `NewClient()` to support OAuth bypass:
```go
func NewClient(ctx context.Context, cfg *config.GoogleConfig, configDir string, log *logging.Logger) (*Client, error) {
	if os.Getenv("PHOTO_COPY_GOOGLE_TOKEN") == "skip" {
		return &Client{
			httpClient: &http.Client{},
			log:        log,
			configDir:  configDir,
		}, nil
	}

	// ... existing OAuth flow unchanged ...
}
```

Update `throttle()`:
```go
func (c *Client) throttle() {
	if isTestMode() {
		return
	}
	// ... existing logic unchanged ...
}
```

Update `retryDelay()`:
```go
func (c *Client) retryDelay(attempt int, resp *http.Response) time.Duration {
	if isTestMode() {
		return 0
	}
	// ... existing logic unchanged ...
}
```

Update `uploadBytes()` — replace `uploadURL` with `getUploadURL()`:
```go
func (c *Client) uploadBytes(ctx context.Context, filePath, filename string) (string, error) {
	// ...
	resp, err := c.retryableDo(ctx, func() (*http.Request, error) {
		req, err := http.NewRequest("POST", getUploadURL(), bytes.NewReader(data))
		// ...
	})
	// ...
}
```

Update `createMediaItem()` — replace `batchCreateURL` with `getBatchCreateURL()`:
```go
func (c *Client) createMediaItem(ctx context.Context, uploadToken, filename string) error {
	// ...
	resp, err := c.retryableDo(ctx, func() (*http.Request, error) {
		req, err := http.NewRequest("POST", getBatchCreateURL(), bytes.NewReader(data))
		// ...
	})
	// ...
}
```

- [ ] **Step 4: Run all google tests to verify they pass**

Run: `go test ./internal/google/ -v && golangci-lint run ./internal/google/`

Expected: All tests PASS. Lint clean.

- [ ] **Step 5: Run full test suite**

Run: `go test ./... && golangci-lint run ./...`

Expected: All tests PASS across all packages. No regressions from production code changes.

- [ ] **Step 6: Commit**

```bash
git add internal/google/google.go internal/google/google_test.go
git commit -m "Add Google URL env var overrides, OAuth bypass, and test mode"
```

---

## Chunk 2: Mock Server Package

### Task 5: Create mockserver helpers

**Files:**
- Create: `internal/testutil/mockserver/helpers.go`

- [ ] **Step 1: Create the helpers file**

```go
// Package mockserver provides configurable mock HTTP servers for integration testing.
package mockserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sync"
)

// RecordedRequest captures request details for test assertions.
type RecordedRequest struct {
	Method  string
	Path    string
	Query   url.Values
	Headers http.Header
	Body    []byte
}

// HandlerFunc is the signature for configurable endpoint handlers.
type HandlerFunc func(w http.ResponseWriter, r *http.Request)

// recordRequest reads and records a request, then returns it for further use.
func recordRequest(r *http.Request, mu *sync.Mutex, requests *[]RecordedRequest) {
	body, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()
	rec := RecordedRequest{
		Method:  r.Method,
		Path:    r.URL.Path,
		Query:   r.URL.Query(),
		Headers: r.Header.Clone(),
		Body:    body,
	}
	mu.Lock()
	*requests = append(*requests, rec)
	mu.Unlock()
}

// RespondJSON returns a handler that responds with the given status and JSON body.
func RespondJSON(status int, body any) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		data, _ := json.Marshal(body)
		_, _ = w.Write(data)
	}
}

// RespondStatus returns a handler that responds with just a status code.
func RespondStatus(status int) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
	}
}

// RespondBytes returns a handler that responds with raw bytes.
func RespondBytes(status int, data []byte) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write(data)
	}
}

// RespondSequence returns a handler that uses a different handler for each
// successive call. After the last handler is used, it repeats the last one.
func RespondSequence(handlers ...HandlerFunc) HandlerFunc {
	var mu sync.Mutex
	call := 0
	return func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		idx := call
		if idx >= len(handlers) {
			idx = len(handlers) - 1
		}
		call++
		mu.Unlock()
		handlers[idx](w, r)
	}
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/testutil/mockserver/`

Expected: Compiles with no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/testutil/mockserver/helpers.go
git commit -m "Add mockserver package with shared types and handler factories"
```

---

### Task 6: Create Flickr mock server

**Files:**
- Create: `internal/testutil/mockserver/flickr.go`

- [ ] **Step 1: Create the Flickr mock server**

```go
package mockserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// FlickrMock is a configurable mock Flickr HTTP server.
type FlickrMock struct {
	Server    *httptest.Server
	APIURL    string
	UploadURL string

	onGetPhotos HandlerFunc
	onGetSizes  HandlerFunc
	onUpload    HandlerFunc
	onDownload  HandlerFunc

	mu       sync.Mutex
	requests []RecordedRequest
}

// NewFlickr creates a new unconfigured Flickr mock. Call builder methods then Start().
func NewFlickr(t *testing.T) *FlickrMock {
	m := &FlickrMock{
		onGetPhotos: defaultHandler("flickr.people.getPhotos"),
		onGetSizes:  defaultHandler("flickr.photos.getSizes"),
		onUpload:    defaultHandler("upload"),
		onDownload:  defaultHandler("download"),
	}
	t.Cleanup(func() {
		if m.Server != nil {
			m.Server.Close()
		}
	})
	return m
}

func defaultHandler(name string) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "mock endpoint not configured: "+name, http.StatusNotImplemented)
	}
}

// OnGetPhotos sets the handler for flickr.people.getPhotos API calls.
func (m *FlickrMock) OnGetPhotos(h HandlerFunc) *FlickrMock {
	m.onGetPhotos = h
	return m
}

// OnGetSizes sets the handler for flickr.photos.getSizes API calls.
func (m *FlickrMock) OnGetSizes(h HandlerFunc) *FlickrMock {
	m.onGetSizes = h
	return m
}

// OnUpload sets the handler for file upload POSTs.
func (m *FlickrMock) OnUpload(h HandlerFunc) *FlickrMock {
	m.onUpload = h
	return m
}

// OnDownload sets the handler for file download GETs at /download/*.
func (m *FlickrMock) OnDownload(h HandlerFunc) *FlickrMock {
	m.onDownload = h
	return m
}

// Start creates and starts the httptest.Server. Returns m for chaining.
func (m *FlickrMock) Start() *FlickrMock {
	m.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recordRequest(r, &m.mu, &m.requests)

		// Route: upload endpoint
		if strings.HasPrefix(r.URL.Path, "/services/upload/") {
			m.onUpload(w, r)
			return
		}

		// Route: download endpoint
		if strings.HasPrefix(r.URL.Path, "/download/") {
			m.onDownload(w, r)
			return
		}

		// Route: API calls (dispatched by "method" query param)
		method := r.URL.Query().Get("method")
		switch method {
		case "flickr.people.getPhotos":
			m.onGetPhotos(w, r)
		case "flickr.photos.getSizes":
			m.onGetSizes(w, r)
		default:
			http.Error(w, "unknown method: "+method, http.StatusNotFound)
		}
	}))
	m.APIURL = m.Server.URL + "/services/rest/"
	m.UploadURL = m.Server.URL + "/services/upload/"
	return m
}

// Close shuts down the server.
func (m *FlickrMock) Close() {
	if m.Server != nil {
		m.Server.Close()
	}
}

// Requests returns a copy of all recorded requests.
func (m *FlickrMock) Requests() []RecordedRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]RecordedRequest, len(m.requests))
	copy(out, m.requests)
	return out
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/testutil/mockserver/`

Expected: Compiles with no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/testutil/mockserver/flickr.go
git commit -m "Add configurable Flickr mock server"
```

---

### Task 7: Create Google mock server

**Files:**
- Create: `internal/testutil/mockserver/google.go`

- [ ] **Step 1: Create the Google mock server**

```go
package mockserver

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// GoogleMock is a configurable mock Google Photos HTTP server.
type GoogleMock struct {
	Server  *httptest.Server
	BaseURL string

	onUploadBytes HandlerFunc
	onBatchCreate HandlerFunc

	mu       sync.Mutex
	requests []RecordedRequest
}

// NewGoogle creates a new unconfigured Google Photos mock. Call builder methods then Start().
func NewGoogle(t *testing.T) *GoogleMock {
	m := &GoogleMock{
		onUploadBytes: defaultHandler("uploadBytes"),
		onBatchCreate: defaultHandler("batchCreate"),
	}
	t.Cleanup(func() {
		if m.Server != nil {
			m.Server.Close()
		}
	})
	return m
}

// OnUploadBytes sets the handler for POST /v1/uploads.
func (m *GoogleMock) OnUploadBytes(h HandlerFunc) *GoogleMock {
	m.onUploadBytes = h
	return m
}

// OnBatchCreate sets the handler for POST /v1/mediaItems:batchCreate.
func (m *GoogleMock) OnBatchCreate(h HandlerFunc) *GoogleMock {
	m.onBatchCreate = h
	return m
}

// Start creates and starts the httptest.Server. Returns m for chaining.
func (m *GoogleMock) Start() *GoogleMock {
	m.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recordRequest(r, &m.mu, &m.requests)

		switch r.URL.Path {
		case "/v1/uploads":
			m.onUploadBytes(w, r)
		case "/v1/mediaItems:batchCreate":
			m.onBatchCreate(w, r)
		default:
			http.Error(w, "unknown path: "+r.URL.Path, http.StatusNotFound)
		}
	}))
	m.BaseURL = m.Server.URL
	return m
}

// Close shuts down the server.
func (m *GoogleMock) Close() {
	if m.Server != nil {
		m.Server.Close()
	}
}

// Requests returns a copy of all recorded requests.
func (m *GoogleMock) Requests() []RecordedRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]RecordedRequest, len(m.requests))
	copy(out, m.requests)
	return out
}
```

- [ ] **Step 2: Verify it compiles and full suite passes**

Run: `go build ./internal/testutil/mockserver/ && go test ./... && golangci-lint run ./...`

Expected: Compiles. All existing tests pass. Lint clean.

- [ ] **Step 3: Commit**

```bash
git add internal/testutil/mockserver/google.go
git commit -m "Add configurable Google Photos mock server"
```

---

## Chunk 3: Integration Tests

### Task 8: Flickr download integration tests (6 tests)

**Files:**
- Create: `internal/cli/integration_test.go`

All integration tests go in one file with the `//go:build integration` tag. This task creates the file with the Flickr download tests. Subsequent tasks append to it.

- [ ] **Step 1: Create integration_test.go with test helpers and first 6 tests**

```go
//go:build integration

package cli

import (
	"archive/zip"
	"encoding/json"
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

	mock := mockserver.NewFlickr(t).
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

	mock := mockserver.NewFlickr(t).
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
	os.WriteFile(filepath.Join(outputDir, "transfer.log"), []byte("1_aaa.jpg\n"), 0644)

	photos := []map[string]string{
		{"id": "1", "secret": "aaa", "server": "1", "title": "p1"},
		{"id": "2", "secret": "bbb", "server": "1", "title": "p2"},
	}

	mock := mockserver.NewFlickr(t).
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

	mock := mockserver.NewFlickr(t).
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

	mock := mockserver.NewFlickr(t).
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

	mock := mockserver.NewFlickr(t).
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
```

Note: The `mock` variable is used in closures passed to `OnGetSizes` before `Start()` is called. This works because closures capture the variable reference, and by the time the handler executes, `Start()` has already set `mock.Server`. However, this requires declaring the variable separately from the builder chain. The test code above uses this pattern correctly — `mock` is declared, the builder configures it, and `Start()` is called last.

- [ ] **Step 2: Run the integration tests**

Run: `go test ./internal/cli/ -tags integration -v -count=1`

Expected: All 6 Flickr download tests PASS.

- [ ] **Step 3: Also verify unit tests still pass**

Run: `go test ./... && golangci-lint run ./...`

Expected: All tests PASS (unit tests unaffected by build tag). Lint clean.

- [ ] **Step 4: Commit**

```bash
git add internal/cli/integration_test.go
git commit -m "Add Flickr download integration tests (6 tests)"
```

---

### Task 9: Flickr upload integration tests (4 tests)

**Files:**
- Modify: `internal/cli/integration_test.go`

- [ ] **Step 1: Append Flickr upload tests**

Add to `internal/cli/integration_test.go`:

```go
// --- Flickr Upload Tests ---

func TestFlickrUpload_HappyPath(t *testing.T) {
	inputDir := t.TempDir()
	configDir := t.TempDir()
	setupFlickrConfig(t, configDir)

	// Create test media files
	os.WriteFile(filepath.Join(inputDir, "photo1.jpg"), testImageData, 0644)
	os.WriteFile(filepath.Join(inputDir, "photo2.jpg"), []byte("jpeg-data-2"), 0644)

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

	os.WriteFile(filepath.Join(inputDir, "photo.jpg"), testImageData, 0644)
	os.WriteFile(filepath.Join(inputDir, "readme.txt"), []byte("not media"), 0644)
	os.WriteFile(filepath.Join(inputDir, "data.csv"), []byte("1,2,3"), 0644)

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

	os.WriteFile(filepath.Join(inputDir, "a.jpg"), testImageData, 0644)
	os.WriteFile(filepath.Join(inputDir, "b.jpg"), testImageData, 0644)
	os.WriteFile(filepath.Join(inputDir, "c.jpg"), testImageData, 0644)

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

	os.WriteFile(filepath.Join(inputDir, "photo.jpg"), testImageData, 0644)

	mock := mockserver.NewFlickr(t).
		OnUpload(mockserver.RespondStatus(500)).
		Start()

	setTestEnv(t, configDir)
	t.Setenv("PHOTO_COPY_FLICKR_UPLOAD_URL", mock.UploadURL)

	err := executeCmd(t, "flickr", "upload", inputDir)
	if err == nil {
		t.Fatal("expected error on upload failure")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention HTTP 500, got: %v", err)
	}
}
```

- [ ] **Step 2: Run the Flickr upload integration tests**

Run: `go test ./internal/cli/ -tags integration -run TestFlickrUpload -v -count=1`

Expected: All 4 Flickr upload tests PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/cli/integration_test.go
git commit -m "Add Flickr upload integration tests (4 tests)"
```

---

### Task 10: Google upload integration tests (5 tests)

**Files:**
- Modify: `internal/cli/integration_test.go`

- [ ] **Step 1: Append Google upload tests**

Add to `internal/cli/integration_test.go`:

```go
// --- Google Upload Tests ---

func TestGoogleUpload_HappyPath(t *testing.T) {
	inputDir := t.TempDir()
	configDir := t.TempDir()
	setupGoogleConfig(t, configDir)

	os.WriteFile(filepath.Join(inputDir, "photo1.jpg"), testImageData, 0644)
	os.WriteFile(filepath.Join(inputDir, "photo2.png"), []byte("png-data"), 0644)

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

	os.WriteFile(filepath.Join(inputDir, "photo1.jpg"), testImageData, 0644)
	os.WriteFile(filepath.Join(inputDir, "photo2.jpg"), []byte("data2"), 0644)
	// Pre-populate upload log — photo1 already uploaded
	os.WriteFile(filepath.Join(inputDir, ".photo-copy-upload.log"), []byte("photo1.jpg\n"), 0644)

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

	os.WriteFile(filepath.Join(inputDir, "photo.jpg"), testImageData, 0644)

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

	os.WriteFile(filepath.Join(inputDir, "a_good.jpg"), testImageData, 0644)
	os.WriteFile(filepath.Join(inputDir, "b_bad.jpg"), []byte("data2"), 0644)

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

	os.WriteFile(filepath.Join(inputDir, "a.jpg"), testImageData, 0644)
	os.WriteFile(filepath.Join(inputDir, "b.jpg"), testImageData, 0644)
	os.WriteFile(filepath.Join(inputDir, "c.jpg"), testImageData, 0644)

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
```

Note: `recordRequest` in the mock consumes `r.Body` before the handler runs. Handlers that need request body data should not read `r.Body` — instead, assert via `mock.Requests()` after execution. The `OnBatchCreate` handler above just responds; the token handoff is verified via `mock.Requests()` in the assertions below.

- [ ] **Step 2: Run the Google upload integration tests**

Run: `go test ./internal/cli/ -tags integration -run TestGoogleUpload -v -count=1`

Expected: All 5 Google upload tests PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/cli/integration_test.go
git commit -m "Add Google upload integration tests (5 tests)"
```

---

### Task 11: Google import-takeout integration tests (2 tests)

**Files:**
- Modify: `internal/cli/integration_test.go`

- [ ] **Step 1: Append import-takeout tests**

Add to `internal/cli/integration_test.go`:

```go
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
```

- [ ] **Step 2: Run the import-takeout integration tests**

Run: `go test ./internal/cli/ -tags integration -run TestGoogleImportTakeout -v -count=1`

Expected: Both tests PASS.

- [ ] **Step 3: Run all integration tests together**

Run: `go test ./internal/cli/ -tags integration -v -count=1`

Expected: All 17 integration tests PASS.

- [ ] **Step 4: Run full test suite (unit + lint)**

Run: `go test ./... && golangci-lint run ./...`

Expected: All unit tests PASS. Lint clean. Integration tests not executed (no build tag).

- [ ] **Step 5: Commit**

```bash
git add internal/cli/integration_test.go
git commit -m "Add Google import-takeout integration tests (2 tests)"
```

---

## Chunk 4: Documentation

### Task 12: Update README.md and CLAUDE.md

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`

- [ ] **Step 1: Add integration test section to README.md**

After the existing "Linting & Testing" section in README.md (around line 105), add:

```markdown
### Integration Tests

Integration tests exercise CLI commands end-to-end against mock HTTP servers
for Flickr and Google Photos. They use a build tag and don't run with
`go test ./...`:

```bash
go test ./internal/cli/ -tags integration
```

S3 integration testing is not included — S3 operations delegate to a rclone
subprocess, and rclone's own test coverage handles that layer. S3 unit tests
cover command arg building, config generation, and binary resolution.
```

- [ ] **Step 2: Add integration test command to CLAUDE.md**

In the "Linting & Testing" section of CLAUDE.md, add after the existing commands:

```bash
go test ./internal/cli/ -tags integration    # run integration tests
```

- [ ] **Step 3: Commit**

```bash
git add README.md CLAUDE.md
git commit -m "Document integration tests in README and CLAUDE.md"
```

---

## Implementation Notes

### Key gotchas to watch for

1. **Flickr mock variable capture**: In download tests, `OnGetSizes` closures reference `mock.Server.URL` to build download URLs. The `mock` variable must be declared before the builder chain so the closure captures the pointer. `Start()` populates `Server` before any handler is called.

2. **recordRequest consumes r.Body**: The mock server's `recordRequest()` reads and closes `r.Body` before the handler runs. Handlers that need the body should use the recorded body from `mock.Requests()` after execution, not read `r.Body` directly.

3. **Google retryableDo rebuilds requests**: `retryableDo` takes a `buildReq func()` that constructs a fresh request per attempt. On retry after 429, the mock's `RespondSequence` advances to the next handler, and `retryableDo` calls `buildReq()` again to create a new request — this re-reads the file from disk. This is correct behavior.

4. **Flickr upload has no retry**: `uploadFile()` calls `c.http.Do()` directly, not `retryableGet()`. A 500 from the upload endpoint fails immediately. This is why `TestFlickrUpload_FailsOnError` expects an error, not a retry.

5. **Google partial failure doesn't error**: `Upload()` logs errors but continues past them (line 127-128 of google.go). The command returns `nil` even if some files fail. Only the successful files appear in the upload log.
