package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/google"
	"github.com/briandeitte/photo-copy/internal/logging"
	"github.com/spf13/cobra"
)

func newGooglePhotosCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "google",
		Short: "Google Photos upload and Takeout import commands",
	}

	cmd.AddCommand(newGoogleUploadCmd())
	cmd.AddCommand(newGoogleImportTakeoutCmd())
	return cmd
}

func newGoogleUploadCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upload <input-dir>",
		Short: "Upload photos to Google Photos",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			inputDir := args[0]

			cfg, err := config.LoadGoogleConfig(config.DefaultDir())
			if err != nil {
				if errors.Is(err, config.ErrNotConfigured) {
					return fmt.Errorf("Google credentials not configured. Run 'photo-copy config google' to set up")
				}
				return fmt.Errorf("loading Google config: %w", err)
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

	return cmd
}

func newGoogleImportTakeoutCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import-takeout <takeout-dir> <output-dir>",
		Short: "Import photos from Google Takeout zip files",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			log := logging.New(debug, nil)
			_, err := google.ImportTakeout(args[0], args[1], log)
			return err
		},
	}

	return cmd
}
