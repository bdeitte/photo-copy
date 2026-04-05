package s3

import (
	"os"
	"path/filepath"
	"strings"
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
