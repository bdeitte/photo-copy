# iCloud Photos Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add iCloud Photos download (via icloudpd) and upload (via osxphotos) support to photo-copy.

**Architecture:** Wraps two external Python CLI tools as subprocesses, following the same pattern as S3/rclone. Download via icloudpd (cross-platform), upload via osxphotos (macOS only, imports into Photos.app which syncs to iCloud). Config stores Apple ID and cookie directory path.

**Tech Stack:** Go, cobra CLI, icloudpd (Python, pip-installed), osxphotos (Python, pip-installed, macOS only)

**Spec:** `plans/2026-03-21-icloud-photos-design.md`

---

### Task 1: Add ICloudConfig to config package

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test for SaveICloudConfig and LoadICloudConfig**

Add to `internal/config/config_test.go`:

```go
func TestSaveAndLoadICloudConfig(t *testing.T) {
	tmpDir := t.TempDir()

	ic := &ICloudConfig{
		AppleID:   "user@example.com",
		CookieDir: "/tmp/cookies",
	}

	if err := SaveICloudConfig(tmpDir, ic); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := LoadICloudConfig(tmpDir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if loaded.AppleID != ic.AppleID {
		t.Fatalf("apple_id mismatch: got %s", loaded.AppleID)
	}
	if loaded.CookieDir != ic.CookieDir {
		t.Fatalf("cookie_dir mismatch: got %s", loaded.CookieDir)
	}
}

func TestLoadICloudConfig_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := LoadICloudConfig(tmpDir)
	if err == nil {
		t.Fatal("expected error for missing config")
	}
}

func TestLoadICloudConfig_MalformedJSON(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "icloud.json"), []byte("{bad"), 0644)

	_, err := LoadICloudConfig(tmpDir)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestSaveAndLoadICloudConfig -v`
Expected: FAIL — `ICloudConfig` undefined

- [ ] **Step 3: Write the implementation**

Add to `internal/config/config.go`:

```go
// Add to the const block:
icloudFile = "icloud.json"

// Add the struct after S3Config:
type ICloudConfig struct {
	AppleID   string `json:"apple_id"`
	CookieDir string `json:"cookie_dir"`
}

// Add the functions after LoadS3Config:
func SaveICloudConfig(configDir string, cfg *ICloudConfig) error {
	return saveJSON(configDir, icloudFile, cfg)
}

func LoadICloudConfig(configDir string) (*ICloudConfig, error) {
	var cfg ICloudConfig
	if err := loadJSON(configDir, icloudFile, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
```

- [ ] **Step 4: Run all config tests**

Run: `go test ./internal/config/ -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "Add ICloudConfig to config package"
```

---

### Task 2: Create icloud package — Client and tool discovery

**Files:**
- Create: `internal/icloud/icloud.go`
- Create: `internal/icloud/icloud_test.go`

- [ ] **Step 1: Write the failing test for NewClient and findTool**

Create `internal/icloud/icloud_test.go`:

```go
package icloud

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/logging"
)

func TestNewClient(t *testing.T) {
	cfg := &config.ICloudConfig{
		AppleID:   "user@example.com",
		CookieDir: "/tmp/cookies",
	}
	log := logging.New(false, nil)
	client := NewClient(cfg, log)

	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestFindTool_EnvOverride(t *testing.T) {
	// Create a real temp file to point the env var at
	tmpFile := filepath.Join(t.TempDir(), "fake-icloudpd")
	if err := os.WriteFile(tmpFile, []byte("#!/bin/sh"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PHOTO_COPY_ICLOUDPD_PATH", tmpFile)
	path, err := findTool("icloudpd", "PHOTO_COPY_ICLOUDPD_PATH")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != tmpFile {
		t.Fatalf("expected env override path, got %s", path)
	}
}

func TestFindTool_EnvOverride_NotFound(t *testing.T) {
	t.Setenv("PHOTO_COPY_ICLOUDPD_PATH", "/nonexistent/path/icloudpd")
	_, err := findTool("icloudpd", "PHOTO_COPY_ICLOUDPD_PATH")
	if err == nil {
		t.Fatal("expected error for nonexistent env override path")
	}
}

func TestFindTool_NotInstalled(t *testing.T) {
	t.Setenv("PHOTO_COPY_ICLOUDPD_PATH", "")
	// Use a tool name that definitely doesn't exist
	_, err := findTool("nonexistent-tool-xyz-12345", "PHOTO_COPY_NONEXISTENT_PATH")
	if err == nil {
		t.Fatal("expected error for missing tool")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/icloud/ -run TestNewClient -v`
