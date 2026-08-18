// geometry.go declares Geometry, the seven-field struct reed is told its coordinates through.
// It declares the type only — New and every method stay in their existing files (lock.go,
// lifecycle.go, strand.go, header.go); this file adds no constructor, no validator, and no default.

package reedengine

// Geometry is the set of paths and identity strings reed is told, once, at construction, and never
// derives itself.
// reedengine.New validates no field of a Geometry, and no method recomputes any of them from the
// others — populating every field with a usable absolute path (or, for SocketKey, a socket-safe key)
// is entirely the caller's obligation.
// hubgeom.ReedGeometry is the hub-mode answer that builds a Geometry from a resolved
// *lyxcwd.Location; reedengine.ServerName(hubPath) is SocketKey's derivation.
// Neither is imported here — this file states the contract, not the implementation.
type Geometry struct {
	// SocketKey is the tmux -L socket name; it is what Engine.Socket returns.
	// It must carry no path separator: tmux resolves -L as a filename under its per-user socket
	// directory, so a separator makes it a path whose parent does not exist — and tmux answers that
	// with a stderr line and exit 0, indistinguishable from a slow boot. ServerName substitutes
	// separators out at the derivation; validateToldTmuxIdentity (server.go) is the backstop for a
	// teller that builds this field some other way.
	SocketKey string
	// SessionName is the tmux session name; it is what Engine.SessionName returns.
	// It must carry neither '.' nor ':' (tmux silently rewrites both to '_') nor any ASCII control
	// character, DEL, or invalid-UTF-8 byte (tmux silently vis-encodes each into a multi-character
	// escape); either rewrite creates the session under the rewritten name with exit 0, which every
	// exact-match "=<name>" target this package issues would then miss forever. A hub-mode caller
	// gets this for free only if the worktree directory name happens to be clean, so the constraint
	// is enforced rather than assumed — validateToldTmuxIdentity (server.go) refuses such a name at
	// every op boundary, before any tmux round trip.
	SessionName string
	// AnchorPath is the base stateDir joins onto for reed.json/reed.lock, and the cwd every pane is
	// spawned with — passed explicitly as tmux's -c at all three spawn sites (new-session, the
	// header split, and each strand split), never left for tmux to infer from the invoking client's
	// own cwd.
	AnchorPath string
	// WorktreeRoot is what Strand.Worktree is stamped with, and what resolveStrandName substitutes
	// for the <WORKTREE> token.
	WorktreeRoot string
	// LogsDir is the shared per-hub server's runtime log directory.
	LogsDir string
	// RepoName is the header pane's "repo" token, passed through internal/tokenvocab.
	RepoName string
	// HubPath is the header pane's "hub" token, passed through internal/tokenvocab.
	HubPath string
}
