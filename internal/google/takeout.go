package google

import (
	"archive/zip"
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/briandeitte/photo-copy/internal/jpegmeta"
	"github.com/briandeitte/photo-copy/internal/logging"
	"github.com/briandeitte/photo-copy/internal/mp4meta"
	"github.com/briandeitte/photo-copy/internal/transfer"
	"github.com/briandeitte/photo-copy/internal/xmp"
)

const importLogFile = ".photo-copy-import.log"

// importOption configures optional ImportTakeout behavior.
type importOption func(*importConfig)

type importConfig struct {
	afterExtract  func() // called after each successful file extraction; nil in production
	beforeExtract func() // called before each extractFile call; nil in production
}

// withAfterExtract returns an option that calls fn after each successful extraction.
// Used by tests for deterministic cancellation.
func withAfterExtract(fn func()) importOption {
	return func(cfg *importConfig) { cfg.afterExtract = fn }
}

// withBeforeExtract returns an option that calls fn before each extractFile call,
// after the top-of-loop context check. Used by tests to cancel mid-extraction.
func withBeforeExtract(fn func()) importOption {
	return func(cfg *importConfig) { cfg.beforeExtract = fn }
}

// ImportTakeout extracts media files from Google Takeout zip archives in takeoutDir
// into outputDir. Album folders are preserved as subdirectories, year folders are
// flattened, and duplicate year-folder entries are skipped. If noMetadata is false,
// JSON sidecar metadata is embedded into extracted files.
func ImportTakeout(ctx context.Context, takeoutDir, outputDir string, log *logging.Logger, noMetadata bool, opts ...importOption) (*transfer.Result, error) {
	var cfg importConfig
	for _, o := range opts {
		o(&cfg)
	}
	if log == nil {
		log = logging.New(false, nil)
	}

	result := transfer.NewResult("google-takeout", "import", outputDir)

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return result, fmt.Errorf("creating output dir: %w", err)
	}

	logPath := filepath.Join(outputDir, importLogFile)
	imported, err := loadImportLog(logPath)
	if err != nil {
		return result, fmt.Errorf("reading import log: %w", err)
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

	// Phase 1: scan all zips to build the index.
	index, err := scanZips(ctx, zipFiles)
	if err != nil {
		return result, fmt.Errorf("scanning takeout zips: %w", err)
	}

	// Log skipped duplicates and count non-skipped entries.
	var toExtract []*mediaEntry
	for _, me := range index.media {
		if me.skip {
			log.Debug("dedup: skipping year-folder %s/%s (exists in album)", me.folderName, me.basename)
			continue
		}
		toExtract = append(toExtract, me)
	}

	result.Expected = len(toExtract)

	if len(toExtract) == 0 {
		result.Finish()
		return result, nil
	}

	log.Info("found %d media files in Google Takeout", result.Expected)

	// Group media entries by zip file to open each zip only once.
	type zipGroup struct {
		zipPath string
		entries []*mediaEntry
	}
	groupOrder := make(map[string]int) // zipPath -> index in groups
	var groups []zipGroup
	for _, me := range toExtract {
		idx, ok := groupOrder[me.zipPath]
		if !ok {
			idx = len(groups)
			groupOrder[me.zipPath] = idx
			groups = append(groups, zipGroup{zipPath: me.zipPath})
		}
		groups[idx].entries = append(groups[idx].entries, me)
	}

	estimator := transfer.NewEstimator()

	// Phase 2: extract entries, one zip at a time.
	for _, g := range groups {
		if err := ctx.Err(); err != nil {
			return result, err
		}

		r, err := zip.OpenReader(g.zipPath)
		if err != nil {
			log.Error("opening zip %s: %v", g.zipPath, err)
			continue
		}

		// Build lookup from entry name to *zip.File.
		zipLookup := make(map[string]*zip.File, len(r.File))
		for _, f := range r.File {
			zipLookup[f.Name] = f
		}

		// Read JSON sidecars needed by this group's entries. Sidecars may be
		// in the current zip or in a different zip part. We only load what's
		// needed for this group, so memory usage stays proportional to one
		// zip's worth of entries rather than the entire export.
		var jsonData map[string][]byte
		if !noMetadata {
			jsonData = readGroupSidecars(log, g.entries, g.zipPath, zipLookup)
		}

		zipSkipped := 0
		for _, me := range g.entries {
			if err := ctx.Err(); err != nil {
				_ = r.Close()
				return result, err
			}

			f, ok := zipLookup[me.entryName]
			if !ok {
				result.RecordError(me.basename, "entry not found in zip")
				estimator.Tick()
				processed := result.Succeeded + result.Skipped + result.Failed
				log.Error("[%d/%d] %sentry %s not found in zip %s", processed, result.Expected, estimator.Estimate(result.Expected-processed), me.entryName, g.zipPath)
				continue
			}

			// Determine destination path: album -> subdirectory, year -> flat.
			// relPath is the source-key form used for the import log so that
			// collision-renamed files still skip on rerun. displayPath reflects
			// the actual on-disk name and is used for user-facing logs.
			var relPath string
			if me.isYearFolder || me.folderName == "" {
				relPath = me.basename
			} else {
				relPath = filepath.Join(me.folderName, me.basename)
			}
			displayPath := relPath

			// Skip files already recorded in the import log.
			if imported[relPath] {
				log.Debug("skipping %s (already imported)", relPath)
				result.RecordSkip(1)
				zipSkipped++
				continue
			}

			var destPath string
			if me.isYearFolder || me.folderName == "" {
				destPath = filepath.Join(outputDir, me.basename)
			} else {
				destDir := filepath.Join(outputDir, me.folderName)
				if err := os.MkdirAll(destDir, 0755); err != nil {
					result.RecordError(me.basename, err.Error())
					estimator.Tick()
					processed := result.Succeeded + result.Skipped + result.Failed
					log.Error("[%d/%d] %screating album dir %s: %v", processed, result.Expected, estimator.Estimate(result.Expected-processed), destDir, err)
					continue
				}
				destPath = filepath.Join(destDir, me.basename)
			}

			// Handle filename collisions.
			if _, err := os.Stat(destPath); err == nil {
				base := strings.TrimSuffix(me.basename, filepath.Ext(me.basename))
				ext := filepath.Ext(me.basename)
				dir := filepath.Dir(destPath)
				collisionErr := false
				for i := 1; ; i++ {
					destPath = filepath.Join(dir, fmt.Sprintf("%s_%d%s", base, i, ext))
					if _, serr := os.Stat(destPath); errors.Is(serr, os.ErrNotExist) {
						break
					} else if serr != nil {
						result.RecordError(me.basename, fmt.Sprintf("checking destination: %v", serr))
						estimator.Tick()
						processed := result.Succeeded + result.Skipped + result.Failed
						log.Error("[%d/%d] %schecking destination %s: %v", processed, result.Expected, estimator.Estimate(result.Expected-processed), destPath, serr)
						collisionErr = true
						break
					}
				}
				if collisionErr {
					continue
				}
				// Update displayPath (but not relPath) to reflect the
				// collision-renamed filename. relPath stays on the source
				// basename so reruns can skip via the import log.
				if me.isYearFolder || me.folderName == "" {
					displayPath = filepath.Base(destPath)
				} else {
					displayPath = filepath.Join(me.folderName, filepath.Base(destPath))
				}
				log.Debug("duplicate filename %s, saving as %s", me.basename, filepath.Base(destPath))
			} else if !errors.Is(err, os.ErrNotExist) {
				result.RecordError(me.basename, err.Error())
				estimator.Tick()
				processed := result.Succeeded + result.Skipped + result.Failed
				log.Error("[%d/%d] %schecking destination %s: %v", processed, result.Expected, estimator.Estimate(result.Expected-processed), destPath, err)
				continue
			}

			log.Debug("extracting %s -> %s", me.entryName, destPath)

			if cfg.beforeExtract != nil {
				cfg.beforeExtract()
			}
			if err := extractFile(ctx, f, destPath); err != nil {
				if ctx.Err() != nil {
					_ = r.Close()
					return result, ctx.Err()
				}
				result.RecordError(me.basename, err.Error())
				estimator.Tick()
				processed := result.Succeeded + result.Skipped + result.Failed
				log.Error("[%d/%d] %sextracting %s: %v", processed, result.Expected, estimator.Estimate(result.Expected-processed), me.entryName, err)
				continue
			}

			info, statErr := os.Stat(destPath)
			if statErr == nil {
				result.RecordSuccess(info.Size())
			} else {
				result.RecordSuccess(0)
			}

			var photoDate time.Time
			if !noMetadata {
				var jd []byte
				if me.jsonEntry != nil {
					jd = jsonData[me.jsonEntry.entryName]
				}
				photoDate = applyTakeoutMetadata(log, me, destPath, jd)
			}

			if err := appendImportLog(logPath, relPath); err != nil {
				log.Error("writing import log for %s: %v", relPath, err)
			}

			estimator.Tick()
			processed := result.Succeeded + result.Skipped + result.Failed
			detail := ""
			if !photoDate.IsZero() {
				detail = fmt.Sprintf(" (%s)", photoDate.Format("2006-01-02"))
			}
			log.Info("[%d/%d] %sextracted %s%s", processed, result.Expected, estimator.Estimate(result.Expected-processed), displayPath, detail)

			if cfg.afterExtract != nil {
				cfg.afterExtract()
			}
		}

		if zipSkipped > 0 {
			processed := result.Succeeded + result.Skipped + result.Failed
			log.Info("[%d/%d] skipped %d already-imported files in %s", processed, result.Expected, zipSkipped, filepath.Base(g.zipPath))
		}

		_ = r.Close()
	}

	result.Finish()
	return result, nil
}

