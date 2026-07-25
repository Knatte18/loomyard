// drift.go implements the stateless pair-in-sync check for fabric topology.
//
// PairInSync derives the weft sibling deterministically and checks that the weft
// worktree is on WeftBranchName(hostBranch), and that the host _lyx junction is
// valid and points to the weft _lyx directory. Adapted from warpengine's drift.go
// — same stateless, no-registry-consulted design, package fabricengine. The
// branch delta: fabric's correspondence check compares the weft branch against
// WeftBranchName(hostBranch) rather than requiring the two names to be equal.
//
// PairInSync and HostClean (hostclean.go) are dead-until-cutover exports by
// design — the differential lifecycle tests (a later card in this batch) are
// their consumers for now; a future cutover task wires them into the loom
// preflight the way warpengine's versions are wired today.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// PairInSync reports whether the host worktree and its paired weft worktree are in sync.
//
// A pair is considered in sync when:
//   - The weft worktree is on WeftBranchName(hostBranch) (via rev-parse --abbrev-ref HEAD
//     on both worktrees)
//   - The host _lyx junction exists and points to the correct weft _lyx directory
//
// The weft sibling is derived deterministically as <worktree-base>-weft (via paths geometry).
// No registry or status.md is consulted; PairInSync is stateless.
//
// Returns (true, "", nil) if the pair is in sync.
// Returns (false, reason, nil) if the pair is out of sync; reason describes the divergence.
// Returns (false, "", err) if the check encounters a system error (e.g., git failure, stat error).
func PairInSync(l *hubgeometry.Layout) (ok bool, reason string, err error) {
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

	// Check branch correspondence: the weft branch must be the suffixed sibling of the
	// host branch, not merely an equal name (fabric's uniform <host>/<host>-weft scheme).
	expectedWeftBranch := WeftBranchName(hostBranch)
	if weftBranch != expectedWeftBranch {
		return false, fmt.Sprintf("host on %s, weft on %s (want %s)", hostBranch, weftBranch, expectedWeftBranch), nil
	}

	// Verify the host _lyx junction is valid and points to the correct weft target.
	hostLink := l.HostLyxLinkHere()
	weftTarget := l.WeftLyxDir()

	// Distinguish a missing _lyx entry from an existing one that is not a
	// link: fslink.IsLink reports (false, nil) for both shapes, and the loom
	// preflight consumes these reason strings — a real directory sitting
	// where the junction belongs must not masquerade as merely missing.
	if _, lstatErr := os.Lstat(hostLink); lstatErr != nil {
		if os.IsNotExist(lstatErr) {
			return false, "junction missing", nil
		}
		return false, "", fmt.Errorf("check host junction: %w", lstatErr)
	}
	isLink, err := fslink.IsLink(hostLink)
	if err != nil {
		return false, "", fmt.Errorf("check host junction: %w", err)
	}
	if !isLink {
		// Same wording as checkJunctionHealth for this drift shape, so
		// status/reconcile and PairInSync describe it identically.
		return false, "host _lyx is not a junction", nil
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
