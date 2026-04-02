package flickr

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/briandeitte/photo-copy/internal/daterange"
	"github.com/briandeitte/photo-copy/internal/media"
	"github.com/briandeitte/photo-copy/internal/mediadate"
	"github.com/briandeitte/photo-copy/internal/transfer"
)

// Upload uploads media files from inputDir to Flickr.
func (c *Client) Upload(ctx context.Context, inputDir string, limit int, dateRange *daterange.DateRange) (*transfer.Result, error) {
	c.log.Debug("starting Flickr upload from %s", inputDir)
	result := transfer.NewResult("flickr", "upload", inputDir)

	entries, err := os.ReadDir(inputDir)
	if err != nil {
		return result, fmt.Errorf("reading input dir: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && media.IsSupportedFile(e.Name()) {
			files = append(files, e.Name())
		}
	}

	if len(files) == 0 {
		c.log.Info("no supported media files found in %s", inputDir)
		result.Finish()
		return result, nil
	}

	// Filter by date range (before applying limit)
	if dateRange != nil {
		var filtered []string
		dateFiltered := 0
		for _, name := range files {
			filePath := filepath.Join(inputDir, name)
			fileDate := mediadate.ResolveDate(filePath)
			switch {
			case fileDate.IsZero():
				c.log.Info("including %s despite date range filter: no date available", name)
				filtered = append(filtered, name)
			case dateRange.Contains(fileDate):
				filtered = append(filtered, name)
			default:
				dateFiltered++
				c.log.Debug("skipping %s: date %s outside range", name, fileDate.Format("2006-01-02"))
			}
		}
		if dateFiltered > 0 {
			c.log.Info("filtered %d files outside date range", dateFiltered)
			result.RecordSkip(dateFiltered)
		}
		files = filtered
	}

	if len(files) == 0 {
		c.log.Info("no supported media files found in %s (after filtering)", inputDir)
		result.Finish()
		return result, nil
	}

	if limit > 0 && len(files) > limit {
		c.log.Info("limiting upload to %d of %d files", limit, len(files))
		files = files[:limit]
	}

	result.Expected = len(files)
	c.log.Info("found %d media files to upload", len(files))
	estimator := transfer.NewEstimator()

	consecutiveFailures := 0
	for i, filename := range files {
		select {
		case <-ctx.Done():
			result.Finish()
			return result, ctx.Err()
		default:
		}

		if err := c.uploadFile(ctx, filepath.Join(inputDir, filename)); err != nil {
			result.RecordError(filename, err.Error())
			estimator.Tick()
			remaining := len(files) - (i + 1)
			c.log.Error("[%d/%d] %suploading %s: %v", i+1, len(files), estimator.Estimate(remaining), filename, err)
			consecutiveFailures++
			if consecutiveFailures >= maxConsecutiveFailures {
				c.log.Error("aborting: %d consecutive upload failures", consecutiveFailures)
				result.Finish()
				return result, fmt.Errorf("aborted after %d consecutive upload failures", consecutiveFailures)
			}
			continue
		}

		consecutiveFailures = 0
		info, statErr := os.Stat(filepath.Join(inputDir, filename))
		if statErr != nil {
			c.log.Error("stat after upload for %s: %v", filename, statErr)
			result.RecordSuccess(0)
		} else {
			result.RecordSuccess(info.Size())
		}
		estimator.Tick()
		remaining := len(files) - (i + 1)
		dateStr := ""
		if fileDate := mediadate.ResolveDate(filepath.Join(inputDir, filename)); !fileDate.IsZero() {
			dateStr = fmt.Sprintf(" (%s)", fileDate.Format("2006-01-02"))
		}
		c.log.Info("[%d/%d] %suploaded %s%s", i+1, len(files), estimator.Estimate(remaining), filename, dateStr)
	}

	result.Finish()
	return result, nil
}

func (c *Client) uploadFile(ctx context.Context, filePath string) error {
	c.throttle()
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("photo", filepath.Base(filePath))
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, f); err != nil {
		return err
	}

	upURL := c.flickrUploadURL()
	params := map[string]string{}
	oauthSign("POST", upURL, params, c.cfg)

	for k, v := range params {
		if strings.HasPrefix(k, "oauth_") {
			if err := writer.WriteField(k, v); err != nil {
				return fmt.Errorf("writing form field %s: %w", k, err)
			}
		}
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("closing multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", upURL, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	c.log.Debug("HTTP POST %s (multipart upload: %s)", upURL, filepath.Base(filePath))
	resp, err := c.http.Do(req)
	if err != nil {
		c.log.Debug("HTTP error: %v", err)
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		c.log.Debug("reading upload response body: %v", readErr)
	}
	c.log.Debug("HTTP response: %d %s", resp.StatusCode, resp.Status)
	c.log.Debug("upload response body: %s", string(respBody))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upload failed HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
