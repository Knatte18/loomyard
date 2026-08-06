// gitwrap.go re-points webster's git-query helpers at internal/gitrepo,
// replacing the helpers builderengine/gitquery.go implements locally over
// internal/gitexec directly: headSHA captures a batch's start-SHA and the
// report cross-check's actual HEAD, and dirty is the half-done-work
// signal. Per the Shared Decision git-verification-via-gitrepo, every
// helper here goes through gitrepo.Repo except dirty, which gitrepo
// exposes no porcelain/status method for and so wraps gitexec.RunGit
// directly — the one carved-out exception the decision names.

package websterengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// headSHA returns worktree's current HEAD commit SHA via gitrepo.Repo.
// An unborn HEAD surfaces as gitrepo.ErrNoCommits.
func headSHA(worktree string) (string, error) {
	sha, err := gitrepo.New(worktree).CurrentSHA()
	if err != nil {
		return "", fmt.Errorf("websterengine: head sha in %s: %w", worktree, err)
	}
	return sha, nil
}

// dirty reports whether worktree has any uncommitted or untracked changes.
// It wraps gitexec.RunGit directly since gitrepo.Repo exposes no porcelain/status method.
func dirty(worktree string) (bool, error) {
	stdout, stderr, exitCode, err := gitexec.RunGit([]string{"status", "--porcelain"}, worktree)
	if err != nil {
		return false, fmt.Errorf("websterengine: git status --porcelain in %s: %w", worktree, err)
	}
	if exitCode != 0 {
		return false, fmt.Errorf("websterengine: git status --porcelain in %s failed: %s", worktree, strings.TrimSpace(stderr))
	}
	return strings.TrimSpace(stdout) != "", nil
}
