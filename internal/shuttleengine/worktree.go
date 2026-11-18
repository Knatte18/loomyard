// worktree.go declares Worktree, the geometry a launching engine is told about the worktree its sessions run in.

package shuttleengine

// WorktreeKind names the role a worktree plays in its hub.
type WorktreeKind string

const (
	// WorktreeTask is a task worktree: a pair that is neither the hub's prime nor a beam.
	WorktreeTask WorktreeKind = "task"
	// WorktreePrime is the hub's prime worktree.
	WorktreePrime WorktreeKind = "prime"
	// WorktreeBeam is a beam's worktree.
	WorktreeBeam WorktreeKind = "beam"
)

// Worktree is what a launching engine is told about the worktree its sessions run in.
// It is told geometry: the engine derives none of it and never imports the package that resolves a worktree.
// The zero value is "told no worktree", which an engine reads as a launch outside any hub worktree.
type Worktree struct {
	// Name is the worktree's name in its hub.
	Name string
	// Kind is whether the worktree is the prime, a beam or a task worktree.
	Kind WorktreeKind
	// AccountDir is the absolute path of the hub's machine-local directory for an engine's account file and usage records.
	AccountDir string
	// Names reads the names of the hub's current worktrees, the prime included.
	// It is nil when the worktree is told none.
	Names func() ([]string, error)
}
