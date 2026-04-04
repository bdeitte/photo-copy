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

func newS3Client(opts *rootOpts) (*s3.Client, *logging.Logger, error) {
	cfg, err := config.LoadS3Config(config.DefaultDir())
	if err != nil {
		if errors.Is(err, config.ErrNotConfigured) {
			return nil, nil, fmt.Errorf("S3 credentials not configured. Run 'photo-copy config s3' to set up")
		}
		return nil, nil, fmt.Errorf("loading S3 config: %w", err)
	}
	log := logging.New(opts.debug, nil)
	return s3.NewClient(cfg, log), log, nil
}

func newS3UploadCmd(opts *rootOpts) *cobra.Command {
	var bucket, prefix string

	cmd := &cobra.Command{
		Use:         "upload <input-dir>",
		Short:       "Upload photos/videos to S3",
		Args:        exactArgs(1),
		Annotations: map[string]string{"supportsDateRange": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, log, err := newS3Client(opts)
			if err != nil {
				return err
			}

			result, err := client.Upload(cmd.Context(), args[0], bucket, prefix, true, opts.limit, opts.parsedDateRange)
			if err != nil {
				return err
			}
			transfer.HandleResult(result, log, args[0])
			return nil
		},
	}

	cmd.Flags().StringVar(&bucket, "bucket", "", "S3 bucket name")
	cmd.Flags().StringVar(&prefix, "prefix", "", "S3 key prefix (optional)")
	_ = cmd.MarkFlagRequired("bucket")
	return cmd
}

func newS3DownloadCmd(opts *rootOpts) *cobra.Command {
	var bucket, prefix string

	cmd := &cobra.Command{
		Use:         "download <output-dir>",
		Short:       "Download photos/videos from S3",
		Args:        exactArgs(1),
		Annotations: map[string]string{"supportsDateRange": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, log, err := newS3Client(opts)
			if err != nil {
				return err
			}

			result, err := client.Download(cmd.Context(), bucket, prefix, args[0], true, opts.limit, opts.parsedDateRange)
			if err != nil {
				return err
			}
			transfer.HandleResult(result, log, args[0])
			return nil
		},
	}

	cmd.Flags().StringVar(&bucket, "bucket", "", "S3 bucket name")
	cmd.Flags().StringVar(&prefix, "prefix", "", "S3 key prefix (optional)")
	_ = cmd.MarkFlagRequired("bucket")
	return cmd
}
