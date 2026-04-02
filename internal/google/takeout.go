package google

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/briandeitte/photo-copy/internal/logging"
	"github.com/briandeitte/photo-copy/internal/media"
	"github.com/briandeitte/photo-copy/internal/transfer"
	"github.com/schollz/progressbar/v3"
)

// ImportTakeout extracts media files from Google Takeout zip archives in takeoutDir
// into outputDir, skipping non-media files and JSON metadata.
func ImportTakeout(ctx context.Context, takeoutDir, outputDir string, log *logging.Logger) (*transfer.Result, error) {
	return importTakeout(ctx, takeoutDir, outputDir, log, nil)
}

// importTakeout is the internal implementation of ImportTakeout.
// afterExtract, if non-nil, is called after each successful file extraction.
func importTakeout(ctx context.Context, takeoutDir, outputDir string, log *logging.Logger, afterExtract func()) (*transfer.Result, error) {
	if log == nil {
		log = logging.New(false, nil)
	}

	result := transfer.NewResult("google-takeout", "import", outputDir)

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return result, fmt.Errorf("creating output dir: %w", err)
	}

	entries, err := os.ReadDir(takeoutDir)
	if err != nil {
		return result, fmt.Errorf("reading takeout dir: %w", err)
	}

	var zipFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".zip") {
			zipFiles = append(zipFiles, filepath.Join(takeoutDir, e.Name()))
		}
	}

	if len(zipFiles) == 0 {
		return result, fmt.Errorf("no zip files found in %s", takeoutDir)
	}

	log.Debug("found %d zip files", len(zipFiles))

	for _, zipPath := range zipFiles {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		log.Debug("processing %s", zipPath)

		if err := extractMediaFromZip(ctx, zipPath, outputDir, log, result, afterExtract); err != nil {
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			log.Error("processing %s: %v", zipPath, err)
			continue
		}
	}

	result.Finish()
	return result, nil
}

// extractMediaFromZip extracts supported media files from a zip archive.
// afterExtract, if non-nil, is called after each successful extraction (used by tests).
func extractMediaFromZip(ctx context.Context, zipPath, outputDir string, log *logging.Logger, result *transfer.Result, afterExtract func()) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("opening zip: %w", err)
	}
	defer func() { _ = r.Close() }()

	var mediaFiles []*zip.File
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := filepath.Base(f.Name)
		if media.IsSupportedFile(name) {
			mediaFiles = append(mediaFiles, f)
		} else {
			log.Debug("skipping non-media: %s", f.Name)
		}
	}

	if len(mediaFiles) == 0 {
		return nil
	}

	bar := progressbar.Default(int64(len(mediaFiles)), filepath.Base(zipPath))

	for _, f := range mediaFiles {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := filepath.Base(f.Name)
		destPath := filepath.Join(outputDir, name)

		if _, err := os.Stat(destPath); err == nil {
			base := strings.TrimSuffix(name, filepath.Ext(name))
			ext := filepath.Ext(name)
			for i := 1; ; i++ {
				destPath = filepath.Join(outputDir, fmt.Sprintf("%s_%d%s", base, i, ext))
				if _, err := os.Stat(destPath); errors.Is(err, os.ErrNotExist) {
					break
				} else if err != nil {
					return fmt.Errorf("checking destination %s: %w", destPath, err)
				}
			}
			log.Debug("duplicate filename %s, saving as %s", name, filepath.Base(destPath))
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("checking destination %s: %w", destPath, err)
		}

		log.Debug("extracting %s -> %s", f.Name, destPath)

		if err := extractFile(ctx, f, destPath); err != nil {
			log.Error("extracting %s: %v", f.Name, err)
			result.RecordError(name, err.Error())
			_ = bar.Add(1)
			continue
		}

		info, statErr := os.Stat(destPath)
		if statErr == nil {
			result.RecordSuccess(info.Size())
		} else {
			result.RecordSuccess(0)
		}
		_ = bar.Add(1)

		if afterExtract != nil {
			afterExtract()
		}
	}

	return nil
}

func extractFile(ctx context.Context, f *zip.File, destPath string) (err error) {
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("opening zip entry: %w", err)
	}
	defer func() { _ = rc.Close() }()

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("creating %s: %w", destPath, err)
	}
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	const maxFileSize = 10 << 30 // 10 GB
	reader := io.LimitReader(rc, maxFileSize+1)
	written, err := copyWithContext(ctx, out, reader)
	if err != nil {
		_ = out.Close()
		_ = os.Remove(destPath)
		return err
	}
	if written > maxFileSize {
		_ = out.Close()
		_ = os.Remove(destPath)
		return fmt.Errorf("zip entry exceeds %d byte limit: %s", maxFileSize, f.Name)
	}
	return nil
}

// copyWithContext copies from src to dst, checking for context cancellation
// between chunks to allow prompt cancellation of large file copies.
func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 32*1024)
	var written int64
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				return written, ew
			}
		}
		if er != nil {
			if er == io.EOF {
				return written, nil
			}
			return written, er
		}
	}
}
