// drift.go implements the stateless pair-in-sync check for fabric topology.
//
// PairInSync derives the weft sibling deterministically and checks that the weft
// worktree is on WeftBranchName(hostBranch), and that every host junction
// (l.HostJunctionsHere(names)) is valid and points to its own weft directory. It
// loads the repo-wide fabric.yaml at hubgeometry.BoardDir(l.Hub) for the junction
// name-set; it still consults no registry/status.md: fabric's correspondence
// check compares the weft branch against WeftBranchName(hostBranch). A
// config-load failure is reported as a junction-check-unavailable reason (not a
// hard error), deliberately containing the "junction" substring the loom
// preflight classifier keys on — see below.
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
//   - Every host junction in l.HostJunctionsHere(names) exists and points to its own
//     weft directory, where names is the wired name-set loaded from the repo-wide
//     BoardDir fabric.yaml
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

	// Load the wired name-set from the repo-wide BoardDir base — durable and
	// independent of the host junction whose health this function checks, and
	// the same repo-wide base checkJunctionHealth in reconcile.go uses. A load
	// failure is reported as a determinable "pair unhealthy: bad config"
	// verdict REASON, not a hard Go error: internal/loomengine/preflight.go:
	// 120-123 propagates a non-nil err straight into an infra-escalating
	// `return Report{}, err`, which a missing/corrupt fabric.yaml does not
	// warrant. The reason string must also contain the substring "junction":
	// preflight.go:125-148's check-3 classifier sets check3BlocksSeed = true
	// only when strings.Contains(reason, "junction") (its own godoc warns any
	// reword must keep that substring), and an undeterminable junction set is
	// exactly the case that must block seed so check 4 reports
	// CheckSeedUnreadable rather than a phantom CheckSeedMissing.
	names, err := RepoWiredNames(l)
	if err != nil {
		return false, fmt.Sprintf("host junction check unavailable: cannot load fabric.yaml: %v", err), nil
	}

	// Verify every host junction is valid and points to its correct weft
	// target — l.HostJunctionsHere(names), the same Here-anchored, slug-free
	// accessor checkJunctionHealth loops in reconcile.go. PairInSync's
	// signature is unchanged and it stays stateless and slug-free, which is
	// exactly why it loops HostJunctionsHere(names) rather than
	// HostJunctions(slug, names).
	for _, j := range l.HostJunctionsHere(names) {
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
