package flickr

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/logging"
)

const (
	maxRetries             = 7
	baseRetryDelay         = 2 * time.Second
	minRequestInterval     = time.Second // Stay under 3,600 requests/hour
	maxConsecutiveFailures = 10          // Abort upload after this many consecutive failures
)

const (
	defaultAPIBaseURL  = "https://api.flickr.com/services/rest/"
	defaultUploadURL   = "https://up.flickr.com/services/upload/"
	defaultOAuthURL    = "https://www.flickr.com/services/oauth"
	transferLogFile    = "transfer.log"
)

// HTTPStatusError records a non-OK HTTP status code from a download.
type HTTPStatusError struct {
	StatusCode int
	URL        string
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("HTTP %d downloading %s", e.StatusCode, e.URL)
}

func apiURL() string {
	if u := os.Getenv("PHOTO_COPY_FLICKR_API_URL"); u != "" {
		return u
	}
	return defaultAPIBaseURL
}

func flickrUploadURL() string {
	if u := os.Getenv("PHOTO_COPY_FLICKR_UPLOAD_URL"); u != "" {
		return u
	}
	return defaultUploadURL
}

func oauthBaseURL() string {
	base := defaultOAuthURL
	if u := os.Getenv("PHOTO_COPY_FLICKR_OAUTH_URL"); u != "" {
		base = u
	}
	return strings.TrimRight(base, "/")
}

func isTestMode() bool {
	return os.Getenv("PHOTO_COPY_TEST_MODE") != ""
}

// Client provides Flickr API operations.
type Client struct {
	cfg             *config.FlickrConfig
	http            *http.Client
	log             *logging.Logger
	lastRequest     time.Time
	throttleInterval time.Duration // current throttle interval, adapts on 429s
}

// NewClient creates a new Flickr client.
func NewClient(cfg *config.FlickrConfig, log *logging.Logger) *Client {
	return &Client{
		cfg:              cfg,
		http:             &http.Client{},
		log:              log,
		throttleInterval: minRequestInterval,
	}
}

// throttle ensures we don't exceed the Flickr API rate limit of 3,600 requests/hour.
// The interval adapts: it increases on 429 responses and gradually decreases on success.
func (c *Client) throttle() {
	if isTestMode() {
		return
	}
	if !c.lastRequest.IsZero() {
		elapsed := time.Since(c.lastRequest)
		if elapsed < c.throttleInterval {
			time.Sleep(c.throttleInterval - elapsed)
		}
	}
	c.lastRequest = time.Now()
}

// maxThrottleInterval is the maximum adaptive throttle interval (30 seconds between requests).
const maxThrottleInterval = 30 * time.Second

// onRateLimited increases the throttle interval when a 429 is received.
func (c *Client) onRateLimited() {
	newInterval := c.throttleInterval * 2
	if newInterval > maxThrottleInterval {
		newInterval = maxThrottleInterval
	}
	if newInterval != c.throttleInterval {
		c.throttleInterval = newInterval
		c.log.Info("rate limited, increasing request interval to %v", c.throttleInterval)
	}
}

// onRequestSuccess gradually decreases the throttle interval after successful requests.
func (c *Client) onRequestSuccess() {
	if c.throttleInterval > minRequestInterval {
		newInterval := c.throttleInterval * 3 / 4
		if newInterval < minRequestInterval {
			newInterval = minRequestInterval
		}
		if newInterval != c.throttleInterval {
			c.throttleInterval = newInterval
			c.log.Debug("decreasing request interval to %v", c.throttleInterval)
		}
	}
}
