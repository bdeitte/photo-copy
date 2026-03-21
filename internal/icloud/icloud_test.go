package icloud

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/daterange"
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

func TestBuildDownloadArgs(t *testing.T) {
	args := buildDownloadArgs("/output", "user@example.com", "/cookies", 0, nil, false)
	expected := []string{
		"--directory", "/output",
		"--username", "user@example.com",
		"--cookie-directory", "/cookies",
		"--no-progress-bar",
	}
	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
	}
	for i, want := range expected {
		if args[i] != want {
			t.Errorf("arg[%d] = %q, want %q", i, args[i], want)
		}
	}
}

func TestBuildDownloadArgs_WithLimit(t *testing.T) {
	args := buildDownloadArgs("/output", "user@example.com", "/cookies", 50, nil, false)
	found := false
	for i, a := range args {
		if a == "--recent" && i+1 < len(args) && args[i+1] == "50" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected --recent 50 in args, got: %v", args)
	}
}

func TestBuildDownloadArgs_WithDebug(t *testing.T) {
	args := buildDownloadArgs("/output", "user@example.com", "/cookies", 0, nil, true)
	found := false
	for _, a := range args {
		if a == "--log-level" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected --log-level in args, got: %v", args)
	}
}

func TestBuildDownloadArgs_WithDateRange(t *testing.T) {
	after := time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local)
	before := time.Date(2024, 1, 1, 0, 0, 0, 0, time.Local)
	dr := &daterange.DateRange{After: &after, Before: &before}
	args := buildDownloadArgs("/output", "user@example.com", "/cookies", 0, dr, false)
	for i, a := range args {
		if a == "--from-date" {
			if i+1 >= len(args) || args[i+1] != "2020-01-01" {
				t.Errorf("expected --from-date 2020-01-01, got: %v", args)
			}
		}
		if a == "--to-date" {
			if i+1 >= len(args) || args[i+1] != "2023-12-31" {
				t.Errorf("expected --to-date 2023-12-31 (Before minus 1 day), got: %v", args)
			}
		}
	}
}

func TestParseDownloadLine(t *testing.T) {
	tests := []struct {
		line     string
		expected string
	}{
		{"Downloading IMG_1234.jpg", "IMG_1234.jpg"},
		{"downloading photo.png", "photo.png"},
		{"Some other line", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := parseDownloadLine(tt.line)
		if got != tt.expected {
			t.Errorf("parseDownloadLine(%q) = %q, want %q", tt.line, got, tt.expected)
		}
	}
}

func TestParsePhotoCount(t *testing.T) {
	tests := []struct {
		line     string
		expected int
	}{
		{"Found 1234 items", 1234},
		{"Found 50 photos", 50},
		{"Downloading 3 photos from album", 0},
		{"No count here", 0},
		{"", 0},
	}
	for _, tt := range tests {
		got := parsePhotoCount(tt.line)
		if got != tt.expected {
			t.Errorf("parsePhotoCount(%q) = %d, want %d", tt.line, got, tt.expected)
		}
	}
}

func TestIsSkipLine(t *testing.T) {
	if !isSkipLine("File already exists") {
		t.Error("expected true for 'already exists'")
	}
	if !isSkipLine("Skipping file") {
		t.Error("expected true for 'skipping'")
	}
	if isSkipLine("Downloading photo.jpg") {
		t.Error("expected false for download line")
	}
}
