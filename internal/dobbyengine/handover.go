// handover.go is what a dobby does on a probe tick that reads its beam owned by another machine.
// It stays up and idle while a hold this machine wrote in Bolt covers the beam's subject.
// That hold stands until the adopt's check is green.
// Otherwise it runs the catch-up that hands its runs over and ends the subject's processes.
// It then archives the local tips origin lacks and exits.
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
// While a hold this machine wrote in Bolt covers the subject of a beam another machine owns, it sets the adopt hold.
// Under the hold the dobby stays up and schedules nothing, and the hold's start and end each log one line.
// Any other foreign owner gets the hand-over.
func (d *daemon) probeOwner(ctx context.Context, now time.Time) bool {
	elsewhere, owner := d.beamOwnedElsewhere()
	holds := elsewhere && d.deps.UnderAdopt != nil && d.deps.UnderAdopt()
	if holds != d.adoptHolds {
		d.adoptHolds = holds
		if holds {
			d.logLine(now, fmt.Sprintf("adopt hold: this machine's Bolt hold covers this beam's subject, and machine %s owns the beam; the dobby schedules nothing", owner))
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
// It first runs the catch-up from origin, which hands over each run another machine owns:
// its processes end, its local tips go under owner archive tags and its local pair is removed.
// Once the catch-up succeeds it ends the subject's processes through Deps.EndSubject.
// It then archives the local tips under the beam that origin lacks, and exits once the archive succeeds.
// A failed catch-up, subject end or archive is logged once per episode and keeps the dobby up.
// The next probe tick retries it.
func (d *daemon) handOver(ctx context.Context, now time.Time, owner string) bool {
	if d.deps.CatchUp.PullBolt != nil {
		if _, err := CatchUp(d.deps.CatchUp); err != nil {
			d.handOverFailed(now, owner, "catch-up failing: ", "the catch-up handing the beam's runs over failed; the dobby stays up and retries at the next probe tick", err)
			return false
		}
	}
	if d.deps.EndSubject != nil {
		if err := d.deps.EndSubject(); err != nil {
			d.handOverFailed(now, owner, "subject end failing: ", "ending the processes of the beam's subject failed; the dobby stays up and retries at the next probe tick", err)
			return false
		}
	}
	if d.deps.ArchiveOwner != nil {
		if err := d.deps.ArchiveOwner(); err != nil {
			d.handOverFailed(now, owner, "archive failing: ", "archiving the local tips for the beam's new owner failed; the dobby stays up and retries at the next probe tick", err)
			return false
		}
	}
	d.endArchiveEpisode("dobbyengine: the hand-over succeeds again")
	logger.Info("dobbyengine: another machine owns this beam; the dobby exits", "worktree", d.geom.WorktreeName, "owner", owner)
	subject := d.geom.Slug
	if subject == "" {
		subject = "root"
	}
	d.logOwn(now, fmt.Sprintf("stop: another machine owns this beam (%s); lym adopt %s takes it over here", owner, subject))
	return true
}

// handOverFailed logs a failed step of the hand-over once per episode and keeps the dobby up.
// The dobby's own log line starts with linePrefix.
func (d *daemon) handOverFailed(now time.Time, owner, linePrefix, message string, err error) {
	d.archiveFailures.Fail("dobbyengine: "+message, "worktree", d.geom.WorktreeName, "owner", owner, "error", err)
	if !d.archiveFailing {
		d.archiveFailing = true
		d.logLine(now, linePrefix+oneLine(err.Error()))
	}
}
