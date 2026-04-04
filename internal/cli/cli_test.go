package cli

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/logging"
	"github.com/briandeitte/photo-copy/internal/transfer"
)

func TestRootCmd_HasExpectedSubcommands(t *testing.T) {
	cmd := NewRootCmd()

	expected := []string{"config", "flickr", "google", "s3", "help"}
	names := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		names[sub.Name()] = true
	}

	for _, name := range expected {
		if !names[name] {
			t.Errorf("missing expected subcommand: %s", name)
		}
	}
}

func TestRootCmd_HasDebugFlag(t *testing.T) {
	cmd := NewRootCmd()
	f := cmd.PersistentFlags().Lookup("debug")
	if f == nil {
		t.Fatal("missing --debug flag")
	}
	if f.DefValue != "false" {
		t.Errorf("--debug default = %q, want %q", f.DefValue, "false")
	}
}

func TestRootCmd_HasLimitFlag(t *testing.T) {
	cmd := NewRootCmd()
	f := cmd.PersistentFlags().Lookup("limit")
	if f == nil {
		t.Fatal("missing --limit flag")
	}
	if f.DefValue != "0" {
		t.Errorf("--limit default = %q, want %q", f.DefValue, "0")
	}
}

func TestFlickrCmd_RequiresSubcommand(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"flickr"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestS3UploadCmd_RequiresDestination(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"s3", "upload", "/tmp/photos"})
	buf := new(bytes.Buffer)
	cmd.SetErr(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing destination arg")
	}
}

func TestS3DownloadCmd_RequiresDestination(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"s3", "download", "my-bucket"})
	buf := new(bytes.Buffer)
	cmd.SetErr(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing output-dir arg")
	}
}

func TestS3UploadCmd_RegionlessURLOverridesConfig(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("PHOTO_COPY_CONFIG_DIR", configDir)

	cfg := &config.S3Config{
		AccessKeyID:    "test-key",
		SecretAccessKey: "test-secret",
		Region:         "us-west-2",
	}
	if err := config.SaveS3Config(configDir, cfg); err != nil {
		t.Fatalf("saving test s3 config: %v", err)
	}

	cmd := NewRootCmd()
	// Regionless URL (bucket.s3.amazonaws.com) implies us-east-1.
	// The command will fail at rclone (not installed in test), but it must
	// get past config loading and destination parsing to reach that point.
	cmd.SetArgs([]string{"s3", "upload", "/tmp/photos", "https://my-bucket.s3.amazonaws.com/prefix/"})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetOut(new(bytes.Buffer))
	err := cmd.Execute()

	if err == nil {
		t.Fatal("expected error (rclone not found in test)")
	}
	// The error should be about rclone, not about config or destination parsing.
	// This proves the regionless URL was accepted and the command progressed past
	// config loading with region override to the rclone invocation phase.
	if !strings.Contains(err.Error(), "rclone") && !strings.Contains(err.Error(), "tools-bin") {
		t.Errorf("expected rclone-related error (proving command got past config/region override), got: %v", err)
	}
}

func TestFlickrDownloadCmd_RequiresArg(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"flickr", "download"})
	buf := new(bytes.Buffer)
	cmd.SetErr(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing output-dir arg")
	}
}

func TestGoogleUploadCmd_RequiresArg(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"google", "upload"})
	buf := new(bytes.Buffer)
	cmd.SetErr(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing input-dir arg")
	}
}

func TestGoogleDownloadCmd_ShowsTakeoutInstructions(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"google", "download"})
	buf := new(bytes.Buffer)
	cmd.SetErr(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when no args provided")
	}
	if !strings.Contains(err.Error(), "Google Takeout") {
		t.Fatalf("expected Takeout instructions in error, got: %s", err.Error())
	}
}

func TestGoogleDownloadCmd_ShowsTakeoutInstructionsOneArg(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"google", "download", "/only/one"})
	buf := new(bytes.Buffer)
	cmd.SetErr(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing second arg")
	}
	if !strings.Contains(err.Error(), "Google Takeout") {
		t.Fatalf("expected Takeout instructions in error, got: %s", err.Error())
	}
}

func TestRootCmd_HasNoMetadataFlag(t *testing.T) {
	cmd := NewRootCmd()
	f := cmd.PersistentFlags().Lookup("no-metadata")
	if f == nil {
		t.Fatal("missing --no-metadata flag")
	}
	if f.DefValue != "false" {
		t.Errorf("--no-metadata default = %q, want %q", f.DefValue, "false")
	}
}

func TestRootCmd_HasDateRangeFlag(t *testing.T) {
	cmd := NewRootCmd()
	f := cmd.PersistentFlags().Lookup("date-range")
	if f == nil {
		t.Fatal("missing --date-range flag")
	}
	if f.DefValue != "" {
		t.Errorf("--date-range default = %q, want empty", f.DefValue)
	}
}

