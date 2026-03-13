# Integration Testing with Mock Services — Design Spec

## Overview

Add CLI-level integration tests that execute cobra commands programmatically against configurable mock HTTP servers for Flickr and Google Photos. Tests verify the full command path: CLI argument parsing → config loading → client construction → HTTP interaction → filesystem output.

S3 integration testing is excluded — S3 operations delegate to a rclone subprocess, and rclone's own test coverage handles that layer. S3 unit tests already cover arg building, config generation, and binary resolution.

## Production Code Changes

### Environment Variable Overrides

Tests need to redirect service URLs, config directory, and disable throttling. Six env vars are introduced, all prefixed with `PHOTO_COPY_`:

| Env Var | Overrides | Used By |
|---------|-----------|---------|
| `PHOTO_COPY_CONFIG_DIR` | `config.DefaultDir()` return value | All CLI commands that load config |
| `PHOTO_COPY_FLICKR_API_URL` | `apiBaseURL` constant (`https://api.flickr.com/services/rest/`) | `buildAPIURL()`, `signedAPIGet()` |
| `PHOTO_COPY_FLICKR_UPLOAD_URL` | Hardcoded `https://up.flickr.com/services/upload/` | `uploadFile()` |
| `PHOTO_COPY_GOOGLE_API_URL` | Base of `uploadURL` and `batchCreateURL` | `uploadBytes()`, `createMediaItem()` |
| `PHOTO_COPY_GOOGLE_TOKEN` | When set to `"skip"`, bypass OAuth flow in `NewClient()` | `google.NewClient()` |
| `PHOTO_COPY_TEST_MODE` | When set, reduce throttle delays to zero | `throttle()` in flickr and google |

### File-by-file changes

**`internal/config/config.go`**
- `DefaultDir()` checks `os.Getenv("PHOTO_COPY_CONFIG_DIR")` first, falls back to `~/.config/photo-copy/`.

**`internal/flickr/flickr.go`**
- Replace the `apiBaseURL` constant with a `func apiURL() string` that checks `PHOTO_COPY_FLICKR_API_URL` env var, falling back to the constant. **Important**: `signedAPIGet()` uses `apiBaseURL` in two places — both the OAuth signing call (line 137: `oauthSign("GET", apiBaseURL, ...)`) and the request URL construction (line 143: `apiBaseURL+"?"+v.Encode()`). Both must use the same resolved value from `apiURL()`, captured once per call to avoid divergence. The mock doesn't validate OAuth signatures, but keeping them consistent avoids subtle implementation bugs.
- Replace the hardcoded upload URL `https://up.flickr.com/services/upload/` with a `func flickrUploadURL() string` that checks `PHOTO_COPY_FLICKR_UPLOAD_URL`. **Important**: This URL also appears in two places in `uploadFile()` — line 421 (OAuth signing) and line 435 (request construction). Both must use the resolved value.
- `throttle()` checks `PHOTO_COPY_TEST_MODE`; when set, skips the sleep.
- `retryDelay()` checks `PHOTO_COPY_TEST_MODE`; when set, returns 0 to avoid slow retry waits in tests.

**`internal/google/google.go`**
- Replace `uploadURL` and `batchCreateURL` constants with helper funcs that check `PHOTO_COPY_GOOGLE_API_URL` env var. When set, construct URLs as `<base>/v1/uploads` and `<base>/v1/mediaItems:batchCreate`.
- `NewClient()` checks `PHOTO_COPY_GOOGLE_TOKEN`; when set to `"skip"`, returns a `Client` with a plain `&http.Client{}`, skipping the entire OAuth2 flow (token loading, interactive auth, token persistence).
- `throttle()` checks `PHOTO_COPY_TEST_MODE`; when set, skips the sleep.
- `retryDelay()` checks `PHOTO_COPY_TEST_MODE`; when set, returns 0 to avoid slow retry waits in tests.

### Package-level variable reset

The `debug` and `limit` variables in `internal/cli/root.go` are package-level vars bound via `PersistentFlags().BoolVar()`/`IntVar()`. Each test calls `cli.NewRootCmd()` which re-binds these flags to the same addresses. Since cobra parses flags fresh each time, the values get set correctly per test invocation. However, if a test does NOT pass `--limit`, the variable retains whatever value the previous test set. To avoid this, each integration test must explicitly pass the flags it needs (including `--limit 0` when the default is desired) or we reset the variables in a test helper. The simplest approach: create a `resetFlags()` helper called at the start of each test that sets `debug = false` and `limit = 0`. This requires exporting them or using a package-level `ResetForTest()` function.

