package cli

import (
	"bytes"
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

func TestGoogleImportTakeoutCmd_RequiresTwoArgs(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"google", "import-takeout", "/only/one"})
	buf := new(bytes.Buffer)
	cmd.SetErr(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing second arg")
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
	if !strings.Contains(buf.String(), "--date-range has no effect on config") {
		t.Errorf("expected date-range config warning on stderr, got: %q", buf.String())
	}
}
