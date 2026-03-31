package flickr

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/briandeitte/photo-copy/internal/config"
)

func TestOAuthSign_SetsRequiredParams(t *testing.T) {
	cfg := &config.FlickrConfig{
		APIKey:           "test-key",
		APISecret:        "test-secret",
		OAuthToken:       "test-token",
		OAuthTokenSecret: "test-token-secret",
	}

	params := map[string]string{
		"method": "flickr.test.echo",
	}

	sig := oauthSign("GET", "https://api.flickr.com/services/rest/", params, cfg)

	// Verify all required OAuth params are set
	required := []string{
		"oauth_consumer_key",
		"oauth_token",
		"oauth_signature_method",
		"oauth_timestamp",
		"oauth_nonce",
		"oauth_version",
		"oauth_signature",
	}
	for _, key := range required {
		if _, ok := params[key]; !ok {
			t.Errorf("missing required OAuth param: %s", key)
		}
	}

	if params["oauth_consumer_key"] != "test-key" {
		t.Errorf("oauth_consumer_key = %q, want %q", params["oauth_consumer_key"], "test-key")
	}
	if params["oauth_token"] != "test-token" {
		t.Errorf("oauth_token = %q, want %q", params["oauth_token"], "test-token")
	}
	if params["oauth_signature_method"] != "HMAC-SHA1" {
		t.Errorf("oauth_signature_method = %q, want %q", params["oauth_signature_method"], "HMAC-SHA1")
	}
	if params["oauth_version"] != "1.0" {
		t.Errorf("oauth_version = %q, want %q", params["oauth_version"], "1.0")
	}
	if sig == "" {
		t.Error("expected non-empty signature")
	}
	if sig != params["oauth_signature"] {
		t.Errorf("returned signature %q != params signature %q", sig, params["oauth_signature"])
	}
}

func TestOAuthSign_DeterministicWithSameInputs(t *testing.T) {
	cfg := &config.FlickrConfig{
		APIKey:           "key",
		APISecret:        "secret",
		OAuthToken:       "token",
		OAuthTokenSecret: "token-secret",
	}

	params1 := map[string]string{"method": "test"}
	params2 := map[string]string{"method": "test"}

	sig1 := oauthSign("GET", "https://example.com/", params1, cfg)
	sig2 := oauthSign("GET", "https://example.com/", params2, cfg)

	if sig1 == "" || sig2 == "" {
		t.Error("signatures should not be empty")
	}

	if params1["oauth_nonce"] == params2["oauth_nonce"] {
		t.Error("nonces should differ between calls")
	}
}

func TestOAuthSign_PreservesExistingParams(t *testing.T) {
	cfg := &config.FlickrConfig{
		APIKey:           "key",
		APISecret:        "secret",
		OAuthToken:       "token",
		OAuthTokenSecret: "token-secret",
	}

	params := map[string]string{
		"method":         "flickr.photos.getInfo",
		"photo_id":       "12345",
		"format":         "json",
		"nojsoncallback": "1",
	}

	oauthSign("GET", "https://api.flickr.com/services/rest/", params, cfg)

	if params["method"] != "flickr.photos.getInfo" {
		t.Error("method param was overwritten")
	}
	if params["photo_id"] != "12345" {
		t.Error("photo_id param was overwritten")
	}
}

// TestOAuthSign_SpecialCharactersInParams is a smoke test verifying that
// oauthSign does not panic or corrupt params when values contain characters
// that require URL encoding (+, =, &, spaces). It does not verify the
// signature value itself because nonce/timestamp make signatures non-deterministic.
func TestOAuthSign_SpecialCharactersInParams(t *testing.T) {
	cfg := &config.FlickrConfig{
		APIKey:           "test-key",
		APISecret:        "test-secret",
		OAuthToken:       "test-token",
		OAuthTokenSecret: "test-token-secret",
	}

	params := map[string]string{
		"method": "flickr.test.echo",
		"text":   "hello world+foo=bar&baz",
	}

	oauthSign("GET", "https://api.flickr.com/services/rest/", params, cfg)

	// The text param should be preserved as-is in the params map (encoding
	// happens only during base string construction).
	if params["text"] != "hello world+foo=bar&baz" {
		t.Errorf("text param was altered: got %q", params["text"])
	}

	// Signature must have been set
	if params["oauth_signature"] == "" {
		t.Fatal("oauth_signature not set")
	}
}

func TestOAuthSign_EmptyParams(t *testing.T) {
	cfg := &config.FlickrConfig{
		APIKey:           "test-key",
		APISecret:        "test-secret",
		OAuthToken:       "test-token",
		OAuthTokenSecret: "test-token-secret",
	}

	params := map[string]string{}

	sig := oauthSign("GET", "https://api.flickr.com/services/rest/", params, cfg)

	if sig == "" {
		t.Fatal("expected non-empty signature")
	}

	// All required OAuth params should still be set even with no extra params
	required := []string{
		"oauth_consumer_key",
		"oauth_token",
		"oauth_signature_method",
		"oauth_timestamp",
		"oauth_nonce",
		"oauth_version",
		"oauth_signature",
	}
	for _, key := range required {
		if _, ok := params[key]; !ok {
			t.Errorf("missing required OAuth param: %s", key)
		}
	}

	if params["oauth_signature"] != sig {
		t.Errorf("returned signature %q != params signature %q", sig, params["oauth_signature"])
	}
}

func TestGetRequestToken_AuthURLIncludesPermsWrite(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-www-form-urlencoded")
		_, _ = w.Write([]byte("oauth_token=test-req-token&oauth_token_secret=test-req-secret&oauth_callback_confirmed=true"))
	}))
	defer server.Close()

	t.Setenv("PHOTO_COPY_FLICKR_OAUTH_URL", server.URL)

	cfg := &config.FlickrConfig{
		APIKey:    "test-key",
		APISecret: "test-secret",
	}

	token, tokenSecret, authURL, err := GetRequestToken(cfg)
	if err != nil {
		t.Fatalf("GetRequestToken failed: %v", err)
	}

	if token != "test-req-token" {
		t.Errorf("token = %q, want %q", token, "test-req-token")
	}
	if tokenSecret != "test-req-secret" {
		t.Errorf("tokenSecret = %q, want %q", tokenSecret, "test-req-secret")
	}
	if !strings.Contains(authURL, "perms=write") {
		t.Errorf("authURL missing perms=write: %s", authURL)
	}
	if !strings.Contains(authURL, "oauth_token=test-req-token") {
		t.Errorf("authURL missing oauth_token: %s", authURL)
	}
}

func TestGenerateNonce(t *testing.T) {
	nonce := generateNonce()
	if len(nonce) != 32 {
		t.Errorf("nonce length = %d, want 32", len(nonce))
	}

	for _, c := range nonce {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			t.Errorf("nonce contains invalid character: %c", c)
		}
	}

	nonce2 := generateNonce()
	if nonce == nonce2 {
		t.Error("two nonces should not be identical")
	}
}
