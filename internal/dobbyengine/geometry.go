// geometry.go declares the absolute paths and names the hub wiring tells dobby, and the pass exit codes dobby reads from a batten run.

package dobbyengine

const (
	// PassExitOwnedElsewhere is the exit code of a `lym batten run` pass that refused because another machine owns the run.
	PassExitOwnedElsewhere = 77
	// PassExitCovered is the exit code of a `lym batten run` pass that refused because a pause covers the run.
	PassExitCovered = 78
	// PassExitContinueRefused is the exit code of a continue pass that refused because the run is not one dobby may continue.
	PassExitContinueRefused = 79
)

const (
	// LockFileName is the daemon's lock file in the worktree's `.lym/beam/` directory.
	LockFileName = "dobby.lock"
	// StateFileName is the State record's file in the worktree's `.lym/beam/` directory.
	StateFileName = "state.json"
	// TidyDirName is the directory of the board-tidy reports in the worktree's `.lym/beam/` directory.
	TidyDirName = "tidy"
)

// Geometry is every path and name dobby needs, absolute and told by the hub wiring.
// The package derives none of them.
type Geometry struct {
	// HubRoot is the hub directory.
	HubRoot string
	// BoardDir is the hub's board directory.
	BoardDir string
	// PrimeAnchor is the prime worktree's anchor.
	PrimeAnchor string
	// WorktreeRoot is the root of the beam's worktree.
	WorktreeRoot string
	// WorktreeName is the beam worktree's directory name.
	WorktreeName string
	// Slug is the beam's slug, empty for the prime, the root beam.
	Slug string
	// LockPath is the daemon's lock file under the worktree's `.lym/beam/` directory.
	LockPath string
	// StatePath is the State record under the worktree's `.lym/beam/` directory.
	StatePath string
	// LogPath is the daemon's log file under the worktree's `.lym/beam/` directory.
	LogPath string
	// TidyDir is the directory of the board-tidy reports under the worktree's `.lym/beam/` directory, each named by its UTC timestamp.
	TidyDir string
	// PrimeRunsDir is the prime's batten runs directory.
	PrimeRunsDir string
	// LymExecutable is the absolute path of the lym binary dobby re-executes and spawns passes with.
	LymExecutable string
	// MoveRecords are the paths of the anchor-move records whose standing keeps this dobby from scheduling: the record on its own worktree's records worktree and the one on the prime's.
	// An empty list gates nothing.
	MoveRecords []string
}
