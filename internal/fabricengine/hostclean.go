// hostclean.go implements a standalone worktree-pair cleanliness check, a
// package-level Clean used by loomengine.Preflight to determine whether both
// sides of a host/weft pair have any dirty (uncommitted or untracked) paths
// before a loom phase transition proceeds.

package fabricengine

import (
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// Clean reports whether both the host worktree at l.WorktreeRoot and its
// paired weft worktree at l.WeftWorktree() have no dirty paths at all —
// tracked or untracked. It is a package-level function, not a *Topology
// method, because loomengine.Preflight calls it standalone against an
// already-resolved Layout with no need for a Topology's config.
//
// Each side is checked with `git status --porcelain` and no
// --untracked-files flag, deliberately stricter than add.go's pre-Add clean
// check (--untracked-files=no): an untracked file left behind still counts
// as dirty here. This is the Weft Git Invariant's host-repo-is-unrestricted
// rationale in the other direction — the host worktree is the one place
// Loomyard's own tooling never commits or gitignores on the caller's
// behalf, so Preflight must surface even a stray untracked file rather than
// silently treating it as clean; the weft worktree gets the same treatment
// for symmetry.
//
// The weft-side check is skipped entirely — not attempted and not an
// error — when l.WeftWorktree() does not exist on disk, since a missing
// weft worktree is a distinct, prior condition
// (loomengine.CheckWeftPairing) that this function must not turn into an
// infra error.
//
// Returns (false, "", err) if a git spawn itself fails or exits non-zero
// (wrapped with context — a "couldn't determine" infra failure, not a
// determined dirty verdict). On success, clean reports whether both sides'
// porcelain output was empty; when clean is false, reason describes which
// side(s) are dirty and lists the dirty paths.
func Clean(l *hubgeometry.Layout) (clean bool, reason string, err error) {
	hostReason, err := dirtyReason("git status --porcelain", l.WorktreeRoot)
	if err != nil {
		return false, "", err
	}

	var weftReason string
	if _, statErr := os.Stat(l.WeftWorktree()); statErr == nil {
		weftReason, err = dirtyReason("git status --porcelain", l.WeftWorktree())
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
