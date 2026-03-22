package flickr

import (
	"testing"
	"time"
)

func TestResolvePhotoDate_DateTaken(t *testing.T) {
	got := resolvePhotoDate("2020-06-15 14:30:00", "1592234567")
	want := time.Date(2020, 6, 15, 14, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("resolvePhotoDate = %v, want %v", got, want)
	}
}

func TestResolvePhotoDate_FallbackToDateUpload(t *testing.T) {
	got := resolvePhotoDate("", "1592234567")
	want := time.Unix(1592234567, 0)
	if !got.Equal(want) {
		t.Errorf("resolvePhotoDate = %v, want %v", got, want)
	}
}

func TestResolvePhotoDate_ZeroDateTakenFallback(t *testing.T) {
	got := resolvePhotoDate("0000-00-00 00:00:00", "1592234567")
	want := time.Unix(1592234567, 0)
	if !got.Equal(want) {
		t.Errorf("resolvePhotoDate = %v, want %v", got, want)
	}
}

func TestResolvePhotoDate_BothUnusable(t *testing.T) {
	got := resolvePhotoDate("", "")
	if !got.IsZero() {
		t.Errorf("resolvePhotoDate = %v, want zero time", got)
	}
}

func TestResolvePhotoDate_BothUnusable_BadValues(t *testing.T) {
	got := resolvePhotoDate("0000-00-00 00:00:00", "not-a-number")
	if !got.IsZero() {
		t.Errorf("resolvePhotoDate = %v, want zero time", got)
	}
}

func TestResolvePhotoDate_EpochSentinel(t *testing.T) {
	// Flickr returns "1970-01-01 00:00:00" for videos with unknown dates
	got := resolvePhotoDate("1970-01-01 00:00:00", "1592234567")
	want := time.Unix(1592234567, 0)
	if !got.Equal(want) {
		t.Errorf("resolvePhotoDate = %v, want fallback to dateUpload %v", got, want)
	}
}

func TestResolvePhotoDate_EpochSentinelNoFallback(t *testing.T) {
	got := resolvePhotoDate("1970-01-01 00:00:00", "")
	if !got.IsZero() {
		t.Errorf("resolvePhotoDate = %v, want zero time", got)
	}
}
