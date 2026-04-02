package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
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

func TestS3UploadCmd_RequiresBucketFlag(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"s3", "upload", "/tmp/photos"})
	buf := new(bytes.Buffer)
	cmd.SetErr(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing --bucket flag")
	}
}

func TestS3DownloadCmd_RequiresBucketFlag(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"s3", "download", "/tmp/photos"})
	buf := new(bytes.Buffer)
	cmd.SetErr(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing --bucket flag")
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
	// s3 upload requires --bucket, so it'll error, but PersistentPreRunE fires first
	cmd.SetArgs([]string{"--no-metadata", "s3", "upload", "--bucket", "b", "/tmp"})
	buf := new(bytes.Buffer)
	cmd.SetErr(buf)
	cmd.SetOut(new(bytes.Buffer))
	_ = cmd.Execute()
	if !strings.Contains(buf.String(), "--no-metadata has no effect") {
		t.Errorf("expected no-metadata warning on stderr, got: %q", buf.String())
	}
}

func TestReadAWSCredentials_ValidFile(t *testing.T) {
	content := "[default]\naws_access_key_id = AKIAIOSFODNN7EXAMPLE\naws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY\n"
	f := t.TempDir() + "/credentials"
	if err := os.WriteFile(f, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := readAWSCredentials(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AccessKeyID != "AKIAIOSFODNN7EXAMPLE" {
		t.Errorf("AccessKeyID = %q, want AKIAIOSFODNN7EXAMPLE", cfg.AccessKeyID)
	}
	if cfg.SecretAccessKey != "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY" {
		t.Errorf("SecretAccessKey = %q, want wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", cfg.SecretAccessKey)
	}
}

func TestReadAWSCredentials_MissingFile(t *testing.T) {
	_, err := readAWSCredentials("/nonexistent/path/credentials")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadAWSCredentials_NoDefaultProfile(t *testing.T) {
	content := "[other-profile]\naws_access_key_id = AKID\naws_secret_access_key = SECRET\n"
	f := t.TempDir() + "/credentials"
	if err := os.WriteFile(f, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := readAWSCredentials(f)
	if err == nil {
		t.Fatal("expected error when [default] profile is missing")
	}
	if !strings.Contains(err.Error(), "default") {
		t.Errorf("error = %q, want mention of 'default'", err.Error())
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
	if !strings.Contains(buf.String(), "--date-range has no effect on flickr") {
		t.Errorf("expected date-range warning on stderr, got: %q", buf.String())
	}
}
