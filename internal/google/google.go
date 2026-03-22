package google

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/daterange"
	"github.com/briandeitte/photo-copy/internal/logging"
	"github.com/briandeitte/photo-copy/internal/media"
	"github.com/briandeitte/photo-copy/internal/mediadate"
	"github.com/briandeitte/photo-copy/internal/transfer"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// errTokenExpired is returned when the OAuth token is expired or revoked.
var errTokenExpired = fmt.Errorf("Google OAuth token has been expired or revoked. Run 'photo-copy config google' to re-authenticate") //nolint:staticcheck // proper noun

const (
	defaultUploadURL      = "https://photoslibrary.googleapis.com/v1/uploads"
	defaultBatchCreateURL = "https://photoslibrary.googleapis.com/v1/mediaItems:batchCreate"
	dailyLimit            = 10000
	maxRetries            = 5
	baseRetryDelay        = 2 * time.Second
	minUploadInterval     = 2 * time.Second // Throttle uploads to avoid rate limiting
)

func getUploadURL() string {
	if base := os.Getenv("PHOTO_COPY_GOOGLE_API_URL"); base != "" {
		return base + "/v1/uploads"
	}
	return defaultUploadURL
}

func getBatchCreateURL() string {
	if base := os.Getenv("PHOTO_COPY_GOOGLE_API_URL"); base != "" {
		return base + "/v1/mediaItems:batchCreate"
	}
	return defaultBatchCreateURL
}

func isTestMode() bool {
	return os.Getenv("PHOTO_COPY_TEST_MODE") != ""
}

var oauthScopes = []string{
	"https://www.googleapis.com/auth/photoslibrary.appendonly",
}

// Client wraps an authenticated HTTP client for Google Photos API.
type Client struct {
	httpClient  *http.Client
	log         *logging.Logger
	configDir   string
	lastRequest time.Time
}

