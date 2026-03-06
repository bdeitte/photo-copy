package s3

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRcloneBinaryName(t *testing.T) {
	tests := []struct {
		goos, goarch string
		want         string
	}{
		{"linux", "amd64", "rclone-linux-amd64"},
		{"linux", "arm64", "rclone-linux-arm64"},
		{"darwin", "amd64", "rclone-darwin-amd64"},
		{"darwin", "arm64", "rclone-darwin-arm64"},
		{"windows", "amd64", "rclone-windows-amd64.exe"},
		{"windows", "arm64", "rclone-windows-arm64.exe"},
	}

	for _, tt := range tests {
		t.Run(tt.goos+"-"+tt.goarch, func(t *testing.T) {
			got := rcloneBinaryName(tt.goos, tt.goarch)
			if got != tt.want {
				t.Errorf("rcloneBinaryName(%q, %q) = %q, want %q", tt.goos, tt.goarch, got, tt.want)
			}
		})
	}
}

func TestFindRcloneBinary(t *testing.T) {
	tmpDir := t.TempDir()
	binDir := filepath.Join(tmpDir, "rclone-bin")
	os.MkdirAll(binDir, 0755)

	name := rcloneBinaryName(runtime.GOOS, runtime.GOARCH)
	fakeBin := filepath.Join(binDir, name)
	os.WriteFile(fakeBin, []byte("fake"), 0755)

	got, err := findRcloneBinary(binDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != fakeBin {
		t.Errorf("got %q, want %q", got, fakeBin)
	}
}

func TestFindRcloneBinary_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := findRcloneBinary(tmpDir)
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
}

func TestWriteRcloneConfig(t *testing.T) {
	tmpFile, err := writeRcloneConfig("AKID", "SECRET", "us-west-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer os.Remove(tmpFile)

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "type = s3") {
		t.Fatal("missing type = s3")
	}
	if !strings.Contains(content, "access_key_id = AKID") {
		t.Fatal("missing access_key_id")
	}
	if !strings.Contains(content, "region = us-west-2") {
		t.Fatal("missing region")
	}
}
