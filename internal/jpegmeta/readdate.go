package jpegmeta

import (
	"os"
	"time"

	"github.com/rwcarlsen/goexif/exif"
)

// ReadDate reads the EXIF DateTimeOriginal from a JPEG file.
// Falls back to DateTime if DateTimeOriginal is not present.
// Returns zero time if the EXIF data is missing or unparseable.
// Returns an error only for file I/O failures.
func ReadDate(filePath string) (time.Time, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return time.Time{}, err
	}
	defer func() { _ = f.Close() }()

	x, err := exif.Decode(f)
	if err != nil {
		return time.Time{}, nil
	}

	// Try DateTimeOriginal first (camera capture date), then DateTime (last modified)
	tag, err := x.Get(exif.DateTimeOriginal)
	if err == nil {
		if sv, err := tag.StringVal(); err == nil {
			if t, err := time.Parse("2006:01:02 15:04:05", sv); err == nil {
				return t, nil
			}
		}
	}

	// Fallback to DateTime
	t, err := x.DateTime()
	if err != nil {
		return time.Time{}, nil
	}

	return t, nil
}
