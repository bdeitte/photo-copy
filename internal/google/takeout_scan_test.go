package google

import (
	"testing"

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

func TestMatchJSONToMedia_EditedWithBracketSwap(t *testing.T) {
	// Combination: edited suffix + bracket swap
	media := []string{"image(3)-edited.jpg"}
	got := matchJSONToMedia("image.jpg(3).json", media)
	if got != "image(3)-edited.jpg" {
		t.Errorf("matchJSONToMedia() = %q, want %q", got, "image(3)-edited.jpg")
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
