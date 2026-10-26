// owner.go is the orch watcher's owner check: a beam's orch whose beam another machine now owns is stopped, so the beam's two parts leave the machine together.

package orchengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/logger"
)

// OwnerWatch is where the watcher reads the owner of the beam it serves and how it removes the orch strand.
type OwnerWatch struct {
	// Read returns the machine the beam reads as owned by on the local board, and whether that is a machine other than this one.
	Read func() (owner string, elsewhere bool, err error)
	// Stop removes the orch strand guid.
	Stop func(guid string) error
}

// WatchOwner tells the watcher where the beam's owner is read and how the orch strand is removed; a watcher never told one reads no owner.
func (w *Watcher) WatchOwner(o OwnerWatch) {
	w.owner = &o
}

// ownerTick reads the beam's owner for this tick and, when another machine owns it, stops the orch strand and records the exit.
// done is true once the strand is stopped; a failed stop is the tick's error and the next tick retries it.
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
	if !elsewhere {
		return false, nil
	}
	if err := w.owner.Stop(st.Strand); err != nil {
		return false, fmt.Errorf("orch: stop the orch strand of a beam owned by machine %s: %w", owner, err)
	}
	logger.Info("orch: the beam is owned by another machine; the orch strand is stopped", "owner", owner, "strand", st.Strand)
	st.WatcherExit = "owned by machine " + owner
	return true, w.save(st)
}
