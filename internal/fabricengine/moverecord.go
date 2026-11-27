// moverecord.go holds the move record a change of a worktree's anchor leaves in its weft's lock directory, and the stop a standing record raises.
// Every weft writer reads it through MoveInProgress before it touches the weft.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/state"
)

// moveLockFile is the name of the lock file a mover holds in a records worktree's lock directory.
const moveLockFile = "anchor-move.lock"

// moveLockPath returns the lock the process carrying out an anchor change of the weft at weftPath holds for the whole change.
func moveLockPath(weftPath string) string {
	return filepath.Join(fabriclayout.RecordsLockDirPath(weftPath), moveLockFile)
}

// moveRecordStateLockPath returns the lock that guards reads and writes of the record file itself, apart from the move lock a mover holds across its change.
func moveRecordStateLockPath(weftPath string) string {
	return fabriclayout.MoveRecordPath(weftPath) + ".lock"
}

// ReadMoveRecord reads the move record of the weft worktree at weftPath.
// An absent record reports found false and creates nothing on disk.
func ReadMoveRecord(weftPath string) (record fabriclayout.MoveRecord, found bool, err error) {
	path := fabriclayout.MoveRecordPath(weftPath)
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		return fabriclayout.MoveRecord{}, false, nil
	}
	record, found, err = state.ReadJSON[fabriclayout.MoveRecord](path, moveRecordStateLockPath(weftPath))
	if err != nil {
		return fabriclayout.MoveRecord{}, false, fmt.Errorf("fabricengine: read the move record in %s: %w", weftPath, err)
	}
	return record, found, nil
}

// RemoveMoveRecord removes the move record of the weft worktree at weftPath, and does nothing when none exists.
func RemoveMoveRecord(weftPath string) error {
	if err := os.Remove(fabriclayout.MoveRecordPath(weftPath)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("fabricengine: remove the move record in %s: %w", weftPath, err)
	}
	return nil
}

// MoveInProgress reports whether the weft worktree at weftPath, the weft of the worktree slug, carries a move record.
// With none it reports not standing.
// With one it returns the way forward:
// a held move lock names the change in progress and the process to wait for before re-running,
// and a free lock names the interrupted change and the `lym fabric anchor` run that recovers it.
// A held lock beside a record whose process is not the live holder is a new run still starting, and is named as that.
func MoveInProgress(slug, weftPath string) (wayForward string, standing bool, err error) {
	record, found, err := ReadMoveRecord(weftPath)
	if err != nil || !found {
		return "", false, err
	}
	moveLock, acquired, err := lock.TryAcquireWriteLock(moveLockPath(weftPath))
	if err != nil {
		return "", false, fmt.Errorf("fabricengine: probe the move lock in %s: %w", weftPath, err)
	}
	rerun := fmt.Sprintf("`lym fabric anchor %s --set %s`", slug, record.B)
	if !acquired {
		if !recordNamesLiveHolder(record) {
			return startingMoveStop(slug) + ", then run the verb again", true, nil
		}
		return fmt.Sprintf("a change of the anchor of %s from %s to %s is in progress in process %d; wait for it to end, then run %s",
			slug, record.A, record.B, record.PID, rerun), true, nil
	}
	if err := moveLock.Release(); err != nil {
		return "", false, fmt.Errorf("fabricengine: release the move lock probe in %s: %w", weftPath, err)
	}
	return fmt.Sprintf("a change of the anchor of %s from %s to %s was interrupted; run %s to finish it",
		slug, record.A, record.B, rerun), true, nil
}

// recordNamesLiveHolder reports whether the process and start time in record are a live process, the one that holds the move lock.
// A record left by a killed run, or not yet rewritten by the run that now holds the lock, names none.
func recordNamesLiveHolder(record fabriclayout.MoveRecord) bool {
	return parentlock.ProcessAlive(record.PID, record.StartTime)
}

// startingMoveStop words the stop for a move lock held by a change that has not yet written its record.
// The holder's target is unknown to a reader of the stale record, so the stop names no re-run.
func startingMoveStop(slug string) string {
	return fmt.Sprintf("a change of the anchor of %s is starting in another process; wait for it to end", slug)
}

// moveRecordStop returns the stop for a weft worktree that carries a move record, and nil when it does not.
// The slug is read from the weft's directory name.
// The way forward comes from probing the move lock beside the record.
func moveRecordStop(weftPath string) (*AnchorStopError, error) {
	slug, ok := fabriclayout.WeftWarpSlug(filepath.Base(weftPath))
	if !ok {
		return nil, fmt.Errorf("fabricengine: %s is no weft sibling worktree", weftPath)
	}
	wayForward, standing, err := MoveInProgress(slug, weftPath)
	if err != nil || !standing {
		return nil, err
	}
	return &AnchorStopError{
		Reason:     "a change of the weft's anchor stands",
		WayForward: wayForward,
	}, nil
}
