// status.go — weft status reporting including drift and junction integrity.

package weft

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/git"
)

// Status returns a status report for the weft worktree, including branch, dirty state,
// upstream tracking, and junction integrity checks.
//
// Returns a map with keys:
//   - weft_worktree: the path to the weft worktree
//   - branch: current branch name (via git rev-parse --abbrev-ref HEAD)
//   - dirty: bool indicating whether pathspec has uncommitted changes
//   - ahead: int (null if no upstream)
//   - behind: int (null if no upstream)
//   - junction_ok: bool
//   - junction_reason: string (empty when ok)
//
// Status completes and returns even if junction_ok=false (config and git are resolved
// from the weft worktree).
func Status(weftWorktree, hostLink, weftLyxDir string, pathspec []string) (map[string]any, error) {
	result := make(map[string]any)

	// weft_worktree path
	result["weft_worktree"] = weftWorktree

	// Get branch name
	branch, _, code, err := git.RunGit([]string{"rev-parse", "--abbrev-ref", "HEAD"}, weftWorktree)
	if err != nil {
		return nil, fmt.Errorf("rev-parse --abbrev-ref HEAD: %w", err)
	}
	if code != 0 {
		return nil, fmt.Errorf("rev-parse failed")
	}
	result["branch"] = strings.TrimSpace(branch)

	// Check if dirty (has uncommitted changes in pathspec)
	args := append([]string{"status", "--porcelain", "--"}, pathspec...)
	dirty, _, code, err := git.RunGit(args, weftWorktree)
	if err != nil {
		return nil, fmt.Errorf("status: %w", err)
	}
	if code != 0 {
		return nil, fmt.Errorf("status failed")
	}
	result["dirty"] = strings.TrimSpace(dirty) != ""

	// Check upstream tracking (ahead/behind)
	aheadOut, _, code, err := git.RunGit([]string{"rev-list", "--count", "@{u}..HEAD"}, weftWorktree)
	if err != nil {
		return nil, fmt.Errorf("rev-list ahead: %w", err)
	}
	if code == 0 {
		// Upstream exists
		var ahead int
		fmt.Sscanf(strings.TrimSpace(aheadOut), "%d", &ahead)
		result["ahead"] = ahead

		behindOut, _, code, err := git.RunGit([]string{"rev-list", "--count", "HEAD..@{u}"}, weftWorktree)
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
		// No upstream
		result["ahead"] = nil
		result["behind"] = nil
	}

	// Check junction integrity
	junctionOk, junctionReason := checkJunction(hostLink, weftLyxDir)
	result["junction_ok"] = junctionOk
	result["junction_reason"] = junctionReason

	return result, nil
}

// checkJunction verifies that hostLink is a junction/symlink pointing to weftLyxDir.
// Returns (ok, reason) where ok is true only if the junction is correctly set up.
func checkJunction(hostLink, weftLyxDir string) (bool, string) {
	// Check if hostLink exists
	_, err := os.Lstat(hostLink)
	if err != nil {
		if os.IsNotExist(err) {
			return false, "host _lyx junction missing"
		}
		return false, fmt.Sprintf("lstat error: %v", err)
	}

	// Check if it's a junction/symlink using fslink.IsLink
	isLink, err := fslink.IsLink(hostLink)
	if err != nil || !isLink {
		return false, "host _lyx is not a junction"
	}

	// Resolve both ends and compare
	hostResolved, err := fslink.PointsTo(hostLink)
	if err != nil {
		return false, fmt.Sprintf("EvalSymlinks(hostLink) error: %v", err)
	}

	weftResolved, err := filepath.EvalSymlinks(filepath.Clean(weftLyxDir))
	if err != nil {
		return false, fmt.Sprintf("EvalSymlinks(weftLyxDir) error: %v", err)
	}

	// Normalize paths for comparison
	hostResolved = filepath.Clean(hostResolved)
	weftResolved = filepath.Clean(weftResolved)

	if hostResolved != weftResolved {
		return false, "host _lyx junction points elsewhere"
	}

	return true, ""
}
