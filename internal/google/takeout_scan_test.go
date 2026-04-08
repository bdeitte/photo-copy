package google

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

func TestMatchJSONToMedia_Direct(t *testing.T) {
	media := []string{"photo.jpg", "video.mp4"}
	got := matchJSONToMedia("photo.jpg.json", media)
	if got != "photo.jpg" {
		t.Errorf("matchJSONToMedia() = %q, want %q", got, "photo.jpg")
	}
}

func TestMatchJSONToMedia_NoMatch(t *testing.T) {
	media := []string{"other.jpg"}
	got := matchJSONToMedia("photo.jpg.json", media)
	if got != "" {
		t.Errorf("matchJSONToMedia() = %q, want empty", got)
	}
}

func TestMatchJSONToMedia_Truncated(t *testing.T) {
	// JSON name (minus .json) is 46 chars — Google truncates long filenames
	longName := "Urlaub in Knaufspesch in der Schneifel (38).JPG"
	// Google truncates to 46 chars: "Urlaub in Knaufspesch in der Schneifel (38).JP"
	truncatedJSON := "Urlaub in Knaufspesch in der Schneifel (38).JP.json"
	media := []string{longName}

	got := matchJSONToMedia(truncatedJSON, media)
	if got != longName {
		t.Errorf("matchJSONToMedia() = %q, want %q", got, longName)
	}
}

func TestMatchJSONToMedia_BracketSwap(t *testing.T) {
	// Google swaps: image(11).jpg -> image.jpg(11).json
	media := []string{"image(11).jpg"}
	got := matchJSONToMedia("image.jpg(11).json", media)
	if got != "image(11).jpg" {
		t.Errorf("matchJSONToMedia() = %q, want %q", got, "image(11).jpg")
	}
}

func TestMatchJSONToMedia_EditedSuffix(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		media    []string
		wantName string
	}{
		{"english", "photo.jpg.json", []string{"photo-edited.jpg"}, "photo-edited.jpg"},
		{"german", "photo.jpg.json", []string{"photo-bearbeitet.jpg"}, "photo-bearbeitet.jpg"},
		{"french", "photo.jpg.json", []string{"photo-modifié.jpg"}, "photo-modifié.jpg"},
		{"polish", "photo.jpg.json", []string{"photo-edytowane.jpg"}, "photo-edytowane.jpg"},
		{"dutch", "photo.jpg.json", []string{"photo-bewerkt.jpg"}, "photo-bewerkt.jpg"},
		{"italian", "photo.jpg.json", []string{"photo-modificato.jpg"}, "photo-modificato.jpg"},
		{"spanish", "photo.jpg.json", []string{"photo-ha editado.jpg"}, "photo-ha editado.jpg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchJSONToMedia(tt.json, tt.media)
			if got != tt.wantName {
				t.Errorf("matchJSONToMedia() = %q, want %q", got, tt.wantName)
			}
		})
	}
}

func TestMatchJSONToMedia_TruncatedPrefersUnedited(t *testing.T) {
	// When both original and edited variant share the same 46-char prefix,
	// the unedited file should be preferred.
	original := "Urlaub in Knaufspesch in der Schneifel (38).JPG"
	edited := "Urlaub in Knaufspesch in der Schneifel (38)-edited.JPG"
	truncatedJSON := "Urlaub in Knaufspesch in der Schneifel (38).JP.json"
	media := []string{edited, original} // edited listed first to test ordering

	got := matchJSONToMedia(truncatedJSON, media)
	if got != original {
		t.Errorf("matchJSONToMedia() = %q, want %q (should prefer unedited)", got, original)
	}
}

func TestMatchJSONToMedia_EditedWithBracketSwap(t *testing.T) {
	// Combination: edited suffix + bracket swap
	media := []string{"image(3)-edited.jpg"}
	got := matchJSONToMedia("image.jpg(3).json", media)
	if got != "image(3)-edited.jpg" {
		t.Errorf("matchJSONToMedia() = %q, want %q", got, "image(3)-edited.jpg")
	}
}

func TestMatchJSONToMedia_TruncatedBracketSwap(t *testing.T) {
	// JSON base is 46 runes, bracket-swapped, and the swapped name doesn't exactly
	// match any media file but does match as a prefix.
	// JSON base: "averylongfilenamefortestingpurposes1234.jpg(3)" = 46 runes
	// Bracket swap: m[1]="averylongfilenamefortestingpurposes1234" m[2]=".jpg" m[3]="(3)"
	// swapped = "averylongfilenamefortestingpurposes1234(3).jpg" (exact media not present)
	// swappedStem = "averylongfilenamefortestingpurposes1234(3)"
	// Media file starts with the swapped stem, so prefix matching should find it.
	mediaFile := "averylongfilenamefortestingpurposes1234(3).JPG"
	jsonName := "averylongfilenamefortestingpurposes1234.jpg(3).json"
	// Verify directBase is 46 runes (the truncation boundary).
	directBase := "averylongfilenamefortestingpurposes1234.jpg(3)"
	if utf8.RuneCountInString(directBase) != 46 {
		t.Fatalf("test setup error: directBase must be 46 runes, got %d", utf8.RuneCountInString(directBase))
	}

	got := matchJSONToMedia(jsonName, []string{mediaFile})
	if got != mediaFile {
		t.Errorf("matchJSONToMedia() = %q, want %q (truncated+bracket-swap prefix match)", got, mediaFile)
	}
}

