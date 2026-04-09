package transfer

import (
	"strings"
	"testing"
	"time"
)

// fakeClock is a test clock that advances only when explicitly told to,
// so estimator tests are deterministic and independent of wall-clock time.
type fakeClock struct {
	t time.Time
}

func (c *fakeClock) now() time.Time       { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

// newTestEstimator returns an Estimator wired to a fakeClock so tests can
// control elapsed time precisely.
func newTestEstimator() (*Estimator, *fakeClock) {
	c := &fakeClock{t: time.Unix(1700000000, 0)}
	e := &Estimator{now: c.now}
	return e, c
}

func TestEstimator_BelowThreshold(t *testing.T) {
	e, c := newTestEstimator()
	// First Tick starts the clock; it does not count as a measured item.
	// estimateThreshold total Ticks gives processed = threshold-1, below threshold.
	for i := 0; i < estimateThreshold; i++ {
		c.advance(time.Second)
		e.Tick()
	}
	if got := e.Estimate(100); got != "" {
		t.Errorf("expected empty estimate below threshold, got %q", got)
	}
}

func TestEstimator_AtThreshold(t *testing.T) {
	e, c := newTestEstimator()
	// One baseline Tick plus estimateThreshold measured Ticks.
	e.Tick()
	for i := 0; i < estimateThreshold; i++ {
		c.advance(time.Second)
		e.Tick()
	}
	got := e.Estimate(100)
	if got == "" {
		t.Error("expected non-empty estimate at threshold")
	}
}

func TestEstimator_ZeroRemaining(t *testing.T) {
	e, c := newTestEstimator()
	e.Tick()
	for i := 0; i < estimateThreshold; i++ {
		c.advance(time.Second)
		e.Tick()
	}
	if got := e.Estimate(0); got != "" {
		t.Errorf("expected empty estimate for zero remaining, got %q", got)
	}
}

// TestEstimator_CumulativeAverage verifies that with a consistent per-item
// duration, the estimate is exactly (avg * remaining).
func TestEstimator_CumulativeAverage(t *testing.T) {
	e, c := newTestEstimator()
	e.Tick() // baseline
	for i := 0; i < 20; i++ {
		c.advance(time.Second)
		e.Tick()
	}
	// elapsed = 20s, processed = 20, avg = 1s. 100 remaining → 1 min 40 sec.
	got := e.Estimate(100)
	if got != "[Estimated 1 min. 40 sec. left] " {
		t.Errorf("expected ~100 sec estimate, got %q", got)
	}
}

// TestEstimator_StartupDelayIgnored is a regression test: a long delay
// between NewEstimator() and the first Tick (e.g., rclone's compare phase
// before any copy events) must not inflate the cumulative per-item average.
func TestEstimator_StartupDelayIgnored(t *testing.T) {
	e, c := newTestEstimator()
	// Simulate 60 seconds of pre-first-Tick setup.
	c.advance(60 * time.Second)
	e.Tick() // baseline; clock anchors here, not at construction
	// 20 items at 1 second each.
	for i := 0; i < 20; i++ {
		c.advance(time.Second)
		e.Tick()
	}
	// elapsed (from first Tick) = 20s, processed = 20, avg = 1s.
	// 100 remaining → ETA = 100 s = "1 min. 40 sec." (NOT ~11 min, which
	// is what the old elapsed-from-construction impl produced).
	got := e.Estimate(100)
	if got != "[Estimated 1 min. 40 sec. left] " {
		t.Errorf("startup delay leaked into estimate: %q", got)
	}
}

// TestEstimator_FastBurstDoesNotCollapseEstimate is a regression test for
// the sliding-window bug where a run of very fast items would drag the
// estimate down to near-zero even though many files were left. With the
// cumulative-average implementation, the overall rate stays accurate
// regardless of short-term bursts.
func TestEstimator_FastBurstDoesNotCollapseEstimate(t *testing.T) {
	e, c := newTestEstimator()
	e.Tick() // baseline
	// 1000 items at 1 second each.
	for i := 0; i < 1000; i++ {
		c.advance(time.Second)
		e.Tick()
	}
	// Fast burst: 100 more ticks without advancing the clock at all.
	for i := 0; i < 100; i++ {
		e.Tick()
	}
	// processed = 1100, elapsed = 1000 s, avg ≈ 909 ms.
	// 500 remaining → ETA ≈ 454 s ≈ 7 min 34 sec.
	got := e.Estimate(500)
	if got == "" {
		t.Fatal("expected non-empty estimate")
	}
	// Key assertion: must not collapse to "0 sec" or "1 sec" like the
	// sliding-window version did when the window was dominated by the burst.
	if strings.Contains(got, " 0 sec.") || strings.Contains(got, " 1 sec.") {
		t.Errorf("estimate collapsed to near-zero after fast burst: %q", got)
	}
}

// TestEstimator_SpikeAmortized verifies that a large one-off spike (e.g. a
// 60-second HTTP retry backoff) does not dominate the estimate — it gets
// amortized across all processed items.
func TestEstimator_SpikeAmortized(t *testing.T) {
	e, c := newTestEstimator()
	e.Tick() // baseline
	// 100 items at 1 second each.
	for i := 0; i < 100; i++ {
		c.advance(time.Second)
		e.Tick()
	}
	baseline := e.Estimate(100)
	if baseline == "" {
		t.Fatal("expected baseline estimate")
	}
	// baseline: elapsed=100s, processed=100, avg=1s, ETA(100)=100s = "1 min. 40 sec."
	if baseline != "[Estimated 1 min. 40 sec. left] " {
		t.Errorf("unexpected baseline: %q", baseline)
	}

	// 60-second spike (e.g., HTTP retry backoff) followed by one more tick.
	c.advance(60 * time.Second)
	e.Tick()
	got := e.Estimate(100)
	// processed=101, elapsed=160s, avg≈1.584s, ETA(100)≈158s = "2 min. 38 sec."
	if got == "" {
		t.Fatal("expected non-empty estimate after spike")
	}
	if got == baseline {
		t.Errorf("spike should shift estimate; still %q", got)
	}
	// Sanity-check the expected post-spike value (deterministic under fake clock).
	if got != "[Estimated 2 min. 38 sec. left] " {
		t.Errorf("unexpected post-spike estimate: %q", got)
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
