// addadoptonly.go holds the adopt-only part of pair creation.
// Its typed refusals cover a pair that cannot be re-made and a path that blocks it.
// It also holds the reuse test for a standing records worktree and the prune of stale worktree registrations.

package fabricengine

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// ErrPairNotLive is the sentinel that errors.Is matches when a branch of a pair is gone for good.
// A caller tests this one value for every such branch, and errors.As reads the branch from the matching error.
var ErrPairNotLive = errors.New("fabricengine: the pair is not live")

// ErrNotLive is the refusal of an adopt-only re-make of a pair whose branch is not live.
// The branch is absent locally and on origin, or, for the weft branch, origin's tip is covered by an archive tag.
// It is returned before any mutation, and errors.Is matches it against ErrPairNotLive.
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

// Is reports whether target is ErrPairNotLive, so errors.Is tests every not-live branch with one sentinel.
func (e *ErrNotLive) Is(target error) bool { return target == ErrPairNotLive }

// ErrRecordsWorktreeNotPair is the refusal to create a pair whose records worktree directory already stands:
// the directory is not a registered worktree on the pair's weft branch.
type ErrRecordsWorktreeNotPair struct {
	// Path is the directory that stands.
	Path string
}

// Error implements the error interface, naming the directory.
func (e *ErrRecordsWorktreeNotPair) Error() string {
	return fmt.Sprintf("weft worktree directory already exists: %s", e.Path)
}

// ErrAdoptLocalCheckedOut is the refusal of an adopt-local pair when a worktree holds the branch to adopt:
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

// ErrTaskPathTaken is the refusal to create a pair when the task worktree's directory already exists.
type ErrTaskPathTaken struct {
	// Path is the directory that exists.
	Path string
}

// Error implements the error interface, naming the directory and the ways out.
func (e *ErrTaskPathTaken) Error() string {
	// A directory here is not always a live pair, so the message offers a recovery path.
	// A leftover, such as a directory a late writer refilled after a remove, is not a worktree.
	// `lym fabric list` enumerates only git-registered worktrees, so it cannot show a leftover.
	// `lym fabric prune` reports leftovers, so the message names it.
	// If it is a live pair, a different slug is the answer.
	return fmt.Sprintf(
		"worktree directory %q already exists; if it is a live pair, use a different slug; if it is a leftover directory that is not a worktree, which `lym fabric prune` reports, save anything worth keeping from it, delete it and retry",
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
