# S3 Support via Embedded Rclone - Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add `photo-copy s3 upload/download` commands that invoke a bundled rclone binary, with interactive S3 credential setup.

**Architecture:** Rclone binaries for 6 platforms stored in `rclone-bin/` via Git LFS. A new `internal/s3` package writes a temp rclone config and invokes the correct binary as a subprocess. New CLI commands wrap this with `--bucket`, `--prefix`, `--debug` flags. A `scripts/update-rclone.sh` handles downloading/updating rclone versions.

**Tech Stack:** Go, cobra (existing), rclone (subprocess), Git LFS

---

### Task 1: Git LFS Setup and Rclone Update Script

**Files:**
- Create: `.gitattributes`
- Create: `scripts/update-rclone.sh`
- Create: `rclone-bin/.gitkeep`

**Step 1: Initialize Git LFS and configure tracking**

Run: `git lfs install`

Create `.gitattributes`:
```
rclone-bin/rclone-* filter=lfs diff=lfs merge=lfs -text
```

**Step 2: Write the rclone download script**

`scripts/update-rclone.sh`:
```bash
#!/bin/bash
set -e

RCLONE_VERSION="${1:-v1.68.2}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BIN_DIR="$SCRIPT_DIR/../rclone-bin"

mkdir -p "$BIN_DIR"

PLATFORMS=(
    "linux-amd64"
    "linux-arm64"
    "osx-amd64"
    "osx-arm64"
    "windows-amd64"
    "windows-arm64"
)

# Map rclone platform names to our binary names
declare -A BINARY_NAMES
BINARY_NAMES["linux-amd64"]="rclone-linux-amd64"
BINARY_NAMES["linux-arm64"]="rclone-linux-arm64"
BINARY_NAMES["osx-amd64"]="rclone-darwin-amd64"
BINARY_NAMES["osx-arm64"]="rclone-darwin-arm64"
BINARY_NAMES["windows-amd64"]="rclone-windows-amd64.exe"
BINARY_NAMES["windows-arm64"]="rclone-windows-arm64.exe"

TMPDIR=$(mktemp -d)
trap "rm -rf $TMPDIR" EXIT

for PLATFORM in "${PLATFORMS[@]}"; do
    BINARY_NAME="${BINARY_NAMES[$PLATFORM]}"
    echo "Downloading rclone $RCLONE_VERSION for $PLATFORM..."

    if [[ "$PLATFORM" == windows-* ]]; then
        URL="https://downloads.rclone.org/${RCLONE_VERSION}/rclone-${RCLONE_VERSION}-${PLATFORM}.zip"
        curl -sL "$URL" -o "$TMPDIR/rclone.zip"
        unzip -q -o "$TMPDIR/rclone.zip" -d "$TMPDIR"
        cp "$TMPDIR/rclone-${RCLONE_VERSION}-${PLATFORM}/rclone.exe" "$BIN_DIR/$BINARY_NAME"
    else
        URL="https://downloads.rclone.org/${RCLONE_VERSION}/rclone-${RCLONE_VERSION}-${PLATFORM}.zip"
        curl -sL "$URL" -o "$TMPDIR/rclone.zip"
        unzip -q -o "$TMPDIR/rclone.zip" -d "$TMPDIR"
        cp "$TMPDIR/rclone-${RCLONE_VERSION}-${PLATFORM}/rclone" "$BIN_DIR/$BINARY_NAME"
        chmod +x "$BIN_DIR/$BINARY_NAME"
    fi

    rm -f "$TMPDIR/rclone.zip"
    echo "  -> $BINARY_NAME"
done

echo ""
echo "Rclone $RCLONE_VERSION downloaded for all platforms."
echo "Files in $BIN_DIR:"
ls -lh "$BIN_DIR"/rclone-*
```

**Step 3: Make script executable and create .gitkeep**

Run: `chmod +x scripts/update-rclone.sh && touch rclone-bin/.gitkeep`

**Step 4: Run the script to download rclone binaries**

Run: `./scripts/update-rclone.sh v1.68.2`
Expected: 6 rclone binaries in `rclone-bin/`.

**Step 5: Verify LFS is tracking the binaries**

Run: `git lfs ls-files` (after staging) should show the rclone binaries.

