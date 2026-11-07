// weftwiring.go implements the weft worktree spawn and teardown helpers for paired topology operations.
//
// These unexported helpers handle the weft-side lifecycle: creating weft worktrees, pushing to the
// weft remote, and tearing down both the weft worktree and branch.
// Every git operation here runs with an explicit cwd (RecordsRepoRoot or RecordsWorktreePath), never an inherited one.
// Operations that spawn git go through gitexec.Run, the checked entry point.
// The bool-returning predicates weftRepoExists and weftBranchExists read through gitrepo.
// Their signatures have no error channel, so every outcome collapses to a bool.
// Every branch argument here is a concrete, already-suffixed weft branch name produced by RecordsBranchName.
// This file never derives a branch name itself; branchname.go holds the single derivation point.
//
// Weft branch model: each weft branch forks from its parent's weft branch, a non-orphan sharing a merge-base.
// _lym is isolated by pathspec, not by orphan topology.
// A detached or unborn warp HEAD aborts the spawn before any creation, so no partial state is left.
//
// Push honors the SkipGit and SkipPush options.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// RecordsWorktreePath returns the path to a sibling weft worktree with the given slug.
func RecordsWorktreePath(l *lymcwd.Location, slug string) string {
	return weftname.SiblingPath(l.HubPath, slug)
}

// WeftLymDirFor returns the path to the _lym directory within a named slug's weft worktree.
// The _lym junction on the code side points at this path.
func WeftLymDirFor(l *lymcwd.Location, slug string) string {
	return filepath.Join(RecordsWorktreePath(l, slug), l.AnchorRel, lymdirs.LymDirName)
}

// WeftWarpSlug parses a weft sibling directory name and returns the warp slug it corresponds to.
// It reports whether name ends with weftname.Suffix and the stripped prefix is non-empty.
func WeftWarpSlug(name string) (slug string, ok bool) {
	if !strings.HasSuffix(name, weftname.Suffix) {
		return "", false
	}
	s := strings.TrimSuffix(name, weftname.Suffix)
	if s == "" {
		return "", false
	}
	return s, true
}

// weftRepoExists reports whether a weft repo exists and is a valid git repository.
// An unresolvable weft repo root reports false, the same as an absent directory.
func weftRepoExists(l *lymcwd.Location) bool {
	weftRepoRoot, err := RecordsRepoRoot(l)
	if err != nil {
		return false
	}

	info, err := os.Stat(weftRepoRoot)
	if err != nil || !info.IsDir() {
		return false
	}

	_, err = gitrepo.New(weftRepoRoot).GitDir()
	return err == nil
}

// weftBranchExists reports whether the weft branch exists in the weft repo.
// An unresolvable weft repo root reports false, the same as a branch that is absent.
func weftBranchExists(l *lymcwd.Location, branch string) bool {
	weftRepoRoot, err := RecordsRepoRoot(l)
	if err != nil {
		return false
	}
	exists, err := gitrepo.New(weftRepoRoot).BranchExists(branch)
	return err == nil && exists
}

// createWeftWorktree creates a new weft worktree on branch, forked from startPoint to share its merge-base.
// On success it records KindWorktreeCreated at the weft worktree path and KindBranchCreated for branch.
// The git call always passes `-b`, so a zero exit means this call created the branch.
// Whether the branch already existed is the caller's adopt-or-create decision, which this function cannot observe.
// Creation is not destruction, so this record is written by hand rather than through the destruction gate.
func createWeftWorktree(rec *Mutations, l *lymcwd.Location, slug, branch, startPoint string) error {
	weftPath := RecordsWorktreePath(l, slug)
	weftRepoRoot, err := RecordsRepoRoot(l)
	if err != nil {
		return fmt.Errorf("resolve weft repo root: %w", err)
	}
	// Git resolves and follows a symlink standing at weftPath itself.
	// A target swapped in during the caller's check-then-act window would carry the worktree outside the hub.
	// The add goes through containedWorktreeAdd, which checks that case; a bare `git worktree add` does not.
	if err := containedWorktreeAdd(weftRepoRoot, l.HubPath, weftPath, func(worktreePath string) []string {
		return []string{"worktree", "add", "-b", branch, worktreePath, startPoint}
	}); err != nil {
		return fmt.Errorf("create weft worktree %q for branch %q failed: %w", weftPath, branch, err)
	}
	rec.Append(KindWorktreeCreated, weftPath, "")
	rec.AppendRef(KindBranchCreated, branch, refDetail("records", weftRepoRoot, ""))
	if _, err := ensureWeftLockDirAt(weftPath); err != nil {
		return fmt.Errorf("create weft lock dir in %q: %w", weftPath, err)
	}
	return nil
}

// removeWarpJunction removes every warp junction for slug.
// It returns nil when all of them are already absent, so a repeat call is safe.
// rec is the calling verb's own recorder, passed through unchanged.
func removeWarpJunction(rec *Mutations, l *lymcwd.Location, slug string, names []string) error {
	return removeJunctionRecords(rec, WorktreePath(l, slug), WarpJunctions(l, slug, names))
}

