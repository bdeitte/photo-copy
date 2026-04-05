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
	"github.com/briandeitte/photo-copy/internal/daterange"
	"github.com/briandeitte/photo-copy/internal/logging"
	"github.com/briandeitte/photo-copy/internal/media"
	"github.com/briandeitte/photo-copy/internal/transfer"
)

type Client struct {
	cfg *config.S3Config
	log *logging.Logger
}

// rcloneSetup holds the paths resolved during shared rclone preparation.
type rcloneSetup struct {
	binaryPath string
	configPath string
}

// prepareRclone discovers the rclone binary and writes a temporary config file.
// The caller must remove configPath when done.
func (c *Client) prepareRclone() (*rcloneSetup, error) {
	binDir, err := rcloneBinDir()
	if err != nil {
		return nil, err
	}
	rclonePath, err := findRcloneBinary(binDir)
	if err != nil {
		return nil, err
	}
	configPath, err := writeRcloneConfig(c.cfg.AccessKeyID, c.cfg.SecretAccessKey, c.cfg.Region)
	if err != nil {
		return nil, err
	}
	return &rcloneSetup{binaryPath: rclonePath, configPath: configPath}, nil
}

// buildFilterArgs combines media include flags and date range flags.
func buildFilterArgs(mediaOnly bool, dr *daterange.DateRange) []string {
	var flags []string
	if mediaOnly {
		flags = append(flags, buildMediaIncludeFlags()...)
	}
	dateFlags := buildDateRangeFlags(dr)
	flags = append(flags, dateFlags...)
	return flags
}

func NewClient(cfg *config.S3Config, log *logging.Logger) *Client {
	return &Client{cfg: cfg, log: log}
}

func (c *Client) Upload(ctx context.Context, inputDir, bucket, prefix string, mediaOnly bool, limit int, dateRange *daterange.DateRange, storageClass string) (*transfer.Result, error) {
	result := transfer.NewResult("s3", "upload", inputDir)

	setup, err := c.prepareRclone()
	if err != nil {
		return result, err
	}
	defer func() { _ = os.Remove(setup.configPath) }()

	filterFlags := buildFilterArgs(mediaOnly, dateRange)
	dateFlags := buildDateRangeFlags(dateRange)

	args := buildUploadArgs(setup.configPath, inputDir, bucket, prefix, storageClass)
	args = append(args, filterFlags...)

	if limit > 0 {
		filesFromPath, err := c.buildFilesFrom(ctx, setup.binaryPath, setup.configPath, inputDir, args, limit)
		if err != nil {
			return result, fmt.Errorf("building file list for limit: %w", err)
		}
		if filesFromPath == "" {
			result.Finish()
			return result, nil
		}
		defer func() { _ = os.Remove(filesFromPath) }()
		// Replace include flags with --files-from (they're mutually exclusive in rclone)
		args = buildUploadArgs(setup.configPath, inputDir, bucket, prefix, storageClass)
		args = append(args, "--files-from", filesFromPath)
		if len(dateFlags) > 0 {
			args = append(args, dateFlags...)
		}
		filterFlags = []string{"--files-from", filesFromPath}
	}

	c.log.Info("Counting local files...")
	total := c.countFiles(ctx, setup.binaryPath, setup.configPath, inputDir, filterFlags)
	if total > 0 {
		c.log.Info("Found %d files. Comparing with S3 destination (this may take a while)...", total)
	} else {
		c.log.Info("Comparing with S3 destination (this may take a while)...")
	}
	c.log.Debug("running: %s %s", setup.binaryPath, strings.Join(args, " "))
	_, err = c.runRcloneWithProgress(ctx, setup.binaryPath, args, total, "uploaded", result)
	result.Finish()
	return result, err
}

