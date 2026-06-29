// status.go — weft content-sync status reporting.
//
// Junction topology reporting has been moved to internal/warpengine (warpengine is the topology
// owner). This file reports only content-sync state: branch, dirty flag, and upstream
// ahead/behind counts.

package weftengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// Status returns a content-sync status report for the weft worktree.
//
// Returns a map with keys:
//   - weft_worktree: the path to the weft worktree
//   - branch: current branch name (via git rev-parse --abbrev-ref HEAD)
//   - dirty: bool indicating whether pathspec has uncommitted changes
//   - ahead: int (null if no upstream)
//   - behind: int (null if no upstream)
//
// Junction integrity is no longer reported here; it is owned by internal/warpengine.
// Status completes and returns even when there is no upstream (ahead/behind are null).
func Status(weftWorktree string, pathspec []string) (map[string]any, error) {
	result := make(map[string]any)

	// Record the weft worktree path for the caller's convenience.
	result["weft_worktree"] = weftWorktree

	// Get branch name.
	branch, _, code, err := gitexec.RunGit([]string{"rev-parse", "--abbrev-ref", "HEAD"}, weftWorktree)
	if err != nil {
		return nil, fmt.Errorf("rev-parse --abbrev-ref HEAD: %w", err)
	}
	if code != 0 {
		return nil, fmt.Errorf("rev-parse failed")
	}
	result["branch"] = strings.TrimSpace(branch)

	// Check whether pathspec has uncommitted changes (dirty flag).
	args := append([]string{"status", "--porcelain", "--"}, pathspec...)
	dirty, _, code, err := gitexec.RunGit(args, weftWorktree)
	if err != nil {
		return nil, fmt.Errorf("status: %w", err)
	}
	if code != 0 {
		return nil, fmt.Errorf("status failed")
	}
	result["dirty"] = strings.TrimSpace(dirty) != ""

	// Check upstream tracking (ahead/behind). A non-zero exit from rev-list means no
	// upstream is configured, which is valid — report null in that case.
	aheadOut, _, code, err := gitexec.RunGit([]string{"rev-list", "--count", "@{u}..HEAD"}, weftWorktree)
	if err != nil {
		return nil, fmt.Errorf("rev-list ahead: %w", err)
	}
	if code == 0 {
		// Upstream exists; parse both counts.
		var ahead int
		fmt.Sscanf(strings.TrimSpace(aheadOut), "%d", &ahead)
		result["ahead"] = ahead

		behindOut, _, code, err := gitexec.RunGit([]string{"rev-list", "--count", "HEAD..@{u}"}, weftWorktree)
		if err != nil {
			return nil, fmt.Errorf("rev-list behind: %w", err)
		}
		if code != 0 {
			return nil, fmt.Errorf("rev-list behind failed")
		}
		var behind int
		fmt.Sscanf(strings.TrimSpace(behindOut), "%d", &behind)
		result["behind"] = behind
	} else {
		// No upstream configured; null signals the absence rather than a zero count.
		result["ahead"] = nil
		result["behind"] = nil
	}

	return result, nil
}
