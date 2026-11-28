// detach.go holds the detached checkout of a task pair's code worktree:
// the marker that says the detach is lym's own, the two checkouts, and the state read and restore.

package fabricengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// DetachMarkerPath returns the path of the detach marker of the code worktree l.
func DetachMarkerPath(l *lymcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lymdirs.DotLymDirName, lymdirs.DetachMarkerName)
}

// WriteDetachMarker records that the code worktree l is detached from the task branch branch.
// While the marker stands, reads of the worktree judge it by that branch, and its code side is not pushed.
func WriteDetachMarker(l *lymcwd.Location, branch string) error {
	if err := fsx.AtomicWriteBytes(DetachMarkerPath(l), []byte(branch+"\n")); err != nil {
		return fmt.Errorf("fabricengine: write the detach marker: %w", err)
	}
	return nil
}

// ReadDetachMarker returns the task branch the detach marker of l names, and whether a marker stands.
func ReadDetachMarker(l *lymcwd.Location) (string, bool, error) {
	data, err := os.ReadFile(DetachMarkerPath(l))
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("fabricengine: read the detach marker: %w", err)
	}
	return strings.TrimSpace(string(data)), true, nil
}

// RemoveDetachMarker removes the detach marker of l; an absent one is no error.
func RemoveDetachMarker(l *lymcwd.Location) error {
	if err := os.Remove(DetachMarkerPath(l)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("fabricengine: remove the detach marker: %w", err)
	}
	return nil
}

// detachWayForward words the way out of a detach of l that lym cannot finish itself.
func detachWayForward(l *lymcwd.Location) string {
	return fmt.Sprintf("check the task branch out in %s with plain git, remove %s, then run `lym loom resume` in the task worktree",
		l.WorktreePath(), DetachMarkerPath(l))
}

// markedWorktree returns the location of the code worktree at path and the branch its detach marker names.
// marked is false when no marker stands, and for a path that is no code worktree.
func markedWorktree(path string) (l *lymcwd.Location, branch string, marked bool, err error) {
	l, err = lymcwd.ResolveWorktree(path)
	if err != nil {
		return nil, "", false, fmt.Errorf("fabricengine: resolve %s: %w", path, err)
	}
	if RequireWarpWorktree(l) != nil {
		return l, "", false, nil
	}
	branch, marked, err = ReadDetachMarker(l)
	return l, branch, marked, err
}

// markedBranchTip returns the tip of the branch the standing detach marker of l names.
// A marker naming no existing branch is an error naming the marker and the way forward.
func markedBranchTip(l *lymcwd.Location, branch string) (string, error) {
	tip, err := gitrepo.New(l.WorktreePath()).RefSHA("refs/heads/" + branch)
	if err != nil {
		return "", fmt.Errorf("fabricengine: the detach marker %s names the branch %s, which cannot be read: %w; way forward: %s",
			DetachMarkerPath(l), branch, err, detachWayForward(l))
	}
	return tip, nil
}

// CheckoutDetached checks commit out in the pair's code worktree with a detached HEAD.
// A worktree with uncommitted or untracked files is refused, naming them.
func (f *Fabric) CheckoutDetached(commit string) error {
	return checkoutClean(f.warpPath, "--detach", commit)
}

// CheckoutBranch checks branch out in the pair's code worktree and leaves the branch where it points.
// A worktree with uncommitted or untracked files is refused, naming them.
func (f *Fabric) CheckoutBranch(branch string) error {
	return checkoutClean(f.warpPath, branch, "--")
}

// checkoutClean runs git checkout with args in dir, refusing a worktree that holds any uncommitted or untracked file.
func checkoutClean(dir string, args ...string) error {
	dirty, err := allDirtyPaths(dir)
	if err != nil {
		return err
	}
	if len(dirty) > 0 {
		return fmt.Errorf("fabricengine: %s has uncommitted changes, so nothing is checked out: %s", dir, strings.Join(dirty, ", "))
	}
	if _, err := gitexec.Run(append([]string{"checkout"}, args...), dir); err != nil {
		return fmt.Errorf("fabricengine: git checkout %s in %s: %w", strings.Join(args, " "), dir, err)
	}
	return nil
}

// allDirtyPaths returns the uncommitted and untracked paths of the checkout at dir, relative to it and slash-separated.
func allDirtyPaths(dir string) ([]string, error) {
	out, err := gitexec.Run([]string{"status", "--porcelain", "-z"}, dir)
	if err != nil {
		return nil, fmt.Errorf("check for uncommitted changes in %s: %w", dir, err)
	}
	// Joined under no directory, the paths stay relative to the checkout.
	paths := parseTrackedDirtyPaths("", out)
	for i, path := range paths {
		paths[i] = filepath.ToSlash(path)
	}
	return paths, nil
}

// DetachState is how a code worktree stands towards its task branch.
type DetachState struct {
	// Detached reports a detached HEAD, and Commit is then the commit it holds.
	Detached bool
	Commit   string
	// Marker reports a standing detach marker, and Branch is the task branch it names.
	Marker bool
	Branch string
	// Dirty are the uncommitted and untracked paths of the worktree.
	Dirty []string
	// Restored reports that a restore put the worktree back on Branch.
	Restored bool
}

// ReadDetachState reads the head, the detach marker and the uncommitted paths of the code worktree l.
// It changes nothing and leaves Restored unset.
func ReadDetachState(l *lymcwd.Location) (DetachState, error) {
	repo := gitrepo.New(l.WorktreePath())
	var state DetachState
	var err error
	if _, state.Detached, err = repo.HeadRef(); err != nil {
		return DetachState{}, fmt.Errorf("fabricengine: read the head of %s: %w", l.WorktreePath(), err)
	}
	if state.Detached {
		if state.Commit, err = repo.CurrentSHA(); err != nil {
			return DetachState{}, fmt.Errorf("fabricengine: read the head of %s: %w", l.WorktreePath(), err)
		}
	}
	if state.Branch, state.Marker, err = ReadDetachMarker(l); err != nil {
		return DetachState{}, err
	}
	if state.Dirty, err = allDirtyPaths(l.WorktreePath()); err != nil {
		return DetachState{}, fmt.Errorf("fabricengine: %w", err)
	}
	return state, nil
}

// RestoreDetached puts a detached, clean code worktree with a detach marker back on the marker's branch.
// It then removes the ignored files left in directories the checkout deleted, and removes the marker.
// A worktree already on a branch only loses a stale marker.
// A detached worktree that is dirty or has no marker is left as it is, for the caller to judge from the state.
// A marker naming no existing branch is an error, and nothing moves.
func RestoreDetached(l *lymcwd.Location) (DetachState, error) {
	state, err := ReadDetachState(l)
	if err != nil {
		return DetachState{}, err
	}
	if !state.Detached {
		return state, RemoveDetachMarker(l)
	}
	if !state.Marker || len(state.Dirty) > 0 {
		return state, nil
	}
	if _, err := markedBranchTip(l, state.Branch); err != nil {
		return state, err
	}
	if err := checkoutClean(l.WorktreePath(), state.Branch, "--"); err != nil {
		return state, err
	}
	if _, err := RemoveIgnoredLeftovers(l, state.Commit); err != nil {
		return state, err
	}
	if err := RemoveDetachMarker(l); err != nil {
		return state, err
	}
	state.Restored = true
	return state, nil
}
