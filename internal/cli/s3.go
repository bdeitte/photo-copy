package cli

import (
	"errors"
	"fmt"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/logging"
	"github.com/briandeitte/photo-copy/internal/s3"
	"github.com/briandeitte/photo-copy/internal/transfer"
	"github.com/spf13/cobra"
)

func newS3Cmd(opts *rootOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "s3",
		Short: "S3 upload and download commands",
	}

	cmd.AddCommand(newS3UploadCmd(opts))
	cmd.AddCommand(newS3DownloadCmd(opts))
	return cmd
}

func newS3UploadCmd(opts *rootOpts) *cobra.Command {
	var storageClass string

	cmd := &cobra.Command{
		Use:         "upload <input-dir> <s3-destination>",
		Short:       "Upload photos/videos to S3",
		Args:        exactArgs(2),
		Annotations: map[string]string{"supportsDateRange": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			bucket, prefix, urlRegion, err := parseS3Destination(args[1])
			if err != nil {
				return err
			}

			cfg, err := config.LoadS3Config(config.DefaultDir())
			if err != nil {
				if errors.Is(err, config.ErrNotConfigured) {
					return fmt.Errorf("S3 credentials not configured. Run 'photo-copy config s3' to set up")
				}
				return fmt.Errorf("loading S3 config: %w", err)
			}

			log := logging.New(opts.debug, nil)

			if urlRegion != "" && urlRegion != cfg.Region {
				log.Info("Using region %s from S3 URL (overrides configured region %s)", urlRegion, cfg.Region)
				cfg.Region = urlRegion
			}

			client := s3.NewClient(cfg, log)

			result, err := client.Upload(cmd.Context(), args[0], bucket, prefix, true, opts.limit, opts.parsedDateRange, storageClass)
			if err != nil {
				return err
			}
			transfer.HandleResult(result, log, args[0])
			return nil
		},
	}

	cmd.Flags().StringVar(&storageClass, "storage-class", "DEEP_ARCHIVE", "S3 storage class (e.g. STANDARD, GLACIER, DEEP_ARCHIVE)")
	return cmd
}

func newS3DownloadCmd(opts *rootOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "download <s3-destination> <output-dir>",
		Short:       "Download photos/videos from S3",
		Args:        exactArgs(2),
		Annotations: map[string]string{"supportsDateRange": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			bucket, prefix, urlRegion, err := parseS3Destination(args[0])
			if err != nil {
				return err
			}

			cfg, err := config.LoadS3Config(config.DefaultDir())
			if err != nil {
				if errors.Is(err, config.ErrNotConfigured) {
					return fmt.Errorf("S3 credentials not configured. Run 'photo-copy config s3' to set up")
				}
				return fmt.Errorf("loading S3 config: %w", err)
			}

			log := logging.New(opts.debug, nil)

			if urlRegion != "" && urlRegion != cfg.Region {
				log.Info("Using region %s from S3 URL (overrides configured region %s)", urlRegion, cfg.Region)
				cfg.Region = urlRegion
			}

			client := s3.NewClient(cfg, log)

			result, err := client.Download(cmd.Context(), bucket, prefix, args[1], true, opts.limit, opts.parsedDateRange)
			if err != nil {
				return err
			}
			transfer.HandleResult(result, log, args[1])
			return nil
		},
	}

	return cmd
}
