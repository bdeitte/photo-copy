package google

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// takeoutJSON represents the JSON sidecar file structure from Google Takeout.
type takeoutJSON struct {
	Title          string `json:"title"`
	Description    string `json:"description"`
	PhotoTakenTime struct {
		Timestamp string `json:"timestamp"`
	} `json:"photoTakenTime"`
}

// takeoutMeta holds parsed metadata from a Google Takeout JSON sidecar.
type takeoutMeta struct {
	Title          string
	Description    string
	PhotoTakenTime time.Time
}

// parseTakeoutJSON parses a Google Takeout JSON sidecar into takeoutMeta.
// A zero or missing timestamp results in a zero time.Time.
func parseTakeoutJSON(data []byte) (*takeoutMeta, error) {
	var raw takeoutJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing takeout JSON: %w", err)
	}

	meta := &takeoutMeta{
		Title:       raw.Title,
		Description: raw.Description,
	}

	if raw.PhotoTakenTime.Timestamp != "" {
		epochSec, err := strconv.ParseInt(raw.PhotoTakenTime.Timestamp, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parsing photoTakenTime timestamp %q: %w", raw.PhotoTakenTime.Timestamp, err)
		}
		if epochSec != 0 {
			meta.PhotoTakenTime = time.Unix(epochSec, 0)
		}
	}

	return meta, nil
}
