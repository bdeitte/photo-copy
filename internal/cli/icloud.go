package cli

import (
	"errors"
	"fmt"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/icloud"
	"github.com/briandeitte/photo-copy/internal/logging"
	"github.com/briandeitte/photo-copy/internal/transfer"
	"github.com/spf13/cobra"
)

func newICloudCmd(opts *rootOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "icloud",
		Short: "iCloud Photos upload and download commands",
	}

	cmd.AddCommand(newICloudDownloadCmd(opts))
	cmd.AddCommand(newICloudUploadCmd(opts))
	return cmd
}

func newICloudDownloadClient(opts *rootOpts) (*icloud.Client, *logging.Logger, error) {
	cfg, err := config.LoadICloudConfig(config.DefaultDir())
	if err != nil {
		if errors.Is(err, config.ErrNotConfigured) {
			return nil, nil, fmt.Errorf("iCloud credentials not configured. Run 'photo-copy config icloud' to set up")
		}
		return nil, nil, fmt.Errorf("loading iCloud config: %w", err)
	}
	log := logging.New(opts.debug, nil)
	return icloud.NewClient(cfg, log), log, nil
}

func newICloudUploadClient(opts *rootOpts) (*icloud.Client, *logging.Logger) {
	log := logging.New(opts.debug, nil)
	cfg := &config.ICloudConfig{}
	if loaded, err := config.LoadICloudConfig(config.DefaultDir()); err == nil {
		cfg = loaded
	}
	return icloud.NewClient(cfg, log), log
}

func newICloudDownloadCmd(opts *rootOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "download <output-dir>",
		Short:       "Download photos/videos from iCloud Photos",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"supportsDateRange": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, log, err := newICloudDownloadClient(opts)
			if err != nil {
				return err
			}

			result, err := client.Download(cmd.Context(), args[0], opts.limit, opts.parsedDateRange)
			if err != nil {
				return err
			}
			transfer.HandleResult(result, log, args[0])
			return nil
		},
	}

	return cmd
}

func newICloudUploadCmd(opts *rootOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "upload <input-dir>",
		Short:       "Import photos/videos into Photos.app (macOS only, syncs to iCloud)",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"supportsDateRange": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, log := newICloudUploadClient(opts)

			result, err := client.Upload(cmd.Context(), args[0], opts.limit, opts.parsedDateRange)
			if err != nil {
				return err
			}
			transfer.HandleResult(result, log, args[0])
			return nil
		},
	}

	return cmd
}