func TestMatchJSONToMedia_TruncatedMultibyte(t *testing.T) {
	// A filename with accented characters where 46 characters != 46 bytes.
	// "Ürlaub ïn Knaufspesch ïn dér Schnéifel (38).JPG" is 47 runes but 52 bytes.
	// Google truncates to 46 characters: "Ürlaub ïn Knaufspesch ïn dér Schnéifel (38).JP"
	// which is 46 runes but 51 bytes — the byte-based check would miss this.
	longName := "Ürlaub ïn Knaufspesch ïn dér Schnéifel (38).JPG"
	truncatedJSON := "Ürlaub ïn Knaufspesch ïn dér Schnéifel (38).JP.json"
	media := []string{longName}

	got := matchJSONToMedia(truncatedJSON, media)
	if got != longName {
		t.Errorf("matchJSONToMedia() = %q, want %q", got, longName)
	}
}

func TestMatchJSONToMedia_NFCNormalization(t *testing.T) {
	// macOS uses NFD for accented chars; JSON may use NFC
	nfdMedia := norm.NFD.String("photo-modifié.jpg")
	media := []string{nfdMedia}
	got := matchJSONToMedia("photo.jpg.json", media)
	if got != nfdMedia {
		t.Errorf("matchJSONToMedia() = %q, want %q", got, nfdMedia)
	}
}

func TestIsYearFolder(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"Photos from 2022", true},
		{"Photos from 1999", true},
		{"Photos from 1800", true},
		{"Photos from 2099", true},
		{"Photos from 123", false},
		{"Trip to Paris", false},
		{"Photos from 2022 extra", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isYearFolder(tt.name); got != tt.want {
				t.Errorf("isYearFolder(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestScanZips_AlbumAndYearFolders(t *testing.T) {
	// Album "Trip" has photo.jpg (12 bytes "jpegdata1234")
	// Year "Photos from 2022" has photo.jpg (same 12 bytes) AND other.jpg
	// Expected: year photo.jpg is skipped (dedup), other.jpg kept
	takeoutDir := t.TempDir()
	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/Trip/photo.jpg":             "jpegdata1234",
		"Google Photos/Trip/photo.jpg.json":         `{"title":"trip photo"}`,
		"Google Photos/Photos from 2022/photo.jpg":  "jpegdata1234",
		"Google Photos/Photos from 2022/other.jpg":  "otherdata",
	})

	zipPath := filepath.Join(takeoutDir, "takeout.zip")
	index, err := scanZips(context.Background(), []string{zipPath})
	if err != nil {
		t.Fatalf("scanZips failed: %v", err)
	}

	var yearPhotoSkip bool
	var albumPhotoFound bool
	var otherFound bool
	for _, entry := range index.media {
		switch {
		case entry.folderName == "Trip" && entry.basename == "photo.jpg":
			albumPhotoFound = true
			if entry.skip {
				t.Error("album photo.jpg should not be skipped")
			}
		case entry.folderName == "Photos from 2022" && entry.basename == "photo.jpg":
			yearPhotoSkip = entry.skip
		case entry.basename == "other.jpg":
			otherFound = true
			if entry.skip {
				t.Error("other.jpg has no album duplicate, should not be skipped")
			}
		}
	}

	if !albumPhotoFound {
		t.Error("album photo.jpg not found in index")
	}
	if !yearPhotoSkip {
		t.Error("year folder photo.jpg should be skipped (duplicate of album)")
	}
	if !otherFound {
		t.Error("other.jpg not found in index")
	}
}

func TestScanZips_DifferentSizeNotDeduped(t *testing.T) {
	takeoutDir := t.TempDir()
	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/Album/photo.jpg":            "short",
		"Google Photos/Photos from 2022/photo.jpg": "muchlongerdata",
	})

	zipPath := filepath.Join(takeoutDir, "takeout.zip")
	index, err := scanZips(context.Background(), []string{zipPath})
	if err != nil {
		t.Fatalf("scanZips failed: %v", err)
	}

	for _, entry := range index.media {
		if entry.skip {
			t.Errorf("no entries should be skipped when sizes differ, but %s/%s was", entry.folderName, entry.basename)
		}
	}
}