Expected: FAIL — package doesn't exist

- [ ] **Step 3: Write the implementation**

Create `internal/icloud/icloud.go`:

```go
package icloud

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/logging"
)

// Client wraps icloudpd (download) and osxphotos (upload) as subprocesses.
type Client struct {
	cfg *config.ICloudConfig
	log *logging.Logger
}

// NewClient creates a new iCloud client. Tool paths are resolved lazily
// in Download() and Upload() so the client can be created even if only
// one tool is installed.
func NewClient(cfg *config.ICloudConfig, log *logging.Logger) *Client {
	return &Client{cfg: cfg, log: log}
}

// findTool resolves a tool path. Checks the env var override first
// (with existence check), then falls back to exec.LookPath.
func findTool(name, envVar string) (string, error) {
	if path := os.Getenv(envVar); path != "" {
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("%s path from %s not found: %s", name, envVar, path)
		}
		return path, nil
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s not found. Install it with: pipx install %s", name, name)
	}
	return path, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/icloud/ -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/icloud/icloud.go internal/icloud/icloud_test.go
git commit -m "Add icloud package with Client and tool discovery"
```

---

### Task 3: Add Logger.IsDebug() method

The icloud package needs `c.log.IsDebug()` to conditionally pass `--log-level debug` to icloudpd and `--verbose` to osxphotos. This method does not currently exist on the Logger.

**Files:**
- Modify: `internal/logging/logger.go`
- Modify: `internal/logging/logger_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/logging/logger_test.go`:

```go
func TestLogger_IsDebug(t *testing.T) {
	debugLog := New(true, nil)
	if !debugLog.IsDebug() {
		t.Fatal("expected IsDebug() true for debug logger")
	}

	normalLog := New(false, nil)
	if normalLog.IsDebug() {
		t.Fatal("expected IsDebug() false for normal logger")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/logging/ -run TestLogger_IsDebug -v`
Expected: FAIL — `IsDebug` not defined

- [ ] **Step 3: Add IsDebug method**

Add to `internal/logging/logger.go`:

```go
// IsDebug returns whether debug logging is enabled.
func (l *Logger) IsDebug() bool {
	return l.debug
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/logging/ -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/logging/logger.go internal/logging/logger_test.go
git commit -m "Add IsDebug() method to Logger"
```

---

### Task 4: Implement icloud download (icloudpd wrapper)

**Files:**
- Create: `internal/icloud/download.go`
- Modify: `internal/icloud/icloud_test.go`

- [ ] **Step 1: Write the failing test for buildDownloadArgs**

Add to `internal/icloud/icloud_test.go`:

```go
import (
	"time"

	"github.com/briandeitte/photo-copy/internal/daterange"
)

func TestBuildDownloadArgs(t *testing.T) {
	args := buildDownloadArgs("/output", "user@example.com", "/cookies", 0, nil, false)
	expected := []string{
		"--directory", "/output",
		"--username", "user@example.com",
		"--cookie-directory", "/cookies",
		"--no-progress-bar",
	}

	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
	}
	for i, want := range expected {
		if args[i] != want {
			t.Errorf("arg[%d] = %q, want %q", i, args[i], want)
		}
	}
}

func TestBuildDownloadArgs_WithLimit(t *testing.T) {
	args := buildDownloadArgs("/output", "user@example.com", "/cookies", 50, nil, false)
	found := false
	for i, a := range args {
		if a == "--recent" && i+1 < len(args) && args[i+1] == "50" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected --recent 50 in args, got: %v", args)
	}
}

func TestBuildDownloadArgs_WithDebug(t *testing.T) {
	args := buildDownloadArgs("/output", "user@example.com", "/cookies", 0, nil, true)
	found := false
	for _, a := range args {
		if a == "--log-level" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected --log-level in args, got: %v", args)
	}
}

func TestBuildDownloadArgs_WithDateRange(t *testing.T) {
	after := time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local)
	before := time.Date(2024, 1, 1, 0, 0, 0, 0, time.Local)
	dr := &daterange.DateRange{After: &after, Before: &before}

	args := buildDownloadArgs("/output", "user@example.com", "/cookies", 0, dr, false)

	// Check that date flags are present with correct values
	// Before is exclusive (2024-01-01 = start of next day), so --to-date should be 2023-12-31
	for i, a := range args {
		if a == "--from-date" {
			if i+1 >= len(args) || args[i+1] != "2020-01-01" {
				t.Errorf("expected --from-date 2020-01-01, got: %v", args)
			}
		}
		if a == "--to-date" {
			if i+1 >= len(args) || args[i+1] != "2023-12-31" {
				t.Errorf("expected --to-date 2023-12-31 (Before minus 1 day), got: %v", args)
			}
		}
	}
}
```

