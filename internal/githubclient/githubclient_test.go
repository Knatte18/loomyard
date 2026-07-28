//go:build windows

// githubclient_test.go is the hermetic, table-driven test suite for the
// whole package: the token resolution chain (token.go), the on-disk cache
// (cache.go/cache_windows.go), and the authenticating RoundTripper
// (transport.go). Every case redirects the cache to a t.TempDir() via
// t.Setenv and reaches the `gh auth token` shell-out only through its
// injected seam, so the suite runs correctly on a machine with no `gh`
// installed and no GitHub credentials, and never touches the operator's
// real credential file. The Windows ACL assertion below is why this file
// carries a platform build tag -- per the Test Tier Purity Invariant, a
// platform-only constraint still counts as untagged Tier 1.

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
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// setCacheDir redirects the token cache to dir by setting LOCALAPPDATA,
// the only environment variable cacheDir consults on Windows. Every test in
// this file calls this (directly or via seedCacheFile) before touching
// anything else, so the operator's real credential file is never read or
// written.
func setCacheDir(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("LOCALAPPDATA", dir)
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

// TestResolveToken_Chain covers the full resolution order end to end: env
// vars always win over the cache, GH_TOKEN before GITHUB_TOKEN, a fresh
// cache entry is used when both env vars are empty, a stale one is not, and
// an unresolvable token surfaces as ErrTokenUnresolvable.
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

			gotToken, gotSource, err := resolveToken()

			if tt.wantErr {
				if !errors.Is(err, ErrTokenUnresolvable) {
					t.Fatalf("resolveToken() error = %v; want ErrTokenUnresolvable", err)
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

// TestCacheDirRedirection_HonoursOverride asserts the redirection itself
// works, rather than assuming it: pointing the environment at an empty
// temp dir must resolve cacheDir under that temp dir and report a cache
// miss there, never silently fall back to a real user path.
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

// TestReadCachedToken_MalformedFileIsMiss covers every way the on-disk file
// can fail to match the exact two-field schema: each is a cache miss, never
// a fatal error, so a foreign or worn-out file never blocks resolution.
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

// TestWriteCachedToken_CreatesFileWithRestrictivePermissions asserts the
// hardening this file requires directly against the Windows security
// descriptor, rather than inferring it from os.Stat's reported mode bits --
// which, on Windows, reflect only the read-only attribute and would report
// the same value regardless of whether hardenCacheFile's os.Chmod call or
// its DACL step ran at all.
func TestWriteCachedToken_CreatesFileWithRestrictivePermissions(t *testing.T) {
	dir := t.TempDir()
	setCacheDir(t, dir)

	writeCachedToken("a-token")

	resolvedDir, ok := cacheDir()
	if !ok {
		t.Fatal("cacheDir() ok = false")
	}
	path := filepath.Join(resolvedDir, credentialFileName)

	// os.Stat's synthesized Mode() on Windows reflects only the read-only
	// attribute bit, not the 0600 os.Chmod call hardenCacheFile makes --
	// Windows has no Unix permission bits to report, which is exactly why
	// the DACL assertion below, not this stat, is the real check.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Stat(%s): %v", path, err)
	}

	assertOwnerOnlyDACL(t, path)
}

// assertOwnerOnlyDACL reads path's security descriptor via
// golang.org/x/sys/windows and asserts its DACL grants access to exactly one
// trustee: the file's own owner. This is the direct ACL assertion card 26
// requires, as distinct from an inference drawn from mode bits.
func assertOwnerOnlyDACL(t *testing.T, path string) {
	t.Helper()

	descriptor, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.OWNER_SECURITY_INFORMATION,
	)
	if err != nil {
		t.Fatalf("GetNamedSecurityInfo(%s): %v", path, err)
	}

	dacl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatalf("descriptor.DACL(): %v", err)
	}
	owner, _, err := descriptor.Owner()
	if err != nil {
		t.Fatalf("descriptor.Owner(): %v", err)
	}

	if dacl.AceCount != 1 {
		t.Fatalf("cache file DACL has %d ACEs; want exactly 1 (owner-only, breaking inheritance)", dacl.AceCount)
	}

	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil {
		t.Fatalf("GetAce(dacl, 0): %v", err)
	}
	// The ACE's SID is stored inline immediately after its fixed fields;
	// this is the same address-of-trailing-field pattern golang.org/x/sys's
	// own tests use to read it back out.
	aceSid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	if !owner.Equals(aceSid) {
		t.Errorf("cache file DACL's sole ACE grants access to a SID other than the file's owner")
	}
}

