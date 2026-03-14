# Transfer Summary & Validation Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add consistent post-transfer validation and summary reporting across all services (Flickr, Google Photos, S3), with file count verification, transfer log consistency checks, zero-size file detection, error re-display, and a detailed report file.

**Architecture:** Introduce a shared `transfer` package with a `Result` struct that all Download/Upload methods populate. Each service accumulates results during its operation and returns a `*transfer.Result`. The CLI layer calls `result.Validate()` to run post-transfer checks, prints the summary to stderr, and writes a detailed report file. S3 scans the target directory after rclone completes. A shared `transfer.HandleResult()` helper avoids duplicating the validate/summary/report logic across CLI commands.

**Tech Stack:** Go standard library (no new dependencies)

**Key design decisions:**
- Count mismatch validation uses `Succeeded + Skipped + Failed` vs `Expected` — a mismatch means some files were unaccounted for (not that failures occurred).
- S3 uses `ScanDir()` since rclone handles its own transfer tracking. The summary labels scanned results as "files in directory" rather than "succeeded" to avoid confusion.
- Flickr Upload changes from fail-fast to continue-on-error (matching Download behavior). The integration test `TestFlickrUpload_FailsOnError` must be updated accordingly.
- All methods return a valid `*Result` even on early returns (e.g., "no files to upload"), so the CLI always gets a summary to print.

---

## File Structure

| Action | File | Responsibility |
|--------|------|----------------|
| Create | `internal/transfer/result.go` | `Result` struct, `Validate()`, `PrintSummary()`, `WriteReport()` |
| Create | `internal/transfer/result_test.go` | Tests for Result validation and formatting |
| Modify | `internal/flickr/flickr.go` | Return `*transfer.Result` from Download/Upload |
| Modify | `internal/flickr/flickr_test.go` | Update tests for new return type |
| Modify | `internal/google/google.go` | Return `*transfer.Result` from Upload |
| Modify | `internal/google/google_test.go` | Update tests for new return type |
| Modify | `internal/google/takeout.go` | Return `*transfer.Result` from ImportTakeout |
| Modify | `internal/google/takeout_test.go` | Update tests for new return type |
| Modify | `internal/s3/s3.go` | Return `*transfer.Result` from Upload/Download |
| Modify | `internal/s3/s3_test.go` | Update tests for new return type |
| Modify | `internal/cli/flickr.go` | Handle Result, call Validate/PrintSummary/WriteReport |
| Modify | `internal/cli/google.go` | Handle Result, call Validate/PrintSummary/WriteReport |
| Modify | `internal/cli/s3.go` | Handle Result, call Validate/PrintSummary/WriteReport |
| Modify | `internal/cli/integration_test.go` | Update integration tests for new return types and behavior changes |

---

## Chunk 1: The `transfer` Package

### Task 1: Define the Result struct and core types

**Files:**
- Create: `internal/transfer/result.go`
- Create: `internal/transfer/result_test.go`

- [ ] **Step 1: Write tests for Result creation and field tracking**

```go
// internal/transfer/result_test.go
package transfer

import (
	"testing"
)

func TestNewResult(t *testing.T) {
	r := NewResult("flickr", "download", "/tmp/photos")
	if r.Service != "flickr" {
		t.Fatalf("expected service flickr, got %s", r.Service)
	}
	if r.Operation != "download" {
		t.Fatalf("expected operation download, got %s", r.Operation)
	}
	if r.Dir != "/tmp/photos" {
		t.Fatalf("expected dir /tmp/photos, got %s", r.Dir)
	}
}

func TestResult_RecordSuccess(t *testing.T) {
	r := NewResult("flickr", "download", "/tmp")
	r.RecordSuccess("photo1.jpg", 1024)
	r.RecordSuccess("photo2.jpg", 2048)

	if r.Succeeded != 2 {
		t.Fatalf("expected 2 succeeded, got %d", r.Succeeded)
	}
	if r.TotalBytes != 3072 {
		t.Fatalf("expected 3072 bytes, got %d", r.TotalBytes)
	}
}

func TestResult_RecordError(t *testing.T) {
	r := NewResult("flickr", "download", "/tmp")
	r.RecordError("bad.jpg", "HTTP 500")

	if r.Failed != 1 {
		t.Fatalf("expected 1 failed, got %d", r.Failed)
	}
	if len(r.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(r.Errors))
	}
	if r.Errors[0].File != "bad.jpg" {
		t.Fatalf("expected error file bad.jpg, got %s", r.Errors[0].File)
	}
}

func TestResult_RecordSkip(t *testing.T) {
	r := NewResult("flickr", "download", "/tmp")
	r.RecordSkip(5)

	if r.Skipped != 5 {
		t.Fatalf("expected 5 skipped, got %d", r.Skipped)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/transfer/ -run TestNewResult -v`
