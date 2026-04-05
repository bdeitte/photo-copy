package s3

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/daterange"
	"github.com/briandeitte/photo-copy/internal/logging"
	"github.com/briandeitte/photo-copy/internal/transfer"
)

func TestBuildUploadArgs(t *testing.T) {
	args := buildUploadArgs("/tmp/config.conf", "/path/to/photos", "my-bucket", "backup/", "DEEP_ARCHIVE")
	expected := []string{
		"copy", "/path/to/photos", "s3:my-bucket/backup/",
		"--config", "/tmp/config.conf",
		"-v", "--use-json-log", "--stats", "0",
		"--s3-storage-class", "DEEP_ARCHIVE",
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

func TestBuildUploadArgs_NoPrefix(t *testing.T) {
	args := buildUploadArgs("/tmp/config.conf", "/path/to/photos", "my-bucket", "", "STANDARD")
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

func TestBuildUploadArgs_EmptyStorageClass(t *testing.T) {
	args := buildUploadArgs("/tmp/config.conf", "/path/to/photos", "my-bucket", "", "")
	for _, a := range args {
		if a == "--s3-storage-class" {
			t.Fatal("expected no --s3-storage-class flag when storage class is empty")
		}
	}
}

func TestBuildDownloadArgs(t *testing.T) {
	args := buildDownloadArgs("/tmp/config.conf", "my-bucket", "photos/", "/path/to/output")
	expected := []string{
		"copy", "s3:my-bucket/photos/", "/path/to/output",
		"--config", "/tmp/config.conf",
		"-v", "--use-json-log", "--stats", "0",
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

func TestBuildMediaIncludeFlags(t *testing.T) {
	flags := buildMediaIncludeFlags()
	if len(flags) == 0 {
		t.Fatal("expected include flags")
	}
	if flags[0] != "--ignore-case" {
		t.Fatalf("expected --ignore-case first, got %s", flags[0])
	}
	if flags[1] != "--include" {
		t.Fatalf("expected --include second, got %s", flags[1])
	}
}

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
	// First flag is --ignore-case, then --include/pattern pairs
	if flags[0] != "--ignore-case" {
		t.Fatalf("flags[0] = %q, want --ignore-case", flags[0])
	}
	rest := flags[1:]
	if len(rest)%2 != 0 {
		t.Fatalf("expected even number of flags after --ignore-case (--include pairs), got %d", len(rest))
	}
	for i := 0; i < len(rest); i += 2 {
		if rest[i] != "--include" {
			t.Errorf("flags[%d] = %q, want --include", i+1, rest[i])
		}
	}
}

func TestBuildMediaIncludeFlags_CoversExpectedExtensions(t *testing.T) {
	flags := buildMediaIncludeFlags()
	flagSet := make(map[string]bool)
	for i := 1; i < len(flags); i++ {
		if flags[i] != "--include" {
			flagSet[flags[i]] = true
		}
	}

	// With --ignore-case, only lowercase patterns are needed
	expected := []string{"*.jpg", "*.mp4", "*.heic", "*.png", "*.mov"}
	for _, ext := range expected {
		if !flagSet[ext] {
			t.Errorf("missing expected extension: %s", ext)
		}
	}
}

func TestBuildDateRangeFlags_Nil(t *testing.T) {
	flags := buildDateRangeFlags(nil)
	if flags != nil {
		t.Fatalf("expected nil, got %v", flags)
	}
}

func TestBuildDateRangeFlags_BothBounds(t *testing.T) {
	after := time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local)
	before := time.Date(2024, 1, 1, 0, 0, 0, 0, time.Local) // exclusive next day
	dr := &daterange.DateRange{After: &after, Before: &before}

	flags := buildDateRangeFlags(dr)

	expected := []string{"--max-age", "2020-01-01", "--min-age", "2024-01-01"}
	if len(flags) != len(expected) {
		t.Fatalf("expected %d flags, got %d: %v", len(expected), len(flags), flags)
	}
	for i, want := range expected {
		if flags[i] != want {
			t.Errorf("flags[%d] = %q, want %q", i, flags[i], want)
		}
	}
}

func TestBuildDateRangeFlags_AfterOnly(t *testing.T) {
	after := time.Date(2020, 6, 15, 0, 0, 0, 0, time.Local)
	dr := &daterange.DateRange{After: &after}

	flags := buildDateRangeFlags(dr)

	if len(flags) != 2 {
		t.Fatalf("expected 2 flags, got %d: %v", len(flags), flags)
	}
	if flags[0] != "--max-age" || flags[1] != "2020-06-15" {
		t.Errorf("got %v, want [--max-age 2020-06-15]", flags)
	}
}

func TestBuildDateRangeFlags_BeforeOnly(t *testing.T) {
	// User specifies --date-range :2023-12-31, so Before = 2024-01-01 (exclusive next day)
	before := time.Date(2024, 1, 1, 0, 0, 0, 0, time.Local)
	dr := &daterange.DateRange{Before: &before}

	flags := buildDateRangeFlags(dr)

	if len(flags) != 2 {
		t.Fatalf("expected 2 flags, got %d: %v", len(flags), flags)
	}
	// --min-age should use the exclusive next-day date directly
	if flags[0] != "--min-age" || flags[1] != "2024-01-01" {
		t.Errorf("got %v, want [--min-age 2024-01-01]", flags)
	}
}

func TestBuildDateRangeFlags_NoBounds(t *testing.T) {
	dr := &daterange.DateRange{}
	flags := buildDateRangeFlags(dr)
	if len(flags) != 0 {
		t.Fatalf("expected no flags for empty range, got %v", flags)
	}
}

func TestBuildFilterArgs_MediaOnlyAndDateRange(t *testing.T) {
	after := time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local)
	before := time.Date(2024, 1, 1, 0, 0, 0, 0, time.Local)
	dr := &daterange.DateRange{After: &after, Before: &before}

	flags := buildFilterArgs(true, dr)

	hasIgnoreCase := false
	hasInclude := false
	hasMaxAge := false
	hasMinAge := false
	for i, f := range flags {
		switch f {
		case "--ignore-case":
			hasIgnoreCase = true
		case "--include":
			hasInclude = true
		case "--max-age":
			if i+1 < len(flags) && flags[i+1] == "2020-01-01" {
				hasMaxAge = true
			}
		case "--min-age":
			if i+1 < len(flags) && flags[i+1] == "2024-01-01" {
				hasMinAge = true
			}
		}
	}
	if !hasIgnoreCase {
		t.Error("expected --ignore-case flag")
	}
	if !hasInclude {
		t.Error("expected --include flag")
	}
	if !hasMaxAge {
		t.Error("expected --max-age 2020-01-01 flag")
	}
	if !hasMinAge {
		t.Error("expected --min-age 2024-01-01 flag")
	}
}

func TestBuildFilterArgs_NoMediaNoDateRange(t *testing.T) {
	flags := buildFilterArgs(false, nil)
	if len(flags) != 0 {
		t.Fatalf("expected empty slice, got %v", flags)
	}
}

// TestHelperProcess is not a real test. It is used as a fake subprocess by
// tests that need to simulate rclone behavior. See exec.Command docs.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_TEST_HELPER_PROCESS") != "1" {
		return
	}
	mode := os.Getenv("GO_TEST_HELPER_MODE")
	switch mode {
	case "warning_then_fail":
		// Emit a JSON warning to stderr, then exit 1
		fmt.Fprintln(os.Stderr, `{"level":"warning","msg":"Failed to create file system: BadRequest"}`)
		os.Exit(1)
	case "exit_no_output":
		// Exit 1 with no stderr output
		os.Exit(1)
	case "success_no_output":
		// Exit 0 with no output — simulates no-op rclone (everything synced)
		os.Exit(0)
	case "lsf_glacier":
		_, _ = fmt.Fprintln(os.Stdout, "photo1.jpg;STANDARD")
		_, _ = fmt.Fprintln(os.Stdout, "photo2.jpg;DEEP_ARCHIVE")
		_, _ = fmt.Fprintln(os.Stdout, "video.mp4;GLACIER")
		os.Exit(0)
	case "restore_success":
		os.Exit(0)
	}
	os.Exit(0)
}

