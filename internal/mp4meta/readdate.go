package mp4meta

import (
	"os"
	"time"

	gomp4 "github.com/abema/go-mp4"
)

// ReadCreationTime reads the creation time from an MP4/MOV file's mvhd box.
// Returns zero time if the file is not a valid MP4 or has no creation time.
// Returns an error only for file I/O failures.
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
		// Not a valid MP4 — return zero time
		return time.Time{}, nil
	}

	if !found || creationTime == 0 {
		return time.Time{}, nil
	}

	// Reuse the existing mp4Epoch constant (seconds offset from Unix epoch)
	return time.Unix(int64(creationTime)-mp4Epoch, 0), nil
}
