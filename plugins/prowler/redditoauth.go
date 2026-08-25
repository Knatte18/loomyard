// redditoauth.go implements the authenticated Reddit OAuth API client: credential resolution,
// app-only token acquisition with a concurrency-safe in-process cache, thread retrieval from
// oauth.reddit.com, and JSON-to-markdown formatting. It is the module's tier-1 Reddit strategy and
// the only file that reads the PROWLER_REDDIT_* environment variables.

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// redditClientIDEnv names the environment variable holding the Reddit
	// app's OAuth client id.
	redditClientIDEnv = "PROWLER_REDDIT_CLIENT_ID"
	// redditClientSecretEnv names the environment variable holding the
	// Reddit app's OAuth client secret.
	redditClientSecretEnv = "PROWLER_REDDIT_CLIENT_SECRET"
	// redditUserAgentEnv names the environment variable that overrides the
	// default Reddit API User-Agent, when set.
	redditUserAgentEnv = "PROWLER_REDDIT_USER_AGENT"
	// defaultRedditAPIUserAgent is sent to Reddit's API when
	// redditUserAgentEnv is unset, per Reddit's descriptive-User-Agent
	// requirement.
	defaultRedditAPIUserAgent = "prowler/1.0"
	// redditTokenURL is Reddit's OAuth token endpoint for the
	// client_credentials grant.
	redditTokenURL = "https://www.reddit.com/api/v1/access_token"
	// redditTokenSafetyMargin is subtracted from a token's reported lifetime
	// before caching its expiry, so a cached token is never handed out
	// within this margin of actually expiring.
	redditTokenSafetyMargin = 60 * time.Second
)

// timeNow is the sole clock read in this file, indirected so tests can
// simulate token expiry without sleeping.
var timeNow = time.Now

// redditCredentials reads the two Reddit OAuth credential environment
// variables. missing names every one of them (sorted) whose value is empty
// after trimming whitespace; callers treat len(missing) == 0 as "tier 1 is
// available". This is the only function in the module that calls
// os.Getenv for the PROWLER_REDDIT_* variables.
func redditCredentials() (clientID, clientSecret string, missing []string) {
	clientID = strings.TrimSpace(os.Getenv(redditClientIDEnv))
	clientSecret = strings.TrimSpace(os.Getenv(redditClientSecretEnv))

	if clientID == "" {
		missing = append(missing, redditClientIDEnv)
	}
	if clientSecret == "" {
		missing = append(missing, redditClientSecretEnv)
	}
	sort.Strings(missing)

	return clientID, clientSecret, missing
}

// redditAPIUserAgent returns the User-Agent sent on Reddit API requests:
// redditUserAgentEnv's value when set and non-empty after trimming, or
// defaultRedditAPIUserAgent otherwise. It never returns browserUA -- Reddit's
// API rules require a descriptive User-Agent and penalize
// generic/impersonating ones with aggressive rate limiting.
func redditAPIUserAgent() string {
	if ua := strings.TrimSpace(os.Getenv(redditUserAgentEnv)); ua != "" {
		return ua
	}
	return defaultRedditAPIUserAgent
}

// redditTokenCache holds one process-wide cached Reddit OAuth token. The
// zero value is empty and unauthenticated; use redditTokens rather than
// constructing one directly. redditTokenCache is safe for concurrent use --
// runAll fetches every URL in its own goroutine, so token acquisition must
// be too.
type redditTokenCache struct {
	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

// redditTokens is the single process-wide token cache shared by every
// Reddit OAuth fetch in this process.
var redditTokens = &redditTokenCache{}

// reset clears the cached token and its expiry. It exists for tests, so
// each test starts from an unauthenticated cache regardless of what an
// earlier test left behind.
func (c *redditTokenCache) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = ""
	c.expiresAt = time.Time{}
}

// get returns a valid Reddit OAuth token, reusing the cached one when it is
// still fresh and otherwise acquiring a new one via requestRedditToken. get
// holds the cache's lock for its entire body -- including the token HTTP
// request when a refresh is needed -- so that under runAll's concurrent
// per-URL goroutines exactly one token request is issued per process; the
// cost is that concurrent Reddit fetches serialise on the first token
// acquisition only.
func (c *redditTokenCache) get(ctx context.Context, f fetcher, clientID, clientSecret string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && timeNow().Before(c.expiresAt) {
		return c.token, nil
	}

	token, lifetime, err := requestRedditToken(ctx, f, clientID, clientSecret)
	if err != nil {
		return "", err
	}

	c.token = token
	c.expiresAt = timeNow().Add(lifetime - redditTokenSafetyMargin)
	return c.token, nil
}

// requestRedditToken performs the client_credentials grant against
// redditTokenURL and returns the issued access token and its reported
// lifetime. A non-2xx status, a transport error, a JSON decode failure, or
// a 2xx response with an empty access_token each yield a non-nil error; no
// error this function returns ever contains clientID, clientSecret, or the
// token itself -- an authentication failure names redditClientIDEnv and
// redditClientSecretEnv instead, so a reader knows what to check without a
// secret leaking into logs or output.
func requestRedditToken(ctx context.Context, f fetcher, clientID, clientSecret string) (token string, lifetime time.Duration, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, redditTokenURL, strings.NewReader("grant_type=client_credentials"))
	if err != nil {
		return "", 0, fmt.Errorf("build reddit token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", redditAPIUserAgent())
	basicAuth := base64.StdEncoding.EncodeToString([]byte(clientID + ":" + clientSecret))
	req.Header.Set("Authorization", "Basic "+basicAuth)

	resp, err := f.do(req)
	if err != nil {
		return "", 0, fmt.Errorf("reddit token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", 0, fmt.Errorf("reddit token request returned status %d; check %s and %s", resp.StatusCode, redditClientIDEnv, redditClientSecretEnv)
	}

	var payload struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", 0, fmt.Errorf("decode reddit token response: %w", err)
	}
	if payload.AccessToken == "" {
		return "", 0, fmt.Errorf("reddit token response had no access_token; check %s and %s", redditClientIDEnv, redditClientSecretEnv)
	}

	return payload.AccessToken, time.Duration(payload.ExpiresIn) * time.Second, nil
}
