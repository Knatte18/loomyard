// moverecord.go holds the move record a change of a worktree's anchor leaves in its weft's lock directory, and the stop a standing record raises.
// Every weft writer reads it through MoveInProgress before it touches the weft.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/state"
)

const (
	moveRecordFile = "anchor-move.json"
	moveLockFile   = "anchor-move.lock"
)

// The kinds of anchor change a MoveRecord stands for.
const (
	// MoveKindMove re-roots the weft's content at the new anchor.
	MoveKindMove = "move"
	// MoveKindIntegrate also carries the weft's local commits across an anchor change made on its upstream.
	MoveKindIntegrate = "integrate"
)

// MoveRecord is the record of an anchor change under way, written before the weft is touched and removed once the change is finished or undone.
// A is the anchor the weft is moving from and B the one it is moving to.
// Kind is MoveKindMove or MoveKindIntegrate, and U is the upstream tip an integration rebuilds on, empty until that step is reached.
// PID and StartTime identify the process that holds the move lock, so a reader can tell a live change from an interrupted one.
type MoveRecord struct {
	A         string `json:"a"`
	B         string `json:"b"`
	Kind      string `json:"kind"`
	U         string `json:"u,omitempty"`
	PID       int    `json:"pid"`
	StartTime string `json:"start_time,omitempty"`
}

// MoveRecordPath returns where the move record of the weft worktree at weftPath lives.
// It sits in the weft's lock directory, which git excludes and no move touches.
func MoveRecordPath(weftPath string) string {
	return filepath.Join(RecordsLockDirPath(weftPath), moveRecordFile)
}

// moveLockPath returns the lock the process carrying out an anchor change of the weft at weftPath holds for the whole change.
func moveLockPath(weftPath string) string {
	return filepath.Join(RecordsLockDirPath(weftPath), moveLockFile)
}

// moveRecordStateLockPath returns the lock that guards reads and writes of the record file itself, apart from the move lock a mover holds across its change.
func moveRecordStateLockPath(weftPath string) string {
	return MoveRecordPath(weftPath) + ".lock"
}

// ReadMoveRecord reads the move record of the weft worktree at weftPath.
// An absent record reports found false and creates nothing on disk.
func ReadMoveRecord(weftPath string) (record MoveRecord, found bool, err error) {
	path := MoveRecordPath(weftPath)
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		return MoveRecord{}, false, nil
	}
	record, found, err = state.ReadJSONNoMkdir[MoveRecord](path, moveRecordStateLockPath(weftPath))
	if err != nil {
		return MoveRecord{}, false, fmt.Errorf("fabricengine: read the move record in %s: %w", weftPath, err)
	}
	return record, found, nil
}

// RemoveMoveRecord removes the move record of the weft worktree at weftPath, and does nothing when none exists.
func RemoveMoveRecord(weftPath string) error {
	if err := os.Remove(MoveRecordPath(weftPath)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("fabricengine: remove the move record in %s: %w", weftPath, err)
	}
	return nil
}

// MoveInProgress reports whether the weft worktree at weftPath, the weft of the worktree slug, carries a move record.
// With none it reports not standing.
// With one it returns the way forward: a held move lock names the change in progress and its process, to wait for before re-running,
// and a free lock names the interrupted change and the `lyx fabric anchor` run that recovers it.
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
	rerun := fmt.Sprintf("`lyx fabric anchor %s --set %s`", slug, record.B)
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
func recordNamesLiveHolder(record MoveRecord) bool {
	return parentlock.ProcessAlive(record.PID, record.StartTime)
}

// startingMoveStop words the stop for a move lock held by a change that has not yet written its record.
// The holder's target is unknown to a reader of the stale record, so the stop names no re-run.
func startingMoveStop(slug string) string {
	return fmt.Sprintf("a change of the anchor of %s is starting in another process; wait for it to end", slug)
}

// moveRecordStop returns the stop for a weft worktree that carries a move record, and nil when it does not.
// The slug is read from the weft's directory name; its way forward is MoveInProgress's.
func moveRecordStop(weftPath string) (*AnchorStopError, error) {
	slug, ok := WeftWarpSlug(filepath.Base(weftPath))
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
