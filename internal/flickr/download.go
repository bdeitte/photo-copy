package flickr

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/briandeitte/photo-copy/internal/daterange"
	"github.com/briandeitte/photo-copy/internal/jpegmeta"
	"github.com/briandeitte/photo-copy/internal/mp4meta"
	"github.com/briandeitte/photo-copy/internal/transfer"
)

// photosResponse represents the Flickr getPhotos API response.
type photosResponse struct {
	Photos struct {
		Page    int `json:"page"`
		Pages   int `json:"pages"`
		Total   int `json:"total"`
		Photo   []struct {
			ID          string             `json:"id"`
			Secret      string             `json:"secret"`
			Server      string             `json:"server"`
			Title       string             `json:"title"`
			DateTaken   string             `json:"datetaken"`
			DateUpload  string             `json:"dateupload"`
			Description flickrDescription  `json:"description"`
			Tags        string             `json:"tags"`
		} `json:"photo"`
	} `json:"photos"`
	Stat    string `json:"stat"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// sizesResponse represents the Flickr getSizes API response.
type sizesResponse struct {
	Sizes struct {
		Size []struct {
			Label  flexString `json:"label"`
			Source string     `json:"source"`
		} `json:"size"`
	} `json:"sizes"`
	Stat    string `json:"stat"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// originalResult holds the URL and metadata from getOriginalURLs.
type originalResult struct {
	URL   string
	Label string
}

// Download fetches all photos from the authenticated user's Flickr account.
func (c *Client) Download(ctx context.Context, outputDir string, limit int, noMetadata bool, dateRange *daterange.DateRange) (*transfer.Result, error) {
	c.log.Debug("starting Flickr download to %s", outputDir)
	result := transfer.NewResult("flickr", "download", outputDir)

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return result, fmt.Errorf("creating output dir: %w", err)
	}

	logPath := filepath.Join(outputDir, transferLogFile)
	transferred, err := loadTransferLog(logPath)
	if err != nil {
		return result, fmt.Errorf("loading transfer log: %w", err)
	}
	c.log.Debug("loaded transfer log with %d entries", len(transferred))
	estimator := transfer.NewEstimator()

	page := 1
	for {
		select {
		case <-ctx.Done():
			result.Finish()
			return result, ctx.Err()
		default:
		}

		c.log.Debug("fetching photo list page %d", page)
		resp, err := c.signedAPIGet(ctx, "flickr.people.getPhotos", map[string]string{
			"user_id":  "me",
			"page":     strconv.Itoa(page),
			"per_page": "500",
			"extras":   "date_taken,date_upload,description,tags",
		})
		if err != nil {
			return result, fmt.Errorf("fetching photos page %d: %w", page, err)
		}

		var photosResp photosResponse
		decodeErr := func() error {
			defer func() { _ = resp.Body.Close() }()
			return json.NewDecoder(resp.Body).Decode(&photosResp)
		}()
		if decodeErr != nil {
			return result, fmt.Errorf("decoding photos response: %w", decodeErr)
		}

		if photosResp.Stat != "ok" {
			return result, fmt.Errorf("Flickr API error on page %d: %s (code %d)", page, photosResp.Message, photosResp.Code) //nolint:staticcheck // proper noun
		}

		c.log.Debug("page %d/%d: %d photos/videos", page, photosResp.Photos.Pages, len(photosResp.Photos.Photo))

		if page == 1 {
			result.Expected = photosResp.Photos.Total
			c.log.Info("found %d photos/videos on Flickr", result.Expected)
		}

		pageSkipped := 0
		pageDateFiltered := 0
		for _, photo := range photosResp.Photos.Photo {
			if transferred[photo.ID] {
				result.RecordSkip(1)
				pageSkipped++
				c.log.Debug("skipping already downloaded: %s", photo.ID)
				continue
			}

			// Resolve photo date once for both filtering and metadata
			photoDate := resolvePhotoDate(photo.DateTaken, photo.DateUpload)

			// Date range filtering
			if dateRange != nil {
				if photoDate.IsZero() {
					c.log.Info("including %s despite date range filter: no date available", photo.ID)
				} else if !dateRange.Contains(photoDate) {
					result.RecordSkip(1)
					pageDateFiltered++
					c.log.Debug("skipping %s: date %s outside range", photo.ID, photoDate.Format("2006-01-02"))
					continue
				}
			}

			candidates, err := c.getOriginalURLs(ctx, photo.ID)
			if err != nil {
				result.RecordError(photo.ID, err.Error())
				estimator.Tick()
				processed := result.Succeeded + result.Skipped + result.Failed
				c.log.Error("[%d/%d] %sgetting original URL for %s: %v", processed, result.Expected, estimator.Estimate(estimateRemaining(limit, result)), photo.ID, err)
				continue
			}

			origResult := candidates[0]
			if origResult.Label != "Original" && origResult.Label != "Video Original" {
				c.log.Info("warning: original not available for %s, using %s", photo.ID, origResult.Label)
			}

			ext := extensionFromURL(origResult.URL, defaultExtForLabel(origResult.Label))
			filename := fmt.Sprintf("%s_%s%s", photo.ID, photo.Secret, ext)

			// Try downloading with fallback to alternative sizes on 404.
			downloadErr := c.downloadFile(ctx, origResult.URL, filepath.Join(outputDir, filename))
			var httpErr *HTTPStatusError
			if downloadErr != nil && len(candidates) > 1 && errors.As(downloadErr, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
				for i := 1; i < len(candidates); i++ {
					alt := candidates[i]
					c.log.Info("download 404 for %s (%s), trying fallback: %s", photo.ID, origResult.Label, alt.Label)
					ext = extensionFromURL(alt.URL, defaultExtForLabel(alt.Label))
					filename = fmt.Sprintf("%s_%s%s", photo.ID, photo.Secret, ext)
					downloadErr = c.downloadFile(ctx, alt.URL, filepath.Join(outputDir, filename))
					if downloadErr == nil || !errors.As(downloadErr, &httpErr) || httpErr.StatusCode != http.StatusNotFound {
						break
					}
				}
			}
			if downloadErr != nil {
				result.RecordError(filename, downloadErr.Error())
				estimator.Tick()
				processed := result.Succeeded + result.Skipped + result.Failed
				c.log.Error("[%d/%d] %sdownloading %s: %v", processed, result.Expected, estimator.Estimate(estimateRemaining(limit, result)), filename, downloadErr)
				continue
			}

			if err := appendTransferLog(logPath, photo.ID); err != nil {
				c.log.Error("updating transfer log for %s: %v", filename, err)
			}

			info, statErr := os.Stat(filepath.Join(outputDir, filename))
			if statErr != nil {
				c.log.Error("stat after download for %s: %v", filename, statErr)
				result.RecordSuccess(0)
			} else {
				result.RecordSuccess(info.Size())
			}

			filePath := filepath.Join(outputDir, filename)

			if !noMetadata {
				// Set original dates in MP4 container metadata. This must run
				// before XMP embedding because gomp4 cannot parse UUID boxes and
				// would hang if SetXMPMetadata appended one first.
				if !photoDate.IsZero() {
					if ext == ".mp4" || ext == ".mov" {
						if err := mp4meta.SetCreationTime(filePath, photoDate); err != nil {
							c.log.Error("setting MP4 metadata for %s: %v", filename, err)
						}
					}
				}

				// Embed title, description, and tags as XMP metadata.
				// For MP4/MOV this appends a UUID box at EOF which gomp4 cannot
				// parse, so it must happen after SetCreationTime.
				meta := buildPhotoMeta(photo.Title, photo.Description.Content, photo.Tags)
				if !meta.IsEmpty() {
					switch ext {
					case ".jpg", ".jpeg":
						if err := jpegmeta.SetMetadata(filePath, meta); err != nil {
							c.log.Error("setting JPEG XMP metadata for %s: %v", filename, err)
						}
					case ".mp4", ".mov":
						if err := mp4meta.SetXMPMetadata(filePath, meta); err != nil {
							c.log.Error("setting MP4 XMP metadata for %s: %v", filename, err)
						}
					}
				}

				// Set filesystem timestamps last since temp-file-rename in both
				// SetCreationTime and SetXMPMetadata resets mtime.
				if !photoDate.IsZero() {
					if err := os.Chtimes(filePath, photoDate, photoDate); err != nil {
						c.log.Error("setting file time for %s: %v", filename, err)
					}
				} else {
					c.log.Info("no date available for %s, skipping date metadata", filename)
				}
			}

			estimator.Tick()
			processed := result.Succeeded + result.Skipped + result.Failed
			detail := ""
			switch {
			case photo.Title != "" && !photoDate.IsZero():
				detail = fmt.Sprintf(" %q (%s)", photo.Title, photoDate.Format("2006-01-02"))
			case photo.Title != "":
				detail = fmt.Sprintf(" %q", photo.Title)
			case !photoDate.IsZero():
				detail = fmt.Sprintf(" (%s)", photoDate.Format("2006-01-02"))
			}
			c.log.Info("[%d/%d] %sdownloaded %s%s", processed, result.Expected, estimator.Estimate(estimateRemaining(limit, result)), filename, detail)

			if limit > 0 && result.Succeeded+result.Failed >= limit {
				c.log.Info("reached limit of %d files (%d downloaded, %d errors)", limit, result.Succeeded, result.Failed)
				break
			}
		}

		if pageSkipped > 0 {
			c.log.Info("[%d/%d] skipped %d already-downloaded photos/videos on page %d", result.Succeeded+result.Skipped+result.Failed, result.Expected, pageSkipped, page)
		}
		if pageDateFiltered > 0 {
			c.log.Info("[%d/%d] skipped %d photos/videos outside date range on page %d", result.Succeeded+result.Skipped+result.Failed, result.Expected, pageDateFiltered, page)
		}

		if limit > 0 && result.Succeeded+result.Failed >= limit {
			break
		}

		if page >= photosResp.Photos.Pages {
			break
		}
		page++
	}

	result.Finish()
	return result, nil
}

