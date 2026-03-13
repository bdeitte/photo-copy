package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/flickr"
	"github.com/briandeitte/photo-copy/internal/logging"
	"github.com/spf13/cobra"
)

func newFlickrCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "flickr",
		Short: "Flickr upload and download commands",
	}

	cmd.AddCommand(newFlickrDownloadCmd())
	cmd.AddCommand(newFlickrUploadCmd())
	return cmd
}

func newFlickrDownloadCmd() *cobra.Command {
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

			log := logging.New(debug, nil)
			client := flickr.NewClient(cfg, log)
			return client.Download(context.Background(), outputDir, limit)
		},
	}

	return cmd
}

func newFlickrUploadCmd() *cobra.Command {
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

			log := logging.New(debug, nil)
			client := flickr.NewClient(cfg, log)
			return client.Upload(context.Background(), inputDir, limit)
		},
	}

	return cmd
}
