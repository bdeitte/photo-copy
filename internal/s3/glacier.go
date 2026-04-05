package s3

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/briandeitte/photo-copy/internal/logging"
)

// isGlacierError returns true if the rclone error message indicates
// the object is in Glacier and needs restore before download.
func isGlacierError(msg string) bool {
	return strings.Contains(msg, "GLACIER, restore first")
}

// parseStorageClasses parses "rclone lsf --format pT" output and returns
// the paths of objects in GLACIER or DEEP_ARCHIVE storage classes.
// Output format is "path;STORAGE_CLASS" per line.
func parseStorageClasses(output string) []string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return nil
	}
	var glacier []string
	for _, line := range strings.Split(trimmed, "\n") {
		// Split on the last semicolon — S3 keys can contain semicolons
		idx := strings.LastIndex(line, ";")
		if idx < 0 {
			continue
		}
		path := line[:idx]
		class := strings.TrimSpace(line[idx+1:])
		if class == "GLACIER" || class == "DEEP_ARCHIVE" {
			glacier = append(glacier, path)
		}
	}
	return glacier
}

// filterOutExisting removes files that already exist in the output directory.
// This avoids initiating restore for files already downloaded.
// Returns an error if a non-NotExist filesystem error is encountered.
func filterOutExisting(files []string, outputDir string) ([]string, error) {
	var missing []string
	for _, f := range files {
		_, err := os.Stat(filepath.Join(outputDir, f))
		if err == nil {
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("checking %s: %w", f, err)
		}
		missing = append(missing, f)
	}
	return missing, nil
}

// detectGlacierFiles runs "rclone lsf --format pT" to identify objects
// in GLACIER or DEEP_ARCHIVE storage classes. Returns their relative paths.
// Returns an error for context cancellation or deadline exceeded;
// other rclone listing errors return nil (best-effort detection).
func detectGlacierFiles(ctx context.Context, rclonePath, configPath, source string, filterFlags []string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	args := []string{"lsf", source, "--config", configPath, "--files-only", "-R", "--format", "pT"}
	args = append(args, filterFlags...)

	cmd := exec.CommandContext(ctx, rclonePath, args...)
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, nil
	}
	return parseStorageClasses(string(out)), nil
}

// initiateRestore calls "rclone backend restore" to begin Glacier restore
// for the specified files. Uses Bulk tier and 7-day lifetime.
// The files list contains relative paths that need restore.
func initiateRestore(ctx context.Context, rclonePath, configPath, source string, files []string, log *logging.Logger) error {
	// Write file list to a temp file for --files-from
	f, err := os.CreateTemp("", "photo-copy-restore-*.txt")
	if err != nil {
		return fmt.Errorf("creating restore file list: %w", err)
	}
	defer func() { _ = os.Remove(f.Name()) }()

	_, writeErr := f.WriteString(strings.Join(files, "\n") + "\n")
	if closeErr := f.Close(); closeErr != nil && writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		return fmt.Errorf("writing restore file list: %w", writeErr)
	}

	args := []string{"backend", "restore", source, "--config", configPath, "-o", "priority=Bulk", "-o", "lifetime=7", "--files-from", f.Name()}

	log.Debug("initiating restore: %s %s", rclonePath, strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, rclonePath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rclone backend restore failed: %w\n%s", err, string(out))
	}
	return nil
}