**Step 6: Commit**

```bash
git add .gitattributes scripts/update-rclone.sh rclone-bin/
git commit -m "feat: add rclone binaries for 6 platforms via Git LFS"
```

---

### Task 2: S3 Config - Credential Storage and Setup

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/cli/config.go`

**Step 1: Write the test for S3 config**

Add to `internal/config/config_test.go`:
```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -v -run TestSaveAndLoadS3Config`
Expected: FAIL - S3Config not defined.

**Step 3: Add S3 config types and functions to config.go**

Add to `internal/config/config.go`:

```go
const s3File = "s3.json"

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
```

Add the `s3File` constant next to the existing ones (`flickrFile`, `googleFile`, `googleTokenFile`).

**Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -v`
Expected: All tests PASS (existing + new).

**Step 5: Add `config s3` CLI command**

Add `newConfigS3Cmd` to `internal/cli/config.go`. In `newConfigCmd()`, add `cmd.AddCommand(newConfigS3Cmd())`.

```go
func newConfigS3Cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "s3",
		Short: "Set up S3 credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			reader := bufio.NewReader(os.Stdin)
			configDir := config.DefaultDir()

			fmt.Println("S3 Credential Setup")
			fmt.Println()

			// Check for existing AWS credentials
			home, _ := os.UserHomeDir()
			awsCredsPath := filepath.Join(home, ".aws", "credentials")
			if _, err := os.Stat(awsCredsPath); err == nil {
				fmt.Print("Found existing AWS credentials at ~/.aws/credentials. Use these? (y/n): ")
				answer, _ := reader.ReadString('\n')
				answer = strings.TrimSpace(strings.ToLower(answer))

				if answer == "y" || answer == "yes" {
					cfg, err := readAWSCredentials(awsCredsPath)
					if err != nil {
						fmt.Printf("Warning: could not read AWS credentials: %v\n", err)
						fmt.Println("Falling back to manual entry.")
					} else {
						// Prompt for region since it's not in credentials file
						fmt.Print("AWS Region (e.g., us-east-1): ")
						region, _ := reader.ReadString('\n')
						region = strings.TrimSpace(region)
						if region == "" {
							region = "us-east-1"
						}
						cfg.Region = region

						if err := config.SaveS3Config(configDir, cfg); err != nil {
							return fmt.Errorf("saving config: %w", err)
						}
						fmt.Printf("\nS3 credentials saved to %s\n", configDir)
						return nil
					}
				}
			}

			fmt.Print("AWS Access Key ID: ")
			accessKey, _ := reader.ReadString('\n')
			accessKey = strings.TrimSpace(accessKey)

			fmt.Print("AWS Secret Access Key: ")
			secretKey, _ := reader.ReadString('\n')
			secretKey = strings.TrimSpace(secretKey)

			fmt.Print("AWS Region (e.g., us-east-1): ")
			region, _ := reader.ReadString('\n')
			region = strings.TrimSpace(region)
			if region == "" {
				region = "us-east-1"
			}

			if accessKey == "" || secretKey == "" {
				return fmt.Errorf("access key and secret key are required")
			}

			cfg := &config.S3Config{
				AccessKeyID:     accessKey,
				SecretAccessKey: secretKey,
				Region:          region,
			}

			if err := config.SaveS3Config(configDir, cfg); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			fmt.Printf("\nS3 credentials saved to %s\n", configDir)
			return nil
		},
	}
}

// readAWSCredentials reads the [default] profile from ~/.aws/credentials
func readAWSCredentials(path string) (*config.S3Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &config.S3Config{}
	lines := strings.Split(string(data), "\n")
	inDefault := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "[default]" {
			inDefault = true
			continue
		}
		if strings.HasPrefix(line, "[") {
			inDefault = false
			continue
		}
		if !inDefault {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch key {
		case "aws_access_key_id":
			cfg.AccessKeyID = val
		case "aws_secret_access_key":
			cfg.SecretAccessKey = val
		}
	}

	if cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		return nil, fmt.Errorf("could not find access key and secret in [default] profile")
	}

	return cfg, nil
}
```

Add `"path/filepath"` to the imports in config.go (CLI file).

