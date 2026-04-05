package s3

import (
	"context"
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
		parts := strings.SplitN(line, ";", 2)
		if len(parts) != 2 {
			continue
		}
		class := strings.TrimSpace(parts[1])
		if class == "GLACIER" || class == "DEEP_ARCHIVE" {
			glacier = append(glacier, parts[0])
		}
	}
	return glacier
}

// filterOutExisting removes files that already exist in the output directory.
// This avoids initiating restore for files already downloaded.
func filterOutExisting(files []string, outputDir string) []string {
	var missing []string
	for _, f := range files {
		if _, err := os.Stat(filepath.Join(outputDir, f)); err != nil {
			missing = append(missing, f)
		}
	}
	return missing
}

// detectGlacierFiles runs "rclone lsf --format pT" to identify objects
// in GLACIER or DEEP_ARCHIVE storage classes. Returns their relative paths.
// Returns nil on error (Glacier detection is best-effort).
func detectGlacierFiles(ctx context.Context, rclonePath, configPath, source string, filterFlags []string) []string {
	args := []string{"lsf", source, "--config", configPath, "--files-only", "-R", "--format", "pT"}
	args = append(args, filterFlags...)

	cmd := exec.CommandContext(ctx, rclonePath, args...)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	return parseStorageClasses(string(out))
}

// initiateRestore calls "rclone backend restore" to begin Glacier restore
// for objects at the given source path. Uses Bulk tier and 7-day lifetime.
func initiateRestore(ctx context.Context, rclonePath, configPath, source string, filterFlags []string, log *logging.Logger) error {
	args := []string{"backend", "restore", source, "--config", configPath, "-o", "priority=Bulk", "-o", "lifetime=7"}
	args = append(args, filterFlags...)

	log.Debug("initiating restore: %s %s", rclonePath, strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, rclonePath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rclone backend restore failed: %w\n%s", err, string(out))
	}
	return nil
}