Expected: FAIL — package doesn't exist yet

- [ ] **Step 3: Write the Result struct implementation**

```go
// internal/transfer/result.go
package transfer

import (
	"time"
)

// FileError records a per-file failure for the end-of-transfer summary.
type FileError struct {
	File   string
	Reason string
}

// ValidationWarning describes a post-transfer validation issue.
type ValidationWarning struct {
	File    string // empty for non-file-specific warnings
	Message string
}

// Result collects transfer statistics, errors, and validation results.
type Result struct {
	Service   string
	Operation string // "download", "upload", "import"
	Dir       string // output dir (download) or input dir (upload)

	Expected   int // expected total (e.g., API-reported photo count); 0 if unknown
	Succeeded  int
	Failed     int
	Skipped    int
	TotalBytes int64

	Scanned    bool // true when counts come from directory scan (S3) rather than per-file tracking

	Errors   []FileError
	Warnings []ValidationWarning

	StartTime time.Time
	EndTime   time.Time
}

// NewResult creates a Result and records the start time.
func NewResult(service, operation, dir string) *Result {
	return &Result{
		Service:   service,
		Operation: operation,
		Dir:       dir,
		StartTime: time.Now(),
	}
}

// RecordSuccess increments the success counter and accumulates bytes.
func (r *Result) RecordSuccess(filename string, sizeBytes int64) {
	r.Succeeded++
	r.TotalBytes += sizeBytes
}

// RecordError records a per-file failure.
func (r *Result) RecordError(filename, reason string) {
	r.Failed++
	r.Errors = append(r.Errors, FileError{File: filename, Reason: reason})
}

// RecordSkip adds to the skipped count (already-transferred files).
func (r *Result) RecordSkip(count int) {
	r.Skipped += count
}

// Finish records the end time.
func (r *Result) Finish() {
	r.EndTime = time.Now()
}

// Duration returns the elapsed transfer time.
func (r *Result) Duration() time.Duration {
	if r.EndTime.IsZero() {
		return time.Since(r.StartTime)
	}
	return r.EndTime.Sub(r.StartTime)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/transfer/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/transfer/result.go internal/transfer/result_test.go
git commit -m "Add transfer.Result struct for tracking transfer statistics"
```

### Task 2: Add Validate method (zero-size check, count verification, transfer log consistency)

**Files:**
- Modify: `internal/transfer/result.go`
- Modify: `internal/transfer/result_test.go`

- [ ] **Step 1: Write tests for Validate**

