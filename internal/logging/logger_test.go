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
