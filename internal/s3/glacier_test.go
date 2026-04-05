package s3

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsGlacierError_Matches(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"Failed to copy: failed to open source object: Object in GLACIER, restore first: bucket=\"b\", key=\"k\"", true},
		{"Object in GLACIER, restore first", true},
		{"GLACIER, restore first", true},
		{"Failed to copy: permission denied", false},
		{"some other error", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isGlacierError(tt.msg); got != tt.want {
			t.Errorf("isGlacierError(%q) = %v, want %v", tt.msg, got, tt.want)
		}
	}
}

func TestParseStorageClasses(t *testing.T) {
	input := "photo1.jpg;STANDARD\nphoto2.jpg;DEEP_ARCHIVE\nsubdir/photo3.mp4;GLACIER\nphoto4.png;STANDARD\n"
	glacier := parseStorageClasses(input)

	expected := []string{"photo2.jpg", "subdir/photo3.mp4"}
	if len(glacier) != len(expected) {
		t.Fatalf("got %d glacier files, want %d: %v", len(glacier), len(expected), glacier)
	}
	for i, want := range expected {
		if glacier[i] != want {
			t.Errorf("glacier[%d] = %q, want %q", i, glacier[i], want)
		}
	}
}

func TestParseStorageClasses_NoGlacier(t *testing.T) {
	input := "photo1.jpg;STANDARD\nphoto2.jpg;STANDARD\n"
	glacier := parseStorageClasses(input)
	if len(glacier) != 0 {
		t.Fatalf("expected no glacier files, got %v", glacier)
	}
}

func TestParseStorageClasses_Empty(t *testing.T) {
	glacier := parseStorageClasses("")
	if len(glacier) != 0 {
		t.Fatalf("expected no glacier files for empty input, got %v", glacier)
	}
}

func TestFilterOutExisting(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "exists.jpg"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	files := []string{"exists.jpg", "missing.jpg", "also-missing.mp4"}
	result := filterOutExisting(files, dir)

	expected := []string{"missing.jpg", "also-missing.mp4"}
	if len(result) != len(expected) {
		t.Fatalf("got %d files, want %d: %v", len(result), len(expected), result)
	}
	for i, want := range expected {
		if result[i] != want {
			t.Errorf("result[%d] = %q, want %q", i, result[i], want)
		}
	}
}

func TestFilterOutExisting_AllExist(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.jpg"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	result := filterOutExisting([]string{"a.jpg"}, dir)
	if len(result) != 0 {
		t.Fatalf("expected empty, got %v", result)
	}
}
