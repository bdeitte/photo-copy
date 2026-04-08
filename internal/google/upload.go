package google

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/briandeitte/photo-copy/internal/daterange"
	"github.com/briandeitte/photo-copy/internal/media"
	"github.com/briandeitte/photo-copy/internal/mediadate"
	"github.com/briandeitte/photo-copy/internal/transfer"
)

const (
	defaultUploadURL      = "https://photoslibrary.googleapis.com/v1/uploads"
	defaultBatchCreateURL = "https://photoslibrary.googleapis.com/v1/mediaItems:batchCreate"
	dailyLimit            = 10000
)

// uploadURL returns the Google Photos upload endpoint URL.
// Checks the struct field first, then env var, then default.
func (c *Client) uploadURL() string {
	if c.apiBaseURL != "" {
		return c.apiBaseURL + "/v1/uploads"
	}
	if base := os.Getenv("PHOTO_COPY_GOOGLE_API_URL"); base != "" {
		return base + "/v1/uploads"
	}
	return defaultUploadURL
}

// batchCreateURL returns the Google Photos batch create endpoint URL.
// Checks the struct field first, then env var, then default.
func (c *Client) batchCreateURL() string {
	if c.apiBaseURL != "" {
		return c.apiBaseURL + "/v1/mediaItems:batchCreate"
	}
	if base := os.Getenv("PHOTO_COPY_GOOGLE_API_URL"); base != "" {
		return base + "/v1/mediaItems:batchCreate"
	}
	return defaultBatchCreateURL
}

// Upload uploads all media files from inputDir to Google Photos.
func (c *Client) Upload(ctx context.Context, inputDir string, limit int, dateRange *daterange.DateRange) (*transfer.Result, error) {
	result := transfer.NewResult("google-photos", "upload", inputDir)

	files, err := collectMediaFiles(inputDir)
	if err != nil {
		return result, fmt.Errorf("collecting media files: %w", err)
	}

	c.log.Info("found %d media files in %s", len(files), inputDir)

	logPath := filepath.Join(inputDir, ".photo-copy-upload.log")
	uploaded, err := loadUploadLog(logPath)
	if err != nil {
		return result, fmt.Errorf("loading upload log: %w", err)
	}

	// Filter already uploaded files
	var toUpload []string
	for _, f := range files {
		if !uploaded[f] {
			toUpload = append(toUpload, f)
		}
	}

	// Filter by date range
	if dateRange != nil {
		var filtered []string
		dateFiltered := 0
		for _, relPath := range toUpload {
			fullPath := filepath.Join(inputDir, relPath)
			fileDate := mediadate.ResolveDate(fullPath)
			switch {
			case fileDate.IsZero():
				c.log.Info("including %s despite date range filter: no date available", filepath.Base(relPath))
				filtered = append(filtered, relPath)
			case dateRange.Contains(fileDate):
				filtered = append(filtered, relPath)
			default:
				dateFiltered++
				c.log.Debug("skipping %s: date %s outside range", filepath.Base(relPath), fileDate.Format("2006-01-02"))
			}
		}
		if dateFiltered > 0 {
			c.log.Info("filtered %d files outside date range", dateFiltered)
			result.RecordSkip(dateFiltered)
		}
		toUpload = filtered
	}

	if len(toUpload) == 0 {
		c.log.Info("all files already uploaded")
		result.Expected = len(uploaded)
		result.RecordSkip(len(uploaded))
		result.Finish()
		return result, nil
	}

	if len(toUpload) > dailyLimit {
		c.log.Info("limiting upload to %d files (daily limit)", dailyLimit)
		toUpload = toUpload[:dailyLimit]
	}

	if limit > 0 && len(toUpload) > limit {
		c.log.Info("limiting upload to %d files (--limit flag)", limit)
		toUpload = toUpload[:limit]
	}

	result.Expected = len(toUpload) + len(uploaded)
	result.RecordSkip(len(uploaded))

	// Log subdirectories after all filtering is done
	seenDirs := make(map[string]bool)
	for _, f := range toUpload {
		dir := filepath.Dir(f)
		if dir != "." && !seenDirs[dir] {
			seenDirs[dir] = true
			c.log.Info("uploading files from subdirectory: %s", dir)
		}
	}

	c.log.Info("uploading %d files (%d already uploaded)", len(toUpload), len(uploaded))
	estimator := transfer.NewEstimator()

	for i, relPath := range toUpload {
		filename := filepath.Base(relPath)
		fullPath := filepath.Join(inputDir, relPath)
		dateStr := ""
		if fileDate := mediadate.ResolveDate(fullPath); !fileDate.IsZero() {
			dateStr = fmt.Sprintf(" (%s)", fileDate.Format("2006-01-02"))
		}
		c.log.Info("[%d/%d] %suploading %s%s", i+1, len(toUpload), estimator.Estimate(len(toUpload)-(i+1)), relPath, dateStr)
		c.log.Debug("reading file: %s", fullPath)

		uploadToken, err := c.uploadBytes(ctx, fullPath, filename)
		if err != nil {
			if errors.Is(err, errTokenExpired) {
				result.Finish()
				return result, err
			}
			result.RecordError(relPath, err.Error())
			c.log.Error("upload failed for %s: %v", relPath, err)
			estimator.Tick()
			continue
		}

		c.log.Debug("got upload token for %s, creating media item", relPath)
		if err := c.createMediaItem(ctx, uploadToken, filename); err != nil {
			if errors.Is(err, errTokenExpired) {
				result.Finish()
				return result, err
			}
			result.RecordError(relPath, err.Error())
			c.log.Error("create media item failed for %s: %v", relPath, err)
			estimator.Tick()
			continue
		}

		if err := appendUploadLog(logPath, relPath); err != nil {
			c.log.Error("failed to update upload log: %v", err)
		}

		info, statErr := os.Stat(fullPath)
		if statErr == nil {
			result.RecordSuccess(info.Size())
		} else {
			result.RecordSuccess(0)
		}
		c.log.Debug("successfully uploaded %s", relPath)
		estimator.Tick()
	}

	result.Finish()
	return result, nil
}

