package cli

import (
	"context"
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
	var outputDir string

	cmd := &cobra.Command{
		Use:   "download",
		Short: "Download all photos from Flickr",
		RunE: func(cmd *cobra.Command, args []string) error {
			if outputDir == "" {
				return fmt.Errorf("--output-dir is required")
			}

			cfg, err := config.LoadFlickrConfig(config.DefaultDir())
			if err != nil {
				return fmt.Errorf("loading Flickr config (run 'photo-copy config flickr' first): %w", err)
			}

			log := logging.New(debug, nil)
			client := flickr.NewClient(cfg, log)
			return client.Download(context.Background(), outputDir)
		},
	}

	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Directory to save downloaded photos")
	cmd.MarkFlagRequired("output-dir")
	return cmd
}

func newFlickrUploadCmd() *cobra.Command {
	var inputDir string

	cmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload photos to Flickr",
		RunE: func(cmd *cobra.Command, args []string) error {
			if inputDir == "" {
				return fmt.Errorf("--input-dir is required")
			}

			cfg, err := config.LoadFlickrConfig(config.DefaultDir())
			if err != nil {
				return fmt.Errorf("loading Flickr config (run 'photo-copy config flickr' first): %w", err)
			}

			log := logging.New(debug, nil)
			client := flickr.NewClient(cfg, log)
			return client.Upload(context.Background(), inputDir)
		},
	}

	cmd.Flags().StringVar(&inputDir, "input-dir", "", "Directory containing photos to upload")
	cmd.MarkFlagRequired("input-dir")
	return cmd
}
