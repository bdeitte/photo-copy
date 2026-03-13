# Linting, Testing, and Claude Enforcement Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add golangci-lint, improve unit test coverage across all packages, and enforce lint+tests via Claude Code pre-commit hooks.

**Architecture:** Add a `.golangci.yml` config at project root, create `.claude/settings.json` with a PreCommit hook running lint+tests, update CLAUDE.md and README.md with dev instructions, then write new tests package by package using table-driven tests and httptest for HTTP mocking.

**Tech Stack:** Go 1.25, golangci-lint, cobra (for CLI testing), net/http/httptest

**Spec:** `docs/superpowers/specs/2026-03-12-linting-testing-design.md`

---

## Chunk 1: Infrastructure (Linting, Hooks, Docs)

### Task 1: Add golangci-lint configuration

**Files:**
- Create: `.golangci.yml`

- [ ] **Step 1: Create `.golangci.yml`**

```yaml
run:
  timeout: 3m

linters:
  enable:
    - govet
    - staticcheck
    - errcheck
    - unused
    - gosimple
    - ineffassign
    - gocritic

issues:
  exclude-dirs:
    - rclone-bin
```

- [ ] **Step 2: Install golangci-lint**

Run: `brew install golangci-lint`
Expected: Successful install (or already installed message)

- [ ] **Step 3: Run golangci-lint and fix any issues**

Run: `golangci-lint run ./...`
Expected: Either clean output, or fixable lint errors. Fix any errors before proceeding. Common issues will be unchecked errors (errcheck) — add explicit `_ =` or handle the error.

- [ ] **Step 4: Commit**

```bash
git add .golangci.yml
git commit -m "Add golangci-lint configuration"
```

If lint errors were fixed, include those files too:
```bash
git add .golangci.yml <fixed-files>
git commit -m "Add golangci-lint configuration and fix lint errors"
```

---

### Task 2: Update CLAUDE.md with lint and test instructions

**Files:**
- Modify: `CLAUDE.md:1-47`

- [ ] **Step 1: Add lint/test section to CLAUDE.md**

Add after the `## Build & Run` section (after line 12):

```markdown
## Linting & Testing

```bash
golangci-lint run ./...                       # run all linters
go test ./...                                 # run all tests
```

Always run `golangci-lint run ./...` and `go test ./...` after making code changes, before committing. Fix any lint errors or test failures before proceeding.

A Claude Code pre-commit hook (`.claude/settings.json`) enforces this — commits will be blocked if lint or tests fail.
```

- [ ] **Step 2: Verify CLAUDE.md is valid markdown**

Read the file and check it renders correctly.

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "Add lint and test instructions to CLAUDE.md"
```

---

### Task 3: Update README.md Development section

**Files:**
- Modify: `README.md:88-100`

- [ ] **Step 1: Add lint/test info to Development section**

Replace the Development section (lines 88-100) with:

```markdown
## Development

See [CLAUDE.md](CLAUDE.md) for some details on the project.

### Linting & Testing

Install golangci-lint:

```bash
brew install golangci-lint
```

Run linting and tests:

```bash
golangci-lint run ./...    # lint
go test ./...              # test
```

### Updating rclone

To update the bundled rclone binaries:

```bash
./scripts/update-rclone.sh v1.68.2
```
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "Add linting and testing instructions to README.md"
```

---

### Task 4: Create Claude Code pre-commit hook

**Files:**
- Create: `.claude/settings.json`

- [ ] **Step 1: Create `.claude/settings.json`**

```json
{
  "hooks": {
    "PreCommit": [
      {
        "command": "golangci-lint run ./... && go test ./..."
      }
    ]
  }
}
```

Note: `.claude/settings.local.json` already exists with user-specific permissions. This new `settings.json` is the project-level shared config.

- [ ] **Step 2: Verify hook works**

Run manually: `golangci-lint run ./... && go test ./...`
Expected: Both pass with zero errors.

- [ ] **Step 3: Commit**

```bash
git add .claude/settings.json
git commit -m "Add Claude Code pre-commit hook for lint and tests"
```

---

## Chunk 2: Flickr Package Tests

### Task 5: Add OAuth signature tests

**Files:**
- Create: `internal/flickr/oauth_test.go`

- [ ] **Step 1: Write OAuth signature tests**

```go
package flickr

