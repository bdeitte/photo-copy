package google

import (
	"testing"
	"time"
)

func TestParseTakeoutJSON_AllFields(t *testing.T) {
	data := []byte(`{
        "title": "Sunset Photo",
        "description": "Beautiful sunset at the beach",
        "photoTakenTime": {"timestamp": "1640000000", "formatted": "Dec 20, 2021"}
    }`)

	meta, err := parseTakeoutJSON(data)
	if err != nil {
		t.Fatalf("parseTakeoutJSON failed: %v", err)
	}
	if meta.Title != "Sunset Photo" {
		t.Errorf("Title = %q, want %q", meta.Title, "Sunset Photo")
	}
	if meta.Description != "Beautiful sunset at the beach" {
		t.Errorf("Description = %q, want %q", meta.Description, "Beautiful sunset at the beach")
	}
	wantTime := time.Unix(1640000000, 0)
	if !meta.PhotoTakenTime.Equal(wantTime) {
		t.Errorf("PhotoTakenTime = %v, want %v", meta.PhotoTakenTime, wantTime)
	}
}

func TestParseTakeoutJSON_MissingTimestamp(t *testing.T) {
	data := []byte(`{"title": "No Date Photo"}`)
	meta, err := parseTakeoutJSON(data)
	if err != nil {
		t.Fatalf("parseTakeoutJSON failed: %v", err)
	}
	if !meta.PhotoTakenTime.IsZero() {
		t.Errorf("expected zero time for missing timestamp, got %v", meta.PhotoTakenTime)
	}
}

func TestParseTakeoutJSON_ZeroTimestamp(t *testing.T) {
	data := []byte(`{"title": "Zero", "photoTakenTime": {"timestamp": "0"}}`)
	meta, err := parseTakeoutJSON(data)
	if err != nil {
		t.Fatalf("parseTakeoutJSON failed: %v", err)
	}
	if !meta.PhotoTakenTime.IsZero() {
		t.Errorf("expected zero time for timestamp '0', got %v", meta.PhotoTakenTime)
	}
}

func TestParseTakeoutJSON_EmptyDescription(t *testing.T) {
	data := []byte(`{"title": "Photo", "description": "", "photoTakenTime": {"timestamp": "1640000000"}}`)
	meta, err := parseTakeoutJSON(data)
	if err != nil {
		t.Fatalf("parseTakeoutJSON failed: %v", err)
	}
	if meta.Description != "" {
		t.Errorf("Description = %q, want empty", meta.Description)
	}
}

func TestParseTakeoutJSON_InvalidJSON(t *testing.T) {
	data := []byte(`not json`)
	_, err := parseTakeoutJSON(data)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
