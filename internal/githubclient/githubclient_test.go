// githubclient_test.go is the hermetic, table-driven test suite for the whole package: the token resolution chain (token.go), the on-disk cache (cache.go/cache_windows.go), and the authenticating RoundTripper (transport.go).
// Every case redirects the cache to a t.TempDir() via t.Setenv and reaches the `gh auth token` shell-out only through its injected seam, so the suite runs correctly on a machine with no `gh` installed and no GitHub credentials, and never touches the operator's real credential file.
// This file carries no build tag -- it is untagged Tier 1, spawning no process and needing no git fixture, so `go test -race -count=1 ./internal/githubclient/...` runs every case here on any platform.
// The one genuinely Windows-only piece -- TestWriteCachedToken_CreatesFileWithRestrictivePermissions and its assertOwnerOnlyDACL helper, which import golang.org/x/sys/windows to assert the cache file's security descriptor directly -- lives in githubclient_windows_test.go behind `//go:build windows`, mirroring this package's cache.go/cache_windows.go/cache_other.go split.
// No test here calls t.Parallel: each redirects the cache through t.Setenv, which edits the process-global environment.
// Both files share the helpers declared below (setCacheDir, seedCacheFile, withFakeGHAuthToken, capturedRequest, newScriptedServer).

package githubclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/pauserecord"
)

