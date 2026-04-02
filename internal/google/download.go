package google

import (
	"github.com/briandeitte/photo-copy/internal/logging"
	"github.com/briandeitte/photo-copy/internal/transfer"
)

// Download extracts media files from Google Takeout zip archives.
// It delegates to ImportTakeout, which handles zip extraction and deduplication.
func Download(takeoutDir, outputDir string, log *logging.Logger) (*transfer.Result, error) {
	return ImportTakeout(takeoutDir, outputDir, log)
}
