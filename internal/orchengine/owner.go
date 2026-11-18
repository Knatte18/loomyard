// owner.go is the orch watcher's owner check.
// When another machine now owns the beam, it ends the beam's processes on this machine and stops the beam's orch,
// so the beam's parts leave the machine together.

package orchengine

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/logger"
)

// OwnerWatch is where the watcher reads the owner of the beam it serves.
// It also carries how the watcher ends the beam's processes on this machine and removes the orch strand.
type OwnerWatch struct {
	// Read returns the owner the beam reads on the local board, and whether that owner is a machine other than this one.
	Read func() (owner string, elsewhere bool, err error)
	// End ends every process this machine runs for the beam but its dobby, the orch's provider process among them.
	// The dobby hands the beam over itself.
	// A nil End ends none.
	End func() error
	// Stop removes the orch strand guid.
	Stop func(guid string) error
	// UnderAdopt reports whether a hold this machine wrote in Bolt covers the beam's subject.
	// The hold stands until the adopt's check is green.
	// It is asked only while the owner reads as another machine, and a nil UnderAdopt covers nothing.
	UnderAdopt func() bool
}

// WatchOwner tells the watcher where the beam's owner is read and how the orch strand is removed;
// a watcher never told one reads no owner.
func (w *Watcher) WatchOwner(o OwnerWatch) {
	w.owner = &o
}

// ownerTick reads the beam's owner for this tick.
// When another machine owns the beam, it ends the beam's processes here, stops the orch strand and records the exit.
// It does none of that when an adopt in flight here covers the beam.
// While such an adopt covers it, the strand stays up, and the hold's start and end each log once.
// done is true once the strand is stopped.
// The strand is stopped even when the end failed, so a failing end never keeps the strand up.
// The exit reason is recorded whenever the strand was stopped, and done is true then too:
// a failed end is logged at Warn and is not the tick's error, since the watcher exits at once and never retries it.
// A failed stop is the tick's error, joined with a failed end, and done stays false.
// A failed stop leaves the strand, so the next tick ends and stops again.
// A failed read reads as not owned elsewhere and logs once per episode.
func (w *Watcher) ownerTick(st State) (done bool, err error) {
	if w.owner == nil {
		return false, nil
	}
	owner, elsewhere, err := w.owner.Read()
	if err != nil {
		w.ownerReadFailures.Fail("orch: the beam's owner is unreadable; the watcher goes on", "error", err)
		return false, nil
	}
	w.ownerReadFailures.Recover("orch: the beam's owner reads again")
	held := elsewhere && w.owner.UnderAdopt != nil && w.owner.UnderAdopt()
	if held != w.ownerAdoptHeld {
		w.ownerAdoptHeld = held
		if held {
			logger.Info("orch: an adopt in flight here covers the beam another machine owns; the orch strand stays up", "owner", owner, "strand", st.Strand)
		} else {
			logger.Info("orch: no adopt in flight covers the beam any more", "strand", st.Strand)
		}
	}
	if !elsewhere || held {
		return false, nil
	}
	var endErr, stopErr error
	if w.owner.End != nil {
		if err := w.owner.End(); err != nil {
			endErr = fmt.Errorf("orch: end the processes of a beam owned by machine %s: %w", owner, err)
		}
	}
	if err := w.owner.Stop(st.Strand); err != nil {
		stopErr = fmt.Errorf("orch: stop the orch strand of a beam owned by machine %s: %w", owner, err)
	}
	if stopErr != nil {
		return false, errors.Join(endErr, stopErr)
	}
	st.WatcherExit = "owned by machine " + owner
	saveErr := w.save(st)
	if endErr != nil {
		logger.Warn("orch: the end of the processes of a beam owned by another machine failed; the orch strand is stopped anyway", "error", endErr)
		return true, saveErr
	}
	logger.Info("orch: the beam is owned by another machine; its processes here are ended and the orch strand is stopped", "owner", owner, "strand", st.Strand)
	return true, saveErr
}
