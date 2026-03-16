package transfer

import (
	"testing"
	"time"
)

func TestEstimator_BelowThreshold(t *testing.T) {
	e := NewEstimator()
	for i := 0; i < estimateThreshold-1; i++ {
		e.Tick()
	}
	if got := e.Estimate(100); got != "" {
		t.Errorf("expected empty estimate below threshold, got %q", got)
	}
}

func TestEstimator_AtThreshold(t *testing.T) {
	e := NewEstimator()
	for i := 0; i < estimateThreshold; i++ {
		e.Tick()
	}
	got := e.Estimate(100)
	if got == "" {
		t.Error("expected non-empty estimate at threshold")
	}
}

func TestEstimator_ZeroRemaining(t *testing.T) {
	e := NewEstimator()
	for i := 0; i < estimateThreshold; i++ {
		e.Tick()
	}
	if got := e.Estimate(0); got != "" {
		t.Errorf("expected empty estimate for zero remaining, got %q", got)
	}
}

func TestFormatEstimate(t *testing.T) {
	tests := []struct {
		duration time.Duration
		want     string
	}{
		{0, "0 sec."},
		{30 * time.Second, "30 sec."},
		{59 * time.Second, "59 sec."},
		{60 * time.Second, "1 min."},
		{90 * time.Second, "1 min. 30 sec."},
		{5*time.Minute + 23*time.Second, "5 min. 23 sec."},
		{60 * time.Minute, "1 hr."},
		{90 * time.Minute, "1 hr. 30 min."},
		{2*time.Hour + 15*time.Minute, "2 hr. 15 min."},
	}
	for _, tt := range tests {
		got := formatEstimate(tt.duration)
		if got != tt.want {
			t.Errorf("formatEstimate(%v) = %q, want %q", tt.duration, got, tt.want)
		}
	}
}
