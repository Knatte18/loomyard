// weftwiring.go implements weft worktree spawn and teardown helpers for paired topology operations
// (add/remove, a later batch).
//
// These unexported helpers handle the weft-side lifecycle: creating weft worktrees, pushing to the
// weft remote, and tearing down both the weft worktree and branch.
// Every git operation here runs with an explicit cwd (WeftRepoRoot or WeftWorktreePath), never an
// inherited process cwd, and all but two of them go through gitexec.Run, the checked entry point.
// The two exceptions are this file's bool-returning predicates, weftRepoExists and
// weftBranchExists, which are the whole of internal/fabricengine's pinned raw-site allowance under
// CONSTRAINTS.md's gitexec Checked-Call Invariant: each carries its own //gitexec:raw marker, and
// each is raw because its signature has no error channel, so every outcome — including an
// exec-level failure git never got to answer — must collapse to a bool.
// Every branch argument here is ALWAYS a concrete, already-suffixed weft branch name produced by
// WeftBranchName — this file never derives a branch name itself, so the "-weft" literal never
// appears in this file's Go source (see branchname.go for the single derivation point).
//
// Weft branch model: each weft branch forks from its parent's weft branch (non-orphan, shared
// merge-base), preserving history for future _lyx/raddle/ squash-merge-back. _lyx is isolated by
// pathspec (never merges back), not by orphan topology.
// A detached or unborn warp HEAD aborts the spawn before any creation, ensuring no partial state.
//
// Push honors SkipGit/SkipPush via fabricengine.SyncOptions, fabric's own options type.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// WeftWorktreePath returns the path to a sibling weft worktree with the given slug.
func WeftWorktreePath(l *lyxcwd.Location, slug string) string {
	return weftname.SiblingPath(l.HubPath, slug)
}

// WeftLyxDirFor returns the path to the _lyx directory within a named slug's weft worktree.
// It is the junction target paired by spawn seeds and pairs with WarpLyxLink(slug).
func WeftLyxDirFor(l *lyxcwd.Location, slug string) string {
	return filepath.Join(WeftWorktreePath(l, slug), l.AnchorRel, lyxdirs.LyxDirName)
}

// WeftWarpSlug parses a weft sibling directory name and returns the warp slug it corresponds to.
// It reports whether name ends with weftname.Suffix AND the stripped prefix is non-empty.
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

// weftRepoExists reports whether a weft repo exists and is a valid git
// repository. An unresolvable weft repo root (PrimeName failure) reports
// false, same as an absent directory — either way, there is no weft repo to
// find.
func weftRepoExists(l *lyxcwd.Location) bool {
	weftRepoRoot, err := WeftRepoRoot(l)
	if err != nil {
		return false
	}

	info, err := os.Stat(weftRepoRoot)
	if err != nil || !info.IsDir() {
		return false
	}

	//gitexec:raw — bool-returning predicate: the signature has no error channel, so every outcome must collapse to a bool.
	_, _, exitCode, err := gitexec.RunGit([]string{"rev-parse", "--is-inside-work-tree"}, weftRepoRoot)
	if err != nil {
		return false
	}

	return exitCode == 0
}

// weftBranchExists reports whether the weft branch exists in the weft repo.
// An unresolvable weft repo root reports false, same as a branch that is
// genuinely absent.
func weftBranchExists(l *lyxcwd.Location, branch string) bool {
	weftRepoRoot, err := WeftRepoRoot(l)
	if err != nil {
		return false
	}
	//gitexec:raw — bool-returning predicate: the signature has no error channel, so every outcome must collapse to a bool.
	_, _, exitCode, err := gitexec.RunGit(
		[]string{"rev-parse", "--verify", "refs/heads/" + branch},
		weftRepoRoot,
	)
	if err != nil {
		return false
	}
	return exitCode == 0
}

// createWeftWorktree creates a new weft worktree on branch, forking from
// startPoint to preserve the merge-base for future squash-merge-back.
// On success it records KindWorktreeCreated at the created weft worktree path and, unconditionally,
// KindBranchCreated for branch via AppendRef: the git invocation below always runs `worktree add -b
// <branch>`, so reaching a zero exit means the branch was created, and the adopt-vs-create decision
// (whether the branch already existed) lives in the caller, not here — this function has no way to
// observe it.
// This site does NOT route through the destruction gate: createGitWorktree in destroy.go is the
// gate's minter for the warp side, and the Fabric Destruction Chokepoint Invariant governs
// destruction, not creation, so this hand-written record is by design, not an oversight.
func createWeftWorktree(rec *Mutations, l *lyxcwd.Location, slug, branch, startPoint string) error {
	weftPath := WeftWorktreePath(l, slug)
	weftRepoRoot, err := WeftRepoRoot(l)
	if err != nil {
		return fmt.Errorf("resolve weft repo root: %w", err)
	}
	// Route through containedWorktreeAdd, not a bare `git worktree add`: git resolves and follows a
	// symlink standing at weftPath itself, so a target toggled during the caller's check-then-act window
	// would carry the worktree outside the hub (R5's create-side escape). This is not the gate's
	// createGitWorktree minter — creation is not destruction — but the containment property is identical.
	if err := containedWorktreeAdd(weftRepoRoot, l.HubPath, weftPath, func(worktreePath string) []string {
		return []string{"worktree", "add", "-b", branch, worktreePath, startPoint}
	}); err != nil {
		return fmt.Errorf("create weft worktree %q for branch %q failed: %w", weftPath, branch, err)
	}
	rec.Append(KindWorktreeCreated, weftPath, "")
	rec.AppendRef(KindBranchCreated, branch, refDetail("weft", weftRepoRoot, ""))
	return nil
}

