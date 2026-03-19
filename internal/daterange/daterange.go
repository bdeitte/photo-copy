// Package daterange provides date range parsing and matching for the --date-range flag.
package daterange

import (
	"fmt"
	"strings"
	"time"
)

const dateFormat = "2006-01-02"

// DateRange represents an optional date range with inclusive bounds.
// Nil fields indicate no bound in that direction.
type DateRange struct {
	After  *time.Time // start of range (inclusive); nil = no lower bound
	Before *time.Time // start of day AFTER end date (exclusive); nil = no upper bound
}

// Parse parses a date range string in the format "YYYY-MM-DD:YYYY-MM-DD".
// Either side may be empty for open-ended ranges: "2020-01-01:", ":2023-12-31".
// Both sides empty (":") is an error.
func Parse(s string) (*DateRange, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid date range %q: expected format YYYY-MM-DD:YYYY-MM-DD", s)
	}

	startStr, endStr := parts[0], parts[1]
	if startStr == "" && endStr == "" {
		return nil, fmt.Errorf("invalid date range %q: at least one date must be specified", s)
	}

	dr := &DateRange{}

	if startStr != "" {
		t, err := time.ParseInLocation(dateFormat, startStr, time.Local)
		if err != nil {
			return nil, fmt.Errorf("invalid start date %q: %w", startStr, err)
		}
		dr.After = &t
	}

	if endStr != "" {
		t, err := time.ParseInLocation(dateFormat, endStr, time.Local)
		if err != nil {
			return nil, fmt.Errorf("invalid end date %q: %w", endStr, err)
		}
		nextDay := t.AddDate(0, 0, 1)
		dr.Before = &nextDay
	}

	if dr.After != nil && dr.Before != nil && !dr.After.Before(*dr.Before) {
		return nil, fmt.Errorf("invalid date range: start %s is not before end %s", startStr, endStr)
	}

	return dr, nil
}

// Contains reports whether t falls within the date range.
// A nil DateRange contains all times.
func (dr *DateRange) Contains(t time.Time) bool {
	if dr == nil {
		return true
	}
	if dr.After != nil && t.Before(*dr.After) {
		return false
	}
	if dr.Before != nil && !t.Before(*dr.Before) {
		return false
	}
	return true
}