// estimateRemaining returns the number of files left to download/upload for
// time estimation. When a limit is set, remaining is based on the limit
// rather than the total expected count.
func estimateRemaining(limit int, result *transfer.Result) int {
	done := result.Succeeded + result.Failed
	if limit > 0 {
		remaining := limit - done
		if remaining < 0 {
			return 0
		}
		return remaining
	}
	return result.Expected - result.Succeeded - result.Skipped - result.Failed
}

// getOriginalURLs retrieves available URLs for a photo or video in preference order.
// Returns multiple candidates so the caller can fall back on download errors (e.g. 404).
func (c *Client) getOriginalURLs(ctx context.Context, photoID string) ([]originalResult, error) {
	resp, err := c.signedAPIGet(ctx, "flickr.photos.getSizes", map[string]string{
		"photo_id": photoID,
	})
	if err != nil {
		return nil, fmt.Errorf("fetching sizes: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var sizesResp sizesResponse
	if err := json.NewDecoder(resp.Body).Decode(&sizesResp); err != nil {
		return nil, fmt.Errorf("decoding sizes response: %w", err)
	}

	if sizesResp.Stat != "ok" {
		return nil, fmt.Errorf("Flickr API error: %s (code %d)", sizesResp.Message, sizesResp.Code) //nolint:staticcheck // proper noun
	}

	// Build ordered list: preferred sizes first, then any remaining as fallbacks.
	// Video sizes are checked first so that a video with both "Original" (photo
	// thumbnail) and "Video Original" picks the actual video file.
	preferred := []string{"Video Original", "Video Player", "Original", "Large"}
	seen := make(map[string]bool)
	var results []originalResult

	for _, pref := range preferred {
		for _, s := range sizesResp.Sizes.Size {
			if string(s.Label) == pref {
				results = append(results, originalResult{URL: s.Source, Label: string(s.Label)})
				seen[s.Source] = true
			}
		}
	}

	// Add any remaining sizes not already included as last-resort fallbacks.
	for _, s := range sizesResp.Sizes.Size {
		if !seen[s.Source] {
			results = append(results, originalResult{URL: s.Source, Label: string(s.Label)})
		}
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no sizes available for photo %s", photoID)
	}

	return results, nil
}

// downloadFile downloads a URL to a local file path with rate limiting and retry.
func (c *Client) downloadFile(ctx context.Context, fileURL, destPath string) (err error) {
	resp, err := c.retryableGet(ctx, fileURL)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return &HTTPStatusError{StatusCode: resp.StatusCode, URL: fileURL}
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	_, err = io.Copy(f, resp.Body)
	return err
}

// extensionFromURL extracts the file extension from a Flickr media URL.
// Falls back to defaultExt if the extension cannot be determined.
func extensionFromURL(rawURL, defaultExt string) string {
	u, err := url.Parse(rawURL)
	if err == nil {
		ext := strings.ToLower(path.Ext(u.Path))
		if ext != "" {
			return ext
		}
	}
	return defaultExt
}

// defaultExtForLabel returns the default file extension based on the Flickr
// size label. Video labels default to ".mp4"; photo labels default to ".jpg".
func defaultExtForLabel(label string) string {
	if strings.HasPrefix(label, "Video") {
		return ".mp4"
	}
	return ".jpg"
}

// loadTransferLog reads the transfer log and returns a set of photo IDs that
// have been transferred. Handles both old format (filename like "ID_SECRET.jpg")
// and new format (bare photo ID).
func loadTransferLog(logPath string) (map[string]bool, error) {
	result := make(map[string]bool)

	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// Old format: "12345_secret.jpg" → extract photo ID "12345"
		// New format: "12345" → use as-is
		if idx := strings.Index(line, "_"); idx > 0 {
			result[line[:idx]] = true
		} else {
			result[line] = true
		}
	}
	return result, scanner.Err()
}

// appendTransferLog appends a filename to the transfer log.
func appendTransferLog(path, filename string) (err error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	_, err = fmt.Fprintln(f, filename)
	return err
}
