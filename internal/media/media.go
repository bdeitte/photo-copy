package media

import (
	"path/filepath"
	"strings"
)

var supportedExtensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".tiff": true,
	".tif":  true,
	".gif":  true,
	".heic": true,
	".webp": true,
	".mp4":  true,
	".mov":  true,
	".avi":  true,
	".mkv":  true,
}

func IsSupportedFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return supportedExtensions[ext]
}
