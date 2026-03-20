package logging

import (
	"bytes"
	"testing"
)

func TestDebugLogger_Enabled(t *testing.T) {
	var buf bytes.Buffer
	log := New(true, &buf)

	log.Debug("downloading file %s", "photo.jpg")

	got := buf.String()
	if got == "" {
		t.Fatal("expected debug output, got empty string")
	}
	if !bytes.Contains([]byte(got), []byte("downloading file photo.jpg")) {
		t.Fatalf("unexpected output: %s", got)
	}
}

func TestDebugLogger_Disabled(t *testing.T) {
	var buf bytes.Buffer
	log := New(false, &buf)

	log.Debug("this should not appear")

	if buf.Len() != 0 {
		t.Fatalf("expected no output when debug disabled, got: %s", buf.String())
	}
}

func TestInfoLogger_AlwaysOutputs(t *testing.T) {
	var buf bytes.Buffer
	log := New(false, &buf)

	log.Info("always visible")

	got := buf.String()
	if got == "" {
		t.Fatal("expected info output even when debug disabled")
	}
}

func TestErrorLogger_AlwaysOutputs(t *testing.T) {
	var buf bytes.Buffer
	log := New(false, &buf)

	log.Error("something went wrong")

	got := buf.String()
	if got == "" {
		t.Fatal("expected error output even when debug disabled")
	}
	if !bytes.Contains([]byte(got), []byte("something went wrong")) {
		t.Fatalf("unexpected output: %s", got)
	}
}

func TestErrorLogger_HasPrefix(t *testing.T) {
	var buf bytes.Buffer
	log := New(false, &buf)

	log.Error("disk full")

	got := buf.String()
	if !bytes.Contains([]byte(got), []byte("ERROR: ")) {
		t.Fatalf("expected ERROR: prefix, got: %s", got)
	}
}

func TestLogger_DefaultsToStderr(t *testing.T) {
	// New(false, nil) should fall back to os.Stderr without panic.
	log := New(false, nil)
	if log.writer == nil {
		t.Fatal("expected writer to default to os.Stderr, got nil")
	}

	// Smoke-test: calls should not panic.
	log.Info("test message")
	log.Error("test error")
	log.Debug("test debug")
}
