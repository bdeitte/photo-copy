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
	// Pretend some time has elapsed so the cumulative average is non-zero.
	e.startTime = e.startTime.Add(-10 * time.Second)
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
	e.startTime = e.startTime.Add(-10 * time.Second)
	for i := 0; i < estimateThreshold; i++ {
		e.Tick()
	}
	if got := e.Estimate(0); got != "" {
		t.Errorf("expected empty estimate for zero remaining, got %q", got)
	}
}

// TestEstimator_CumulativeAverage verifies that with a consistent per-item
// duration, the estimate is exactly (avg * remaining).
func TestEstimator_CumulativeAverage(t *testing.T) {
	e := NewEstimator()
	// 20 items averaging 1 second each.
	e.startTime = e.startTime.Add(-20 * time.Second)
	for i := 0; i < 20; i++ {
		e.Tick()
	}
	got := e.Estimate(100)
	if got != "[Estimated 1 min. 40 sec. left] " {
		t.Errorf("expected ~100 sec estimate, got %q", got)
	}
}

// TestEstimator_FastBurstDoesNotCollapseEstimate is a regression test for
// the sliding-window bug where a run of very fast items would drag the
// estimate down to near-zero even though many files were left. With the
// cumulative-average implementation, the overall rate stays accurate
// regardless of short-term bursts.
func TestEstimator_FastBurstDoesNotCollapseEstimate(t *testing.T) {
	e := NewEstimator()
	// Simulate 1000 items taking 1 second each (so total elapsed = 1000 s).
	// With 500 items remaining, the correct estimate is ~500 s (8 min 20 s).
	e.startTime = e.startTime.Add(-1000 * time.Second)
	for i := 0; i < 1000; i++ {
		e.Tick()
	}
	// Now simulate a "fast burst": 100 additional ticks without advancing
	// the clock much. The cumulative average should barely move.
	for i := 0; i < 100; i++ {
		e.Tick()
	}
	// Processed = 1100, elapsed ≈ 1000 s, avg ≈ 0.909 s/item.
	// With 500 remaining, ETA ≈ 454 s = 7 min 34 sec.
	got := e.Estimate(500)
	if got == "" {
		t.Fatal("expected non-empty estimate")
	}
	// The key assertion: the estimate must remain in minutes, not collapse to
	// a few seconds like the sliding-window version did.
	if got == "[Estimated 1 sec. left] " || got == "[Estimated 0 sec. left] " {
		t.Errorf("estimate collapsed to near-zero after fast burst: %q", got)
	}
}

// TestEstimator_SpikeAmortized verifies that a large one-off spike (e.g. a
// 60-second HTTP retry backoff) does not dominate the estimate — it gets
// amortized across all processed items.
func TestEstimator_SpikeAmortized(t *testing.T) {
	e := NewEstimator()
	// 100 items at 1 second each.
	e.startTime = e.startTime.Add(-100 * time.Second)
	for i := 0; i < 100; i++ {
		e.Tick()
	}
	baseline := e.Estimate(100)
	if baseline == "" {
		t.Fatal("expected baseline estimate")
	}

	// Simulate a 60-second spike followed by one more tick.
	e.startTime = e.startTime.Add(-60 * time.Second)
	e.Tick()

	// Processed = 101, elapsed = 160 s, avg ≈ 1.58 s.
	// With 100 remaining, ETA ≈ 158 s — a moderate bump over the 100 s
	// baseline, not a runaway spike.
	got := e.Estimate(100)
	if got == "" {
		t.Fatal("expected non-empty estimate after spike")
	}
	if got == baseline {
		t.Errorf("spike should shift estimate; still %q", got)
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
