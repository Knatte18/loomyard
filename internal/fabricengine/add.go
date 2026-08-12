// add.go implements the transactional Add: it creates the warp worktree, portal, and launchers,
// then pushes last, performing a best-effort full rollback on any post-creation failure so a
// partial worktree pair is never left behind.
// The weft side always uses the suffixed branch produced by WeftBranchName.

package fabricengine

import (
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
// It validates the slug, creates both worktrees, wires junctions, and pushes branches, rolling back
// all changes on any failure.
func (t *Topology) Add(l *lyxcwd.Location, slug string, opts AddOptions) (res AddResult, err error) {
	rec := NewMutations(l.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()

	// (0) Slug validation, shared with Remove via slug.go's single validator.
	if err := validateWorktreeSlug(slug, t.cfg.Dirs()); err != nil {
		return AddResult{}, err
	}

	dirty, _, err := worktreeDirty(scopeTracked, l.WorktreePath())
	if err != nil {
		return AddResult{}, fmt.Errorf("read warp worktree status at %s: %w", l.WorktreePath(), err)
	}
	if dirty {
		return AddResult{}, fmt.Errorf("source worktree has uncommitted changes")
	}

	warpBranch := t.cfg.BranchPrefix + slug
	weftBranch := WeftBranchName(warpBranch)

	_, _, exitCode, err := gitexec.RunGit([]string{"rev-parse", "--verify", "refs/heads/" + warpBranch}, l.WorktreePath())
	if err != nil {
		return AddResult{}, fmt.Errorf("check whether warp branch %q exists: %w", warpBranch, err)
	}
	if exitCode == 0 {
		// Name the way forward: Remove deliberately leaves the warp branch
		// behind (it may carry unmerged work), so remove-then-re-add of the
		// same slug lands here — a bare "already exists" gives the operator no
		// path out of that everyday cycle.
		return AddResult{}, fmt.Errorf(
			"branch %q already exists; switch a pair onto it with \"lyx fabric checkout %s\", or delete it first with \"git branch -D %s\" if it is a leftover from a removed pair",
			warpBranch, warpBranch, warpBranch,
		)
	}

	target := WorktreePath(l, slug)
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		return AddResult{}, fmt.Errorf("worktree directory %q already exists", target)
	}

	stdout, stderr, exitCode, err := gitexec.RunGit([]string{"remote"}, l.WorktreePath())
	if err != nil {
		return AddResult{}, fmt.Errorf("list warp remotes: %w", err)
	}
	if exitCode != 0 {
		return AddResult{}, fmt.Errorf("list warp remotes (git exit %d): %s", exitCode, strings.TrimSpace(stderr))
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
	stdout, stderr, exitCode, err = gitexec.RunGit([]string{"rev-parse", "--abbrev-ref", "HEAD"}, l.WorktreePath())
	if err != nil {
		return AddResult{}, fmt.Errorf("rev-parse abbrev-ref HEAD: %w", err)
	}
	if exitCode != 0 || strings.TrimSpace(stdout) == "HEAD" {
		return AddResult{}, fmt.Errorf("cannot spawn weft branch: warp worktree is on a detached HEAD or unborn branch")
	}
	parentBranch := strings.TrimSpace(stdout)
	parentWeftBranch := WeftBranchName(parentBranch)

	warpTok, exitCode, stderr, err := createGitWorktree(l.WorktreePath(), []string{"worktree", "add", "-b", warpBranch, target}, target)
	if err != nil {
		return AddResult{}, fmt.Errorf("create warp worktree %q for branch %q: %w", target, warpBranch, err)
	}
	if exitCode != 0 {
		return AddResult{}, fmt.Errorf("create worktree %q for branch %q failed (git exit %d): %s",
			target, warpBranch, exitCode, strings.TrimSpace(stderr))
	}

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
		// Adopt: git worktree add <path> <branch> (no -b, branch exists)
		_, adoptStderr, exitCode, err := gitexec.RunGit(
			[]string{"worktree", "add", weftPath, weftBranch},
			weftRepoRoot,
		)
		if err != nil {
			_ = t.rollbackAdd(l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok)
			return AddResult{}, fmt.Errorf("failed to adopt weft worktree: %w", err)
		}
		if exitCode != 0 {
			_ = t.rollbackAdd(l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok)
			return AddResult{}, fmt.Errorf("adopt weft worktree for branch %q failed (git exit %d): %s",
				weftBranch, exitCode, strings.TrimSpace(adoptStderr))
		}
	} else {
		// Create: git worktree add -b <weftBranch> <path> <parentWeftBranch> (fork from parent's weft branch)
		if err := createWeftWorktree(l, slug, weftBranch, parentWeftBranch); err != nil {
			_ = t.rollbackAdd(l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok)
			return AddResult{}, err
		}
	}

	if err := createPortal(l, slug); err != nil {
		_ = t.rollbackAdd(l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok)
		return AddResult{}, err
	}

	if err := writeLaunchers(l, slug); err != nil {
		_ = t.rollbackAdd(l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok)
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
		_ = t.rollbackAdd(l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok)
		return AddResult{}, fmt.Errorf("wire junctions: load fabric config: %w", err)
	}
	if err := WireJunctions(l, slug, names); err != nil {
		_ = t.rollbackAdd(l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok)
		return AddResult{}, fmt.Errorf("wire junctions: %w", err)
	}

	// (10c) Wire the operator-convenience _board junction as a named special
	// case, alongside the pathspec junctions above — see junction.go's
	// wireBoardLink doc for why it is wired unconditionally rather than
	// folded into names.
	if err := wireBoardLink(l, slug); err != nil {
		_ = t.rollbackAdd(l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok)
		return AddResult{}, fmt.Errorf("wire board junction: %w", err)
	}

	// (11) Push warp branch (LAST step for warp)
	_, pushStderr, exitCode, err := gitexec.RunGit([]string{"push", "-u", "origin", warpBranch}, l.WorktreePath())
	if err != nil {
		_ = t.rollbackAdd(l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok)
		return AddResult{}, fmt.Errorf("push: %w", err)
	}
	if exitCode != 0 {
		_ = t.rollbackAdd(l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok)
		return AddResult{}, fmt.Errorf("push branch %q failed (git exit %d): %s",
			warpBranch, exitCode, strings.TrimSpace(pushStderr))
	}

	// (12) Push weft branch
	if err := pushWeftBranch(l, slug, weftBranch, opts); err != nil {
		_ = t.rollbackAdd(l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok)
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
func (t *Topology) rollbackAdd(l *lyxcwd.Location, slug, warpBranch, weftBranch, target string, weftBranchAdopted bool, warpTok createdToken) error {
	var firstErr error

	// (1) Remove the weft worktree; delete the weft branch only when this Add
	// created it, so a rollback never destroys pre-existing weft history.
	if err := removeWeftWorktree(l, slug, weftBranch, true, !weftBranchAdopted, t.cfg.BranchPrefix); err != nil {
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
	if err := removeWarpJunction(l, slug, names); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}

	// (2) Remove warp portal
	if err := removePortal(l, slug); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}

	// (3) Remove warp launchers
	if err := removeLaunchers(l, slug); err != nil {
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
	exitCode, _, err := removeGitWorktree(removeReq, l.WorktreePath())
	if refusalErr := surfaceRefusal(err); refusalErr != nil {
		if firstErr == nil {
			firstErr = refusalErr
		}
	} else if err != nil || exitCode != 0 {
		if firstErr == nil {
			if err != nil {
				firstErr = err
			} else {
				firstErr = fmt.Errorf("git worktree remove failed with exit code %d", exitCode)
			}
		}
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
	exitCode, _, err = deleteBranch(branchReq)
	if refusalErr := surfaceRefusal(err); refusalErr != nil {
		if firstErr == nil {
			firstErr = refusalErr
		}
	} else if err != nil || exitCode != 0 {
		if firstErr == nil {
			if err != nil {
				firstErr = err
			} else {
				firstErr = fmt.Errorf("git branch -D failed with exit code %d", exitCode)
			}
		}
	}

	// (6) Prune warp worktrees
	_, _, exitCode, err = gitexec.RunGit([]string{"worktree", "prune"}, l.WorktreePath())
	if err != nil || exitCode != 0 {
		if firstErr == nil {
			if err != nil {
				firstErr = err
			} else {
				firstErr = fmt.Errorf("git worktree prune failed with exit code %d", exitCode)
			}
		}
	}

	return firstErr
}
