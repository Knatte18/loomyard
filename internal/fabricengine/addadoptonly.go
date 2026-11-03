// addadoptonly.go holds what AddOptions.AdoptOnly adds to Add: the typed answers of a pair that cannot be re-made or a path that blocks it, the reuse test for a records worktree that already stands, and the prune of stale worktree registrations a caller runs before re-making a pair.

package fabricengine

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// ErrPairNotLive is matched with errors.Is against the *ErrNotLive an adopt-only Add returns when a branch of the pair is gone for good,
// so a caller tests the closed set by this sentinel and reads which branch with errors.As.
var ErrPairNotLive = errors.New("fabricengine: the pair is not live")

// ErrNotLive is an adopt-only Add's refusal when a branch of the pair is not live: it neither stands locally nor on origin, or origin's weft tip is covered by an archive tag.
// It is returned before any mutation, and it matches ErrPairNotLive under errors.Is.
type ErrNotLive struct {
	// Branch is the branch that is not live, prefix and suffix included.
	Branch string
	// Records is true when Branch is the weft branch, false for the task branch.
	Records bool
}

// Error implements the error interface, naming the branch and which side of the pair it is.
func (e *ErrNotLive) Error() string {
	if e.Records {
		return fmt.Sprintf("records branch %q is not live: it is absent locally and on origin, or origin's tip is covered by an archive tag, so the pair cannot be re-made over it", e.Branch)
	}
	return fmt.Sprintf("task branch %q is not live: it is absent locally and on origin, so the pair cannot be re-made over it", e.Branch)
}

// Is reports whether target is ErrPairNotLive.
func (e *ErrNotLive) Is(target error) bool { return target == ErrPairNotLive }

// ErrRecordsWorktreeNotPair is Add's refusal when the records worktree directory of the new pair stands without being a registered worktree on the pair's weft branch.
type ErrRecordsWorktreeNotPair struct {
	// Path is the directory that stands.
	Path string
}

// Error implements the error interface, naming the directory.
func (e *ErrRecordsWorktreeNotPair) Error() string {
	return fmt.Sprintf("weft worktree directory already exists: %s", e.Path)
}

// ErrAdoptLocalCheckedOut is an AdoptLocal Add's refusal when a worktree holds the branch it would adopt:
// the warp branch checked out in a worktree, or the weft branch checked out in a records worktree.
type ErrAdoptLocalCheckedOut struct {
	// Branch is the branch another worktree holds.
	Branch string
	// Path is the worktree that holds it.
	Path string
	// Records is true when Branch is the weft branch and Path a records worktree.
	Records bool

	slug string
}

// Error implements the error interface, naming the worktree and the way forward that releases the branch.
func (e *ErrAdoptLocalCheckedOut) Error() string {
	if e.Records {
		return fmt.Sprintf("--adopt-local: records branch %q is checked out in the records worktree %s; way forward: in %s switch off it, back to its own pair's records branch with `git switch <records branch>`, or detached with `git switch --detach` when it belongs to no pair, then re-run `lym fabric add %s --adopt-local`", e.Branch, e.Path, e.Path, e.slug)
	}
	return fmt.Sprintf("--adopt-local: branch %q is checked out in %s; way forward: run `lym fabric checkout <other branch>` in %s to move it off the branch, then re-run `lym fabric add %s --adopt-local`", e.Branch, e.Path, e.Path, e.slug)
}

// ErrTaskPathTaken is Add's refusal when the task worktree's directory already exists.
type ErrTaskPathTaken struct {
	// Path is the directory that exists.
	Path string
}

// Error implements the error interface, naming the directory and the ways out.
func (e *ErrTaskPathTaken) Error() string {
	// Name the recovery path, since a directory here is not always a live pair.
	// An empty leftover stranded by an interrupted remove/reconcile is invisible to `lym fabric list`/`prune` (they enumerate only git-registered worktrees), so an operator hitting one otherwise has no clue why the slug is blocked.
	// If it IS a live pair, a different slug is the answer.
	return fmt.Sprintf(
		"worktree directory %q already exists; if it is a live pair, use a different slug; if it is a leftover directory from an interrupted remove/reconcile (which `lym fabric list` and `prune` cannot see, as they list only git-registered worktrees), remove the directory and retry",
		e.Path)
}

// PruneRegistrations runs `git worktree prune` in the code repository of l and in its records repository, dropping every registration whose worktree directory is gone.
// A caller re-making a pair whose worktree directories were deleted by hand runs it first, so a stale registration holds no branch.
func PruneRegistrations(l *lymcwd.Location) error {
	recordsRoot, err := RecordsRepoRoot(l)
	if err != nil {
		return fmt.Errorf("resolve weft repo root: %w", err)
	}
	for _, dir := range []string{l.WorktreePath(), recordsRoot} {
		if _, err := gitexec.Run([]string{"worktree", "prune"}, dir); err != nil {
			return fmt.Errorf("prune worktree registrations in %s: %w", dir, err)
		}
	}
	return nil
}

// reusableRecordsWorktree reports whether the directory at path is a registered, not prunable worktree of the records repository at recordsRoot, checked out on branch.
func reusableRecordsWorktree(recordsRoot, path, branch string) (bool, error) {
	entries, err := List(recordsRoot)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if !entry.Prunable && entry.Branch == branch && samePath(entry.Path, path) {
			return true, nil
		}
	}
	return false, nil
}