## Mock Server Package

New package: `internal/testutil/mockserver/`

Three files: `helpers.go`, `flickr.go`, `google.go`.

### helpers.go — Shared Types and Handler Factories

```go
// RecordedRequest captures request details for test assertions.
type RecordedRequest struct {
    Method  string
    Path    string
    Query   url.Values
    Headers http.Header
    Body    []byte
}

// HandlerFunc is the signature for configurable endpoint handlers.
type HandlerFunc func(w http.ResponseWriter, r *http.Request)

// RespondJSON returns a handler that responds with the given status and JSON body.
func RespondJSON(status int, body any) HandlerFunc

// RespondStatus returns a handler that responds with just a status code.
func RespondStatus(status int) HandlerFunc

// RespondBytes returns a handler that responds with raw bytes.
func RespondBytes(status int, data []byte) HandlerFunc

// RespondSequence returns a handler that uses a different handler for each
// successive call (e.g., 429 on first call, 200 on second).
func RespondSequence(handlers ...HandlerFunc) HandlerFunc
```

### flickr.go — Flickr Mock Server

```go
type FlickrMock struct {
    Server    *httptest.Server
    APIURL    string // base URL for API calls (points to Server)
    UploadURL string // base URL for uploads (points to Server)

    onGetPhotos HandlerFunc // flickr.people.getPhotos
    onGetSizes  HandlerFunc // flickr.photos.getSizes
    onUpload    HandlerFunc // POST /services/upload/
    onDownload  HandlerFunc // GET /download/<filename> (file content)

    mu       sync.Mutex
    requests []RecordedRequest
}

func NewFlickr(t *testing.T) *FlickrMock
func (m *FlickrMock) OnGetPhotos(h HandlerFunc) *FlickrMock
func (m *FlickrMock) OnGetSizes(h HandlerFunc) *FlickrMock
func (m *FlickrMock) OnUpload(h HandlerFunc) *FlickrMock
func (m *FlickrMock) OnDownload(h HandlerFunc) *FlickrMock
func (m *FlickrMock) Start() *FlickrMock
func (m *FlickrMock) Close()
func (m *FlickrMock) Requests() []RecordedRequest
```

**Routing logic** (single `httptest.Server`, dispatches internally):
- Query param `method=flickr.people.getPhotos` → `onGetPhotos`
- Query param `method=flickr.photos.getSizes` → `onGetSizes`
- Path prefix `/services/upload/` → `onUpload`
- Path prefix `/download/` → `onDownload` (getSizes responses return URLs pointing here)
- Unmatched → 404

Default handlers (if not configured) return 501 so unconfigured endpoints fail loudly.

**getSizes response shape**: The `OnGetSizes` handler must return a properly shaped Flickr `sizesResponse` JSON. The `Source` field must point back to the mock server's `/download/` path. Example:

```json
{
  "sizes": {
    "size": [
      {"label": "Original", "source": "http://127.0.0.1:PORT/download/photo123.jpg"}
    ]
  },
  "stat": "ok"
}
```

**Server cleanup**: `Start()` registers `t.Cleanup(m.Server.Close)` so the server is cleaned up even if the test panics before `defer mock.Close()` runs. The explicit `Close()` method is still available for tests that need deterministic shutdown ordering.

### google.go — Google Photos Mock Server

```go
type GoogleMock struct {
    Server  *httptest.Server
    BaseURL string // e.g., http://127.0.0.1:PORT

    onUploadBytes HandlerFunc // POST /v1/uploads
    onBatchCreate HandlerFunc // POST /v1/mediaItems:batchCreate

    mu       sync.Mutex
    requests []RecordedRequest
}

func NewGoogle(t *testing.T) *GoogleMock
func (m *GoogleMock) OnUploadBytes(h HandlerFunc) *GoogleMock
func (m *GoogleMock) OnBatchCreate(h HandlerFunc) *GoogleMock
func (m *GoogleMock) Start() *GoogleMock
func (m *GoogleMock) Close()
func (m *GoogleMock) Requests() []RecordedRequest
```

