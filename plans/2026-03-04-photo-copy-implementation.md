# Photo Copy Go CLI - Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a Go CLI that copies photos/videos between Flickr, Google Photos, and local directories, with debug logging throughout.

**Architecture:** Cobra CLI with subcommands (`flickr download/upload`, `google-photos upload/import-takeout`, `config flickr/google`). Each service gets its own `internal/` package. A shared debug logger is passed through all operations. OAuth tokens and API keys stored in `~/.config/photo-copy/`.

**Tech Stack:** Go, cobra, golang.org/x/oauth2, google.golang.org/api, schollz/progressbar/v3

---

### Task 1: Project Scaffolding - Go Module and Root Command

**Files:**
- Create: `go.mod`
- Create: `cmd/photo-copy/main.go`
- Create: `internal/cli/root.go`

**Step 1: Initialize Go module**

Run: `cd /Users/briandeitte/photo-copy && go mod init github.com/briandeitte/photo-copy`

**Step 2: Install cobra dependency**

Run: `go get github.com/spf13/cobra@latest`

**Step 3: Write root command with --debug flag**

`internal/cli/root.go`:
```go
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var debug bool

func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "photo-copy",
		Short: "Copy photos and videos between Flickr, Google Photos, S3, and local directories",
	}

	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable verbose debug logging to stderr")

	return rootCmd
}

func Execute() {
	rootCmd := NewRootCmd()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

`cmd/photo-copy/main.go`:
```go
package main

import "github.com/briandeitte/photo-copy/internal/cli"

func main() {
	cli.Execute()
}
```

**Step 4: Verify it builds and runs**

Run: `go build -o photo-copy ./cmd/photo-copy && ./photo-copy --help`
Expected: Help output showing "Copy photos and videos..." and the `--debug` flag.

**Step 5: Clean up old Python scaffolding**

Delete: `src/`, `pyproject.toml`, `.venv/` (the old Python project files are being replaced by Go).

**Step 6: Commit**

```bash
git add go.mod go.sum cmd/ internal/cli/
git rm -r src/ pyproject.toml
git commit -m "feat: initialize Go project with cobra root command and --debug flag"
```

---

### Task 2: Debug Logger

**Files:**
- Create: `internal/logging/logger.go`
- Create: `internal/logging/logger_test.go`

**Step 1: Write the test**

`internal/logging/logger_test.go`:
```go
package logging

import (
	"bytes"
	"testing"
)

func TestDebugLogger_Enabled(t *testing.T) {
	var buf bytes.Buffer
	log := New(true, &buf)

	log.Debug("downloading file %s", "photo.jpg")

	got := buf.String()
	if got == "" {
		t.Fatal("expected debug output, got empty string")
	}
	if !bytes.Contains([]byte(got), []byte("downloading file photo.jpg")) {
		t.Fatalf("unexpected output: %s", got)
	}
}

func TestDebugLogger_Disabled(t *testing.T) {
	var buf bytes.Buffer
	log := New(false, &buf)

	log.Debug("this should not appear")

	if buf.Len() != 0 {
		t.Fatalf("expected no output when debug disabled, got: %s", buf.String())
	}
}

