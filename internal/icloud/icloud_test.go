package icloud

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/daterange"
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
	tmpFile := filepath.Join(t.TempDir(), "fake-icloudpd")
	if err := os.WriteFile(tmpFile, []byte("#!/bin/sh"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PHOTO_COPY_ICLOUDPD_PATH", tmpFile)
	path, err := FindTool("icloudpd", "PHOTO_COPY_ICLOUDPD_PATH")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != tmpFile {
		t.Fatalf("expected env override path, got %s", path)
	}
}

func TestFindTool_EnvOverride_NotFound(t *testing.T) {
	t.Setenv("PHOTO_COPY_ICLOUDPD_PATH", "/nonexistent/path/icloudpd")
	_, err := FindTool("icloudpd", "PHOTO_COPY_ICLOUDPD_PATH")
	if err == nil {
		t.Fatal("expected error for nonexistent env override path")
	}
}

func TestFindTool_EnvOverride_NotExecutable(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "fake-icloudpd")
	if err := os.WriteFile(tmpFile, []byte("not executable"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PHOTO_COPY_ICLOUDPD_PATH", tmpFile)
	_, err := FindTool("icloudpd", "PHOTO_COPY_ICLOUDPD_PATH")
	if err == nil {
		t.Fatal("expected error for non-executable env override path")
	}
}

func TestFindTool_NotInstalled(t *testing.T) {
	t.Setenv("PHOTO_COPY_ICLOUDPD_PATH", "")
	_, err := FindTool("nonexistent-tool-xyz-12345", "PHOTO_COPY_NONEXISTENT_PATH")
	if err == nil {
		t.Fatal("expected error for missing tool")
	}
}

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
	foundFrom := false
	foundTo := false
	for i, a := range args {
		if a == "--from-date" {
			foundFrom = true
			if i+1 >= len(args) || args[i+1] != "2020-01-01" {
				t.Errorf("expected --from-date 2020-01-01, got: %v", args)
			}
		}
		if a == "--to-date" {
			foundTo = true
			if i+1 >= len(args) || args[i+1] != "2023-12-31" {
				t.Errorf("expected --to-date 2023-12-31 (Before minus 1 day), got: %v", args)
			}
		}
	}
	if !foundFrom {
		t.Error("expected --from-date flag in args")
	}
	if !foundTo {
		t.Error("expected --to-date flag in args")
	}
}

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
		t.Run(tt.line, func(t *testing.T) {
			got := parseDownloadLine(tt.line)
			if got != tt.expected {
				t.Errorf("parseDownloadLine(%q) = %q, want %q", tt.line, got, tt.expected)
			}
		})
	}
}

func TestParseDownloadError(t *testing.T) {
	filename, reason := parseDownloadError("ERROR downloading IMG_5678.jpg: connection reset")
	if filename != "IMG_5678.jpg" {
		t.Errorf("expected filename 'IMG_5678.jpg', got %q", filename)
	}
	if reason != "connection reset" {
		t.Errorf("expected reason 'connection reset', got %q", reason)
	}

	filename, _ = parseDownloadError("error downloading photo.png")
	if filename != "photo.png" {
		t.Errorf("expected filename 'photo.png', got %q", filename)
	}

	_, reason = parseDownloadError("Downloading IMG_1234.jpg")
	if reason != "" {
		t.Errorf("expected no match for download line, got reason %q", reason)
	}

	_, reason = parseDownloadError("Found 50 photos")
	if reason != "" {
		t.Errorf("expected no match for count line, got reason %q", reason)
	}
}

func TestErrorLineNotMatchedAsSuccess(t *testing.T) {
	// Error lines containing "downloading" must be caught by parseDownloadError,
	// not parseDownloadLine. This verifies the parse ordering is correct.
	errorLine := "ERROR downloading IMG_5678.jpg: connection reset"

	// parseDownloadError must always match error lines
	filename, reason := parseDownloadError(errorLine)
	if filename == "" || reason == "" {
		t.Fatal("parseDownloadError should match error lines containing 'downloading'")
	}

	// Verify normal download lines are NOT matched by parseDownloadError
	normalLine := "Downloading IMG_1234.jpg"
	if _, reason := parseDownloadError(normalLine); reason != "" {
		t.Errorf("parseDownloadError should not match normal download line, got reason %q", reason)
	}
	if parseDownloadLine(normalLine) == "" {
		t.Error("parseDownloadLine should match normal download line")
	}
}

