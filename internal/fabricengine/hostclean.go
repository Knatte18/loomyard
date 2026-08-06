// hostclean.go implements a standalone worktree-pair cleanliness check, a package-level Clean used by loomengine.Preflight to determine whether both sides of a host/weft pair have any dirty (uncommitted or untracked) paths before a loom phase transition proceeds.

package fabricengine

import (
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// Clean reports whether both the host and weft worktrees have no dirty paths, including untracked files.
// It is package-level for use by loomengine.Preflight.
// The weft-side check is skipped when the weft worktree does not exist.
// Returns (false, reason, nil) when dirty or (false, "", err) for system errors.
func Clean(l *lyxcwd.Location) (clean bool, reason string, err error) {
	hostReason, err := dirtyReason("git status --porcelain", l.WorktreePath())
	if err != nil {
		return false, "", err
	}

	var weftReason string
	if _, statErr := os.Stat(WeftWorktree(l)); statErr == nil {
		weftReason, err = dirtyReason("git status --porcelain", WeftWorktree(l))
		if err != nil {
			return false, "", err
		}
	} else if !os.IsNotExist(statErr) {
		return false, "", statErr
	}

	var reasons []string
	if hostReason != "" {
		reasons = append(reasons, fmt.Sprintf("host: %s", hostReason))
	}
	if weftReason != "" {
		reasons = append(reasons, fmt.Sprintf("weft: %s", weftReason))
	}
	if len(reasons) == 0 {
		return true, "", nil
	}
	return false, strings.Join(reasons, "; "), nil
}

// dirtyReason runs `git status --porcelain` at dir and returns its trimmed
// output — empty when clean, non-empty when dirty. label names the command
// in wrapped errors so a host-vs-weft spawn failure is distinguishable.
func dirtyReason(label, dir string) (string, error) {
	stdout, _, exitCode, err := gitexec.RunGit([]string{"status", "--porcelain"}, dir)
	if err != nil {
		return "", fmt.Errorf("%s: %w", label, err)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("%s failed with exit code %d", label, exitCode)
	}
	return strings.TrimSpace(stdout), nil
}
