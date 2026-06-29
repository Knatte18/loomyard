// Package weftengine owns all git operations into the paired weft worktree (`git -C <weft>`).
// It is one-shot and daemonless, mirroring the board's git-ownership contract.
// Weft provides commit, push, pull, and sync operations scoped to a configurable
// pathspec of directories (e.g., ["_lyx", "_codeguide"]), and manages locks to
// serialize writes and pushes.
package weftengine

import (
	"path/filepath"
)

const (
	commitMessage = "weft sync"
	lockDirName   = ".weft"
	writeLockFile = "weft.write.lock"
	pushLockFile  = "weft.push.lock"
)

// ScopedPathspec returns a slice of pathspec entries, each being the join of relPath
// with each directory in dirs. At relPath == ".", this returns dirs unchanged;
// at relPath == "sub", ["_lyx"] → ["sub/_lyx"].
// It is exported so that weftcli's PersistentPreRunE can call it cross-package.
func ScopedPathspec(relPath string, dirs []string) []string {
	result := make([]string, len(dirs))
	for i, dir := range dirs {
		result[i] = filepath.Join(relPath, dir)
	}
	return result
}
