// cache.go implements the cross-platform half of githubclient's
// machine-global token cache: directory resolution, the on-disk schema,
// freshness computation, and the atomic-replace write path. Platform-specific
// file hardening — stripping inherited ACLs on Windows so the cache file is
// readable only by its owner — is delegated to the unexported hardenCacheFile
// hook, implemented per platform in cache_windows.go and cache_other.go.

package githubclient

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// cacheFreshness is how long a cached token is trusted before resolveToken
// treats it as a miss and falls through to `gh auth token`. No expiry field
// is stored on disk; the TTL is applied entirely at read time so changing it
// never requires migrating existing cache files.
const cacheFreshness = 12 * time.Hour

// credentialFileName is the on-disk name of the cache file within its
// resolved directory.
const credentialFileName = "credentials.json"

// cachedCredentials is the on-disk schema of the token cache file: exactly
// two fields, with ResolvedAt stored as RFC 3339 UTC. Any other field shape,
// a missing field, or an unparseable timestamp is treated as a cache miss by
// readCachedToken, never as a fatal error — this file is an optimization,
// not a source of truth.
type cachedCredentials struct {
	Token      string `json:"token"`
	ResolvedAt string `json:"resolved_at"`
}

// cacheMu serializes this process's own cache reads/writes. It complements,
// rather than replaces, the atomic-rename protocol in writeCachedToken:
// the rename is what keeps concurrent *processes* safe, while this mutex
// avoids two goroutines in the same process racing on temp-file naming.
var cacheMu sync.Mutex

// cacheDir resolves the directory holding the token cache file. On Windows
// this is %LOCALAPPDATA%\lyx; elsewhere it is $XDG_CONFIG_HOME/lyx, falling
// back to $HOME/.config/lyx. Resolution reads only these environment
// variables — never os.UserConfigDir, never a hardcoded absolute path — so a
// test can redirect the whole cache to a temp dir with t.Setenv and can never
// read or write the operator's real credential file. Returns ("", false)
// when no usable environment variable is set; callers treat that as a cache
// miss, never an error.
func cacheDir() (string, bool) {
	if runtime.GOOS == "windows" {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			return "", false
		}
		return filepath.Join(base, "lyx"), true
	}

	if base := os.Getenv("XDG_CONFIG_HOME"); base != "" {
		return filepath.Join(base, "lyx"), true
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".config", "lyx"), true
	}
	return "", false
}

// credentialFilePath resolves the full path to the cache file, or ("", false)
// when cacheDir itself cannot be resolved.
func credentialFilePath() (string, bool) {
	dir, ok := cacheDir()
	if !ok {
		return "", false
	}
	return filepath.Join(dir, credentialFileName), true
}

// readCachedToken reads and validates the token cache file, returning the
// token and true only when the file exists, parses into the exact two-field
// schema, and its resolved_at timestamp is within cacheFreshness of now. Any
// other outcome — an unresolvable directory, a missing file, a parse
// failure, an unexpected field shape, or a stale or unparseable timestamp —
// is a cache miss, never an error, so a worn or foreign file never blocks
// resolution.
func readCachedToken() (string, bool) {
	path, ok := credentialFilePath()
	if !ok {
		return "", false
	}

	cacheMu.Lock()
	defer cacheMu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}

	// Require the exact two-field shape rather than tolerating extra or
	// missing fields: a foreign or malformed file is a miss, not a
	// half-trusted partial read.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil || len(raw) != 2 {
		return "", false
	}

	var creds cachedCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return "", false
	}
	if creds.Token == "" || creds.ResolvedAt == "" {
		return "", false
	}

	resolvedAt, err := time.Parse(time.RFC3339, creds.ResolvedAt)
	if err != nil {
		return "", false
	}
	if time.Now().UTC().Sub(resolvedAt.UTC()) >= cacheFreshness {
		return "", false
	}

	return creds.Token, true
}

// writeCachedToken atomically writes token to the cache file, recording the
// current time as resolved_at in RFC 3339 UTC. The write goes to a temp file
// in the same directory as the target; hardenCacheFile applies the
// restrictive permissions (0600, plus an owner-only ACL on Windows) to that
// temp file BEFORE the rename — hardening after the rename would leave a
// window where the real path exists world-readable, quietly undoing the
// protection. A cache directory that cannot be created, or any other failure
// along the way, degrades to a silent no-op: an unwritable cache is a missed
// optimization, never a resolution failure.
func writeCachedToken(token string) {
	dir, ok := cacheDir()
	if !ok {
		return
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}

	creds := cachedCredentials{
		Token:      token,
		ResolvedAt: time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.Marshal(&creds)
	if err != nil {
		return
	}

	cacheMu.Lock()
	defer cacheMu.Unlock()

	tmp, err := os.CreateTemp(dir, credentialFileName+".tmp-*")
	if err != nil {
		return
	}
	tmpPath := tmp.Name()

	// Any early return past this point must clean up the temp file; only a
	// successful rename below consumes it.
	renamed := false
	defer func() {
		if !renamed {
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return
	}
	if err := tmp.Close(); err != nil {
		return
	}

	if err := hardenCacheFile(tmpPath); err != nil {
		return
	}

	targetPath := filepath.Join(dir, credentialFileName)
	if err := os.Rename(tmpPath, targetPath); err != nil {
		return
	}
	renamed = true
}

// invalidateCachedToken removes the on-disk cache file so the next
// resolveToken call cannot reuse a token the server has just rejected with a
// 401. A missing file is not an error — there was nothing to invalidate.
func invalidateCachedToken() {
	path, ok := credentialFilePath()
	if !ok {
		return
	}

	cacheMu.Lock()
	defer cacheMu.Unlock()

	os.Remove(path)
}
