package transfer

import (
	"fmt"
	"time"
)

const (
	estimateThreshold = 10
	windowSize        = 50
)

// Estimator tracks transfer progress and provides time-remaining estimates.
// It uses a sliding window of recent item durations so that temporary spikes
// (e.g., HTTP 429 retry backoffs) smooth out quickly rather than permanently
// shifting the estimate.
type Estimator struct {
	processed int
	lastTick  time.Time
	window    []time.Duration // circular buffer, up to windowSize entries
	windowIdx int             // next write position (wraps around)
	windowLen int             // number of valid entries (grows to windowSize)
}

// NewEstimator creates a new time estimator. Call Tick after each item completes.
func NewEstimator() *Estimator {
	return &Estimator{
		lastTick: time.Now(),
		window:   make([]time.Duration, windowSize),
	}
}

// Tick records that one work item (download/upload) has completed.
// It measures wall-clock time since the previous Tick (or since creation
// for the first call), capturing the real per-item completion rate.
func (e *Estimator) Tick() {
	now := time.Now()
	dur := now.Sub(e.lastTick)
	e.lastTick = now

	e.window[e.windowIdx] = dur
	e.windowIdx = (e.windowIdx + 1) % windowSize
	if e.windowLen < windowSize {
		e.windowLen++
	}
	e.processed++
}

// Estimate returns a formatted estimate string like "[Estimated 5 min. 23 sec. left] "
// or empty string if not enough data yet.
//
// Callers may call Estimate before or after Tick for a given item:
//   - Tick-then-Estimate (Flickr download): estimate shown on the completion log line
//   - Estimate-then-Tick (Google upload): estimate shown before work starts
func (e *Estimator) Estimate(remaining int) string {
	if e.processed < estimateThreshold || remaining <= 0 || e.windowLen == 0 {
		return ""
	}

	var total time.Duration
	for i := 0; i < e.windowLen; i++ {
		total += e.window[i]
	}
	avg := total / time.Duration(e.windowLen)
	eta := avg * time.Duration(remaining)
	return fmt.Sprintf("[Estimated %s left] ", formatEstimate(eta))
}

// formatEstimate formats a duration as a human-readable estimate.
func formatEstimate(d time.Duration) string {
	d = d.Truncate(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%d sec.", int(d.Seconds()))
	}
	totalSeconds := int(d.Seconds())
	minutes := totalSeconds / 60
	seconds := totalSeconds % 60
	if minutes < 60 {
		if seconds == 0 {
			return fmt.Sprintf("%d min.", minutes)
		}
		return fmt.Sprintf("%d min. %d sec.", minutes, seconds)
	}
	hours := minutes / 60
	minutes %= 60
	if minutes == 0 {
		return fmt.Sprintf("%d hr.", hours)
	}
	return fmt.Sprintf("%d hr. %d min.", hours, minutes)
}