Also add output parsing tests:

```go
func TestParseDownloadLine(t *testing.T) {
	tests := []struct {
		line     string
		expected string
	}{
		{"Downloading IMG_1234.jpg", "IMG_1234.jpg"},
		{"downloading photo.png", "photo.png"},
		{"Some other line", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := parseDownloadLine(tt.line)
		if got != tt.expected {
			t.Errorf("parseDownloadLine(%q) = %q, want %q", tt.line, got, tt.expected)
		}
	}
}

func TestParsePhotoCount(t *testing.T) {
	tests := []struct {
		line     string
		expected int
	}{
		{"Found 1234 items", 1234},
		{"Found 50 photos", 50},
		{"Downloading 3 photos from album", 0}, // should NOT match without "Found"
		{"No count here", 0},
		{"", 0},
	}
	for _, tt := range tests {
		got := parsePhotoCount(tt.line)
		if got != tt.expected {
			t.Errorf("parsePhotoCount(%q) = %d, want %d", tt.line, got, tt.expected)
		}
	}
}

func TestIsSkipLine(t *testing.T) {
	if !isSkipLine("File already exists") {
		t.Error("expected true for 'already exists'")
	}
	if !isSkipLine("Skipping file") {
		t.Error("expected true for 'skipping'")
	}
	if isSkipLine("Downloading photo.jpg") {
		t.Error("expected false for download line")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/icloud/ -run "TestBuildDownloadArgs|TestParseDownloadLine|TestParsePhotoCount|TestIsSkipLine" -v`
Expected: FAIL — `buildDownloadArgs` undefined

- [ ] **Step 3: Write the implementation**

Create `internal/icloud/download.go`:

```go
package icloud

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/briandeitte/photo-copy/internal/daterange"
	"github.com/briandeitte/photo-copy/internal/transfer"
)

// Download downloads photos from iCloud via icloudpd.
func (c *Client) Download(ctx context.Context, outputDir string, limit int, dateRange *daterange.DateRange) (*transfer.Result, error) {
	result := transfer.NewResult("icloud", "download", outputDir)

	icloudpdPath, err := findTool("icloudpd", "PHOTO_COPY_ICLOUDPD_PATH")
	if err != nil {
		return result, err
	}

	args := buildDownloadArgs(outputDir, c.cfg.AppleID, c.cfg.CookieDir, limit, dateRange, c.log.IsDebug())

	c.log.Debug("running: %s %s", icloudpdPath, strings.Join(args, " "))

	cmd := exec.CommandContext(ctx, icloudpdPath, args...)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return result, fmt.Errorf("creating stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return result, fmt.Errorf("starting icloudpd: %w", err)
	}

	estimator := transfer.NewEstimator()
	downloaded := 0
	total := 0
	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		// Try to extract total count from icloudpd startup output
		if total == 0 {
			if n := parsePhotoCount(line); n > 0 {
				total = n
				c.log.Info("found %d photos in iCloud library", total)
			}
		}

		// Track download completions
		if filename := parseDownloadLine(line); filename != "" {
			downloaded++
			estimator.Tick()
			result.RecordSuccess(filename, 0)
			if total > 0 {
				remaining := total - downloaded
				c.log.Info("[%d/%d] %sdownloaded %s", downloaded, total, estimator.Estimate(remaining), filename)
			} else {
				c.log.Info("[%d] %sdownloaded %s", downloaded, estimator.Estimate(0), filename)
			}
			continue
		}

		// Track skipped files (already exist)
		if isSkipLine(line) {
			result.RecordSkip(1)
			c.log.Debug("icloudpd: %s", line)
			continue
		}

		// Pass through other output
		c.log.Debug("icloudpd: %s", line)
	}

	if err := scanner.Err(); err != nil {
		_ = cmd.Wait()
		return result, fmt.Errorf("reading icloudpd output: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		// Check for session expiry
		return result, fmt.Errorf("icloudpd failed: %w (if authentication expired, run 'photo-copy config icloud' to re-authenticate)", err)
	}

	result.Finish()
	return result, nil
}

func buildDownloadArgs(outputDir, appleID, cookieDir string, limit int, dateRange *daterange.DateRange, debug bool) []string {
	args := []string{
		"--directory", outputDir,
		"--username", appleID,
		"--cookie-directory", cookieDir,
		"--no-progress-bar",
	}

	if limit > 0 {
		args = append(args, "--recent", strconv.Itoa(limit))
	}

	if dateRange != nil {
		if dateRange.After != nil {
			args = append(args, "--from-date", dateRange.After.Format("2006-01-02"))
		}
		if dateRange.Before != nil {
			// dateRange.Before is exclusive (start of next day), subtract a day for icloudpd's inclusive --to-date
			toDate := dateRange.Before.AddDate(0, 0, -1)
			args = append(args, "--to-date", toDate.Format("2006-01-02"))
		}
	}

	if debug {
		args = append(args, "--log-level", "debug")
	}

	return args
}

// Regex to match icloudpd's "Downloading <filename>" output
var downloadLineRe = regexp.MustCompile(`(?i)downloading\s+(.+)`)

func parseDownloadLine(line string) string {
	m := downloadLineRe.FindStringSubmatch(line)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// Regex to match photo count like "Found 1234 items" — anchored with "Found" to avoid
// false positives on lines like "Downloading 3 photos from album..."
var photoCountRe = regexp.MustCompile(`(?i)found\s+(\d+)\s+(?:items?|photos?|assets?)`)

func parsePhotoCount(line string) int {
	m := photoCountRe.FindStringSubmatch(line)
	if m == nil {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return n
}

func isSkipLine(line string) bool {
	lower := strings.ToLower(line)
	return strings.Contains(lower, "already exists") || strings.Contains(lower, "skipping")
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/icloud/ -v`
Expected: All PASS

