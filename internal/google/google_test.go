package google

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadUploadLog_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	log, err := loadUploadLog(filepath.Join(tmpDir, "upload.log"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(log) != 0 {
		t.Fatalf("expected empty log, got %d entries", len(log))
	}
}

func TestUploadLog_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "upload.log")

	if err := appendUploadLog(logPath, "photo1.jpg"); err != nil {
		t.Fatalf("append failed: %v", err)
	}
	if err := appendUploadLog(logPath, "photo2.jpg"); err != nil {
		t.Fatalf("append failed: %v", err)
	}

	log, err := loadUploadLog(logPath)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if !log["photo1.jpg"] || !log["photo2.jpg"] {
		t.Fatalf("expected both photos in log, got: %v", log)
	}
}

func TestCollectMediaFiles(t *testing.T) {
	tmpDir := t.TempDir()

	os.WriteFile(filepath.Join(tmpDir, "photo.jpg"), []byte("fake"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "video.mp4"), []byte("fake"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("fake"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "metadata.json"), []byte("fake"), 0644)

	files, err := collectMediaFiles(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("expected 2 media files, got %d: %v", len(files), files)
	}
}
