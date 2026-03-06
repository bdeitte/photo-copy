package flickr

import (
	"path/filepath"
	"testing"
)

func TestLoadTransferLog_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	log, err := loadTransferLog(filepath.Join(tmpDir, "transfer.log"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(log) != 0 {
		t.Fatalf("expected empty log, got %d entries", len(log))
	}
}

func TestTransferLog_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "transfer.log")

	if err := appendTransferLog(logPath, "photo1.jpg"); err != nil {
		t.Fatalf("append failed: %v", err)
	}
	if err := appendTransferLog(logPath, "photo2.jpg"); err != nil {
		t.Fatalf("append failed: %v", err)
	}

	log, err := loadTransferLog(logPath)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if !log["photo1.jpg"] || !log["photo2.jpg"] {
		t.Fatalf("expected both photos in log, got: %v", log)
	}
}

func TestBuildAPIURL(t *testing.T) {
	url := buildAPIURL("flickr.people.getPhotos", "testkey", map[string]string{
		"user_id": "me",
		"page":    "1",
	})

	if url == "" {
		t.Fatal("expected non-empty URL")
	}
	if !containsSubstr(url, "flickr.people.getPhotos") {
		t.Fatalf("URL missing method: %s", url)
	}
	if !containsSubstr(url, "testkey") {
		t.Fatalf("URL missing API key: %s", url)
	}
}

func containsSubstr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
