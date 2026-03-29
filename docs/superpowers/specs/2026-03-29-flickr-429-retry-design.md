# Flickr 429 Unlimited Retry Design

## Problem

When downloading large Flickr libraries (2000+ files), the Flickr API begins returning HTTP 429 (Too Many Requests) consistently. The current retry logic caps at 7 attempts per request, after which the file is marked as failed and the next file is attempted. Since Flickr is still rate limiting, the next file also exhausts its retries and fails, creating a cascade where every subsequent file fails.

The throttle interval caps at 30 seconds, which is insufficient when Flickr's rolling quota window has been exhausted.

## Design

### Change: Split retry behavior by status code in `retryableGet`

**File:** `internal/flickr/flickr.go`

Currently `retryableGet` treats 429 and 5xx identically — both get max 7 retries. These are fundamentally different situations:

- **429** means "slow down" — the correct response is to keep waiting.
- **5xx** means "something is broken" — giving up after retries is reasonable.

**429 behavior (new):**
- Retry indefinitely (no max retry count).
- Backoff escalates: 2s, 4s, 8s, 16s, 32s, 1m, 2m, 5m, then stays at 5m per attempt.
- Max backoff cap: 5 minutes. Long enough for Flickr's quota to slide, short enough to resume promptly.
- Continue calling `onRateLimited()` to bump the throttle interval for subsequent requests.
- `Retry-After` header still honored when present (overrides calculated backoff).
- Log format: "HTTP 429, retrying in Xm (attempt N)" — no max shown, so the user knows it won't give up.
- No upper limit on total wait time. User can Ctrl+C to cancel.

**5xx behavior (unchanged):**
- Max 7 retries with exponential backoff (2s base, doubling).
- After 7 retries, return error.
- Log format unchanged: "HTTP 5xx, retrying in X (attempt N/7)".

### Change: Update README

Document that Flickr downloads now retry 429s indefinitely instead of failing after 7 attempts.

## Scope

- Only `internal/flickr/flickr.go` changes (the `retryableGet` function and `retryDelay`).
- `README.md` updated to document the behavior.
- No new flags, no changes to the download loop, no changes to other packages.

## Testing

- Existing retry tests updated to reflect split behavior.
- New test: verify 429 retries continue beyond 7 attempts and eventually succeed.
- New test: verify 5xx still fails after 7 retries (existing behavior preserved).
