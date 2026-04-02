package transfer

import (
	"context"

	"github.com/briandeitte/photo-copy/internal/daterange"
)

// UploadOpts holds common options for upload operations.
type UploadOpts struct {
	Limit     int
	DateRange *daterange.DateRange
	MediaOnly bool
}

// DownloadOpts holds common options for download operations.
type DownloadOpts struct {
	Limit      int
	DateRange  *daterange.DateRange
	NoMetadata bool
}

// PhotoService defines the common interface for photo service operations.
type PhotoService interface {
	Upload(ctx context.Context, dir string, opts UploadOpts) (*Result, error)
	Download(ctx context.Context, dir string, opts DownloadOpts) (*Result, error)
}
