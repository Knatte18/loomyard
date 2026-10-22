// reconcile.go runs the hub's origin reconcile for the hibernate daemon's wake and maps its report onto the engine's own view of it.

package hibernatecli

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hibernateengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/pairteardown"
)

const (
	// wakeRetryBound is how long the wake reconcile retries an unreachable origin before it reports so.
	wakeRetryBound = 2 * time.Minute
	// wakeRetryInterval is the wait between two attempts within the bound.
	wakeRetryInterval = 10 * time.Second
	// wakeAttemptDeadline bounds each remote call of one attempt.
	wakeAttemptDeadline = 20 * time.Second
)

// The repository sides a branch outcome names whose stop is shared by every pair.
const (
	sideBolt    = "bolt"
	sideRecords = "records"
)

// stopsEntries reports whether a shared branch in this state stops every entry; a busy one does not, since a lock holder ends on its own.
func stopsEntries(state fabricengine.BranchState) bool {
	switch state {
	case fabricengine.BranchRewound, fabricengine.BranchRewritten, fabricengine.BranchDiverged, fabricengine.BranchGone, fabricengine.BranchDirty, fabricengine.BranchLive:
		return true
	}
	return false
}

// wakeReconcile runs the hub-scope origin reconcile from the prime, moving branches, and returns the engine's view of its report.
func wakeReconcile(location *lyxcwd.Location) (hibernateengine.WakeReconcile, error) {
	teardown, err := pairteardown.New(location)
	if err != nil {
		return hibernateengine.WakeReconcile{}, err
	}
	options := fabricengine.ReconcileOptions{RetryBound: wakeRetryBound, RetryInterval: wakeRetryInterval, AttemptDeadline: wakeAttemptDeadline}
	report, err := fabricengine.ReconcileOrigin(location, fabricengine.ReconcileScope{}, teardown.OriginSeams(pairteardown.HubSeamOptions(location)), options)
	return wakeReconcileFromReport(report), err
}

// wakeReconcileFromReport maps a reconcile report onto the engine's view.
// The stop is the way forward of the first branch every pair shares, Bolt or the prime's records branch, that stops and is not merely busy.
// A run is each stopped entry, each ended entry that has a local pair and each owned_elsewhere entry, which always has a local pair; its way forward is the report's, never composed here.
// Mutations are rendered as kind and target, with the detail in parentheses when there is one.
func wakeReconcileFromReport(report fabricengine.OriginReport) hibernateengine.WakeReconcile {
	wake := hibernateengine.WakeReconcile{Unreachable: report.Unreachable}
	for _, branch := range report.Branches {
		shared := branch.Repo == sideBolt || (branch.Repo == sideRecords && branch.Prime)
		if shared && stopsEntries(branch.State) {
			wake.Stop = branch.WayForward
			break
		}
	}

	for _, entry := range report.Entries {
		run := hibernateengine.WakeRun{Slug: entry.Slug, WayForward: entry.WayForward}
		switch entry.State {
		case fabricengine.EntryEnded:
			if !entry.Local {
				continue
			}
			run.Ended, run.Busy = true, entry.Busy
		case fabricengine.EntryStopped:
			for _, branch := range report.Branches {
				if branch.Slug != entry.Slug {
					continue
				}
				switch branch.State {
				case fabricengine.BranchBusy:
					run.Busy = true
					run.ParkedPush = run.ParkedPush || branch.PushLockHeld
				case fabricengine.BranchGone:
					run.Gone = true
				}
			}
		case fabricengine.EntryOwnedElsewhere:
			run.Owner = entry.Owner
		default:
			continue
		}
		wake.Runs = append(wake.Runs, run)
	}

	for _, mutation := range report.Mutated().Entries() {
		line := fmt.Sprintf("%s %s", mutation.Kind, mutation.Target)
		if mutation.Detail != "" {
			line += fmt.Sprintf(" (%s)", mutation.Detail)
		}
		wake.Mutations = append(wake.Mutations, line)
	}
	return wake
}