// pushWeftBranch pushes the weft branch to origin, honoring SkipGit/SkipPush.
// On success it records KindBranchPushed for branch via AppendRef.
func pushWeftBranch(rec *Mutations, l *lyxcwd.Location, slug, branch string, opts SyncOptions) error {
	if opts.SkipGit || opts.SkipPush {
		return nil
	}

	weftPath := WeftWorktreePath(l, slug)
	_, err := gitexec.Run(
		[]string{"push", "-u", "origin", branch},
		weftPath,
	)
	if err != nil {
		return fmt.Errorf("push weft branch %q failed: %w", branch, err)
	}
	rec.AppendRef(KindBranchPushed, branch, refDetail("weft", weftPath, "origin"))

	return nil
}

// removeWarpJunction removes every warp junction for slug via the gate's removeLink executor.
// Returns nil if all are absent (idempotent).
// rec is the calling verb's own recorder, threaded straight through to removeJunctionRecords.
func removeWarpJunction(rec *Mutations, l *lyxcwd.Location, slug string, names []string) error {
	return removeJunctionRecords(rec, WorktreePath(l, slug), WarpJunctions(l, slug, names))
}

// removeJunctionRecords removes each junction via the gate's removeLink executor in a best-effort
// loop, continuing past per-junction failures and accumulating errors. Returns nil if empty or all
// absent (idempotent); non-nil error does not mean no junction was removed.
//
// container is the containment boundary every junction in junctions must resolve strictly below —
// a gated site cannot declare containment against a parent it never receives. Both of this
// function's callers have l and slug in scope and pass WorktreePath(l, slug).
// rec is the calling verb's own recorder, passed straight through to removeLink for each junction.
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

// weftTeardownResult carries the remote outcome of removeWeftWorktree's branch deletion, so it can
// reach Remove without passing through the error return, whose meaning must not change.
type weftTeardownResult struct {
	// remoteBranchDeleted reports whether the weft branch's copy on the remote was observably
	// removed.
	remoteBranchDeleted bool
	// remoteBranchError is non-empty when the remote deletion was attempted and did not succeed.
	remoteBranchError string
	// remoteSkippedReason carries the once-per-call reason no remote deletion was attempted at all
	// — today only a weft repo with no origin remote configured.
	remoteSkippedReason string
}

// removeWeftWorktree tears down the weft worktree, optionally its branch, and
// prunes stale worktree entries. Returns the accumulated remote outcome alongside the first error
// encountered among worktree removal, local branch deletion, and prune, or nil if all three succeed
// — the error return's meaning is unchanged, and a remote deletion failure is never accumulated into
// it.
// branchPrefix is the caller's configured warp branch prefix, forwarded to ownedManagedBranch — this
// function has no config in scope of its own.
// rec is the calling verb's own recorder, threaded through to removeGitWorktree, deleteBranch, and
// deleteRemoteBranch below.
func removeWeftWorktree(rec *Mutations, l *lyxcwd.Location, slug, branch string, force, alsoDeleteBranch, remote bool, branchPrefix string) (weftTeardownResult, error) {
	weftPath := WeftWorktreePath(l, slug)
	weftRoot, err := WeftRepoRoot(l)
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
	if err := removeGitWorktree(rec, req, weftRoot); err != nil {
		firstErr = err
	}

	if alsoDeleteBranch {
		branchReq := branchRequest{
			what:      "delete weft branch",
			repoDir:   weftRoot,
			branch:    branch,
			ownership: ownedManagedBranch(l, branchPrefix),
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
				remoteReq := remoteBranchRequest{
					what:      "delete weft branch on remote",
					repoDir:   weftRoot,
					remote:    originRemoteName,
					branch:    branch,
					ownership: ownedManagedBranch(l, branchPrefix),
					dirtiness: dirtyCheckedOutBranch(),
					force:     false,
				}
				deleted, remoteErr := deleteRemoteBranch(rec, remoteReq)
				if remoteErr != nil {
					if refusal, ok := RefusalOf(remoteErr); ok {
						result.remoteBranchError = fmt.Sprintf(
							"gate refused remote deletion of %q: %s", branch, refusal.Reason)
					} else {
						result.remoteBranchError = fmt.Sprintf(
							"delete remote branch %q on %q failed: %v", branch, originRemoteName, remoteErr)
					}
				} else {
					result.remoteBranchDeleted = deleted
				}
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
