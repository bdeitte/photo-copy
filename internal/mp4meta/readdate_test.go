package mp4meta

import (
	"os"
	"testing"
)

func TestReadCreationTimeInvalidFile(t *testing.T) {
	tmp := t.TempDir()
	path := tmp + "/test.mp4"
	if err := os.WriteFile(path, []byte("not an mp4"), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadCreationTime(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.IsZero() {
		t.Errorf("expected zero time for invalid MP4, got %v", got)
	}
}

func TestReadCreationTimeNonExistent(t *testing.T) {
	_, err := ReadCreationTime("/nonexistent/file.mp4")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}
