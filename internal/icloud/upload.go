package icloud

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/briandeitte/photo-copy/internal/daterange"
	"github.com/briandeitte/photo-copy/internal/media"
	"github.com/briandeitte/photo-copy/internal/mediadate"
	"github.com/briandeitte/photo-copy/internal/transfer"
)

// Upload imports photos into Photos.app via osxphotos. macOS only.
func (c *Client) Upload(ctx context.Context, inputDir string, limit int, dateRange *daterange.DateRange) (*transfer.Result, error) {
	result := transfer.NewResult("icloud", "upload", inputDir)

	if runtime.GOOS != "darwin" {
		return result, fmt.Errorf("iCloud upload requires macOS with Photos.app and iCloud Photos sync enabled")
	}

	osxphotosPath, err := findTool("osxphotos", "PHOTO_COPY_OSXPHOTOS_PATH")
	if err != nil {
		return result, err
	}

	files, err := collectFiles(inputDir, limit, dateRange)
	if err != nil {
		return result, fmt.Errorf("scanning files: %w", err)
	}

	if len(files) == 0 {
		c.log.Info("no files found to upload")
		result.Finish()
		return result, nil
	}

	c.log.Info("found %d files to import into Photos.app", len(files))

	args := buildUploadArgs(files, c.log.IsDebug())
	c.log.Debug("running: %s %s", osxphotosPath, strings.Join(args, " "))

	cmd := exec.CommandContext(ctx, osxphotosPath, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return result, fmt.Errorf("creating stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return result, fmt.Errorf("starting osxphotos: %w", err)
	}

	estimator := transfer.NewEstimator()
	imported := 0
	total := len(files)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		if filename := parseImportLine(line); filename != "" {
			imported++
			estimator.Tick()
			remaining := total - imported
			c.log.Info("[%d/%d] %suploaded %s", imported, total, estimator.Estimate(remaining), filename)
			result.RecordSuccess(filename, 0)
			continue
		}

		if filename, reason := parseImportError(line); reason != "" {
			c.log.Error("osxphotos: %s", line)
			result.RecordError(filename, reason)
			continue
		}

		c.log.Debug("osxphotos: %s", line)
	}

	if err := scanner.Err(); err != nil {
		_ = cmd.Wait()
		return result, fmt.Errorf("reading osxphotos output: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		result.Finish()
		return result, fmt.Errorf("osxphotos failed: %w", err)
	}

	result.Finish()
	return result, nil
}

func buildUploadArgs(files []string, debug bool) []string {
	args := []string{"import"}
	args = append(args, files...)

	if debug {
		args = append(args, "--verbose")
	}

	return args
}

// collectFiles walks inputDir and returns paths of supported media files,
// applying limit and date-range filters.
func collectFiles(inputDir string, limit int, dateRange *daterange.DateRange) ([]string, error) {
	var files []string

	err := filepath.WalkDir(inputDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !media.IsSupportedFile(path) {
			return nil
		}

		if dateRange != nil {
			fileDate := mediadate.ResolveDate(path)
			if !fileDate.IsZero() && !dateRange.Contains(fileDate) {
				return nil
			}
		}

		files = append(files, path)

		if limit > 0 && len(files) >= limit {
			return filepath.SkipAll
		}
		return nil
	})

	return files, err
}

// parseImportLine extracts a filename from osxphotos import output.
func parseImportLine(line string) string {
	lower := strings.ToLower(line)
	if strings.Contains(lower, "imported") || strings.Contains(lower, "importing") {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			return filepath.Base(parts[len(parts)-1])
		}
	}
	return ""
}

// importErrorRe matches osxphotos error lines like "Error importing /path/to/file: reason"
// or "Failed to import /path/to/file: reason".
var importErrorRe = regexp.MustCompile(`(?i)(?:error|failed)\s+(?:importing|to import)\s+([^\s:]+)(?::\s*(.*))?`)

func parseImportError(line string) (filename, reason string) {
	m := importErrorRe.FindStringSubmatch(line)
	if m == nil {
		return "", ""
	}
	filename = filepath.Base(m[1])
	reason = line
	if len(m) > 2 && m[2] != "" {
		reason = m[2]
	}
	return filename, reason
}
