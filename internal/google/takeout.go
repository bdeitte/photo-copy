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

	"github.com/briandeitte/photo-copy/internal/jpegmeta"
	"github.com/briandeitte/photo-copy/internal/logging"
	"github.com/briandeitte/photo-copy/internal/mp4meta"
	"github.com/briandeitte/photo-copy/internal/transfer"
	"github.com/briandeitte/photo-copy/internal/xmp"
	"github.com/schollz/progressbar/v3"
)

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

	if len(toExtract) == 0 {
		result.Finish()
		return result, nil
	}

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

	bar := progressbar.Default(int64(len(toExtract)), "extracting")

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

		for _, me := range g.entries {
			if err := ctx.Err(); err != nil {
				_ = r.Close()
				return result, err
			}

			f, ok := zipLookup[me.entryName]
			if !ok {
				log.Error("entry %s not found in zip %s", me.entryName, g.zipPath)
				result.RecordError(me.basename, "entry not found in zip")
				_ = bar.Add(1)
				continue
			}

			// Determine destination path: album -> subdirectory, year -> flat.
			var destPath string
			if me.isYearFolder || me.folderName == "" {
				destPath = filepath.Join(outputDir, me.basename)
			} else {
				destDir := filepath.Join(outputDir, me.folderName)
				if err := os.MkdirAll(destDir, 0755); err != nil {
					log.Error("creating album dir %s: %v", destDir, err)
					result.RecordError(me.basename, err.Error())
					_ = bar.Add(1)
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
						log.Error("checking destination %s: %v", destPath, serr)
						result.RecordError(me.basename, fmt.Sprintf("checking destination: %v", serr))
						_ = bar.Add(1)
						collisionErr = true
						break
					}
				}
				if collisionErr {
					continue
				}
				log.Debug("duplicate filename %s, saving as %s", me.basename, filepath.Base(destPath))
			} else if !errors.Is(err, os.ErrNotExist) {
				log.Error("checking destination %s: %v", destPath, err)
				result.RecordError(me.basename, err.Error())
				_ = bar.Add(1)
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
				log.Error("extracting %s: %v", me.entryName, err)
				result.RecordError(me.basename, err.Error())
				_ = bar.Add(1)
				continue
			}

			info, statErr := os.Stat(destPath)
			if statErr == nil {
				result.RecordSuccess(info.Size())
			} else {
				result.RecordSuccess(0)
			}

			if !noMetadata {
				var jd []byte
				if me.jsonEntry != nil {
					jd = jsonData[me.jsonEntry.entryName]
				}
				applyTakeoutMetadata(log, me, destPath, jd)
			}

			_ = bar.Add(1)

			if cfg.afterExtract != nil {
				cfg.afterExtract()
			}
		}

		_ = r.Close()
	}

	result.Finish()
	return result, nil
}

// applyTakeoutMetadata embeds metadata (title, description, creation date) into
// the extracted file using the pre-read JSON sidecar data. If jsonData is nil,
// the file has no matched sidecar and metadata is skipped.
func applyTakeoutMetadata(log *logging.Logger, entry *mediaEntry, destPath string, jsonData []byte) {
	if jsonData == nil {
		log.Debug("no JSON sidecar for %s, skipping metadata", entry.basename)
		return
	}

	meta, err := parseTakeoutJSON(jsonData)
	if err != nil {
		log.Error("parsing JSON sidecar for %s: %v", entry.basename, err)
		return
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
