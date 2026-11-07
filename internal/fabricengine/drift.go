// drift.go implements Healthy, the stateless check that a warp worktree and its weft sibling are in sync.
// It checks branch correspondence and the health of every wired junction.
// Healthy and Clean (warpclean.go) are wired into loom's preflight via internal/preflight.
// internal/preflightshed consumes them.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// HealthCause names the condition that made a health check fail,
// so a caller can classify the failure without parsing a display string.
type HealthCause string

// The closed set of causes a failed health check can report, one per drift shape.
const (
	// CauseBranchMismatch reports that the weft worktree is not on the paired weft branch.
	CauseBranchMismatch HealthCause = "branch-mismatch"
	// CauseConfigLoadFailed reports that the wired name-set could not be loaded from fabric.yaml.
	CauseConfigLoadFailed HealthCause = "config-load-failed"
	// CauseJunctionMissing reports that a warp junction entry does not exist on disk.
	CauseJunctionMissing HealthCause = "junction-missing"
	// CauseNotAJunction reports that a warp junction entry exists but is not a link.
	CauseNotAJunction HealthCause = "not-a-junction"
	// CauseJunctionPointsElsewhere reports that a warp junction link resolves to the wrong target.
	CauseJunctionPointsElsewhere HealthCause = "junction-points-elsewhere"
)

// HealthReason describes why a health check failed.
// Cause is the drift shape a caller switches on.
// Detail is a display string, worded for fabric, that a caller may print verbatim.
// The zero value carries no reason, as it does for a passing check or an error.
type HealthReason struct {
	Cause  HealthCause
	Detail string
}

// Healthy reports whether the warp worktree and its paired weft worktree are in sync.
// The weft worktree must be on the paired weft branch,
// and every warp junction must exist and point to its correct weft directory.
// The check keeps no state between calls: each call reads the branches, the wired names, and the junctions afresh.
// It returns (true, HealthReason{}, nil) when they are in sync.
// It returns (false, reason, nil) when they are out of sync.
// It returns (false, HealthReason{}, err) when a system error occurs.
func Healthy(l *lymcwd.Location) (ok bool, reason HealthReason, err error) {
	// Both branch reads go through readBranch, not a bare `rev-parse --abbrev-ref HEAD`.
	// readBranch answers an unborn branch, the state of the weft primary just after a clone of an empty remote.
	// loom's preflight consumes this verdict, and a just-cloned hub must not make it hard-error.
	warpBranch, err := readBranch(l.WorktreePath())
	if err != nil {
		return false, HealthReason{}, fmt.Errorf("get warp branch: %w", err)
	}

	weftWorktree := RecordsWorktree(l)
	weftBranch, err := readBranch(weftWorktree)
	if err != nil {
		return false, HealthReason{}, fmt.Errorf("get weft branch: %w", err)
	}

	// The weft branch must be the suffixed sibling of the warp branch, not merely an equal name.
	// The uniform fabric scheme is `<warp>/<warp>-weft`.
	expectedWeftBranch := RecordsBranchName(warpBranch)
	if weftBranch != expectedWeftBranch {
		return false, HealthReason{
			Cause:  CauseBranchMismatch,
			Detail: fmt.Sprintf("fabric out of sync: on %s (want %s)", warpBranch, expectedWeftBranch),
		}, nil
	}

	// The wired name-set loads from the repo-wide BoardDir base.
	// That base is durable and independent of the warp junctions checked below.
	// A load failure becomes a verdict with CauseConfigLoadFailed, not a Go error,
	// so the caller treats it as a failed check rather than an aborted preflight.
	names, err := RepoWiredNames(l)
	if err != nil {
		return false, HealthReason{
			Cause:  CauseConfigLoadFailed,
			Detail: fmt.Sprintf("junction check unavailable: cannot load fabric.yaml: %v", err),
		}, nil
	}

	// Verify every warp junction is valid and points to its correct weft target.
	// WarpJunctionsHere(l, names) lists them, through the Here-anchored, slug-free accessor.
	for _, j := range WarpJunctionsHere(l, names) {
		// Distinguish a missing junction entry from one that is not a link.
		// fslink.IsLink reports (false, nil) for both shapes, and the loom preflight consumes these typed reasons,
		// so a real directory where the junction belongs must not look merely missing.
		if _, lstatErr := os.Lstat(j.Link); lstatErr != nil {
			if os.IsNotExist(lstatErr) {
				return false, HealthReason{
					Cause:  CauseJunctionMissing,
					Detail: fmt.Sprintf("%s junction missing", j.Name),
				}, nil
			}
			return false, HealthReason{}, fmt.Errorf("check warp junction: %w", lstatErr)
		}
		isLink, err := fslink.IsLink(j.Link)
		if err != nil {
			return false, HealthReason{}, fmt.Errorf("check warp junction: %w", err)
		}
		if !isLink {
			// The status and reconcile paths word this drift shape the same way, so they describe it identically.
			return false, HealthReason{
				Cause:  CauseNotAJunction,
				Detail: fmt.Sprintf("%s is not a junction", j.Name),
			}, nil
		}

		linkTarget, err := fslink.PointsTo(j.Link)
		if err != nil {
			return false, HealthReason{}, fmt.Errorf("resolve warp junction: %w", err)
		}

		weftTargetResolved, err := filepath.EvalSymlinks(j.Target)
		if err != nil {
			return false, HealthReason{}, fmt.Errorf("resolve weft target: %w", err)
		}

		if linkTarget != weftTargetResolved {
			return false, HealthReason{
				Cause:  CauseJunctionPointsElsewhere,
				Detail: fmt.Sprintf("%s junction points elsewhere", j.Name),
			}, nil
		}
	}

	return true, HealthReason{}, nil
}
