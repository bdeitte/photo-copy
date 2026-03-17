package flickr

import (
	"strconv"
	"time"
)

const flickrDateFormat = "2006-01-02 15:04:05"

// resolvePhotoDate parses date_taken and date_upload from the Flickr API,
// returning the best available time. Prefers date_taken; falls back to
// date_upload. Returns zero time if both are unusable.
func resolvePhotoDate(dateTaken, dateUpload string) time.Time {
	if dateTaken != "" && dateTaken != "0000-00-00 00:00:00" {
		if t, err := time.Parse(flickrDateFormat, dateTaken); err == nil {
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