func TestScanZips_MultipleAlbumsNotDeduped(t *testing.T) {
	takeoutDir := t.TempDir()
	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/Trip/photo.jpg":        "jpegdata",
		"Google Photos/Best Photos/photo.jpg": "jpegdata",
	})

	zipPath := filepath.Join(takeoutDir, "takeout.zip")
	index, err := scanZips(context.Background(), []string{zipPath})
	if err != nil {
		t.Fatalf("scanZips failed: %v", err)
	}

	skipped := 0
	for _, entry := range index.media {
		if entry.skip {
			skipped++
		}
	}
	if skipped != 0 {
		t.Errorf("no inter-album dedup should occur, but %d entries were skipped", skipped)
	}
}

func TestScanZips_JSONSidecarMatched(t *testing.T) {
	takeoutDir := t.TempDir()
	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/Trip/photo.jpg":      "jpegdata",
		"Google Photos/Trip/photo.jpg.json": `{"title":"my photo"}`,
	})

	zipPath := filepath.Join(takeoutDir, "takeout.zip")
	index, err := scanZips(context.Background(), []string{zipPath})
	if err != nil {
		t.Fatalf("scanZips failed: %v", err)
	}

	for _, entry := range index.media {
		if entry.basename == "photo.jpg" {
			if entry.jsonEntry == nil {
				t.Error("photo.jpg should have a matched JSON sidecar")
			}
			return
		}
	}
	t.Error("photo.jpg not found in index")
}

func TestScanZips_MultipleZips(t *testing.T) {
	dir := t.TempDir()
	createTestZip(t, dir, map[string]string{
		"Google Photos/Album1/a.jpg": "data1",
	})
	_ = os.Rename(filepath.Join(dir, "takeout.zip"), filepath.Join(dir, "takeout-001.zip"))
	createTestZip(t, dir, map[string]string{
		"Google Photos/Album2/b.jpg": "data2",
	})
	_ = os.Rename(filepath.Join(dir, "takeout.zip"), filepath.Join(dir, "takeout-002.zip"))

	index, err := scanZips(context.Background(), []string{
		filepath.Join(dir, "takeout-001.zip"),
		filepath.Join(dir, "takeout-002.zip"),
	})
	if err != nil {
		t.Fatalf("scanZips failed: %v", err)
	}

	if len(index.media) != 2 {
		t.Errorf("expected 2 media entries, got %d", len(index.media))
	}
}

func TestScanZips_YearFolderOnlyNotSkipped(t *testing.T) {
	takeoutDir := t.TempDir()
	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/Photos from 2022/solo.jpg": "solodata",
	})

	zipPath := filepath.Join(takeoutDir, "takeout.zip")
	index, err := scanZips(context.Background(), []string{zipPath})
	if err != nil {
		t.Fatalf("scanZips failed: %v", err)
	}

	if len(index.media) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(index.media))
	}
	if index.media[0].skip {
		t.Error("year-folder-only file should not be skipped")
	}
}

func TestScanZips_JSONSidecarAcrossZips(t *testing.T) {
	// Media in one zip, JSON sidecar in another (same folder split across parts).
	dir := t.TempDir()
	createTestZip(t, dir, map[string]string{
		"Google Photos/Trip/photo.jpg": "jpegdata",
	})
	_ = os.Rename(filepath.Join(dir, "takeout.zip"), filepath.Join(dir, "takeout-001.zip"))
	createTestZip(t, dir, map[string]string{
		"Google Photos/Trip/photo.jpg.json": `{"title":"my photo"}`,
	})
	_ = os.Rename(filepath.Join(dir, "takeout.zip"), filepath.Join(dir, "takeout-002.zip"))

	index, err := scanZips(context.Background(), []string{
		filepath.Join(dir, "takeout-001.zip"),
		filepath.Join(dir, "takeout-002.zip"),
	})
	if err != nil {
		t.Fatalf("scanZips failed: %v", err)
	}

	if len(index.media) != 1 {
		t.Fatalf("expected 1 media entry, got %d", len(index.media))
	}
	if index.media[0].jsonEntry == nil {
		t.Error("photo.jpg should have a matched JSON sidecar from a different zip")
	}
}

func TestScanZips_DedupAcrossZips(t *testing.T) {
	dir := t.TempDir()
	createTestZip(t, dir, map[string]string{
		"Google Photos/Trip/photo.jpg": "jpegdata1234",
	})
	_ = os.Rename(filepath.Join(dir, "takeout.zip"), filepath.Join(dir, "takeout-001.zip"))
	createTestZip(t, dir, map[string]string{
		"Google Photos/Photos from 2022/photo.jpg": "jpegdata1234",
	})
	_ = os.Rename(filepath.Join(dir, "takeout.zip"), filepath.Join(dir, "takeout-002.zip"))

	index, err := scanZips(context.Background(), []string{
		filepath.Join(dir, "takeout-001.zip"),
		filepath.Join(dir, "takeout-002.zip"),
	})
	if err != nil {
		t.Fatalf("scanZips failed: %v", err)
	}

	if len(index.media) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(index.media))
	}

	for _, entry := range index.media {
		if entry.isYearFolder && entry.basename == "photo.jpg" && !entry.skip {
			t.Error("year folder photo.jpg should be skipped (album duplicate in different zip)")
		}
	}
}
