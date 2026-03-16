package transfer

import (
	"fmt"
	"time"
)

const estimateThreshold = 10

// Estimator tracks transfer progress and provides time-remaining estimates.
type Estimator struct {
	start     time.Time
	processed int
}

// NewEstimator creates a new time estimator.
func NewEstimator() *Estimator {
	return &Estimator{}
}

// Tick records that one work item (download/upload) has completed.
func (e *Estimator) Tick() {
	if e.processed == 0 {
		e.start = time.Now()
	}
	e.processed++
}

// Estimate returns a formatted estimate string like "[Estimated 5 min. 23 sec. left] "
// or empty string if not enough data yet.
func (e *Estimator) Estimate(remaining int) string {
	if e.processed < estimateThreshold || remaining <= 0 {
		return ""
	}
	elapsed := time.Since(e.start)
	avgPerItem := elapsed / time.Duration(e.processed)
	eta := avgPerItem * time.Duration(remaining)
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