```go
// Add to internal/transfer/result_test.go

func TestValidate_ZeroSizeFiles(t *testing.T) {
	dir := t.TempDir()
	// Create a zero-size file
	if err := os.WriteFile(filepath.Join(dir, "empty.jpg"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	// Create a normal file
	if err := os.WriteFile(filepath.Join(dir, "ok.jpg"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewResult("flickr", "download", dir)
	r.RecordSuccess("empty.jpg", 0)
	r.RecordSuccess("ok.jpg", 4)
	r.Finish()

	r.Validate()

	if len(r.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(r.Warnings), r.Warnings)
	}
	if r.Warnings[0].File != "empty.jpg" {
		t.Fatalf("expected warning for empty.jpg, got %s", r.Warnings[0].File)
	}
}

func TestValidate_CountMismatch(t *testing.T) {
	r := NewResult("flickr", "download", t.TempDir())
	r.Expected = 10
	r.RecordSuccess("a.jpg", 100)
	r.Finish()

	r.Validate()

	found := false
	for _, w := range r.Warnings {
		if w.File == "" { // non-file-specific warning
			found = true
		}
	}
	if !found {
		t.Fatalf("expected count mismatch warning, got: %v", r.Warnings)
	}
}

func TestValidate_TransferLogConsistency(t *testing.T) {
	dir := t.TempDir()

	// Transfer log says "111" was downloaded, but file doesn't exist
	logPath := filepath.Join(dir, "transfer.log")
	if err := os.WriteFile(logPath, []byte("111\n222\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Only create file for 222
	if err := os.WriteFile(filepath.Join(dir, "222_abc.jpg"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewResult("flickr", "download", dir)
	r.Finish()

	r.ValidateTransferLog(logPath, func(id string) string {
		// Simulate: check if any file starts with this ID
		matches, _ := filepath.Glob(filepath.Join(dir, id+"_*"))
		if len(matches) > 0 {
			return matches[0]
		}
		return ""
	})

	if len(r.Warnings) != 1 {
		t.Fatalf("expected 1 warning for missing file, got %d: %v", len(r.Warnings), r.Warnings)
	}
}

func TestValidate_NoWarningsWhenClean(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ok.jpg"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewResult("flickr", "download", dir)
	r.Expected = 1
	r.RecordSuccess("ok.jpg", 4)
	r.Finish()

	r.Validate()

	if len(r.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %d: %v", len(r.Warnings), r.Warnings)
	}
}

func TestValidate_NoMismatchWhenFailuresAccountedFor(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ok.jpg"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewResult("flickr", "download", dir)
	r.Expected = 2
	r.RecordSuccess("ok.jpg", 4)
	r.RecordError("bad.jpg", "HTTP 500")
	r.Finish()

	r.Validate()

	// No count mismatch warning — 1 succeeded + 1 failed = 2 = expected
	for _, w := range r.Warnings {
		if w.File == "" {
			t.Fatalf("unexpected count mismatch warning: %s", w.Message)
		}
	}
}

func TestResult_RecordSkip_Accumulates(t *testing.T) {
	r := NewResult("flickr", "download", "/tmp")
	r.RecordSkip(3)
	r.RecordSkip(2)

	if r.Skipped != 5 {
		t.Fatalf("expected 5 skipped, got %d", r.Skipped)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/transfer/ -run TestValidate -v`
Expected: FAIL — Validate and ValidateTransferLog don't exist

- [ ] **Step 3: Implement Validate and ValidateTransferLog**

```go
// Update the import block in internal/transfer/result.go to include all needed imports:

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

// Validate runs post-transfer checks: count verification and zero-size file detection.
func (r *Result) Validate() {
	// Check expected vs actual count — includes failed since those were accounted for
	accounted := r.Succeeded + r.Skipped + r.Failed
	if r.Expected > 0 && accounted != r.Expected {
		r.Warnings = append(r.Warnings, ValidationWarning{
			Message: fmt.Sprintf("expected %d files but processed %d (succeeded=%d, skipped=%d, failed=%d)",
				r.Expected, accounted, r.Succeeded, r.Skipped, r.Failed),
		})
	}

	// Check for zero-size files in the output directory
	if r.Dir != "" && (r.Operation == "download" || r.Operation == "import") {
		entries, err := os.ReadDir(r.Dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			if info.Size() == 0 {
				r.Warnings = append(r.Warnings, ValidationWarning{
					File:    e.Name(),
					Message: "zero-size file",
				})
			}
		}
	}
}

// ValidateTransferLog checks that each entry in the transfer log has a corresponding
// file on disk. The resolve function maps a log entry (e.g., photo ID) to a file path,
// returning "" if no matching file exists.
func (r *Result) ValidateTransferLog(logPath string, resolve func(entry string) string) {
	f, err := os.Open(logPath)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		entry := strings.TrimSpace(scanner.Text())
		if entry == "" {
			continue
		}
		if resolve(entry) == "" {
			r.Warnings = append(r.Warnings, ValidationWarning{
				File:    entry,
				Message: fmt.Sprintf("transfer log entry %q has no matching file on disk", entry),
			})
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/transfer/ -run TestValidate -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/transfer/result.go internal/transfer/result_test.go
git commit -m "Add Validate and ValidateTransferLog to transfer.Result"
```

