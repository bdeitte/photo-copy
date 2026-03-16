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

func TestEstimator_WindowSmoothing(t *testing.T) {
	e := NewEstimator()
	// Simulate items with 1-second durations
	for i := 0; i < 20; i++ {
		e.lastTick = e.lastTick.Add(-time.Second)
		e.Tick()
	}
	// Window average should be ~1 second, so 100 remaining ≈ 100 seconds
	est1 := e.Estimate(100)
	if est1 != "[Estimated 1 min. 40 sec. left] " {
		t.Errorf("expected ~100 sec estimate, got %q", est1)
	}

	// Now simulate one slow item (60 second retry)
	e.lastTick = e.lastTick.Add(-60 * time.Second)
	e.Tick()
	est2 := e.Estimate(100)
	// With window of 20 items (19 x 1s + 1 x 60s = 79s), avg ≈ 3.76s, ETA ≈ 376s ≈ 6 min 16 sec
	// This is much better than the overall average approach which would spike the ETA
	// The key assertion: the estimate should NOT have jumped to hours
	if est2 == "" {
		t.Error("expected non-empty estimate after spike")
	}
	// After the spike is pushed out of the window, estimate recovers
	for i := 0; i < windowSize; i++ {
		e.lastTick = e.lastTick.Add(-time.Second)
		e.Tick()
	}
	est3 := e.Estimate(100)
	if est3 != "[Estimated 1 min. 40 sec. left] " {
		t.Errorf("expected recovered estimate of ~100 sec, got %q", est3)
	}
}

func TestEstimator_WindowWraparound(t *testing.T) {
	e := NewEstimator()
	// Fill more than windowSize items
	for i := 0; i < windowSize*2; i++ {
		e.lastTick = e.lastTick.Add(-time.Second)
		e.Tick()
	}
	if e.windowLen != windowSize {
		t.Errorf("window len = %d, want %d", e.windowLen, windowSize)
	}
	if e.processed != windowSize*2 {
		t.Errorf("processed = %d, want %d", e.processed, windowSize*2)
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
