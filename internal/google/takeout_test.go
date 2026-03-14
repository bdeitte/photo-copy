package google

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func createTestZip(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	zipPath := filepath.Join(dir, "takeout.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for name, content := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = fw.Write([]byte(content))
	}
	_ = w.Close()
	_ = f.Close()
	return zipPath
}

func TestImportTakeout_ExtractsMediaOnly(t *testing.T) {
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()

	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/Trip/photo1.jpg":                "jpegdata",
		"Google Photos/Trip/photo1.jpg.json":           `{"title":"photo1"}`,
		"Google Photos/Trip/video.mp4":                 "mp4data",
		"Google Photos/Trip/metadata.json":             `{"albums":[]}`,
		"Google Photos/Trip/print-subscriptions.json":  `{}`,
	})

	result, err := ImportTakeout(takeoutDir, outputDir, nil)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	if result.Succeeded != 2 {
		t.Fatalf("expected 2 files extracted, got %d", result.Succeeded)
	}

	if _, err := os.Stat(filepath.Join(outputDir, "photo1.jpg")); err != nil {
		t.Fatal("photo1.jpg not found in output")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "video.mp4")); err != nil {
		t.Fatal("video.mp4 not found in output")
	}
}

func TestImportTakeout_SkipsNonMedia(t *testing.T) {
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()

	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/readme.html": "<html>hi</html>",
		"Google Photos/data.json":   "{}",
	})

	result, err := ImportTakeout(takeoutDir, outputDir, nil)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	if result.Succeeded != 0 {
		t.Fatalf("expected 0 files extracted, got %d", result.Succeeded)
	}
}

func TestImportTakeout_MultipleZips(t *testing.T) {
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()

	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/a.jpg": "data1",
	})
	_ = os.Rename(filepath.Join(takeoutDir, "takeout.zip"), filepath.Join(takeoutDir, "takeout-001.zip"))

	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/b.png": "data2",
	})
	_ = os.Rename(filepath.Join(takeoutDir, "takeout.zip"), filepath.Join(takeoutDir, "takeout-002.zip"))

	result, err := ImportTakeout(takeoutDir, outputDir, nil)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	if result.Succeeded != 2 {
		t.Fatalf("expected 2 files, got %d", result.Succeeded)
	}
}