### Task 3: Add PrintSummary and WriteReport

**Files:**
- Modify: `internal/transfer/result.go`
- Modify: `internal/transfer/result_test.go`

- [ ] **Step 1: Write tests for PrintSummary and WriteReport**

```go
// Add to internal/transfer/result_test.go

func TestPrintSummary(t *testing.T) {
	var buf bytes.Buffer
	log := logging.New(false, &buf)

	r := NewResult("flickr", "download", t.TempDir())
	r.Expected = 10
	r.Succeeded = 8
	r.Skipped = 1
	r.Failed = 1
	r.TotalBytes = 1024 * 1024 * 50 // 50 MB
	r.RecordError("bad.jpg", "HTTP 500")
	r.Finish()

	r.PrintSummary(log)

	output := buf.String()
	if !strings.Contains(output, "8 succeeded") {
		t.Fatalf("expected '8 succeeded' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "1 failed") {
		t.Fatalf("expected '1 failed' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "bad.jpg") {
		t.Fatalf("expected failed file 'bad.jpg' in output, got:\n%s", output)
	}
}

func TestWriteReport(t *testing.T) {
	dir := t.TempDir()

	r := NewResult("flickr", "download", dir)
	r.Expected = 5
	r.Succeeded = 4
	r.Failed = 1
	r.TotalBytes = 1024
	r.RecordError("bad.jpg", "HTTP 500")
	r.Warnings = append(r.Warnings, ValidationWarning{File: "empty.jpg", Message: "zero-size file"})
	r.Finish()

	reportPath, err := r.WriteReport(dir)
	if err != nil {
		t.Fatalf("WriteReport failed: %v", err)
	}

	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("reading report: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "flickr") {
		t.Fatalf("expected 'flickr' in report, got:\n%s", content)
	}
	if !strings.Contains(content, "bad.jpg") {
		t.Fatalf("expected 'bad.jpg' in report, got:\n%s", content)
	}
	if !strings.Contains(content, "zero-size") {
		t.Fatalf("expected 'zero-size' in report, got:\n%s", content)
	}
}

func TestWriteReport_Clean(t *testing.T) {
	dir := t.TempDir()

	r := NewResult("flickr", "download", dir)
	r.Succeeded = 5
	r.Finish()

	reportPath, err := r.WriteReport(dir)
	if err != nil {
		t.Fatalf("WriteReport failed: %v", err)
	}

	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("reading report: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "No errors") {
		t.Fatalf("expected 'No errors' in clean report, got:\n%s", content)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/transfer/ -run "TestPrintSummary|TestWriteReport" -v`
Expected: FAIL

- [ ] **Step 3: Implement PrintSummary and WriteReport**