- [ ] **Step 5: Run lint**

Run: `golangci-lint run ./internal/icloud/...`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
git add internal/icloud/download.go internal/icloud/icloud_test.go
git commit -m "Add iCloud download via icloudpd subprocess"
```

---

### Task 5: Implement icloud upload (osxphotos wrapper)

**Files:**
- Create: `internal/icloud/upload.go`
- Modify: `internal/icloud/icloud_test.go`

- [ ] **Step 1: Write the failing test for buildUploadArgs**

Add to `internal/icloud/icloud_test.go`:

```go
func TestBuildUploadArgs_Basic(t *testing.T) {
	files := []string{"/photos/a.jpg", "/photos/b.png"}
	args := buildUploadArgs(files, false)

	// Should contain "import" and the file paths
	if args[0] != "import" {
		t.Fatalf("expected 'import' first, got %q", args[0])
	}

	hasFileA := false
	hasFileB := false
	for _, a := range args {
		if a == "/photos/a.jpg" {
			hasFileA = true
		}
		if a == "/photos/b.png" {
			hasFileB = true
		}
	}
	if !hasFileA || !hasFileB {
		t.Fatalf("expected both file paths in args, got: %v", args)
	}
}

func TestBuildUploadArgs_WithDebug(t *testing.T) {
	files := []string{"/photos/a.jpg"}
	args := buildUploadArgs(files, true)

	found := false
	for _, a := range args {
		if a == "--verbose" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected --verbose in args, got: %v", args)
	}
}

