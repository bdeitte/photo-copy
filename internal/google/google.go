package google

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/logging"
	"github.com/briandeitte/photo-copy/internal/media"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	uploadURL      = "https://photoslibrary.googleapis.com/v1/uploads"
	batchCreateURL = "https://photoslibrary.googleapis.com/v1/mediaItems:batchCreate"
	dailyLimit     = 10000
)

var oauthScopes = []string{
	"https://www.googleapis.com/auth/photoslibrary.appendonly",
}

// Client wraps an authenticated HTTP client for Google Photos API.
type Client struct {
	httpClient *http.Client
	log        *logging.Logger
	configDir  string
}

// NewClient creates a new Google Photos client with OAuth2 authentication.
func NewClient(ctx context.Context, cfg *config.GoogleConfig, configDir string, log *logging.Logger) (*Client, error) {
	oauthCfg := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Scopes:       oauthScopes,
		Endpoint:     google.Endpoint,
		RedirectURL:  "urn:ietf:wg:oauth:2.0:oob",
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
func (c *Client) Upload(ctx context.Context, inputDir string) error {
	files, err := collectMediaFiles(inputDir)
	if err != nil {
		return fmt.Errorf("collecting media files: %w", err)
	}

	c.log.Info("found %d media files in %s", len(files), inputDir)

	logPath := filepath.Join(inputDir, ".photo-copy-upload.log")
	uploaded, err := loadUploadLog(logPath)
	if err != nil {
		return fmt.Errorf("loading upload log: %w", err)
	}

	// Filter already uploaded files
	var toUpload []string
	for _, f := range files {
		if !uploaded[filepath.Base(f)] {
			toUpload = append(toUpload, f)
		}
	}

	if len(toUpload) == 0 {
		c.log.Info("all files already uploaded")
		return nil
	}

	if len(toUpload) > dailyLimit {
		c.log.Info("limiting upload to %d files (daily limit)", dailyLimit)
		toUpload = toUpload[:dailyLimit]
	}

	c.log.Info("uploading %d files (%d already uploaded)", len(toUpload), len(uploaded))

	totalUploaded := 0
	totalErrors := 0

	for i, filePath := range toUpload {
		filename := filepath.Base(filePath)
		c.log.Info("[%d/%d] uploading %s", i+1, len(toUpload), filename)
		c.log.Debug("reading file: %s", filePath)

		uploadToken, err := c.uploadBytes(filePath, filename)
		if err != nil {
			totalErrors++
			c.log.Error("upload failed for %s: %v", filename, err)
			continue
		}

		c.log.Debug("got upload token for %s, creating media item", filename)
		if err := c.createMediaItem(uploadToken, filename); err != nil {
			totalErrors++
			c.log.Error("create media item failed for %s: %v", filename, err)
			continue
		}

		if err := appendUploadLog(logPath, filename); err != nil {
			c.log.Error("failed to update upload log: %v", err)
		}

		totalUploaded++
		c.log.Debug("successfully uploaded %s", filename)
	}

	parts := []string{fmt.Sprintf("%d uploaded", totalUploaded)}
	if len(uploaded) > 0 {
		parts = append(parts, fmt.Sprintf("%d already existed", len(uploaded)))
	}
	if totalErrors > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", totalErrors))
	}
	c.log.Info("upload complete: %s", strings.Join(parts, ", "))
	return nil
}

// uploadBytes uploads raw file bytes and returns an upload token.
func (c *Client) uploadBytes(filePath, filename string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("reading file: %w", err)
	}

	req, err := http.NewRequest("POST", uploadURL, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Goog-Upload-File-Name", filename)
	req.Header.Set("X-Goog-Upload-Protocol", "raw")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	return string(body), nil
}

// createMediaItem creates a media item in Google Photos from an upload token.
func (c *Client) createMediaItem(uploadToken, filename string) error {
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

	req, err := http.NewRequest("POST", batchCreateURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("create request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

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

// appendUploadLog adds a filename to the upload log.
func appendUploadLog(path, filename string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening upload log: %w", err)
	}
	defer f.Close()

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

// runOAuthFlow runs an interactive OAuth2 flow via the terminal.
func runOAuthFlow(ctx context.Context, cfg *oauth2.Config) (*oauth2.Token, error) {
	authURL := cfg.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Println("Visit this URL to authorize the application:")
	fmt.Println(authURL)
	fmt.Println()
	fmt.Print("Enter the authorization code: ")

	reader := bufio.NewReader(os.Stdin)
	code, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("reading authorization code: %w", err)
	}

	code = strings.TrimSpace(code)
	token, err := cfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("exchanging code for token: %w", err)
	}

	return token, nil
}
