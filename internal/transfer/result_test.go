package transfer

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/briandeitte/photo-copy/internal/logging"
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
	r.RecordSuccess(1024)
	r.RecordSuccess(2048)
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

func TestResult_RecordSkip_Accumulates(t *testing.T) {
	r := NewResult("flickr", "download", "/tmp")
	r.RecordSkip(3)
	r.RecordSkip(2)
	if r.Skipped != 5 {
		t.Fatalf("expected 5 skipped, got %d", r.Skipped)
	}
}

func TestValidate_ZeroSizeFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "empty.jpg"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ok.jpg"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	r := NewResult("flickr", "download", dir)
	r.RecordSuccess(0)
	r.RecordSuccess(4)
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
	r.RecordSuccess(100)
	r.Finish()
	r.Validate()
	found := false
	for _, w := range r.Warnings {
		if w.File == "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected count mismatch warning, got: %v", r.Warnings)
	}
}

func TestValidate_NoWarningsWhenClean(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ok.jpg"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	r := NewResult("flickr", "download", dir)
	r.Expected = 1
	r.RecordSuccess(4)
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
	r.RecordSuccess(4)
	r.RecordError("bad.jpg", "HTTP 500")
	r.Finish()
	r.Validate()
	for _, w := range r.Warnings {
		if w.File == "" {
			t.Fatalf("unexpected count mismatch warning: %s", w.Message)
		}
	}
}

func TestPrintSummary(t *testing.T) {
	var buf bytes.Buffer
	log := logging.New(false, &buf)

	r := NewResult("flickr", "download", t.TempDir())
	r.Expected = 10
	r.Succeeded = 8
	r.Skipped = 1
	r.TotalBytes = 1024 * 1024 * 50
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

func TestScanDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.jpg"), []byte("data1"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.jpg"), []byte("data22"), 0644); err != nil {
		t.Fatal(err)
	}
	r := NewResult("s3", "download", dir)
	if err := r.ScanDir(); err != nil {
		t.Fatalf("ScanDir failed: %v", err)
	}
	if r.Succeeded != 2 {
		t.Fatalf("expected 2 files, got %d", r.Succeeded)
	}
	if r.TotalBytes != 11 {
		t.Fatalf("expected 11 bytes, got %d", r.TotalBytes)
	}
}

func TestScanDir_IgnoresReportFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "photo.jpg"), []byte("jpeg"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "photo-copy-report-s3-download-20240101-100000.txt"), []byte("report data"), 0644); err != nil {
		t.Fatal(err)
	}
	r := NewResult("s3", "download", dir)
	if err := r.ScanDir(); err != nil {
		t.Fatalf("ScanDir failed: %v", err)
	}
	if r.Succeeded != 1 {
		t.Fatalf("expected 1 file (report excluded), got %d", r.Succeeded)
	}
	if r.TotalBytes != int64(len("jpeg")) {
		t.Fatalf("expected %d bytes, got %d", len("jpeg"), r.TotalBytes)
	}
}

func TestValidate_ZeroSizeFiles_SkippedForUpload(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "empty.jpg"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	r := NewResult("flickr", "upload", dir)
	r.RecordSuccess(0)
	r.Finish()
	r.Validate()
	for _, w := range r.Warnings {
		if w.Message == "zero-size file" {
			t.Fatalf("zero-size file warning should not appear for upload operations, got: %v", r.Warnings)
		}
	}
}

func TestResult_HandleNilResult(t *testing.T) {
	var buf bytes.Buffer
	log := logging.New(false, &buf)
	// Should return early without panic
	HandleResult(nil, log, t.TempDir())
}

func TestHandleResult_NoOpPrintsSummary(t *testing.T) {
	var buf bytes.Buffer
	log := logging.New(false, &buf)

	dir := t.TempDir()
	r := NewResult("s3", "upload", dir)
	r.Finish()

	HandleResult(r, log, dir)

	output := buf.String()
	if !strings.Contains(output, "s3 upload summary") {
		t.Fatalf("expected summary header for no-op result, got:\n%s", output)
	}
	if !strings.Contains(output, "0 succeeded") {
		t.Fatalf("expected '0 succeeded' in no-op summary, got:\n%s", output)
	}
}

func TestResult_Duration(t *testing.T) {
	r := NewResult("flickr", "download", "/tmp")
	r.StartTime = time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	r.EndTime = time.Date(2024, 1, 1, 10, 5, 30, 0, time.UTC)
	got := r.Duration()
	want := 5*time.Minute + 30*time.Second
	if got != want {
		t.Fatalf("Duration() = %v, want %v", got, want)
	}
}

func TestPrintSummary_WithRestoring(t *testing.T) {
	var buf bytes.Buffer
	log := logging.New(false, &buf)

	r := &Result{
		Service:   "s3",
		Operation: "download",
		Succeeded: 5,
		Restoring: 3,
		StartTime: time.Now(),
		EndTime:   time.Now(),
	}
	r.PrintSummary(log)

	output := buf.String()
	if !strings.Contains(output, "3 still restoring") {
		t.Errorf("expected '3 still restoring' in output, got:\n%s", output)
	}
}

func TestPrintSummary_NoRestoringWhenZero(t *testing.T) {
	var buf bytes.Buffer
	log := logging.New(false, &buf)

	r := &Result{
		Service:   "s3",
		Operation: "download",
		Succeeded: 5,
		Restoring: 0,
		StartTime: time.Now(),
		EndTime:   time.Now(),
	}
	r.PrintSummary(log)

	output := buf.String()
	if strings.Contains(output, "restoring") {
		t.Errorf("expected no 'restoring' in output when Restoring=0, got:\n%s", output)
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1024 * 1024, "1.0 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
	}
	for _, tt := range tests {
		got := formatBytes(tt.input)
		if got != tt.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
