// drift.go implements Healthy, the stateless pair-in-sync check for fabric topology: branch
// correspondence between a warp worktree and its weft sibling, plus every wired junction's health.
// Healthy and Clean (warpclean.go) are wired into loom's preflight via internal/preflight,
// consumed by internal/preflightshed.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// HealthCause names the specific condition a failed Healthy check found, letting a caller like
// preflight.CheckResolved classify the failure without parsing a display string.
type HealthCause string

// The closed set of causes Healthy can report, one per drift shape drift.go detects.
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

// HealthReason is the typed verdict Healthy returns for a failed check: Cause is the drift shape a
// caller switches on, and Detail is an already-fabric-worded display string a caller may print
// verbatim.
// The zero HealthReason (Cause == "") is what Healthy returns alongside ok == true;
// a caller that checks ok first never reads it.
type HealthReason struct {
	Cause  HealthCause
	Detail string
}

// Healthy reports whether the warp worktree and its paired weft worktree are in sync: the weft
// worktree is on the paired weft branch and every warp junction exists and points to its correct
// weft directory.
// The weft sibling is determined deterministically and no external state is consulted, so Healthy
// is stateless.
// Returns (true, HealthReason{}, nil) if in sync;
// (false, reason, nil) if out of sync;
// (false, HealthReason{}, err) if a system error occurs.
func Healthy(l *lyxcwd.Location) (ok bool, reason HealthReason, err error) {
	// Both branch reads go through readBranch (reconcile.go) rather than a bare
	// rev-parse --abbrev-ref HEAD, so an UNBORN branch — the weft primary's ordinary state
	// immediately after a clone against an empty remote — is answered rather than reported as an
	// aborted check. loom's preflight consumes this verdict, and a just-cloned hub must not make it
	// hard-error.
	warpBranch, err := readBranch(l.WorktreePath())
	if err != nil {
		return false, HealthReason{}, fmt.Errorf("get warp branch: %w", err)
	}

	weftWorktree := WeftWorktree(l)
	weftBranch, err := readBranch(weftWorktree)
	if err != nil {
		return false, HealthReason{}, fmt.Errorf("get weft branch: %w", err)
	}

	// Check branch correspondence: the weft branch must be the suffixed sibling of the
	// warp branch, not merely an equal name (fabric's uniform <warp>/<warp>-weft scheme).
	expectedWeftBranch := WeftBranchName(warpBranch)
	if weftBranch != expectedWeftBranch {
		return false, HealthReason{
			Cause:  CauseBranchMismatch,
			Detail: fmt.Sprintf("fabric out of sync: on %s (want %s)", warpBranch, expectedWeftBranch),
		}, nil
	}

	// Load the wired name-set from the repo-wide BoardDir base — durable and
	// independent of the warp junction whose health this function checks, and
	// the same repo-wide base checkJunctionHealth in reconcile.go uses. A load
	// failure is reported as a determinable "unhealthy: bad config" verdict
	// via CauseConfigLoadFailed, not a hard Go error — the caller keeps
	// treating it as a check failure rather than an aborted preflight.
	names, err := RepoWiredNames(l)
	if err != nil {
		return false, HealthReason{
			Cause:  CauseConfigLoadFailed,
			Detail: fmt.Sprintf("junction check unavailable: cannot load fabric.yaml: %v", err),
		}, nil
	}

	// Verify every warp junction is valid and points to its correct weft
	// target — WarpJunctionsHere(l, names), the same Here-anchored, slug-free
	// accessor checkJunctionHealth loops in reconcile.go.
	for _, j := range WarpJunctionsHere(l, names) {
		// Distinguish a missing junction entry from an existing one that is not
		// a link: fslink.IsLink reports (false, nil) for both shapes, and the
		// loom preflight consumes these typed reasons — a real directory
		// sitting where the junction belongs must not masquerade as merely
		// missing.
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
			// Same wording as checkJunctionHealth for this drift shape, so
			// status/reconcile and Healthy describe it identically.
			return false, HealthReason{
				Cause:  CauseNotAJunction,
				Detail: fmt.Sprintf("%s is not a junction", j.Name),
			}, nil
		}

		// Resolve the junction and verify it points to the correct target.
		linkTarget, err := fslink.PointsTo(j.Link)
		if err != nil {
			return false, HealthReason{}, fmt.Errorf("resolve warp junction: %w", err)
		}

		// Resolve weft target for comparison.
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
