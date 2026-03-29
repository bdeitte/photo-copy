# Flickr 429 Unlimited Retry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Flickr downloads retry HTTP 429 responses indefinitely instead of failing after 7 attempts, so large downloads survive rate limit windows without manual intervention.

**Architecture:** Split the `retryableGet` retry loop into two paths — 429s retry forever with backoff capped at 5 minutes, 5xx errors keep the existing 7-retry limit. Add a `maxRateLimitBackoff` constant. Update README to document the new behavior.

**Tech Stack:** Go, net/http, httptest

---

### Task 1: Add `maxRateLimitBackoff` constant and update `retryDelay` for 429

**Files:**
- Modify: `internal/flickr/flickr.go:32-36` (constants)
- Modify: `internal/flickr/flickr.go:195-206` (`retryDelay`)
- Test: `internal/flickr/flickr_test.go`

- [ ] **Step 1: Write failing test for 429 backoff cap at 5 minutes**

Add a test that verifies `retryDelay` caps at 5 minutes for high attempt numbers (429 scenario):

```go
func TestRetryDelay_CapsAt5MinutesForHighAttempts(t *testing.T) {
	t.Setenv("PHOTO_COPY_TEST_MODE", "")
	c := newTestClient()
	resp := &http.Response{Header: http.Header{}}

	// At attempt 20, uncapped exponential would be enormous.
	// With 429 cap, it should be maxRateLimitBackoff (5 minutes).
	delay := c.retryDelay(20, resp)

	// Without the cap, 2s * 2^20 = ~2097152s. With cap, should be 5m.
	if delay > 5*time.Minute {
		t.Errorf("expected delay capped at 5m, got %v", delay)
	}
	if delay < 5*time.Minute {
		t.Errorf("expected delay to reach 5m cap at attempt 20, got %v", delay)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/flickr/ -run TestRetryDelay_CapsAt5MinutesForHighAttempts -v`
Expected: FAIL — current `retryDelay` returns `2s * 2^20` which far exceeds 5 minutes.

- [ ] **Step 3: Add constant and update `retryDelay` to cap at 5 minutes**

In `internal/flickr/flickr.go`, add the constant after `maxThrottleInterval`:

```go
// maxRateLimitBackoff is the maximum backoff between retries for 429 responses (5 minutes).
const maxRateLimitBackoff = 5 * time.Minute
```

Update `retryDelay` to cap the exponential backoff:

```go
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
	delay := baseRetryDelay * time.Duration(math.Pow(2, float64(attempt)))
	if delay > maxRateLimitBackoff {
		delay = maxRateLimitBackoff
	}
	return delay
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/flickr/ -run TestRetryDelay_CapsAt5MinutesForHighAttempts -v`
Expected: PASS

- [ ] **Step 5: Run all existing tests to verify no regressions**

Run: `go test ./internal/flickr/ -v`
Expected: All PASS — existing `retryDelay` tests should still pass since the cap only affects high attempt values.

- [ ] **Step 6: Commit**

```bash
git add internal/flickr/flickr.go internal/flickr/flickr_test.go
git commit -m "Add 5-minute backoff cap for high retry attempts"
```

---

### Task 2: Split `retryableGet` to retry 429 indefinitely

**Files:**
- Modify: `internal/flickr/flickr.go:152-193` (`retryableGet`)
- Test: `internal/flickr/flickr_test.go`

- [ ] **Step 1: Write failing test for 429 retrying beyond 7 attempts**

Add a test where the server returns 429 for 10 requests then succeeds. With the current code this fails after 7 retries:

```go
func TestRetryableGet_429RetriesIndefinitely(t *testing.T) {
	t.Setenv("PHOTO_COPY_TEST_MODE", "1")
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 10 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := newTestClient()
	c.http = server.Client()

	resp, err := c.retryableGet(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("expected success after 429s clear, got error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if attempts != 11 {
		t.Errorf("expected 11 attempts (10 x 429 + 1 success), got %d", attempts)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/flickr/ -run TestRetryableGet_429RetriesIndefinitely -v`
Expected: FAIL — `retryableGet` gives up after 7 retries with "HTTP 429 after 7 retries".

