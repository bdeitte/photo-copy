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

func (c *Client) Upload(ctx context.Context, inputDir, bucket, prefix string, mediaOnly bool, limit int) error {
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

	if limit > 0 {
		filesFromPath, err := c.buildFilesFrom(ctx, rclonePath, configPath, inputDir, args, limit)
		if err != nil {
			return fmt.Errorf("building file list for limit: %w", err)
		}
		defer os.Remove(filesFromPath)
		// Replace include flags with --files-from (they're mutually exclusive in rclone)
		args = buildUploadArgs(configPath, inputDir, bucket, prefix)
		args = append(args, "--files-from", filesFromPath)
	}

	c.log.Debug("running: %s %s", rclonePath, strings.Join(args, " "))
	return c.runRclone(ctx, rclonePath, args)
}

func (c *Client) Download(ctx context.Context, bucket, prefix, outputDir string, mediaOnly bool, limit int) error {
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

	if limit > 0 {
		src := "s3:" + bucket
		if prefix != "" {
			src += "/" + prefix
		}
		filesFromPath, err := c.buildFilesFrom(ctx, rclonePath, configPath, src, args, limit)
		if err != nil {
			return fmt.Errorf("building file list for limit: %w", err)
		}
		defer os.Remove(filesFromPath)
		args = buildDownloadArgs(configPath, bucket, prefix, outputDir)
		args = append(args, "--files-from", filesFromPath)
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

// buildFilesFrom runs "rclone lsf" on the source to list files, takes the first `limit` entries,
// writes them to a temp file, and returns the path. The caller must remove the file when done.
func (c *Client) buildFilesFrom(ctx context.Context, rclonePath, configPath, source string, copyArgs []string, limit int) (string, error) {
	// Build lsf args, carrying over any --include filters from the copy args
	lsfArgs := []string{"lsf", source, "--config", configPath, "--files-only", "-R"}
	for i := 0; i < len(copyArgs); i++ {
		if copyArgs[i] == "--include" && i+1 < len(copyArgs) {
			lsfArgs = append(lsfArgs, "--include", copyArgs[i+1])
			i++
		}
	}

	c.log.Debug("listing files: %s %s", rclonePath, strings.Join(lsfArgs, " "))
	cmd := exec.CommandContext(ctx, rclonePath, lsfArgs...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("rclone lsf failed: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) > limit {
		c.log.Info("limiting transfer to %d of %d files", limit, len(lines))
		lines = lines[:limit]
	} else {
		c.log.Debug("found %d files (within limit of %d)", len(lines), limit)
	}

	f, err := os.CreateTemp("", "photo-copy-files-from-*.txt")
	if err != nil {
		return "", err
	}
	_, err = f.WriteString(strings.Join(lines, "\n") + "\n")
	f.Close()
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}

	return f.Name(), nil
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
