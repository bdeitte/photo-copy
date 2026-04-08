package google

import (
	"archive/zip"
	"context"
	"fmt"
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
//
// JSON sidecars are matched to media across all zip files, not just within the
// same zip. Google Takeout splits large exports into multiple zip parts, and a
// folder's media and JSON sidecar may end up in different parts.
func scanZips(ctx context.Context, zipPaths []string) (*scanIndex, error) {
	// Collect all entries across all zips, grouped by folder name.
	// The same folder can appear in multiple zip parts.
	allFolders := make(map[string]*folderContents)

	for _, zipPath := range zipPaths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := scanOneZip(zipPath, allFolders); err != nil {
			return nil, err
		}
	}

	// Match JSON sidecars to media within each folder (across all zips).
	var allMedia []*mediaEntry
	type dedupKey struct {
		basename string
		size     uint64
	}
	albumKeys := make(map[dedupKey]bool)

	for _, fc := range allFolders {
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

		for _, me := range fc.mediaEntries {
			allMedia = append(allMedia, me)
			if !me.isYearFolder {
				albumKeys[dedupKey{basename: normalizedBasename(me.basename), size: me.size}] = true
			}
		}
	}

	// Mark year-folder entries as skipped when an album entry
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

// scanOneZip scans a single zip file and adds its media and JSON entries to
// allFolders, grouped by folder name. The same folder may span multiple zips.
func scanOneZip(zipPath string, allFolders map[string]*folderContents) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip %s: %w", zipPath, err)
	}
	defer func() { _ = r.Close() }()

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

		if _, ok := allFolders[folder]; !ok {
			allFolders[folder] = &folderContents{
				jsonEntries: make(map[string]*zipEntry),
			}
		}
		fc := allFolders[folder]

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

	return nil
}