```go
// Add to internal/transfer/result.go (update imports to also include
// "path/filepath" and "github.com/briandeitte/photo-copy/internal/logging")

// PrintSummary writes a human-readable summary to the logger (stderr).
func (r *Result) PrintSummary(log *logging.Logger) {
	log.Info("")
	log.Info("=== %s %s summary ===", r.Service, r.Operation)

	if r.Scanned {
		// S3/rclone: we scanned the directory after transfer, not tracked per-file
		log.Info("files in directory: %d", r.Succeeded)
	} else {
		parts := []string{fmt.Sprintf("%d succeeded", r.Succeeded)}
		if r.Skipped > 0 {
			parts = append(parts, fmt.Sprintf("%d skipped", r.Skipped))
		}
		if r.Failed > 0 {
			parts = append(parts, fmt.Sprintf("%d failed", r.Failed))
		}
		log.Info("files: %s", strings.Join(parts, ", "))
	}

	if r.Expected > 0 {
		log.Info("expected: %d", r.Expected)
	}
	log.Info("total size: %s", formatBytes(r.TotalBytes))
	log.Info("duration: %s", r.Duration().Truncate(time.Second))

	if len(r.Errors) > 0 {
		log.Info("")
		log.Info("failed files:")
		for _, e := range r.Errors {
			log.Error("  %s: %s", e.File, e.Reason)
		}
	}

	if len(r.Warnings) > 0 {
		log.Info("")
		log.Info("validation warnings:")
		for _, w := range r.Warnings {
			if w.File != "" {
				log.Info("  [%s] %s", w.File, w.Message)
			} else {
				log.Info("  %s", w.Message)
			}
		}
	}

	if len(r.Errors) == 0 && len(r.Warnings) == 0 {
		log.Info("status: OK")
	}
}

// HandleResult runs validation, prints summary, and writes report.
// This is the standard CLI handler — call it from every command's RunE.
func HandleResult(result *Result, log *logging.Logger, reportDir string) {
	if result == nil {
		return
	}
	result.Validate()
	result.PrintSummary(log)
	if reportPath, err := result.WriteReport(reportDir); err != nil {
		log.Error("writing report: %v", err)
	} else {
		log.Info("report written to %s", reportPath)
	}
}

// WriteReport writes a detailed report file and returns its path.
func (r *Result) WriteReport(dir string) (string, error) {
	filename := fmt.Sprintf("photo-copy-report-%s-%s-%s.txt",
		r.Service, r.Operation, r.StartTime.Format("20060102-150405"))
	reportPath := filepath.Join(dir, filename)

	var b strings.Builder
	fmt.Fprintf(&b, "photo-copy %s %s report\n", r.Service, r.Operation)
	fmt.Fprintf(&b, "generated: %s\n", r.EndTime.Format(time.RFC3339))
	fmt.Fprintf(&b, "duration: %s\n", r.Duration().Truncate(time.Second))
	fmt.Fprintf(&b, "directory: %s\n\n", r.Dir)

	fmt.Fprintf(&b, "--- counts ---\n")
	fmt.Fprintf(&b, "succeeded: %d\n", r.Succeeded)
	fmt.Fprintf(&b, "skipped:   %d\n", r.Skipped)
	fmt.Fprintf(&b, "failed:    %d\n", r.Failed)
	if r.Expected > 0 {
		fmt.Fprintf(&b, "expected:  %d\n", r.Expected)
	}
	fmt.Fprintf(&b, "total size: %s\n\n", formatBytes(r.TotalBytes))

	fmt.Fprintf(&b, "--- errors ---\n")
	if len(r.Errors) == 0 {
		fmt.Fprintf(&b, "No errors.\n")
	} else {
		for _, e := range r.Errors {
			fmt.Fprintf(&b, "%s: %s\n", e.File, e.Reason)
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintf(&b, "--- validation ---\n")
	if len(r.Warnings) == 0 {
		fmt.Fprintf(&b, "No warnings.\n")
	} else {
		for _, w := range r.Warnings {
			if w.File != "" {
				fmt.Fprintf(&b, "[%s] %s\n", w.File, w.Message)
			} else {
				fmt.Fprintf(&b, "%s\n", w.Message)
			}
		}
	}

	return reportPath, os.WriteFile(reportPath, []byte(b.String()), 0644)
}

// formatBytes returns a human-readable byte size.
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/transfer/ -v`
Expected: PASS

- [ ] **Step 5: Run linter**

Run: `golangci-lint run ./internal/transfer/`
Expected: No issues

- [ ] **Step 6: Commit**

```bash
git add internal/transfer/result.go internal/transfer/result_test.go
git commit -m "Add PrintSummary and WriteReport to transfer.Result"
```

---

## Chunk 2: Integrate with Flickr

### Task 4: Update Flickr Download to return Result

**Files:**
- Modify: `internal/flickr/flickr.go:201-320` (Download method)
- Modify: `internal/flickr/flickr_test.go`

- [ ] **Step 1: Write a test for Flickr Download returning Result**

Add a test that calls Download and verifies the returned `*transfer.Result` has correct counts. Use the existing test server pattern from `flickr_test.go`. The test should verify:
- `result.Succeeded` matches number of downloaded files
- `result.Skipped` matches number of skipped (already in transfer log)
- `result.Expected` is set from API total
- `result.Service == "flickr"` and `result.Operation == "download"`
- Zero-size files produce validation warnings after `result.Validate()`

