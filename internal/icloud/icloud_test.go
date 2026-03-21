package icloud

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/logging"
)

func TestNewClient(t *testing.T) {
	cfg := &config.ICloudConfig{
		AppleID:   "user@example.com",
		CookieDir: "/tmp/cookies",
	}
	log := logging.New(false, nil)
	client := NewClient(cfg, log)

	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestFindTool_EnvOverride(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "fake-icloudpd")
	if err := os.WriteFile(tmpFile, []byte("#!/bin/sh"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PHOTO_COPY_ICLOUDPD_PATH", tmpFile)
	path, err := findTool("icloudpd", "PHOTO_COPY_ICLOUDPD_PATH")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != tmpFile {
		t.Fatalf("expected env override path, got %s", path)
	}
}

func TestFindTool_EnvOverride_NotFound(t *testing.T) {
	t.Setenv("PHOTO_COPY_ICLOUDPD_PATH", "/nonexistent/path/icloudpd")
	_, err := findTool("icloudpd", "PHOTO_COPY_ICLOUDPD_PATH")
	if err == nil {
		t.Fatal("expected error for nonexistent env override path")
	}
}

func TestFindTool_NotInstalled(t *testing.T) {
	t.Setenv("PHOTO_COPY_ICLOUDPD_PATH", "")
	_, err := findTool("nonexistent-tool-xyz-12345", "PHOTO_COPY_NONEXISTENT_PATH")
	if err == nil {
		t.Fatal("expected error for missing tool")
	}
}