func (c *Client) Download(ctx context.Context, bucket, prefix, outputDir string, mediaOnly bool, limit int, dateRange *daterange.DateRange) (*transfer.Result, error) {
	result := transfer.NewResult("s3", "download", outputDir)

	setup, err := c.prepareRclone()
	if err != nil {
		return result, err
	}
	defer func() { _ = os.Remove(setup.configPath) }()

	src := "s3:" + bucket
	if prefix != "" {
		src += "/" + prefix
	}

	filterFlags := buildFilterArgs(mediaOnly, dateRange)
	dateFlags := buildDateRangeFlags(dateRange)

	args := buildDownloadArgs(setup.configPath, bucket, prefix, outputDir)
	args = append(args, filterFlags...)

	if limit > 0 {
		filesFromPath, err := c.buildFilesFrom(ctx, setup.binaryPath, setup.configPath, src, args, limit)
		if err != nil {
			return result, fmt.Errorf("building file list for limit: %w", err)
		}
		if filesFromPath == "" {
			result.Finish()
			return result, nil
		}
		defer func() { _ = os.Remove(filesFromPath) }()
		// Replace include flags with --files-from (they're mutually exclusive in rclone)
		args = buildDownloadArgs(setup.configPath, bucket, prefix, outputDir)
		args = append(args, "--files-from", filesFromPath)
		if len(dateFlags) > 0 {
			args = append(args, dateFlags...)
		}
		filterFlags = []string{"--files-from", filesFromPath}
	}

	// Detect and restore Glacier objects
	glacierFiles := detectGlacierFiles(ctx, setup.binaryPath, setup.configPath, src, filterFlags)
	if len(glacierFiles) > 0 {
		needRestore := filterOutExisting(glacierFiles, outputDir)
		if len(needRestore) > 0 {
			c.log.Info("Initiating Glacier restore for %d files (Bulk tier, ~5-12 hours)...", len(needRestore))
			if restoreErr := initiateRestore(ctx, setup.binaryPath, setup.configPath, src, filterFlags, c.log); restoreErr != nil {
				c.log.Error("restore request failed: %v", restoreErr)
				c.log.Info("Continuing with download — already-restored files will still be downloaded")
			}
		}
	}

	c.log.Info("Counting remote files...")
	total := c.countFiles(ctx, setup.binaryPath, setup.configPath, src, filterFlags)
	if total > 0 {
		c.log.Info("Found %d files. Comparing with local directory (this may take a while)...", total)
	} else {
		c.log.Info("Comparing with local directory (this may take a while)...")
	}
	c.log.Debug("running: %s %s", setup.binaryPath, strings.Join(args, " "))
	glacierPending, err := c.runRcloneWithProgress(ctx, setup.binaryPath, args, total, "downloaded", nil)
	result.Restoring = glacierPending
	if glacierPending > 0 {
		c.log.Info("%d files still restoring from Glacier — re-run this command in a few hours", glacierPending)
	}
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

func (c *Client) runRcloneWithProgress(ctx context.Context, rclonePath string, args []string, total int, verb string, result *transfer.Result) (glacierPending int, err error) {
	cmd := exec.CommandContext(ctx, rclonePath, args...)
	cmd.Stdout = os.Stdout

	stderr, pipeErr := cmd.StderrPipe()
	if pipeErr != nil {
		return 0, fmt.Errorf("creating stderr pipe: %w", pipeErr)
	}

	if startErr := cmd.Start(); startErr != nil {
		return 0, fmt.Errorf("starting rclone: %w", startErr)
	}

	estimator := transfer.NewEstimator()
	copied := 0
	var lastError string
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

		if entry.Level == "error" || entry.Level == "warning" {
			if isGlacierError(entry.Msg) {
				glacierPending++
				c.log.Debug("glacier restore pending: %s", entry.Object)
				continue
			}
			c.log.Error("rclone: %s", entry.Msg)
			lastError = entry.Msg
			continue
		}

		if !strings.HasPrefix(entry.Msg, "Copied") {
			c.log.Debug("rclone: %s", entry.Msg)
			continue
		}

		copied++
		estimator.Tick()
		if result != nil {
			result.RecordSuccess(0)
		}
		filename := filepath.Base(entry.Object)
		if total > 0 {
			remaining := total - copied
			c.log.Info("[%d/%d] %s%s %s", copied, total, estimator.Estimate(remaining), verb, filename)
		} else {
			c.log.Info("[%d] %s%s %s", copied, estimator.Estimate(0), verb, filename)
		}
	}

	if scanErr := scanner.Err(); scanErr != nil {
		_ = cmd.Wait()
		return 0, fmt.Errorf("reading rclone output: %w", scanErr)
	}

	if waitErr := cmd.Wait(); waitErr != nil {
		if glacierPending > 0 && lastError == "" {
			// All errors were glacier-related — not a real failure
			return glacierPending, nil
		}
		if lastError != "" {
			return glacierPending, fmt.Errorf("rclone failed (%w): %s", waitErr, lastError)
		}
		return glacierPending, fmt.Errorf("rclone failed: %w", waitErr)
	}
	return glacierPending, nil
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

func buildUploadArgs(configPath, inputDir, bucket, prefix, storageClass string) []string {
	dest := "s3:" + bucket
	if prefix != "" {
		dest += "/" + prefix
	}

	args := []string{
		"copy", inputDir, dest,
		"--config", configPath,
		"-v", "--use-json-log", "--stats", "0",
	}
	if storageClass != "" {
		args = append(args, "--s3-storage-class", storageClass)
	}
	return args
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
	// Build lsf args, carrying over any --include and --min-age/--max-age filters from the copy args
	lsfArgs := []string{"lsf", source, "--config", configPath, "--files-only", "-R"}
	for i := 0; i < len(copyArgs); i++ {
		switch copyArgs[i] {
		case "--include":
			if i+1 < len(copyArgs) {
				lsfArgs = append(lsfArgs, "--include", copyArgs[i+1])
				i++
			}
		case "--min-age", "--max-age":
			if i+1 < len(copyArgs) {
				lsfArgs = append(lsfArgs, copyArgs[i], copyArgs[i+1])
				i++
			}
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

// buildDateRangeFlags converts a DateRange into rclone --min-age/--max-age flags.
func buildDateRangeFlags(dr *daterange.DateRange) []string {
	if dr == nil {
		return nil
	}
	var flags []string
	if dr.After != nil {
		// --max-age = "files no older than" = our After bound
		flags = append(flags, "--max-age", dr.After.Format("2006-01-02"))
	}
	if dr.Before != nil {
		// --min-age = "files at least this old" = our Before bound
		// dr.Before is already start of next day (exclusive), which is exactly what
		// rclone needs: --min-age 2024-01-01 includes files from 2023-12-31
		flags = append(flags, "--min-age", dr.Before.Format("2006-01-02"))
	}
	return flags
}

func buildMediaIncludeFlags() []string {
	// --ignore-case is a global rclone flag that applies to all --include patterns.
	flags := []string{"--ignore-case"}
	for _, ext := range media.SupportedExtensions() {
		flags = append(flags, "--include", "*"+ext)
	}
	return flags
}