Look at the existing `TestDownload` tests in `flickr_test.go` for the httptest pattern — adapt one to check the Result.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/flickr/ -run TestDownload -v`
Expected: FAIL — Download returns error, not Result

- [ ] **Step 3: Update Download signature and implementation**

Change `Download` from returning `error` to returning `(*transfer.Result, error)`.

Key changes in `internal/flickr/flickr.go`:
1. Import `"github.com/briandeitte/photo-copy/internal/transfer"`
2. Change signature: `func (c *Client) Download(ctx context.Context, outputDir string, limit int) (*transfer.Result, error)`
3. Create result at top: `result := transfer.NewResult("flickr", "download", outputDir)`
4. Set `result.Expected = totalPhotos` when page 1 is fetched
5. Replace `totalDownloaded++` with `result.RecordSuccess(filename, fileSize)` — get file size from `os.Stat` after download
6. Replace `totalErrors++` with `result.RecordError(filename, err.Error())`
7. Replace `totalSkipped++` / `pageSkipped++` with `result.RecordSkip(1)` (use `result.Skipped` for skip counts in log messages)
8. At end, call `result.Finish()` and return `result, nil`
9. Remove the old summary log lines (lines 311-318) — the CLI will call PrintSummary instead
10. Keep the per-file info/error log lines for real-time progress

- [ ] **Step 4: Update all existing Download tests to handle new return type**

Every test calling `client.Download(...)` needs to change from:
```go
err := client.Download(ctx, dir, 0)
```
to:
```go
result, err := client.Download(ctx, dir, 0)
```
Add basic checks on `result` where appropriate.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/flickr/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/flickr/flickr.go internal/flickr/flickr_test.go
git commit -m "Return transfer.Result from Flickr Download"
```

### Task 5: Update Flickr Upload to return Result

**Files:**
- Modify: `internal/flickr/flickr.go:392-441` (Upload method)
- Modify: `internal/flickr/flickr_test.go`

- [ ] **Step 1: Write test for Flickr Upload returning Result**

- [ ] **Step 2: Run test to verify it fails**

- [ ] **Step 3: Update Upload signature and implementation**

Change `Upload` from returning `error` to returning `(*transfer.Result, error)`.

Key changes:
1. Change signature: `func (c *Client) Upload(ctx context.Context, inputDir string, limit int) (*transfer.Result, error)`
2. Create result at top: `result := transfer.NewResult("flickr", "upload", inputDir)`
3. When `len(files) == 0`: call `result.Finish()` and return `result, nil` (not just `nil`) so CLI gets a summary
4. Set `result.Expected = len(files)`
5. On success: get file size from `os.Stat`, call `result.RecordSuccess(filename, size)`
6. **Behavior change:** On error, instead of returning immediately, call `result.RecordError(filename, err.Error())` and continue — this matches Download behavior. The integration test `TestFlickrUpload_FailsOnError` must be updated in Task 12.
7. Call `result.Finish()` at end, return `result, nil`

- [ ] **Step 4: Update existing Upload tests**

- [ ] **Step 5: Run tests**

Run: `go test ./internal/flickr/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/flickr/flickr.go internal/flickr/flickr_test.go
git commit -m "Return transfer.Result from Flickr Upload"
```

### Task 6: Update Flickr CLI to handle Result

**Files:**
- Modify: `internal/cli/flickr.go`

- [ ] **Step 1: Update CLI to call Validate/PrintSummary/WriteReport**

In `newFlickrDownloadCmd`, change the RunE from:
```go
return client.Download(context.Background(), outputDir, opts.limit)
```
to:
```go
result, err := client.Download(context.Background(), outputDir, opts.limit)
transfer.HandleResult(result, log, outputDir)
return err
```

For download, also add transfer log validation before HandleResult:
```go
logPath := filepath.Join(outputDir, "transfer.log")
result.ValidateTransferLog(logPath, func(entry string) string {
    matches, _ := filepath.Glob(filepath.Join(outputDir, entry+"_*"))
    if len(matches) > 0 {
        return matches[0]
    }
    return ""
})
```

