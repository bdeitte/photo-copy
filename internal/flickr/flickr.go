package flickr

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/daterange"
	"github.com/briandeitte/photo-copy/internal/jpegmeta"
	"github.com/briandeitte/photo-copy/internal/logging"
	"github.com/briandeitte/photo-copy/internal/media"
	"github.com/briandeitte/photo-copy/internal/mediadate"
	"github.com/briandeitte/photo-copy/internal/mp4meta"
	"github.com/briandeitte/photo-copy/internal/transfer"
)

const (
	maxRetries             = 5
	baseRetryDelay         = 2 * time.Second
	minRequestInterval     = time.Second // Stay under 3,600 requests/hour
	maxConsecutiveFailures = 10          // Abort upload after this many consecutive failures
)

const (
	defaultAPIBaseURL = "https://api.flickr.com/services/rest/"
	defaultUploadURL  = "https://up.flickr.com/services/upload/"
	transferLogFile   = "transfer.log"
)

func apiURL() string {
	if u := os.Getenv("PHOTO_COPY_FLICKR_API_URL"); u != "" {
		return u
	}
	return defaultAPIBaseURL
}

func flickrUploadURL() string {
	if u := os.Getenv("PHOTO_COPY_FLICKR_UPLOAD_URL"); u != "" {
		return u
	}
	return defaultUploadURL
}

func isTestMode() bool {
	return os.Getenv("PHOTO_COPY_TEST_MODE") != ""
}

// Client provides Flickr API operations.
type Client struct {
	cfg        *config.FlickrConfig
	http       *http.Client
	log        *logging.Logger
	lastRequest time.Time
}

// NewClient creates a new Flickr client.
func NewClient(cfg *config.FlickrConfig, log *logging.Logger) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{},
		log:  log,
	}
}

// throttle ensures we don't exceed the Flickr API rate limit of 3,600 requests/hour.
func (c *Client) throttle() {
	if isTestMode() {
		return
	}
	if !c.lastRequest.IsZero() {
		elapsed := time.Since(c.lastRequest)
		if elapsed < minRequestInterval {
			time.Sleep(minRequestInterval - elapsed)
		}
	}
	c.lastRequest = time.Now()
}

// retryableGet performs an HTTP GET with retry on 429 and 5xx errors.
func (c *Client) retryableGet(ctx context.Context, url string) (*http.Response, error) {
	for attempt := 0; attempt <= maxRetries; attempt++ {
		c.throttle()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}

		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			_ = resp.Body.Close()
			if attempt == maxRetries {
				return nil, fmt.Errorf("HTTP %d after %d retries: %s", resp.StatusCode, maxRetries, url)
			}
			delay := c.retryDelay(attempt, resp)
			c.log.Info("HTTP %d, retrying in %v (attempt %d/%d)", resp.StatusCode, delay, attempt+1, maxRetries)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
			continue
		}

		return resp, nil
	}
	// unreachable
	return nil, fmt.Errorf("retries exhausted for %s", url)
}

// retryDelay calculates the backoff delay, honoring the Retry-After header if present.
func (c *Client) retryDelay(attempt int, resp *http.Response) time.Duration {
	if isTestMode() {
		return time.Millisecond
	}
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		if seconds, err := strconv.Atoi(ra); err == nil {
			return time.Duration(seconds) * time.Second
		}
	}
	return baseRetryDelay * time.Duration(math.Pow(2, float64(attempt)))
}