func TestInfoLogger_AlwaysOutputs(t *testing.T) {
	var buf bytes.Buffer
	log := New(false, &buf)

	log.Info("always visible")

	got := buf.String()
	if got == "" {
		t.Fatal("expected info output even when debug disabled")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/briandeitte/photo-copy && go test ./internal/logging/ -v`
Expected: FAIL - functions not defined.

**Step 3: Write the implementation**

`internal/logging/logger.go`:
```go
package logging

import (
	"fmt"
	"io"
	"os"
	"time"
)

type Logger struct {
	debug  bool
	writer io.Writer
}

func New(debug bool, writer io.Writer) *Logger {
	if writer == nil {
		writer = os.Stderr
	}
	return &Logger{debug: debug, writer: writer}
}

func (l *Logger) Debug(format string, args ...any) {
	if !l.debug {
		return
	}
	l.write("DEBUG", format, args...)
}

func (l *Logger) Info(format string, args ...any) {
	l.write("INFO", format, args...)
}

func (l *Logger) Error(format string, args ...any) {
	l.write("ERROR", format, args...)
}

func (l *Logger) write(level, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("15:04:05")
	fmt.Fprintf(l.writer, "[%s] %s: %s\n", timestamp, level, msg)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/logging/ -v`
Expected: All 3 tests PASS.

**Step 5: Commit**

```bash
git add internal/logging/
git commit -m "feat: add debug logger with configurable output levels"
```

---

### Task 3: Config Package - Credential Storage

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

**Step 1: Write the test**

`internal/config/config_test.go`:
```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoadFlickrConfig(t *testing.T) {
	tmpDir := t.TempDir()

	fc := &FlickrConfig{
		APIKey:    "test-key",
		APISecret: "test-secret",
		OAuthToken:       "token",
		OAuthTokenSecret: "token-secret",
	}

	if err := SaveFlickrConfig(tmpDir, fc); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := LoadFlickrConfig(tmpDir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if loaded.APIKey != fc.APIKey || loaded.APISecret != fc.APISecret {
		t.Fatalf("loaded config doesn't match: got %+v", loaded)
	}
}

func TestLoadFlickrConfig_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := LoadFlickrConfig(tmpDir)
	if err == nil {
		t.Fatal("expected error for missing config, got nil")
	}
}

func TestSaveAndLoadGoogleConfig(t *testing.T) {
	tmpDir := t.TempDir()

	gc := &GoogleConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
	}

	if err := SaveGoogleConfig(tmpDir, gc); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := LoadGoogleConfig(tmpDir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if loaded.ClientID != gc.ClientID {
		t.Fatalf("loaded config doesn't match: got %+v", loaded)
	}
}

func TestConfigDir_CreatesDir(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "newdir")

	fc := &FlickrConfig{APIKey: "k", APISecret: "s"}
	if err := SaveFlickrConfig(subDir, fc); err != nil {
		t.Fatalf("save to new dir failed: %v", err)
	}

	if _, err := os.Stat(subDir); os.IsNotExist(err) {
		t.Fatal("expected config dir to be created")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -v`
Expected: FAIL - types not defined.

**Step 3: Write the implementation**

`internal/config/config.go`:
```go
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	flickrFile = "flickr.json"
	googleFile = "google_credentials.json"
	googleTokenFile = "google_token.json"
)

type FlickrConfig struct {
	APIKey           string `json:"api_key"`
	APISecret        string `json:"api_secret"`
	OAuthToken       string `json:"oauth_token,omitempty"`
	OAuthTokenSecret string `json:"oauth_token_secret,omitempty"`
}

type GoogleConfig struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

func DefaultDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "photo-copy")
}

func SaveFlickrConfig(configDir string, cfg *FlickrConfig) error {
	return saveJSON(configDir, flickrFile, cfg)
}

func LoadFlickrConfig(configDir string) (*FlickrConfig, error) {
	var cfg FlickrConfig
	if err := loadJSON(configDir, flickrFile, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func SaveGoogleConfig(configDir string, cfg *GoogleConfig) error {
	return saveJSON(configDir, googleFile, cfg)
}

func LoadGoogleConfig(configDir string) (*GoogleConfig, error) {
	var cfg GoogleConfig
	if err := loadJSON(configDir, googleFile, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func SaveGoogleToken(configDir string, token any) error {
	return saveJSON(configDir, googleTokenFile, token)
}

func LoadGoogleToken(configDir string) (map[string]any, error) {
	var token map[string]any
	if err := loadJSON(configDir, googleTokenFile, &token); err != nil {
		return nil, err
	}
	return token, nil
}

func saveJSON(configDir, filename string, v any) error {
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	path := filepath.Join(configDir, filename)
	return os.WriteFile(path, data, 0600)
}

func loadJSON(configDir, filename string, v any) error {
	path := filepath.Join(configDir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filename, err)
	}
	return json.Unmarshal(data, v)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: All 4 tests PASS.

**Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: add config package for credential storage"
```

---

### Task 4: Media File Type Detection

**Files:**
- Create: `internal/media/media.go`
- Create: `internal/media/media_test.go`

**Step 1: Write the test**

`internal/media/media_test.go`:
```go
package media

import "testing"

func TestIsSupportedFile(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"photo.jpg", true},
		{"photo.JPEG", true},
		{"video.mp4", true},
		{"video.MOV", true},
		{"photo.heic", true},
		{"photo.webp", true},
		{"photo.png", true},
		{"photo.tiff", true},
		{"photo.gif", true},
		{"video.avi", true},
		{"video.mkv", true},
		{"readme.txt", false},
		{"metadata.json", false},
		{"photo.jpg.bak", false},
		{".DS_Store", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSupportedFile(tt.name); got != tt.want {
				t.Errorf("IsSupportedFile(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/media/ -v`
Expected: FAIL - function not defined.

**Step 3: Write the implementation**

`internal/media/media.go`:
```go
package media

import (
	"path/filepath"
	"strings"
)

var supportedExtensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".tiff": true,
	".tif":  true,
	".gif":  true,
	".heic": true,
	".webp": true,
	".mp4":  true,
	".mov":  true,
	".avi":  true,
	".mkv":  true,
}

func IsSupportedFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return supportedExtensions[ext]
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/media/ -v`
Expected: All tests PASS.

**Step 5: Commit**

```bash
git add internal/media/
git commit -m "feat: add media file type detection"
```

---

### Task 5: Config CLI Commands

**Files:**
- Create: `internal/cli/config.go`
- Modify: `internal/cli/root.go`

**Step 1: Write the config subcommands**

`internal/cli/config.go`:
```go
package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Configure API credentials",
	}

	cmd.AddCommand(newConfigFlickrCmd())
	cmd.AddCommand(newConfigGoogleCmd())
	return cmd
}

func newConfigFlickrCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "flickr",
		Short: "Set up Flickr API credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			reader := bufio.NewReader(os.Stdin)

			fmt.Println("Flickr API Setup")
			fmt.Println("Get your API key at: https://www.flickr.com/services/apps/create/")
			fmt.Println()

			fmt.Print("API Key: ")
			apiKey, _ := reader.ReadString('\n')
			apiKey = strings.TrimSpace(apiKey)

			fmt.Print("API Secret: ")
			apiSecret, _ := reader.ReadString('\n')
			apiSecret = strings.TrimSpace(apiSecret)

			if apiKey == "" || apiSecret == "" {
				return fmt.Errorf("API key and secret are required")
			}

			cfg := &config.FlickrConfig{
				APIKey:    apiKey,
				APISecret: apiSecret,
			}

			configDir := config.DefaultDir()
			if err := config.SaveFlickrConfig(configDir, cfg); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			fmt.Printf("\nFlickr credentials saved to %s\n", configDir)
			return nil
		},
	}
}

func newConfigGoogleCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "google",
		Short: "Set up Google OAuth credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			reader := bufio.NewReader(os.Stdin)

			fmt.Println("Google OAuth Setup")
			fmt.Println("Create credentials at: https://console.cloud.google.com/apis/credentials")
			fmt.Println("Enable the Photos Library API at: https://console.cloud.google.com/apis/library/photoslibrary.googleapis.com")
			fmt.Println()

			fmt.Print("Client ID: ")
			clientID, _ := reader.ReadString('\n')
			clientID = strings.TrimSpace(clientID)

			fmt.Print("Client Secret: ")
			clientSecret, _ := reader.ReadString('\n')
			clientSecret = strings.TrimSpace(clientSecret)

			if clientID == "" || clientSecret == "" {
				return fmt.Errorf("client ID and secret are required")
			}

			cfg := &config.GoogleConfig{
				ClientID:     clientID,
				ClientSecret: clientSecret,
			}

			configDir := config.DefaultDir()
			if err := config.SaveGoogleConfig(configDir, cfg); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			fmt.Printf("\nGoogle credentials saved to %s\n", configDir)
			return nil
		},
	}
}
```

**Step 2: Wire config command into root**

Update `internal/cli/root.go` - add `rootCmd.AddCommand(newConfigCmd())` inside `NewRootCmd()`, after the flag definition.

**Step 3: Verify it builds and runs**

Run: `go build -o photo-copy ./cmd/photo-copy && ./photo-copy config --help`
Expected: Shows `flickr` and `google` subcommands.

**Step 4: Commit**

```bash
git add internal/cli/
git commit -m "feat: add interactive config commands for Flickr and Google credentials"
```

---

### Task 6: Flickr Download

**Files:**
- Create: `internal/flickr/flickr.go`
- Create: `internal/flickr/flickr_test.go`
- Create: `internal/cli/flickr.go`

This task implements Flickr photo download using the Flickr REST API directly (no third-party Flickr library needed - the API is simple enough to call with `net/http`).

**Step 1: Write the test for the download tracking/resumability logic**

`internal/flickr/flickr_test.go`:
```go
package flickr

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTransferLog_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	log, err := loadTransferLog(filepath.Join(tmpDir, "transfer.log"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(log) != 0 {
		t.Fatalf("expected empty log, got %d entries", len(log))
	}
}

func TestTransferLog_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "transfer.log")

	if err := appendTransferLog(logPath, "photo1.jpg"); err != nil {
		t.Fatalf("append failed: %v", err)
	}
	if err := appendTransferLog(logPath, "photo2.jpg"); err != nil {
		t.Fatalf("append failed: %v", err)
	}

	log, err := loadTransferLog(logPath)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if !log["photo1.jpg"] || !log["photo2.jpg"] {
		t.Fatalf("expected both photos in log, got: %v", log)
	}
}

func TestBuildAPIURL(t *testing.T) {
	url := buildAPIURL("flickr.people.getPhotos", "testkey", map[string]string{
		"user_id": "me",
		"page":    "1",
	})

	if url == "" {
		t.Fatal("expected non-empty URL")
	}
	// Should contain the method and API key
	if !contains(url, "flickr.people.getPhotos") {
		t.Fatalf("URL missing method: %s", url)
	}
	if !contains(url, "testkey") {
		t.Fatalf("URL missing API key: %s", url)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/flickr/ -v`
Expected: FAIL - functions not defined.

**Step 3: Write the Flickr implementation**

`internal/flickr/flickr.go`:
```go
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
	"strings"
	"time"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/logging"
	"github.com/briandeitte/photo-copy/internal/media"
	"github.com/schollz/progressbar/v3"
)

const (
	apiBaseURL = "https://api.flickr.com/services/rest/"
	perPage    = 500
)

type Client struct {
	cfg    *config.FlickrConfig
	http   *http.Client
	log    *logging.Logger
}

func NewClient(cfg *config.FlickrConfig, log *logging.Logger) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: 60 * time.Second},
		log:  log,
	}
}

func buildAPIURL(method, apiKey string, params map[string]string) string {
	v := url.Values{}
	v.Set("method", method)
	v.Set("api_key", apiKey)
	v.Set("format", "json")
	v.Set("nojsoncallback", "1")
	for k, val := range params {
		v.Set(k, val)
	}
	return apiBaseURL + "?" + v.Encode()
}

type photosResponse struct {
	Photos struct {
		Page    int `json:"page"`
		Pages   int `json:"pages"`
		Total   int `json:"total"`
		Photo   []struct {
			ID       string `json:"id"`
			Secret   string `json:"secret"`
			Server   string `json:"server"`
			Title    string `json:"title"`
			OriginalSecret string `json:"originalsecret"`
			OriginalFormat string `json:"originalformat"`
		} `json:"photo"`
	} `json:"photos"`
	Stat string `json:"stat"`
}

type sizesResponse struct {
	Sizes struct {
		Size []struct {
			Label  string `json:"label"`
			Source string `json:"source"`
		} `json:"size"`
	} `json:"sizes"`
	Stat string `json:"stat"`
}

func (c *Client) Download(ctx context.Context, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	logPath := filepath.Join(outputDir, ".photo-copy-transfer.log")
	transferred, err := loadTransferLog(logPath)
	if err != nil {
		return fmt.Errorf("loading transfer log: %w", err)
	}
	c.log.Debug("loaded transfer log with %d entries", len(transferred))

	page := 1
	totalDownloaded := 0

	for {
		c.log.Debug("fetching photo list page %d", page)
		apiURL := buildAPIURL("flickr.people.getPhotos", c.cfg.APIKey, map[string]string{
			"user_id":  "me",
			"page":     fmt.Sprintf("%d", page),
			"per_page": fmt.Sprintf("%d", perPage),
			"extras":   "url_o,original_format",
		})

		resp, err := c.http.Get(apiURL)
		if err != nil {
			return fmt.Errorf("fetching photos page %d: %w", page, err)
		}

		var photos photosResponse
		if err := json.NewDecoder(resp.Body).Decode(&photos); err != nil {
			resp.Body.Close()
			return fmt.Errorf("decoding photos response: %w", err)
		}
		resp.Body.Close()

		if photos.Stat != "ok" {
			return fmt.Errorf("flickr API error: %s", photos.Stat)
		}

		c.log.Info("page %d/%d - %d photos total", page, photos.Photos.Pages, photos.Photos.Total)

		if page == 1 {
			fmt.Fprintf(os.Stderr, "Found %d photos to download\n", photos.Photos.Total)
		}

		for _, photo := range photos.Photos.Photo {
			ext := photo.OriginalFormat
			if ext == "" {
				ext = "jpg"
			}
			filename := fmt.Sprintf("%s.%s", photo.ID, ext)

			if transferred[filename] {
				c.log.Debug("skipping %s (already downloaded)", filename)
				continue
			}

			photoURL, err := c.getOriginalURL(photo.ID)
			if err != nil {
				c.log.Error("getting URL for %s: %v", photo.ID, err)
				continue
			}

			c.log.Debug("downloading %s -> %s", photoURL, filename)

			if err := c.downloadFile(photoURL, filepath.Join(outputDir, filename)); err != nil {
				c.log.Error("downloading %s: %v", filename, err)
				continue
			}

			if err := appendTransferLog(logPath, filename); err != nil {
				c.log.Error("updating transfer log: %v", err)
			}

			totalDownloaded++
		}

		if page >= photos.Photos.Pages {
			break
		}
		page++
	}

	fmt.Fprintf(os.Stderr, "Downloaded %d photos\n", totalDownloaded)
	return nil
}

func (c *Client) getOriginalURL(photoID string) (string, error) {
	apiURL := buildAPIURL("flickr.photos.getSizes", c.cfg.APIKey, map[string]string{
		"photo_id": photoID,
	})

	resp, err := c.http.Get(apiURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var sizes sizesResponse
	if err := json.NewDecoder(resp.Body).Decode(&sizes); err != nil {
		return "", err
	}

	// Prefer "Original", fall back to "Large"
	for _, s := range sizes.Sizes.Size {
		if s.Label == "Original" {
			return s.Source, nil
		}
	}
	for _, s := range sizes.Sizes.Size {
		if s.Label == "Large" {
			return s.Source, nil
		}
	}

	if len(sizes.Sizes.Size) > 0 {
		return sizes.Sizes.Size[len(sizes.Sizes.Size)-1].Source, nil
	}

	return "", fmt.Errorf("no sizes available for photo %s", photoID)
}

func (c *Client) downloadFile(url, destPath string) error {
	resp, err := c.http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

func (c *Client) Upload(ctx context.Context, inputDir string) error {
	entries, err := os.ReadDir(inputDir)
	if err != nil {
		return fmt.Errorf("reading input dir: %w", err)
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if media.IsSupportedFile(e.Name()) {
			files = append(files, e.Name())
		} else {
			c.log.Debug("skipping non-media file: %s", e.Name())
		}
	}

	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "No media files found to upload")
		return nil
	}

	fmt.Fprintf(os.Stderr, "Found %d media files to upload\n", len(files))

	bar := progressbar.Default(int64(len(files)), "Uploading to Flickr")

	for _, filename := range files {
		c.log.Debug("uploading %s", filename)

		filePath := filepath.Join(inputDir, filename)
		if err := c.uploadFile(filePath); err != nil {
			c.log.Error("uploading %s: %v", filename, err)
			bar.Add(1)
			continue
		}

		bar.Add(1)
	}

	return nil
}

func (c *Client) uploadFile(filePath string) error {
	// Flickr upload uses a different endpoint: https://up.flickr.com/services/upload/
	// This requires OAuth 1.0a signing, which will be implemented in the OAuth task.
	// For now, this is a placeholder that will be completed after OAuth is wired in.
	return fmt.Errorf("upload requires OAuth - run 'photo-copy config flickr' first, then authenticate")
}

// Transfer log functions for resumability

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

func appendTransferLog(path, filename string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = fmt.Fprintln(f, filename)
	return err
}
```

**Step 4: Install progressbar dependency**

Run: `go get github.com/schollz/progressbar/v3@latest`

**Step 5: Run tests to verify they pass**

Run: `go test ./internal/flickr/ -v`
Expected: All tests PASS.

**Step 6: Wire up the Flickr CLI commands**

`internal/cli/flickr.go`:
```go
package cli

import (
	"context"
	"fmt"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/flickr"
	"github.com/briandeitte/photo-copy/internal/logging"
	"github.com/spf13/cobra"
)

func newFlickrCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "flickr",
		Short: "Flickr upload and download commands",
	}

	cmd.AddCommand(newFlickrDownloadCmd())
	cmd.AddCommand(newFlickrUploadCmd())
	return cmd
}

func newFlickrDownloadCmd() *cobra.Command {
	var outputDir string

	cmd := &cobra.Command{
		Use:   "download",
		Short: "Download all photos from Flickr",
		RunE: func(cmd *cobra.Command, args []string) error {
			if outputDir == "" {
				return fmt.Errorf("--output-dir is required")
			}

			cfg, err := config.LoadFlickrConfig(config.DefaultDir())
			if err != nil {
				return fmt.Errorf("loading Flickr config (run 'photo-copy config flickr' first): %w", err)
			}

			log := logging.New(debug, nil)
			client := flickr.NewClient(cfg, log)
			return client.Download(context.Background(), outputDir)
		},
	}

	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Directory to save downloaded photos")
	cmd.MarkFlagRequired("output-dir")
	return cmd
}

func newFlickrUploadCmd() *cobra.Command {
	var inputDir string

	cmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload photos to Flickr",
		RunE: func(cmd *cobra.Command, args []string) error {
			if inputDir == "" {
				return fmt.Errorf("--input-dir is required")
			}

			cfg, err := config.LoadFlickrConfig(config.DefaultDir())
			if err != nil {
				return fmt.Errorf("loading Flickr config (run 'photo-copy config flickr' first): %w", err)
			}

			log := logging.New(debug, nil)
			client := flickr.NewClient(cfg, log)
			return client.Upload(context.Background(), inputDir)
		},
	}

	cmd.Flags().StringVar(&inputDir, "input-dir", "", "Directory containing photos to upload")
	cmd.MarkFlagRequired("input-dir")
	return cmd
}
```

**Step 7: Add flickr command to root**

Update `internal/cli/root.go` - add `rootCmd.AddCommand(newFlickrCmd())` inside `NewRootCmd()`.

**Step 8: Verify it builds**

Run: `go build -o photo-copy ./cmd/photo-copy && ./photo-copy flickr --help`
Expected: Shows `download` and `upload` subcommands.

**Step 9: Commit**

```bash
git add internal/flickr/ internal/cli/flickr.go internal/cli/root.go go.mod go.sum
git commit -m "feat: add Flickr download and upload with resumability and debug logging"
```

---

### Task 7: Flickr OAuth 1.0a for Upload

**Files:**
- Create: `internal/flickr/oauth.go`
- Modify: `internal/flickr/flickr.go`

Flickr upload requires OAuth 1.0a signed requests. The download endpoint uses API key auth (public photos), but upload needs full OAuth.

**Step 1: Write OAuth implementation**

`internal/flickr/oauth.go`:
```go
package flickr

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"math/rand"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/briandeitte/photo-copy/internal/config"
)

func oauthSign(method, endpoint string, params map[string]string, cfg *config.FlickrConfig) string {
	// Add OAuth params
	params["oauth_consumer_key"] = cfg.APIKey
	params["oauth_token"] = cfg.OAuthToken
	params["oauth_signature_method"] = "HMAC-SHA1"
	params["oauth_timestamp"] = fmt.Sprintf("%d", time.Now().Unix())
	params["oauth_nonce"] = generateNonce()
	params["oauth_version"] = "1.0"

	// Build base string
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, url.QueryEscape(k)+"="+url.QueryEscape(params[k]))
	}
	paramString := strings.Join(pairs, "&")

	baseString := method + "&" + url.QueryEscape(endpoint) + "&" + url.QueryEscape(paramString)

	// Sign
	signingKey := url.QueryEscape(cfg.APISecret) + "&" + url.QueryEscape(cfg.OAuthTokenSecret)
	mac := hmac.New(sha1.New, []byte(signingKey))
	mac.Write([]byte(baseString))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	params["oauth_signature"] = signature
	return signature
}

func generateNonce() string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 32)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}
```

**Step 2: Update uploadFile in flickr.go to use OAuth-signed multipart upload**

Replace the placeholder `uploadFile` method with a real implementation that:
- Opens the file
- Creates a multipart form with the photo data
- Signs the request with OAuth 1.0a
- Posts to `https://up.flickr.com/services/upload/`

```go
func (c *Client) uploadFile(filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("photo", filepath.Base(filePath))
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, f); err != nil {
		return err
	}

	params := map[string]string{}
	oauthSign("POST", "https://up.flickr.com/services/upload/", params, c.cfg)

	for k, v := range params {
		if strings.HasPrefix(k, "oauth_") {
			writer.WriteField(k, v)
		}
	}

	writer.Close()

	req, err := http.NewRequest("POST", "https://up.flickr.com/services/upload/", body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
```

**Step 3: Add necessary imports to flickr.go**

Add `"bytes"`, `"mime/multipart"` to the import block.

**Step 4: Verify it builds**

Run: `go build -o photo-copy ./cmd/photo-copy`
Expected: Builds successfully.

**Step 5: Commit**

```bash
git add internal/flickr/
git commit -m "feat: add Flickr OAuth 1.0a signing for upload"
```

---

### Task 8: Flickr OAuth Token Flow in Config

**Files:**
- Modify: `internal/cli/config.go`
- Modify: `internal/flickr/oauth.go`

After the user enters their API key/secret, we need to do the OAuth 1.0a dance to get access tokens. This involves:
1. Get a request token from Flickr
2. Direct user to authorize URL in browser
3. User enters verifier code
4. Exchange for access token
5. Save tokens to config

**Step 1: Add OAuth token exchange functions to oauth.go**

Add these functions to `internal/flickr/oauth.go`:

```go
func GetRequestToken(cfg *config.FlickrConfig) (string, string, string, error) {
	params := map[string]string{
		"oauth_callback": "oob",
	}

	// Use a temporary config with empty token for request token step
	tempCfg := &config.FlickrConfig{
		APIKey:           cfg.APIKey,
		APISecret:        cfg.APISecret,
		OAuthToken:       "",
		OAuthTokenSecret: "",
	}

	oauthSign("GET", "https://www.flickr.com/services/oauth/request_token", params, tempCfg)

	v := url.Values{}
	for k, val := range params {
		v.Set(k, val)
	}

	resp, err := http.Get("https://www.flickr.com/services/oauth/request_token?" + v.Encode())
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	vals, _ := url.ParseQuery(string(body))

	token := vals.Get("oauth_token")
	tokenSecret := vals.Get("oauth_token_secret")
	authURL := "https://www.flickr.com/services/oauth/authorize?oauth_token=" + token

	return token, tokenSecret, authURL, nil
}

func ExchangeToken(cfg *config.FlickrConfig, requestToken, requestTokenSecret, verifier string) (string, string, error) {
	tempCfg := &config.FlickrConfig{
		APIKey:           cfg.APIKey,
		APISecret:        cfg.APISecret,
		OAuthToken:       requestToken,
		OAuthTokenSecret: requestTokenSecret,
	}

	params := map[string]string{
		"oauth_verifier": verifier,
	}

	oauthSign("GET", "https://www.flickr.com/services/oauth/access_token", params, tempCfg)

	v := url.Values{}
	for k, val := range params {
		v.Set(k, val)
	}

	resp, err := http.Get("https://www.flickr.com/services/oauth/access_token?" + v.Encode())
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	vals, _ := url.ParseQuery(string(body))

	return vals.Get("oauth_token"), vals.Get("oauth_token_secret"), nil
}
```

**Step 2: Update config flickr command to include OAuth flow**

After saving API key/secret, add the OAuth dance:

```go
fmt.Println("\nStarting OAuth authorization...")
reqToken, reqSecret, authURL, err := flickr.GetRequestToken(cfg)
if err != nil {
    return fmt.Errorf("getting request token: %w", err)
}

fmt.Printf("\nOpen this URL in your browser:\n%s\n\n", authURL)
fmt.Print("Enter the verification code: ")
verifier, _ := reader.ReadString('\n')
verifier = strings.TrimSpace(verifier)

accessToken, accessSecret, err := flickr.ExchangeToken(cfg, reqToken, reqSecret, verifier)
if err != nil {
    return fmt.Errorf("exchanging token: %w", err)
}

cfg.OAuthToken = accessToken
cfg.OAuthTokenSecret = accessSecret

if err := config.SaveFlickrConfig(configDir, cfg); err != nil {
    return fmt.Errorf("saving config with tokens: %w", err)
}

fmt.Println("Flickr OAuth complete! Credentials saved.")
```

**Step 3: Add necessary imports to oauth.go**

Add `"io"`, `"net/http"` to the import block.

**Step 4: Verify it builds**

Run: `go build -o photo-copy ./cmd/photo-copy`
Expected: Builds successfully.

**Step 5: Commit**

```bash
git add internal/flickr/oauth.go internal/cli/config.go
git commit -m "feat: add Flickr OAuth token exchange flow"
```

---

### Task 9: Google Photos Upload

**Files:**
- Create: `internal/google/google.go`
- Create: `internal/google/google_test.go`
- Create: `internal/cli/google.go`

Google Photos upload uses the Photos Library API. The flow is:
1. Upload bytes to get an upload token
2. Create a media item using that token

**Step 1: Write the test**

`internal/google/google_test.go`:
```go
package google

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadUploadLog_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	log, err := loadUploadLog(filepath.Join(tmpDir, "upload.log"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(log) != 0 {
		t.Fatalf("expected empty log, got %d entries", len(log))
	}
}

func TestUploadLog_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "upload.log")

	if err := appendUploadLog(logPath, "photo1.jpg"); err != nil {
		t.Fatalf("append failed: %v", err)
	}
	if err := appendUploadLog(logPath, "photo2.jpg"); err != nil {
		t.Fatalf("append failed: %v", err)
	}

	log, err := loadUploadLog(logPath)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if !log["photo1.jpg"] || !log["photo2.jpg"] {
		t.Fatalf("expected both photos in log, got: %v", log)
	}
}

func TestCollectMediaFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	os.WriteFile(filepath.Join(tmpDir, "photo.jpg"), []byte("fake"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "video.mp4"), []byte("fake"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("fake"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "metadata.json"), []byte("fake"), 0644)

	files, err := collectMediaFiles(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("expected 2 media files, got %d: %v", len(files), files)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/google/ -v`
Expected: FAIL - functions not defined.

**Step 3: Write the implementation**

`internal/google/google.go`:
```go
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
	"github.com/schollz/progressbar/v3"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	uploadURL      = "https://photoslibrary.googleapis.com/v1/uploads"
	batchCreateURL = "https://photoslibrary.googleapis.com/v1/mediaItems:batchCreate"
	dailyLimit     = 10000
)

var scopes = []string{
	"https://www.googleapis.com/auth/photoslibrary.appendonly",
}

type Client struct {
	httpClient *http.Client
	log        *logging.Logger
}

func NewClient(ctx context.Context, cfg *config.GoogleConfig, configDir string, log *logging.Logger) (*Client, error) {
	oauthCfg := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Scopes:       scopes,
		Endpoint:     google.Endpoint,
		RedirectURL:  "urn:ietf:wg:oauth:2.0:oob",
	}

	tokenData, err := config.LoadGoogleToken(configDir)
	if err != nil {
		// Need to do OAuth flow
		token, err := doOAuthFlow(ctx, oauthCfg)
		if err != nil {
			return nil, fmt.Errorf("OAuth flow: %w", err)
		}
		if err := config.SaveGoogleToken(configDir, token); err != nil {
			return nil, fmt.Errorf("saving token: %w", err)
		}
		httpClient := oauthCfg.Client(ctx, token)
		return &Client{httpClient: httpClient, log: log}, nil
	}

	// Reconstruct token from saved data
	token := &oauth2.Token{
		AccessToken:  fmt.Sprintf("%v", tokenData["access_token"]),
		RefreshToken: fmt.Sprintf("%v", tokenData["refresh_token"]),
		TokenType:    fmt.Sprintf("%v", tokenData["token_type"]),
	}

	httpClient := oauthCfg.Client(ctx, token)
	return &Client{httpClient: httpClient, log: log}, nil
}

func doOAuthFlow(ctx context.Context, cfg *oauth2.Config) (*oauth2.Token, error) {
	authURL := cfg.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("\nOpen this URL in your browser:\n%s\n\n", authURL)
	fmt.Print("Enter the authorization code: ")

	reader := bufio.NewReader(os.Stdin)
	code, _ := reader.ReadString('\n')
	code = strings.TrimSpace(code)

	return cfg.Exchange(ctx, code)
}

func (c *Client) Upload(ctx context.Context, inputDir string) error {
	files, err := collectMediaFiles(inputDir)
	if err != nil {
		return fmt.Errorf("collecting media files: %w", err)
	}

	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "No media files found to upload")
		return nil
	}

	logPath := filepath.Join(inputDir, ".photo-copy-upload.log")
	uploaded, err := loadUploadLog(logPath)
	if err != nil {
		return fmt.Errorf("loading upload log: %w", err)
	}
	c.log.Debug("loaded upload log with %d entries", len(uploaded))

	var toUpload []string
	for _, f := range files {
		name := filepath.Base(f)
		if uploaded[name] {
			c.log.Debug("skipping %s (already uploaded)", name)
			continue
		}
		toUpload = append(toUpload, f)
	}

	if len(toUpload) == 0 {
		fmt.Fprintln(os.Stderr, "All files already uploaded")
		return nil
	}

	if len(toUpload) > dailyLimit {
		fmt.Fprintf(os.Stderr, "Warning: %d files to upload exceeds daily limit of %d. Will upload first %d.\n",
			len(toUpload), dailyLimit, dailyLimit)
		toUpload = toUpload[:dailyLimit]
	}

	fmt.Fprintf(os.Stderr, "Uploading %d media files to Google Photos\n", len(toUpload))
	bar := progressbar.Default(int64(len(toUpload)), "Uploading")

	for _, filePath := range toUpload {
		filename := filepath.Base(filePath)
		c.log.Debug("uploading %s", filePath)

		uploadToken, err := c.uploadBytes(filePath, filename)
		if err != nil {
			c.log.Error("uploading bytes for %s: %v", filename, err)
			bar.Add(1)
			continue
		}

		c.log.Debug("creating media item for %s (token: %s)", filename, uploadToken[:20]+"...")
		if err := c.createMediaItem(uploadToken, filename); err != nil {
			c.log.Error("creating media item for %s: %v", filename, err)
			bar.Add(1)
			continue
		}

		if err := appendUploadLog(logPath, filename); err != nil {
			c.log.Error("updating upload log: %v", err)
		}

		bar.Add(1)
	}

	return nil
}

func (c *Client) uploadBytes(filePath, filename string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	req, err := http.NewRequest("POST", uploadURL, f)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Goog-Upload-File-Name", filename)
	req.Header.Set("X-Goog-Upload-Protocol", "raw")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("upload failed HTTP %d: %s", resp.StatusCode, string(body))
	}

	return string(body), nil
}

func (c *Client) createMediaItem(uploadToken, filename string) error {
	payload := map[string]any{
		"newMediaItems": []map[string]any{
			{
				"description": filename,
				"simpleMediaItem": map[string]string{
					"uploadToken": uploadToken,
					"fileName":   filename,
				},
			},
		},
	}

	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", batchCreateURL, bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("batchCreate failed HTTP %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func collectMediaFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if media.IsSupportedFile(e.Name()) {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	return files, nil
}

func loadUploadLog(path string) (map[string]bool, error) {
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

func appendUploadLog(path, filename string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = fmt.Fprintln(f, filename)
	return err
}
```

**Step 4: Install OAuth2 dependency**

Run: `go get golang.org/x/oauth2@latest && go get golang.org/x/oauth2/google@latest`

**Step 5: Run tests to verify they pass**

Run: `go test ./internal/google/ -v`
Expected: All 3 tests PASS.

**Step 6: Wire up Google Photos CLI commands**

`internal/cli/google.go`:
```go
package cli

import (
	"context"
	"fmt"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/google"
	"github.com/briandeitte/photo-copy/internal/logging"
	"github.com/spf13/cobra"
)

func newGooglePhotosCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "google-photos",
		Short: "Google Photos upload and Takeout import commands",
	}

	cmd.AddCommand(newGoogleUploadCmd())
	cmd.AddCommand(newGoogleImportTakeoutCmd())
	return cmd
}

func newGoogleUploadCmd() *cobra.Command {
	var inputDir string

	cmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload photos to Google Photos",
		RunE: func(cmd *cobra.Command, args []string) error {
			if inputDir == "" {
				return fmt.Errorf("--input-dir is required")
			}

			cfg, err := config.LoadGoogleConfig(config.DefaultDir())
			if err != nil {
				return fmt.Errorf("loading Google config (run 'photo-copy config google' first): %w", err)
			}

			log := logging.New(debug, nil)
			ctx := context.Background()
			client, err := google.NewClient(ctx, cfg, config.DefaultDir(), log)
			if err != nil {
				return fmt.Errorf("creating Google Photos client: %w", err)
			}

			return client.Upload(ctx, inputDir)
		},
	}

	cmd.Flags().StringVar(&inputDir, "input-dir", "", "Directory containing photos to upload")
	cmd.MarkFlagRequired("input-dir")
	return cmd
}

func newGoogleImportTakeoutCmd() *cobra.Command {
	// Placeholder - implemented in Task 10
	var takeoutDir, outputDir string

	cmd := &cobra.Command{
		Use:   "import-takeout",
		Short: "Import photos from Google Takeout zip files",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("not yet implemented")
		},
	}

	cmd.Flags().StringVar(&takeoutDir, "takeout-dir", "", "Directory containing Takeout zip files")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Directory to extract photos to")
	cmd.MarkFlagRequired("takeout-dir")
	cmd.MarkFlagRequired("output-dir")
	return cmd
}
```

**Step 7: Add google-photos command to root**

Update `internal/cli/root.go` - add `rootCmd.AddCommand(newGooglePhotosCmd())` inside `NewRootCmd()`.

**Step 8: Verify it builds**

Run: `go build -o photo-copy ./cmd/photo-copy && ./photo-copy google-photos --help`
Expected: Shows `upload` and `import-takeout` subcommands.

**Step 9: Commit**

```bash
git add internal/google/ internal/cli/google.go internal/cli/root.go go.mod go.sum
git commit -m "feat: add Google Photos upload with OAuth flow and resumability"
```

---

### Task 10: Google Takeout Import

**Files:**
- Create: `internal/google/takeout.go`
- Create: `internal/google/takeout_test.go`
- Modify: `internal/cli/google.go`

**Step 1: Write the test**

`internal/google/takeout_test.go`:
```go
package google

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func createTestZip(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	zipPath := filepath.Join(dir, "takeout.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for name, content := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		fw.Write([]byte(content))
	}
	w.Close()
	f.Close()
	return zipPath
}

func TestImportTakeout_ExtractsMediaOnly(t *testing.T) {
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()

	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/Trip/photo1.jpg":           "jpegdata",
		"Google Photos/Trip/photo1.jpg.json":      `{"title":"photo1"}`,
		"Google Photos/Trip/video.mp4":            "mp4data",
		"Google Photos/Trip/metadata.json":        `{"albums":[]}`,
		"Google Photos/Trip/print-subscriptions.json": `{}`,
	})

	count, err := ImportTakeout(takeoutDir, outputDir, nil)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	if count != 2 {
		t.Fatalf("expected 2 files extracted, got %d", count)
	}

	// Verify files exist
	if _, err := os.Stat(filepath.Join(outputDir, "photo1.jpg")); err != nil {
		t.Fatal("photo1.jpg not found in output")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "video.mp4")); err != nil {
		t.Fatal("video.mp4 not found in output")
	}
}

func TestImportTakeout_SkipsNonMedia(t *testing.T) {
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()

	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/readme.html": "<html>hi</html>",
		"Google Photos/data.json":   "{}",
	})

	count, err := ImportTakeout(takeoutDir, outputDir, nil)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	if count != 0 {
		t.Fatalf("expected 0 files extracted, got %d", count)
	}
}

func TestImportTakeout_MultipleZips(t *testing.T) {
	takeoutDir := t.TempDir()
	outputDir := t.TempDir()

	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/a.jpg": "data1",
	})
	// Rename first zip
	os.Rename(filepath.Join(takeoutDir, "takeout.zip"), filepath.Join(takeoutDir, "takeout-001.zip"))

	createTestZip(t, takeoutDir, map[string]string{
		"Google Photos/b.png": "data2",
	})
	os.Rename(filepath.Join(takeoutDir, "takeout.zip"), filepath.Join(takeoutDir, "takeout-002.zip"))

	count, err := ImportTakeout(takeoutDir, outputDir, nil)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	if count != 2 {
		t.Fatalf("expected 2 files, got %d", count)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/google/ -v -run TestImportTakeout`
Expected: FAIL - ImportTakeout not defined.

**Step 3: Write the implementation**

`internal/google/takeout.go`:
```go
package google

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/briandeitte/photo-copy/internal/logging"
	"github.com/briandeitte/photo-copy/internal/media"
	"github.com/schollz/progressbar/v3"
)

func ImportTakeout(takeoutDir, outputDir string, log *logging.Logger) (int, error) {
	if log == nil {
		log = logging.New(false, nil)
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return 0, fmt.Errorf("creating output dir: %w", err)
	}

	// Find all zip files
	entries, err := os.ReadDir(takeoutDir)
	if err != nil {
		return 0, fmt.Errorf("reading takeout dir: %w", err)
	}

	var zipFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".zip") {
			zipFiles = append(zipFiles, filepath.Join(takeoutDir, e.Name()))
		}
	}

	if len(zipFiles) == 0 {
		return 0, fmt.Errorf("no zip files found in %s", takeoutDir)
	}

	log.Debug("found %d zip files", len(zipFiles))

	totalExtracted := 0

	for _, zipPath := range zipFiles {
		log.Debug("processing %s", zipPath)

		count, err := extractMediaFromZip(zipPath, outputDir, log)
		if err != nil {
			log.Error("processing %s: %v", zipPath, err)
			continue
		}

		totalExtracted += count
	}

	fmt.Fprintf(os.Stderr, "Extracted %d media files\n", totalExtracted)
	return totalExtracted, nil
}

func extractMediaFromZip(zipPath, outputDir string, log *logging.Logger) (int, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return 0, fmt.Errorf("opening zip: %w", err)
	}
	defer r.Close()

	// Count media files for progress bar
	var mediaFiles []*zip.File
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := filepath.Base(f.Name)
		if media.IsSupportedFile(name) {
			mediaFiles = append(mediaFiles, f)
		} else {
			log.Debug("skipping non-media: %s", f.Name)
		}
	}

	if len(mediaFiles) == 0 {
		return 0, nil
	}

	bar := progressbar.Default(int64(len(mediaFiles)), filepath.Base(zipPath))
	extracted := 0

	for _, f := range mediaFiles {
		name := filepath.Base(f.Name)
		destPath := filepath.Join(outputDir, name)

		// Handle duplicate filenames
		if _, err := os.Stat(destPath); err == nil {
			base := strings.TrimSuffix(name, filepath.Ext(name))
			ext := filepath.Ext(name)
			for i := 1; ; i++ {
				destPath = filepath.Join(outputDir, fmt.Sprintf("%s_%d%s", base, i, ext))
				if _, err := os.Stat(destPath); os.IsNotExist(err) {
					break
				}
			}
			log.Debug("duplicate filename %s, saving as %s", name, filepath.Base(destPath))
		}

		log.Debug("extracting %s -> %s", f.Name, destPath)

		if err := extractFile(f, destPath); err != nil {
			log.Error("extracting %s: %v", f.Name, err)
			bar.Add(1)
			continue
		}

		extracted++
		bar.Add(1)
	}

	return extracted, nil
}

func extractFile(f *zip.File, destPath string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, rc)
	return err
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/google/ -v -run TestImportTakeout`
Expected: All 3 tests PASS.

**Step 5: Update the import-takeout CLI command**

Replace the placeholder in `internal/cli/google.go` `newGoogleImportTakeoutCmd` RunE with:

```go
RunE: func(cmd *cobra.Command, args []string) error {
    log := logging.New(debug, nil)
    _, err := google.ImportTakeout(takeoutDir, outputDir, log)
    return err
},
```

**Step 6: Verify it builds**

Run: `go build -o photo-copy ./cmd/photo-copy && ./photo-copy google-photos import-takeout --help`
Expected: Shows `--takeout-dir` and `--output-dir` flags.

**Step 7: Commit**

```bash
git add internal/google/takeout.go internal/google/takeout_test.go internal/cli/google.go
git commit -m "feat: add Google Takeout import - extracts media from zip files"
```

---

### Task 11: Setup Script and README

**Files:**
- Create: `setup.sh`
- Create: `README.md`
- Create: `.gitignore`

**Step 1: Write .gitignore**

`.gitignore`:
```
photo-copy
.venv/
src/
*.egg-info/
__pycache__/
```

**Step 2: Write setup.sh**

`setup.sh`:
```bash
#!/bin/bash
set -e

echo "=== photo-copy setup ==="

# Install rclone if not present
if ! command -v rclone &> /dev/null; then
    echo "Installing rclone..."
    curl https://rclone.org/install.sh | sudo bash
else
    echo "rclone already installed: $(rclone version | head -1)"
fi

# Build photo-copy
echo "Building photo-copy..."
if ! command -v go &> /dev/null; then
    echo "Error: Go is not installed. Install from https://go.dev/dl/"
    exit 1
fi

go build -o photo-copy ./cmd/photo-copy
echo "Built ./photo-copy"

echo ""
echo "Setup complete! Next steps:"
echo "  1. Run './photo-copy config flickr' to set up Flickr credentials"
echo "  2. Run './photo-copy config google' to set up Google credentials"
echo "  3. Run 'rclone config' to set up S3 remote"
```

**Step 3: Write README.md**

`README.md`:
```markdown
# photo-copy

Copy photos and videos between Flickr, Google Photos, S3, and local directories.

## Setup

```bash
./setup.sh
```

Requires Go 1.21+ and (optionally) rclone for S3 operations.

## Usage

### Configure credentials

```bash
./photo-copy config flickr    # Flickr API key + OAuth
./photo-copy config google    # Google OAuth credentials
```

### Flickr

```bash
# Download all photos from Flickr
./photo-copy flickr download --output-dir ./flickr-photos

# Upload local photos to Flickr
./photo-copy flickr upload --input-dir ./photos
```

### Google Photos

```bash
# Upload local photos to Google Photos
./photo-copy google-photos upload --input-dir ./photos

# Extract media from Google Takeout zips
./photo-copy google-photos import-takeout --takeout-dir ./takeout-zips --output-dir ./google-photos
```

### S3 (via rclone)

```bash
# Copy to S3
rclone copy ./photos s3remote:my-bucket/photos/ --progress

# Copy from S3
rclone copy s3remote:my-bucket/photos/ ./photos/ --progress
```

### Debug mode

Add `--debug` to any command for verbose logging:

```bash
./photo-copy flickr download --output-dir ./photos --debug
```

## Supported file types

JPEG, PNG, TIFF, GIF, HEIC, WebP, MP4, MOV, AVI, MKV
```

**Step 4: Make setup.sh executable and verify build**

Run: `chmod +x setup.sh && go build -o photo-copy ./cmd/photo-copy`

**Step 5: Commit**

```bash
git add setup.sh README.md .gitignore
git commit -m "feat: add setup script, README, and gitignore"
```

---

### Task 12: Run All Tests and Final Verification

**Step 1: Run all tests**

Run: `go test ./... -v`
Expected: All tests pass across all packages.

**Step 2: Build and smoke test**

Run: `go build -o photo-copy ./cmd/photo-copy && ./photo-copy --help`
Expected: Shows all subcommands (flickr, google-photos, config) and --debug flag.

Run: `./photo-copy flickr --help`
Expected: Shows download and upload subcommands.

Run: `./photo-copy google-photos --help`
Expected: Shows upload and import-takeout subcommands.

Run: `./photo-copy config --help`
Expected: Shows flickr and google subcommands.

**Step 3: Verify debug flag works**

Run: `./photo-copy flickr download --output-dir /tmp/test-photos --debug 2>&1 | head -5`
Expected: Should show debug-level log output (will fail on auth but should show debug messages before that).

**Step 4: Clean up old Python files if any remain**

Run: `rm -rf src/ pyproject.toml .venv/` (if not already done in Task 1)

**Step 5: Final commit**

```bash
git add -A
git commit -m "chore: final cleanup and verification"
```

---

Plan complete and saved to `docs/plans/2026-03-04-photo-copy-implementation.md`. Two execution options:

**1. Subagent-Driven (this session)** - I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Parallel Session (separate)** - Open a new session with executing-plans, batch execution with checkpoints

Which approach?