**Step 6: Verify it builds**

Run: `go build -o photo-copy ./cmd/photo-copy && ./photo-copy config s3 --help`
Expected: Shows "Set up S3 credentials".

**Step 7: Commit**

```bash
git add internal/config/ internal/cli/config.go
git commit -m "feat: add S3 credential config with AWS credentials import"
```

---

### Task 3: Rclone Binary Resolution

**Files:**
- Create: `internal/s3/rclone.go`
- Create: `internal/s3/rclone_test.go`

**Step 1: Write the test**

`internal/s3/rclone_test.go`:
```go
package s3

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRcloneBinaryName(t *testing.T) {
	tests := []struct {
		goos, goarch string
		want         string
	}{
		{"linux", "amd64", "rclone-linux-amd64"},
		{"linux", "arm64", "rclone-linux-arm64"},
		{"darwin", "amd64", "rclone-darwin-amd64"},
		{"darwin", "arm64", "rclone-darwin-arm64"},
		{"windows", "amd64", "rclone-windows-amd64.exe"},
		{"windows", "arm64", "rclone-windows-arm64.exe"},
	}

	for _, tt := range tests {
		t.Run(tt.goos+"-"+tt.goarch, func(t *testing.T) {
			got := rcloneBinaryName(tt.goos, tt.goarch)
			if got != tt.want {
				t.Errorf("rcloneBinaryName(%q, %q) = %q, want %q", tt.goos, tt.goarch, got, tt.want)
			}
		})
	}
}

func TestFindRcloneBinary(t *testing.T) {
	// Create a fake rclone binary in a temp dir
	tmpDir := t.TempDir()
	binDir := filepath.Join(tmpDir, "rclone-bin")
	os.MkdirAll(binDir, 0755)

	name := rcloneBinaryName(runtime.GOOS, runtime.GOARCH)
	fakeBin := filepath.Join(binDir, name)
	os.WriteFile(fakeBin, []byte("fake"), 0755)

	got, err := findRcloneBinary(binDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != fakeBin {
		t.Errorf("got %q, want %q", got, fakeBin)
	}
}

func TestFindRcloneBinary_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := findRcloneBinary(tmpDir)
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
}

func TestWriteRcloneConfig(t *testing.T) {
	tmpFile, err := writeRcloneConfig("AKID", "SECRET", "us-west-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer os.Remove(tmpFile)

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}

	content := string(data)
	if !containsStr(content, "type = s3") {
		t.Fatal("missing type = s3")
	}
	if !containsStr(content, "access_key_id = AKID") {
		t.Fatal("missing access_key_id")
	}
	if !containsStr(content, "region = us-west-2") {
		t.Fatal("missing region")
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/s3/ -v`
Expected: FAIL - functions not defined.

**Step 3: Write the implementation**

`internal/s3/rclone.go`:
```go
package s3

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func rcloneBinaryName(goos, goarch string) string {
	name := fmt.Sprintf("rclone-%s-%s", goos, goarch)
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

func findRcloneBinary(binDir string) (string, error) {
	name := rcloneBinaryName(runtime.GOOS, runtime.GOARCH)
	path := filepath.Join(binDir, name)

	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("rclone binary not found at %s: %w", path, err)
	}

	return path, nil
}

// rcloneBinDir returns the path to the rclone-bin directory relative to the executable.
// It checks: 1) next to the executable, 2) current working directory.
func rcloneBinDir() (string, error) {
	// Try relative to executable
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Join(filepath.Dir(exe), "rclone-bin")
		if _, err := os.Stat(dir); err == nil {
			return dir, nil
		}
	}

	// Try current working directory
	cwd, err := os.Getwd()
	if err == nil {
		dir := filepath.Join(cwd, "rclone-bin")
		if _, err := os.Stat(dir); err == nil {
			return dir, nil
		}
	}

	return "", fmt.Errorf("rclone-bin directory not found (checked next to executable and current directory)")
}

func writeRcloneConfig(accessKeyID, secretAccessKey, region string) (string, error) {
	content := fmt.Sprintf(`[s3]