func TestRunRcloneWithProgress_WarningAndFailPreservesBoth(t *testing.T) {
	// Set env so the helper process activates
	t.Setenv("GO_TEST_HELPER_PROCESS", "1")
	t.Setenv("GO_TEST_HELPER_MODE", "warning_then_fail")

	log := logging.New(false, nil)
	client := NewClient(&config.S3Config{}, log)

	binary := os.Args[0]
	args := []string{"-test.run=TestHelperProcess", "--"}

	result := transfer.NewResult("s3", "upload", "/tmp")
	err := client.runRcloneWithProgress(context.Background(), binary, args, 0, "uploaded", result)

	if err == nil {
		t.Fatal("expected error from failed subprocess")
	}

	errMsg := err.Error()
	// Must contain the original exit error
	if !strings.Contains(errMsg, "exit status 1") {
		t.Errorf("error should contain exit status, got: %s", errMsg)
	}
	// Must contain the rclone warning message
	if !strings.Contains(errMsg, "Failed to create file system: BadRequest") {
		t.Errorf("error should contain rclone message, got: %s", errMsg)
	}
}

func TestRunRcloneWithProgress_FailWithoutWarningShowsExitError(t *testing.T) {
	t.Setenv("GO_TEST_HELPER_PROCESS", "1")
	t.Setenv("GO_TEST_HELPER_MODE", "exit_no_output")

	log := logging.New(false, nil)
	client := NewClient(&config.S3Config{}, log)

	binary := os.Args[0]
	args := []string{"-test.run=TestHelperProcess", "--"}

	result := transfer.NewResult("s3", "upload", "/tmp")
	err := client.runRcloneWithProgress(context.Background(), binary, args, 0, "uploaded", result)

	if err == nil {
		t.Fatal("expected error from failed subprocess")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "exit status 1") {
		t.Errorf("error should contain exit status, got: %s", errMsg)
	}
	// Should be a plain "rclone failed:" without a message appended
	if !strings.HasPrefix(errMsg, "rclone failed:") {
		t.Errorf("error should start with 'rclone failed:', got: %s", errMsg)
	}
}

