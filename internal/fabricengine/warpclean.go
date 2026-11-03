// warpclean.go implements a standalone worktree-pair cleanliness check, a package-level Clean used
// by preflight.CheckResolved to determine whether both sides of a warp/weft pair have any dirty
// (uncommitted or untracked) paths before a loom phase transition proceeds.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// Clean reports whether both the warp and weft worktrees have no dirty paths, including untracked
// files.
// It is package-level for use by preflight.CheckResolved.
// The weft-side check is skipped when the weft worktree does not exist.
// weftExempt names anchor-relative weft paths, or git pathspec globs, whose dirty content the weft-side check ignores, everything beneath each included.
// Returns (false, reason, nil) when dirty or (false, "", err) for system errors.
func Clean(l *lymcwd.Location, weftExempt ...string) (clean bool, reason string, err error) {
	warpReason, err := dirtyReason("git status --porcelain", l.WorktreePath())
	if err != nil {
		return false, "", err
	}

	var weftReason string
	if _, statErr := os.Stat(RecordsWorktree(l)); statErr == nil {
		weftReason, err = dirtyReason("git status --porcelain", RecordsWorktree(l), exemptPathspec(l.AnchorRel, weftExempt)...)
		if err != nil {
			return false, "", err
		}
	} else if !os.IsNotExist(statErr) {
		return false, "", statErr
	}

	var reasons []string
	if warpReason != "" {
		reasons = append(reasons, fmt.Sprintf("uncommitted code changes: %s", warpReason))
	}
	if weftReason != "" {
		reasons = append(reasons, fmt.Sprintf("uncommitted state changes under `_lym`: %s", weftReason))
	}
	if len(reasons) == 0 {
		return true, "", nil
	}
	return false, strings.Join(reasons, "; "), nil
}

// exemptPathspec returns the status pathspec that matches the whole worktree but each of the anchor-relative exempt paths, scoped under anchorRel.
// It returns nil when exempt is empty, so the probe runs unscoped.
func exemptPathspec(anchorRel string, exempt []string) []string {
	if len(exempt) == 0 {
		return nil
	}
	pathspec := []string{"."}
	for _, path := range weftCommitPathspec(anchorRel, exempt) {
		pathspec = append(pathspec, ":(exclude)"+filepath.ToSlash(path))
	}
	return pathspec
}

// dirtyReason runs `git status --porcelain` at dir, limited to pathspec when given, and returns its trimmed output.
// The output is empty when clean and non-empty when dirty.
// label names the command in wrapped errors so a warp-vs-weft spawn failure is distinguishable.
func dirtyReason(label, dir string, pathspec ...string) (string, error) {
	_, detail, err := worktreeDirty(scopeAll, dir, pathspec...)
	if err != nil {
		return "", fmt.Errorf("%s: %w", label, err)
	}
	return detail, nil
}