// applyTakeoutMetadata embeds metadata (title, description, creation date) into
// the extracted file using the pre-read JSON sidecar data. If jsonData is nil,
// the file has no matched sidecar and metadata is skipped. Returns the photo
// date used (or zero time if none was resolved) so callers can reuse it for
// logging.
func applyTakeoutMetadata(log *logging.Logger, entry *mediaEntry, destPath string, jsonData []byte) time.Time {
	if jsonData == nil {
		log.Debug("no JSON sidecar for %s, skipping metadata", entry.basename)
		return time.Time{}
	}

	meta, err := parseTakeoutJSON(jsonData)
	if err != nil {
		log.Error("parsing JSON sidecar for %s: %v", entry.basename, err)
		return time.Time{}
	}

	photoDate := meta.PhotoTakenTime
	if photoDate.IsZero() && !entry.zipModTime.IsZero() {
		photoDate = entry.zipModTime
	}

	ext := strings.ToLower(filepath.Ext(destPath))

	// Set MP4 container creation time before XMP (gomp4 can't parse UUID boxes).
	if !photoDate.IsZero() && (ext == ".mp4" || ext == ".mov") {
		if err := mp4meta.SetCreationTime(destPath, photoDate); err != nil {
			log.Error("setting MP4 metadata for %s: %v", entry.basename, err)
		}
	}

	xmpMeta := xmp.Metadata{
		Title:       meta.Title,
		Description: meta.Description,
		CreateDate:  photoDate,
	}
	if !xmpMeta.IsEmpty() {
		switch ext {
		case ".jpg", ".jpeg":
			if err := jpegmeta.SetMetadata(destPath, xmpMeta); err != nil {
				log.Error("setting JPEG XMP metadata for %s: %v", entry.basename, err)
			}
		case ".mp4", ".mov":
			if err := mp4meta.SetXMPMetadata(destPath, xmpMeta); err != nil {
				log.Error("setting MP4 XMP metadata for %s: %v", entry.basename, err)
			}
		}
	}

	if !photoDate.IsZero() {
		if err := os.Chtimes(destPath, photoDate, photoDate); err != nil {
			log.Error("setting file time for %s: %v", entry.basename, err)
		}
	}

	return photoDate
}

