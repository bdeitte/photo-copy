package cli

import (
	"context"
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
	cmd := &cobra.Command{
		Use:   "download <output-dir>",
		Short: "Download all photos from Flickr",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			outputDir := args[0]

			cfg, err := config.LoadFlickrConfig(config.DefaultDir())
			if err != nil {
				if errors.Is(err, config.ErrNotConfigured) {
					return fmt.Errorf("Flickr credentials not configured. Run 'photo-copy config flickr' to set up") //nolint:staticcheck // proper noun
				}
				return fmt.Errorf("loading Flickr config: %w", err)
			}

			log := logging.New(opts.debug, nil)
			client := flickr.NewClient(cfg, log)
			result, err := client.Download(context.Background(), outputDir, opts.limit)
			if result != nil {
				logPath := filepath.Join(outputDir, "transfer.log")
				// Build lookup map once instead of reading directory per log entry.
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
				transfer.HandleResult(result, log, outputDir)
			}
			return err
		},
	}

	return cmd
}

func newFlickrUploadCmd(opts *rootOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upload <input-dir>",
		Short: "Upload photos to Flickr",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			inputDir := args[0]

			cfg, err := config.LoadFlickrConfig(config.DefaultDir())
			if err != nil {
				if errors.Is(err, config.ErrNotConfigured) {
					return fmt.Errorf("Flickr credentials not configured. Run 'photo-copy config flickr' to set up") //nolint:staticcheck // proper noun
				}
				return fmt.Errorf("loading Flickr config: %w", err)
			}

			log := logging.New(opts.debug, nil)
			client := flickr.NewClient(cfg, log)
			result, err := client.Upload(context.Background(), inputDir, opts.limit)
			transfer.HandleResult(result, log, inputDir)
			return err
		},
	}

	return cmd
}
