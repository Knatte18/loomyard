// reconcile.go runs the hub's origin reconcile ahead of cleanup's segments: the seams it takes, and the refusal text of a stop on the branches every pair shares.

package cleanupcli

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hibernateengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/tendercli"
	"github.com/Knatte18/loomyard/internal/tenderengine"
)

// The repository sides a branch outcome names that a stop on is shared by every pair.
const (
	sideBolt    = "bolt"
	sideRecords = "records"
)

// reentry is what a stop that cleanup's own repair leaves undone is followed by.
const reentry = "then re-run lyx cleanup --apply from the prime"

// reconcileHub runs the hub-scope origin reconcile from the prime, judging only unless apply is set.
func reconcileHub(prime *lyxcwd.Location, apply bool) (fabricengine.OriginReport, error) {
	teardown, err := pairteardown.New(prime)
	if err != nil {
		return fabricengine.OriginReport{}, err
	}
	scope := fabricengine.ReconcileScope{JudgeOnly: !apply}
	return fabricengine.ReconcileOrigin(prime, scope, teardown.OriginSeams(cleanupSeamOptions(prime)), fabricengine.ReconcileOptions{})
}

// cleanupSeamOptions returns the hub's seam options extended with the wake hold.
// WakeHeld reads the pause record for a live entry of the slug whose caller is the hibernate daemon, and LiftWakeHold lifts that entry by identity through tender.
func cleanupSeamOptions(prime *lyxcwd.Location) pairteardown.SeamOptions {
	opts := pairteardown.HubSeamOptions(prime)
	opts.WakeHeld = func(slug string) (bool, error) {
		_, held, err := wakeEntry(prime, slug)
		return held, err
	}
	opts.LiftWakeHold = func(slug string) error {
		entry, held, err := wakeEntry(prime, slug)
		if err != nil || !held {
			return err
		}
		deps, _, err := tendercli.HubDeps(prime, hibernateengine.PauseCaller)
		if err != nil {
			return err
		}
		_, err = tenderengine.Lift(deps, tenderengine.EntryRef{Scope: entry.Scope, Caller: entry.Caller, PausedAt: entry.PausedAt})
		return err
	}
	return opts
}

// wakeEntry returns the live pause entry the hibernate daemon wrote for slug, and whether there is one.
func wakeEntry(prime *lyxcwd.Location, slug string) (pauserecord.Entry, bool, error) {
	geometry, err := hubgeom.PauseGeometry(prime)
	if err != nil {
		return pauserecord.Entry{}, false, err
	}
	record, err := pauserecord.Read(geometry.RecordPath)
	if err != nil {
		return pauserecord.Entry{}, false, err
	}
	for _, live := range record.Live {
		if live.Scope == slug && live.Caller == hibernateengine.PauseCaller {
			return live, true, nil
		}
	}
	return pauserecord.Entry{}, false, nil
}

// sharedStopText renders each stop on Bolt or the prime's records branch as its way forward, followed by the re-entry where one retries it.
// It is empty when neither branch stopped.
func sharedStopText(report fabricengine.OriginReport) string {
	var stops []string
	for _, branch := range report.Branches {
		shared := branch.Repo == sideBolt || (branch.Repo == sideRecords && branch.Prime)
		if !shared {
			continue
		}
		reenters := false
		switch branch.State {
		case fabricengine.BranchDirty, fabricengine.BranchGone:
			reenters = true
		case fabricengine.BranchBusy:
			reenters = !branch.PushLockHeld
		case fabricengine.BranchRewound, fabricengine.BranchRewritten, fabricengine.BranchDiverged, fabricengine.BranchAnchorStopped:
		default:
			continue
		}
		line := fmt.Sprintf("%s (%s): %s", branch.Branch, branch.State, branch.WayForward)
		if reenters {
			line += "; " + reentry
		}
		stops = append(stops, line)
	}
	return strings.Join(stops, "; ")
}
