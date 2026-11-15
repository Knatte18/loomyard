// intents.go writes the cancel and pause intents of a run's batten record, the tracked status file under StatusFile.

package shedrun

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// errIntentRecordAbsent stops an intent write over a run with no status file, which has no intent to hold.
var errIntentRecordAbsent = errors.New("run has no status file")

// updateStatusIfPresent rewrites the run's status file through mutate under the status lock.
// It writes nothing and reports found false when the file is absent.
// Keys the binary does not know survive the rewrite.
// It commits and pushes nothing.
func updateStatusIfPresent(l *lymcwd.Location, runID string, mutate func(st *shedengine.Status) error) (found bool, err error) {
	err = state.UpdateJSON(StatusFile(l, runID), StatusLock(l, runID), func(cur shedengine.Status, present bool) (shedengine.Status, error) {
		if !present {
			return cur, errIntentRecordAbsent
		}
		if err := mutate(&cur); err != nil {
			return cur, err
		}
		return cur, nil
	})
	if errors.Is(err, errIntentRecordAbsent) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// WriteCancelIntent records intent as the run's cancel intent, replacing any recorded before it, its finish included.
// It reports found false and writes nothing when the run has no status file.
// The caller commits the record.
func WriteCancelIntent(l *lymcwd.Location, runID string, intent shedengine.CancelIntent) (found bool, err error) {
	return updateStatusIfPresent(l, runID, func(st *shedengine.Status) error {
		st.Cancel = &intent
		return nil
	})
}

// FinishCancel records finish on the run's cancel intent and sets the run's state to cancelled.
// It reports found false and writes nothing when the run has no status file.
// It returns an error when the run records no cancel intent, since a cancel writes its intent before it ends anything.
// The caller commits the record.
func FinishCancel(l *lymcwd.Location, runID string, finish shedengine.CancelFinish) (found bool, err error) {
	return updateStatusIfPresent(l, runID, func(st *shedengine.Status) error {
		if st.Cancel == nil {
			return fmt.Errorf("finish cancel of run %q: no cancel intent is recorded", runID)
		}
		st.Cancel.Finish = &finish
		st.State = shedengine.StateCancelled
		return nil
	})
}

// WritePauseIntent records intent as the run's pause intent.
// It reports found false and writes nothing when the run has no status file.
// The caller commits the record.
func WritePauseIntent(l *lymcwd.Location, runID string, intent shedengine.PauseIntent) (found bool, err error) {
	return updateStatusIfPresent(l, runID, func(st *shedengine.Status) error {
		st.PauseIntent = &intent
		return nil
	})
}

// ClearPauseIntent removes the run's pause intent.
// It reports found true when the status file exists, even if no intent stood.
// It writes nothing and reports found false when the run has no status file.
// The caller commits the record.
func ClearPauseIntent(l *lymcwd.Location, runID string) (found bool, err error) {
	return updateStatusIfPresent(l, runID, func(st *shedengine.Status) error {
		st.PauseIntent = nil
		return nil
	})
}
