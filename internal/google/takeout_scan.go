package google

import (
	"archive/zip"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/briandeitte/photo-copy/internal/media"
)

// normalizedBasename returns a case-folded NFC-normalized basename for dedup comparisons.
func normalizedBasename(s string) string {
	return norm.NFC.String(strings.ToLower(s))
}

// editedSuffixes are localized suffixes Google appends to edited photos.
// The JSON sidecar matches the original filename without the suffix.
var editedSuffixes = []string{
	"-edited",
	"-bearbeitet",  // German
	"-modifié",     // French
	"-edytowane",   // Polish
	"-bewerkt",     // Dutch
	"-modificato",  // Italian
	"-ha editado",  // Spanish
}

// bracketSwapRe matches Google's bracket-swapped JSON naming:
// image.jpg(11).json comes from image(11).jpg
var bracketSwapRe = regexp.MustCompile(`^(.+)(\.\w+)(\(\d+\))\.json$`)

// matchJSONToMedia finds the media filename that a JSON sidecar belongs to.
// Returns empty string if no match is found.
// Rules applied in order: direct match, truncated (46-char), bracket swap,
// then edited-suffix variants of each.
func matchJSONToMedia(jsonName string, mediaNames []string) string {
	if !strings.HasSuffix(strings.ToLower(jsonName), ".json") {
		return ""
	}

	// Try direct match and its edited variants
	directBase := strings.TrimSuffix(jsonName, filepath.Ext(jsonName)) // strips .json
	if m := findMedia(directBase, mediaNames); m != "" {
		return m
	}

	// Google truncates JSON sidecar base names to 46 characters.
	if utf8.RuneCountInString(directBase) == 46 {
		if m := findMediaByPrefix(directBase, mediaNames); m != "" {
			return m
		}
	}

	// Try bracket swap: image.jpg(11).json -> image(11).jpg
	if m := bracketSwapRe.FindStringSubmatch(jsonName); m != nil {
		// m[1]="image", m[2]=".jpg", m[3]="(11)"
		swapped := m[1] + m[3] + m[2] // "image(11).jpg"
		if match := findMedia(swapped, mediaNames); match != "" {
			return match
		}
		// Truncated + bracket-swapped: if the JSON base was 46 runes (truncated),
		// also try prefix matching on the swapped stem (m[1]+m[3]).
		if utf8.RuneCountInString(directBase) == 46 {
			swappedStem := m[1] + m[3]
			if match := findMediaByPrefix(swappedStem, mediaNames); match != "" {
				return match
			}
		}
	}

	return ""
}

// findMedia checks if baseName matches a media file directly, or with an
// edited suffix stripped. baseName should be the full media filename the JSON
// refers to (e.g., "photo.jpg").
func findMedia(baseName string, mediaNames []string) string {
	normalized := norm.NFC.String(baseName)
	for _, m := range mediaNames {
		if norm.NFC.String(m) == normalized {
			return m
		}
	}

	// Try edited suffix: JSON is "photo.jpg.json" -> baseName is "photo.jpg"
	// Media might be "photo-edited.jpg"
	ext := filepath.Ext(baseName)
	stem := strings.TrimSuffix(baseName, ext)
	for _, suffix := range editedSuffixes {
		candidate := stem + suffix + ext
		candidateNorm := norm.NFC.String(candidate)
		for _, m := range mediaNames {
			if norm.NFC.String(m) == candidateNorm {
				return m
			}
		}
	}

	return ""
}

// findMediaByPrefix matches a truncated JSON base (46 chars) as a prefix of media filenames.
func findMediaByPrefix(prefix string, mediaNames []string) string {
	normalizedPrefix := norm.NFC.String(prefix)
	for _, m := range mediaNames {
		if strings.HasPrefix(norm.NFC.String(m), normalizedPrefix) {
			return m
		}
	}

	// Also try with edited suffixes stripped from the media name
	for _, m := range mediaNames {
		ext := filepath.Ext(m)
		stem := strings.TrimSuffix(m, ext)
		for _, suffix := range editedSuffixes {
			stripped := strings.TrimSuffix(stem, suffix) + ext
			if strings.HasPrefix(norm.NFC.String(stripped), normalizedPrefix) {
				return m
			}
		}
	}

	return ""
}

// yearFolderRe matches Google Takeout's date-based folder names like "Photos from 2022".
// Valid years: 1800–1899 (18xx), 1900–1999 (19xx), and 2000–2099 (20xx).
var yearFolderRe = regexp.MustCompile(`^Photos from (18|19|20)\d{2}$`)

// isYearFolder reports whether name is a Google Takeout year folder.
func isYearFolder(name string) bool {
	return yearFolderRe.MatchString(name)
}

// mediaEntry represents a single media file found in a Takeout zip.
type mediaEntry struct {
	zipPath      string
	entryName    string    // full path within the zip
	folderName   string    // immediate parent folder name
	basename     string
	size         uint64    // uncompressed file size
	isYearFolder bool
	skip         bool      // true if deduped away
	jsonEntry    *zipEntry // matched JSON sidecar, if any
	zipModTime   time.Time // modification time from zip entry header
}

