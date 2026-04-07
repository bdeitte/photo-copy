package google

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

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
