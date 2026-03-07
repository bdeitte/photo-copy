package cli

import (
	"context"
	"fmt"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/logging"
	"github.com/briandeitte/photo-copy/internal/s3"
	"github.com/spf13/cobra"
)

func newS3Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "s3",
		Short: "S3 upload and download commands",
	}

	cmd.AddCommand(newS3UploadCmd())
	cmd.AddCommand(newS3DownloadCmd())
	return cmd
}

func newS3UploadCmd() *cobra.Command {
	var bucket, prefix string

	cmd := &cobra.Command{
		Use:   "upload <input-dir>",
		Short: "Upload photos to S3",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadS3Config(config.DefaultDir())
			if err != nil {
				return fmt.Errorf("loading S3 config (run 'photo-copy config s3' first): %w", err)
			}

			log := logging.New(debug, nil)
			client := s3.NewClient(cfg, log)
			return client.Upload(context.Background(), args[0], bucket, prefix, true)
		},
	}

	cmd.Flags().StringVar(&bucket, "bucket", "", "S3 bucket name")
	cmd.Flags().StringVar(&prefix, "prefix", "", "S3 key prefix (optional)")
	cmd.MarkFlagRequired("bucket")
	return cmd
}

func newS3DownloadCmd() *cobra.Command {
	var bucket, prefix string

	cmd := &cobra.Command{
		Use:   "download <output-dir>",
		Short: "Download photos from S3",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadS3Config(config.DefaultDir())
			if err != nil {
				return fmt.Errorf("loading S3 config (run 'photo-copy config s3' first): %w", err)
			}

			log := logging.New(debug, nil)
			client := s3.NewClient(cfg, log)
			return client.Download(context.Background(), bucket, prefix, args[0], true)
		},
	}

	cmd.Flags().StringVar(&bucket, "bucket", "", "S3 bucket name")
	cmd.Flags().StringVar(&prefix, "prefix", "", "S3 key prefix (optional)")
	cmd.MarkFlagRequired("bucket")
	return cmd
}