import (
	"testing"

	"github.com/briandeitte/photo-copy/internal/config"
)

func TestOAuthSign_SetsRequiredParams(t *testing.T) {
	cfg := &config.FlickrConfig{
		APIKey:           "test-key",
		APISecret:        "test-secret",
		OAuthToken:       "test-token",
		OAuthTokenSecret: "test-token-secret",
	}

	params := map[string]string{
		"method": "flickr.test.echo",
	}

	sig := oauthSign("GET", "https://api.flickr.com/services/rest/", params, cfg)

	// Verify all required OAuth params are set
	required := []string{
		"oauth_consumer_key",
		"oauth_token",
		"oauth_signature_method",
		"oauth_timestamp",
		"oauth_nonce",
		"oauth_version",
		"oauth_signature",
	}
	for _, key := range required {
		if _, ok := params[key]; !ok {
			t.Errorf("missing required OAuth param: %s", key)
		}
	}

	if params["oauth_consumer_key"] != "test-key" {
		t.Errorf("oauth_consumer_key = %q, want %q", params["oauth_consumer_key"], "test-key")
	}
	if params["oauth_token"] != "test-token" {
		t.Errorf("oauth_token = %q, want %q", params["oauth_token"], "test-token")
	}
	if params["oauth_signature_method"] != "HMAC-SHA1" {
		t.Errorf("oauth_signature_method = %q, want %q", params["oauth_signature_method"], "HMAC-SHA1")
	}
	if params["oauth_version"] != "1.0" {
		t.Errorf("oauth_version = %q, want %q", params["oauth_version"], "1.0")
	}
	if sig == "" {
		t.Error("expected non-empty signature")
	}
	if sig != params["oauth_signature"] {
		t.Errorf("returned signature %q != params signature %q", sig, params["oauth_signature"])
	}
}

func TestOAuthSign_DeterministicWithSameInputs(t *testing.T) {
	// oauthSign uses time.Now() and a random nonce, so two calls will differ.
	// But we can verify the signature is non-empty and the params are populated.
	cfg := &config.FlickrConfig{
		APIKey:           "key",
		APISecret:        "secret",
		OAuthToken:       "token",
		OAuthTokenSecret: "token-secret",
	}

	params1 := map[string]string{"method": "test"}
	params2 := map[string]string{"method": "test"}

	sig1 := oauthSign("GET", "https://example.com/", params1, cfg)
	sig2 := oauthSign("GET", "https://example.com/", params2, cfg)

	// Both should produce non-empty signatures
	if sig1 == "" || sig2 == "" {
		t.Error("signatures should not be empty")
	}

	// Nonces should differ (extremely unlikely to collide)
	if params1["oauth_nonce"] == params2["oauth_nonce"] {
		t.Error("nonces should differ between calls")
	}
}

func TestOAuthSign_PreservesExistingParams(t *testing.T) {
	cfg := &config.FlickrConfig{
		APIKey:           "key",
		APISecret:        "secret",
		OAuthToken:       "token",
		OAuthTokenSecret: "token-secret",
	}

	params := map[string]string{
		"method":         "flickr.photos.getInfo",
		"photo_id":       "12345",
		"format":         "json",
		"nojsoncallback": "1",
	}

	oauthSign("GET", "https://api.flickr.com/services/rest/", params, cfg)

	// Original params should still be present
	if params["method"] != "flickr.photos.getInfo" {
		t.Error("method param was overwritten")
	}
	if params["photo_id"] != "12345" {
		t.Error("photo_id param was overwritten")
	}
}

