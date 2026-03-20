package media

import "testing"

func TestIsSupportedFile(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"photo.jpg", true},
		{"photo.JPEG", true},
		{"video.mp4", true},
		{"video.MOV", true},
		{"photo.heic", true},
		{"photo.webp", true},
		{"photo.png", true},
		{"photo.tiff", true},
		{"photo.gif", true},
		{"video.avi", true},
		{"video.mkv", true},
		{"readme.txt", false},
		{"metadata.json", false},
		{"photo.jpg.bak", false},
		{".DS_Store", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSupportedFile(tt.name); got != tt.want {
				t.Errorf("IsSupportedFile(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestIsSupportedFile_CaseSensitivity(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"photo.JPG", true},
		{"video.Mp4", true},
		{"photo.HEIC", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSupportedFile(tt.name); got != tt.want {
				t.Errorf("IsSupportedFile(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestSupportedExtensions_Complete(t *testing.T) {
	exts := SupportedExtensions()

	expected := []string{".jpg", ".jpeg", ".png", ".tiff", ".tif", ".gif", ".heic", ".webp", ".mp4", ".mov", ".avi", ".mkv"}
	extSet := make(map[string]bool)
	for _, ext := range exts {
		extSet[ext] = true
	}
	for _, ext := range expected {
		if !extSet[ext] {
			t.Errorf("SupportedExtensions() missing %q", ext)
		}
	}
	if len(exts) != len(expected) {
		t.Errorf("SupportedExtensions() returned %d extensions, want %d; got %v", len(exts), len(expected), exts)
	}
}

func TestIsSupportedFile_NoExtension(t *testing.T) {
	if got := IsSupportedFile("README"); got {
		t.Errorf("IsSupportedFile(%q) = true, want false", "README")
	}
}

func TestIsSupportedFile_DotOnly(t *testing.T) {
	if got := IsSupportedFile("."); got {
		t.Errorf("IsSupportedFile(%q) = true, want false", ".")
	}
}
