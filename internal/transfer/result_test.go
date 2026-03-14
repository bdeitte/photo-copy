package transfer

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
		if w.File == "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected count mismatch warning, got: %v", r.Warnings)
	}
}

func TestValidate_TransferLogConsistency(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "transfer.log")
	if err := os.WriteFile(logPath, []byte("111\n222\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "222_abc.jpg"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	r := NewResult("flickr", "download", dir)
	r.Finish()
	r.ValidateTransferLog(logPath, func(id string) string {
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
	if !r.Scanned {
		t.Fatal("expected Scanned to be true")
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
