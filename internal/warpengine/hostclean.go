// hostclean.go implements a standalone host-worktree cleanliness check used by
// loomengine.Preflight to determine whether the host worktree has any dirty
// (uncommitted or untracked) paths before a loom phase transition proceeds.

package warpengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// HostClean reports whether the host worktree at l.WorktreeRoot has no dirty
// paths at all — tracked or untracked. It is a package-level function, not a
// *Worktree method, because loomengine.Preflight calls it standalone against
// an already-resolved Layout with no need for a Worktree's config.
//
// It runs `git status --porcelain` with no --untracked-files flag, which is
// deliberately stricter than add.go's pre-Add clean check
// (--untracked-files=no): an untracked file left behind in the host worktree
// still counts as dirty here. This is the Weft Git Invariant's
// host-repo-is-unrestricted rationale in the other direction — the host
// worktree is the one place Loomyard's own tooling never commits or
// gitignores on the caller's behalf, so Preflight must surface even a stray
// untracked file rather than silently treating it as clean.
//
// Returns (false, "", err) if the git spawn itself fails or git exits
// non-zero (wrapped with context — a "couldn't determine" infra failure, not
// a determined dirty verdict). On a successful git invocation, clean reports
// whether the porcelain output was empty; when clean is false, reason is the
// trimmed porcelain output so the operator can see exactly which paths are
// dirty.
func HostClean(l *hubgeometry.Layout) (clean bool, reason string, err error) {
	stdout, _, exitCode, err := gitexec.RunGit([]string{"status", "--porcelain"}, l.WorktreeRoot)
	if err != nil {
		return false, "", fmt.Errorf("git status --porcelain: %w", err)
	}
	if exitCode != 0 {
		return false, "", fmt.Errorf("git status --porcelain failed with exit code %d", exitCode)
	}

	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return true, "", nil
	}
	return false, trimmed, nil
}
