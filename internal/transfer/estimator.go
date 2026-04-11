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
// By default the clock starts at NewEstimator() time, which is correct for
// callers that begin measurable work immediately. Callers with a non-trivial
// setup/compare phase before the first measurable unit of work (for example,
// rclone's scan phase before the first copy event) should call Start() when
// real work begins, so that pre-work delay is not folded into the per-item
// rate.
type Estimator struct {
	now       func() time.Time // injectable for tests; defaults to time.Now
	startTime time.Time
	processed int
}

// NewEstimator creates a new time estimator. Call Tick after each item completes.
func NewEstimator() *Estimator {
	now := time.Now
	return &Estimator{now: now, startTime: now()}
}

// NewEstimatorWithClock creates an Estimator that uses the provided clock
// function instead of time.Now. Intended for testing from external packages.
func NewEstimatorWithClock(now func() time.Time) *Estimator {
	return &Estimator{now: now, startTime: now()}
}

// Start (re)anchors the elapsed-time clock to now. Use this from callers that
// have a setup/compare phase between NewEstimator() and the first real unit of
// work, so that pre-work delay does not inflate the cumulative per-item
// average. Callers without such a setup phase can ignore this method.
func (e *Estimator) Start() {
	e.startTime = e.now()
}

// Tick records that one work item (download/upload) has completed.
func (e *Estimator) Tick() {
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
