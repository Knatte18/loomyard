// add.go implements the transactional Add: it creates the warp worktree, portal, and launchers,
// wires junctions, records and commits the pair's parent-branch provenance, then pushes last,
// performing a best-effort full rollback on any post-creation failure so a partial worktree PAIR
// is never left behind.
// One residue the rollback cannot always clear is the warp branch this Add created: the gate deletes
// it only when it can prove the branch is fabric's (a non-empty branch_prefix, or a -weft weft
// branch), so under the default empty prefix the bare-slug warp branch is left behind — see
// rollbackAdd for why, and the "already exists" remedy Add's own re-add error names for the recovery.
// The weft side always uses the suffixed branch produced by WeftBranchName.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// AddOptions controls optional behaviour for Add.
// It is an alias of SyncOptions (same SkipGit/SkipPush field shape as warp's own AddOptions) rather
// than a distinct type, so Add can pass opts straight through to pushWeftBranch, which already
// takes SyncOptions.
// Tests pass these directly instead of relying on environment variables, which makes t.Parallel()
// safe.
type AddOptions = SyncOptions

// AddResult contains the result of successfully adding a new worktree pair.
// It embeds MutationRecord, which carries the mutation record accumulated over the call.
type AddResult struct {
	MutationRecord
	Slug   string `json:"slug"`
	Branch string `json:"branch"`
	Path   string `json:"path"`
	Pushed bool   `json:"pushed"`
}

