package mediadate

import (
	"os"
	"testing"
	"time"
)

func TestResolveDate_FallbackToModTime(t *testing.T) {
	tmp := t.TempDir()
	path := tmp + "/test.png"
	if err := os.WriteFile(path, []byte("fake png"), 0644); err != nil {
		t.Fatal(err)
	}
	want := time.Date(2022, 6, 15, 12, 0, 0, 0, time.Local)
	if err := os.Chtimes(path, want, want); err != nil {
		t.Fatal(err)
	}

	got := ResolveDate(path)
	if got.IsZero() {
		t.Fatal("expected non-zero time")
	}
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestResolveDate_NonExistentFile(t *testing.T) {
	got := ResolveDate("/nonexistent/file.jpg")
	if !got.IsZero() {
		t.Errorf("expected zero time for nonexistent file, got %v", got)
	}
}
