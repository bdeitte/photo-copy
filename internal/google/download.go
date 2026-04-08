package google

import (
	"context"

	"github.com/briandeitte/photo-copy/internal/logging"
	"github.com/briandeitte/photo-copy/internal/transfer"
)

// Download extracts media files from Google Takeout zip archives.
// It delegates to ImportTakeout, which handles zip extraction and deduplication.
func Download(ctx context.Context, takeoutDir, outputDir string, log *logging.Logger, noMetadata bool) (*transfer.Result, error) {
	return ImportTakeout(ctx, takeoutDir, outputDir, log, noMetadata)
}
