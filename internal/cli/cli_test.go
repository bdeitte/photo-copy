package cli

import (
	"bytes"
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