type = s3
provider = AWS
access_key_id = %s
secret_access_key = %s
region = %s
`, accessKeyID, secretAccessKey, region)

	f, err := os.CreateTemp("", "rclone-config-*.conf")
	if err != nil {
		return "", fmt.Errorf("creating temp config: %w", err)
	}

	if _, err := f.WriteString(content); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", fmt.Errorf("writing config: %w", err)
	}

	f.Close()
	return f.Name(), nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/s3/ -v`
Expected: All tests PASS.

**Step 5: Commit**

```bash
git add internal/s3/
git commit -m "feat: add rclone binary resolution and config generation"
```

---

### Task 4: S3 Upload and Download via Rclone Subprocess

**Files:**
- Create: `internal/s3/s3.go`
- Create: `internal/s3/s3_test.go`

**Step 1: Write the test**

`internal/s3/s3_test.go`:
```go
package s3

import (
	"testing"
)

func TestBuildUploadArgs(t *testing.T) {
	args := buildUploadArgs("/tmp/config.conf", "/path/to/photos", "my-bucket", "backup/")
	expected := []string{
		"copy", "/path/to/photos", "s3:my-bucket/backup/",
		"--config", "/tmp/config.conf",
		"--progress",
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

func TestBuildUploadArgs_NoPrefix(t *testing.T) {
	args := buildUploadArgs("/tmp/config.conf", "/path/to/photos", "my-bucket", "")
	// Should be s3:my-bucket (no trailing slash from empty prefix)
	found := false
	for _, a := range args {
		if a == "s3:my-bucket" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected 's3:my-bucket' in args, got: %v", args)
	}
}

func TestBuildDownloadArgs(t *testing.T) {
	args := buildDownloadArgs("/tmp/config.conf", "my-bucket", "photos/", "/path/to/output")
	expected := []string{
		"copy", "s3:my-bucket/photos/", "/path/to/output",
		"--config", "/tmp/config.conf",
		"--progress",
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

func TestBuildIncludeFlags(t *testing.T) {
	flags := buildMediaIncludeFlags()
	// Should have --include flags for each supported extension
	if len(flags) == 0 {
		t.Fatal("expected include flags")
	}
	// First flag should be --include
	if flags[0] != "--include" {
		t.Fatalf("expected --include, got %s", flags[0])
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/s3/ -v -run TestBuild`
Expected: FAIL - functions not defined.

**Step 3: Write the implementation**

`internal/s3/s3.go`:
```go
package s3

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/logging"
)

type Client struct {
	cfg *config.S3Config
	log *logging.Logger
}

func NewClient(cfg *config.S3Config, log *logging.Logger) *Client {
	return &Client{cfg: cfg, log: log}
}

func (c *Client) Upload(ctx context.Context, inputDir, bucket, prefix string, mediaOnly bool) error {
	binDir, err := rcloneBinDir()
	if err != nil {
		return err
	}

	rclonePath, err := findRcloneBinary(binDir)
	if err != nil {
		return err
	}

	configPath, err := writeRcloneConfig(c.cfg.AccessKeyID, c.cfg.SecretAccessKey, c.cfg.Region)
	if err != nil {
		return err
	}
	defer os.Remove(configPath)

	args := buildUploadArgs(configPath, inputDir, bucket, prefix)
	if mediaOnly {
		args = append(args, buildMediaIncludeFlags()...)
	}

	c.log.Debug("running: %s %s", rclonePath, strings.Join(args, " "))
	return c.runRclone(ctx, rclonePath, args)
}

func (c *Client) Download(ctx context.Context, bucket, prefix, outputDir string, mediaOnly bool) error {
	binDir, err := rcloneBinDir()
	if err != nil {
		return err
	}

	rclonePath, err := findRcloneBinary(binDir)
	if err != nil {
		return err
	}

	configPath, err := writeRcloneConfig(c.cfg.AccessKeyID, c.cfg.SecretAccessKey, c.cfg.Region)
	if err != nil {
		return err
	}
	defer os.Remove(configPath)

	args := buildDownloadArgs(configPath, bucket, prefix, outputDir)
	if mediaOnly {
		args = append(args, buildMediaIncludeFlags()...)
	}

	c.log.Debug("running: %s %s", rclonePath, strings.Join(args, " "))
	return c.runRclone(ctx, rclonePath, args)
}

func (c *Client) runRclone(ctx context.Context, rclonePath string, args []string) error {
	cmd := exec.CommandContext(ctx, rclonePath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rclone failed: %w", err)
	}
	return nil
}

func buildUploadArgs(configPath, inputDir, bucket, prefix string) []string {
	dest := "s3:" + bucket
	if prefix != "" {
		dest += "/" + prefix
	}

	return []string{
		"copy", inputDir, dest,
		"--config", configPath,
		"--progress",
	}
}

func buildDownloadArgs(configPath, bucket, prefix, outputDir string) []string {
	src := "s3:" + bucket
	if prefix != "" {
		src += "/" + prefix
	}

	return []string{
		"copy", src, outputDir,
		"--config", configPath,
		"--progress",
	}
}

func buildMediaIncludeFlags() []string {
	extensions := []string{
		"*.jpg", "*.jpeg", "*.png", "*.tiff", "*.tif", "*.gif",
		"*.heic", "*.webp", "*.mp4", "*.mov", "*.avi", "*.mkv",
		"*.JPG", "*.JPEG", "*.PNG", "*.TIFF", "*.TIF", "*.GIF",
		"*.HEIC", "*.WEBP", "*.MP4", "*.MOV", "*.AVI", "*.MKV",
	}

	var flags []string
	for _, ext := range extensions {
		flags = append(flags, "--include", ext)
	}
	return flags
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/s3/ -v`
Expected: All tests PASS.