// Add creates a new paired warp and weft git worktree with the given slug.
// It validates the slug, creates both worktrees, wires junctions, records and commits the pair's
// parent-branch provenance, and pushes branches, rolling back all changes on any failure.
func (t *Topology) Add(l *lyxcwd.Location, slug string, opts AddOptions) (res AddResult, err error) {
	rec := NewMutations(l.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()

	// (0) Slug validation, shared with Remove via slug.go's single validator.
	if err := validateWorktreeSlug(slug, t.cfg.Dirs()); err != nil {
		return AddResult{}, err
	}

	dirty, _, err := worktreeDirty(scopeTracked, l.WorktreePath())
	if err != nil {
		return AddResult{}, fmt.Errorf("read warp worktree status: %w", err)
	}
	if dirty {
		return AddResult{}, fmt.Errorf("source worktree has uncommitted changes")
	}

	warpBranch := t.cfg.BranchPrefix + slug
	weftBranch := WeftBranchName(warpBranch)

	// rev-parse --verify is a mixed probe: its exit path is an answer ("the branch does not
	// exist"), recovered via errors.As, while its exec path returns a real error.
	_, verifyErr := gitexec.Run([]string{"rev-parse", "--verify", "refs/heads/" + warpBranch}, l.WorktreePath())
	if verifyErr == nil {
		// Name the way forward: Remove deliberately leaves the warp branch
		// behind (it may carry unmerged work), so remove-then-re-add of the
		// same slug lands here — a bare "already exists" gives the operator no
		// path out of that everyday cycle.
		return AddResult{}, fmt.Errorf(
			"branch %q already exists; switch a pair onto it with \"lyx fabric checkout %s\", or delete it first with \"git branch -D %s\" if it is a leftover from a removed pair",
			warpBranch, warpBranch, warpBranch,
		)
	}
	var verifyGitErr *gitexec.GitError
	if !errors.As(verifyErr, &verifyGitErr) {
		return AddResult{}, fmt.Errorf("check whether warp branch %q exists: %w", warpBranch, verifyErr)
	}

	target := WorktreePath(l, slug)
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		// Name the recovery path, since a directory here is not always a live pair: an empty leftover
		// stranded by an interrupted remove/reconcile is invisible to `lyx fabric list`/`prune`
		// (they enumerate only git-registered worktrees), so an operator hitting one otherwise has no
		// clue why the slug is blocked. If it IS a live pair, a different slug is the answer.
		return AddResult{}, fmt.Errorf(
			"worktree directory %q already exists; if it is a live pair, use a different slug; if it is a leftover directory from an interrupted remove/reconcile (which `lyx fabric list` and `prune` cannot see, as they list only git-registered worktrees), remove the directory and retry",
			target)
	}

	stdout, err := gitexec.Run([]string{"remote"}, l.WorktreePath())
	if err != nil {
		return AddResult{}, fmt.Errorf("list warp remotes: %w", err)
	}
	if strings.TrimSpace(stdout) == "" {
		return AddResult{}, fmt.Errorf("no remote configured")
	}

	if !weftRepoExists(l) {
		weftRepoRoot, weftRepoRootErr := WeftRepoRoot(l)
		if weftRepoRootErr != nil {
			return AddResult{}, fmt.Errorf("resolve weft repo root: %w", weftRepoRootErr)
		}
		return AddResult{}, fmt.Errorf("no weft repo at %s; create the hub with \"lyx fabric clone\" first", weftRepoRoot)
	}

	weftTarget := WeftWorktreePath(l, slug)
	if _, err := os.Stat(weftTarget); !os.IsNotExist(err) {
		return AddResult{}, fmt.Errorf("weft worktree directory already exists: %s", weftTarget)
	}

	weftBranchAlreadyExists := weftBranchExists(l, weftBranch)

	// Resolve parent warp branch before worktree creation to avoid partial state on failure.
	// This is a compound guard, not a two-message merge: the second disjunct of the old
	// exitCode != 0 || TrimSpace(stdout) == "HEAD" condition fires on a *successful* git call
	// (a detached HEAD), so there is no error to wrap on that arm — the two conditions stay
	// apart rather than collapsing into one errors.As recovery.
	headStdout, headErr := gitexec.Run([]string{"rev-parse", "--abbrev-ref", "HEAD"}, l.WorktreePath())
	if headErr != nil {
		var headGitErr *gitexec.GitError
		if !errors.As(headErr, &headGitErr) {
			return AddResult{}, fmt.Errorf("rev-parse abbrev-ref HEAD: %w", headErr)
		}
		return AddResult{}, fmt.Errorf("cannot spawn weft branch: warp worktree is on a detached HEAD or unborn branch")
	}
	if strings.TrimSpace(headStdout) == "HEAD" {
		return AddResult{}, fmt.Errorf("cannot spawn weft branch: warp worktree is on a detached HEAD or unborn branch")
	}
	parentBranch := strings.TrimSpace(headStdout)
	parentWeftBranch := WeftBranchName(parentBranch)

	warpTok, err := createGitWorktree(rec, l.WorktreePath(), l.HubPath, target, func(worktreePath string) []string {
		return []string{"worktree", "add", "-b", warpBranch, worktreePath}
	})
	if err != nil {
		return AddResult{}, fmt.Errorf("create worktree %q for branch %q failed: %w", target, warpBranch, err)
	}
	// The `-b warpBranch` argument to the worktree add above means this same call created a branch,
	// not merely a worktree; a branch is a ref, so it records via AppendRef rather than Append.
	rec.AppendRef(KindBranchCreated, warpBranch, "")

	// Install the post-checkout hook now that the warp worktree exists.
	// Hook installation is non-fatal: a failure is logged but does not abort
	// Add or trigger the all-or-nothing rollback (the hook is belt-and-suspenders).
	if hookErr := InstallPostCheckoutHook(l); hookErr != nil {
		logger.Warn("fabricengine: post-checkout hook install failed (non-fatal)", "verb", "add", "slug", slug, "error", hookErr)
	}

	weftPath := WeftWorktreePath(l, slug)
	if weftBranchAlreadyExists {
		weftRepoRoot, weftRepoRootErr := WeftRepoRoot(l)
		if weftRepoRootErr != nil {
			return AddResult{}, fmt.Errorf("resolve weft repo root: %w", weftRepoRootErr)
		}
		// Adopt: git worktree add <path> <branch> (no -b, branch exists), through
		// containedWorktreeAdd so a symlink toggled at weftPath cannot carry the worktree outside the hub.
		err := containedWorktreeAdd(weftRepoRoot, l.HubPath, weftPath, func(worktreePath string) []string {
			return []string{"worktree", "add", worktreePath, weftBranch}
		})
		if err != nil {
			_ = t.rollbackAdd(rec, l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok)
			return AddResult{}, fmt.Errorf("adopt weft worktree for branch %q failed: %w", weftBranch, err)
		}
	} else {
		// Create: git worktree add -b <weftBranch> <path> <parentWeftBranch> (fork from parent's weft branch)
		if err := createWeftWorktree(rec, l, slug, weftBranch, parentWeftBranch); err != nil {
			_ = t.rollbackAdd(rec, l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok)
			return AddResult{}, err
		}
	}

	if err := createPortal(rec, l, slug); err != nil {
		_ = t.rollbackAdd(rec, l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok)
		return AddResult{}, err
	}

	if err := writeLaunchers(rec, l, slug); err != nil {
		_ = t.rollbackAdd(rec, l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok)
		return AddResult{}, err
	}

	// (10b) Wire the new worktree's warp junctions eagerly, sourcing the wired
	// name-set from the repo-wide BoardDir base (not any per-pair weft base or
	// acting-worktree config) — every worktree must converge to the same
	// repo-wide pathspec, matching Checkout's re-point call. The weft worktree
	// already exists from step 8, so junction targets resolve. On failure, roll
	// back the whole pair via the existing post-step-7 path.
	names, err := RepoWiredNames(l)
	if err != nil {
		_ = t.rollbackAdd(rec, l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok)
		return AddResult{}, fmt.Errorf("wire junctions: load fabric config: %w", err)
	}
	if err := WireJunctionsWith(rec, l, slug, names); err != nil {
		_ = t.rollbackAdd(rec, l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok)
		return AddResult{}, fmt.Errorf("wire junctions: %w", err)
	}

	// (10c) Record and commit the pair's provenance now that the pair is fully wired, and
	// before step 11's warp push and step 12's weft push — the weft push that already runs at
	// step 12 carries this commit to the remote, so no new push call is added here.
	if err := WriteOrigin(rec, l, slug, Origin{ParentBranch: parentBranch}); err != nil {
		_ = t.rollbackAdd(rec, l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok)
		return AddResult{}, fmt.Errorf("record parent branch: %w", err)
	}
	// The commit's sha and committed returns are not read here: CommitWeftPaths records the
	// KindCommitCreated entry itself, at its own success site, per the
	// origin-record-records-both-its-write-and-its-commit decision.
	if _, _, err := CommitWeftPaths(rec, weftPath, l.AnchorRel, []string{OriginRecordRel()}, "fabric: record parent branch for "+slug, opts); err != nil {
		_ = t.rollbackAdd(rec, l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok)
		return AddResult{}, fmt.Errorf("commit parent branch record: %w", err)
	}

	// (11) Push warp branch (LAST step for warp)
	if _, err := gitexec.Run([]string{"push", "-u", "origin", warpBranch}, l.WorktreePath()); err != nil {
		_ = t.rollbackAdd(rec, l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok)
		return AddResult{}, fmt.Errorf("push branch %q failed: %w", warpBranch, err)
	}
	rec.AppendRef(KindBranchPushed, warpBranch, "")

	// (12) Push weft branch
	if err := pushWeftBranch(rec, l, slug, weftBranch, opts); err != nil {
		_ = t.rollbackAdd(rec, l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok)
		return AddResult{}, err
	}

	return AddResult{
		Slug:   slug,
		Branch: warpBranch,
		Path:   target,
		// Pushed reflects whether the weft branch was actually pushed to the remote.
		// It is false when either SkipPush or SkipGit suppresses the push.
		Pushed: !opts.SkipPush && !opts.SkipGit,
	}, nil
}

