package flickr

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/briandeitte/photo-copy/internal/config"
)

func oauthSign(method, endpoint string, params map[string]string, cfg *config.FlickrConfig) string {
	params["oauth_consumer_key"] = cfg.APIKey
	params["oauth_token"] = cfg.OAuthToken
	params["oauth_signature_method"] = "HMAC-SHA1"
	params["oauth_timestamp"] = fmt.Sprintf("%d", time.Now().Unix())
	params["oauth_nonce"] = generateNonce()
	params["oauth_version"] = "1.0"

	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, url.QueryEscape(k)+"="+url.QueryEscape(params[k]))
	}
	paramString := strings.Join(pairs, "&")

	baseString := method + "&" + url.QueryEscape(endpoint) + "&" + url.QueryEscape(paramString)

	signingKey := url.QueryEscape(cfg.APISecret) + "&" + url.QueryEscape(cfg.OAuthTokenSecret)
	mac := hmac.New(sha1.New, []byte(signingKey))
	mac.Write([]byte(baseString))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	params["oauth_signature"] = signature
	return signature
}

func generateNonce() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// GetRequestToken initiates the OAuth 1.0a flow by obtaining a request token.
func GetRequestToken(cfg *config.FlickrConfig) (string, string, string, error) {
	params := map[string]string{
		"oauth_callback": "oob",
	}

	tempCfg := &config.FlickrConfig{
		APIKey:    cfg.APIKey,
		APISecret: cfg.APISecret,
	}

	oauthSign("GET", "https://www.flickr.com/services/oauth/request_token", params, tempCfg)

	v := url.Values{}
	for k, val := range params {
		v.Set(k, val)
	}

	resp, err := http.Get("https://www.flickr.com/services/oauth/request_token?" + v.Encode())
	if err != nil {
		return "", "", "", err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", "", fmt.Errorf("reading token response: %w", err)
	}
	vals, err := url.ParseQuery(string(body))
	if err != nil {
		return "", "", "", fmt.Errorf("parsing token response: %w", err)
	}

	token := vals.Get("oauth_token")
	tokenSecret := vals.Get("oauth_token_secret")
	authURL := "https://www.flickr.com/services/oauth/authorize?oauth_token=" + token

	return token, tokenSecret, authURL, nil
}

// ExchangeToken exchanges a request token and verifier for an access token.
func ExchangeToken(cfg *config.FlickrConfig, requestToken, requestTokenSecret, verifier string) (string, string, error) {
	tempCfg := &config.FlickrConfig{
		APIKey:           cfg.APIKey,
		APISecret:        cfg.APISecret,
		OAuthToken:       requestToken,
		OAuthTokenSecret: requestTokenSecret,
	}

	params := map[string]string{
		"oauth_verifier": verifier,
	}

	oauthSign("GET", "https://www.flickr.com/services/oauth/access_token", params, tempCfg)

	v := url.Values{}
	for k, val := range params {
		v.Set(k, val)
	}

	resp, err := http.Get("https://www.flickr.com/services/oauth/access_token?" + v.Encode())
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("reading token response: %w", err)
	}
	vals, err := url.ParseQuery(string(body))
	if err != nil {
		return "", "", fmt.Errorf("parsing token response: %w", err)
	}

	return vals.Get("oauth_token"), vals.Get("oauth_token_secret"), nil
}
