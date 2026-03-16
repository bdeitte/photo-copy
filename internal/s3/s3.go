package s3

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/logging"
	"github.com/briandeitte/photo-copy/internal/media"
	"github.com/briandeitte/photo-copy/internal/transfer"
)

type Client struct {
	cfg *config.S3Config
	log *logging.Logger
}

func NewClient(cfg *config.S3Config, log *logging.Logger) *Client {
	return &Client{cfg: cfg, log: log}
}

func (c *Client) Upload(ctx context.Context, inputDir, bucket, prefix string, mediaOnly bool, limit int) (*transfer.Result, error) {
	result := transfer.NewResult("s3", "upload", inputDir)

	binDir, err := rcloneBinDir()
	if err != nil {
		return result, err
	}

	rclonePath, err := findRcloneBinary(binDir)
	if err != nil {
		return result, err
	}

	configPath, err := writeRcloneConfig(c.cfg.AccessKeyID, c.cfg.SecretAccessKey, c.cfg.Region)
	if err != nil {
		return result, err
	}
	defer func() { _ = os.Remove(configPath) }()

	args := buildUploadArgs(configPath, inputDir, bucket, prefix)
	var filterFlags []string
	if mediaOnly {
		filterFlags = buildMediaIncludeFlags()
		args = append(args, filterFlags...)
	}

	if limit > 0 {
		filesFromPath, err := c.buildFilesFrom(ctx, rclonePath, configPath, inputDir, args, limit)
		if err != nil {
			return result, fmt.Errorf("building file list for limit: %w", err)
		}
		if filesFromPath == "" {
			result.Finish()
			return result, nil
		}
		defer func() { _ = os.Remove(filesFromPath) }()
		// Replace include flags with --files-from (they're mutually exclusive in rclone)
		args = buildUploadArgs(configPath, inputDir, bucket, prefix)
		args = append(args, "--files-from", filesFromPath)
		filterFlags = []string{"--files-from", filesFromPath}
	}

	total := c.countFiles(ctx, rclonePath, configPath, inputDir, filterFlags)
	c.log.Debug("running: %s %s", rclonePath, strings.Join(args, " "))
	err = c.runRcloneWithProgress(ctx, rclonePath, args, total, "uploaded")
	result.Finish()
	return result, err
}

func (c *Client) Download(ctx context.Context, bucket, prefix, outputDir string, mediaOnly bool, limit int) (*transfer.Result, error) {
	result := transfer.NewResult("s3", "download", outputDir)

	binDir, err := rcloneBinDir()
	if err != nil {
		return result, err
	}

	rclonePath, err := findRcloneBinary(binDir)
	if err != nil {
		return result, err
	}

	configPath, err := writeRcloneConfig(c.cfg.AccessKeyID, c.cfg.SecretAccessKey, c.cfg.Region)
	if err != nil {
		return result, err
	}
	defer func() { _ = os.Remove(configPath) }()

	src := "s3:" + bucket
	if prefix != "" {
		src += "/" + prefix
	}

	args := buildDownloadArgs(configPath, bucket, prefix, outputDir)
	var filterFlags []string
	if mediaOnly {
		filterFlags = buildMediaIncludeFlags()
		args = append(args, filterFlags...)
	}

	if limit > 0 {
		filesFromPath, err := c.buildFilesFrom(ctx, rclonePath, configPath, src, args, limit)
		if err != nil {
			return result, fmt.Errorf("building file list for limit: %w", err)
		}
		if filesFromPath == "" {
			result.Finish()
			return result, nil
		}
		defer func() { _ = os.Remove(filesFromPath) }()
		// Replace include flags with --files-from (they're mutually exclusive in rclone)
		args = buildDownloadArgs(configPath, bucket, prefix, outputDir)
		args = append(args, "--files-from", filesFromPath)
		filterFlags = []string{"--files-from", filesFromPath}
	}

	total := c.countFiles(ctx, rclonePath, configPath, src, filterFlags)
	c.log.Debug("running: %s %s", rclonePath, strings.Join(args, " "))
	err = c.runRcloneWithProgress(ctx, rclonePath, args, total, "downloaded")
	result.Finish()
	if scanErr := result.ScanDir(); scanErr != nil {
		c.log.Debug("scanning directory: %v", scanErr)
	}
	return result, err
}

