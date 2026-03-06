package s3

import (
	"testing"
)

func TestBuildUploadArgs(t *testing.T) {
	args := buildUploadArgs("/tmp/config.conf", "/path/to/photos", "my-bucket", "backup/")
	expected := []string{
		"copy", "/path/to/photos", "s3:my-bucket/backup/",
		"--config", "/tmp/config.conf",
		"--progress",
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
		"--progress",
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
	if flags[0] != "--include" {
		t.Fatalf("expected --include, got %s", flags[0])
	}
}