// NewClient creates a new Google Photos client with OAuth2 authentication.
func NewClient(ctx context.Context, cfg *config.GoogleConfig, configDir string, log *logging.Logger) (*Client, error) {
	if os.Getenv("PHOTO_COPY_GOOGLE_TOKEN") == "skip" {
		return &Client{
			httpClient: &http.Client{},
			log:        log,
			configDir:  configDir,
		}, nil
	}

	oauthCfg := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Scopes:       oauthScopes,
		Endpoint:     google.Endpoint,
		RedirectURL:  "http://localhost", // placeholder, updated with actual port in runOAuthFlow
	}

	token, err := loadToken(configDir)
	if err != nil {
		log.Debug("no saved token found, starting OAuth flow")
		token, err = runOAuthFlow(ctx, oauthCfg)
		if err != nil {
			return nil, fmt.Errorf("OAuth flow failed: %w", err)
		}
		if err := saveToken(configDir, token); err != nil {
			log.Error("failed to save token: %v", err)
		}
	}

	client := oauthCfg.Client(ctx, token)

	return &Client{
		httpClient: client,
		log:        log,
		configDir:  configDir,
	}, nil
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
		if !uploaded[filepath.Base(f)] {
			toUpload = append(toUpload, f)
		}
	}

	// Filter by date range
	if dateRange != nil {
		var filtered []string
		dateFiltered := 0
		for _, filePath := range toUpload {
			fileDate := mediadate.ResolveDate(filePath)
			switch {
			case fileDate.IsZero():
				c.log.Info("including %s despite date range filter: no date available", filepath.Base(filePath))
				filtered = append(filtered, filePath)
			case dateRange.Contains(fileDate):
				filtered = append(filtered, filePath)
			default:
				dateFiltered++
				c.log.Debug("skipping %s: date %s outside range", filepath.Base(filePath), fileDate.Format("2006-01-02"))
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

	c.log.Info("uploading %d files (%d already uploaded)", len(toUpload), len(uploaded))
	estimator := transfer.NewEstimator()

	for i, filePath := range toUpload {
		filename := filepath.Base(filePath)
		dateStr := ""
		if fi, ferr := os.Stat(filePath); ferr == nil {
			dateStr = fmt.Sprintf(" (%s)", fi.ModTime().Format("2006-01-02"))
		}
		c.log.Info("[%d/%d] %suploading %s%s", i+1, len(toUpload), estimator.Estimate(len(toUpload)-(i+1)), filename, dateStr)
		c.log.Debug("reading file: %s", filePath)

		uploadToken, err := c.uploadBytes(ctx, filePath, filename)
		if err != nil {
			if errors.Is(err, errTokenExpired) {
				result.Finish()
				return result, err
			}
			result.RecordError(filename, err.Error())
			c.log.Error("upload failed for %s: %v", filename, err)
			estimator.Tick()
			continue
		}

		c.log.Debug("got upload token for %s, creating media item", filename)
		if err := c.createMediaItem(ctx, uploadToken, filename); err != nil {
			if errors.Is(err, errTokenExpired) {
				result.Finish()
				return result, err
			}
			result.RecordError(filename, err.Error())
			c.log.Error("create media item failed for %s: %v", filename, err)
			estimator.Tick()
			continue
		}

		if err := appendUploadLog(logPath, filename); err != nil {
			c.log.Error("failed to update upload log: %v", err)
		}

		info, statErr := os.Stat(filePath)
		if statErr == nil {
			result.RecordSuccess(filename, info.Size())
		} else {
			result.RecordSuccess(filename, 0)
		}
		c.log.Debug("successfully uploaded %s", filename)
		estimator.Tick()
	}

	result.Finish()
	return result, nil
}

// throttle ensures we don't exceed Google Photos API rate limits.
func (c *Client) throttle() {
	if isTestMode() {
		return
	}
	if !c.lastRequest.IsZero() {
		elapsed := time.Since(c.lastRequest)
		if elapsed < minUploadInterval {
			time.Sleep(minUploadInterval - elapsed)
		}
	}
	c.lastRequest = time.Now()
}

// retryDelay calculates the backoff delay, honoring the Retry-After header if present.
func (c *Client) retryDelay(attempt int, resp *http.Response) time.Duration {
	if isTestMode() {
		return time.Millisecond
	}
	if resp != nil {
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if seconds, err := strconv.Atoi(ra); err == nil {
				return time.Duration(seconds) * time.Second
			}
		}
	}
	return baseRetryDelay * (1 << uint(attempt))
}

// retryableDo performs an HTTP request with throttling and retry on 429/5xx errors.
func (c *Client) retryableDo(ctx context.Context, buildReq func() (*http.Request, error)) (*http.Response, error) {
	for attempt := 0; attempt <= maxRetries; attempt++ {
		c.throttle()

		req, err := buildReq()
		if err != nil {
			return nil, err
		}
		req = req.WithContext(ctx)

		c.log.Debug("HTTP %s %s", req.Method, req.URL.String())
		for key, vals := range req.Header {
			c.log.Debug("  %s: %s", key, strings.Join(vals, ", "))
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if strings.Contains(strings.ToLower(err.Error()), "invalid_grant") {
				c.log.Debug("OAuth error: %v", err)
				return nil, errTokenExpired
			}
			if attempt == maxRetries {
				return nil, err
			}
			delay := c.retryDelay(attempt, nil)
			c.log.Info("network error, retrying in %v (attempt %d/%d): %v", delay, attempt+1, maxRetries, err)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
			continue
		}

		c.log.Debug("HTTP response: %d %s", resp.StatusCode, resp.Status)

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			_ = resp.Body.Close()
			if attempt == maxRetries {
				return nil, fmt.Errorf("HTTP %d after %d retries", resp.StatusCode, maxRetries)
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
	return nil, fmt.Errorf("unreachable")
}

// uploadBytes uploads raw file bytes and returns an upload token.
func (c *Client) uploadBytes(ctx context.Context, filePath, filename string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("reading file: %w", err)
	}

	resp, err := c.retryableDo(ctx, func() (*http.Request, error) {
		req, err := http.NewRequest("POST", getUploadURL(), bytes.NewReader(data))
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
		req, err := http.NewRequest("POST", getBatchCreateURL(), bytes.NewReader(data))
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

// collectMediaFiles lists media files in a directory filtered by supported types.
func collectMediaFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading directory: %w", err)
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if media.IsSupportedFile(entry.Name()) {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}

	return files, nil
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

// loadToken loads a saved OAuth2 token from the config directory.
func loadToken(configDir string) (*oauth2.Token, error) {
	tokenData, err := config.LoadGoogleToken(configDir)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(tokenData)
	if err != nil {
		return nil, fmt.Errorf("marshaling token data: %w", err)
	}

	var token oauth2.Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("unmarshaling token: %w", err)
	}

	return &token, nil
}

// saveToken saves an OAuth2 token to the config directory.
func saveToken(configDir string, token *oauth2.Token) error {
	return config.SaveGoogleToken(configDir, token)
}

// runOAuthFlow runs an OAuth2 flow using a localhost redirect.
// It starts a temporary HTTP server to receive the authorization code from Google.
func runOAuthFlow(ctx context.Context, cfg *oauth2.Config) (*oauth2.Token, error) {
	// Listen on a random available port
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, fmt.Errorf("starting local server: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	// Update the redirect URL with the actual port
	cfg.RedirectURL = fmt.Sprintf("http://localhost:%d", port)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errMsg := r.URL.Query().Get("error")
			if errMsg == "" {
				errMsg = "no authorization code received"
			}
			_, _ = fmt.Fprintf(w, "<html><body><h2>Authorization failed: %s</h2><p>You can close this window.</p></body></html>", errMsg)
			errCh <- fmt.Errorf("authorization failed: %s", errMsg)
			return
		}
		_, _ = fmt.Fprint(w, "<html><body><h2>Authorization successful!</h2><p>You can close this window and return to the terminal.</p></body></html>")
		codeCh <- code
	})

	server := &http.Server{Handler: mux}

	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- fmt.Errorf("local server error: %w", serveErr)
		}
	}()
	defer func() { _ = server.Close() }()

	authURL := cfg.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Println("Opening browser for Google authorization...")
	fmt.Println("Note: Google will show an 'unverified app' warning because this is your")
	fmt.Println("own personal OAuth app. Click 'Advanced' then 'Go to photo-copy (unsafe)'")
	fmt.Println("to proceed — this is expected and safe.")
	fmt.Println()
	fmt.Println("If the browser doesn't open, visit this URL:")
	fmt.Println(authURL)
	fmt.Println()

	// Try to open the browser automatically
	openBrowser(authURL)

	fmt.Println("Waiting for authorization...")

	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	token, err := cfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("exchanging code for token: %w", err)
	}

	return token, nil
}

// openBrowser tries to open a URL in the default browser.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	_ = cmd.Start()
}
