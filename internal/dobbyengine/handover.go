// handover.go is what a dobby does on a probe tick that reads its beam owned by another machine.
// It stays up and idle while an adopt in flight here covers the beam.
// Otherwise it pauses its runs, archives the local tips origin lacks and exits.
// It also holds the probe's periodic Bolt pull, through which an idle machine learns a new owner.

package dobbyengine

import (
	"context"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// boltPullInterval is the shortest gap between two Bolt pulls of the probe.
const boltPullInterval = 5 * time.Minute

// pullBolt pulls Bolt on a probe tick once boltPullInterval has passed since the last pull.
// The first probe tick pulls.
// A failed pull is logged once per episode, and the owner read goes on over the local Bolt.
func (d *daemon) pullBolt(now time.Time) {
	if d.deps.PullBolt == nil || (!d.lastBoltPull.IsZero() && now.Sub(d.lastBoltPull) < boltPullInterval) {
		return
	}
	d.lastBoltPull = now
	if err := d.deps.PullBolt(); err != nil {
		d.boltPull.Fail("dobbyengine: the Bolt pull failed; the owner is read from the local Bolt", "worktree", d.geom.WorktreeName, "error", err)
		return
	}
	d.boltPull.Recover("dobbyengine: the Bolt pulls again", "worktree", d.geom.WorktreeName)
}

// probeOwner reads the beam's owner on a probe tick and reports whether the dobby exits.
// While an adopt in flight on this machine covers a beam another machine owns, it sets the adopt hold.
// Under the hold the dobby stays up and schedules nothing, and the hold's start and end each log one line.
// Any other foreign owner gets the hand-over.
func (d *daemon) probeOwner(ctx context.Context, now time.Time) bool {
	elsewhere, owner := d.beamOwnedElsewhere()
	holds := elsewhere && d.deps.UnderAdopt != nil && d.deps.UnderAdopt()
	if holds != d.adoptHolds {
		d.adoptHolds = holds
		if holds {
			d.logLine(now, fmt.Sprintf("adopt hold: an adopt in flight here covers this beam, which machine %s owns; the dobby schedules nothing", owner))
		} else {
			d.logLine(now, "adopt hold ended")
		}
	}
	if !elsewhere || holds {
		// An unreadable owner says nothing of whether a hand-over is still due, so it leaves the episode standing.
		if !d.ownerUnreadable {
			d.endArchiveEpisode("dobbyengine: the beam needs no hand-over any more; the archive failure ends")
		}
		return false
	}
	return d.handOver(ctx, now, owner)
}

// endArchiveEpisode closes the hand-over's archive failure episode, logging msg, so a later failure logs its line again.
func (d *daemon) endArchiveEpisode(msg string) {
	d.archiveFailures.Recover(msg, "worktree", d.geom.WorktreeName)
	d.archiveFailing = false
}

// handOver hands the beam to owner, the machine that owns it now, and reports whether the dobby exits.
// It first owner-pauses the runs it drives, so no pass of a run whose pause stands starts while it archives.
// It then archives the local tips under the beam that origin lacks, and exits once the archive succeeds.
// A failed archive is logged once per episode and keeps the dobby up; the next probe tick retries it.
func (d *daemon) handOver(ctx context.Context, now time.Time, owner string) bool {
	d.pauseRunsForOwner(ctx, now)
	if d.deps.ArchiveOwner != nil {
		if err := d.deps.ArchiveOwner(); err != nil {
			d.archiveFailures.Fail("dobbyengine: archiving the local tips for the beam's new owner failed; the dobby stays up and retries at the next probe tick",
				"worktree", d.geom.WorktreeName, "owner", owner, "error", err)
			if !d.archiveFailing {
				d.archiveFailing = true
				d.logLine(now, "archive failing: "+oneLine(err.Error()))
			}
			return false
		}
		d.endArchiveEpisode("dobbyengine: the archive succeeds again")
	}
	logger.Info("dobbyengine: another machine owns this beam; the dobby exits", "worktree", d.geom.WorktreeName, "owner", owner)
	subject := d.geom.Slug
	if subject == "" {
		subject = "root"
	}
	d.logOwn(now, fmt.Sprintf("stop: another machine owns this beam (%s); lym adopt %s takes it over here", owner, subject))
	return true
}

// pauseRunsForOwner owner-pauses each run this dobby drives that is neither done nor recorded owned elsewhere.
// It pauses as a pass exiting owned elsewhere does.
// A run listing or state read that fails is logged at Warn and pauses nothing; the next probe tick tries again.
func (d *daemon) pauseRunsForOwner(ctx context.Context, now time.Time) {
	runs, err := d.deps.Runs()
	if err != nil {
		logger.Warn("dobbyengine: the runs could not be listed for the hand-over; none is paused", "worktree", d.geom.WorktreeName, "error", err)
		return
	}
	current, _, err := ReadState(d.geom.StatePath)
	if err != nil {
		logger.Warn("dobbyengine: the state record could not be read for the hand-over; no run is paused", "worktree", d.geom.WorktreeName, "error", err)
		return
	}
	for _, run := range runs {
		if run.State == runStateDone || current.Runs[run.Slug].OwnedElsewhere {
			continue
		}
		d.ownedElsewhere(ctx, run.Slug, now)
	}
}
