package flickr

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// maxRateLimitBackoff is the maximum backoff between retries for 429 responses (5 minutes).
const maxRateLimitBackoff = 5 * time.Minute

// isFlickrPermanentError returns true for Flickr API error codes that are permanent
// and should not be retried. Transient errors like "Service currently unavailable"
// (code 105) are not in this list and will be retried.
func isFlickrPermanentError(code int) bool {
	switch code {
	case 1, // Photo/resource not found
		2,  // Permission denied
		95, // SSL required
		96, // Invalid signature
		97, // Missing signature
		98, // Login failed / Invalid auth token
		99, // Insufficient permissions
		100, // Invalid API key
		111, // Format not found
		112, // Method not found
		116: // Bad URL found
		return true
	default:
		return false
	}
}

// sanitizeURL strips OAuth and API key params from a URL for safe debug logging.
func sanitizeURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	for key := range q {
		if strings.HasPrefix(key, "oauth_") || key == "api_key" {
			q.Del(key)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// retryableGet performs an HTTP GET with retry logic.
// HTTP 429 (rate limited) responses are retried indefinitely with escalating backoff
// capped at 5 minutes. HTTP 5xx (server error) responses are retried up to 7 times.
func (c *Client) retryableGet(ctx context.Context, url string) (*http.Response, error) {
	serverErrors := 0
	rateLimitAttempt := 0

	for {
		c.throttle()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}

		c.log.Debug("HTTP GET %s", sanitizeURL(url))
		resp, err := c.http.Do(req)
		if err != nil {
			c.log.Debug("HTTP error: %v", err)
			return nil, err
		}
		c.log.Debug("HTTP response: %d %s", resp.StatusCode, resp.Status)

		if resp.StatusCode == http.StatusTooManyRequests {
			_ = resp.Body.Close()
			c.onRateLimited()
			rateLimitAttempt++
			delay := c.retryDelay(rateLimitAttempt-1, resp)
			c.log.Info("HTTP 429, retrying in %v (attempt %d)", delay, rateLimitAttempt)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
			continue
		}

		if resp.StatusCode >= 500 {
			_ = resp.Body.Close()
			serverErrors++
			if serverErrors > maxRetries {
				return nil, fmt.Errorf("HTTP %d after %d retries: %s", resp.StatusCode, maxRetries, url)
			}
			delay := c.retryDelay(serverErrors-1, resp)
			c.log.Info("HTTP %d, retrying in %v (attempt %d/%d)", resp.StatusCode, delay, serverErrors, maxRetries)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
			continue
		}

		c.onRequestSuccess()
		return resp, nil
	}
}

// retryDelay calculates the backoff delay, honoring the Retry-After header if present.
func (c *Client) retryDelay(attempt int, resp *http.Response) time.Duration {
	if isTestMode() {
		return time.Millisecond
	}
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		if seconds, err := strconv.Atoi(ra); err == nil {
			return time.Duration(seconds) * time.Second
		}
	}
	// Guard against time.Duration (int64 nanoseconds) overflow.
	// baseRetryDelay * (1<<attempt) overflows int64 around attempt 33.
	// The cap kicks in much earlier (attempt 8), so 28 is conservative.
	if attempt >= 28 {
		return maxRateLimitBackoff
	}
	delay := baseRetryDelay * time.Duration(1<<uint(attempt))
	if delay > maxRateLimitBackoff {
		delay = maxRateLimitBackoff
	}
	return delay
}

