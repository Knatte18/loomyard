// add.go implements the transactional Add: it creates the host worktree,
// portal, and launchers, then pushes last, performing a best-effort full
// rollback on any post-creation failure so a partial worktree pair is never
// left behind. Adapted from warpengine's add.go — same transactional sequence
// and rollback discipline, package fabricengine. The branch delta: the weft
// side always uses the suffixed branch produced by WeftBranchName rather than
// a mirrored (identical) branch name.

package fabricengine

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
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
//
// The Layout l provides geometry information; all git operations use the
// appropriate cwd (l.WorktreeRoot for host, l.WeftRepoRoot for weft). The
// slug becomes the final path component; the host branch name is formed by
// prepending the configured BranchPrefix. The weft branch is
// WeftBranchName(hostBranch) — always suffixed, never mirrored.
//
// opts controls optional behaviour such as suppressing the weft-branch push.
// Tests pass AddOptions directly so they do not rely on environment variables
// and can safely use t.Parallel(). Production callers populate AddOptions
// from environment variables at the CLI edge (a later batch).
//
// Steps:
//  0. Slug validation: slug must be non-empty, a single path component (no '/'
//     or '\'), must not end in the weft suffix (reserved for weft worktrees),
//     and must not name a reserved hub-level geometry entry
//     (hubgeometry.IsReservedHubName: _lyx, _raddle, _board, _portals, _launchers).
//  1. Clean check: l.WorktreeRoot must have no uncommitted changes.
//  2. Branch name: hostBranch := t.cfg.BranchPrefix + slug; weftBranch := WeftBranchName(hostBranch)
//  3. Branch-exists check: hostBranch must not already exist in host.
//  4. Target path: sibling directory named slug; must not exist.
//  5. Remote check: must have at least one remote configured.
//  6. Weft prechecks: weft repo must exist; weft worktree must not exist yet.
//     6b. Resolve parent host branch: capture host HEAD as branch name; abort if detached/unborn.
//  7. Create: git worktree add -b <hostBranch> <target> in host repo.
//  8. Create or adopt weft worktree: if weftBranch exists, adopt it (build from existing
//     branch); otherwise create new weft worktree with weftBranch forking from the
//     parent's weft branch (WeftBranchName(parentBranch)).
//  9. Create portal junction to _lyx/ in the new host worktree.
//
// 10. Write per-worktree launchers.
// 11. Push host branch: git push -u origin <hostBranch> (LAST step for host).
// 12. Push weft branch: git push -u origin <weftBranch> to weft remote (respects opts).
//
// On ANY error at or after step 7, performs a best-effort full paired rollback:
// - removeWeftWorktree — tear down the weft worktree (and the weft branch only
//   when Add created it; an adopted pre-existing branch is never deleted)
// - removePortal(l, slug)
// - removeLaunchers(l, slug)
// - git worktree remove --force <host-target>
// - git branch -D <hostBranch> in host
// - git worktree prune in host
//
// The ORIGINAL error is returned; rollback-step failures are not masked.
//
// Returns AddResult on success or an error if any step fails.
func (t *Topology) Add(l *hubgeometry.Layout, slug string, opts AddOptions) (AddResult, error) {
	// (0) Slug validation. A slug is by contract a single path component:
	// every consumer re-derives it from the host worktree path via
	// filepath.Base (status, reconcile, prune) and the hub scan only looks at
	// the hub's top level, so a separator-containing slug would create a pair
	// the rest of the module cannot re-identify. Reject both separators on
	// every platform — a slash-free contract must not depend on GOOS.
	if strings.TrimSpace(slug) == "" {
		// An empty (or whitespace-only) slug has no name for the pair and would
		// otherwise fall through to step 4, where l.WorktreePath("") resolves to
		// the hub root and Add fails with a misleading "worktree directory
		// <HUB> already exists". Reject it here with an honest message.
		return AddResult{}, fmt.Errorf("invalid slug %q: a slug must not be empty", slug)
	}

	if strings.ContainsAny(slug, `/\`) {
		return AddResult{}, fmt.Errorf("invalid slug %q: a slug must be a single path component (no '/' or '\\')", slug)
	}

	// A slug ending in the weft suffix would name a host worktree directory
	// (l.WorktreePath(slug)) that is indistinguishable from a weft worktree
	// directory: hubgeometry.WeftHostSlug accepts it, so prune's hub scan would
	// misclassify the host worktree as an orphaned weft and — under --apply —
	// os.RemoveAll it, destroying the host worktree and any uncommitted work in
	// it. fabric owns the weft suffix namespace, so the safe place to close this
	// is here, before any git operation, rejecting the collision at the source.
	if strings.HasSuffix(slug, hubgeometry.WeftSuffix) {
		return AddResult{}, fmt.Errorf("invalid slug %q: a slug must not end in %q (that suffix is reserved for weft worktrees)", slug, hubgeometry.WeftSuffix)
	}

	// A slug naming a reserved hub-level geometry entry (_lyx, _raddle, _board,
	// _portals, _launchers) would create a host worktree directory colliding
	// with the paths lyx composes at the hub level — e.g. a worktree named
	// "_portals" on a fresh hub would later have portal junctions created
	// inside it, and a hub-level "_lyx" worktree shadows the config-dir token
	// every module resolves. Some of these are blocked incidentally by the
	// step-4 directory-exists check on mature hubs; rejecting them here makes
	// the guard unconditional and the error honest.
	if hubgeometry.IsReservedHubName(slug) {
		return AddResult{}, fmt.Errorf("invalid slug %q: that name is reserved for lyx hub geometry", slug)
	}

	// (1) Clean check
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

	// (2) Branch names
	hostBranch := t.cfg.BranchPrefix + slug
	weftBranch := WeftBranchName(hostBranch)

	// (3) Branch-exists check
	_, _, exitCode, err = gitexec.RunGit([]string{"rev-parse", "--verify", "refs/heads/" + hostBranch}, l.WorktreeRoot)
	if err != nil {
		return AddResult{}, fmt.Errorf("cwd is not a valid git worktree")
	}
	if exitCode == 0 {
		return AddResult{}, fmt.Errorf("branch %q already exists", hostBranch)
	}

	// (4) Target path check
	target := l.WorktreePath(slug)
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		return AddResult{}, fmt.Errorf("worktree directory %q already exists", target)
	}

	// (5) Remote check
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

	// (6) Weft prechecks: must run BEFORE any creation (no partial state)
	if !weftRepoExists(l) {
		return AddResult{}, fmt.Errorf("no weft repo at %s; run the hub-creator first", l.WeftRepoRoot())
	}

	weftTarget := l.WeftWorktreePath(slug)
	if _, err := os.Stat(weftTarget); !os.IsNotExist(err) {
		return AddResult{}, fmt.Errorf("weft worktree directory already exists: %s", weftTarget)
	}

	weftBranchAlreadyExists := weftBranchExists(l, weftBranch)

	// (6b) Resolve parent host branch; abort if detached/unborn.
	// This must run BEFORE host worktree creation to avoid partial state.
	stdout, _, exitCode, err = gitexec.RunGit([]string{"rev-parse", "--abbrev-ref", "HEAD"}, l.WorktreeRoot)
	if err != nil {
		return AddResult{}, fmt.Errorf("rev-parse abbrev-ref HEAD: %w", err)
	}
	if exitCode != 0 || strings.TrimSpace(stdout) == "HEAD" {
		return AddResult{}, fmt.Errorf("cannot spawn weft branch: host worktree is on a detached HEAD or unborn branch")
	}
	parentBranch := strings.TrimSpace(stdout)
	parentWeftBranch := WeftBranchName(parentBranch)

	// (7) Create host worktree
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

	// (8) Create or adopt weft worktree: if the weft branch already exists,
	// adopt it (without -b); otherwise create new with -b forking from the
	// parent's weft branch.
	weftPath := l.WeftWorktreePath(slug)
	if weftBranchAlreadyExists {
		// Adopt: git worktree add <path> <branch> (no -b, branch exists)
		_, _, exitCode, err := gitexec.RunGit(
			[]string{"worktree", "add", weftPath, weftBranch},
			l.WeftRepoRoot(),
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

	// (9) Create portal junction
	if err := createPortal(l, slug); err != nil {
		_ = t.rollbackAdd(l, slug, hostBranch, weftBranch, target, weftBranchAlreadyExists)
		return AddResult{}, err
	}

	// (10) Write launchers
	if err := writeLaunchers(l, slug); err != nil {
		_ = t.rollbackAdd(l, slug, hostBranch, weftBranch, target, weftBranchAlreadyExists)
		return AddResult{}, err
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

// rollbackAdd performs best-effort paired cleanup on Add failure.
//
// Steps (best-effort, errors collected but not masked):
//  1. removeWeftWorktree — tear down the weft worktree (and the weft branch
//     only when Add created it — see weftBranchAdopted below)
//  2. removePortal — remove host portal junction
//  3. removeLaunchers — remove host launchers
//  4. git worktree remove --force <host-target>
//  5. git branch -D <hostBranch> (host)
//  6. git worktree prune (host)
//
// weftBranchAdopted reports whether Add adopted a pre-existing weft branch
// (step 8's adopt path) rather than creating one. An adopted branch — and any
// unpushed history it carries — predates this Add and is never deleted by its
// rollback; only a branch Add itself created is torn down.
//
// Note: Add does not wire the host _lyx junction (it is dormant), so rollback
// does not remove it. The junction is wired by lyx init via WireJunctions.
// All errors are collected; the original error passed to the caller is preserved.
func (t *Topology) rollbackAdd(l *hubgeometry.Layout, slug, hostBranch, weftBranch, target string, weftBranchAdopted bool) error {
	var firstErr error

	// (1) Remove the weft worktree; delete the weft branch only when this Add
	// created it, so a rollback never destroys pre-existing weft history.
	if err := removeWeftWorktree(l, slug, weftBranch, true, !weftBranchAdopted); err != nil {
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
