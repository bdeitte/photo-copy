package flickr

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/logging"
	"github.com/briandeitte/photo-copy/internal/media"
	"github.com/schollz/progressbar/v3"
)

const (
	apiBaseURL     = "https://api.flickr.com/services/rest/"
	transferLogFile = "transfer.log"
)

// Client provides Flickr API operations.
type Client struct {
	cfg    *config.FlickrConfig
	http   *http.Client
	log    *logging.Logger
}

// NewClient creates a new Flickr client.
func NewClient(cfg *config.FlickrConfig, log *logging.Logger) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{},
		log:  log,
	}
}

// buildAPIURL constructs a Flickr REST API URL.
func buildAPIURL(method, apiKey string, params map[string]string) string {
	u, _ := url.Parse(apiBaseURL)
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

// photosResponse represents the Flickr getPhotos API response.
type photosResponse struct {
	Photos struct {
		Page    int `json:"page"`
		Pages   int `json:"pages"`
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
func (c *Client) Download(ctx context.Context, outputDir string) error {
	c.log.Debug("starting Flickr download to %s", outputDir)

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	logPath := filepath.Join(outputDir, transferLogFile)
	transferred, err := loadTransferLog(logPath)
	if err != nil {
		return fmt.Errorf("loading transfer log: %w", err)
	}
	c.log.Debug("loaded transfer log with %d entries", len(transferred))

	page := 1
	totalDownloaded := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		c.log.Debug("fetching photo list page %d", page)
		apiURL := buildAPIURL("flickr.people.getPhotos", c.cfg.APIKey, map[string]string{
			"user_id":  "me",
			"page":     strconv.Itoa(page),
			"per_page": "500",
		})

		resp, err := c.http.Get(apiURL)
		if err != nil {
			return fmt.Errorf("fetching photos page %d: %w", page, err)
		}

		var photosResp photosResponse
		if err := json.NewDecoder(resp.Body).Decode(&photosResp); err != nil {
			resp.Body.Close()
			return fmt.Errorf("decoding photos response: %w", err)
		}
		resp.Body.Close()

		if photosResp.Stat != "ok" {
			return fmt.Errorf("Flickr API error on page %d: stat=%s", page, photosResp.Stat)
		}

		c.log.Debug("page %d/%d: %d photos", page, photosResp.Photos.Pages, len(photosResp.Photos.Photo))

		for _, photo := range photosResp.Photos.Photo {
			filename := fmt.Sprintf("%s_%s.jpg", photo.ID, photo.Secret)
			if transferred[filename] {
				c.log.Debug("skipping already downloaded: %s", filename)
				continue
			}

			downloadURL, err := c.getOriginalURL(photo.ID)
			if err != nil {
				c.log.Error("getting original URL for %s: %v", photo.ID, err)
				continue
			}

			if err := c.downloadFile(ctx, downloadURL, filepath.Join(outputDir, filename)); err != nil {
				c.log.Error("downloading %s: %v", filename, err)
				continue
			}

			if err := appendTransferLog(logPath, filename); err != nil {
				c.log.Error("updating transfer log for %s: %v", filename, err)
			}

			totalDownloaded++
			c.log.Debug("downloaded %s (%d total)", filename, totalDownloaded)
		}

		if page >= photosResp.Photos.Pages {
			break
		}
		page++
	}

	c.log.Info("download complete: %d photos downloaded", totalDownloaded)
	return nil
}

// getOriginalURL retrieves the original-size URL for a photo.
func (c *Client) getOriginalURL(photoID string) (string, error) {
	apiURL := buildAPIURL("flickr.photos.getSizes", c.cfg.APIKey, map[string]string{
		"photo_id": photoID,
	})

	resp, err := c.http.Get(apiURL)
	if err != nil {
		return "", fmt.Errorf("fetching sizes: %w", err)
	}
	defer resp.Body.Close()

	var sizesResp sizesResponse
	if err := json.NewDecoder(resp.Body).Decode(&sizesResp); err != nil {
		return "", fmt.Errorf("decoding sizes response: %w", err)
	}

	if sizesResp.Stat != "ok" {
		return "", fmt.Errorf("Flickr API error: stat=%s", sizesResp.Stat)
	}

	// Prefer "Original", fall back to "Large", then last available size.
	for _, pref := range []string{"Original", "Large"} {
		for _, s := range sizesResp.Sizes.Size {
			if s.Label == pref {
				return s.Source, nil
			}
		}
	}

	if len(sizesResp.Sizes.Size) > 0 {
		return sizesResp.Sizes.Size[len(sizesResp.Sizes.Size)-1].Source, nil
	}

	return "", fmt.Errorf("no sizes available for photo %s", photoID)
}

// downloadFile downloads a URL to a local file path.
func (c *Client) downloadFile(ctx context.Context, fileURL, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d downloading %s", resp.StatusCode, fileURL)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

// Upload uploads media files from inputDir to Flickr.
func (c *Client) Upload(ctx context.Context, inputDir string) error {
	c.log.Debug("starting Flickr upload from %s", inputDir)

	entries, err := os.ReadDir(inputDir)
	if err != nil {
		return fmt.Errorf("reading input dir: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && media.IsSupportedFile(e.Name()) {
			files = append(files, e.Name())
		}
	}

	if len(files) == 0 {
		c.log.Info("no supported media files found in %s", inputDir)
		return nil
	}

	c.log.Info("found %d media files to upload", len(files))

	bar := progressbar.NewOptions(len(files),
		progressbar.OptionSetDescription("Uploading"),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(40),
	)

	for _, filename := range files {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := c.uploadFile(filepath.Join(inputDir, filename)); err != nil {
			return fmt.Errorf("uploading %s: %w", filename, err)
		}
		bar.Add(1)
	}

	fmt.Println()
	return nil
}

// uploadFile is a placeholder until OAuth is wired in.
func (c *Client) uploadFile(path string) error {
	return fmt.Errorf("upload requires OAuth (not yet implemented)")
}

// loadTransferLog reads the transfer log and returns a set of transferred filenames.
func loadTransferLog(path string) (map[string]bool, error) {
	result := make(map[string]bool)

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			result[line] = true
		}
	}
	return result, scanner.Err()
}

// appendTransferLog appends a filename to the transfer log.
func appendTransferLog(path, filename string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = fmt.Fprintln(f, filename)
	return err
}
