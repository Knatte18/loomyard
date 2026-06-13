// worktree.go — the Worktree facade, the entry point for worktree operations.
//
// The worktree module manages the lifecycle of git worktrees: add new worktrees,
// list existing ones, and remove worktrees. This file defines the main Worktree
// struct and its constructor.

package worktree

// Worktree is the high-level facade over worktree operations.
// It holds configuration needed by all worktree methods.
type Worktree struct {
	cfg Config
}

// New returns a Worktree operating with the given config.
func New(cfg Config) *Worktree {
	return &Worktree{
		cfg: cfg,
	}
}