// TestWriteCachedToken_UnwritableDirDegradesToInProcessResolution covers the
// requirement that a cache directory which cannot be created degrades to
// in-process resolution rather than failing the command: writeCachedToken
// must not panic or error out loud, and a subsequent resolveToken must still
// succeed via the (faked) gh CLI fallback.
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

// TestCache_ConcurrentWriters drives many goroutines through writeCachedToken
// and readCachedToken at once. The requirement is not that any particular
// write wins -- it is that the file is always either absent or fully
// parseable, never left half-written by a torn concurrent write.
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

// TestAuthRT_401InvalidatesAndReplaysExactlyOnce covers the transport's core
// contract: a 401 invalidates the rejected (cache-sourced) token, resolves a
// fresh one exactly once, and replays with it -- never in a loop.
func TestAuthRT_401InvalidatesAndReplaysExactlyOnce(t *testing.T) {
	dir := t.TempDir()
	setCacheDir(t, dir)
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	seedCacheFile(t, "cached-token", time.Now())

	withFakeGHAuthToken(t, func(ctx context.Context) (string, error) {
		return "fresh-token", nil
	})

	var captured []capturedRequest
	server := newScriptedServer(t, []int{http.StatusUnauthorized, http.StatusOK}, &captured)

	client := &http.Client{Transport: &authRT{}}
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("client.Get() error = %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("final status = %d; want %d", resp.StatusCode, http.StatusOK)
	}
	if len(captured) != 2 {
		t.Fatalf("server saw %d requests; want exactly 2 (original + one replay)", len(captured))
	}
	if captured[0].authHeader != "Bearer cached-token" {
		t.Errorf("first request Authorization = %q; want %q", captured[0].authHeader, "Bearer cached-token")
	}
	if captured[1].authHeader != "Bearer fresh-token" {
		t.Errorf("replay Authorization = %q; want %q", captured[1].authHeader, "Bearer fresh-token")
	}

	if tok, ok := readCachedToken(); !ok || tok != "fresh-token" {
		t.Errorf("readCachedToken() = (%q, %v); want the freshly resolved token to have been cached", tok, ok)
	}
}

// TestAuthRT_SecondConsecutive401Propagates asserts the replay never loops:
// once the replay itself is rejected, the second 401 goes back to the
// caller unchanged.
func TestAuthRT_SecondConsecutive401Propagates(t *testing.T) {
	dir := t.TempDir()
	setCacheDir(t, dir)
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	seedCacheFile(t, "cached-token", time.Now())

	withFakeGHAuthToken(t, func(ctx context.Context) (string, error) {
		return "fresh-token", nil
	})

	var captured []capturedRequest
	server := newScriptedServer(t, []int{http.StatusUnauthorized, http.StatusUnauthorized}, &captured)

	client := &http.Client{Transport: &authRT{}}
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("client.Get() error = %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("final status = %d; want %d (a second consecutive 401 must propagate, never loop)", resp.StatusCode, http.StatusUnauthorized)
	}
	if len(captured) != 2 {
		t.Fatalf("server saw %d requests; want exactly 2 -- no further retries after the replay's own 401", len(captured))
	}
}

// TestAuthRT_EnvSourcedTokenSkipsReplayOn401 asserts that an
// environment-sourced token is never invalidated and replayed: replaying it
// is guaranteed to reproduce the identical value, so the 401 is surfaced as
// an error naming the rejected variable, with no second request at all.
func TestAuthRT_EnvSourcedTokenSkipsReplayOn401(t *testing.T) {
	dir := t.TempDir()
	setCacheDir(t, dir)
	t.Setenv("GH_TOKEN", "env-token")

	var captured []capturedRequest
	server := newScriptedServer(t, []int{http.StatusUnauthorized}, &captured)

	client := &http.Client{Transport: &authRT{}}
	resp, err := client.Get(server.URL)
	if resp != nil {
		resp.Body.Close()
	}
	if err == nil {
		t.Fatal("client.Get() error = nil; want an error naming the rejected GH_TOKEN source")
	}
	if got := err.Error(); !strings.Contains(got, "GH_TOKEN") {
		t.Errorf("error = %q; want it to name GH_TOKEN as the rejected source", got)
	}
	if len(captured) != 1 {
		t.Fatalf("server saw %d requests; want exactly 1 -- an env-sourced token must never be replayed", len(captured))
	}
}

