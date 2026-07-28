// drift.go implements the stateless pair-in-sync check for fabric topology.
//
// PairInSync derives the weft sibling deterministically and checks that the weft
// worktree is on WeftBranchName(hostBranch), and that every host junction
// (l.HostJunctionsHere()) is valid and points to its own weft directory. It is
// stateless, consulting no registry: fabric's correspondence check compares the
// weft branch against WeftBranchName(hostBranch).
//
// PairInSync and HostClean (hostclean.go) are wired into the loom preflight
// via internal/loomengine.

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
//   - Every host junction in l.HostJunctionsHere() exists and points to its own
//     weft directory
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

	// Verify every host junction is valid and points to its correct weft
	// target — l.HostJunctionsHere(), the same Here-anchored, slug-free
	// accessor checkJunctionHealth loops in reconcile.go. PairInSync's
	// signature is unchanged and it stays stateless and slug-free, which is
	// exactly why it loops HostJunctionsHere() rather than HostJunctions(slug).
	for _, j := range l.HostJunctionsHere() {
		// Distinguish a missing junction entry from an existing one that is not
		// a link: fslink.IsLink reports (false, nil) for both shapes, and the
		// loom preflight consumes these reason strings — a real directory
		// sitting where the junction belongs must not masquerade as merely
		// missing.
		if _, lstatErr := os.Lstat(j.Link); lstatErr != nil {
			if os.IsNotExist(lstatErr) {
				return false, fmt.Sprintf("host %s junction missing", j.Name), nil
			}
			return false, "", fmt.Errorf("check host junction: %w", lstatErr)
		}
		isLink, err := fslink.IsLink(j.Link)
		if err != nil {
			return false, "", fmt.Errorf("check host junction: %w", err)
		}
		if !isLink {
			// Same wording as checkJunctionHealth for this drift shape, so
			// status/reconcile and PairInSync describe it identically.
			return false, fmt.Sprintf("host %s is not a junction", j.Name), nil
		}

		// Resolve the junction and verify it points to the correct target.
		linkTarget, err := fslink.PointsTo(j.Link)
		if err != nil {
			return false, "", fmt.Errorf("resolve host junction: %w", err)
		}

		// Resolve weft target for comparison.
		weftTargetResolved, err := filepath.EvalSymlinks(j.Target)
		if err != nil {
			return false, "", fmt.Errorf("resolve weft target: %w", err)
		}

		if linkTarget != weftTargetResolved {
			return false, fmt.Sprintf("host %s junction points elsewhere", j.Name), nil
		}
	}

	return true, "", nil
}