// buildFakeRclone compiles a minimal Go binary that exits 0, suitable as a
// cross-platform fake rclone. Returns the path to the binary.
func buildFakeRclone(t *testing.T, dir string) string {
	t.Helper()
	name := rcloneBinaryName(runtime.GOOS, runtime.GOARCH)
	binary := filepath.Join(dir, name)

	src := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(src, []byte("package main\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", binary, src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building fake rclone: %v\n%s", err, out)
	}
	return binary
}

// TestClientDownload_NoOpProducesSummaryWithBytes drives Client.Download
// end-to-end with a fake rclone binary that exits 0 (simulating a no-op
// where everything is already synced). Verifies the returned Result has
// non-empty counts and real byte totals from ScanDir.
func TestClientDownload_NoOpProducesSummaryWithBytes(t *testing.T) {
	// Create a workspace with a fake rclone binary that exits 0
	workspace := t.TempDir()
	rcloneDir := filepath.Join(workspace, "tools-bin", "rclone")
	if err := os.MkdirAll(rcloneDir, 0755); err != nil {
		t.Fatal(err)
	}
	buildFakeRclone(t, rcloneDir)

	// Create output dir with pre-existing files (simulates already-synced state)
	outputDir := filepath.Join(workspace, "output")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "photo1.jpg"), []byte("jpeg-data-here"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "photo2.jpg"), []byte("more-jpeg-data"), 0644); err != nil {
		t.Fatal(err)
	}

	// Chdir so rcloneBinDir finds the fake binary via cwd fallback
	t.Chdir(workspace)

	log := logging.New(false, nil)
	client := NewClient(&config.S3Config{
		AccessKeyID:     "test",
		SecretAccessKey: "test",
		Region:          "us-east-1",
	}, log)

	result, err := client.Download(context.Background(), "test-bucket", "", outputDir, false, 0, nil)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	// ScanDir should have populated the result with real file data
	if result.Succeeded != 2 {
		t.Errorf("Succeeded = %d, want 2", result.Succeeded)
	}
	expectedBytes := int64(len("jpeg-data-here") + len("more-jpeg-data"))
	if result.TotalBytes != expectedBytes {
		t.Errorf("TotalBytes = %d, want %d", result.TotalBytes, expectedBytes)
	}
	// ScanLabel should be set by ScanDir
	if result.ScanLabel != "files in directory" {
		t.Errorf("ScanLabel = %q, want %q", result.ScanLabel, "files in directory")
	}
}
