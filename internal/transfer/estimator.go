package transfer

import (
	"fmt"
	"time"
)

const estimateThreshold = 10

// Estimator tracks transfer progress and provides time-remaining estimates.
// It uses a cumulative average (total elapsed time / total items processed)
// rather than a sliding window. This is stable against heterogeneous item
// durations — for example, bursts of very fast small-file extracts won't
// collapse the estimate to near-zero just because the most recent items were
// fast. Retry spikes (e.g., HTTP 429 backoffs) are gracefully amortized over
// the full run rather than dominating a small window.
//
// The clock starts on the first Tick, not at construction time, so any
// pre-first-Tick setup (for example, rclone's compare phase before the first
// copy event) is excluded from the per-item average.
type Estimator struct {
	now       func() time.Time // injectable for tests; defaults to time.Now
	startTime time.Time
	started   bool
	processed int
}

// NewEstimator creates a new time estimator. Call Tick after each item completes.
func NewEstimator() *Estimator {
	return &Estimator{now: time.Now}
}

// Tick records that one work item (download/upload) has completed. The first
// Tick establishes the clock baseline rather than counting as a measured item,
// so that any pre-first-Tick setup/compare delay is not folded into the
// per-item rate.
func (e *Estimator) Tick() {
	if !e.started {
		e.startTime = e.now()
		e.started = true
		return
	}
	e.processed++
}

// Estimate returns a formatted estimate string like "[Estimated 5 min. 23 sec. left] "
// or an empty string if not enough data yet.
//
// Callers may call Estimate before or after Tick for a given item:
//   - Tick-then-Estimate (Flickr download): estimate shown on the completion log line
//   - Estimate-then-Tick (Google upload): estimate shown before work starts
func (e *Estimator) Estimate(remaining int) string {
	if e.processed < estimateThreshold || remaining <= 0 {
		return ""
	}

	elapsed := e.now().Sub(e.startTime)
	avg := elapsed / time.Duration(e.processed)
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