// zipEntry is a lightweight reference to a zip file entry (used for JSON sidecars).
type zipEntry struct {
	zipPath   string
	entryName string
}

// scanIndex holds the results of scanning all Takeout zip files.
type scanIndex struct {
	media []*mediaEntry
}

// folderContents groups media and JSON entries found in a single folder within a zip.
type folderContents struct {
	mediaEntries []*mediaEntry
	jsonEntries  map[string]*zipEntry // basename -> zipEntry
}

// scanZips reads directory entries from all provided zip files, classifies them,
// matches JSON sidecars to media, and deduplicates year-folder entries that also
// appear in an album folder (same basename and uncompressed size).
func scanZips(zipPaths []string) (*scanIndex, error) {
	// albumKeys collects (basename, size) pairs from album folders so we can
	// identify year-folder duplicates.
	type dedupKey struct {
		basename string
		size     uint64
	}
	albumKeys := make(map[dedupKey]bool)

	// First pass: collect all entries, classifying folders and matching sidecars.
	var allMedia []*mediaEntry

	for _, zipPath := range zipPaths {
		entries, err := scanOneZip(zipPath)
		if err != nil {
			return nil, err
		}
		for _, me := range entries {
			allMedia = append(allMedia, me)
			if !me.isYearFolder {
				albumKeys[dedupKey{basename: normalizedBasename(me.basename), size: me.size}] = true
			}
		}
	}

	// Second pass: mark year-folder entries as skipped when an album entry
	// with the same basename and size exists.
	for _, me := range allMedia {
		if me.isYearFolder {
			key := dedupKey{basename: normalizedBasename(me.basename), size: me.size}
			if albumKeys[key] {
				me.skip = true
			}
		}
	}

	return &scanIndex{media: allMedia}, nil
}

// scanOneZip scans a single zip file and returns all media entries with matched
// JSON sidecars. It classifies each entry's parent folder as a year folder or album.
func scanOneZip(zipPath string) ([]*mediaEntry, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("open zip %s: %w", zipPath, err)
	}
	defer func() { _ = r.Close() }()

	// Group entries by folder within the zip.
	folders := make(map[string]*folderContents)

	for _, f := range r.File {
		name := f.Name
		// Only handle entries under "Google Photos/" with a subfolder.
		const prefix = "Google Photos/"
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		rest := strings.TrimPrefix(name, prefix)
		// Skip the top-level directory entry itself.
		if rest == "" || strings.HasSuffix(name, "/") {
			continue
		}

		parts := strings.SplitN(rest, "/", 2)
		if len(parts) != 2 {
			// File directly under "Google Photos/" with no subfolder — treat as album.
			parts = []string{"", rest}
		}
		folder := parts[0]
		base := parts[1]

		if _, ok := folders[folder]; !ok {
			folders[folder] = &folderContents{
				jsonEntries: make(map[string]*zipEntry),
			}
		}
		fc := folders[folder]

		lowerBase := strings.ToLower(base)
		if strings.HasSuffix(lowerBase, ".json") {
			fc.jsonEntries[base] = &zipEntry{zipPath: zipPath, entryName: name}
			continue
		}

		if !media.IsSupportedFile(base) {
			continue
		}

		entry := &mediaEntry{
			zipPath:      zipPath,
			entryName:    name,
			folderName:   folder,
			basename:     base,
			size:         f.UncompressedSize64,
			isYearFolder: isYearFolder(folder),
			zipModTime:   f.Modified,
		}
		fc.mediaEntries = append(fc.mediaEntries, entry)
	}

	// Match JSON sidecars within each folder.
	var result []*mediaEntry
	for _, fc := range folders {
		mediaNames := make([]string, len(fc.mediaEntries))
		for i, me := range fc.mediaEntries {
			mediaNames[i] = me.basename
		}

		for jsonBase, ze := range fc.jsonEntries {
			matched := matchJSONToMedia(jsonBase, mediaNames)
			if matched == "" {
				continue
			}
			for _, me := range fc.mediaEntries {
				if me.basename == matched {
					me.jsonEntry = ze
					break
				}
			}
		}

		result = append(result, fc.mediaEntries...)
	}

	return result, nil
}

// readJSONSidecar reads and parses the JSON sidecar for a media entry from its zip.
// Returns nil if the entry has no matched sidecar.
func readJSONSidecar(entry *mediaEntry) (*takeoutMeta, error) { //nolint:unused // wired up in Task 4
	if entry.jsonEntry == nil {
		return nil, nil
	}

	r, err := zip.OpenReader(entry.jsonEntry.zipPath)
	if err != nil {
		return nil, fmt.Errorf("opening zip for JSON: %w", err)
	}
	defer func() { _ = r.Close() }()

	for _, f := range r.File {
		if f.Name != entry.jsonEntry.entryName {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("opening JSON entry: %w", err)
		}
		defer func() { _ = rc.Close() }()

		data, err := io.ReadAll(rc)
		if err != nil {
			return nil, fmt.Errorf("reading JSON sidecar: %w", err)
		}
		return parseTakeoutJSON(data)
	}

	return nil, fmt.Errorf("JSON entry %s not found in zip", entry.jsonEntry.entryName)
}
