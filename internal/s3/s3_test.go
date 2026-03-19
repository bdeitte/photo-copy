package s3

import (
	"testing"
	"time"

	"github.com/briandeitte/photo-copy/internal/daterange"
)

func TestBuildUploadArgs(t *testing.T) {
	args := buildUploadArgs("/tmp/config.conf", "/path/to/photos", "my-bucket", "backup/")
	expected := []string{
		"copy", "/path/to/photos", "s3:my-bucket/backup/",
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

func TestBuildUploadArgs_NoPrefix(t *testing.T) {
	args := buildUploadArgs("/tmp/config.conf", "/path/to/photos", "my-bucket", "")
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