Apply same pattern to upload command (using `inputDir` for report location, no transfer log validation).

- [ ] **Step 2: Run linter and tests**

Run: `golangci-lint run ./internal/cli/ && go test ./internal/cli/ -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/cli/flickr.go
git commit -m "Add validation and summary to Flickr CLI commands"
```

---

## Chunk 3: Integrate with Google Photos

### Task 7: Update Google Upload to return Result

**Files:**
- Modify: `internal/google/google.go:102-181` (Upload method)
- Modify: `internal/google/google_test.go`

- [ ] **Step 1: Write test for Google Upload returning Result**

- [ ] **Step 2: Run test to verify it fails**

- [ ] **Step 3: Update Upload signature and implementation**

Change `Upload` from returning `error` to returning `(*transfer.Result, error)`.

Key changes:
1. Import `"github.com/briandeitte/photo-copy/internal/transfer"`
2. Change signature: `func (c *Client) Upload(ctx context.Context, inputDir string, limit int) (*transfer.Result, error)`
3. Create result at top: `result := transfer.NewResult("google-photos", "upload", inputDir)`
4. When `len(toUpload) == 0`: set `result.RecordSkip(len(uploaded))`, call `result.Finish()`, return `result, nil` (not just `nil`)
5. Set `result.Expected = len(toUpload)`
6. On success: get file size from `os.Stat`, call `result.RecordSuccess(filename, size)`
7. On error: call `result.RecordError(filename, err.Error())`
8. Set `result.RecordSkip(len(uploaded))` for already-uploaded files
9. Remove old summary log lines (lines 172-179)
10. Call `result.Finish()`, return `result, nil`

- [ ] **Step 4: Update existing Upload tests**

- [ ] **Step 5: Run tests**

Run: `go test ./internal/google/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/google/google.go internal/google/google_test.go
git commit -m "Return transfer.Result from Google Photos Upload"
```

### Task 8: Update Google Takeout ImportTakeout to return Result

**Files:**
- Modify: `internal/google/takeout.go`
- Modify: `internal/google/takeout_test.go`

- [ ] **Step 1: Write test for ImportTakeout returning Result**

- [ ] **Step 2: Run test to verify it fails**

- [ ] **Step 3: Update ImportTakeout signature and implementation**

Change `ImportTakeout` from returning `(int, error)` to returning `(*transfer.Result, error)`.

Key changes:
1. Import `"github.com/briandeitte/photo-copy/internal/transfer"`
2. Change signature: `func ImportTakeout(takeoutDir, outputDir string, log *logging.Logger) (*transfer.Result, error)`
3. Create result: `result := transfer.NewResult("google-takeout", "import", outputDir)`
4. Pass result into `extractMediaFromZip` (change its signature to accept `*transfer.Result`)
5. In `extractMediaFromZip`: call `result.RecordSuccess(name, info.Size())` after successful extraction, `result.RecordError(name, err.Error())` on failure
6. Remove old `fmt.Fprintf(os.Stderr, "Extracted %d media files\n", totalExtracted)` — the CLI will print the summary
7. Call `result.Finish()`, return `result, nil`

- [ ] **Step 4: Update existing ImportTakeout tests**