func TestCollectFiles_Basic(t *testing.T) {
	tmpDir := t.TempDir()
	// Create some test files
	for _, name := range []string{"a.jpg", "b.png", "c.txt", "d.mp4"} {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	files, err := collectFiles(tmpDir, 0, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should include a.jpg, b.png, d.mp4 but not c.txt
	if len(files) != 3 {
		t.Fatalf("expected 3 media files, got %d: %v", len(files), files)
	}
}

func TestCollectFiles_WithLimit(t *testing.T) {
	tmpDir := t.TempDir()
	for _, name := range []string{"a.jpg", "b.jpg", "c.jpg"} {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	files, err := collectFiles(tmpDir, 2, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files with limit, got %d", len(files))
	}
}
```

Also add output parsing tests and update the import block to include `"os"` and `"path/filepath"`:

```go
func TestParseImportLine(t *testing.T) {
	tests := []struct {
		line     string
		expected string
	}{
		{"Imported /path/to/photo.jpg", "photo.jpg"},
		{"Importing /path/to/video.mp4", "video.mp4"},
		{"Some other line", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := parseImportLine(tt.line)
		if got != tt.expected {
			t.Errorf("parseImportLine(%q) = %q, want %q", tt.line, got, tt.expected)
		}
	}
}

func TestParseImportError(t *testing.T) {
	// Should match targeted error patterns
	filename, reason := parseImportError("Error importing /path/to/photo.jpg: permission denied")
	if filename != "photo.jpg" {
		t.Errorf("expected filename 'photo.jpg', got %q", filename)
	}
	if reason != "permission denied" {
		t.Errorf("expected reason 'permission denied', got %q", reason)
	}

	// Should match "Failed to import"
	filename, reason = parseImportError("Failed to import /path/to/video.mp4")
	if filename != "video.mp4" {
		t.Errorf("expected filename 'video.mp4', got %q", filename)
	}

	// Should NOT match informational lines
	_, reason = parseImportError("0 errors")
	if reason != "" {
		t.Errorf("expected no match for '0 errors', got reason %q", reason)
	}

	// Should NOT match success lines
	_, reason = parseImportError("Imported photo.jpg")
	if reason != "" {
		t.Errorf("expected no match for success line, got reason %q", reason)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/icloud/ -run "TestBuildUploadArgs|TestCollectFiles|TestParseImportLine|TestParseImportError" -v`
Expected: FAIL — functions undefined

- [ ] **Step 3: Write the implementation**

Create `internal/icloud/upload.go`:

```go
package icloud

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/briandeitte/photo-copy/internal/daterange"
	"github.com/briandeitte/photo-copy/internal/media"
	"github.com/briandeitte/photo-copy/internal/mediadate"
	"github.com/briandeitte/photo-copy/internal/transfer"
)

// Upload imports photos into Photos.app via osxphotos. macOS only.
func (c *Client) Upload(ctx context.Context, inputDir string, limit int, dateRange *daterange.DateRange) (*transfer.Result, error) {
	result := transfer.NewResult("icloud", "upload", inputDir)

	if runtime.GOOS != "darwin" {
		return result, fmt.Errorf("iCloud upload requires macOS with Photos.app and iCloud Photos sync enabled")
	}

	osxphotosPath, err := findTool("osxphotos", "PHOTO_COPY_OSXPHOTOS_PATH")
	if err != nil {
		return result, err
	}

	files, err := collectFiles(inputDir, limit, dateRange)
	if err != nil {
		return result, fmt.Errorf("scanning files: %w", err)
	}

	if len(files) == 0 {
		c.log.Info("no files found to upload")
		result.Finish()
		return result, nil
	}

	c.log.Info("found %d files to import into Photos.app", len(files))

	args := buildUploadArgs(files, c.log.IsDebug())
	c.log.Debug("running: %s %s", osxphotosPath, strings.Join(args, " "))

	cmd := exec.CommandContext(ctx, osxphotosPath, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return result, fmt.Errorf("creating stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return result, fmt.Errorf("starting osxphotos: %w", err)
	}

	estimator := transfer.NewEstimator()
	imported := 0
	total := len(files)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		if filename := parseImportLine(line); filename != "" {
			imported++
			estimator.Tick()
			remaining := total - imported
			c.log.Info("[%d/%d] %suploaded %s", imported, total, estimator.Estimate(remaining), filename)
			result.RecordSuccess(filename, 0)
			continue
		}

		if filename, reason := parseImportError(line); reason != "" {
			c.log.Error("osxphotos: %s", line)
			result.RecordError(filename, reason)
			continue
		}

		c.log.Debug("osxphotos: %s", line)
	}

	if err := scanner.Err(); err != nil {
		_ = cmd.Wait()
		return result, fmt.Errorf("reading osxphotos output: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		result.Finish()
		return result, fmt.Errorf("osxphotos failed: %w", err)
	}

	result.Finish()
	return result, nil
}

func buildUploadArgs(files []string, debug bool) []string {
	args := []string{"import"}
	args = append(args, files...)

	if debug {
		args = append(args, "--verbose")
	}

	return args
}

// collectFiles walks inputDir and returns paths of supported media files,
// applying limit and date-range filters.
func collectFiles(inputDir string, limit int, dateRange *daterange.DateRange) ([]string, error) {
	var files []string

	err := filepath.WalkDir(inputDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !media.IsSupportedFile(path) {
			return nil
		}

		if dateRange != nil {
			fileDate := mediadate.ResolveDate(path)
			if fileDate.IsZero() {
				// Can't resolve date — include file anyway (same as other services)
				// Caller should enable --debug to see which files had unresolvable dates
			} else if !dateRange.Contains(fileDate) {
				return nil
			}
		}

		files = append(files, path)

		if limit > 0 && len(files) >= limit {
			return filepath.SkipAll
		}
		return nil
	})

	return files, err
}

// parseImportLine extracts a filename from osxphotos import output.
// osxphotos outputs lines like "Imported <filename>" during import.
func parseImportLine(line string) string {
	lower := strings.ToLower(line)
	if strings.Contains(lower, "imported") || strings.Contains(lower, "importing") {
		// Extract the last path component if present
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			return filepath.Base(parts[len(parts)-1])
		}
	}
	return ""
}

// importErrorRe matches osxphotos error lines like "Error importing /path/to/file: reason"
// or "Failed to import /path/to/file: reason". More targeted than broad substring matching
// to avoid false positives on informational lines like "0 errors".
var importErrorRe = regexp.MustCompile(`(?i)(?:error|failed)\s+(?:importing|to import)\s+(\S+)(?::\s*(.*))?`)

func parseImportError(line string) (filename, reason string) {
	m := importErrorRe.FindStringSubmatch(line)
	if m == nil {
		return "", ""
	}
	filename = filepath.Base(m[1])
	reason = line
	if len(m) > 2 && m[2] != "" {
		reason = m[2]
	}
	return filename, reason
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/icloud/ -v`
Expected: All PASS

- [ ] **Step 5: Run lint**

Run: `golangci-lint run ./internal/icloud/...`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
git add internal/icloud/upload.go internal/icloud/icloud_test.go
git commit -m "Add iCloud upload via osxphotos subprocess"
```

---

### Task 6: Add CLI commands — icloud download, icloud upload

**Files:**
- Create: `internal/cli/icloud.go`
- Modify: `internal/cli/root.go`

- [ ] **Step 1: Create the CLI command file**

Create `internal/cli/icloud.go`:

```go
package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/icloud"
	"github.com/briandeitte/photo-copy/internal/logging"
	"github.com/briandeitte/photo-copy/internal/transfer"
	"github.com/spf13/cobra"
)

func newICloudCmd(opts *rootOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "icloud",
		Short: "iCloud Photos upload and download commands",
	}

	cmd.AddCommand(newICloudDownloadCmd(opts))
	cmd.AddCommand(newICloudUploadCmd(opts))
	return cmd
}

func newICloudDownloadCmd(opts *rootOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "download <output-dir>",
		Short: "Download photos/videos from iCloud Photos",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadICloudConfig(config.DefaultDir())
			if err != nil {
				if errors.Is(err, config.ErrNotConfigured) {
					return fmt.Errorf("iCloud credentials not configured. Run 'photo-copy config icloud' to set up")
				}
				return fmt.Errorf("loading iCloud config: %w", err)
			}

			log := logging.New(opts.debug, nil)
			client := icloud.NewClient(cfg, log)
			result, err := client.Download(context.Background(), args[0], opts.limit, opts.parsedDateRange)
			if err != nil {
				return err
			}
			transfer.HandleResult(result, log, args[0])
			return nil
		},
	}

	return cmd
}

func newICloudUploadCmd(opts *rootOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upload <input-dir>",
		Short: "Import photos/videos into Photos.app (macOS only, syncs to iCloud)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			log := logging.New(opts.debug, nil)
			cfg := &config.ICloudConfig{}

			// Upload doesn't need iCloud credentials — it uses osxphotos locally
			// Try loading config for consistency, but don't require it
			if loaded, err := config.LoadICloudConfig(config.DefaultDir()); err == nil {
				cfg = loaded
			}

			client := icloud.NewClient(cfg, log)
			result, err := client.Upload(context.Background(), args[0], opts.limit, opts.parsedDateRange)
			if err != nil {
				return err
			}
			transfer.HandleResult(result, log, args[0])
			return nil
		},
	}

	return cmd
}
```

- [ ] **Step 2: Register icloud command in root.go**

In `internal/cli/root.go`, add `newICloudCmd(opts)` after the S3 command:

```go
// After: rootCmd.AddCommand(newS3Cmd(opts))
rootCmd.AddCommand(newICloudCmd(opts))
```

- [ ] **Step 3: Update PersistentPreRunE for --no-metadata warning**

In `internal/cli/root.go`, update the `--no-metadata` warning to also exclude iCloud commands (they don't have metadata operations). The existing check already covers this since it only allows Flickr download — no change needed for `--no-metadata`.

For `--date-range`, the existing config command check already covers `config icloud` since it checks `cmd.Parent().Name() == "config"`. No change needed.

- [ ] **Step 4: Verify the build compiles**

Run: `go build ./cmd/photo-copy/`
Expected: Compiles successfully

- [ ] **Step 5: Verify the command appears in help**

Run: `./photo-copy --help`
Expected: `icloud` appears in the command list

Run: `./photo-copy icloud --help`
Expected: Shows `download` and `upload` subcommands

- [ ] **Step 6: Run all tests**

Run: `go test ./...`
Expected: All PASS

- [ ] **Step 7: Run lint**

Run: `golangci-lint run ./...`
Expected: No errors

- [ ] **Step 8: Commit**

```bash
git add internal/cli/icloud.go internal/cli/root.go
git commit -m "Add icloud CLI commands for download and upload"
```

---

### Task 7: Add config icloud command

**Files:**
- Modify: `internal/cli/config.go`

- [ ] **Step 1: Add the config command**

Add to `internal/cli/config.go`:

```go
func newConfigICloudCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "icloud",
		Short: "Set up iCloud credentials and authenticate",
		RunE: func(cmd *cobra.Command, args []string) error {
			reader := bufio.NewReader(os.Stdin)

			fmt.Println("iCloud Setup")
			fmt.Println()

			// Check icloudpd is installed
			icloudpdPath, err := exec.LookPath("icloudpd")
			if err != nil {
				return fmt.Errorf("icloudpd not found. Install it with: pipx install icloudpd")
			}
			fmt.Printf("Found icloudpd at: %s\n", icloudpdPath)

			// Check osxphotos (optional)
			if osxphotosPath, err := exec.LookPath("osxphotos"); err == nil {
				fmt.Printf("Found osxphotos at: %s\n", osxphotosPath)
			} else {
				fmt.Println("Warning: osxphotos not found. Upload to iCloud will not be available.")
				fmt.Println("Install with: pipx install osxphotos")
			}
			fmt.Println()

			fmt.Print("Apple ID (email): ")
			appleID, _ := reader.ReadString('\n')
			appleID = strings.TrimSpace(appleID)

			if appleID == "" {
				return fmt.Errorf("Apple ID is required")
			}

			configDir := config.DefaultDir()
			cookieDir := filepath.Join(configDir, "icloud-cookies")
			if err := os.MkdirAll(cookieDir, 0700); err != nil {
				return fmt.Errorf("creating cookie directory: %w", err)
			}

			cfg := &config.ICloudConfig{
				AppleID:   appleID,
				CookieDir: cookieDir,
			}

			if err := config.SaveICloudConfig(configDir, cfg); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			fmt.Println("\nStarting icloudpd authentication (2FA required)...")
			fmt.Println("Follow the prompts to complete authentication.")
			fmt.Println()

			authCmd := exec.Command(icloudpdPath,
				"--username", appleID,
				"--cookie-directory", cookieDir,
				"--auth-only",
			)
			authCmd.Stdin = os.Stdin
			authCmd.Stdout = os.Stdout
			authCmd.Stderr = os.Stderr

			if err := authCmd.Run(); err != nil {
				return fmt.Errorf("icloudpd authentication failed: %w", err)
			}

			fmt.Println("\niCloud authentication complete! Credentials saved.")
			fmt.Println("Session cookies are valid for approximately 2 months.")
			fmt.Println("Re-run 'photo-copy config icloud' when they expire.")
			return nil
		},
	}
}
```

- [ ] **Step 2: Register in newConfigCmd**

In `internal/cli/config.go`, add to `newConfigCmd()`:

```go
cmd.AddCommand(newConfigICloudCmd())
```

- [ ] **Step 3: Add imports**

Add `"os/exec"` and `"path/filepath"` to the imports in `config.go` (if not already present).

- [ ] **Step 4: Verify build**

Run: `go build ./cmd/photo-copy/`
Expected: Compiles

Run: `./photo-copy config --help`
Expected: `icloud` appears in subcommands

- [ ] **Step 5: Run all tests and lint**

Run: `go test ./...`
Run: `golangci-lint run ./...`
Expected: All PASS, no lint errors

- [ ] **Step 6: Commit**

```bash
git add internal/cli/config.go
git commit -m "Add config icloud command for authentication setup"
```

---

### Task 8: Update README.md

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update tagline**

Change:
```
<b>Copy between Google Photos, Flickr, AWS S3, and local directories.</b>
```
To:
```
<b>Copy between iCloud Photos, Google Photos, Flickr, AWS S3, and local directories.</b>
```

- [ ] **Step 2: Add iCloud config command**

In the "Configure credentials" section, add:

```bash
./photo-copy config icloud   # iCloud authentication (requires icloudpd)
```

- [ ] **Step 3: Add iCloud usage section**

After the S3 section, add:

```markdown
### iCloud Photos

Download works on all platforms. Upload requires macOS with Photos.app and iCloud Photos sync enabled.

**Prerequisites:** Install [icloudpd](https://github.com/icloud-photos-downloader/icloud_photos_downloader) for downloads and [osxphotos](https://github.com/RhetTbull/osxphotos) for uploads:

\`\`\`bash
pipx install icloudpd       # Required for download (all platforms)
pipx install osxphotos      # Required for upload (macOS only)
\`\`\`

\`\`\`bash
# Download all photos from iCloud
./photo-copy icloud download ../icloud-photos

# Upload local photos to iCloud (macOS only — imports into Photos.app)
./photo-copy icloud upload ../photos
\`\`\`

**Notes:**
- Download requires Apple ID with 2FA. Run `photo-copy config icloud` to authenticate.
- Session cookies expire approximately every 2 months — re-run `config icloud` to re-authenticate.
- Advanced Data Protection must be disabled for downloads.
- Upload imports files into Photos.app. If iCloud Photos sync is enabled in System Settings, they automatically upload to iCloud.
- `--no-metadata` has no effect on iCloud commands.
```

- [ ] **Step 4: Update resumable transfers section**

Add to the resumable transfers list:

```markdown
- **iCloud downloads** — Handled by icloudpd, which skips files that already exist in the output directory by filename matching.
- **iCloud uploads** — Each run imports all files; Photos.app deduplicates automatically.
```

- [ ] **Step 5: Update date-range section**

Add to the "Date sources by command" list:

```markdown
- **iCloud download**: Uses icloudpd's date filtering (photo creation date from iCloud). Note: `--limit` maps to icloudpd's `--recent` flag, which selects the N most recently uploaded photos.
- **iCloud upload**: Reads EXIF DateTimeOriginal (JPEG) or MP4 creation time, falling back to file modification time (same as Flickr/Google upload).
```

- [ ] **Step 6: Commit**

```bash
git add README.md
git commit -m "Add iCloud Photos documentation to README"
```

---

### Task 9: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update the Short description**

In the root command description, change:
```
Short: "Copy photos and videos between Flickr, Google Photos, S3, and local directories"
```
Note: this is in `root.go`, but that was already handled. Just ensure CLAUDE.md references match.

- [ ] **Step 2: Add icloud package to package layout**

Add after the `s3/` entry in the package layout section:

```markdown
- **icloud/** — iCloud Photos client. Downloads via icloudpd subprocess (cross-platform). Uploads via osxphotos subprocess (macOS only, imports into Photos.app which syncs to iCloud). No direct Apple API — both operations delegate to external Python tools, similar to how S3 delegates to rclone.
```

- [ ] **Step 3: Add design constraints**

Add to the design constraints section:

```markdown
- **iCloud Photos upload:** macOS only — imports into Photos.app via osxphotos, relies on iCloud Photos sync to upload to cloud. No cross-platform upload API exists.
- **iCloud Photos authentication:** Requires Apple ID with 2FA. Session cookies expire ~2 months. Advanced Data Protection must be disabled.
```

- [ ] **Step 4: Update root command Short string in root.go**

In `internal/cli/root.go`, update:
```go
Short: "Copy photos and videos between iCloud Photos, Flickr, Google Photos, S3, and local directories",
```

- [ ] **Step 5: Run lint and tests**

Run: `golangci-lint run ./...`
Run: `go test ./...`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add CLAUDE.md internal/cli/root.go
git commit -m "Update CLAUDE.md and root command description for iCloud support"
```

---

### Task 10: Final verification

- [ ] **Step 1: Run full test suite**

Run: `go test ./...`
Expected: All PASS

- [ ] **Step 2: Run full lint**

Run: `golangci-lint run ./...`
Expected: No errors

- [ ] **Step 3: Build and verify help output**

Run: `go build -o photo-copy ./cmd/photo-copy`
Run: `./photo-copy --help`
Expected: Shows iCloud in description and command list

Run: `./photo-copy icloud --help`
Expected: Shows download and upload subcommands

Run: `./photo-copy config --help`
Expected: Shows icloud subcommand

- [ ] **Step 4: Verify icloud download help**

Run: `./photo-copy icloud download --help`
Expected: Shows usage with `<output-dir>` arg and inherited flags (`--debug`, `--limit`, `--date-range`)

- [ ] **Step 5: Verify icloud upload help**

Run: `./photo-copy icloud upload --help`
Expected: Shows usage with `<input-dir>` arg, mentions macOS in short description

- [ ] **Step 6: Commit any remaining changes**

If any fixes were needed, commit them.
