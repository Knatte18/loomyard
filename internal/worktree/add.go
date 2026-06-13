package worktree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/mhgo/internal/git"
)

// AddResult contains the result of successfully adding a new worktree.
type AddResult struct {
	Slug   string `json:"slug"`
	Branch string `json:"branch"`
	Path   string `json:"path"`
	Pushed bool   `json:"pushed"`
}

// Add creates a new git worktree with the given slug in a sibling directory.
//
// The sourceDir is the existing worktree to work from (e.g., the main checkout).
// It must be a clean git repository with at least one remote configured.
// The slug becomes the final path component; the branch name is formed by prepending
// the configured BranchPrefix.
//
// Steps:
//  1. Clean check: sourceDir must have no uncommitted changes.
//  2. Branch name: branch := w.cfg.BranchPrefix + slug
//  3. Branch-exists check: branch must not already exist.
//  4. Target path: sibling directory named slug; must not exist.
//  5. Remote check: must have at least one remote configured.
//  6. Create: git worktree add -b <branch> <target>
//  7. Push: git push -u origin <branch>
//
// Returns AddResult on success or an error if any step fails.
func (w *Worktree) Add(sourceDir, slug string) (AddResult, error) {
	// (1) Clean check
	stdout, stderr, exitCode, err := git.RunGit([]string{"status", "--porcelain", "--untracked-files=no"}, sourceDir)
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
	_, _, exitCode, err = git.RunGit([]string{"rev-parse", "--verify", "refs/heads/" + branch}, sourceDir)
	if err != nil {
		return AddResult{}, fmt.Errorf("cwd is not a valid git worktree")
	}
	if exitCode == 0 {
		return AddResult{}, fmt.Errorf("branch %q already exists", branch)
	}

	// (4) Target path check
	container := filepath.Dir(sourceDir)
	target := filepath.Join(container, slug)
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		return AddResult{}, fmt.Errorf("worktree directory %q already exists", target)
	}

	// (5) Remote check
	stdout, _, exitCode, err = git.RunGit([]string{"remote"}, sourceDir)
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
	_, stderr, exitCode, err = git.RunGit([]string{"worktree", "add", "-b", branch, target}, sourceDir)
	if err != nil {
		return AddResult{}, fmt.Errorf("cwd is not a valid git worktree")
	}
	if exitCode != 0 {
		return AddResult{}, fmt.Errorf("worktree add failed: %s", stderr)
	}

	// (7) Push
	_, stderr, exitCode, err = git.RunGit([]string{"push", "-u", "origin", branch}, sourceDir)
	if err != nil {
		return AddResult{}, fmt.Errorf("cwd is not a valid git worktree")
	}
	if exitCode != 0 {
		return AddResult{}, fmt.Errorf("push failed: %s", stderr)
	}

	return AddResult{
		Slug:   slug,
		Branch: branch,
		Path:   target,
		Pushed: true,
	}, nil
}
