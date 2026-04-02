package cli

import (
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
		Short: "Google Photos commands (upload via API, download via Takeout)",
	}

	cmd.AddCommand(newGoogleUploadCmd(opts))
	cmd.AddCommand(newGoogleDownloadCmd(opts))
	return cmd
}

func newGoogleClient(cmd *cobra.Command, opts *rootOpts) (*google.Client, *logging.Logger, error) {
	cfg, err := config.LoadGoogleConfig(config.DefaultDir())
	if err != nil {
		if errors.Is(err, config.ErrNotConfigured) {
			return nil, nil, fmt.Errorf("Google credentials not configured. Run 'photo-copy config google' to set up (required for uploads only; to download, use Google Takeout with 'photo-copy google download')") //nolint:staticcheck // proper noun
		}
		return nil, nil, fmt.Errorf("loading Google config: %w", err)
	}
	log := logging.New(opts.debug, nil)
	client, err := google.NewClient(cmd.Context(), cfg, config.DefaultDir(), log)
	if err != nil {
		return nil, nil, fmt.Errorf("creating Google Photos client: %w", err)
	}
	return client, log, nil
}

func newGoogleUploadCmd(opts *rootOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "upload <input-dir>",
		Short:       "Upload photos/videos to Google Photos",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"supportsDateRange": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			inputDir := args[0]

			client, log, err := newGoogleClient(cmd, opts)
			if err != nil {
				return err
			}

			result, err := client.Upload(cmd.Context(), inputDir, opts.limit, opts.parsedDateRange)
			if err != nil {
				return err
			}
			transfer.HandleResult(result, log, inputDir)
			return nil
		},
	}

	return cmd
}

func newGoogleDownloadCmd(opts *rootOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "download <takeout-dir> <output-dir>",
		Short: "Download photos/videos from Google Takeout zip files",
		Args:  cobra.RangeArgs(0, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				//nolint:staticcheck // user-facing message, capitalization is intentional
				return fmt.Errorf(`Google Photos download requires Google Takeout zip files.

The Google Photos API only allows access to photos the app itself uploaded,
so downloading your full library requires Google Takeout (a manual zip export).

To get your photos:
  1. Go to https://takeout.google.com
  2. Deselect all, then select only "Google Photos"
  3. Choose your export format (zip) and file size
  4. Click "Create export" and wait for Google to prepare your files
  5. Download the zip files to a local directory

Then run:
  photo-copy google download <takeout-dir> <output-dir>

  <takeout-dir>  Directory containing the downloaded Takeout zip files
  <output-dir>   Directory where photos/videos will be extracted`)
			}
			log := logging.New(opts.debug, nil)
			result, err := google.ImportTakeout(args[0], args[1], log)
			if err != nil {
				return err
			}
			transfer.HandleResult(result, log, args[1])
			return nil
		},
	}

	return cmd
}