// rcloneLogEntry represents a single JSON log line from rclone --use-json-log.
type rcloneLogEntry struct {
	Level  string `json:"level"`
	Msg    string `json:"msg"`
	Object string `json:"object"`
}

func (c *Client) runRcloneWithProgress(ctx context.Context, rclonePath string, args []string, total int, verb string) error {
	cmd := exec.CommandContext(ctx, rclonePath, args...)
	cmd.Stdout = os.Stdout

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("creating stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting rclone: %w", err)
	}

	estimator := transfer.NewEstimator()
	copied := 0
	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), 1024*1024) // 1MB max line
	for scanner.Scan() {
		line := scanner.Text()

		var entry rcloneLogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			// Not JSON — pass through as-is
			c.log.Debug("rclone: %s", line)
			continue
		}

		if entry.Level == "error" {
			c.log.Error("rclone: %s", entry.Msg)
			continue
		}

		if !strings.HasPrefix(entry.Msg, "Copied") {
			c.log.Debug("rclone: %s", entry.Msg)
			continue
		}

		copied++
		estimator.Tick()
		filename := filepath.Base(entry.Object)
		if total > 0 {
			remaining := total - copied
			c.log.Info("[%d/%d] %s%s %s", copied, total, estimator.Estimate(remaining), verb, filename)
		} else {
			c.log.Info("[%d] %s%s %s", copied, estimator.Estimate(0), verb, filename)
		}
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("rclone failed: %w", err)
	}
	return nil
}

// countFiles counts source files for progress tracking.
// When a files-from path is present in the args, it counts lines in that file directly.
// Otherwise it runs "rclone lsf" with the appropriate filter flags.
// Returns 0 on error (progress will show [X] instead of [X/Total]).
func (c *Client) countFiles(ctx context.Context, rclonePath, configPath, source string, filterFlags []string) int {
	// If we have a --files-from file, count its lines directly
	for i, f := range filterFlags {
		if f == "--files-from" && i+1 < len(filterFlags) {
			return countLinesInFile(filterFlags[i+1])
		}
	}

	lsfArgs := []string{"lsf", source, "--config", configPath, "--files-only", "-R"}
	lsfArgs = append(lsfArgs, filterFlags...)

	c.log.Debug("counting files: %s %s", rclonePath, strings.Join(lsfArgs, " "))
	cmd := exec.CommandContext(ctx, rclonePath, lsfArgs...)
	out, err := cmd.Output()
	if err != nil {
		c.log.Debug("counting files failed: %v", err)
		return 0
	}

	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return 0
	}
	return len(strings.Split(trimmed, "\n"))
}

// countLinesInFile counts non-empty lines in a file.
func countLinesInFile(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line != "" {
			count++
		}
	}
	return count
}

func buildUploadArgs(configPath, inputDir, bucket, prefix string) []string {
	dest := "s3:" + bucket
	if prefix != "" {
		dest += "/" + prefix
	}

	return []string{
		"copy", inputDir, dest,
		"--config", configPath,
		"-v", "--use-json-log", "--stats", "0",
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
		"-v", "--use-json-log", "--stats", "0",
	}
}

// buildFilesFrom runs "rclone lsf" on the source to list files, takes the first `limit` entries,
// writes them to a temp file, and returns the path. The caller must remove the file when done.
func (c *Client) buildFilesFrom(ctx context.Context, rclonePath, configPath, source string, copyArgs []string, limit int) (_ string, err error) {
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

	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		c.log.Info("no files found to transfer")
		return "", nil
	}
	lines := strings.Split(trimmed, "\n")
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
	if cerr := f.Close(); cerr != nil && err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}

	return f.Name(), nil
}

func buildMediaIncludeFlags() []string {
	// --ignore-case is a global rclone flag that applies to all --include patterns.
	flags := []string{"--ignore-case"}
	for _, ext := range media.SupportedExtensions() {
		flags = append(flags, "--include", "*"+ext)
	}
	return flags
}
