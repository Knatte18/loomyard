// server.go computes the per-hub tmux server identity: the server name (also reused as the -L
// socket name) and the per-worktree session name.
// Both construction rules live here, in the tmux domain, rather than in lyxcwd, because each is a
// tmux-specific derivation (a socket key and a session name, neither of them a filesystem path)
// over a plain hub or worktree path string its caller already resolved.
// Neither function sees a *lyxcwd.Location: hubgeom.ReedGeometry calls them to fill
// Geometry.SocketKey/SessionName, and this package only ever reads those told fields back.
// The file is named server.go, not naming.go, so it is not confusable with the strand-name helpers
// in name.go.
package reedengine

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
)

// ServerName returns the deterministic tmux server name for the hub: "lyx-<basename>-<hash>", where
// hash ensures distinct hubs are distinct.
func ServerName(hubPath string) string {
	abs := cleanAbsHubPath(hubPath)
	base := filepath.Base(abs)
	sum := sha256.Sum256([]byte(abs))
	shortHash := hex.EncodeToString(sum[:])[:8]
	return "lyx-" + base + "-" + shortHash
}

// SessionName returns the tmux session name for a worktree: its directory slug.
func SessionName(worktreeRoot string) string {
	return filepath.Base(worktreeRoot)
}

// cleanAbsHubPath resolves hubPath to its cleaned absolute form for stable hashing.
func cleanAbsHubPath(hubPath string) string {
	abs, err := filepath.Abs(hubPath)
	if err != nil {
		// filepath.Abs only fails when the current working directory cannot
		// be resolved; fall back to a cleaned version of the input so
		// ServerName stays total rather than panicking or returning an error
		// the caller would have to plumb through every call site.
		return filepath.Clean(hubPath)
	}
	return abs
}
