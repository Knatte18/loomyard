// server.go computes the per-hub tmux server identity: the server name
// (also reused as the -L socket name) and the per-worktree session name.
// Server-name construction lives here, in the tmux domain, rather than in
// hubgeometry, because it is a tmux-specific derivation (not a filesystem
// path) computed from a Layout.Hub value hubgeometry already resolves. The
// file is named server.go, not naming.go, so it is not confusable with the
// strand-name helpers in name.go.
package muxengine

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
)

// ServerName returns the deterministic tmux server name for the hub at
// hubPath: "lyx-<hub-basename>-<short-hash>", where <short-hash> is the
// first 8 hex characters of sha256(abs-hub-path). Every worktree under the
// same hub computes the same name, which is the mechanism behind the
// one-named-server-per-hub firewall: two worktrees under one hub share a
// server, and two hubs that happen to share a basename still get distinct
// names because the hash is computed over the full absolute path.
func ServerName(hubPath string) string {
	abs := cleanAbsHubPath(hubPath)
	base := filepath.Base(abs)
	sum := sha256.Sum256([]byte(abs))
	shortHash := hex.EncodeToString(sum[:])[:8]
	return "lyx-" + base + "-" + shortHash
}

// SessionName returns the tmux session name for a worktree: the worktree's
// directory slug. Each strand's parent worktree maps to its own session
// within the shared per-hub server.
func SessionName(worktreeRoot string) string {
	return filepath.Base(worktreeRoot)
}

// socketName returns the socket-safe server name for the hub at hubPath.
// It is identical to ServerName: the "lyx-<base>-<sha8>" form is already
// composed only of lowercase-safe hex and the hub basename run through
// filepath.Base, which on both Windows and POSIX never contains ':', '\',
// or a space, so no further sanitization is required to make it a safe -L
// socket argument.
func socketName(hubPath string) string {
	return ServerName(hubPath)
}

// cleanAbsHubPath resolves hubPath to its cleaned absolute form so that the
// hash is stable regardless of how the caller spelled the path (relative,
// trailing separator, mixed case on a case-insensitive filesystem is left
// to the caller — ServerName does not itself lowercase, since hub paths are
// case-sensitive on POSIX).
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