func TestGenerateNonce(t *testing.T) {
	nonce := generateNonce()
	if len(nonce) != 32 {
		t.Errorf("nonce length = %d, want 32", len(nonce))
	}

	// Should only contain alphanumeric characters
	for _, c := range nonce {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			t.Errorf("nonce contains invalid character: %c", c)
		}
	}

	// Two nonces should differ
	nonce2 := generateNonce()
	if nonce == nonce2 {
		t.Error("two nonces should not be identical")
	}
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test ./internal/flickr/ -run TestOAuth -v`
Expected: All pass

Run: `go test ./internal/flickr/ -run TestGenerateNonce -v`
Expected: Pass

- [ ] **Step 3: Commit**

```bash
git add internal/flickr/oauth_test.go
git commit -m "Add OAuth signature and nonce generation tests"
```

---

### Task 6: Add retry and throttle tests for Flickr client

**Files:**
- Modify: `internal/flickr/flickr_test.go`

- [ ] **Step 1: Write retryDelay tests**

Add to `internal/flickr/flickr_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test ./internal/flickr/ -run TestRetryDelay -v`
Expected: All 3 tests pass

- [ ] **Step 3: Write retryableGet tests with httptest**

Add to `internal/flickr/flickr_test.go`:

```go
func TestRetryableGet_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	c := newTestClient()
	c.http = server.Client()

	resp, err := c.retryableGet(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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
	// Cancel immediately so the retry wait is interrupted
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
```

- [ ] **Step 4: Run all flickr tests**

Run: `go test ./internal/flickr/ -v`
Expected: All pass

- [ ] **Step 5: Run lint**

Run: `golangci-lint run ./internal/flickr/`
Expected: Clean

- [ ] **Step 6: Commit**

```bash
git add internal/flickr/flickr_test.go
git commit -m "Add retry, backoff, and HTTP error handling tests for Flickr client"
```

---

## Chunk 3: Google Package Tests

### Task 7: Add Google Photos tests

**Files:**
- Modify: `internal/google/google_test.go`

- [ ] **Step 1: Write retryDelay tests for Google client**

Add to `internal/google/google_test.go`:

```go
import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/briandeitte/photo-copy/internal/logging"
)

