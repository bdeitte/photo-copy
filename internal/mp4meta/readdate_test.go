package mp4meta

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestReadCreationTime_V0(t *testing.T) {
	dir := t.TempDir()
	mp4Path := filepath.Join(dir, "test.mp4")
	writeMinimalMP4(t, mp4Path, 0)

	targetTime := time.Date(2020, 6, 15, 14, 30, 0, 0, time.UTC)
	if err := SetCreationTime(mp4Path, targetTime); err != nil {
		t.Fatalf("SetCreationTime failed: %v", err)
	}

	got, err := ReadCreationTime(mp4Path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Equal(targetTime) {
		t.Errorf("ReadCreationTime() = %v, want %v", got, targetTime)
	}
}

func TestReadCreationTime_V1(t *testing.T) {
	dir := t.TempDir()
	mp4Path := filepath.Join(dir, "test.mp4")
	writeMinimalMP4(t, mp4Path, 1)

	targetTime := time.Date(2020, 6, 15, 14, 30, 0, 0, time.UTC)
	if err := SetCreationTime(mp4Path, targetTime); err != nil {
		t.Fatalf("SetCreationTime failed: %v", err)
	}

	got, err := ReadCreationTime(mp4Path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Equal(targetTime) {
		t.Errorf("ReadCreationTime() = %v, want %v", got, targetTime)
	}
}

func TestReadCreationTime_ZeroTimestamp(t *testing.T) {
	dir := t.TempDir()
	mp4Path := filepath.Join(dir, "test.mp4")
	writeMinimalMP4(t, mp4Path, 0)
	// Don't set creation time — it should be zero from writeMinimalMP4

	got, err := ReadCreationTime(mp4Path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.IsZero() {
		t.Errorf("expected zero time for MP4 with zero creation time, got %v", got)
	}
}
