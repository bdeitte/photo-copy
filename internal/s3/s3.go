package s3

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/logging"
)

type Client struct {
	cfg *config.S3Config
	log *logging.Logger
}

func NewClient(cfg *config.S3Config, log *logging.Logger) *Client {
	return &Client{cfg: cfg, log: log}
}

func (c *Client) Upload(ctx context.Context, inputDir, bucket, prefix string, mediaOnly bool) error {
	binDir, err := rcloneBinDir()
	if err != nil {
		return err
	}

	rclonePath, err := findRcloneBinary(binDir)
	if err != nil {
		return err
	}

	configPath, err := writeRcloneConfig(c.cfg.AccessKeyID, c.cfg.SecretAccessKey, c.cfg.Region)
	if err != nil {
		return err
	}
	defer os.Remove(configPath)

	args := buildUploadArgs(configPath, inputDir, bucket, prefix)
	if mediaOnly {
		args = append(args, buildMediaIncludeFlags()...)
	}

	c.log.Debug("running: %s %s", rclonePath, strings.Join(args, " "))
	return c.runRclone(ctx, rclonePath, args)
}

func (c *Client) Download(ctx context.Context, bucket, prefix, outputDir string, mediaOnly bool) error {
	binDir, err := rcloneBinDir()
	if err != nil {
		return err
	}

	rclonePath, err := findRcloneBinary(binDir)
	if err != nil {
		return err
	}

	configPath, err := writeRcloneConfig(c.cfg.AccessKeyID, c.cfg.SecretAccessKey, c.cfg.Region)
	if err != nil {
		return err
	}
	defer os.Remove(configPath)

	args := buildDownloadArgs(configPath, bucket, prefix, outputDir)
	if mediaOnly {
		args = append(args, buildMediaIncludeFlags()...)
	}

	c.log.Debug("running: %s %s", rclonePath, strings.Join(args, " "))
	return c.runRclone(ctx, rclonePath, args)
}

func (c *Client) runRclone(ctx context.Context, rclonePath string, args []string) error {
	cmd := exec.CommandContext(ctx, rclonePath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rclone failed: %w", err)
	}
	return nil
}

func buildUploadArgs(configPath, inputDir, bucket, prefix string) []string {
	dest := "s3:" + bucket
	if prefix != "" {
		dest += "/" + prefix
	}

	return []string{
		"copy", inputDir, dest,
		"--config", configPath,
		"--progress",
	}
}

func buildDownloadArgs(configPath, bucket, prefix, outputDir string) []string {
	src := "s3:" + bucket
	if prefix != "" {
		src += "/" + prefix
	}

	return []string{
		"copy", src, outputDir,
		"--config", configPath,
		"--progress",
	}
}

func buildMediaIncludeFlags() []string {
	extensions := []string{
		"*.jpg", "*.jpeg", "*.png", "*.tiff", "*.tif", "*.gif",
		"*.heic", "*.webp", "*.mp4", "*.mov", "*.avi", "*.mkv",
		"*.JPG", "*.JPEG", "*.PNG", "*.TIFF", "*.TIF", "*.GIF",
		"*.HEIC", "*.WEBP", "*.MP4", "*.MOV", "*.AVI", "*.MKV",
	}

	var flags []string
	for _, ext := range extensions {
		flags = append(flags, "--include", ext)
	}
	return flags
}