func newTestGoogleClient() *Client {
	return &Client{
		httpClient: &http.Client{},
		log:        logging.New(false, nil),
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
		got := c.retryDelay(tt.attempt, nil)
		if got != tt.expected {
			t.Errorf("retryDelay(%d) = %v, want %v", tt.attempt, got, tt.expected)
		}
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
```

- [ ] **Step 2: Write upload log deduplication test**

Add to `internal/google/google_test.go`:

```go
func TestUploadLog_Deduplication(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "upload.log")

	// Append the same file twice
	appendUploadLog(logPath, "photo1.jpg")
	appendUploadLog(logPath, "photo1.jpg")
	appendUploadLog(logPath, "photo2.jpg")

	log, err := loadUploadLog(logPath)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	// The log file has 3 lines, but the map should have 2 unique entries
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
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestCollectMediaFiles_SkipsDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "subdir"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "photo.jpg"), []byte("fake"), 0644)

	files, err := collectMediaFiles(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 {
		t.Errorf("expected 1 file, got %d", len(files))
	}
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/google/ -v`
Expected: All pass

- [ ] **Step 4: Run lint**

Run: `golangci-lint run ./internal/google/`
Expected: Clean

- [ ] **Step 5: Commit**

```bash
git add internal/google/google_test.go
git commit -m "Add retry delay, upload log dedup, and media collection tests for Google"
```

---

## Chunk 4: S3 and Config Package Tests

### Task 8: Add S3 package tests

**Files:**
- Modify: `internal/s3/s3_test.go`
- Modify: `internal/s3/rclone_test.go`

- [ ] **Step 1: Add download args and media filter tests to s3_test.go**

Add to `internal/s3/s3_test.go`:

```go
func TestBuildDownloadArgs_NoPrefix(t *testing.T) {
	args := buildDownloadArgs("/tmp/config.conf", "my-bucket", "", "/output")
	found := false
	for _, a := range args {
		if a == "s3:my-bucket" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected 's3:my-bucket' in args, got: %v", args)
	}
}

func TestBuildMediaIncludeFlags_HasPairs(t *testing.T) {
	flags := buildMediaIncludeFlags()
	// Every flag should be paired: --include <pattern>
	if len(flags)%2 != 0 {
		t.Fatalf("expected even number of flags (--include pairs), got %d", len(flags))
	}
	for i := 0; i < len(flags); i += 2 {
		if flags[i] != "--include" {
			t.Errorf("flags[%d] = %q, want --include", i, flags[i])
		}
	}
}

func TestBuildMediaIncludeFlags_CoversExpectedExtensions(t *testing.T) {
	flags := buildMediaIncludeFlags()
	flagSet := make(map[string]bool)
	for i := 1; i < len(flags); i += 2 {
		flagSet[flags[i]] = true
	}

	// Spot-check a few key extensions
	expected := []string{"*.jpg", "*.JPG", "*.mp4", "*.MP4", "*.heic", "*.HEIC"}
	for _, ext := range expected {
		if !flagSet[ext] {
			t.Errorf("missing expected extension: %s", ext)
		}
	}
}
```

- [ ] **Step 2: Add rclone config edge case tests to rclone_test.go**

Add to `internal/s3/rclone_test.go`:

```go
func TestWriteRcloneConfig_ContainsAllFields(t *testing.T) {
	path, err := writeRcloneConfig("AKID", "SECRET", "eu-west-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}

	content := string(data)
	checks := map[string]string{
		"[s3]":                       "section header",
		"type = s3":                  "type",
		"provider = AWS":             "provider",
		"access_key_id = AKID":       "access key",
		"secret_access_key = SECRET": "secret key",
		"region = eu-west-1":         "region",
	}

	for want, desc := range checks {
		if !strings.Contains(content, want) {
			t.Errorf("missing %s: %q", desc, want)
		}
	}
}

func TestWriteRcloneConfig_CreatesReadableFile(t *testing.T) {
	path, err := writeRcloneConfig("A", "B", "us-east-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer os.Remove(path)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if info.Size() == 0 {
		t.Error("config file is empty")
	}
}

func TestRcloneBinaryName_Windows(t *testing.T) {
	name := rcloneBinaryName("windows", "amd64")
	if !strings.HasSuffix(name, ".exe") {
		t.Errorf("windows binary should end in .exe, got %q", name)
	}
}

func TestRcloneBinaryName_NonWindows(t *testing.T) {
	name := rcloneBinaryName("linux", "arm64")
	if strings.HasSuffix(name, ".exe") {
		t.Errorf("linux binary should not end in .exe, got %q", name)
	}
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/s3/ -v`
Expected: All pass

- [ ] **Step 4: Run lint**

Run: `golangci-lint run ./internal/s3/`
Expected: Clean

- [ ] **Step 5: Commit**

```bash
git add internal/s3/s3_test.go internal/s3/rclone_test.go
git commit -m "Add S3 arg building, media filter, and rclone config tests"
```

---

### Task 9: Add config package edge case tests

**Files:**
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write malformed JSON and edge case tests**

Add to `internal/config/config_test.go`:

```go
func TestLoadFlickrConfig_MalformedJSON(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "flickr.json"), []byte("{bad json"), 0644)

	_, err := LoadFlickrConfig(tmpDir)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestLoadGoogleConfig_MalformedJSON(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "google_credentials.json"), []byte("not json"), 0644)

	_, err := LoadGoogleConfig(tmpDir)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestLoadS3Config_MalformedJSON(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "s3.json"), []byte("{invalid"), 0644)

	_, err := LoadS3Config(tmpDir)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestLoadS3Config_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := LoadS3Config(tmpDir)
	if err == nil {
		t.Fatal("expected error for missing config")
	}
}

