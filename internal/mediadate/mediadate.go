// Package mediadate resolves the best available date from a media file's
// embedded metadata, falling back to file modification time.
package mediadate

import (
	"os"
	"strings"
	"time"

	"github.com/briandeitte/photo-copy/internal/jpegmeta"
	"github.com/briandeitte/photo-copy/internal/mp4meta"
)

// ResolveDate returns the best available date for a media file.
// For JPEGs, reads EXIF DateTimeOriginal. For MP4/MOV, reads creation time
// from the mvhd box. Falls back to file modification time.
// Returns zero time if the file cannot be read.
func ResolveDate(filePath string) time.Time {
	ext := strings.ToLower(filePath)

	switch {
	case strings.HasSuffix(ext, ".jpg") || strings.HasSuffix(ext, ".jpeg"):
		if t, err := jpegmeta.ReadDate(filePath); err == nil && !t.IsZero() {
			return t
		}
	case strings.HasSuffix(ext, ".mp4") || strings.HasSuffix(ext, ".mov"):
		if t, err := mp4meta.ReadCreationTime(filePath); err == nil && !t.IsZero() {
			return t
		}
	}

	info, err := os.Stat(filePath)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}
