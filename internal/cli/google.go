package cli

import (
	"context"
	"fmt"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/google"
	"github.com/briandeitte/photo-copy/internal/logging"
	"github.com/spf13/cobra"
)

func newGooglePhotosCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "google-photos",
		Short: "Google Photos upload and Takeout import commands",
	}

	cmd.AddCommand(newGoogleUploadCmd())
	cmd.AddCommand(newGoogleImportTakeoutCmd())
	return cmd
}

func newGoogleUploadCmd() *cobra.Command {
	var inputDir string

	cmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload photos to Google Photos",
		RunE: func(cmd *cobra.Command, args []string) error {
			if inputDir == "" {
				return fmt.Errorf("--input-dir is required")
			}

			cfg, err := config.LoadGoogleConfig(config.DefaultDir())
			if err != nil {
				return fmt.Errorf("loading Google config (run 'photo-copy config google' first): %w", err)
			}

			log := logging.New(debug, nil)
			ctx := context.Background()
			client, err := google.NewClient(ctx, cfg, config.DefaultDir(), log)
			if err != nil {
				return fmt.Errorf("creating Google Photos client: %w", err)
			}

			return client.Upload(ctx, inputDir)
		},
	}

	cmd.Flags().StringVar(&inputDir, "input-dir", "", "Directory containing photos to upload")
	cmd.MarkFlagRequired("input-dir")
	return cmd
}

func newGoogleImportTakeoutCmd() *cobra.Command {
	var takeoutDir, outputDir string

	cmd := &cobra.Command{
		Use:   "import-takeout",
		Short: "Import photos from Google Takeout zip files",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := logging.New(debug, nil)
			_, err := google.ImportTakeout(takeoutDir, outputDir, log)
			return err
		},
	}

	cmd.Flags().StringVar(&takeoutDir, "takeout-dir", "", "Directory containing Takeout zip files")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Directory to extract photos to")
	cmd.MarkFlagRequired("takeout-dir")
	cmd.MarkFlagRequired("output-dir")
	return cmd
}