// TestAuthRT_ReplayRewindsRequestBodyByteIdentical is the case the design
// flags as silently misbehaving without it: a 401 then success, asserting
// the replayed request's body is byte-identical to the first. This is what
// proves the req.GetBody rewind actually happened, rather than the replay
// sending an already-drained (empty) body.
func TestAuthRT_ReplayRewindsRequestBodyByteIdentical(t *testing.T) {
	dir := t.TempDir()
	setCacheDir(t, dir)
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	seedCacheFile(t, "cached-token", time.Now())

	withFakeGHAuthToken(t, func(ctx context.Context) (string, error) {
		return "fresh-token", nil
	})

	var captured []capturedRequest
	server := newScriptedServer(t, []int{http.StatusUnauthorized, http.StatusCreated}, &captured)

	wantBody := []byte(`{"title":"a test issue"}`)
	req, err := http.NewRequest(http.MethodPost, server.URL, bytes.NewReader(wantBody))
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}

	client := &http.Client{Transport: &authRT{}}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("client.Do() error = %v", err)
	}
	resp.Body.Close()

	if len(captured) != 2 {
		t.Fatalf("server saw %d requests; want exactly 2", len(captured))
	}
	if !bytes.Equal(captured[0].body, wantBody) {
		t.Errorf("first request body = %q; want %q", captured[0].body, wantBody)
	}
	if !bytes.Equal(captured[1].body, wantBody) {
		t.Errorf("replayed request body = %q; want byte-identical to the first (%q) -- proves the GetBody rewind happened", captured[1].body, wantBody)
	}
}

// TestRunGHAuthTokenSeam_HonoursGhAuthTokenTimeout encodes the operator
// requirement directly rather than mere behaviour: a fake that hangs like a
// stuck `gh` process must still cause resolveToken to return once
// ghAuthTokenTimeout elapses. This is a test that would hang forever on a
// regression that dropped the context deadline.
func TestRunGHAuthTokenSeam_HonoursGhAuthTokenTimeout(t *testing.T) {
	dir := t.TempDir()
	setCacheDir(t, dir)
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")

	// A well-behaved fake -- like the real exec.CommandContext-launched
	// process it stands in for -- respects ctx's deadline instead of
	// running forever.
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
	const slack = 5 * time.Second
	if elapsed > ghAuthTokenTimeout+slack {
		t.Errorf("resolveToken() took %v; want close to ghAuthTokenTimeout (%v), not an unbounded hang", elapsed, ghAuthTokenTimeout)
	}
}

// TestResolveToken_UnresolvableReturnsTypedErrorWithoutBlocking encodes the
// second operator requirement: when the gh CLI itself fails fast (no
// session), resolution must return ErrTokenUnresolvable immediately, never
// wait or prompt.
func TestResolveToken_UnresolvableReturnsTypedErrorWithoutBlocking(t *testing.T) {
	dir := t.TempDir()
	setCacheDir(t, dir)
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")

	withFakeGHAuthToken(t, func(ctx context.Context) (string, error) {
		return "", errors.New("gh: not logged in to any GitHub hosts")
	})

	start := time.Now()
	tok, source, err := resolveToken()
	elapsed := time.Since(start)

	if !errors.Is(err, ErrTokenUnresolvable) {
		t.Fatalf("resolveToken() error = %v; want ErrTokenUnresolvable", err)
	}
	if tok != "" || source != sourceUnknown {
		t.Errorf("resolveToken() = (%q, %v); want (\"\", sourceUnknown) alongside the error", tok, source)
	}
	if elapsed > time.Second {
		t.Errorf("resolveToken() took %v to report an unresolvable token; want a fast typed error, never a wait", elapsed)
	}
}
