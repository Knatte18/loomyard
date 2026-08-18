// server.go computes the per-hub tmux server identity: the server name (also reused as the -L
// socket name) and the per-worktree session name.
// Both construction rules live here, in the tmux domain, rather than in lyxcwd, because each is a
// tmux-specific derivation (a socket key and a session name, neither of them a filesystem path)
// over a plain hub or worktree path string its caller already resolved.
// Neither function sees a *lyxcwd.Location: hubgeom.ReedGeometry calls them to fill
// Geometry.SocketKey/SessionName, and this package only ever reads those told fields back.
// It also carries validateToldTmuxIdentity, the pre-flight that refuses a told tmux identity the
// multiplexer could not spend verbatim — the one check that keeps "New validates nothing" from
// meaning "an unusable identity is discovered only as a 20-second boot timeout".
// The file is named server.go, not naming.go, so it is not confusable with the strand-name helpers
// in name.go.
package reedengine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
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

// rewrittenSessionNameChars are the characters tmux silently rewrites to '_' inside a session name.
// tmux does not REJECT them: new-session exits 0 and creates a session under the rewritten name
// (verified live, tmux 3.6: "a.b" and "a:b" both become "a_b"; no other character is touched).
// That silence is what makes the ban necessary rather than cosmetic — every other -t target this
// package issues is an EXACT-match "=<name>" form (see exactSessionTarget), deliberately so, and an
// exact target can never match a name tmux rewrote behind reed's back.
const rewrittenSessionNameChars = ".:"

// validateToldTmuxIdentity reports an error when the told SessionName is one the configured
// multiplexer cannot spend verbatim, naming the worktree directory it derives from so the operator
// can act on it.
//
// It exists because reedengine.New validates no Geometry field by contract (see geometry.go) while
// tmux answers a malformed session name with silence rather than an error — a rewritten name looks,
// from every probe reed has, exactly like a server that has not finished booting. Without this check
// the boot loop burns its full attempt budget and reports a timeout that names neither cause, and it
// leaves the rewritten session running on the shared per-hub server with no reed verb able to
// address, or tear down, the thing it just created.
//
// The answer is a refusal, never a sanitization: rewriting '.' to '_' here would map sibling
// worktrees "svc.v2" and "svc_v2" onto one session name, and each worktree's engine would then adopt
// the other's panes — strictly worse than refusing to start.
func validateToldTmuxIdentity(geom Geometry) error {
	if geom.SessionName == "" {
		return fmt.Errorf("told geometry has an empty tmux session name (worktree %q)", geom.WorktreeRoot)
	}
	if i := strings.IndexAny(geom.SessionName, rewrittenSessionNameChars); i >= 0 {
		return fmt.Errorf(
			"tmux will not create session %q verbatim: it contains %q, which tmux silently rewrites to \"_\" — rename the worktree directory %q so its name carries no %q",
			geom.SessionName, string(geom.SessionName[i]), geom.WorktreeRoot, rewrittenSessionNameChars)
	}
	return nil
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
