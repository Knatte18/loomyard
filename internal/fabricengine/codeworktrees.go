// codeworktrees.go holds the path-based reads of a hub's code worktrees for a caller told its paths: list them, open one's fabric handle, and ask whether a pair is complete.

package fabricengine

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// CodeWorktree names one code worktree of a hub.
type CodeWorktree struct {
	// Path is the worktree root, cleaned the way WorktreePath builds a pair's path.
	Path string
	// Anchor is the worktree's anchor dir, where its _lyx/config lives.
	Anchor string
	// Main is true for the hub's main worktree, the prime.
	Main bool
}

// CodeWorktrees lists the code worktrees of the repository the worktree at worktreePath belongs to, the main worktree first.
// A prunable entry and an entry whose directory no longer exists are skipped;
// any other failure to resolve an entry is returned naming its path.
func CodeWorktrees(worktreePath string) ([]CodeWorktree, error) {
	entries, err := List(worktreePath)
	if err != nil {
		return nil, err
	}
	var worktrees []CodeWorktree
	for _, entry := range entries {
		if entry.Prunable {
			continue
		}
		path := filepath.Clean(filepath.FromSlash(entry.Path))
		if requireDir(path) != nil {
			continue
		}
		l, err := lyxcwd.ResolveWorktree(path)
		if err != nil {
			return nil, fmt.Errorf("fabricengine: resolve code worktree %s: %w", path, err)
		}
		worktrees = append(worktrees, CodeWorktree{Path: path, Anchor: l.AnchorPath(), Main: entry.Main})
	}
	return worktrees, nil
}

// OpenCodeWorktree resolves the worktree root at path and opens its fabric handle.
// A path that does not exist returns an *ErrMissingPath naming it.
func OpenCodeWorktree(path string) (*Fabric, error) {
	if err := requireDir(path); err != nil {
		return nil, err
	}
	l, err := lyxcwd.ResolveWorktree(path)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: resolve code worktree %s: %w", path, err)
	}
	return Open(l)
}

// PairCompleteAt reports whether the pair whose worktree root is path is complete, with the reason when it is not.
// A pair Add is still creating, or one a killed Add left partway, reads as not complete.
// A path that does not exist returns an *ErrMissingPath naming it.
func PairCompleteAt(path string) (ok bool, reason string, err error) {
	if err := requireDir(path); err != nil {
		return false, "", err
	}
	l, err := lyxcwd.ResolveWorktree(path)
	if err != nil {
		return false, "", fmt.Errorf("fabricengine: resolve code worktree %s: %w", path, err)
	}
	return PairComplete(l)
}