The existing tests check `count, err := ImportTakeout(...)`. Change to check `result, err := ImportTakeout(...)` and use `result.Succeeded` instead of `count`.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/google/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/google/takeout.go internal/google/takeout_test.go
git commit -m "Return transfer.Result from Google Takeout ImportTakeout"
```

### Task 9: Update Google CLI to handle Result

**Files:**
- Modify: `internal/cli/google.go`

- [ ] **Step 1: Update CLI to call Validate/PrintSummary/WriteReport**

Same pattern as Flickr CLI — handle the `*transfer.Result` return with `transfer.HandleResult(result, log, dir)`.

For upload: `transfer.HandleResult(result, log, inputDir)`
For `import-takeout`: `transfer.HandleResult(result, log, args[1])` (output dir)

- [ ] **Step 2: Run linter and tests**

Run: `golangci-lint run ./internal/cli/ && go test ./internal/cli/ -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/cli/google.go
git commit -m "Add validation and summary to Google Photos CLI commands"
```

---

## Chunk 4: Integrate with S3

### Task 10: Update S3 Upload/Download to return Result

**Files:**
- Modify: `internal/s3/s3.go`
- Modify: `internal/s3/s3_test.go`

- [ ] **Step 1: Write tests for S3 returning Result**

- [ ] **Step 2: Run tests to verify they fail**

- [ ] **Step 3: Update S3 Upload and Download implementations**

S3 is special — it delegates to rclone. We can't track per-file results. Instead:

For both Upload and Download:
1. Change signatures to return `(*transfer.Result, error)`
2. Create result: `result := transfer.NewResult("s3", "upload", inputDir)` (or "download")
3. After `runRclone` completes successfully, scan the target directory to count files and total size:
   ```go
   result.Finish()
   if err := result.ScanDir(); err != nil {
       // log but don't fail
   }
   return result, nil
   ```
4. If `runRclone` fails, still return the result (with the error) so the CLI can report partial results.

Add a `ScanDir()` method to `transfer.Result` that walks `r.Dir`, counts files, and sums their sizes. This sets `Scanned = true` so `PrintSummary` labels the output as "files in directory" rather than "succeeded":
```go
// Add to internal/transfer/result.go
func (r *Result) ScanDir() error {
    r.Scanned = true
    entries, err := os.ReadDir(r.Dir)
    if err != nil {
        return err
    }
    for _, e := range entries {
        if e.IsDir() {
            continue
        }
        info, err := e.Info()
        if err != nil {
            continue
        }
        r.Succeeded++
        r.TotalBytes += info.Size()
    }
    return nil
}
```

- [ ] **Step 4: Update existing S3 tests**

- [ ] **Step 5: Run tests**

Run: `go test ./internal/s3/ -v && go test ./internal/transfer/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/s3/s3.go internal/s3/s3_test.go internal/transfer/result.go internal/transfer/result_test.go
git commit -m "Return transfer.Result from S3 Upload/Download with directory scanning"
```

### Task 11: Update S3 CLI to handle Result

**Files:**
- Modify: `internal/cli/s3.go`

- [ ] **Step 1: Update CLI to call Validate/PrintSummary/WriteReport**

Same pattern as Flickr/Google CLI using `transfer.HandleResult(result, log, dir)`.

For upload: `transfer.HandleResult(result, log, args[0])` (inputDir)
For download: `transfer.HandleResult(result, log, args[0])` (outputDir)

- [ ] **Step 2: Run linter and tests**

Run: `golangci-lint run ./internal/cli/ && go test ./internal/cli/ -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/cli/s3.go
git commit -m "Add validation and summary to S3 CLI commands"
```

---

## Chunk 5: Integration tests and final validation

### Task 12: Update integration tests

**Files:**
- Modify: `internal/cli/integration_test.go`

- [ ] **Step 1: Review existing integration tests**

Read `internal/cli/integration_test.go` to understand how tests invoke commands and check output.

- [ ] **Step 2: Update `TestFlickrUpload_FailsOnError`**

This test currently expects upload to return an error on the first failure. Since Flickr Upload now continues past failures (matching Download behavior), update this test to:
- Expect `nil` error (upload completes)
- Verify stderr output contains "failed" in the summary
- Verify the report file is written and contains the error details

- [ ] **Step 3: Update other integration tests to verify summary output**

For all happy-path tests, add assertions that:
- The summary header appears (e.g., "=== flickr download summary ===")
- "report written to" appears in stderr
- The report file exists on disk

For error cases, verify:
- Failed files are re-listed in the summary
- Validation warnings appear when appropriate (e.g., zero-size files)

- [ ] **Step 4: Run integration tests**

Run: `go test ./internal/cli/ -tags integration -v`
Expected: PASS

- [ ] **Step 5: Run all tests and linter**

Run: `golangci-lint run ./... && go test ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/cli/integration_test.go
git commit -m "Update integration tests for transfer summary and validation"
```
