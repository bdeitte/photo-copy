package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrNotConfigured is returned when a config file does not exist.
var ErrNotConfigured = errors.New("not configured")

const (
	flickrFile      = "flickr.json"
	googleFile      = "google_credentials.json"
	googleTokenFile = "google_token.json"
	s3File          = "s3.json"
	icloudFile      = "icloud.json"
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
	if dir := os.Getenv("PHOTO_COPY_CONFIG_DIR"); dir != "" {
		return dir
	}
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

type S3Config struct {
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	Region          string `json:"region"`
}

func SaveS3Config(configDir string, cfg *S3Config) error {
	return saveJSON(configDir, s3File, cfg)
}

func LoadS3Config(configDir string) (*S3Config, error) {
	var cfg S3Config
	if err := loadJSON(configDir, s3File, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

type ICloudConfig struct {
	AppleID   string `json:"apple_id"`
	CookieDir string `json:"cookie_dir"`
}

func SaveICloudConfig(configDir string, cfg *ICloudConfig) error {
	return saveJSON(configDir, icloudFile, cfg)
}

func LoadICloudConfig(configDir string) (*ICloudConfig, error) {
	var cfg ICloudConfig
	if err := loadJSON(configDir, icloudFile, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
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
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotConfigured
		}
		return fmt.Errorf("reading %s: %w", filename, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("parsing %s: %w", filename, err)
	}
	return nil
}
