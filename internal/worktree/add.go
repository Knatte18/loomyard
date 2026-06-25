// add.go implements the transactional Add: it creates the worktree, portal, and
// launchers, then pushes last, performing a best-effort full rollback on any
// post-creation failure so a partial worktree is never left behind.

package worktree

import (
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/paths"
)

// AddOptions controls optional behaviour for Add. Tests pass these directly
// instead of relying on environment variables, which makes t.Parallel() safe.
type AddOptions struct {
	// SkipGit disables all weft-side git operations (push suppressed entirely).
	SkipGit bool
	// SkipPush disables only the weft-branch push while keeping all local git
	// operations intact.
	SkipPush bool
}

// AddResult contains the result of successfully adding a new worktree.
type AddResult struct {
	Slug   string `json:"slug"`
	Branch string `json:"branch"`
	Path   string `json:"path"`
	Pushed bool   `json:"pushed"`
}

// Add creates a new paired host and weft git worktree with the given slug.
//
// The Layout l provides geometry information; all git operations use the appropriate cwd
// (l.WorktreeRoot for host, l.WeftRepoRoot for weft). The slug becomes the final path
// component; the branch name is formed by prepending the configured BranchPrefix and
// is used for both host and weft worktrees (mirrored).
//
// opts controls optional behaviour such as suppressing the weft-branch push. Tests
// pass AddOptions directly so they do not rely on environment variables and can
// safely use t.Parallel(). Production callers populate AddOptions from environment
// variables at the CLI edge (see cli.go).
//
// Steps:
//  1. Clean check: l.WorktreeRoot must have no uncommitted changes.
//  2. Branch name: branch := w.cfg.BranchPrefix + slug
//  3. Branch-exists check: branch must not already exist in host.
//  4. Target path: sibling directory named slug; must not exist.
//  5. Remote check: must have at least one remote configured.
//  6. Weft prechecks: weft repo must exist; weft worktree and branch must not exist yet.
//  6b. Resolve parent host branch: capture host HEAD as branch name; abort if detached/unborn.
//  7. Create: git worktree add -b <branch> <target> in host repo.
//  8. Create weft worktree with mirrored branch, forking from parent weft branch start-point.
//  9. Seed host _lyx junction pointing to weft _lyx (also enforces pristine host).
//
// 10. Seed host git exclude to skip _lyx.
// 11. Create portal junction to _lyx/ in the new host worktree.
// 12. Write per-worktree launchers.
// 13. Push host branch: git push -u origin <branch> (LAST step).
// 14. Push weft branch: git push -u origin <branch> to weft remote (respects opts).
//
// On ANY error at or after step 7, performs a best-effort full paired rollback:
// - removeHostJunction(l, slug) — remove host _lyx junction first (Windows junction-lock hazard)
// - removeWeftWorktree(l, slug, branch, true) — tear down weft side (worktree + branch + prune)
// - removePortal(l, slug)
// - removeLaunchers(l, slug)
// - git worktree remove --force <host-target>
// - git branch -D <branch> in host
// - git worktree prune in host
//
// The ORIGINAL error is returned; rollback-step failures are not masked.
//
// Returns AddResult on success or an error if any step fails.
func (w *Worktree) Add(l *paths.Layout, slug string, opts AddOptions) (AddResult, error) {
	// (1) Clean check
	stdout, stderr, exitCode, err := gitexec.RunGit([]string{"status", "--porcelain", "--untracked-files=no"}, l.WorktreeRoot)
	if err != nil {
		return AddResult{}, fmt.Errorf("cwd is not a valid git worktree")
	}
	if exitCode != 0 {
		return AddResult{}, fmt.Errorf("cwd is not a valid git worktree")
	}
	if strings.TrimSpace(stdout) != "" {
		return AddResult{}, fmt.Errorf("source worktree has uncommitted changes")
	}

	// (2) Branch name
	branch := w.cfg.BranchPrefix + slug

	// (3) Branch-exists check
	_, _, exitCode, err = gitexec.RunGit([]string{"rev-parse", "--verify", "refs/heads/" + branch}, l.WorktreeRoot)
	if err != nil {
		return AddResult{}, fmt.Errorf("cwd is not a valid git worktree")
	}
	if exitCode == 0 {
		return AddResult{}, fmt.Errorf("branch %q already exists", branch)
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

	if weftBranchExists(l, branch) {
		return AddResult{}, fmt.Errorf("weft branch %q already exists", branch)
	}

	// (6b) Resolve parent host branch; abort if detached/unborn
	// This must run BEFORE host worktree creation to avoid partial state.
	stdout, _, exitCode, err = gitexec.RunGit([]string{"rev-parse", "--abbrev-ref", "HEAD"}, l.WorktreeRoot)
	if err != nil {
		return AddResult{}, fmt.Errorf("rev-parse abbrev-ref HEAD: %w", err)
	}
	if exitCode != 0 || strings.TrimSpace(stdout) == "HEAD" {
		return AddResult{}, fmt.Errorf("cannot spawn weft branch: host worktree is on a detached HEAD or unborn branch")
	}
	parentBranch := strings.TrimSpace(stdout)

	// (7) Create host worktree
	_, stderr, exitCode, err = gitexec.RunGit([]string{"worktree", "add", "-b", branch, target}, l.WorktreeRoot)
	if err != nil {
		return AddResult{}, fmt.Errorf("cwd is not a valid git worktree")
	}
	if exitCode != 0 {
		return AddResult{}, fmt.Errorf("worktree add failed: %s", stderr)
	}

	// (8) Create weft worktree with mirrored branch forking from parent weft branch
	if err := createWeftWorktree(l, slug, branch, parentBranch); err != nil {
		w.rollbackAdd(l, slug, branch, target)
		return AddResult{}, err
	}

	// (9) Seed host _lyx junction (also enforces pristine host via error on real _lyx)
	if err := seedLyxJunction(l, slug); err != nil {
		w.rollbackAdd(l, slug, branch, target)
		return AddResult{}, err
	}

	// (10) Seed host git exclude
	if err := seedGitExclude(l, slug); err != nil {
		w.rollbackAdd(l, slug, branch, target)
		return AddResult{}, err
	}

	// (11) Create portal junction
	if err := createPortal(l, slug); err != nil {
		w.rollbackAdd(l, slug, branch, target)
		return AddResult{}, err
	}

	// (12) Write launchers
	if err := writeLaunchers(l, slug); err != nil {
		w.rollbackAdd(l, slug, branch, target)
		return AddResult{}, err
	}

	// (13) Push host branch (LAST step for host)
	_, stderr, exitCode, err = gitexec.RunGit([]string{"push", "-u", "origin", branch}, l.WorktreeRoot)
	if err != nil {
		w.rollbackAdd(l, slug, branch, target)
		return AddResult{}, fmt.Errorf("push: %w", err)
	}
	if exitCode != 0 {
		w.rollbackAdd(l, slug, branch, target)
		return AddResult{}, fmt.Errorf("push failed: %s", stderr)
	}

	// (14) Push weft branch
	if err := pushWeftBranch(l, slug, branch, opts); err != nil {
		w.rollbackAdd(l, slug, branch, target)
		return AddResult{}, err
	}

	return AddResult{
		Slug:   slug,
		Branch: branch,
		Path:   target,
		// Pushed reflects whether the weft branch was actually pushed to the remote.
		// It is false when either SkipPush or SkipGit suppresses the push.
		Pushed: !opts.SkipPush && !opts.SkipGit,
	}, nil
}

// rollbackAdd performs best-effort paired cleanup on Add failure.
//
// Steps (best-effort, errors collected but not masked):
//  1. removeHostJunction — remove host _lyx junction FIRST (Windows junction-lock hazard)
//  2. removeWeftWorktree — tear down weft side (worktree + branch + prune)
//  3. removePortal — remove host portal junction
//  4. removeLaunchers — remove host launchers
//  5. git worktree remove --force <host-target>
//  6. git branch -D <branch> (host)
//  7. git worktree prune (host)
//
// Junction removal must precede any worktree removal. All errors are collected;
// the original error passed to the caller is preserved.
func (w *Worktree) rollbackAdd(l *paths.Layout, slug, branch, target string) error {
	var firstErr error

	// (1) Remove host junction FIRST (must precede worktree removal on Windows)
	if err := removeHostJunction(l, slug); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}

	// (2) Remove weft worktree and branch
	if err := removeWeftWorktree(l, slug, branch, true); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}

	// (3) Remove host portal
	if err := removePortal(l, slug); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}

	// (4) Remove host launchers
	if err := removeLaunchers(l, slug); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}

	// (5) Remove host worktree
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

	// (6) Delete host branch
	_, _, exitCode, err = gitexec.RunGit([]string{"branch", "-D", branch}, l.WorktreeRoot)
	if err != nil || exitCode != 0 {
		if firstErr == nil {
			if err != nil {
				firstErr = err
			} else {
				firstErr = fmt.Errorf("git branch -D failed with exit code %d", exitCode)
			}
		}
	}

	// (7) Prune host worktrees
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
