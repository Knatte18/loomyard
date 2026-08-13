// gitwrap.go implements webster's own git-query helpers over internal/gitrepo: headSHA captures a
// batch's start-SHA and the report cross-check's actual HEAD, and dirty is the half-done-work
// signal.
// Per the Shared Decision git-verification-via-gitrepo, every helper here goes through gitrepo.Repo
// except dirty, which gitrepo exposes no porcelain/status method for and so wraps gitexec.Run
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
// It wraps gitexec.Run directly since gitrepo.Repo exposes no porcelain/status method.
func dirty(worktree string) (bool, error) {
	stdout, err := gitexec.Run([]string{"status", "--porcelain"}, worktree)
	if err != nil {
		return false, fmt.Errorf("websterengine: git status --porcelain in %s: %w", worktree, err)
	}
	return strings.TrimSpace(stdout) != "", nil
}
