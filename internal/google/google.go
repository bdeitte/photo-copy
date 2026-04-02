package google

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/briandeitte/photo-copy/internal/config"
	"github.com/briandeitte/photo-copy/internal/logging"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// errTokenExpired is returned when the OAuth token is expired or revoked.
var errTokenExpired = fmt.Errorf("Google OAuth token has been expired or revoked. Run 'photo-copy config google' to re-authenticate") //nolint:staticcheck // proper noun

const (
	maxRetries        = 5
	baseRetryDelay    = 2 * time.Second
	minUploadInterval = 2 * time.Second // Throttle uploads to avoid rate limiting
)

func isTestMode() bool {
	return os.Getenv("PHOTO_COPY_TEST_MODE") != ""
}

var oauthScopes = []string{
	"https://www.googleapis.com/auth/photoslibrary.appendonly",
}

// Client wraps an authenticated HTTP client for Google Photos API.
type Client struct {
	httpClient  *http.Client
	log         *logging.Logger
	configDir   string
	lastRequest time.Time
}

// NewClient creates a new Google Photos client with OAuth2 authentication.
func NewClient(ctx context.Context, cfg *config.GoogleConfig, configDir string, log *logging.Logger) (*Client, error) {
	if os.Getenv("PHOTO_COPY_GOOGLE_TOKEN") == "skip" {
		return &Client{
			httpClient: &http.Client{},
			log:        log,
			configDir:  configDir,
		}, nil
	}

	oauthCfg := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Scopes:       oauthScopes,
		Endpoint:     google.Endpoint,
		RedirectURL:  "http://localhost", // placeholder, updated with actual port in runOAuthFlow
	}

	token, err := loadToken(configDir)
	if err != nil {
		log.Debug("no saved token found, starting OAuth flow")
		token, err = runOAuthFlow(ctx, oauthCfg)
		if err != nil {
			return nil, fmt.Errorf("OAuth flow failed: %w", err)
		}
		if err := saveToken(configDir, token); err != nil {
			log.Error("failed to save token: %v", err)
		}
	}

	client := oauthCfg.Client(ctx, token)

	return &Client{
		httpClient: client,
		log:        log,
		configDir:  configDir,
	}, nil
}

// throttle ensures we don't exceed Google Photos API rate limits.
func (c *Client) throttle() {
	if isTestMode() {
		return
	}
	if !c.lastRequest.IsZero() {
		elapsed := time.Since(c.lastRequest)
		if elapsed < minUploadInterval {
			time.Sleep(minUploadInterval - elapsed)
		}
	}
	c.lastRequest = time.Now()
}

// retryDelay calculates the backoff delay, honoring the Retry-After header if present.
func (c *Client) retryDelay(attempt int, resp *http.Response) time.Duration {
	if isTestMode() {
		return time.Millisecond
	}
	if resp != nil {
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if seconds, err := strconv.Atoi(ra); err == nil {
				return time.Duration(seconds) * time.Second
			}
		}
	}
	return baseRetryDelay * (1 << uint(attempt))
}

