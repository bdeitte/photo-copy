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
	"github.com/briandeitte/photo-copy/internal/logging"
	"github.com/briandeitte/photo-copy/internal/media"
	"github.com/briandeitte/photo-copy/internal/transfer"
	"github.com/schollz/progressbar/v3"
)

const (
	maxRetries       = 5
	baseRetryDelay   = 2 * time.Second
	minRequestInterval = time.Second // Stay under 3,600 requests/hour
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
func (c *Client) signedAPIGet(ctx context.Context, method string, extra map[string]string) (*http.Response, error) {
	baseURL := apiURL()
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
	return c.retryableGet(ctx, baseURL+"?"+v.Encode())
}

// photosResponse represents the Flickr getPhotos API response.
type photosResponse struct {
	Photos struct {
		Page    int `json:"page"`
		Pages   int `json:"pages"`
		Total   int `json:"total"`
		Photo   []struct {
			ID     string `json:"id"`
			Secret string `json:"secret"`
			Server string `json:"server"`
			Title  string `json:"title"`
		} `json:"photo"`
	} `json:"photos"`
	Stat string `json:"stat"`
}

// sizesResponse represents the Flickr getSizes API response.
type sizesResponse struct {
	Sizes struct {
		Size []struct {
			Label  string `json:"label"`
			Source string `json:"source"`
		} `json:"size"`
	} `json:"sizes"`
	Stat string `json:"stat"`
}

// Download fetches all photos from the authenticated user's Flickr account.
func (c *Client) Download(ctx context.Context, outputDir string, limit int) (*transfer.Result, error) {
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
		})
		if err != nil {
			return result, fmt.Errorf("fetching photos page %d: %w", page, err)
		}

		var photosResp photosResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&photosResp)
		_ = resp.Body.Close()
		if decodeErr != nil {
			return result, fmt.Errorf("decoding photos response: %w", decodeErr)
		}

		if photosResp.Stat != "ok" {
			return result, fmt.Errorf("Flickr API error on page %d: stat=%s", page, photosResp.Stat) //nolint:staticcheck // proper noun
		}

		c.log.Debug("page %d/%d: %d photos", page, photosResp.Photos.Pages, len(photosResp.Photos.Photo))

		if page == 1 {
			result.Expected = photosResp.Photos.Total
			c.log.Info("found %d photos on Flickr", result.Expected)
		}

		pageSkipped := 0
		for _, photo := range photosResp.Photos.Photo {
			if transferred[photo.ID] {
				result.RecordSkip(1)
				pageSkipped++
				c.log.Debug("skipping already downloaded: %s", photo.ID)
				continue
			}

			origResult, err := c.getOriginalURL(ctx, photo.ID)
			if err != nil {
				result.RecordError(photo.ID, err.Error())
				c.log.Error("[%d/%d] getting original URL for %s: %v", result.Succeeded+result.Skipped+result.Failed, result.Expected, photo.ID, err)
				continue
			}

			if origResult.Label != "Original" && origResult.Label != "Video Original" {
				c.log.Info("warning: original not available for %s, using %s", photo.ID, origResult.Label)
			}

			ext := extensionFromURL(origResult.URL)
			filename := fmt.Sprintf("%s_%s%s", photo.ID, photo.Secret, ext)

			if err := c.downloadFile(ctx, origResult.URL, filepath.Join(outputDir, filename)); err != nil {
				result.RecordError(filename, err.Error())
				c.log.Error("[%d/%d] downloading %s: %v", result.Succeeded+result.Skipped+result.Failed, result.Expected, filename, err)
				continue
			}

			if err := appendTransferLog(logPath, photo.ID); err != nil {
				c.log.Error("updating transfer log for %s: %v", filename, err)
			}

			info, statErr := os.Stat(filepath.Join(outputDir, filename))
			if statErr != nil {
				result.RecordSuccess(filename, 0)
			} else {
				result.RecordSuccess(filename, info.Size())
			}
			c.log.Info("[%d/%d] downloaded %s", result.Succeeded+result.Skipped+result.Failed, result.Expected, filename)

			if limit > 0 && result.Succeeded+result.Failed >= limit {
				c.log.Info("reached limit of %d files (%d downloaded, %d errors)", limit, result.Succeeded, result.Failed)
				break
			}
		}

		if pageSkipped > 0 {
			c.log.Info("[%d/%d] skipped %d already-downloaded photos on page %d", result.Succeeded+result.Skipped+result.Failed, result.Expected, pageSkipped, page)
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

// originalResult holds the URL and metadata from getOriginalURL.
type originalResult struct {
	URL   string
	Label string
}

// getOriginalURL retrieves the best available URL for a photo or video.
func (c *Client) getOriginalURL(ctx context.Context, photoID string) (*originalResult, error) {
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

	// Prefer original sizes, then large fallbacks, then last available.
	// Video sizes are checked first so that a video with both "Original" (photo
	// thumbnail) and "Video Original" picks the actual video file.
	for _, pref := range []string{"Video Original", "Original", "Video Player", "Large"} {
		for _, s := range sizesResp.Sizes.Size {
			if s.Label == pref {
				return &originalResult{URL: s.Source, Label: s.Label}, nil
			}
		}
	}

	if len(sizesResp.Sizes.Size) > 0 {
		last := sizesResp.Sizes.Size[len(sizesResp.Sizes.Size)-1]
		return &originalResult{URL: last.Source, Label: last.Label}, nil
	}

	return nil, fmt.Errorf("no sizes available for photo %s", photoID)
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
func (c *Client) Upload(ctx context.Context, inputDir string, limit int) (*transfer.Result, error) {
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

	if limit > 0 && len(files) > limit {
		c.log.Info("limiting upload to %d of %d files", limit, len(files))
		files = files[:limit]
	}

	result.Expected = len(files)
	c.log.Info("found %d media files to upload", len(files))

	bar := progressbar.NewOptions(len(files),
		progressbar.OptionSetDescription("Uploading"),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(40),
	)

	for _, filename := range files {
		select {
		case <-ctx.Done():
			result.Finish()
			return result, ctx.Err()
		default:
		}

		if err := c.uploadFile(ctx, filepath.Join(inputDir, filename)); err != nil {
			result.RecordError(filename, err.Error())
			c.log.Error("uploading %s: %v", filename, err)
			_ = bar.Add(1)
			continue
		}

		info, statErr := os.Stat(filepath.Join(inputDir, filename))
		if statErr != nil {
			result.RecordSuccess(filename, 0)
		} else {
			result.RecordSuccess(filename, info.Size())
		}
		_ = bar.Add(1)
	}

	fmt.Println()
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
// Falls back to ".jpg" if the extension cannot be determined.
func extensionFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err == nil {
		ext := strings.ToLower(path.Ext(u.Path))
		if ext != "" {
			return ext
		}
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
