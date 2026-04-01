package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/flickr"
	"github.com/briandeitte/photo-copy/internal/logging"
	"github.com/briandeitte/photo-copy/internal/transfer"
	"github.com/spf13/cobra"
)

func newFlickrCmd(opts *rootOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "flickr",
		Short: "Flickr upload and download commands",
	}

	cmd.AddCommand(newFlickrDownloadCmd(opts))
	cmd.AddCommand(newFlickrUploadCmd(opts))
	return cmd
}

func newFlickrDownloadCmd(opts *rootOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "download <output-dir>",
		Short: "Download all photos/videos from Flickr",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFlickrDownload(cmd, opts, args[0])
		},
	}
}

func newFlickrUploadCmd(opts *rootOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "upload <input-dir>",
		Short: "Upload photos/videos to Flickr",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFlickrUpload(cmd, opts, args[0])
		},
	}
}

func newFlickrClient(opts *rootOpts) (*flickr.Client, *logging.Logger, error) {
	cfg, err := config.LoadFlickrConfig(config.DefaultDir())
	if err != nil {
		if errors.Is(err, config.ErrNotConfigured) {
			return nil, nil, fmt.Errorf("Flickr credentials not configured. Run 'photo-copy config flickr' to set up") //nolint:staticcheck // proper noun
		}
		return nil, nil, fmt.Errorf("loading Flickr config: %w", err)
	}
	log := logging.New(opts.debug, nil)
	return flickr.NewClient(cfg, log), log, nil
}

func runFlickrDownload(cmd *cobra.Command, opts *rootOpts, outputDir string) error {
	client, log, err := newFlickrClient(opts)
	if err != nil {
		return err
	}
	result, err := client.Download(cmd.Context(), outputDir, opts.limit, opts.noMetadata, opts.parsedDateRange)
	if err != nil {
		return err
	}
	validateFlickrTransferLog(result, outputDir, log)
	transfer.HandleResult(result, log, outputDir)
	return nil
}

func runFlickrUpload(cmd *cobra.Command, opts *rootOpts, inputDir string) error {
	client, log, err := newFlickrClient(opts)
	if err != nil {
		return err
	}
	result, err := client.Upload(cmd.Context(), inputDir, opts.limit, opts.parsedDateRange)
	if err != nil {
		return err
	}
	transfer.HandleResult(result, log, inputDir)
	return nil
}

// validateFlickrTransferLog checks that each transfer log entry has a
// corresponding file on disk, matching by the Flickr photo-ID prefix
// (the portion before the first underscore in each filename).
func validateFlickrTransferLog(result *transfer.Result, outputDir string, log *logging.Logger) {
	if result == nil {
		return
	}
	logPath := filepath.Join(outputDir, "transfer.log")
	dirEntries, readDirErr := os.ReadDir(outputDir)
	if readDirErr != nil {
		log.Error("reading output dir for validation: %v", readDirErr)
	}
	filesByPrefix := make(map[string]string, len(dirEntries))
	for _, e := range dirEntries {
		if idx := strings.Index(e.Name(), "_"); idx > 0 {
			filesByPrefix[e.Name()[:idx]] = e.Name()
		}
	}
	result.ValidateTransferLog(logPath, func(entry string) string {
		if name, ok := filesByPrefix[entry]; ok {
			return filepath.Join(outputDir, name)
		}
		return ""
	})
}
