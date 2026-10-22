// wake.go implements what the daemon does between the wake and the lift of its pause entry: the hub's origin reconcile under a network exemption, the pair pause entries it leaves behind and the notice text.

package hibernateengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/tenderengine"
)

// WakeRun is the wake's view of one stopped or ended entry of the hub's reconcile.
// Ended is false for a stopped entry.
// WayForward is the one line that repairs the stop, or for an ended entry what lets its removal go through.
// Busy marks an entry whose pair a driver or a lock still holds, ParkedPush a busy pair whose records push lock a parked push holds, and Gone a stop whose branch origin no longer has.
// Owner names the other machine that owns the entry's run, and is empty for a run owned here.
type WakeRun struct {
	Slug       string
	Ended      bool
	WayForward string
	Busy       bool
	ParkedPush bool
	Gone       bool
	Owner      string
}

// WakeReconcile is the engine's own view of one hub reconcile at the wake, so the engine imports no fabric package.
// Unreachable means origin could not be reached through the retry bound and nothing moved.
// Stop is the way forward of a stop on a branch every pair shares, empty when none stopped.
// Mutations holds one rendered line per entry of the reconcile's mutation record.
type WakeReconcile struct {
	Unreachable bool
	Stop        string
	Runs        []WakeRun
	Mutations   []string
}

// reconcileAtWake runs the hub reconcile for the wake and reports whether the daemon lifts its own pause entry, with the sentence the wake notice appends.
// A hub entry the request does not own that stands skips the reconcile and the exemption.
// Otherwise the owned entry, when it is live, is exempted from the network gate for the reconcile alone.
// A stop on a shared branch keeps the entry live; every other outcome lifts it, after the pair pause entries of the stopped runs and the busy ended ones are added.
// A TenderStatus error is returned, since nothing was reconciled.
func reconcileAtWake(deps Deps, r Request) (lift bool, notice string, err error) {
	status, err := deps.TenderStatus()
	if err != nil {
		return false, "", fmt.Errorf("read the hub at the wake: %w", err)
	}

	var exempt *tenderengine.EntryRef
	for _, entry := range status.Live {
		if entry.Scope != pauserecord.ScopeHub {
			continue
		}
		if r.Owned == nil || entry.Caller != r.Owned.Caller || !entry.PausedAt.Equal(r.Owned.PausedAt) {
			return true, fmt.Sprintf("the hub pause of %s stands, so the origin reconcile was skipped; lift it with `lyx resume`, and then `lyx orch start` or `lyx cleanup --apply` runs the reconcile.", entry.Caller), nil
		}
		exempt = r.Owned
	}

	wake, reconcileErr := reconcileExempt(deps, exempt)
	for _, line := range wake.Mutations {
		logger.Info("hibernate: the wake reconcile changed", "mutation", line)
	}
	switch {
	case reconcileErr != nil:
		return true, fmt.Sprintf("the origin reconcile failed (%v), so nothing further moved; run `lyx cleanup --apply` to reconcile.", reconcileErr), nil
	case wake.Unreachable:
		return true, "origin was unreachable through the retry bound, so nothing was reconciled; run `lyx cleanup --apply` to reconcile once the network is back.", nil
	case wake.Stop != "":
		return false, fmt.Sprintf("the origin reconcile stopped: %s; settle it, and then `lyx resume` lifts the hub pause.", wake.Stop), nil
	}
	return true, holdRuns(deps, wake.Runs), nil
}

// reconcileExempt runs the reconcile with owned, when not nil, exempted from the network gate until the reconcile returns.
func reconcileExempt(deps Deps, owned *tenderengine.EntryRef) (WakeReconcile, error) {
	if owned != nil {
		defer deps.ExemptNetwork(*owned)()
	}
	return deps.Reconcile()
}

// holdRuns adds a pair pause entry for each stopped run, each run owned by another machine and each busy ended one, not recorded as owned so it outlives the request, and returns the notice sentence naming each run's next step.
// A run owned elsewhere is paused with the owner caller, so a slug entry already standing adds none.
// A pair whose pause fails is left unheld and named with the step that holds it.
func holdRuns(deps Deps, runs []WakeRun) string {
	var steps, unheld, ended []string
	for _, run := range runs {
		if run.Ended && !run.Busy {
			ended = append(ended, run.Slug)
			continue
		}
		request := tenderengine.PauseRequest{Slug: run.Slug, Grace: pauseGrace}
		if run.Owner != "" {
			request.Caller = pauserecord.OwnerCaller
		}
		if _, err := deps.Pause(request); err != nil {
			if run.Owner != "" {
				unheld = append(unheld, fmt.Sprintf("%s could not be paused (%v); the next wake or `lyx orch start` pauses it again", run.Slug, err))
			} else {
				unheld = append(unheld, fmt.Sprintf("%s could not be paused (%v); hold it with `lyx tender pause %s`", run.Slug, err, run.Slug))
			}
		}
		steps = append(steps, runStep(run))
	}
	if len(ended) > 0 {
		steps = append(steps, "ended on origin: "+strings.Join(ended, ", "))
	}
	return strings.Join(append(steps, unheld...), "; ")
}

// runStep names what an operator does for one held run.
// A parked push is cleared by the resume alone; a pair stopped as gone, which no resume can carry on, and an ended one are cleared by the cleanup.
func runStep(run WakeRun) string {
	resume := fmt.Sprintf("`lyx resume %s`", run.Slug)
	switch {
	case run.Owner != "":
		return fmt.Sprintf("%s is owned by machine %s: `lyx adopt %s` from this machine's prime takes it over here", run.Slug, run.Owner, run.Slug)
	case run.Ended:
		return fmt.Sprintf("%s ended on origin by its landing or removal but is still busy: %s; then `lyx cleanup --apply`", run.Slug, run.WayForward)
	case run.ParkedPush:
		return fmt.Sprintf("%s is busy on a parked push: %s", run.Slug, resume)
	case run.Gone:
		return fmt.Sprintf("%s stopped: %s; then `lyx cleanup --apply`", run.Slug, run.WayForward)
	}
	return fmt.Sprintf("%s stopped: %s; then %s", run.Slug, run.WayForward, resume)
}
