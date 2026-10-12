// pause.go is the orch watcher's reading of the hub pause record: it holds the cycle machine while a hub entry is live, interrupts a busy orch at the entry's grace end, and queues the orch's resume line after the entry is resumed.
// The watcher stays the only writer into the orch pane; a slug entry never covers the orch.

package orchengine

import (
	"errors"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

var (
	// errPauseEntryGone means the entry was resumed before the watcher could record the orch in it.
	errPauseEntryGone = errors.New("orch: the hub pause entry is no longer live")
	// errPauseAlreadyOwned means a live entry already owns the orch's session.
	errPauseAlreadyOwned = errors.New("orch: a live pause entry already owns the orch session")
)

// PauseWatch is where the watcher reads the pause record.
type PauseWatch struct {
	// RecordPath is the pause record file.
	RecordPath string
	// Alive reports whether the process with the given pid and start time is running.
	Alive func(pid int, startTime string) bool
}

// WatchPause tells the watcher where the pause record is; a watcher never told one reads no record.
func (w *Watcher) WatchPause(p PauseWatch) {
	w.pause = &p
}

// pauseTick reads the pause record and acts on it for this tick.
// held is true while a live hub entry stands, and the caller then runs nothing else this tick.
// Under a live hub entry a busy orch gets the first guarded key at the entry's grace end, and a second only on a tick at least the provider's settle time later that still reads it busy;
// with no live entry, the resume line of an orch the last entry interrupted is queued once.
// An unreadable record holds nothing and logs one Warn per change of error.
func (w *Watcher) pauseTick(st State, now time.Time) (held bool, err error) {
	if w.firstTick.IsZero() {
		w.firstTick = now
	}
	if w.pause == nil {
		return false, nil
	}
	record, err := pauserecord.Read(w.pause.RecordPath)
	if err != nil {
		if err.Error() != w.pauseReadError {
			logger.Warn("orch: the pause record cannot be read, so the watcher reads the orch uncovered", "error", err)
		}
		w.pauseReadError = err.Error()
		return false, nil
	}
	w.pauseReadError = ""

	entry, live := record.Covering("")
	if !live {
		return false, w.queueResumeLine(record, st, now)
	}
	return true, w.handleHubEntry(entry, record, st, now)
}

// handleHubEntry acts on each live hub entry until it is handled, remembered in memory by its PausedAt.
// A tick after the first key waits out the provider's settle, then re-probes the orch and sends the second and last key only to a busy one.
// That step runs before settledWithoutInterrupt, because the first key's record already makes the entry own the orch.
func (w *Watcher) handleHubEntry(entry pauserecord.Entry, record pauserecord.Record, st State, now time.Time) error {
	key := entry.PausedAt.UnixNano()
	if w.pauseHandled[key] {
		return nil
	}
	if firstKey, waiting := w.pauseFirstEsc[key]; waiting {
		if now.Sub(firstKey) < w.session.InterruptSettle() {
			return nil
		}
		delete(w.pauseFirstEsc, key)
		idle, err := w.pauseProbeIdle(st)
		if err != nil {
			return err
		}
		w.markPauseHandled(key)
		if !idle {
			w.pauseInterrupt(st)
		}
		return nil
	}
	if w.settledWithoutInterrupt(entry, st) {
		w.markPauseHandled(key)
		return nil
	}

	idle, err := w.pauseProbeIdle(st)
	if err != nil {
		return err
	}
	if now.Before(entry.GraceEnd) {
		if idle && !w.orchNetworkOperationInFlight() {
			w.markPauseHandled(key)
		}
		return nil
	}
	if idle {
		w.markPauseHandled(key)
		return nil
	}
	if err := w.recordOrchInEntry(entry, st, now); err != nil {
		w.markPauseHandled(key)
		if errors.Is(err, errPauseEntryGone) || errors.Is(err, errPauseAlreadyOwned) {
			return nil
		}
		return err
	}
	if !w.pauseInterrupt(st) {
		w.markPauseHandled(key)
		return nil
	}
	if w.pauseFirstEsc == nil {
		w.pauseFirstEsc = map[int64]time.Time{}
	}
	w.pauseFirstEsc[key] = now
	return nil
}

// pauseInterrupt sends the orch one guarded interrupt and reports whether the key went.
// A pane the guard finds with no running turn reads as an idle orch and is logged at Info; an open rewind picker is closed by the guard and logged at Warn.
func (w *Watcher) pauseInterrupt(st State) (sent bool) {
	err := w.session.Interrupt(st.Strand)
	switch {
	case err == nil:
		return true
	case errors.Is(err, shuttleengine.ErrNoRunningTurn):
		logger.Info("orch: the pause found no running turn on the orch pane, so the orch reads idle", "strandGUID", st.Strand)
	case errors.Is(err, shuttleengine.ErrRewindPickerOpen):
		logger.Warn("orch: the pause found the rewind picker open on the orch pane and closed it", "strandGUID", st.Strand, "error", err)
	default:
		logger.Warn("orch: the pause could not interrupt the orch", "strandGUID", st.Strand, "error", err)
	}
	return false
}

// settledWithoutInterrupt reports whether entry needs no interrupt and no record of the orch: the orch ran the pause, the entry already owns the orch, the orch started under it, or the watcher was not running at its grace end.
func (w *Watcher) settledWithoutInterrupt(entry pauserecord.Entry, st State) bool {
	if name, err := agentname.Parse(entry.Caller); err == nil && name.Role == agentname.RoleOrch && name.Slug == "" {
		return true
	}
	for _, session := range entry.Sessions {
		if session.GUID == st.Strand {
			return true
		}
	}
	if !entry.PausedAt.After(st.LaunchedAt) {
		return true
	}
	return entry.GraceEnd.Before(w.firstTick)
}

// markPauseHandled remembers that the hub entry with the given PausedAt key needs nothing more from this watcher.
func (w *Watcher) markPauseHandled(key int64) {
	if w.pauseHandled == nil {
		w.pauseHandled = map[int64]bool{}
	}
	w.pauseHandled[key] = true
}

// pauseProbeIdle probes whether the orch session is idle, without recording a hold in the state.
func (w *Watcher) pauseProbeIdle(st State) (bool, error) {
	probe, err := w.session.SessionIdle(st.Strand)
	if err != nil {
		return false, err
	}
	return probe.Idle, nil
}

// orchNetworkOperationInFlight reports whether a network operation marker names the orch's strand.
// A marker list that cannot be read counts as none, with one Warn.
func (w *Watcher) orchNetworkOperationInFlight() bool {
	ops, err := pauserecord.ListNetOps(w.pause.RecordPath, w.pause.Alive)
	if err != nil {
		logger.Warn("orch: the pause's network operation markers cannot be read", "error", err)
		return false
	}
	for _, op := range ops {
		if name, err := agentname.Parse(op.Strand); err == nil && name.Role == agentname.RoleOrch && name.Slug == "" {
			return true
		}
	}
	return false
}

// recordOrchInEntry adds the orch's session to the live hub entry with entry's PausedAt, under the record lock.
// It fails with errPauseEntryGone when no such entry is live and errPauseAlreadyOwned when a live entry already owns the session.
func (w *Watcher) recordOrchInEntry(entry pauserecord.Entry, st State, now time.Time) error {
	_, err := pauserecord.Update(w.pause.RecordPath, now, func(record *pauserecord.Record) error {
		if _, owned := record.Owner(st.Strand); owned {
			return errPauseAlreadyOwned
		}
		for i := range record.Live {
			live := &record.Live[i]
			if live.Scope == pauserecord.ScopeHub && live.PausedAt.Equal(entry.PausedAt) {
				live.Sessions = append(live.Sessions, pauserecord.Session{GUID: st.Strand, Orch: true})
				return nil
			}
		}
		return errPauseEntryGone
	})
	return err
}

// queueResumeLine queues the resume line of the newest resumed entry that owns the orch and has not yet noticed it, then marks it noticed under the record lock.
// The line is queued before the mark, so a crash between the two delivers it twice rather than never.
func (w *Watcher) queueResumeLine(record pauserecord.Record, st State, now time.Time) error {
	var resumed pauserecord.Entry
	found := false
	for _, entry := range record.Resumed {
		if !ownsUnnoticedOrch(entry, st.Strand) {
			continue
		}
		if !found || entry.ResumedAt.After(resumed.ResumedAt) {
			resumed, found = entry, true
		}
	}
	if !found {
		return nil
	}
	if _, err := QueueNotice(w.paths, pauserecord.ResumeLine, now); err != nil {
		return err
	}
	_, err := pauserecord.Update(w.pause.RecordPath, now, func(record *pauserecord.Record) error {
		for i := range record.Resumed {
			entry := &record.Resumed[i]
			if !entry.PausedAt.Equal(resumed.PausedAt) || entry.Scope != resumed.Scope {
				continue
			}
			for j := range entry.Sessions {
				if entry.Sessions[j].Orch && entry.Sessions[j].GUID == st.Strand {
					entry.Sessions[j].ResumeNoticed = true
				}
			}
		}
		return nil
	})
	return err
}

// ownsUnnoticedOrch reports whether entry owns the orch session guid with its resume line not yet queued.
func ownsUnnoticedOrch(entry pauserecord.Entry, guid string) bool {
	for _, session := range entry.Sessions {
		if session.Orch && session.GUID == guid && !session.ResumeNoticed {
			return true
		}
	}
	return false
}