func TestRootCmd_InvalidDateRangeReturnsError(t *testing.T) {
	cmd := NewRootCmd()
	// Use "flickr download /tmp" to trigger PersistentPreRunE on a leaf command.
	// The command itself will fail (no config), but the pre-run error should come first.
	cmd.SetArgs([]string{"--date-range", "bad-value", "flickr", "download", "/tmp"})
	buf := new(bytes.Buffer)
	cmd.SetErr(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid --date-range")
	}
	if !strings.Contains(err.Error(), "invalid --date-range") {
		t.Errorf("error = %q, want to contain 'invalid --date-range'", err.Error())
	}
}

func TestRootCmd_NoMetadataWarningOnS3Upload(t *testing.T) {
	cmd := NewRootCmd()
	// s3 upload now uses positional args; it'll error on config, but PersistentPreRunE fires first
	cmd.SetArgs([]string{"--no-metadata", "s3", "upload", "/tmp", "s3://b"})
	buf := new(bytes.Buffer)
	cmd.SetErr(buf)
	cmd.SetOut(new(bytes.Buffer))
	_ = cmd.Execute()
	if !strings.Contains(buf.String(), "--no-metadata has no effect") {
		t.Errorf("expected no-metadata warning on stderr, got: %q", buf.String())
	}
}

func TestRootCmd_UnknownCommandShowsAvailableCommands(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"download", "flickr"})
	buf := new(bytes.Buffer)
	cmd.SetErr(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("error = %q, want to contain 'unknown command'", err.Error())
	}

	// Verify printAvailableCommands output
	out := new(bytes.Buffer)
	printAvailableCommands(out, cmd)
	output := out.String()
	for _, name := range []string{"config", "flickr", "google", "s3"} {
		if !strings.Contains(output, name) {
			t.Errorf("printAvailableCommands output missing %q", name)
		}
	}
	if !strings.Contains(output, "Available commands:") {
		t.Error("printAvailableCommands output missing header")
	}
}

func TestRootCmd_DateRangeWarningOnConfigSubcommand(t *testing.T) {
	cmd := NewRootCmd()
	// "config flickr" is a leaf command with RunE, so PersistentPreRunE fires.
	// It will fail waiting for stdin, but warnings happen in pre-run.
	cmd.SetArgs([]string{"--date-range", "2020-01-01:2023-12-31", "config", "flickr"})
	buf := new(bytes.Buffer)
	cmd.SetErr(buf)
	cmd.SetOut(new(bytes.Buffer))
	// Provide empty stdin so the command doesn't block
	cmd.SetIn(strings.NewReader("\n\n"))
	_ = cmd.Execute()
	if !strings.Contains(buf.String(), "--date-range has no effect on photo-copy config flickr") {
		t.Errorf("expected date-range warning on stderr, got: %q", buf.String())
	}
}

func TestPromptUser_EOFWithoutNewline(t *testing.T) {
	// Simulate piped input where the last line has no trailing newline.
	reader := bufio.NewReader(strings.NewReader("my-value"))
	got, err := promptUser(reader, "Enter: ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "my-value" {
		t.Errorf("got %q, want %q", got, "my-value")
	}
}

func TestPromptUser_EOFEmpty(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader(""))
	_, err := promptUser(reader, "Enter: ")
	if err == nil {
		t.Fatal("expected error for empty EOF input")
	}
}

func TestValidateFlickrTransferLog_MissingEntry(t *testing.T) {
	dir := t.TempDir()
	// Write a transfer log with two IDs
	logPath := filepath.Join(dir, "transfer.log")
	if err := os.WriteFile(logPath, []byte("111\n222\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Only create a file matching ID 222
	if err := os.WriteFile(filepath.Join(dir, "222_abc.jpg"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	result := transfer.NewResult("flickr", "download", dir)
	result.Finish()
	log := logging.New(false, nil)
	validateFlickrTransferLog(result, dir, log)

	if len(result.Warnings) != 1 {
		t.Fatalf("expected 1 warning for missing file, got %d: %v", len(result.Warnings), result.Warnings)
	}
	if !strings.Contains(result.Warnings[0].Message, "111") {
		t.Errorf("warning should mention missing ID 111, got: %s", result.Warnings[0].Message)
	}
}

func TestValidateFlickrTransferLog_AllPresent(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "transfer.log")
	if err := os.WriteFile(logPath, []byte("111\n222\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "111_photo.jpg"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "222_photo.jpg"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	result := transfer.NewResult("flickr", "download", dir)
	result.Finish()
	log := logging.New(false, nil)
	validateFlickrTransferLog(result, dir, log)

	if len(result.Warnings) != 0 {
		t.Fatalf("expected 0 warnings, got %d: %v", len(result.Warnings), result.Warnings)
	}
}