**Step 5: Commit**

```bash
git add internal/s3/
git commit -m "feat: add S3 upload/download via rclone subprocess"
```

---

### Task 5: S3 CLI Commands

**Files:**
- Create: `internal/cli/s3.go`
- Modify: `internal/cli/root.go`

**Step 1: Write the S3 CLI commands**

`internal/cli/s3.go`:
```go
package cli

import (
	"context"
	"fmt"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/logging"
	"github.com/briandeitte/photo-copy/internal/s3"
	"github.com/spf13/cobra"
)

func newS3Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "s3",
		Short: "S3 upload and download commands",
	}

	cmd.AddCommand(newS3UploadCmd())
	cmd.AddCommand(newS3DownloadCmd())
	return cmd
}

func newS3UploadCmd() *cobra.Command {
	var inputDir, bucket, prefix string

	cmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload photos to S3",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadS3Config(config.DefaultDir())
			if err != nil {
				return fmt.Errorf("loading S3 config (run 'photo-copy config s3' first): %w", err)
			}

			log := logging.New(debug, nil)
			client := s3.NewClient(cfg, log)
			return client.Upload(context.Background(), inputDir, bucket, prefix, true)
		},
	}

	cmd.Flags().StringVar(&inputDir, "input-dir", "", "Directory containing photos to upload")
	cmd.Flags().StringVar(&bucket, "bucket", "", "S3 bucket name")
	cmd.Flags().StringVar(&prefix, "prefix", "", "S3 key prefix (optional)")
	cmd.MarkFlagRequired("input-dir")
	cmd.MarkFlagRequired("bucket")
	return cmd
}

func newS3DownloadCmd() *cobra.Command {
	var outputDir, bucket, prefix string

	cmd := &cobra.Command{
		Use:   "download",
		Short: "Download photos from S3",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadS3Config(config.DefaultDir())
			if err != nil {
				return fmt.Errorf("loading S3 config (run 'photo-copy config s3' first): %w", err)
			}

			log := logging.New(debug, nil)
			client := s3.NewClient(cfg, log)
			return client.Download(context.Background(), bucket, prefix, outputDir, true)
		},
	}

	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Directory to save downloaded photos")
	cmd.Flags().StringVar(&bucket, "bucket", "", "S3 bucket name")
	cmd.Flags().StringVar(&prefix, "prefix", "", "S3 key prefix (optional)")
	cmd.MarkFlagRequired("output-dir")
	cmd.MarkFlagRequired("bucket")
	return cmd
}
```

**Step 2: Wire into root command**

Add `rootCmd.AddCommand(newS3Cmd())` in `NewRootCmd()` in `internal/cli/root.go`.

**Step 3: Verify it builds**

