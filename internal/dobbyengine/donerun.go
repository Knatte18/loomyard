// donerun.go implements dobby's done-run clear: the removal of a finished run's records and task branch from the prime.
// It frees the run's slug for a later run, such as a second merge-in task filed under the same name.

package dobbyengine

import (
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// clearDoneRuns removes, through Deps.ClearDoneRun, the records and task branch of each listed run.
// It takes only a run whose batten status reads done.
// It runs after the landing count and the landing notices of the tick, and it clears a run only when all of these hold:
// no pass of it is in flight and its driver lock is free;
// its pair is gone, so the leftover teardown, which reads the run's records, takes a pair left standing first;
// the landing-notice memory holds nothing of it, so its notice is decided and posted;
// and a landing it holds is counted.
// A cleared run logs `run <slug>: done run cleared`, and its slug leaves the counted landings,
// so a later run under the slug counts its own landing.
// Every tick also drops the counted landings of a slug that is neither landed nor listed,
// so a drop the state update missed is repaired at the next tick.
// A run the clear leaves alone is tried again at the next tick.
// A failed clear is logged once per episode and never fails the tick, since the run is finished either way.
// A failed branch removal leaves the records, so it is such a failure.
// A nil Deps.ClearDoneRun clears nothing.
func (d *daemon) clearDoneRuns(now time.Time) {
	if d.deps.ClearDoneRun == nil {
		return
	}
	current, _, err := ReadState(d.geom.StatePath)
	if err != nil {
		logger.Warn("dobbyengine: the state record is unreadable, so no done run is cleared this tick", "worktree", d.geom.WorktreeName, "error", err)
		return
	}
	var cleared []string
	for _, run := range d.listedRuns {
		if run.State != runStateDone || run.DriverLockHeld || run.PairPresent {
			continue
		}
		if _, inFlight := d.flights[run.Slug]; inFlight {
			continue
		}
		if _, remembered := current.LandNotices[run.Slug]; remembered {
			continue
		}
		if slices.Contains(d.landed, run.Slug) && !current.CountedLandings[run.Slug] {
			continue
		}
		done, err := d.deps.ClearDoneRun(run.Slug)
		if err != nil {
			d.doneRunClears.Fail(run.Slug, "dobbyengine: a done run's records could not be cleared; the next tick tries again", "slug", run.Slug, "error", err)
			continue
		}
		d.doneRunClears.Recover(run.Slug, "dobbyengine: a done run's records clear again", "slug", run.Slug)
		if done {
			cleared = append(cleared, run.Slug)
			d.logLine(now, "run "+run.Slug+": done run cleared")
		}
	}
	dropped := cleared
	for slug := range current.CountedLandings {
		listed := slices.ContainsFunc(d.listedRuns, func(run RunView) bool { return run.Slug == slug })
		if !listed && !slices.Contains(d.landed, slug) {
			dropped = append(dropped, slug)
		}
	}
	if len(dropped) == 0 {
		return
	}
	err = UpdateState(d.geom.StatePath, func(s *State) error {
		for _, slug := range dropped {
			delete(s.CountedLandings, slug)
		}
		return nil
	})
	if err != nil {
		logger.Warn("dobbyengine: the cleared runs could not leave the counted landings", "worktree", d.geom.WorktreeName, "slugs", dropped, "error", err)
	}
}