// retryableDo performs an HTTP request with throttling and retry on 429/5xx errors.
func (c *Client) retryableDo(ctx context.Context, buildReq func() (*http.Request, error)) (*http.Response, error) {
	for attempt := 0; attempt <= maxRetries; attempt++ {
		c.throttle()

		req, err := buildReq()
		if err != nil {
			return nil, err
		}
		req = req.WithContext(ctx)

		c.log.Debug("HTTP %s %s", req.Method, req.URL.String())
		for key, vals := range req.Header {
			if strings.EqualFold(key, "Authorization") {
				c.log.Debug("  %s: [redacted]", key)
				continue
			}
			c.log.Debug("  %s: %s", key, strings.Join(vals, ", "))
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			var retrieveErr *oauth2.RetrieveError
			if errors.As(err, &retrieveErr) && retrieveErr.ErrorCode == "invalid_grant" {
				c.log.Debug("OAuth error: %v", err)
				return nil, errTokenExpired
			}
			if attempt == maxRetries {
				return nil, err
			}
			delay := c.retryDelay(attempt, nil)
			c.log.Info("network error, retrying in %v (attempt %d/%d): %v", delay, attempt+1, maxRetries, err)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
			continue
		}

		c.log.Debug("HTTP response: %d %s", resp.StatusCode, resp.Status)

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			_ = resp.Body.Close()
			if attempt == maxRetries {
				return nil, fmt.Errorf("HTTP %d after %d retries", resp.StatusCode, maxRetries)
			}
			delay := c.retryDelay(attempt, resp)
			c.log.Info("HTTP %d, retrying in %v (attempt %d/%d)", resp.StatusCode, delay, attempt+1, maxRetries)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
			continue
		}

		return resp, nil
	}
	return nil, fmt.Errorf("unreachable")
}

// loadToken loads a saved OAuth2 token from the config directory.
func loadToken(configDir string) (*oauth2.Token, error) {
	tokenData, err := config.LoadGoogleToken(configDir)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(tokenData)
	if err != nil {
		return nil, fmt.Errorf("marshaling token data: %w", err)
	}

	var token oauth2.Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("unmarshaling token: %w", err)
	}

	return &token, nil
}

// saveToken saves an OAuth2 token to the config directory.
func saveToken(configDir string, token *oauth2.Token) error {
	return config.SaveGoogleToken(configDir, token)
}

// runOAuthFlow runs an OAuth2 flow using a localhost redirect.
// It starts a temporary HTTP server to receive the authorization code from Google.
func runOAuthFlow(ctx context.Context, cfg *oauth2.Config) (*oauth2.Token, error) {
	// Copy the config to avoid mutating the caller's object
	localCfg := *cfg

	// Listen on a random available port
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, fmt.Errorf("starting local server: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	localCfg.RedirectURL = fmt.Sprintf("http://localhost:%d", port)

	// Generate a random state parameter for CSRF protection
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("generating state token: %w", err)
	}
	state := base64.URLEncoding.EncodeToString(stateBytes)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			_, _ = fmt.Fprint(w, "<html><body><h2>Authorization failed: invalid state parameter</h2><p>You can close this window.</p></body></html>")
			errCh <- fmt.Errorf("authorization failed: state mismatch (possible CSRF)")
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			errMsg := r.URL.Query().Get("error")
			if errMsg == "" {
				errMsg = "no authorization code received"
			}
			_, _ = fmt.Fprintf(w, "<html><body><h2>Authorization failed: %s</h2><p>You can close this window.</p></body></html>", html.EscapeString(errMsg))
			errCh <- fmt.Errorf("authorization failed: %s", errMsg)
			return
		}
		_, _ = fmt.Fprint(w, "<html><body><h2>Authorization successful!</h2><p>You can close this window and return to the terminal.</p></body></html>")
		codeCh <- code
	})

	server := &http.Server{Handler: mux}

	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- fmt.Errorf("local server error: %w", serveErr)
		}
	}()
	defer func() { _ = server.Close() }()

	authURL := localCfg.AuthCodeURL(state, oauth2.AccessTypeOffline)
	fmt.Println("Opening browser for Google authorization...")
	fmt.Println("Note: Google will show an 'unverified app' warning because this is your")
	fmt.Println("own personal OAuth app. Click 'Advanced' then 'Go to photo-copy (unsafe)'")
	fmt.Println("to proceed — this is expected and safe.")
	fmt.Println()
	fmt.Println("If the browser doesn't open, visit this URL:")
	fmt.Println(authURL)
	fmt.Println()

	// Try to open the browser automatically
	openBrowser(authURL)

	fmt.Println("Waiting for authorization...")

	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	token, err := localCfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("exchanging code for token: %w", err)
	}

	return token, nil
}

// openBrowser tries to open a URL in the default browser.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	_ = cmd.Start()
}