// uploadBytes uploads raw file bytes and returns an upload token.
func (c *Client) uploadBytes(ctx context.Context, filePath, filename string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("reading file: %w", err)
	}

	resp, err := c.retryableDo(ctx, func() (*http.Request, error) {
		req, err := http.NewRequest("POST", c.uploadURL(), bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("X-Goog-Upload-File-Name", filename)
		req.Header.Set("X-Goog-Upload-Protocol", "raw")
		return req, nil
	})
	if err != nil {
		return "", fmt.Errorf("upload request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}
	c.log.Debug("upload response body: %s", string(body))

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	return string(body), nil
}

// createMediaItem creates a media item in Google Photos from an upload token.
func (c *Client) createMediaItem(ctx context.Context, uploadToken, filename string) error {
	payload := map[string]any{
		"newMediaItems": []map[string]any{
			{
				"description": filename,
				"simpleMediaItem": map[string]string{
					"uploadToken": uploadToken,
					"fileName":    filename,
				},
			},
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}
	c.log.Debug("createMediaItem request body: %s", string(data))

	resp, err := c.retryableDo(ctx, func() (*http.Request, error) {
		req, err := http.NewRequest("POST", c.batchCreateURL(), bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	})
	if err != nil {
		return fmt.Errorf("create request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}
	c.log.Debug("createMediaItem response body: %s", string(body))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("create failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// collectMediaFiles walks a directory recursively and returns relative paths
// of supported media files.
func collectMediaFiles(dir string) ([]string, error) {
	var files []string

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if media.IsSupportedFile(d.Name()) {
			rel, relErr := filepath.Rel(dir, path)
			if relErr != nil {
				return fmt.Errorf("computing relative path: %w", relErr)
			}
			files = append(files, rel)
		}
		return nil
	})

	return files, err
}

// loadUploadLog reads the upload log and returns a set of already-uploaded filenames.
func loadUploadLog(path string) (map[string]bool, error) {
	result := make(map[string]bool)

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return nil, fmt.Errorf("opening upload log: %w", err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			result[line] = true
		}
	}

	return result, scanner.Err()
}

// appendUploadLog adds a filename to the upload log.
func appendUploadLog(path, filename string) (err error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening upload log: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	_, err = fmt.Fprintln(f, filename)
	return err
}
