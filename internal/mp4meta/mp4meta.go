// Package mp4meta provides utilities for editing MP4/MOV container metadata.
package mp4meta

import (
	"fmt"
	"os"
	"time"

	gomp4 "github.com/abema/go-mp4"
)

// mp4Epoch is the offset in seconds between Unix epoch (1970-01-01) and
// the MP4/QuickTime epoch (1904-01-01).
const mp4Epoch = 2082844800

// SetCreationTime sets the creation and modification timestamps in the
// mvhd, tkhd, and mdhd boxes of an MP4 or MOV file. It writes to a temp
// file and renames over the original on success.
func SetCreationTime(filePath string, t time.Time) error {
	mp4Time := uint64(t.Unix()) + mp4Epoch

	in, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("opening %s: %w", filePath, err)
	}
	defer func() { _ = in.Close() }()

	tmpPath := filePath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}

	w := gomp4.NewWriter(out)

	_, err = gomp4.ReadBoxStructure(in, func(h *gomp4.ReadHandle) (any, error) {
		switch h.BoxInfo.Type {
		case gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia():
			_, err := w.StartBox(&h.BoxInfo)
			if err != nil {
				return nil, err
			}
			val, err := h.Expand()
			if err != nil {
				return nil, err
			}
			_, err = w.EndBox()
			return val, err

		case gomp4.BoxTypeMvhd():
			return nil, rewriteMvhd(h, w, mp4Time)

		case gomp4.BoxTypeTkhd():
			return nil, rewriteTkhd(h, w, mp4Time)

		case gomp4.BoxTypeMdhd():
			return nil, rewriteMdhd(h, w, mp4Time)

		default:
			return nil, w.CopyBox(in, &h.BoxInfo)
		}
	})

	closeErr := out.Close()
	_ = in.Close()

	if err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("processing MP4: %w", err)
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", closeErr)
	}

	if renameErr := os.Rename(tmpPath, filePath); renameErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("renaming temp file: %w", renameErr)
	}
	return nil
}

func rewriteMvhd(h *gomp4.ReadHandle, w *gomp4.Writer, mp4Time uint64) error {
	box, _, err := h.ReadPayload()
	if err != nil {
		return err
	}
	mvhd := box.(*gomp4.Mvhd)

	if mvhd.GetVersion() == 0 {
		mvhd.CreationTimeV0 = uint32(mp4Time)
		mvhd.ModificationTimeV0 = uint32(mp4Time)
	} else {
		mvhd.CreationTimeV1 = mp4Time
		mvhd.ModificationTimeV1 = mp4Time
	}

	bi, err := w.StartBox(&h.BoxInfo)
	if err != nil {
		return err
	}
	if _, err := gomp4.Marshal(w, mvhd, bi.Context); err != nil {
		return err
	}
	_, err = w.EndBox()
	return err
}

func rewriteTkhd(h *gomp4.ReadHandle, w *gomp4.Writer, mp4Time uint64) error {
	box, _, err := h.ReadPayload()
	if err != nil {
		return err
	}
	tkhd := box.(*gomp4.Tkhd)

	if tkhd.GetVersion() == 0 {
		tkhd.CreationTimeV0 = uint32(mp4Time)
		tkhd.ModificationTimeV0 = uint32(mp4Time)
	} else {
		tkhd.CreationTimeV1 = mp4Time
		tkhd.ModificationTimeV1 = mp4Time
	}

	bi, err := w.StartBox(&h.BoxInfo)
	if err != nil {
		return err
	}
	if _, err := gomp4.Marshal(w, tkhd, bi.Context); err != nil {
		return err
	}
	_, err = w.EndBox()
	return err
}

func rewriteMdhd(h *gomp4.ReadHandle, w *gomp4.Writer, mp4Time uint64) error {
	box, _, err := h.ReadPayload()
	if err != nil {
		return err
	}
	mdhd := box.(*gomp4.Mdhd)

	if mdhd.GetVersion() == 0 {
		mdhd.CreationTimeV0 = uint32(mp4Time)
		mdhd.ModificationTimeV0 = uint32(mp4Time)
	} else {
		mdhd.CreationTimeV1 = mp4Time
		mdhd.ModificationTimeV1 = mp4Time
	}

	bi, err := w.StartBox(&h.BoxInfo)
	if err != nil {
		return err
	}
	if _, err := gomp4.Marshal(w, mdhd, bi.Context); err != nil {
		return err
	}
	_, err = w.EndBox()
	return err
}
