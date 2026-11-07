// geometry.go declares Geometry, the paths and identity strings reed is given at construction.
// It also declares the button types ButtonContext and ButtonBeam.
// The package's constructor and methods live in other files, so this file declares types only.

package reedengine

import "gopkg.in/yaml.v3"

// Geometry is the set of paths and identity strings that reed is given once, at construction.
// reed validates no field and derives none from the others.
// The caller must populate every field with a usable absolute path;
// SocketKey takes a socket-safe key instead.
// Several fields are empty or nil in standalone mode, and each field comment says so.
type Geometry struct {
	// SocketKey is the tmux -L socket name.
	// It must carry no path separator: tmux resolves -L as a filename under its per-user socket directory,
	// so a separator makes it a path whose parent does not exist.
	// tmux answers with a stderr line and exit 0, indistinguishable from a slow boot.
	// The name derivation replaces separators;
	// `validateToldTmuxIdentity` (server.go) is the backstop for a caller that builds this field another way.
	SocketKey string
	// SessionName is the tmux session name.
	// It must avoid tmux's three rewrite classes.
	// tmux silently rewrites '.' and ':' to '_' and doubles '\' to "\\".
	// It vis-encodes any ASCII control character, DEL, or invalid-UTF-8 byte into a multi-character escape.
	// Any of these creates the session under the rewritten name with exit 0,
	// so every exact-match "=<name>" target this package issues would miss it forever.
	// Hub mode gets a clean name only when the worktree directory name is clean,
	// so the constraint is enforced rather than assumed.
	// `validateToldTmuxIdentity` (server.go) refuses such a name at every op boundary, before any tmux round trip.
	SessionName string
	// AnchorPath is the base stateDir joins onto for reed.json/reed.lock.
	AnchorPath string
	// PaneCwd is the cwd every tmux pane is spawned with.
	// It equals AnchorPath in hub mode and the standalone target directory in standalone mode.
	// An empty PaneCwd must never silently mean AnchorPath.
	// A caller that forgets the field would spawn panes in the wrong directory, with nothing to catch it.
	PaneCwd string
	// WorktreeRoot is the value that every strand's Worktree field is stamped with.
	WorktreeRoot string
	// LogsDir is the shared per-hub server's runtime log directory.
	LogsDir string
	// DiscoverSignalPath is the hub-level file a cold session boot touches to wake the watchdog daemon's discovery loop.
	// Shutting the server down also writes the discovery stop marker into it.
	// It is empty in standalone mode, where nothing is touched.
	DiscoverSignalPath string
	// WorktreeName names the worktree in the strand-name refusals.
	WorktreeName string
	// HubPath names the hub in the told-identity error messages.
	HubPath string
	// NameShortname is the shortname every strand name starts with; empty means the hub records none.
	NameShortname string
	// NameSlug is the task worktree's slug segment of a strand name;
	// empty in the prime and in a standalone run.
	NameSlug string
	// PauseRecordPath is the hub's pause record, which a resume reads to leave a covered pair's strands down.
	// It is empty in standalone mode, where no pause covers anything.
	PauseRecordPath string
	// ParentName is the full name of the session that spawned this worktree's default run;
	// empty when none is recorded.
	// The value is a session name, not a layout guid.
	ParentName string
	// SpawnOrder lists the hub's worktrees in spawn order, for the revival of a restarted server's sessions (revive.go).
	// It is nil in standalone mode, and called only when a revival is due, so building an engine costs nothing.
	SpawnOrder func() ([]ReviveEntry, error)
	// Buttons supplies the session's status-bar button context.
	// It is nil in standalone mode, where the row shows follow alone.
	// It is called only by the status pins, at boot and attach.
	Buttons func() (ButtonContext, error)
	// PaneEnv supplies the environment variables every strand pane exports before its launch command, name to value:
	// the hub's env config module with this machine's override.
	// It is nil in standalone mode, where a pane exports none.
	// It is called at every strand launch, so a changed value reaches the next launch or relaunch.
	PaneEnv func() (map[string]string, error)
}

// ReviveEntry describes one worktree of a hub, for bringing its session back after a server restart.
type ReviveEntry struct {
	// Worktree names the worktree; the entry that matches the booting worktree is the booter's own.
	Worktree string
	// Revive brings that worktree's recorded session back when it is not live, and reports whether it did.
	Revive func() (revived bool, err error)
}

// ButtonContext is the status-bar button context that a session supplies to reed.
type ButtonContext struct {
	// Place is the kind of session the row is rendered for.
	Place ButtonPlace
	// Slug, Worktree and Session are the cheap placeholder values:
	// the worktree's name, its anchor path and its tmux session name.
	Slug     string
	Worktree string
	Session  string
	// MachineButtons is desktop's buttons node, appended after reed's buttons of the same place;
	// zero when the machine adds none.
	MachineButtons yaml.Node
	// Beam resolves the {beam} placeholder and is called only when the render needs it.
	Beam func() (ButtonBeam, error)
}

// ButtonBeam holds the values that a session's {beam} placeholder and run directory name resolve to.
type ButtonBeam struct {
	// Anchor is the beam's anchor path, empty when that directory is absent on this machine.
	Anchor string
	// Name is the beam's worktree name.
	Name string
	// PrimeAnchor is the prime's anchor path, the run directory of a task's button when Anchor is empty.
	PrimeAnchor string
}
