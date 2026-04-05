package s3

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/briandeitte/photo-copy/internal/logging"
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
	result, err := filterOutExisting(files, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

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

	result, err := filterOutExisting([]string{"a.jpg"}, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty, got %v", result)
	}
}

// buildFakeBinary compiles a tiny Go program from src and returns its path.
func buildFakeBinary(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	name := rcloneBinaryName(runtime.GOOS, runtime.GOARCH)
	binary := filepath.Join(dir, name)
	srcFile := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(srcFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("go", "build", "-o", binary, srcFile).CombinedOutput(); err != nil {
		t.Fatalf("building fake binary: %v\n%s", err, out)
	}
	return binary
}

func TestDetectGlacierFiles(t *testing.T) {
	src := `package main
import "fmt"
func main() {
	fmt.Println("photo1.jpg;STANDARD")
	fmt.Println("photo2.jpg;DEEP_ARCHIVE")
	fmt.Println("video.mp4;GLACIER")
}
`
	binary := buildFakeBinary(t, src)
	glacier, err := detectGlacierFiles(context.Background(), binary, "/tmp/config.conf", "s3:bucket/prefix", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"photo2.jpg", "video.mp4"}
	if len(glacier) != len(expected) {
		t.Fatalf("got %d glacier files, want %d: %v", len(glacier), len(expected), glacier)
	}
	for i, want := range expected {
		if glacier[i] != want {
			t.Errorf("glacier[%d] = %q, want %q", i, glacier[i], want)
		}
	}
}

func TestDetectGlacierFiles_ListingError(t *testing.T) {
	src := `package main
import (
	"fmt"
	"os"
)
func main() {
	fmt.Fprintln(os.Stderr, "AccessDenied: bucket not accessible")
	os.Exit(1)
}
`
	binary := buildFakeBinary(t, src)
	_, err := detectGlacierFiles(context.Background(), binary, "/tmp/config.conf", "s3:bucket/prefix", nil)
	if err == nil {
		t.Fatal("expected error from failed listing")
	}
	if !strings.Contains(err.Error(), "AccessDenied") {
		t.Errorf("error should include stderr text, got: %v", err)
	}
}

func TestParseStorageClasses_SemicolonInKey(t *testing.T) {
	input := "albums/2024;trip.mov;GLACIER\nnormal.jpg;STANDARD\n"
	glacier := parseStorageClasses(input)

	expected := []string{"albums/2024;trip.mov"}
	if len(glacier) != len(expected) {
		t.Fatalf("got %d glacier files, want %d: %v", len(glacier), len(expected), glacier)
	}
	if glacier[0] != expected[0] {
		t.Errorf("glacier[0] = %q, want %q", glacier[0], expected[0])
	}
}

func TestDetectGlacierFiles_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := detectGlacierFiles(ctx, "/nonexistent", "/tmp/config.conf", "s3:bucket/prefix", nil)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestInitiateRestore(t *testing.T) {
	src := `package main
func main() {}
`
	binary := buildFakeBinary(t, src)
	log := logging.New(false, nil)
	err := initiateRestore(context.Background(), binary, "/tmp/config.conf", "s3:bucket/prefix", []string{"photo.jpg"}, log)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInitiateRestore_Failure(t *testing.T) {
	src := `package main
import "os"
func main() { os.Exit(1) }
`
	binary := buildFakeBinary(t, src)
	log := logging.New(false, nil)
	err := initiateRestore(context.Background(), binary, "/tmp/config.conf", "s3:bucket/prefix", []string{"photo.jpg"}, log)
	if err == nil {
		t.Fatal("expected error from failed restore")
	}
}