// buildAPIURL constructs a Flickr REST API URL (unsigned, for non-authenticated calls).
func buildAPIURL(method, apiKey string, params map[string]string) string {
	u, _ := url.Parse(apiURL())
	q := u.Query()
	q.Set("method", method)
	q.Set("api_key", apiKey)
	q.Set("format", "json")
	q.Set("nojsoncallback", "1")
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// signedAPIGet makes an OAuth-signed GET request to the Flickr REST API with rate limiting and retry.
// It retries when the API returns non-JSON responses (e.g. HTML error pages with 200 status).
// Note: each iteration calls retryableGet which has its own retry loop for 429/5xx, so in the
// worst case a single signedAPIGet call may make up to (maxRetries+1)^2 HTTP requests.
func (c *Client) signedAPIGet(ctx context.Context, method string, extra map[string]string) (*http.Response, error) {
	baseURL := apiURL()

	for attempt := 0; attempt <= maxRetries; attempt++ {
		params := map[string]string{
			"method":         method,
			"format":         "json",
			"nojsoncallback": "1",
		}
		for k, v := range extra {
			params[k] = v
		}

		oauthSign("GET", baseURL, params, c.cfg)

		v := url.Values{}
		for k, val := range params {
			v.Set(k, val)
		}
		resp, err := c.retryableGet(ctx, baseURL+"?"+v.Encode())
		if err != nil {
			return nil, err
		}

		// Flickr sometimes returns HTML error pages with a 200 status.
		// Detect this by checking Content-Type and retry.
		ct := resp.Header.Get("Content-Type")
		if ct != "" && !strings.Contains(ct, "json") && !strings.Contains(ct, "javascript") {
			// Read body to log the error details from Flickr
			bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024))
			_ = resp.Body.Close()
			bodySnippet := ""
			if readErr == nil && len(bodyBytes) > 0 {
				bodySnippet = string(bodyBytes)
			}
			if attempt == maxRetries {
				return nil, fmt.Errorf("API returned non-JSON response (Content-Type: %s, status: %d, body: %s) after %d retries", ct, resp.StatusCode, bodySnippet, maxRetries)
			}
			delay := baseRetryDelay * time.Duration(math.Pow(2, float64(attempt)))
			if isTestMode() {
				delay = time.Millisecond
			}
			c.log.Info("API returned non-JSON response (Content-Type: %s, status: %d, body: %s), retrying in %v (attempt %d/%d)", ct, resp.StatusCode, bodySnippet, delay, attempt+1, maxRetries)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
			continue
		}

		return resp, nil
	}
	return nil, fmt.Errorf("retries exhausted for %s API call", method)
}

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
	Stat string `json:"stat"`
}

// flexString handles JSON fields that may be a string or a number.
// The Flickr API sometimes returns numeric labels (e.g. 75 instead of "Square").
type flexString string

func (f *flexString) UnmarshalJSON(data []byte) error {
	// Try string first
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*f = flexString(s)
		return nil
	}
	// Fall back to number
	var n json.Number
	if err := json.Unmarshal(data, &n); err != nil {
		return fmt.Errorf("flexString: cannot unmarshal %s", string(data))
	}
	*f = flexString(n.String())
	return nil
}

// sizesResponse represents the Flickr getSizes API response.
type sizesResponse struct {
	Sizes struct {
		Size []struct {
			Label  flexString `json:"label"`
			Source string     `json:"source"`
		} `json:"size"`
	} `json:"sizes"`
	Stat string `json:"stat"`
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
			return result, fmt.Errorf("Flickr API error on page %d: stat=%s", page, photosResp.Stat) //nolint:staticcheck // proper noun
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
			if downloadErr != nil && len(candidates) > 1 && strings.Contains(downloadErr.Error(), "HTTP 404") {
				for i := 1; i < len(candidates); i++ {
					alt := candidates[i]
					c.log.Info("download 404 for %s (%s), trying fallback: %s", photo.ID, origResult.Label, alt.Label)
					ext = extensionFromURL(alt.URL, defaultExtForLabel(alt.Label))
					filename = fmt.Sprintf("%s_%s%s", photo.ID, photo.Secret, ext)
					downloadErr = c.downloadFile(ctx, alt.URL, filepath.Join(outputDir, filename))
					if downloadErr == nil || !strings.Contains(downloadErr.Error(), "HTTP 404") {
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
				result.RecordSuccess(filename, 0)
			} else {
				result.RecordSuccess(filename, info.Size())
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
				if !meta.isEmpty() {
					switch ext {
					case ".jpg", ".jpeg":
						if err := jpegmeta.SetMetadata(filePath, jpegmeta.Metadata{
							Title:       meta.Title,
							Description: meta.Description,
							Tags:        meta.Tags,
						}); err != nil {
							c.log.Error("setting JPEG XMP metadata for %s: %v", filename, err)
						}
					case ".mp4", ".mov":
						if err := mp4meta.SetXMPMetadata(filePath, mp4meta.XMPMetadata{
							Title:       meta.Title,
							Description: meta.Description,
							Tags:        meta.Tags,
						}); err != nil {
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

// originalResult holds the URL and metadata from getOriginalURLs.
type originalResult struct {
	URL   string
	Label string
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
		return nil, fmt.Errorf("Flickr API error: stat=%s", sizesResp.Stat) //nolint:staticcheck // proper noun
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
		return fmt.Errorf("HTTP %d downloading %s", resp.StatusCode, fileURL)
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
			result.RecordSuccess(filename, 0)
		} else {
			result.RecordSuccess(filename, info.Size())
		}
		estimator.Tick()
		remaining := len(files) - (i + 1)
		dateStr := ""
		if info != nil {
			dateStr = fmt.Sprintf(" (%s)", info.ModTime().Format("2006-01-02"))
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

	upURL := flickrUploadURL()
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

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
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
