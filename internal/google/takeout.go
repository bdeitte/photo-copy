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
	"github.com/schollz/progressbar/v3"
)

func ImportTakeout(takeoutDir, outputDir string, log *logging.Logger) (int, error) {
	if log == nil {
		log = logging.New(false, nil)
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return 0, fmt.Errorf("creating output dir: %w", err)
	}

	entries, err := os.ReadDir(takeoutDir)
	if err != nil {
		return 0, fmt.Errorf("reading takeout dir: %w", err)
	}

	var zipFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".zip") {
			zipFiles = append(zipFiles, filepath.Join(takeoutDir, e.Name()))
		}
	}

	if len(zipFiles) == 0 {
		return 0, fmt.Errorf("no zip files found in %s", takeoutDir)
	}

	log.Debug("found %d zip files", len(zipFiles))

	totalExtracted := 0

	for _, zipPath := range zipFiles {
		log.Debug("processing %s", zipPath)

		count, err := extractMediaFromZip(zipPath, outputDir, log)
		if err != nil {
			log.Error("processing %s: %v", zipPath, err)
			continue
		}

		totalExtracted += count
	}

	fmt.Fprintf(os.Stderr, "Extracted %d media files\n", totalExtracted)
	return totalExtracted, nil
}

func extractMediaFromZip(zipPath, outputDir string, log *logging.Logger) (int, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return 0, fmt.Errorf("opening zip: %w", err)
	}
	defer r.Close()

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
		return 0, nil
	}

	bar := progressbar.Default(int64(len(mediaFiles)), filepath.Base(zipPath))
	extracted := 0

	for _, f := range mediaFiles {
		name := filepath.Base(f.Name)
		destPath := filepath.Join(outputDir, name)

		if _, err := os.Stat(destPath); err == nil {
			base := strings.TrimSuffix(name, filepath.Ext(name))
			ext := filepath.Ext(name)
			for i := 1; ; i++ {
				destPath = filepath.Join(outputDir, fmt.Sprintf("%s_%d%s", base, i, ext))
				if _, err := os.Stat(destPath); os.IsNotExist(err) {
					break
				}
			}
			log.Debug("duplicate filename %s, saving as %s", name, filepath.Base(destPath))
		}

		log.Debug("extracting %s -> %s", f.Name, destPath)

		if err := extractFile(f, destPath); err != nil {
			log.Error("extracting %s: %v", f.Name, err)
			bar.Add(1)
			continue
		}

		extracted++
		bar.Add(1)
	}

	return extracted, nil
}

func extractFile(f *zip.File, destPath string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, rc)
	return err
}
