package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoadFlickrConfig(t *testing.T) {
	tmpDir := t.TempDir()

	fc := &FlickrConfig{
		APIKey:           "test-key",
		APISecret:        "test-secret",
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

func TestSaveAndLoadS3Config(t *testing.T) {
	tmpDir := t.TempDir()

	sc := &S3Config{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		Region:          "us-east-1",
	}

	if err := SaveS3Config(tmpDir, sc); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := LoadS3Config(tmpDir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if loaded.AccessKeyID != sc.AccessKeyID {
		t.Fatalf("access key mismatch: got %s", loaded.AccessKeyID)
	}
	if loaded.SecretAccessKey != sc.SecretAccessKey {
		t.Fatalf("secret key mismatch")
	}
	if loaded.Region != sc.Region {
		t.Fatalf("region mismatch: got %s", loaded.Region)
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