// readGroupSidecars reads JSON sidecar data needed by a group of media entries.
// Sidecars whose recorded zipPath matches currentZipPath are read directly from
// zipLookup (the already-open zip). Sidecars in other zips are opened on demand.
func readGroupSidecars(log *logging.Logger, entries []*mediaEntry, currentZipPath string, zipLookup map[string]*zip.File) map[string][]byte {
	jsonData := make(map[string][]byte)

	// Separate sidecars into local (same zip, already open) and external.
	type externalRef struct {
		entryName string
	}
	externalByZip := make(map[string][]externalRef)

	for _, me := range entries {
		if me.jsonEntry == nil {
			continue
		}
		if me.jsonEntry.zipPath == currentZipPath {
			// Sidecar is in the current zip — read directly from the open reader.
			jf, ok := zipLookup[me.jsonEntry.entryName]
			if !ok {
				continue
			}
			data := readZipEntry(log, jf, me.jsonEntry.entryName)
			if data != nil {
				jsonData[me.jsonEntry.entryName] = data
			}
		} else {
			// Sidecar is in a different zip part.
			externalByZip[me.jsonEntry.zipPath] = append(
				externalByZip[me.jsonEntry.zipPath],
				externalRef{entryName: me.jsonEntry.entryName},
			)
		}
	}

	// Open each external zip, read the needed sidecars, then close.
	for zp, refs := range externalByZip {
		r, err := zip.OpenReader(zp)
		if err != nil {
			log.Error("opening zip for sidecars %s: %v", zp, err)
			continue
		}
		lookup := make(map[string]*zip.File, len(r.File))
		for _, f := range r.File {
			lookup[f.Name] = f
		}
		for _, ref := range refs {
			jf, ok := lookup[ref.entryName]
			if !ok {
				continue
			}
			data := readZipEntry(log, jf, ref.entryName)
			if data != nil {
				jsonData[ref.entryName] = data
			}
		}
		_ = r.Close()
	}

	return jsonData
}

// readZipEntry reads the full contents of a zip file entry.
func readZipEntry(log *logging.Logger, f *zip.File, name string) []byte {
	rc, err := f.Open()
	if err != nil {
		log.Error("opening JSON sidecar %s: %v", name, err)
		return nil
	}
	data, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		log.Error("reading JSON sidecar %s: %v", name, err)
		return nil
	}
	return data
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

// loadImportLog reads the import log and returns a set of relative paths
// that have already been imported.
func loadImportLog(logPath string) (map[string]bool, error) {
	result := make(map[string]bool)

	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			result[line] = true
		}
	}
	return result, scanner.Err()
}

// appendImportLog appends a relative path to the import log.
func appendImportLog(path, relPath string) (err error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	_, err = fmt.Fprintln(f, relPath)
	return err
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
