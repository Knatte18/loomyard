// add.go implements the transactional Add: it creates the worktree, portal, and
// launchers, then pushes last, performing a best-effort full rollback on any
// post-creation failure so a partial worktree is never left behind.

package worktree

import (
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/mhgo/internal/git"
	"github.com/Knatte18/mhgo/internal/paths"
)

// AddResult contains the result of successfully adding a new worktree.
type AddResult struct {
	Slug   string `json:"slug"`
	Branch string `json:"branch"`
	Path   string `json:"path"`
	Pushed bool   `json:"pushed"`
}

// Add creates a new git worktree with the given slug in a sibling directory,
// along with associated portal junctions and launchers.
//
// The Layout l provides geometry information; all git operations use l.WorktreeRoot as cwd.
// The slug becomes the final path component; the branch name is formed by prepending
// the configured BranchPrefix.
//
// Steps:
//  1. Clean check: l.WorktreeRoot must have no uncommitted changes.
//  2. Branch name: branch := w.cfg.BranchPrefix + slug
//  3. Branch-exists check: branch must not already exist.
//  4. Target path: sibling directory named slug; must not exist.
//  5. Remote check: must have at least one remote configured.
//  6. Create: git worktree add -b <branch> <target>
//  7. Create portal junction to _mhgo/ in the new worktree
//  8. Write per-worktree launchers
//  9. Push: git push -u origin <branch> (LAST step, so rollback can skip push)
//
// On ANY error at or after step 6, performs a best-effort full rollback:
// - removePortal(l, slug)
// - removeLaunchers(l, slug)
// - git worktree remove --force <target>
// - git branch -D <branch>
// - git worktree prune
//
// The ORIGINAL error is returned; rollback-step failures are not masked.
//
// Returns AddResult on success or an error if any step fails.
func (w *Worktree) Add(l *paths.Layout, slug string) (AddResult, error) {
	// (1) Clean check
	stdout, stderr, exitCode, err := git.RunGit([]string{"status", "--porcelain", "--untracked-files=no"}, l.WorktreeRoot)
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
	_, _, exitCode, err = git.RunGit([]string{"rev-parse", "--verify", "refs/heads/" + branch}, l.WorktreeRoot)
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
	stdout, _, exitCode, err = git.RunGit([]string{"remote"}, l.WorktreeRoot)
	if err != nil {
		return AddResult{}, fmt.Errorf("cwd is not a valid git worktree")
	}
	if exitCode != 0 {
		return AddResult{}, fmt.Errorf("cwd is not a valid git worktree")
	}
	if strings.TrimSpace(stdout) == "" {
		return AddResult{}, fmt.Errorf("no remote configured")
	}

	// (6) Create worktree
	_, stderr, exitCode, err = git.RunGit([]string{"worktree", "add", "-b", branch, target}, l.WorktreeRoot)
	if err != nil {
		return AddResult{}, fmt.Errorf("cwd is not a valid git worktree")
	}
	if exitCode != 0 {
		return AddResult{}, fmt.Errorf("worktree add failed: %s", stderr)
	}

	// (7) Create portal junction
	if err := createPortal(l, slug); err != nil {
		// Rollback on portal creation failure
		w.rollbackAdd(l, slug, branch, target)
		return AddResult{}, err
	}

	// (8) Write launchers
	if err := writeLaunchers(l, slug); err != nil {
		// Rollback on launcher write failure
		w.rollbackAdd(l, slug, branch, target) // Best-effort rollback (errors masked)
		return AddResult{}, err                // Return original error
	}

	// (9) Push (LAST step)
	_, stderr, exitCode, err = git.RunGit([]string{"push", "-u", "origin", branch}, l.WorktreeRoot)
	if err != nil {
		// Rollback on push failure
		w.rollbackAdd(l, slug, branch, target) // Best-effort rollback (errors masked)
		return AddResult{}, fmt.Errorf("push: %w", err)
	}
	if exitCode != 0 {
		// Rollback on push failure
		w.rollbackAdd(l, slug, branch, target) // Best-effort rollback (errors masked)
		return AddResult{}, fmt.Errorf("push failed: %s", stderr)
	}

	return AddResult{
		Slug:   slug,
		Branch: branch,
		Path:   target,
		Pushed: true,
	}, nil
}

// rollbackAdd performs best-effort cleanup on Add failure.
// It continues through all steps even if one fails, returning nil if all steps succeed,
// or the first error encountered otherwise. All errors are best-effort (logged/ignored).
func (w *Worktree) rollbackAdd(l *paths.Layout, slug, branch, target string) error {
	var firstErr error

	// Remove portal
	if err := removePortal(l, slug); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}

	// Remove launchers
	if err := removeLaunchers(l, slug); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}

	// Remove worktree
	_, _, _, err := git.RunGit([]string{"worktree", "remove", "--force", target}, l.WorktreeRoot)
	if err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}

	// Delete branch
	_, _, _, err = git.RunGit([]string{"branch", "-D", branch}, l.WorktreeRoot)
	if err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}

	// Prune worktrees
	_, _, _, err = git.RunGit([]string{"worktree", "prune"}, l.WorktreeRoot)
	if err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}