func TestParsePhotoCount(t *testing.T) {
	tests := []struct {
		line     string
		expected int
	}{
		{"Found 1234 items", 1234},
		{"Found 50 photos", 50},
		{"Downloading 3 photos from album", 0},
		{"No count here", 0},
		{"", 0},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got := parsePhotoCount(tt.line)
			if got != tt.expected {
				t.Errorf("parsePhotoCount(%q) = %d, want %d", tt.line, got, tt.expected)
			}
		})
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

func TestBuildUploadArgs_Basic(t *testing.T) {
	files := []string{"/photos/a.jpg", "/photos/b.png"}
	args := buildUploadArgs(files, false)

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
	for _, name := range []string{"a.jpg", "b.png", "c.txt", "d.mp4"} {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	files, err := collectFiles(tmpDir, 0, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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

func TestParseImportLine(t *testing.T) {
	tests := []struct {
		line     string
		expected string
	}{
		{"Imported /path/to/photo.jpg", "photo.jpg"},
		{"Importing /path/to/video.mp4", "video.mp4"},
		{"Some other line", ""},
		{"", ""},
		{"5 imported, 0 skipped", ""},
		{"Finished importing batch", ""},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got := parseImportLine(tt.line)
			if got != tt.expected {
				t.Errorf("parseImportLine(%q) = %q, want %q", tt.line, got, tt.expected)
			}
		})
	}
}

func TestParseImportError(t *testing.T) {
	filename, reason := parseImportError("Error importing /path/to/photo.jpg: permission denied")
	if filename != "photo.jpg" {
		t.Errorf("expected filename 'photo.jpg', got %q", filename)
	}
	if reason != "permission denied" {
		t.Errorf("expected reason 'permission denied', got %q", reason)
	}

	filename, reason = parseImportError("Failed to import /path/to/video.mp4")
	if filename != "video.mp4" {
		t.Errorf("expected filename 'video.mp4', got %q", filename)
	}
	if reason != "Failed to import /path/to/video.mp4" {
		t.Errorf("expected full line as reason fallback, got %q", reason)
	}

	_, reason = parseImportError("0 errors")
	if reason != "" {
		t.Errorf("expected no match for '0 errors', got reason %q", reason)
	}

	_, reason = parseImportError("Imported photo.jpg")
	if reason != "" {
		t.Errorf("expected no match for success line, got reason %q", reason)
	}
}

func TestFindTool_BundledBinary(t *testing.T) {
	t.Setenv("PHOTO_COPY_ICLOUDPD_PATH", "")
	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "tools-bin", "icloudpd")
	_ = os.MkdirAll(toolDir, 0755)

	goos := runtime.GOOS
	goarch := runtime.GOARCH
	binName := fmt.Sprintf("icloudpd-%s-%s", goos, goarch)
	fakeBin := filepath.Join(toolDir, binName)
	_ = os.WriteFile(fakeBin, []byte("#!/bin/sh"), 0755)

	path, err := FindTool("icloudpd", "PHOTO_COPY_ICLOUDPD_PATH", tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != fakeBin {
		t.Errorf("got %q, want %q", path, fakeBin)
	}
}

func TestFindTool_RosettaFallback(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("Rosetta fallback only applies on darwin/arm64")
	}

	t.Setenv("PHOTO_COPY_ICLOUDPD_PATH", "")
	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "tools-bin", "icloudpd")
	_ = os.MkdirAll(toolDir, 0755)

	fakeBin := filepath.Join(toolDir, "icloudpd-darwin-amd64")
	_ = os.WriteFile(fakeBin, []byte("#!/bin/sh"), 0755)

	path, err := FindTool("icloudpd", "PHOTO_COPY_ICLOUDPD_PATH", tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != fakeBin {
		t.Errorf("got %q, want %q", path, fakeBin)
	}
}