// buildAPIURL constructs a Flickr REST API URL (unsigned, for non-authenticated calls).
func (c *Client) buildAPIURL(method, apiKey string, params map[string]string) string {
	u, _ := url.Parse(c.apiURL())
	q := u.Query()
	q.Set("method", method)
	q.Set("api_key", apiKey)
	q.Set("format", "json")
	q.Set("nojsoncallback", "1")
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// signedAPIGet makes an OAuth-signed GET request to the Flickr REST API with rate limiting and retry.
// It retries when the API returns non-JSON responses (e.g. HTML error pages with 200 status).
// Note: each iteration calls retryableGet which has its own retry loop for 429/5xx, so in the
// worst case a single signedAPIGet call may make up to (maxRetries+1)^2 HTTP requests.
func (c *Client) signedAPIGet(ctx context.Context, method string, extra map[string]string) (*http.Response, error) {
	baseURL := c.apiURL()

	for attempt := 0; attempt <= maxRetries; attempt++ {
		params := map[string]string{
			"method":         method,
			"format":         "json",
			"nojsoncallback": "1",
		}
		for k, v := range extra {
			params[k] = v
		}

		oauthSign("GET", baseURL, params, c.cfg)

		v := url.Values{}
		for k, val := range params {
			v.Set(k, val)
		}
		resp, err := c.retryableGet(ctx, baseURL+"?"+v.Encode())
		if err != nil {
			return nil, err
		}

		// Flickr sometimes returns HTML error pages with a 200 status.
		// Detect this by checking Content-Type and retry.
		ct := resp.Header.Get("Content-Type")
		if ct != "" && !strings.Contains(ct, "json") && !strings.Contains(ct, "javascript") {
			// Read body to log the error details from Flickr
			bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024))
			_ = resp.Body.Close()
			bodySnippet := ""
			if readErr == nil && len(bodyBytes) > 0 {
				bodySnippet = string(bodyBytes)
			}
			// Check if this is a structured Flickr XML error response.
			// Only skip retries for known permanent errors (permission, auth, not found).
			// Transient errors like "Service currently unavailable" (code 105) should still retry.
			if strings.Contains(ct, "xml") && len(bodyBytes) > 0 {
				var rsp struct {
					Stat string `xml:"stat,attr"`
					Err  struct {
						Code int    `xml:"code,attr"`
						Msg  string `xml:"msg,attr"`
					} `xml:"err"`
				}
				if xmlErr := xml.Unmarshal(bodyBytes, &rsp); xmlErr == nil && rsp.Stat == "fail" {
					if isFlickrPermanentError(rsp.Err.Code) {
						return nil, fmt.Errorf("Flickr API error: %s (code %d)", rsp.Err.Msg, rsp.Err.Code) //nolint:staticcheck // proper noun
					}
				}
			}

			if attempt == maxRetries {
				return nil, fmt.Errorf("API returned non-JSON response (Content-Type: %s, status: %d, body: %s) after %d retries", ct, resp.StatusCode, bodySnippet, maxRetries)
			}
			delay := baseRetryDelay * time.Duration(math.Pow(2, float64(attempt)))
			if isTestMode() {
				delay = time.Millisecond
			}
			c.log.Info("API returned non-JSON response (Content-Type: %s, status: %d, body: %s), retrying in %v (attempt %d/%d)", ct, resp.StatusCode, bodySnippet, delay, attempt+1, maxRetries)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
			continue
		}

		// Buffer and log JSON response body for debugging.
		respBody, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("reading response body: %w", readErr)
		}
		if len(respBody) <= 4096 {
			c.log.Debug("API response body: %s", string(respBody))
		} else {
			c.log.Debug("API response body (%d bytes, truncated): %s", len(respBody), string(respBody[:4096]))
		}
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
		return resp, nil
	}
	return nil, fmt.Errorf("retries exhausted for %s API call", method)
}

// flexString handles JSON fields that may be a string or a number.
// The Flickr API sometimes returns numeric labels (e.g. 75 instead of "Square").
type flexString string

func (f *flexString) UnmarshalJSON(data []byte) error {
	// Try string first
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*f = flexString(s)
		return nil
	}
	// Fall back to number
	var n json.Number
	if err := json.Unmarshal(data, &n); err != nil {
		return fmt.Errorf("flexString: cannot unmarshal %s", string(data))
	}
	*f = flexString(n.String())
	return nil
}
