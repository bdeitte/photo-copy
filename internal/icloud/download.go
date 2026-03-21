package icloud

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/briandeitte/photo-copy/internal/daterange"
	"github.com/briandeitte/photo-copy/internal/transfer"
)

func (c *Client) Download(ctx context.Context, outputDir string, limit int, dateRange *daterange.DateRange) (*transfer.Result, error) {
	result := transfer.NewResult("icloud", "download", outputDir)

	icloudpdPath, err := findTool("icloudpd", "PHOTO_COPY_ICLOUDPD_PATH")
	if err != nil {
		return result, err
	}

	args := buildDownloadArgs(outputDir, c.cfg.AppleID, c.cfg.CookieDir, limit, dateRange, c.log.IsDebug())
	c.log.Debug("running: %s %s", icloudpdPath, strings.Join(args, " "))

	cmd := exec.CommandContext(ctx, icloudpdPath, args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return result, fmt.Errorf("creating stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return result, fmt.Errorf("starting icloudpd: %w", err)
	}

	estimator := transfer.NewEstimator()
	downloaded := 0
	total := 0
	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		if total == 0 {
			if n := parsePhotoCount(line); n > 0 {
				total = n
				c.log.Info("found %d photos in iCloud library", total)
			}
		}

		if filename := parseDownloadLine(line); filename != "" {
			downloaded++
			estimator.Tick()
			result.RecordSuccess(filename, 0)
			if total > 0 {
				remaining := total - downloaded
				c.log.Info("[%d/%d] %sdownloaded %s", downloaded, total, estimator.Estimate(remaining), filename)
			} else {
				c.log.Info("[%d] %sdownloaded %s", downloaded, estimator.Estimate(0), filename)
			}
			continue
		}

		if isSkipLine(line) {
			result.RecordSkip(1)
			c.log.Debug("icloudpd: %s", line)
			continue
		}

		if filename, reason := parseDownloadError(line); reason != "" {
			c.log.Error("icloudpd: %s", line)
			result.RecordError(filename, reason)
			continue
		}

		c.log.Debug("icloudpd: %s", line)
	}

	if err := scanner.Err(); err != nil {
		_ = cmd.Wait()
		return result, fmt.Errorf("reading icloudpd output: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		return result, fmt.Errorf("icloudpd failed: %w (if authentication expired, run 'photo-copy config icloud' to re-authenticate)", err)
	}

	result.Finish()
	return result, nil
}

func buildDownloadArgs(outputDir, appleID, cookieDir string, limit int, dateRange *daterange.DateRange, debug bool) []string {
	args := []string{
		"--directory", outputDir,
		"--username", appleID,
		"--cookie-directory", cookieDir,
		"--no-progress-bar",
	}

	if limit > 0 {
		args = append(args, "--recent", strconv.Itoa(limit))
	}

	if dateRange != nil {
		if dateRange.After != nil {
			args = append(args, "--from-date", dateRange.After.Format("2006-01-02"))
		}
		if dateRange.Before != nil {
			toDate := dateRange.Before.AddDate(0, 0, -1)
			args = append(args, "--to-date", toDate.Format("2006-01-02"))
		}
	}

	if debug {
		args = append(args, "--log-level", "debug")
	}

	return args
}

var downloadLineRe = regexp.MustCompile(`(?i)downloading\s+(.+)`)

func parseDownloadLine(line string) string {
	m := downloadLineRe.FindStringSubmatch(line)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1])
}

var photoCountRe = regexp.MustCompile(`(?i)found\s+(\d+)\s+(?:items?|photos?|assets?)`)

func parsePhotoCount(line string) int {
	m := photoCountRe.FindStringSubmatch(line)
	if m == nil {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return n
}

func isSkipLine(line string) bool {
	lower := strings.ToLower(line)
	return strings.Contains(lower, "already exists") || strings.Contains(lower, "skipping")
}

// downloadErrorRe matches icloudpd error lines like "ERROR downloading IMG_5678.jpg: reason".
var downloadErrorRe = regexp.MustCompile(`(?i)error\S*\s+(?:.*\s)?downloading\s+(\S+?)(?::\s*(.*))?$`)

func parseDownloadError(line string) (filename, reason string) {
	m := downloadErrorRe.FindStringSubmatch(line)
	if m == nil {
		return "", ""
	}
	filename = strings.TrimSpace(m[1])
	reason = line
	if len(m) > 2 && m[2] != "" {
		reason = m[2]
	}
	return filename, reason
}
