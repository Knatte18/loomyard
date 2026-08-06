// cache.go implements the cross-platform half of githubclient's machine-global token cache:
// directory resolution, the on-disk schema, freshness computation, and the atomic-replace write
// path.
// Platform-specific file hardening — stripping inherited ACLs on Windows so the cache file is
// readable only by its owner — is delegated to the unexported hardenCacheFile hook, implemented per
// platform in cache_windows.go and cache_other.go.

package githubclient

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// cacheFreshness is how long a cached token is trusted before falling back to
// `gh auth token`. The TTL is applied at read time only; no disk migration needed.
const cacheFreshness = 12 * time.Hour

// credentialFileName is the on-disk name of the cache file within its
// resolved directory.
const credentialFileName = "credentials.json"

// cachedCredentials is the on-disk token cache schema: exactly two fields, with
// ResolvedAt as RFC 3339 UTC. Any deviation is a cache miss, never a fatal error.
type cachedCredentials struct {
	Token      string `json:"token"`
	ResolvedAt string `json:"resolved_at"`
}

// cacheMu serializes cache reads/writes within this process. The atomic-rename
// in writeCachedToken keeps concurrent processes safe.
var cacheMu sync.Mutex

// cacheDir resolves the cache directory: %LOCALAPPDATA%\lyx on Windows,
// $XDG_CONFIG_HOME/lyx or $HOME/.config/lyx elsewhere. Returns ("", false)
// when no usable environment variable is set.
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

// readCachedToken reads the cache file, returning the token only when it
// exists, parses correctly, and is fresh. Any other outcome is a cache miss.
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

// writeCachedToken atomically writes token to the cache file via a temp file
// and rename. Hardens the file before rename to avoid a world-readable window.
// Degradation to silent no-op on any failure.
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

// invalidateCachedToken removes the cache file so a rejected token is not
// reused. A missing file is not an error.
func invalidateCachedToken() {
	path, ok := credentialFilePath()
	if !ok {
		return
	}

	cacheMu.Lock()
	defer cacheMu.Unlock()

	os.Remove(path)
}