Run: `go build -o photo-copy ./cmd/photo-copy && ./photo-copy s3 --help`
Expected: Shows `upload` and `download` subcommands.

Run: `./photo-copy s3 upload --help`
Expected: Shows `--input-dir`, `--bucket`, `--prefix` flags.

**Step 4: Commit**

```bash
git add internal/cli/s3.go internal/cli/root.go
git commit -m "feat: add S3 upload/download CLI commands"
```

---

### Task 6: Update Setup Script, README, and Design Doc

**Files:**
- Modify: `setup.sh`
- Modify: `README.md`
- Modify: `docs/plans/2026-03-04-photo-copy-design.md`

**Step 1: Update setup.sh**

Remove the rclone install section (no longer needed - it's bundled). Replace with a note that rclone binaries are included.

New `setup.sh`:
```bash
#!/bin/bash
set -e

echo "=== photo-copy setup ==="

# Build photo-copy
echo "Building photo-copy..."
if ! command -v go &> /dev/null; then
    echo "Error: Go is not installed. Install from https://go.dev/dl/"
    exit 1
fi

go build -o photo-copy ./cmd/photo-copy
echo "Built ./photo-copy"

# Verify rclone binaries are present
if [ ! -d "rclone-bin" ] || [ -z "$(ls rclone-bin/rclone-* 2>/dev/null)" ]; then
    echo ""
    echo "Warning: rclone binaries not found in rclone-bin/"
    echo "Run: ./scripts/update-rclone.sh"
    echo "(S3 commands will not work without rclone binaries)"
fi

echo ""
echo "Setup complete! Next steps:"
echo "  1. Run './photo-copy config flickr' to set up Flickr credentials"
echo "  2. Run './photo-copy config google' to set up Google credentials"
echo "  3. Run './photo-copy config s3' to set up S3 credentials"
```

**Step 2: Update README.md**

Add S3 section to usage:
```markdown
### S3

```bash
# Upload local photos to S3
./photo-copy s3 upload --input-dir ./photos --bucket my-bucket --prefix photos/

# Download photos from S3
./photo-copy s3 download --bucket my-bucket --prefix photos/ --output-dir ./photos
```
```

Update the config section to include `./photo-copy config s3`.

Remove the old "S3 (via rclone)" section that told users to run rclone directly.

**Step 3: Update design doc**

Update `docs/plans/2026-03-04-photo-copy-design.md`:
- Change the architecture section to note rclone is embedded
- Update the copy matrix so all S3 operations go through `photo-copy s3`
- Update the commands section to include `s3 upload/download` and `config s3`
- Update the repo structure to include `rclone-bin/` and `scripts/`
- Remove the "Rclone for S3" section about external install

**Step 4: Verify build and all tests**

Run: `go test ./... && go build -o photo-copy ./cmd/photo-copy && ./photo-copy --help`
Expected: All tests pass, help shows `s3` command alongside others.

**Step 5: Commit**

```bash
git add setup.sh README.md docs/plans/2026-03-04-photo-copy-design.md
git commit -m "docs: update setup, README, and design doc for embedded rclone S3 support"
```

---

### Task 7: Final Verification

**Step 1: Run all tests**

Run: `go test ./... -v`
Expected: All tests pass across all packages.

**Step 2: Smoke test all commands**

Run: `./photo-copy --help`
Expected: Shows config, flickr, google-photos, s3 commands.

Run: `./photo-copy s3 upload --help`
Expected: Shows --input-dir, --bucket, --prefix, --debug flags.

Run: `./photo-copy s3 download --help`
Expected: Shows --output-dir, --bucket, --prefix, --debug flags.

Run: `./photo-copy config --help`
Expected: Shows flickr, google, s3 subcommands.

**Step 3: Verify rclone binary resolution works**

Run: `./photo-copy s3 upload --input-dir /tmp --bucket test-bucket --debug 2>&1 | head -5`
Expected: Debug output should show rclone command being constructed (will fail on credentials but should show the path resolution working).

**Step 4: Verify rclone binaries are tracked by LFS**

Run: `git lfs ls-files`
Expected: Shows the 6 rclone binaries.

**Step 5: Final commit if any cleanup needed**

```bash
git add -A
git commit -m "chore: final verification and cleanup for S3 support"
```