**Routing logic**:
- Path `/v1/uploads` → `onUploadBytes`
- Path `/v1/mediaItems:batchCreate` → `onBatchCreate`
- Unmatched → 404

## Integration Test Structure

**File**: `internal/cli/integration_test.go`
**Build tag**: `//go:build integration`

### Test Pattern

Every integration test follows this structure:

```go
func TestFlickrDownload_HappyPath(t *testing.T) {
    // 1. Setup temp dirs
    outputDir := t.TempDir()
    configDir := t.TempDir()

    // 2. Write test config files
    config.SaveFlickrConfig(configDir, &config.FlickrConfig{
        APIKey: "test-key", APISecret: "test-secret",
        OAuthToken: "test-token", OAuthTokenSecret: "test-secret",
    })

    // 3. Setup mock server with desired behavior
    mock := mockserver.NewFlickr(t).
        OnGetPhotos(mockserver.RespondJSON(200, photosPage)).
        OnGetSizes(func(w http.ResponseWriter, r *http.Request) {
            photoID := r.URL.Query().Get("photo_id")
            mockserver.RespondJSON(200, map[string]any{
                "sizes": map[string]any{
                    "size": []map[string]string{
                        {"label": "Original", "source": mock.Server.URL + "/download/" + photoID + ".jpg"},
                    },
                },
                "stat": "ok",
            })(w, r)
        }).
        OnDownload(mockserver.RespondBytes(200, testImageData)).
        Start()

    // 4. Set env vars (t.Setenv auto-restores after test)
    t.Setenv("PHOTO_COPY_CONFIG_DIR", configDir)
    t.Setenv("PHOTO_COPY_FLICKR_API_URL", mock.APIURL)
    t.Setenv("PHOTO_COPY_TEST_MODE", "1")

    // 5. Execute cobra command
    cmd := cli.NewRootCmd()
    cmd.SetArgs([]string{"flickr", "download", outputDir})
    err := cmd.Execute()

    // 6. Assert results
    require.NoError(t, err)
    // Verify files exist on disk
    // Verify transfer log contents
    // Verify mock received expected requests
}
```

**Google test pattern** — Google tests must write credential files AND set the OAuth bypass:

```go
func TestGoogleUpload_HappyPath(t *testing.T) {
    inputDir := t.TempDir()
    configDir := t.TempDir()

    // Write Google credentials (required for config.LoadGoogleConfig to succeed)
    config.SaveGoogleConfig(configDir, &config.GoogleConfig{
        ClientID: "test-id", ClientSecret: "test-secret",
    })

    // Create test media files in inputDir
    os.WriteFile(filepath.Join(inputDir, "photo1.jpg"), testImageData, 0644)

    mock := mockserver.NewGoogle(t).
        OnUploadBytes(func(w http.ResponseWriter, r *http.Request) {
            w.Write([]byte("upload-token-123")) // return upload token
        }).
        OnBatchCreate(mockserver.RespondJSON(200, map[string]any{})).
        Start()

    t.Setenv("PHOTO_COPY_CONFIG_DIR", configDir)
    t.Setenv("PHOTO_COPY_GOOGLE_API_URL", mock.BaseURL)
    t.Setenv("PHOTO_COPY_GOOGLE_TOKEN", "skip")
    t.Setenv("PHOTO_COPY_TEST_MODE", "1")

    cmd := cli.NewRootCmd()
    cmd.SetArgs([]string{"google", "upload", inputDir})
    err := cmd.Execute()
    require.NoError(t, err)

    // Verify upload log written inside inputDir
    // Verify mock received upload token in batchCreate request
}
```

### Test Scenarios (~16 tests)

#### Flickr Download (6 tests)

1. **TestFlickrDownload_HappyPath** — 3 photos on 1 page, all download successfully, transfer log updated, files on disk match expected content.
2. **TestFlickrDownload_Pagination** — Mock returns 2 pages (per_page=2, total=3). Verifies both pages are fetched and all 3 photos downloaded.
3. **TestFlickrDownload_ResumesFromLog** — Pre-populate transfer.log with 1 filename. Verify that file is skipped, others downloaded. getSizes should not be called for the skipped file.
4. **TestFlickrDownload_RetryOn429** — getSizes returns 429 on first call, 200 on second (via `RespondSequence`). Verify download succeeds.
5. **TestFlickrDownload_RetryOn5xx** — Download URL returns 500 first, then 200. File is saved correctly.
6. **TestFlickrDownload_LimitFlag** — 5 photos available, `--limit 2`. Verify only 2 files downloaded.

