package jpegmeta

import (
	"os"
	"testing"
	"time"
)

func TestReadDateNoExif(t *testing.T) {
	tmp := t.TempDir()
	path := tmp + "/test.jpg"
	if err := os.WriteFile(path, []byte{0xFF, 0xD8, 0xFF, 0xD9}, 0644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadDate(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.IsZero() {
		t.Errorf("expected zero time for JPEG without EXIF, got %v", got)
	}
}

func TestReadDateNonExistentFile(t *testing.T) {
	_, err := ReadDate("/nonexistent/file.jpg")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestReadDateWithExif(t *testing.T) {
	t.Skip("requires JPEG fixture with EXIF DateTimeOriginal")
	_ = time.Now()
}
