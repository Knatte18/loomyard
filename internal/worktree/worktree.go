// Package worktree manages the lifecycle of git worktrees in the Loomyard container
// layout: adding new worktrees, listing existing ones, and removing them with
// junction/symlink cleanup. It exposes the Worktree facade (the entry point for
// all operations) together with RunCLI, the subcommand router, so the lyx
// binary stays a thin module dispatcher.
//
// Configuration is resolved cwd-authoritatively via internal/config; the package
// never reads config files or knows their on-disk layout itself.
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