// setCacheDir redirects the token cache to dir by setting every environment
// variable cacheDir consults, on either branch of its runtime.GOOS check:
// LOCALAPPDATA on Windows, XDG_CONFIG_HOME everywhere else. Setting both
// unconditionally (rather than branching on runtime.GOOS here too) means
// this helper redirects cacheDir correctly regardless of which platform the
// suite is actually running on, since cacheDir itself ignores whichever
// variable its own branch does not consult. Every test in this file calls
// this (directly or via seedCacheFile) before touching anything else, so the
// operator's real credential file is never read or written.
func setCacheDir(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("LOCALAPPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
}

// seedCacheFile writes a cache file with the given token and resolved-at
// time directly, bypassing writeCachedToken so a test can construct a
// specific age (e.g. past the freshness TTL) that writeCachedToken itself,
// which always stamps "now", cannot produce.
func seedCacheFile(t *testing.T, token string, resolvedAt time.Time) {
	t.Helper()

	dir, ok := cacheDir()
	if !ok {
		t.Fatal("cacheDir() ok = false; call setCacheDir before seedCacheFile")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll(%s): %v", dir, err)
	}

	creds := cachedCredentials{Token: token, ResolvedAt: resolvedAt.UTC().Format(time.RFC3339)}
	data, err := json.Marshal(&creds)
	if err != nil {
		t.Fatalf("marshal seed cache: %v", err)
	}

	path := filepath.Join(dir, credentialFileName)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

// withFakeGHAuthToken replaces the package-level runGHAuthToken seam with fn
// for the duration of the test, restoring the original afterward. This is
// the seam every case in this file uses to reach the `gh auth token`
// shell-out -- no test here ever spawns a real gh process.
func withFakeGHAuthToken(t *testing.T, fn func(ctx context.Context) (string, error)) {
	t.Helper()
	orig := runGHAuthToken
	runGHAuthToken = fn
	t.Cleanup(func() { runGHAuthToken = orig })
}

// capturedRequest records the parts of an incoming request a transport test
// cares about: the Authorization header it was sent with and its body.
type capturedRequest struct {
	authHeader string
	body       []byte
}

// newScriptedServer returns an httptest server that replies with the status
// codes in statuses, one per request in order (repeating the last entry for
// any request beyond len(statuses)), and appends a capturedRequest for every
// request it receives to captured.
func newScriptedServer(t *testing.T, statuses []int, captured *[]capturedRequest) *httptest.Server {
	t.Helper()

	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		mu.Lock()
		idx := len(*captured)
		*captured = append(*captured, capturedRequest{authHeader: r.Header.Get("Authorization"), body: body})
		mu.Unlock()

		status := statuses[len(statuses)-1]
		if idx < len(statuses) {
			status = statuses[idx]
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(server.Close)
	return server
}

// TestResolveToken_Chain covers the full resolution order end to end: env vars always win over the cache, GH_TOKEN before GITHUB_TOKEN, a fresh cache entry is used when both env vars are empty, a stale one is not, and an unresolvable token surfaces as ErrTokenUnresolvable alongside an empty token, without blocking or prompting.
func TestResolveToken_Chain(t *testing.T) {
	tests := []struct {
		name           string
		ghTokenEnv     string
		githubTokenEnv string
		seedToken      string // empty means no cache file is seeded
		cacheAge       time.Duration
		ghCLIToken     string
		ghCLIErr       error
		wantToken      string
		wantSource     tokenSource
		wantErr        bool
	}{
		{
			name:           "GH_TOKEN wins over GITHUB_TOKEN and a fresh cache",
			ghTokenEnv:     "gh-token-value",
			githubTokenEnv: "github-token-value",
			seedToken:      "cached-token",
			cacheAge:       time.Minute,
			wantToken:      "gh-token-value",
			wantSource:     sourceGHTokenEnv,
		},
		{
			name:           "GITHUB_TOKEN used when GH_TOKEN is empty",
			githubTokenEnv: "github-token-value",
			seedToken:      "cached-token",
			cacheAge:       time.Minute,
			wantToken:      "github-token-value",
			wantSource:     sourceGitHubTokenEnv,
		},
		{
			name:       "fresh cache used when both env vars are empty",
			seedToken:  "cached-token",
			cacheAge:   time.Minute,
			wantToken:  "cached-token",
			wantSource: sourceCache,
		},
		{
			name:       "cache past its TTL falls through to gh CLI",
			seedToken:  "stale-cached-token",
			cacheAge:   13 * time.Hour,
			ghCLIToken: "gh-cli-token",
			wantToken:  "gh-cli-token",
			wantSource: sourceGHCLI,
		},
		{
			name:       "no cache falls through to gh CLI",
			ghCLIToken: "gh-cli-token",
			wantToken:  "gh-cli-token",
			wantSource: sourceGHCLI,
		},
		{
			name:     "gh CLI failure is unresolvable",
			ghCLIErr: errors.New("gh: not logged in to any GitHub hosts"),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setCacheDir(t, t.TempDir())
			t.Setenv("GH_TOKEN", tt.ghTokenEnv)
			t.Setenv("GITHUB_TOKEN", tt.githubTokenEnv)

			if tt.seedToken != "" {
				seedCacheFile(t, tt.seedToken, time.Now().Add(-tt.cacheAge))
			}

			withFakeGHAuthToken(t, func(ctx context.Context) (string, error) {
				return tt.ghCLIToken, tt.ghCLIErr
			})

			start := time.Now()
			gotToken, gotSource, err := resolveToken()
			elapsed := time.Since(start)

			if elapsed > time.Second {
				t.Errorf("resolveToken() took %v; want a fast result, never a wait or prompt", elapsed)
			}
			if tt.wantErr {
				if !errors.Is(err, ErrTokenUnresolvable) {
					t.Fatalf("resolveToken() error = %v; want ErrTokenUnresolvable", err)
				}
				if gotToken != "" || gotSource != sourceUnknown {
					t.Errorf("resolveToken() = (%q, %v); want (\"\", sourceUnknown) alongside the error", gotToken, gotSource)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveToken() error = %v; want nil", err)
			}
			if gotToken != tt.wantToken {
				t.Errorf("resolveToken() token = %q; want %q", gotToken, tt.wantToken)
			}
			if gotSource != tt.wantSource {
				t.Errorf("resolveToken() source = %v; want %v", gotSource, tt.wantSource)
			}
		})
	}
}

// TestCacheDirRedirection_HonoursOverride asserts the redirection itself works, rather than assuming it: pointing the environment at an empty temp dir must resolve cacheDir under that temp dir and report a cache miss there, never silently fall back to a real user path.
//
//testtiming:keep pins that the cache redirection resolves under the temp dir rather than a real user path, which no other test asserts directly
func TestCacheDirRedirection_HonoursOverride(t *testing.T) {
	dir := t.TempDir()
	setCacheDir(t, dir)

	gotDir, ok := cacheDir()
	if !ok {
		t.Fatal("cacheDir() ok = false; want true once LOCALAPPDATA is set")
	}
	if !isUnder(dir, gotDir) {
		t.Fatalf("cacheDir() = %q; want a path under the redirected temp dir %q -- resolution must never silently fall back to a real user path", gotDir, dir)
	}

	if tok, ok := readCachedToken(); ok {
		t.Errorf("readCachedToken() = (%q, true) for an empty redirected cache dir; want a miss", tok)
	}
}

// isUnder reports whether candidate is base itself or a path beneath it.
func isUnder(base, candidate string) bool {
	rel, err := filepath.Rel(base, candidate)
	if err != nil {
		return false
	}
	return rel == "." || !strings.HasPrefix(rel, "..")
}

// TestReadCachedToken_MalformedFileIsMiss covers every way the on-disk file can fail to match the
// exact two-field schema: each is a cache miss, never a fatal error, so a foreign or worn-out file
// never blocks resolution.
func TestReadCachedToken_MalformedFileIsMiss(t *testing.T) {
	freshTimestamp := time.Now().UTC().Format(time.RFC3339)

	tests := []struct {
		name    string
		content string
	}{
		{"not JSON at all", "definitely not json"},
		{"empty file", ""},
		{"missing resolved_at", `{"token":"abc"}`},
		{"missing token", `{"resolved_at":"` + freshTimestamp + `"}`},
		{"extra unexpected field", `{"token":"abc","resolved_at":"` + freshTimestamp + `","extra":"nope"}`},
		{"unparseable timestamp", `{"token":"abc","resolved_at":"not-a-timestamp"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			setCacheDir(t, dir)

			resolvedDir, ok := cacheDir()
			if !ok {
				t.Fatal("cacheDir() ok = false")
			}
			if err := os.MkdirAll(resolvedDir, 0o700); err != nil {
				t.Fatalf("MkdirAll(%s): %v", resolvedDir, err)
			}
			path := filepath.Join(resolvedDir, credentialFileName)
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatalf("WriteFile(%s): %v", path, err)
			}

			if tok, ok := readCachedToken(); ok {
				t.Errorf("readCachedToken() = (%q, true); want a miss for content %q", tok, tt.content)
			}
		})
	}
}

// TestWriteCachedToken_UnwritableDirDegradesToInProcessResolution covers the requirement that a
// cache directory which cannot be created degrades to in-process resolution rather than failing the
// command: writeCachedToken must not panic or error out loud,
// and a subsequent resolveToken must still succeed via the (faked) gh CLI fallback.
func TestWriteCachedToken_UnwritableDirDegradesToInProcessResolution(t *testing.T) {
	root := t.TempDir()
	blockingFile := filepath.Join(root, "blocks-mkdir")
	if err := os.WriteFile(blockingFile, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("seed blocking file: %v", err)
	}

	// LOCALAPPDATA now points at a regular file, so cacheDir's "lyx"
	// subdirectory can never be created underneath it.
	setCacheDir(t, blockingFile)
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")

	writeCachedToken("some-token")

	withFakeGHAuthToken(t, func(ctx context.Context) (string, error) {
		return "in-process-token", nil
	})

	tok, source, err := resolveToken()
	if err != nil {
		t.Fatalf("resolveToken() error = %v; want a degrade to in-process resolution, not a failure", err)
	}
	if tok != "in-process-token" || source != sourceGHCLI {
		t.Errorf("resolveToken() = (%q, %v); want (%q, sourceGHCLI)", tok, source, "in-process-token")
	}
}

// TestCache_ConcurrentWriters drives many goroutines through writeCachedToken and readCachedToken at once.
// The requirement is not that any particular write wins -- it is that the file is always either absent or fully parseable, never left half-written by a torn concurrent write.
//
//testtiming:keep pins that concurrent writers never leave a torn cache file, which the sequential cache tests do not exercise
func TestCache_ConcurrentWriters(t *testing.T) {
	dir := t.TempDir()
	setCacheDir(t, dir)

	const writers = 25
	var wg sync.WaitGroup
	wg.Add(writers)
	for i := 0; i < writers; i++ {
		i := i
		go func() {
			defer wg.Done()
			writeCachedToken(fmt.Sprintf("token-%d", i))
			readCachedToken()
		}()
	}
	wg.Wait()

	resolvedDir, ok := cacheDir()
	if !ok {
		t.Fatal("cacheDir() ok = false")
	}
	path := filepath.Join(resolvedDir, credentialFileName)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Absent is an acceptable final state -- every writer races
			// every other one, and the guarantee is only that none of
			// them leaves a half-written file behind.
			return
		}
		t.Fatalf("ReadFile(%s): %v", path, err)
	}

	var creds cachedCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		t.Fatalf("final cache file is not valid JSON (a torn write): %v\ncontents: %q", err, data)
	}
	if creds.Token == "" || creds.ResolvedAt == "" {
		t.Fatalf("final cache file is missing a field (a torn write): %+v", creds)
	}
}

// TestAuthRT_401Handling covers the transport's 401 contract: a 401 on a cache-sourced token invalidates it, resolves a fresh one exactly once and replays with it -- never in a loop -- with the request body rewound byte-identical (the req.GetBody rewind a drained replay would silently skip); a second consecutive 401 goes back to the caller unchanged; and an environment-sourced token is never replayed, since replaying it reproduces the identical value, so the 401 surfaces as an error naming the rejected variable.
func TestAuthRT_401Handling(t *testing.T) {
	tests := []struct {
		name       string
		ghTokenEnv string
		statuses   []int
		body       []byte
		wantStatus int
		wantErr    string
		wantAuth   []string
		wantCached string
	}{
		{
			name:       "401 then success replays once with a fresh token and an identical body",
			statuses:   []int{http.StatusUnauthorized, http.StatusCreated},
			body:       []byte(`{"title":"a test issue"}`),
			wantStatus: http.StatusCreated,
			wantAuth:   []string{"Bearer cached-token", "Bearer fresh-token"},
			wantCached: "fresh-token",
		},
		{
			name:       "second consecutive 401 propagates without a further retry",
			statuses:   []int{http.StatusUnauthorized, http.StatusUnauthorized},
			wantStatus: http.StatusUnauthorized,
			wantAuth:   []string{"Bearer cached-token", "Bearer fresh-token"},
		},
		{
			name:       "env-sourced token is never replayed",
			ghTokenEnv: "env-token",
			statuses:   []int{http.StatusUnauthorized},
			wantErr:    "GH_TOKEN",
			wantAuth:   []string{"Bearer env-token"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setCacheDir(t, t.TempDir())
			t.Setenv("GH_TOKEN", tt.ghTokenEnv)
			t.Setenv("GITHUB_TOKEN", "")
			seedCacheFile(t, "cached-token", time.Now())

			withFakeGHAuthToken(t, func(ctx context.Context) (string, error) {
				return "fresh-token", nil
			})

			var captured []capturedRequest
			server := newScriptedServer(t, tt.statuses, &captured)

			method := http.MethodGet
			var body io.Reader
			if tt.body != nil {
				method = http.MethodPost
				body = bytes.NewReader(tt.body)
			}
			req, err := http.NewRequest(method, server.URL, body)
			if err != nil {
				t.Fatalf("http.NewRequest() error = %v", err)
			}

			client := &http.Client{Transport: &authRT{}}
			resp, err := client.Do(req)
			if resp != nil {
				resp.Body.Close()
			}

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("client.Do() error = %v; want it to name %q as the rejected source", err, tt.wantErr)
				}
			} else {
				if err != nil {
					t.Fatalf("client.Do() error = %v", err)
				}
				if resp.StatusCode != tt.wantStatus {
					t.Errorf("final status = %d; want %d", resp.StatusCode, tt.wantStatus)
				}
			}

			if len(captured) != len(tt.wantAuth) {
				t.Fatalf("server saw %d requests; want exactly %d", len(captured), len(tt.wantAuth))
			}
			for i, c := range captured {
				if c.authHeader != tt.wantAuth[i] {
					t.Errorf("request %d Authorization = %q; want %q", i, c.authHeader, tt.wantAuth[i])
				}
				if tt.body != nil && !bytes.Equal(c.body, tt.body) {
					t.Errorf("request %d body = %q; want byte-identical to the first (%q) -- proves the GetBody rewind happened", i, c.body, tt.body)
				}
			}

			if tt.wantCached != "" {
				if tok, ok := readCachedToken(); !ok || tok != tt.wantCached {
					t.Errorf("readCachedToken() = (%q, %v); want the freshly resolved token %q to have been cached", tok, ok, tt.wantCached)
				}
			}
		})
	}
}

// TestRunGHAuthTokenSeam_HonoursGhAuthTokenTimeout encodes the operator requirement directly rather
// than mere behaviour: a fake that hangs like a stuck `gh` process must still cause resolveToken to
// return once ghAuthTokenTimeout elapses.
// This is a test that would hang forever on a regression that dropped the context deadline.
func TestRunGHAuthTokenSeam_HonoursGhAuthTokenTimeout(t *testing.T) {
	dir := t.TempDir()
	setCacheDir(t, dir)
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")

	// Shrink the timeout for this test so it proves the seam engages without
	// paying the real 5s production wait -- same save/override/restore shape
	// withFakeGHAuthToken uses above for runGHAuthToken.
	origTimeout := ghAuthTokenTimeout
	ghAuthTokenTimeout = 10 * time.Millisecond
	t.Cleanup(func() { ghAuthTokenTimeout = origTimeout })

	// A well-behaved fake -- like the real `gh auth token` subprocess it
	// stands in for -- respects ctx's deadline instead of running forever.
	withFakeGHAuthToken(t, func(ctx context.Context) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})

	start := time.Now()
	_, _, err := resolveToken()
	elapsed := time.Since(start)

	if !errors.Is(err, ErrTokenUnresolvable) {
		t.Fatalf("resolveToken() error = %v; want ErrTokenUnresolvable once the timeout fires", err)
	}
	if elapsed < ghAuthTokenTimeout {
		t.Errorf("resolveToken() returned after %v; want at least ghAuthTokenTimeout (%v) -- an early return here would mean this test is not exercising the timeout at all", elapsed, ghAuthTokenTimeout)
	}
	// 200ms is 20x the shrunk 10ms timeout -- generous against scheduling
	// jitter while still tight enough to catch a regression where the
	// override silently fails to apply (which would make the real call take
	// ~5s and trip this check).
	const slack = 200 * time.Millisecond
	if elapsed > ghAuthTokenTimeout+slack {
		t.Errorf("resolveToken() took %v; want close to ghAuthTokenTimeout (%v), not an unbounded hang", elapsed, ghAuthTokenTimeout)
	}
}

// TestAuthRT_WaitsWhileAPauseCoversTheProcess pins that a request waits at entry while a live pause covers the process, writes one gate line, and goes out once the pause is resumed.
// It is not parallel: the pause network gate is process-wide state.
func TestAuthRT_WaitsWhileAPauseCoversTheProcess(t *testing.T) {
	setCacheDir(t, t.TempDir())
	t.Setenv("GH_TOKEN", "env-token")
	t.Setenv("GITHUB_TOKEN", "")

	recordPath := filepath.Join(t.TempDir(), pauserecord.RecordFileName)
	setLive := func(entries ...pauserecord.Entry) {
		t.Helper()
		if _, err := pauserecord.Update(recordPath, time.Now(), func(r *pauserecord.Record) error {
			r.Live = entries
			return nil
		}); err != nil {
			t.Fatalf("update the pause record: %v", err)
		}
	}
	setLive(pauserecord.Entry{Scope: pauserecord.ScopeHub, PausedAt: time.Now()})

	var mu sync.Mutex
	var lines []string
	wakes := make(chan struct{}, 100)
	t.Cleanup(pauserecord.InstallGate(pauserecord.Gate{
		Geometry: pauserecord.Geometry{RecordPath: recordPath},
		Notify: func(line string) {
			mu.Lock()
			defer mu.Unlock()
			lines = append(lines, line)
		},
		Sleep: func(ctx context.Context, d time.Duration) error {
			wakes <- struct{}{}
			time.Sleep(20 * time.Millisecond)
			return nil
		},
	}))

	var captured []capturedRequest
	server := newScriptedServer(t, []int{http.StatusOK}, &captured)
	do := func() error {
		req, err := http.NewRequest(http.MethodGet, server.URL, nil)
		if err != nil {
			return err
		}
		resp, err := (&http.Client{Transport: &authRT{}}).Do(req)
		if resp != nil {
			resp.Body.Close()
		}
		return err
	}

	finished := make(chan error, 1)
	go func() { finished <- do() }()

	select {
	case <-wakes:
	case <-time.After(10 * time.Second):
		t.Fatal("the held request never woke its wait")
	}
	if len(captured) != 0 {
		t.Fatalf("server hits after the first wake = %d; want 0 while the pause covers the process", len(captured))
	}
	mu.Lock()
	if len(lines) != 1 || !strings.Contains(lines[0], `"lyx resume"`) {
		t.Fatalf("gate lines = %q; want exactly one naming lyx resume", lines)
	}
	mu.Unlock()

	setLive()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("held request error = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the request did not complete after the pause was resumed")
	}
	if len(captured) != 1 {
		t.Fatalf("server hits after the resume = %d; want 1", len(captured))
	}

	for len(wakes) > 0 {
		<-wakes
	}
	if err := do(); err != nil {
		t.Fatalf("request with no pause: %v", err)
	}
	if len(wakes) != 0 || len(captured) != 2 {
		t.Fatalf("with no pause: wakes = %d, hits = %d; want no wake and a second hit", len(wakes), len(captured))
	}
}

// pauseHoldDuration is a top-level constant so the untagged-test sleep guard can resolve it.
const pauseHoldDuration = 150 * time.Millisecond

// TestAuthRT_HeldCallGetsItsFullDeadlineAfterRelease pins the transport's deadlines around a pause hold:
// a held call gets its caller deadline moved later by the held time and its client timeout from the release, an unheld call and a call admitted before the pause keep theirs,
// and a cancellation ends a held call unless the caller's own deadline had already passed.
// It is not parallel: the pause network gate is process-wide state.
func TestAuthRT_HeldCallGetsItsFullDeadlineAfterRelease(t *testing.T) {
	setCacheDir(t, t.TempDir())
	t.Setenv("GH_TOKEN", "test-token")
	t.Setenv("GITHUB_TOKEN", "")

	const budget = 50 * time.Millisecond

	type env struct {
		cancel  context.CancelFunc
		setLive func(live bool)
	}
	type result struct {
		err    error
		sleeps int32
		hits   int32
	}
	// run sends one request through an authRT with a 50 ms timeout, under a caller deadline of callerTimeout.
	// A live hub entry exists from the start when live is set; sleep is the gate's wait seam and handler serves the request.
	run := func(t *testing.T, callerTimeout time.Duration, live bool, sleep func(*env), handler func(*env, http.ResponseWriter, *http.Request)) result {
		t.Helper()
		recordPath := filepath.Join(t.TempDir(), pauserecord.RecordFileName)
		e := &env{}
		e.setLive = func(live bool) {
			t.Helper()
			if _, err := pauserecord.Update(recordPath, time.Now(), func(r *pauserecord.Record) error {
				r.Live = nil
				if live {
					r.Live = []pauserecord.Entry{{Scope: pauserecord.ScopeHub, PausedAt: time.Now()}}
				}
				return nil
			}); err != nil {
				t.Fatalf("update the pause record: %v", err)
			}
		}
		e.setLive(live)

		var sleeps, hits atomic.Int32
		t.Cleanup(pauserecord.InstallGate(pauserecord.Gate{
			Geometry: pauserecord.Geometry{RecordPath: recordPath},
			Notify:   func(string) {},
			Sleep: func(ctx context.Context, d time.Duration) error {
				sleeps.Add(1)
				sleep(e)
				return nil
			},
		}))
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			handler(e, w, r)
		}))
		t.Cleanup(server.Close)

		ctx, cancel := context.WithTimeout(t.Context(), callerTimeout)
		defer cancel()
		e.cancel = cancel
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := (&http.Client{Transport: &authRT{timeout: budget}}).Do(req)
		if resp != nil {
			resp.Body.Close()
		}
		return result{err: err, sleeps: sleeps.Load(), hits: hits.Load()}
	}
	answerAtOnce := func(*env, http.ResponseWriter, *http.Request) {}
	liftAfterHold := func(e *env) {
		time.Sleep(pauseHoldDuration)
		e.setLive(false)
	}
	neverSleeps := func(*env) {}
	blockUntilGone := func(e *env, w http.ResponseWriter, r *http.Request) {
		e.cancel()
		<-r.Context().Done()
	}

	t.Run("a held call succeeds after release", func(t *testing.T) {
		got := run(t, budget, true, liftAfterHold, answerAtOnce)
		if got.err != nil || got.sleeps == 0 || got.hits != 1 {
			t.Fatalf("err = %v, sleeps = %d, hits = %d; want success after a hold and one hit", got.err, got.sleeps, got.hits)
		}
	})

	t.Run("an unheld call keeps its deadline", func(t *testing.T) {
		got := run(t, budget, false, neverSleeps, func(*env, http.ResponseWriter, *http.Request) { time.Sleep(pauseHoldDuration) })
		if !errors.Is(got.err, context.DeadlineExceeded) || got.sleeps != 0 {
			t.Fatalf("err = %v, sleeps = %d; want a deadline error and no hold", got.err, got.sleeps)
		}
	})

	t.Run("a call admitted before the pause is neither held nor extended", func(t *testing.T) {
		got := run(t, budget, false, neverSleeps, func(e *env, w http.ResponseWriter, r *http.Request) {
			e.setLive(true)
			time.Sleep(pauseHoldDuration)
		})
		if !errors.Is(got.err, context.DeadlineExceeded) || got.sleeps != 0 {
			t.Fatalf("err = %v, sleeps = %d; want a deadline error and no hold", got.err, got.sleeps)
		}
	})

	t.Run("cancelled during the hold sends nothing", func(t *testing.T) {
		got := run(t, 10*time.Second, true, func(e *env) {
			e.cancel()
			e.setLive(false)
		}, answerAtOnce)
		if !errors.Is(got.err, context.Canceled) || got.hits != 0 {
			t.Fatalf("err = %v, hits = %d; want context.Canceled and nothing sent", got.err, got.hits)
		}
	})

	t.Run("cancelled after release ends the call", func(t *testing.T) {
		started := time.Now()
		got := run(t, 10*time.Second, true, func(e *env) { e.setLive(false) }, blockUntilGone)
		if !errors.Is(got.err, context.Canceled) || time.Since(started) > 5*time.Second {
			t.Fatalf("err = %v after %v; want context.Canceled well within the deadline", got.err, time.Since(started))
		}
	})

	t.Run("cancelled after its own deadline passed during the hold waits for the moved deadline", func(t *testing.T) {
		got := run(t, budget, true, liftAfterHold, blockUntilGone)
		if !errors.Is(got.err, context.DeadlineExceeded) || got.hits != 1 {
			t.Fatalf("err = %v, hits = %d; want a deadline error after the request went out", got.err, got.hits)
		}
	})
}
