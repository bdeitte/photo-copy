package flickr

import (
	"strconv"
	"time"
)

const flickrDateFormat = "2006-01-02 15:04:05"

// flickrEpochSentinel is the earliest date we accept from Flickr. The API
// returns "1970-01-01 00:00:00" as date_taken for videos when the actual
// capture date is unknown — treat anything before 1990 as a sentinel.
var flickrEpochSentinel = time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC)

// resolvePhotoDate parses date_taken and date_upload from the Flickr API,
// returning the best available time. Prefers date_taken; falls back to
// date_upload. Returns zero time if both are unusable.
func resolvePhotoDate(dateTaken, dateUpload string) time.Time {
	if dateTaken != "" && dateTaken != "0000-00-00 00:00:00" {
		if t, err := time.Parse(flickrDateFormat, dateTaken); err == nil && t.After(flickrEpochSentinel) {
			return t
		}
	}

	if dateUpload != "" {
		if epoch, err := strconv.ParseInt(dateUpload, 10, 64); err == nil && epoch > 0 {
			return time.Unix(epoch, 0)
		}
	}

	return time.Time{}
}