#### Flickr Upload (3 tests)

7. **TestFlickrUpload_HappyPath** — 2 media files in input dir, both uploaded. Mock verifies multipart POST received with correct file content.
8. **TestFlickrUpload_SkipsNonMedia** — Directory with .jpg and .txt files. Only .jpg is uploaded.
9. **TestFlickrUpload_LimitFlag** — 3 media files, `--limit 1`. Only 1 uploaded.
10. **TestFlickrUpload_FailsOnError** — Upload returns HTTP 500. Verify command returns error immediately (Flickr upload fails fast, unlike Google which continues).

#### Google Upload (5 tests)

11. **TestGoogleUpload_HappyPath** — Upload 2 files. Verify the two-step flow: uploadBytes → createMediaItem for each file. Upload log updated. Assert that the upload token returned by `OnUploadBytes` is passed through to the `OnBatchCreate` request body (verifies the token handoff).
12. **TestGoogleUpload_ResumesFromLog** — Pre-populate `.photo-copy-upload.log` inside `inputDir` with 1 filename. Verify that file is skipped, others uploaded.
13. **TestGoogleUpload_RetryOnUpload429** — uploadBytes returns 429 then 200 (via `RespondSequence`). File uploads successfully.
14. **TestGoogleUpload_PartialFailure** — 2 files. First file succeeds. Second file's createMediaItem returns 500 on all retries. Verify first file in upload log, second is not, command does not return error (continues past failures).
15. **TestGoogleUpload_LimitFlag** — 3 files, `--limit 2`. Only 2 uploaded.

#### Google Import Takeout (2 tests)

16. **TestGoogleImportTakeout_HappyPath** — Create test zip with media files, run `google import-takeout`, verify media extracted to output dir.
17. **TestGoogleImportTakeout_FiltersNonMedia** — Zip contains .jpg and .json metadata. Only .jpg extracted.

Note: Import Takeout tests don't need mock servers (purely filesystem-based) but are included here to get CLI-level coverage of that command.

Total: 17 tests.

## Running Integration Tests

Integration tests use a build tag so they don't run with the normal `go test ./...` command:

```bash
go test ./internal/cli/ -tags integration           # run all integration tests
go test ./internal/cli/ -tags integration -run TestFlickr  # run subset
go test ./...                                        # unit tests only (unchanged)
```

## Documentation Updates

### README.md

Add to the testing section:

```
## Integration Tests

Integration tests exercise CLI commands end-to-end against mock HTTP servers
for Flickr and Google Photos. They use a build tag and don't run with
`go test ./...`:

    go test ./internal/cli/ -tags integration

S3 integration testing is not included — S3 operations delegate to a rclone
subprocess, and rclone's own test coverage handles that layer. S3 unit tests
cover command arg building, config generation, and binary resolution.
```

### CLAUDE.md

Add the integration test command to the Build & Run section.

## Files Changed / Created

| File | Action | Description |
|------|--------|-------------|
| `internal/config/config.go` | Modified | `DefaultDir()` checks `PHOTO_COPY_CONFIG_DIR` env var |
| `internal/flickr/flickr.go` | Modified | URL helper funcs with env var override; throttle + retry delay test mode |
| `internal/google/google.go` | Modified | URL helpers; OAuth bypass; throttle + retry delay test mode |
| `internal/cli/root.go` | Modified | Add `ResetForTest()` to reset package-level `debug`/`limit` vars |
| `internal/testutil/mockserver/helpers.go` | New | Shared types (`RecordedRequest`) and handler factories |
| `internal/testutil/mockserver/flickr.go` | New | Configurable Flickr mock server with `t.Cleanup` auto-shutdown |
| `internal/testutil/mockserver/google.go` | New | Configurable Google Photos mock server with `t.Cleanup` auto-shutdown |
| `internal/cli/integration_test.go` | New | 17 integration tests with `//go:build integration` tag |
| `README.md` | Modified | Add integration test section |
| `CLAUDE.md` | Modified | Add integration test run command |
