package daterange

import (
	"testing"
	"time"
)

func TestParseBothBounds(t *testing.T) {
	dr, err := Parse("2020-01-01:2023-12-31")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dr.After == nil || dr.Before == nil {
		t.Fatal("expected both bounds to be set")
	}
	expectedAfter := time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local)
	if !dr.After.Equal(expectedAfter) {
		t.Errorf("After = %v, want %v", *dr.After, expectedAfter)
	}
	expectedBefore := time.Date(2024, 1, 1, 0, 0, 0, 0, time.Local)
	if !dr.Before.Equal(expectedBefore) {
		t.Errorf("Before = %v, want %v", *dr.Before, expectedBefore)
	}
}

func TestParseOpenStart(t *testing.T) {
	dr, err := Parse(":2023-12-31")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dr.After != nil {
		t.Error("expected After to be nil")
	}
	if dr.Before == nil {
		t.Fatal("expected Before to be set")
	}
}

func TestParseOpenEnd(t *testing.T) {
	dr, err := Parse("2020-01-01:")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dr.After == nil {
		t.Fatal("expected After to be set")
	}
	if dr.Before != nil {
		t.Error("expected Before to be nil")
	}
}

func TestParseBadFormat(t *testing.T) {
	cases := []string{"", "2020-01-01", "bad:date", "2023-12-31:2020-01-01"}
	for _, c := range cases {
		if _, err := Parse(c); err == nil {
			t.Errorf("Parse(%q) should have returned error", c)
		}
	}
}

func TestParseEmptyBothSides(t *testing.T) {
	_, err := Parse(":")
	if err == nil {
		t.Error("Parse(':') should return error")
	}
}

func TestContains(t *testing.T) {
	dr, _ := Parse("2020-01-01:2023-12-31")
	tests := []struct {
		t    time.Time
		want bool
	}{
		{time.Date(2019, 12, 31, 23, 59, 59, 0, time.Local), false},
		{time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local), true},
		{time.Date(2022, 6, 15, 12, 0, 0, 0, time.Local), true},
		{time.Date(2023, 12, 31, 23, 59, 59, 999999999, time.Local), true},
		{time.Date(2024, 1, 1, 0, 0, 0, 0, time.Local), false},
	}
	for _, tt := range tests {
		if got := dr.Contains(tt.t); got != tt.want {
			t.Errorf("Contains(%v) = %v, want %v", tt.t, got, tt.want)
		}
	}
}

func TestContainsNilRange(t *testing.T) {
	var dr *DateRange
	if !dr.Contains(time.Now()) {
		t.Error("nil DateRange should contain all times")
	}
}
