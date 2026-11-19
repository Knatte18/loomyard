// onerepository.go reads from git which layout a worktree belongs to.
// It also names the records geometry that layout implies.
// In a one-repository hub a worktree is its own records worktree.
// Its records branch is its code branch, and its records locks sit in its git directory.

package fabricengine

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/dotgit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

const (
	// BoltBranchName is the orphan branch holding the board in a one-repository hub.
	BoltBranchName = "lym-bolt"
	// recordsLockDirName is the directory in a one-repository worktree's git directory that holds its records locks.
	recordsLockDirName = "lym-locks"
)

// oneRepositoryAt reports whether the worktree at path belongs to a one-repository hub, reading git alone.
// A repository whose main worktree is a records sibling with its owner beside it is a two-repository records repository.
// For such a path the answer is false.
// Otherwise the answer is whether `.lym-anchor` is committed at HEAD of the worktree at path.
// The repository's main worktree and the Bolt checkout may have a checked-out branch that lacks the marker.
// For those, a marker committed on the `lym-bolt` branch answers true too.
// A failed read answers false and is logged at Warn, so a path that is no repository keeps the two-repository geometry.
func oneRepositoryAt(path string) bool {
	geometry, err := dotgit.Read(path)
	if err != nil {
		logger.Warn("fabricengine: reading the layout of a worktree failed; reading it as two repositories", "path", path, "error", err)
		return false
	}

	mainWorktree := filepath.Dir(geometry.CommonDir)
	if owner, isSibling := WeftWarpSlug(filepath.Base(mainWorktree)); isSibling && isDirectory(filepath.Join(filepath.Dir(mainWorktree), owner)) {
		return false
	}

	repo := gitrepo.New(path)
	_, headErr := repo.CurrentSHA()
	switch {
	case headErr == nil:
		_, found, err := committedMarker(path, "HEAD")
		if err != nil {
			logger.Warn("fabricengine: reading the committed anchor marker failed; reading the layout as two repositories", "path", path, "error", err)
			return false
		}
		if found {
			return true
		}
	case !errors.Is(headErr, gitrepo.ErrNoCommits):
		logger.Warn("fabricengine: reading HEAD failed; reading the layout as two repositories", "path", path, "error", headErr)
		return false
	}

	if filepath.Base(path) != BoltDirName && !sameDirectory(path, mainWorktree) {
		return false
	}
	exists, err := repo.BranchExists(BoltBranchName)
	if err != nil {
		logger.Warn("fabricengine: looking for the Bolt branch failed; reading the layout as two repositories", "path", path, "error", err)
		return false
	}
	if !exists {
		return false
	}
	_, found, err := committedMarker(path, "refs/heads/"+BoltBranchName)
	if err != nil {
		logger.Warn("fabricengine: reading the Bolt branch's anchor marker failed; reading the layout as two repositories", "path", path, "error", err)
		return false
	}
	return found
}

// recordsBranchAt returns the records branch of branch for the worktree at worktreePath:
// branch itself in a one-repository hub, and the paired records branch name otherwise.
func recordsBranchAt(worktreePath, branch string) string {
	if oneRepositoryAt(worktreePath) {
		return branch
	}
	return RecordsBranchName(branch)
}

// recordsBranchIn returns the records branch of branch for l's worktree, as recordsBranchAt names it.
func recordsBranchIn(l *lymcwd.Location, branch string) string {
	return recordsBranchAt(l.WorktreePath(), branch)
}

// pairSlugOfRecords returns the slug of the pair whose records worktree is at recordsPath.
// In a two-repository hub it is the sibling's name without the records suffix.
// In a one-repository hub it is the worktree's own name.
// It reports false for a path that is neither.
func pairSlugOfRecords(recordsPath string) (slug string, ok bool) {
	if oneRepositoryAt(recordsPath) {
		return filepath.Base(recordsPath), true
	}
	return WeftWarpSlug(filepath.Base(recordsPath))
}

// recordsLockDir returns the directory holding the records locks of the worktree at path.
// It also reports whether that worktree belongs to a one-repository hub.
// A one-repository worktree keeps them in its git directory, which needs no exclude line.
// Any other path, and a git directory that cannot be read, gets the lock directory inside the worktree.
func recordsLockDir(path string) (dir string, oneRepository bool) {
	if oneRepositoryAt(path) {
		if geometry, err := dotgit.Read(path); err == nil {
			return filepath.Join(geometry.GitDir, recordsLockDirName), true
		}
	}
	return filepath.Join(path, weftLockDirName), false
}

// isDirectory reports whether path is an existing directory.
func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// sameDirectory reports whether a and b are the same existing directory, whatever symlinks lead to it.
func sameDirectory(a, b string) bool {
	aInfo, aErr := os.Stat(a)
	bInfo, bErr := os.Stat(b)
	return aErr == nil && bErr == nil && os.SameFile(aInfo, bInfo)
}
