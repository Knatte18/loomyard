// onerepository.go reads from git which layout a worktree belongs to, once per worktree for the process.
// It also names the records geometry that layout implies.
// In a one-repository hub a worktree is its own records worktree.
// Its records branch is its code branch, and its records locks sit in its git directory.

package fabricengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

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

// layoutAnswers maps the cleaned path of a worktree on disk to whether it belongs to a one-repository hub.
// A hub's layout is fixed once its marker is committed, so an answer holds for the process;
// fabric's own marker writes clear the map through forgetLayouts.
var layoutAnswers sync.Map

// oneRepositoryAt reports whether the worktree at path belongs to a one-repository hub, reading git alone.
// A path with no `.git` entry is no worktree on disk and answers false without running git or logging.
// That answer is not kept, so a worktree created there later is read then.
// Any other answer is kept for the process, and a later call for the same path runs no git.
// A failed read answers false, is logged at Warn and is not kept,
// so a path that is no repository keeps the two-repository geometry.
func oneRepositoryAt(path string) bool {
	key := filepath.Clean(path)
	if answer, ok := layoutAnswers.Load(key); ok {
		return answer.(bool)
	}
	if _, err := os.Lstat(filepath.Join(key, ".git")); errors.Is(err, fs.ErrNotExist) {
		return false
	}

	answer, err := readOneRepositoryLayout(key)
	if err != nil {
		logger.Warn("fabricengine: reading the layout of a worktree failed; reading it as two repositories", "path", key, "error", err)
		return false
	}
	layoutAnswers.Store(key, answer)
	return answer
}

// forgetLayouts drops every kept layout answer.
// A write that commits or removes a `.lym-anchor` marker calls it, so the next read sees the new layout.
func forgetLayouts() {
	layoutAnswers.Clear()
}

// readOneRepositoryLayout reads from git whether the worktree at path belongs to a one-repository hub.
// A repository whose main worktree is a records sibling with its owner beside it is a two-repository records repository.
// For such a path the answer is false.
// Otherwise the answer is whether `.lym-anchor` is committed at HEAD of the worktree at path.
// The repository's main worktree and the Bolt checkout may have a checked-out branch that lacks the marker.
// For those, a marker committed on the `lym-bolt` branch answers true too.
func readOneRepositoryLayout(path string) (bool, error) {
	geometry, err := dotgit.Read(path)
	if err != nil {
		return false, err
	}

	mainWorktree := filepath.Dir(geometry.CommonDir)
	if owner, isSibling := WeftWarpSlug(filepath.Base(mainWorktree)); isSibling && isDirectory(filepath.Join(filepath.Dir(mainWorktree), owner)) {
		return false, nil
	}

	repo := gitrepo.New(path)
	_, headErr := repo.CurrentSHA()
	switch {
	case headErr == nil:
		_, found, err := committedMarker(path, "HEAD")
		if err != nil {
			return false, err
		}
		if found {
			return true, nil
		}
	case !errors.Is(headErr, gitrepo.ErrNoCommits):
		return false, fmt.Errorf("read HEAD: %w", headErr)
	}

	if filepath.Base(path) != BoltDirName && !sameDirectory(path, mainWorktree) {
		return false, nil
	}
	exists, err := repo.BranchExists(BoltBranchName)
	if err != nil {
		return false, fmt.Errorf("look for the %s branch: %w", BoltBranchName, err)
	}
	if !exists {
		return false, nil
	}
	_, found, err := committedMarker(path, "refs/heads/"+BoltBranchName)
	return found, err
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
