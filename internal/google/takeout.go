package google

import (
	"archive/zip"
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

func ImportTakeout(takeoutDir, outputDir string, log *logging.Logger) (*transfer.Result, error) {
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
		log.Debug("processing %s", zipPath)

		if err := extractMediaFromZip(zipPath, outputDir, log, result); err != nil {
			log.Error("processing %s: %v", zipPath, err)
			continue
		}
	}

	result.Finish()
	return result, nil
}

func extractMediaFromZip(zipPath, outputDir string, log *logging.Logger, result *transfer.Result) error {
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
		name := filepath.Base(f.Name)
		destPath := filepath.Join(outputDir, name)

		if _, err := os.Stat(destPath); err == nil {
			base := strings.TrimSuffix(name, filepath.Ext(name))
			ext := filepath.Ext(name)
			for i := 1; ; i++ {
				destPath = filepath.Join(outputDir, fmt.Sprintf("%s_%d%s", base, i, ext))
				if _, err := os.Stat(destPath); err != nil {
					break
				}
			}
			log.Debug("duplicate filename %s, saving as %s", name, filepath.Base(destPath))
		}

		log.Debug("extracting %s -> %s", f.Name, destPath)

		if err := extractFile(f, destPath); err != nil {
			log.Error("extracting %s: %v", f.Name, err)
			result.RecordError(name, err.Error())
			_ = bar.Add(1)
			continue
		}

		info, statErr := os.Stat(destPath)
		if statErr == nil {
			result.RecordSuccess(name, info.Size())
		} else {
			result.RecordSuccess(name, 0)
		}
		_ = bar.Add(1)
	}

	return nil
}

func extractFile(f *zip.File, destPath string) (err error) {
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
	_, err = io.Copy(out, io.LimitReader(rc, maxFileSize))
	return err
}
