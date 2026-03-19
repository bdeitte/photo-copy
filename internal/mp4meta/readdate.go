package mp4meta

import (
	"errors"
	"io/fs"
	"os"
	"time"

	gomp4 "github.com/abema/go-mp4"
)

// isIOError reports whether err is an I/O or filesystem error (as opposed
// to an MP4 parse/format error).
func isIOError(err error) bool {
	var pathErr *fs.PathError
	return errors.As(err, &pathErr)
}

// ReadCreationTime reads the creation time from an MP4/MOV file's mvhd box.
// Returns zero time if the file is not a valid MP4, has no creation time, or
// the creation time represents a date before the Unix epoch (1970).
// Returns an error for file I/O failures; MP4 parse errors return zero time.
func ReadCreationTime(filePath string) (time.Time, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return time.Time{}, err
	}
	defer func() { _ = f.Close() }()

	var creationTime uint64
	found := false

	_, err = gomp4.ReadBoxStructure(f, func(h *gomp4.ReadHandle) (interface{}, error) {
		switch h.BoxInfo.Type {
		case gomp4.BoxTypeMoov():
			// Must expand container boxes to reach mvhd inside
			_, err := h.Expand()
			return nil, err

		case gomp4.BoxTypeMvhd():
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			if mvhd, ok := box.(*gomp4.Mvhd); ok {
				if mvhd.GetVersion() == 0 {
					creationTime = uint64(mvhd.CreationTimeV0)
				} else {
					creationTime = mvhd.CreationTimeV1
				}
				found = true
			}
			return nil, nil

		default:
			return nil, nil
		}
	})
	if err != nil {
		// Distinguish I/O errors from parse errors
		if isIOError(err) {
			return time.Time{}, err
		}
		// MP4 parse/format error — not a valid MP4
		return time.Time{}, nil
	}

	if !found || creationTime == 0 {
		return time.Time{}, nil
	}

	// Guard against pre-Unix-epoch dates (creation time < MP4 epoch offset)
	if int64(creationTime) < mp4Epoch {
		return time.Time{}, nil
	}

	// Reuse the existing mp4Epoch constant (seconds offset from Unix epoch)
	return time.Unix(int64(creationTime)-mp4Epoch, 0), nil
}
