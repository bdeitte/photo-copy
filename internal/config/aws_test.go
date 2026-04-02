package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadAWSCredentials_DefaultProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")
	content := `[default]
aws_access_key_id = AKIAIOSFODNN7EXAMPLE
aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY

[other]
aws_access_key_id = OTHER
aws_secret_access_key = OTHERKEY
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := ReadAWSCredentials(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AccessKeyID != "AKIAIOSFODNN7EXAMPLE" {
		t.Errorf("got access key %q, want AKIAIOSFODNN7EXAMPLE", cfg.AccessKeyID)
	}
	if cfg.SecretAccessKey != "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY" {
		t.Errorf("got secret key %q", cfg.SecretAccessKey)
	}
}

func TestReadAWSCredentials_MissingDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")
	content := `[other]
aws_access_key_id = OTHER
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ReadAWSCredentials(path)
	if err == nil {
		t.Fatal("expected error for missing default profile")
	}
}