// removeJunctionRecords removes each junction in a best-effort loop.
// It continues past per-junction failures and accumulates their errors.
// It returns nil if empty or all absent, so a repeat call is safe.
// A non-nil error does not mean that no junction was removed.
//
// container is the containment boundary that every junction must resolve strictly below.
// It is passed in because the gated site can only check containment against a parent it receives.
// Callers pass WorktreePath(l, slug).
// rec is the calling verb's own recorder, passed through to each removal.
func removeJunctionRecords(rec *Mutations, container string, junctions []WarpJunction) error {
	links := make([]string, len(junctions))
	for i, j := range junctions {
		links[i] = j.Link
	}

	var errs []error
	for _, j := range junctions {
		req := pathRequest{
			what:      "remove warp junction",
			container: container,
			target:    j.Link,
			slug:      nil,
			ownership: ownedWiredJunction(links, j.Target),
			dirtiness: dirtinessNA("a junction holds no content; the weft target it points at is untouched"),
			force:     false,
		}
		if err := removeLink(rec, req); err != nil {
			errs = append(errs, fmt.Errorf("remove warp junction %s: %w", j.Link, err))
		}
	}
	return errors.Join(errs...)
}

// weftTeardownResult carries the remote outcome of deleting the weft branch on origin.
// It travels beside the error return, so a remote outcome never changes what that error means.
type weftTeardownResult struct {
	// remoteBranchDeleted reports whether the weft branch's copy on the remote was observably
	// removed.
	remoteBranchDeleted bool
	// remoteBranchError is non-empty when the remote deletion was attempted and did not succeed.
	remoteBranchError string
	// remoteSkippedReason is non-empty when no remote deletion was attempted at all;
	// it gives the cause.
	remoteSkippedReason string
	// remoteBranchKeptReason is non-empty when origin moved the branch after its tip was read; the lease kept it.
	remoteBranchKeptReason string
}

// removeWeftWorktree tears down the weft worktree, optionally its branch, and prunes stale worktree entries.
// It returns the remote outcome with the first error among worktree removal, local branch deletion and prune.
// The error is nil when all three succeed, and a remote deletion failure never enters it.
// leaseSHA is the origin tip that the remote deletion is leased on, the tip the caller's archive read.
// With an origin configured, an empty leaseSHA means origin held no copy of the branch, and nothing is deleted there.
// A branch that origin has moved since is kept and reported in the result, not as a failure.
// rec is the calling verb's own recorder, passed on to every removal this function makes.
// A worktree directory that git deregistered but left on disk is finished by the leftover seam.
func removeWeftWorktree(rec *Mutations, l *lymcwd.Location, slug, branch string, force, alsoDeleteBranch, remote bool, leaseSHA string, leftover leftoverSeam) (weftTeardownResult, error) {
	weftPath := RecordsWorktreePath(l, slug)
	weftRoot, err := RecordsRepoRoot(l)
	if err != nil {
		return weftTeardownResult{}, fmt.Errorf("resolve weft repo root: %w", err)
	}

	var firstErr error
	var result weftTeardownResult

	req := pathRequest{
		what:      "remove weft worktree",
		container: l.HubPath,
		target:    weftPath,
		slug:      nil,
		ownership: ownedRegisteredLinkedWorktree(weftRoot),
		dirtiness: dirtyScopeAll(),
		force:     force,
	}
	if err := finishGitWorktreeRemoval(rec, weftRoot, l.HubPath, weftPath, removeGitWorktree(rec, req, weftRoot), leftover); err != nil {
		firstErr = err
	}

	if alsoDeleteBranch {
		branchReq := branchRequest{
			what:      "delete weft branch",
			repoDir:   weftRoot,
			branch:    branch,
			ownership: ownedManagedBranch(l),
			dirtiness: dirtyCheckedOutBranch(),
			force:     false,
		}
		deleteErr := deleteBranch(rec, branchReq)
		if deleteErr != nil && firstErr == nil {
			firstErr = deleteErr
		}

		if deleteErr == nil && remote {
			if _, urlErr := gitrepo.New(weftRoot).RemoteURL(originRemoteName); urlErr != nil {
				result.remoteSkippedReason = fmt.Sprintf(
					"no remote deletion attempted: the weft repo has no %q remote configured: %v",
					originRemoteName, urlErr)
			} else {
				entry := CleanupBranchEntry{Branch: branch}
				deleteWeftBranchOnRemote(rec, l, branch, weftRoot, leaseSHA, &entry)
				result.remoteBranchDeleted = entry.RemoteDeleted
				result.remoteBranchError = entry.RemoteError
				result.remoteBranchKeptReason = entry.RemoteKeptReason
			}
		}
	}

	if _, err := gitexec.Run([]string{"worktree", "prune"}, weftRoot); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}

	return result, firstErr
}
