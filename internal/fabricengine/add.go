// add.go implements the transactional Add: it creates the host worktree,
// portal, and launchers, then pushes last, performing a best-effort full
// rollback on any post-creation failure so a partial worktree pair is never
// left behind. The weft side always uses the suffixed branch produced by
// WeftBranchName.

package fabricengine

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// AddOptions controls optional behaviour for Add. It is an alias of
// SyncOptions (same SkipGit/SkipPush field shape as warp's own AddOptions)
// rather than a distinct type, so Add can pass opts straight through to
// pushWeftBranch, which already takes SyncOptions. Tests pass these directly
// instead of relying on environment variables, which makes t.Parallel() safe.
type AddOptions = SyncOptions

// AddResult contains the result of successfully adding a new worktree pair.
type AddResult struct {
	Slug   string `json:"slug"`
	Branch string `json:"branch"`
	Path   string `json:"path"`
	Pushed bool   `json:"pushed"`
}

// Add creates a new paired host and weft git worktree with the given slug.
// It validates the slug, creates both worktrees, wires junctions, and pushes branches,
// rolling back all changes on any failure.
func (t *Topology) Add(l *hubgeometry.Layout, slug string, opts AddOptions) (AddResult, error) {
	// (0) Slug validation. A slug is by contract a single path component:
	// every consumer re-derives it from the host worktree path via
	// filepath.Base (status, reconcile, prune) and the hub scan only looks at
	// the hub's top level, so a separator-containing slug would create a pair
	// the rest of the module cannot re-identify. Reject both separators on
	// every platform — a slash-free contract must not depend on GOOS.
	if strings.TrimSpace(slug) == "" {
		return AddResult{}, fmt.Errorf("invalid slug %q: a slug must not be empty", slug)
	}

	if strings.ContainsAny(slug, `/\`) {
		return AddResult{}, fmt.Errorf("invalid slug %q: a slug must be a single path component (no '/' or '\\')", slug)
	}

	// Reject slugs ending with weft suffix to prevent collision with weft worktree directory naming.
	if strings.HasSuffix(slug, weftname.Suffix) {
		return AddResult{}, fmt.Errorf("invalid slug %q: a slug must not end in %q (that suffix is reserved for weft worktrees)", slug, weftname.Suffix)
	}

	// Reject reserved hub-level geometry names that would collide with hub structure.
	if hubgeometry.IsReservedHubName(slug, t.cfg.Dirs()) {
		return AddResult{}, fmt.Errorf("invalid slug %q: that name is reserved for lyx hub geometry", slug)
	}

	stdout, _, exitCode, err := gitexec.RunGit([]string{"status", "--porcelain", "--untracked-files=no"}, l.WorktreeRoot)
	if err != nil {
		return AddResult{}, fmt.Errorf("cwd is not a valid git worktree")
	}
	if exitCode != 0 {
		return AddResult{}, fmt.Errorf("cwd is not a valid git worktree")
	}
	if strings.TrimSpace(stdout) != "" {
		return AddResult{}, fmt.Errorf("source worktree has uncommitted changes")
	}

	hostBranch := t.cfg.BranchPrefix + slug
	weftBranch := WeftBranchName(hostBranch)

	_, _, exitCode, err = gitexec.RunGit([]string{"rev-parse", "--verify", "refs/heads/" + hostBranch}, l.WorktreeRoot)
	if err != nil {
		return AddResult{}, fmt.Errorf("cwd is not a valid git worktree")
	}
	if exitCode == 0 {
		// Name the way forward: Remove deliberately leaves the host branch
		// behind (it may carry unmerged work), so remove-then-re-add of the
		// same slug lands here — a bare "already exists" gives the operator no
		// path out of that everyday cycle.
		return AddResult{}, fmt.Errorf(
			"branch %q already exists; switch a pair onto it with \"lyx fabric checkout %s\", or delete it first with \"git branch -D %s\" if it is a leftover from a removed pair",
			hostBranch, hostBranch, hostBranch,
		)
	}

	target := l.WorktreePath(slug)
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		return AddResult{}, fmt.Errorf("worktree directory %q already exists", target)
	}

	stdout, _, exitCode, err = gitexec.RunGit([]string{"remote"}, l.WorktreeRoot)
	if err != nil {
		return AddResult{}, fmt.Errorf("cwd is not a valid git worktree")
	}
	if exitCode != 0 {
		return AddResult{}, fmt.Errorf("cwd is not a valid git worktree")
	}
	if strings.TrimSpace(stdout) == "" {
		return AddResult{}, fmt.Errorf("no remote configured")
	}

	if !weftRepoExists(l) {
		weftRepoRoot, weftRepoRootErr := WeftRepoRoot(l)
		if weftRepoRootErr != nil {
			return AddResult{}, fmt.Errorf("resolve weft repo root: %w", weftRepoRootErr)
		}
		return AddResult{}, fmt.Errorf("no weft repo at %s; run the hub-creator first", weftRepoRoot)
	}

	weftTarget := l.WeftWorktreePath(slug)
	if _, err := os.Stat(weftTarget); !os.IsNotExist(err) {
		return AddResult{}, fmt.Errorf("weft worktree directory already exists: %s", weftTarget)
	}

	weftBranchAlreadyExists := weftBranchExists(l, weftBranch)

	// Resolve parent host branch before worktree creation to avoid partial state on failure.
	stdout, _, exitCode, err = gitexec.RunGit([]string{"rev-parse", "--abbrev-ref", "HEAD"}, l.WorktreeRoot)
	if err != nil {
		return AddResult{}, fmt.Errorf("rev-parse abbrev-ref HEAD: %w", err)
	}
	if exitCode != 0 || strings.TrimSpace(stdout) == "HEAD" {
		return AddResult{}, fmt.Errorf("cannot spawn weft branch: host worktree is on a detached HEAD or unborn branch")
	}
	parentBranch := strings.TrimSpace(stdout)
	parentWeftBranch := WeftBranchName(parentBranch)

	_, _, exitCode, err = gitexec.RunGit([]string{"worktree", "add", "-b", hostBranch, target}, l.WorktreeRoot)
	if err != nil {
		return AddResult{}, fmt.Errorf("cwd is not a valid git worktree")
	}
	if exitCode != 0 {
		return AddResult{}, fmt.Errorf("create worktree %q for branch %q failed (git exit %d)", target, hostBranch, exitCode)
	}

	// Install the post-checkout hook now that the host worktree exists.
	// Hook installation is non-fatal: a failure is logged but does not abort
	// Add or trigger the all-or-nothing rollback (the hook is belt-and-suspenders).
	if hookErr := InstallPostCheckoutHook(l); hookErr != nil {
		log.Printf("fabric add: post-checkout hook install (non-fatal): %v", hookErr)
	}

	weftPath := l.WeftWorktreePath(slug)
	if weftBranchAlreadyExists {
		weftRepoRoot, weftRepoRootErr := WeftRepoRoot(l)
		if weftRepoRootErr != nil {
			return AddResult{}, fmt.Errorf("resolve weft repo root: %w", weftRepoRootErr)
		}
		// Adopt: git worktree add <path> <branch> (no -b, branch exists)
		_, _, exitCode, err := gitexec.RunGit(
			[]string{"worktree", "add", weftPath, weftBranch},
			weftRepoRoot,
		)
		if err != nil {
			_ = t.rollbackAdd(l, slug, hostBranch, weftBranch, target, weftBranchAlreadyExists)
			return AddResult{}, fmt.Errorf("failed to adopt weft worktree: %w", err)
		}
		if exitCode != 0 {
			_ = t.rollbackAdd(l, slug, hostBranch, weftBranch, target, weftBranchAlreadyExists)
			return AddResult{}, fmt.Errorf("adopt weft worktree for branch %q failed (git exit %d)", weftBranch, exitCode)
		}
	} else {
		// Create: git worktree add -b <weftBranch> <path> <parentWeftBranch> (fork from parent's weft branch)
		if err := createWeftWorktree(l, slug, weftBranch, parentWeftBranch); err != nil {
			_ = t.rollbackAdd(l, slug, hostBranch, weftBranch, target, weftBranchAlreadyExists)
			return AddResult{}, err
		}
	}

	if err := createPortal(l, slug); err != nil {
		_ = t.rollbackAdd(l, slug, hostBranch, weftBranch, target, weftBranchAlreadyExists)
		return AddResult{}, err
	}

	if err := writeLaunchers(l, slug); err != nil {
		_ = t.rollbackAdd(l, slug, hostBranch, weftBranch, target, weftBranchAlreadyExists)
		return AddResult{}, err
	}

	// (10b) Wire the new worktree's host junctions eagerly, sourcing the wired
	// name-set from the repo-wide BoardDir base (not any per-pair weft base or
	// acting-worktree config) — every worktree must converge to the same
	// repo-wide pathspec, matching Checkout's re-point call. The weft worktree
	// already exists from step 8, so junction targets resolve. On failure, roll
	// back the whole pair via the existing post-step-7 path.
	names, err := RepoWiredNames(l)
	if err != nil {
		_ = t.rollbackAdd(l, slug, hostBranch, weftBranch, target, weftBranchAlreadyExists)
		return AddResult{}, fmt.Errorf("wire junctions: load fabric config: %w", err)
	}
	if err := WireJunctions(l, slug, names); err != nil {
		_ = t.rollbackAdd(l, slug, hostBranch, weftBranch, target, weftBranchAlreadyExists)
		return AddResult{}, fmt.Errorf("wire junctions: %w", err)
	}

	// (11) Push host branch (LAST step for host)
	_, _, exitCode, err = gitexec.RunGit([]string{"push", "-u", "origin", hostBranch}, l.WorktreeRoot)
	if err != nil {
		_ = t.rollbackAdd(l, slug, hostBranch, weftBranch, target, weftBranchAlreadyExists)
		return AddResult{}, fmt.Errorf("push: %w", err)
	}
	if exitCode != 0 {
		_ = t.rollbackAdd(l, slug, hostBranch, weftBranch, target, weftBranchAlreadyExists)
		return AddResult{}, fmt.Errorf("push branch %q failed (git exit %d)", hostBranch, exitCode)
	}

	// (12) Push weft branch
	if err := pushWeftBranch(l, slug, weftBranch, opts); err != nil {
		_ = t.rollbackAdd(l, slug, hostBranch, weftBranch, target, weftBranchAlreadyExists)
		return AddResult{}, err
	}

	return AddResult{
		Slug:   slug,
		Branch: hostBranch,
		Path:   target,
		// Pushed reflects whether the weft branch was actually pushed to the remote.
		// It is false when either SkipPush or SkipGit suppresses the push.
		Pushed: !opts.SkipPush && !opts.SkipGit,
	}, nil
}

// rollbackAdd performs best-effort paired cleanup on Add failure, unwiring junctions,
// removing worktrees and branches, preserving pre-existing adopted weft branches.
func (t *Topology) rollbackAdd(l *hubgeometry.Layout, slug, hostBranch, weftBranch, target string, weftBranchAdopted bool) error {
	var firstErr error

	// (1) Remove the weft worktree; delete the weft branch only when this Add
	// created it, so a rollback never destroys pre-existing weft history.
	if err := removeWeftWorktree(l, slug, weftBranch, true, !weftBranchAdopted); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}

	// (1b) Remove host junctions wired by step 10b, best-effort. Source names
	// from the repo-wide BoardDir base, mirroring Remove's step 5: a rollback
	// must not hard-fail when the repo-wide config is unreadable (Add is
	// failing already), so a load error falls back to names == nil, which
	// removes nothing rather than guessing a wiring set.
	names, namesErr := RepoWiredNames(l)
	if namesErr != nil {
		names = nil
	}
	if err := removeHostJunction(l, slug, names); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}

	// (2) Remove host portal
	if err := removePortal(l, slug); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}

	// (3) Remove host launchers
	if err := removeLaunchers(l, slug); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}

	// (4) Remove host worktree
	_, _, exitCode, err := gitexec.RunGit([]string{"worktree", "remove", "--force", target}, l.WorktreeRoot)
	if err != nil || exitCode != 0 {
		if firstErr == nil {
			if err != nil {
				firstErr = err
			} else {
				firstErr = fmt.Errorf("git worktree remove failed with exit code %d", exitCode)
			}
		}
	}

	// (5) Delete host branch
	_, _, exitCode, err = gitexec.RunGit([]string{"branch", "-D", hostBranch}, l.WorktreeRoot)
	if err != nil || exitCode != 0 {
		if firstErr == nil {
			if err != nil {
				firstErr = err
			} else {
				firstErr = fmt.Errorf("git branch -D failed with exit code %d", exitCode)
			}
		}
	}

	// (6) Prune host worktrees
	_, _, exitCode, err = gitexec.RunGit([]string{"worktree", "prune"}, l.WorktreeRoot)
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
