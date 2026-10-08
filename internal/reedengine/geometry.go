// geometry.go declares Geometry, the struct reed is told its coordinates through.
// It declares the type only — New and every method stay in their existing files (lock.go,
// lifecycle.go, strand.go); this file adds no constructor, no validator, and no
// default.

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
	// It must carry none of tmux's three rewrite classes: '.' or ':' (silently rewritten to '_'),
	// '\' (silently doubled to "\\"), or any ASCII control character, DEL, or invalid-UTF-8 byte
	// (silently vis-encoded into a multi-character escape).
	// Any of the three creates the session under the rewritten name with exit 0, which every
	// exact-match "=<name>" target this package issues would then miss forever. A hub-mode caller
	// gets this for free only if the worktree directory name happens to be clean, so the constraint
	// is enforced rather than assumed — validateToldTmuxIdentity (server.go) refuses such a name at
	// every op boundary, before any tmux round trip.
	SessionName string
	// AnchorPath is the base stateDir joins onto for reed.json/reed.lock.
	AnchorPath string
	// PaneCwd is the cwd every tmux pane is spawned with. It equals AnchorPath in hub mode and the
	// standalone target directory in standalone mode. There is deliberately no zero-value fallback:
	// an empty PaneCwd must never silently mean AnchorPath, or a caller that forgets the field spawns
	// panes in the wrong directory with nothing to catch it.
	PaneCwd string
	// WorktreeRoot is what Strand.Worktree is stamped with.
	WorktreeRoot string
	// LogsDir is the shared per-hub server's runtime log directory.
	LogsDir string
	// WorktreeName names the worktree in the strand-name refusals.
	WorktreeName string
	// HubPath names the hub in the told-identity error messages.
	HubPath string
	// NameShortname is the shortname every strand name starts with; empty means the hub records none.
	NameShortname string
	// NameSlug is the task worktree's slug segment of a strand name;
	// empty in the prime and in a standalone run.
	NameSlug string
	// ParentName is the full name of the session that spawned this worktree's default run;
	// empty when none is recorded.
	// It is unrelated to Strand.Parent, which is a layout guid.
	ParentName string
}