- [ ] **Step 3: Write failing test for 5xx still limited to 7 retries**

Add a test to confirm 5xx behavior is preserved:

```go
func TestRetryableGet_5xxStillLimitedTo7Retries(t *testing.T) {
	t.Setenv("PHOTO_COPY_TEST_MODE", "1")
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := newTestClient()
	c.http = server.Client()

	_, err := c.retryableGet(context.Background(), server.URL)
	if err == nil {
		t.Fatal("expected error after exhausting 5xx retries")
	}
	// 1 initial + 7 retries = 8 total attempts
	if attempts != 8 {
		t.Errorf("expected 8 attempts for 5xx, got %d", attempts)
	}
}
```

- [ ] **Step 4: Run test to verify it fails (or passes if current behavior matches)**

Run: `go test ./internal/flickr/ -run TestRetryableGet_5xxStillLimitedTo7Retries -v`
Expected: Should PASS with current code (verifying existing behavior). If it fails, note the actual attempt count.

- [ ] **Step 5: Rewrite `retryableGet` to split 429 and 5xx handling**

Replace the `retryableGet` function in `internal/flickr/flickr.go`:

```go
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
```

- [ ] **Step 6: Run all three new tests**

Run: `go test ./internal/flickr/ -run "TestRetryableGet_429RetriesIndefinitely|TestRetryableGet_5xxStillLimitedTo7Retries" -v`
Expected: Both PASS.

- [ ] **Step 7: Update `TestRetryableGet_ExhaustsRetries` for new behavior**

The existing `TestRetryableGet_ExhaustsRetries` test uses 429 and expects failure after 7 retries. Since 429 now retries indefinitely, this test needs to use 500 instead, or be replaced with a context-cancellation test for 429. Replace it:

```go
func TestRetryableGet_ExhaustsRetries(t *testing.T) {
	t.Setenv("PHOTO_COPY_TEST_MODE", "1")
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := newTestClient()
	c.http = server.Client()

	_, err := c.retryableGet(context.Background(), server.URL)
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if attempts != 8 {
		t.Errorf("expected 8 attempts (1 initial + 7 retries), got %d", attempts)
	}
}
```

- [ ] **Step 8: Run full test suite**

Run: `go test ./internal/flickr/ -v`
Expected: All PASS.

Run: `golangci-lint run ./...`
Expected: No errors.

- [ ] **Step 9: Commit**

```bash
git add internal/flickr/flickr.go internal/flickr/flickr_test.go
git commit -m "Retry 429 responses indefinitely instead of failing after 7 attempts

5xx server errors still fail after 7 retries. 429 backoff escalates
up to 5 minutes between attempts, allowing large Flickr downloads to
survive rate limit windows without manual intervention."
```

---

### Task 3: Update README

**Files:**
- Modify: `README.md:139-143` (Rate limiting & retry section)

- [ ] **Step 1: Update the Flickr rate limiting description in README**

In `README.md`, replace the first bullet under "### Rate limiting & retry" (line 141):

Old text:
```
- **Flickr** — Requests are throttled to stay under Flickr's 3,600 requests/hour API limit, starting at 1 request/second. The interval adapts automatically: on HTTP 429 (rate limit) responses, the interval doubles (up to 30s between requests), then gradually decreases back to 1/second as requests succeed. HTTP 429 and 5xx errors are retried up to 7 times with exponential backoff, honoring the `Retry-After` header when present. This applies to both API calls and photo downloads.
```

New text:
```
- **Flickr** — Requests are throttled to stay under Flickr's 3,600 requests/hour API limit, starting at 1 request/second. The interval adapts automatically: on HTTP 429 (rate limit) responses, the interval doubles (up to 30s between requests), then gradually decreases back to 1/second as requests succeed. HTTP 429 responses are retried indefinitely with exponential backoff capped at 5 minutes between attempts — large downloads will pause and resume automatically when Flickr's rate limit window resets. HTTP 5xx server errors are retried up to 7 times with exponential backoff. Both honor the `Retry-After` header when present. This applies to both API calls and photo downloads.
```

- [ ] **Step 2: Run lint**

Run: `golangci-lint run ./...`
Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "Update README to document unlimited 429 retry behavior"
```