// rollbackAdd performs best-effort paired cleanup on Add failure, unwiring junctions,
// removing worktrees and branches, preserving pre-existing adopted weft branches.
// warpTok is the token createGitWorktree minted when this Add call created the warp worktree at
// target; it is the ownership proof the gate's warp-side removal requires.
// The warp-branch deletion (step 5) is the one cleanup the gate may refuse: ownedManagedBranch can
// prove a branch is fabric's only via a -weft suffix or a non-empty branch_prefix, so under the
// default empty prefix the bare-slug warp branch is indistinguishable from a user's own branch and is
// left behind rather than risk deleting the operator's work. That refusal is logged, not swallowed
// (this function's return is discarded by every caller), so the leftover branch is visible in the
// trace; recovery is the "already exists" remedy Add's own re-add error already names.
// rec is Add's own recorder, threaded through to all six gate-bound calls this function reaches
// (its own removeGitWorktree and deleteBranch, plus removeWeftWorktree, removeWarpJunction,
// removePortal and removeLaunchers), so a rollback's own destructions land in the same record as
// Add's creations, in execution order.
// The origin record needs no removal step of its own: on the created-branch path the existing
// weft-worktree and weft-branch removal (step 1) takes the record with it, and on the
// adopted-weft-branch path the record's commit is deliberately left in place — reverting or
// resetting an adopted branch to undo one commit is exactly the pre-existing-history destruction
// the !weftBranchAdopted guard above exists to prevent.
func (t *Topology) rollbackAdd(rec *Mutations, l *lyxcwd.Location, slug, warpBranch, weftBranch, target string, weftBranchAdopted bool, warpTok createdToken) error {
	var firstErr error

	// (1) Remove the weft worktree; delete the weft branch only when this Add
	// created it, so a rollback never destroys pre-existing weft history.
	if err := removeWeftWorktree(rec, l, slug, weftBranch, true, !weftBranchAdopted, t.cfg.BranchPrefix); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}

	// (1b) Remove warp junctions wired by step 10b, best-effort. Source names
	// from the repo-wide BoardDir base, mirroring Remove's step 5: a rollback
	// must not hard-fail when the repo-wide config is unreadable (Add is
	// failing already), so a load error falls back to names == nil, which
	// removes nothing rather than guessing a wiring set.
	names, namesErr := RepoWiredNames(l)
	if namesErr != nil {
		names = nil
	}
	if err := removeWarpJunction(rec, l, slug, names); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}

	// (2) Remove warp portal
	if err := removePortal(rec, l, slug); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}

	// (3) Remove warp launchers
	if err := removeLaunchers(rec, l, slug); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}

	// (4) Remove warp worktree
	removeReq := pathRequest{
		what:      "remove warp worktree",
		container: l.HubPath,
		target:    target,
		ownership: ownedFreshlyCreatedWorktree(warpTok),
		dirtiness: dirtinessNA("rollback of the worktree this Add created"),
		force:     true,
	}
	err := removeGitWorktree(rec, removeReq, l.WorktreePath())
	if refusalErr := surfaceRefusal(err); refusalErr != nil {
		if firstErr == nil {
			firstErr = refusalErr
		}
	} else if err != nil && firstErr == nil {
		firstErr = err
	}

	// (5) Delete warp branch
	branchReq := branchRequest{
		what:      "delete warp branch",
		repoDir:   l.WorktreePath(),
		branch:    warpBranch,
		ownership: ownedManagedBranch(l, t.cfg.BranchPrefix),
		dirtiness: dirtyCheckedOutBranch(),
		force:     false,
	}
	err = deleteBranch(rec, branchReq)
	if refusalErr := surfaceRefusal(err); refusalErr != nil {
		// Log the swallowed refusal so the leftover warp branch is visible in the trace: this
		// function's return is discarded by every caller, and under the default empty branch_prefix the
		// gate always refuses to delete the bare-slug warp branch (it cannot prove the branch is
		// fabric's). Mirrors rollbackSwitch's own logger.Warn for the identical best-effort-void case.
		var refusal *destructiveRefusal
		if errors.As(refusalErr, &refusal) {
			logger.Warn("fabricengine: rollbackAdd's warp-branch deletion was refused by the destructive gate; the branch is left behind (retry `lyx fabric add`, or `git branch -D`)", "branch", warpBranch, "check", string(refusal.Check))
		}
		if firstErr == nil {
			firstErr = refusalErr
		}
	} else if err != nil && firstErr == nil {
		firstErr = err
	}

	// (6) Prune warp worktrees
	if _, err := gitexec.Run([]string{"worktree", "prune"}, l.WorktreePath()); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}
