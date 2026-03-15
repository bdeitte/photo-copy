package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/google"
	"github.com/briandeitte/photo-copy/internal/logging"
	"github.com/briandeitte/photo-copy/internal/transfer"
	"github.com/spf13/cobra"
)

func newGooglePhotosCmd(opts *rootOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "google",
		Short: "Google Photos commands (upload via API, download via Takeout import)",
	}

	cmd.AddCommand(newGoogleUploadCmd(opts))
	cmd.AddCommand(newGoogleImportTakeoutCmd(opts))
	return cmd
}

func newGoogleUploadCmd(opts *rootOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upload <input-dir>",
		Short: "Upload photos to Google Photos",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			inputDir := args[0]

			cfg, err := config.LoadGoogleConfig(config.DefaultDir())
			if err != nil {
				if errors.Is(err, config.ErrNotConfigured) {
					return fmt.Errorf("Google credentials not configured. Run 'photo-copy config google' to set up (required for uploads only; to download, use Google Takeout with 'photo-copy google import-takeout')") //nolint:staticcheck // proper noun
				}
				return fmt.Errorf("loading Google config: %w", err)
			}

			log := logging.New(opts.debug, nil)
			ctx := context.Background()
			client, err := google.NewClient(ctx, cfg, config.DefaultDir(), log)
			if err != nil {
				return fmt.Errorf("creating Google Photos client: %w", err)
			}

			result, err := client.Upload(ctx, inputDir, opts.limit)
			transfer.HandleResult(result, log, inputDir)
			return err
		},
	}

	return cmd
}

func newGoogleImportTakeoutCmd(opts *rootOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import-takeout <takeout-dir> <output-dir>",
		Short: "Import photos from Google Takeout zip files",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			log := logging.New(opts.debug, nil)
			result, err := google.ImportTakeout(args[0], args[1], log)
			transfer.HandleResult(result, log, args[1])
			return err
		},
	}

	return cmd
}
