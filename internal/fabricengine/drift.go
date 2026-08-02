// drift.go implements Healthy, the stateless pair-in-sync check for fabric
// topology: branch correspondence between a host worktree and its weft
// sibling, plus every wired junction's health. Healthy and Clean
// (hostclean.go) are wired into the loom preflight via internal/loomengine.

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

// Healthy reports whether the host worktree and its paired weft worktree are
// in sync: the weft worktree is on WeftBranchName(hostBranch), and every host
// junction in l.HostJunctionsHere(names) exists and points to its own weft
// directory, where names is the wired name-set loaded from the repo-wide
// BoardDir fabric.yaml. The weft sibling is derived deterministically as
// <worktree-base>-weft; no registry or status.md is consulted, so Healthy is
// stateless.
//
// Returns (true, "", nil) if the pair is in sync.
// Returns (false, reason, nil) if the pair is out of sync; reason describes
// the divergence. A config-load failure is reported this way too — as a
// "junction check unavailable" reason, not a hard error — deliberately
// containing the substring "junction" the loom preflight classifier keys on
// (preflight.go's check-3 classification).
// Returns (false, "", err) if the check encounters a system error (e.g., git
// failure, stat error).
func Healthy(l *hubgeometry.Layout) (ok bool, reason string, err error) {
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
	// failure is reported as a determinable "unhealthy: bad config" verdict
	// reason, not a hard Go error — see the doc comment above for why the
	// reason string must keep the substring "junction".
	names, err := RepoWiredNames(l)
	if err != nil {
		return false, fmt.Sprintf("host junction check unavailable: cannot load fabric.yaml: %v", err), nil
	}

	// Verify every host junction is valid and points to its correct weft
	// target — l.HostJunctionsHere(names), the same Here-anchored, slug-free
	// accessor checkJunctionHealth loops in reconcile.go.
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
			// status/reconcile and Healthy describe it identically.
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
