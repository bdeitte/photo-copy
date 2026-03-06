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