func TestLoadGoogleConfig_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := LoadGoogleConfig(tmpDir)
	if err == nil {
		t.Fatal("expected error for missing config")
	}
}

func TestDefaultDir(t *testing.T) {
	dir := DefaultDir()
	if dir == "" {
		t.Fatal("expected non-empty default dir")
	}
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/config/ -v`
Expected: All pass

- [ ] **Step 3: Run lint**

Run: `golangci-lint run ./internal/config/`
Expected: Clean

- [ ] **Step 4: Commit**

```bash
git add internal/config/config_test.go
git commit -m "Add malformed JSON and missing config edge case tests"
```

---

## Chunk 5: CLI Package Tests

### Task 10: Add CLI command tests

**Files:**
- Create: `internal/cli/cli_test.go`

- [ ] **Step 1: Write CLI tests**

```go
package cli

import (
	"bytes"
	"testing"
)

func TestRootCmd_HasExpectedSubcommands(t *testing.T) {
	cmd := NewRootCmd()

	expected := []string{"config", "flickr", "google", "s3", "help"}
	names := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		names[sub.Name()] = true
	}

	for _, name := range expected {
		if !names[name] {
			t.Errorf("missing expected subcommand: %s", name)
		}
	}
}

func TestRootCmd_HasDebugFlag(t *testing.T) {
	cmd := NewRootCmd()
	f := cmd.PersistentFlags().Lookup("debug")
	if f == nil {
		t.Fatal("missing --debug flag")
	}
	if f.DefValue != "false" {
		t.Errorf("--debug default = %q, want %q", f.DefValue, "false")
	}
}

func TestRootCmd_HasLimitFlag(t *testing.T) {
	cmd := NewRootCmd()
	f := cmd.PersistentFlags().Lookup("limit")
	if f == nil {
		t.Fatal("missing --limit flag")
	}
	if f.DefValue != "0" {
		t.Errorf("--limit default = %q, want %q", f.DefValue, "0")
	}
}

func TestFlickrCmd_RequiresSubcommand(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"flickr"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	// Running "flickr" alone should show help (not error)
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestS3UploadCmd_RequiresBucketFlag(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"s3", "upload", "/tmp/photos"})
	buf := new(bytes.Buffer)
	cmd.SetErr(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing --bucket flag")
	}
}

func TestS3DownloadCmd_RequiresBucketFlag(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"s3", "download", "/tmp/photos"})
	buf := new(bytes.Buffer)
	cmd.SetErr(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing --bucket flag")
	}
}

func TestFlickrDownloadCmd_RequiresArg(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"flickr", "download"})
	buf := new(bytes.Buffer)
	cmd.SetErr(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing output-dir arg")
	}
}

func TestGoogleUploadCmd_RequiresArg(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"google", "upload"})
	buf := new(bytes.Buffer)
	cmd.SetErr(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing input-dir arg")
	}
}

func TestGoogleImportTakeoutCmd_RequiresTwoArgs(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"google", "import-takeout", "/only/one"})
	buf := new(bytes.Buffer)
	cmd.SetErr(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing second arg")
	}
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/cli/ -v`
Expected: All pass

- [ ] **Step 3: Run lint**

Run: `golangci-lint run ./internal/cli/`
Expected: Clean

- [ ] **Step 4: Commit**

```bash
git add internal/cli/cli_test.go
git commit -m "Add CLI command structure, flag, and argument validation tests"
```

---

## Chunk 6: Final Verification

### Task 11: Full verification pass

- [ ] **Step 1: Run all linters**

Run: `golangci-lint run ./...`
Expected: Zero warnings/errors

- [ ] **Step 2: Run all tests**

Run: `go test ./... -v`
Expected: All pass

- [ ] **Step 3: Verify pre-commit hook works**

Run: `golangci-lint run ./... && go test ./...`
Expected: Both pass (this is what the hook runs)

- [ ] **Step 4: Final commit if any cleanup was needed**

Only if fixes were required in steps 1-3.
