// drift.go implements the stateless pair-in-sync check for warp topology.
//
// PairInSync derives the weft sibling deterministically and checks that both
// the host and weft worktrees are on the same branch, and that the host _lyx
// junction is valid and points to the weft _lyx directory.

package warpengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/paths"
)

// PairInSync reports whether the host worktree and its paired weft worktree are in sync.
//
// A pair is considered in sync when:
//   - The host and weft worktrees are both on the same branch (via rev-parse --abbrev-ref HEAD)
//   - The host _lyx junction exists and points to the correct weft _lyx directory
//
// The weft sibling is derived deterministically as <worktree-base>-weft (via paths geometry).
// No registry or status.md is consulted; PairInSync is stateless.
//
// Returns (true, "", nil) if the pair is in sync.
// Returns (false, reason, nil) if the pair is out of sync; reason describes the divergence.
// Returns (false, "", err) if the check encounters a system error (e.g., git failure, stat error).
func PairInSync(l *paths.Layout) (ok bool, reason string, err error) {
	// Verify the host worktree's current branch via rev-parse --abbrev-ref HEAD.
	hostOut, _, exitCode, err := gitexec.RunGit(
		[]string{"rev-parse", "--abbrev-ref", "HEAD"},
		l.WorktreeRoot,
	)
	if err != nil {
		return false, "", fmt.Errorf("get host branch: %w", err)
	}
	if exitCode != 0 {
		return false, "", fmt.Errorf("get host branch failed with exit code %d", exitCode)
	}
	hostBranch := strings.TrimSpace(hostOut)

	// Verify the weft worktree's current branch via rev-parse --abbrev-ref HEAD.
	weftWorktree := l.WeftWorktree()
	weftOut, _, exitCode, err := gitexec.RunGit(
		[]string{"rev-parse", "--abbrev-ref", "HEAD"},
		weftWorktree,
	)
	if err != nil {
		return false, "", fmt.Errorf("get weft branch: %w", err)
	}
	if exitCode != 0 {
		return false, "", fmt.Errorf("get weft branch failed with exit code %d", exitCode)
	}
	weftBranch := strings.TrimSpace(weftOut)

	// Check branch divergence.
	if hostBranch != weftBranch {
		return false, fmt.Sprintf("host on %s, weft on %s", hostBranch, weftBranch), nil
	}

	// Verify the host _lyx junction is valid and points to the correct weft target.
	hostLink := l.HostLyxLinkHere()
	weftTarget := l.WeftLyxDir()

	// Check if the junction exists and is a link.
	isLink, err := fslink.IsLink(hostLink)
	if err != nil && !os.IsNotExist(err) {
		return false, "", fmt.Errorf("check host junction: %w", err)
	}
	if !isLink {
		if os.IsNotExist(err) {
			return false, "junction missing", nil
		}
		return false, "junction missing", nil
	}

	// Resolve the junction and verify it points to the correct target.
	linkTarget, err := fslink.PointsTo(hostLink)
	if err != nil {
		return false, "", fmt.Errorf("resolve host junction: %w", err)
	}

	// Resolve weft target for comparison.
	weftTargetResolved, err := filepath.EvalSymlinks(weftTarget)
	if err != nil {
		return false, "", fmt.Errorf("resolve weft target: %w", err)
	}

	if linkTarget != weftTargetResolved {
		return false, "junction points elsewhere", nil
	}

	return true, "", nil
}
