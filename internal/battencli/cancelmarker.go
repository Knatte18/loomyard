// cancelmarker.go implements the cancel marker of "lym batten cancel": a record that a cancel of a slug is under way, whose writer's liveness is a lock it holds, never its pid.
//
// A driving verb and a second cancel read the marker to refuse while a cancel runs, and to point at the cancel that finishes an interrupted one.
// The lock is held exclusively from the marker's write to its release, so a marker whose lock is free belongs to a writer that died.

package battencli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
)

// cancelMarker is the JSON record at shedrun.CancelMarker.
type cancelMarker struct {
	// PID is the writer's process id.
	PID int `json:"pid"`
	// StartTime is the writer's process start time, empty where the platform cannot read it.
	StartTime string `json:"start_time"`
	// StartedAt is when the marker was written, in RFC 3339.
	StartedAt string `json:"started_at"`
}

// cancelMarkerReading is what a read of the cancel marker found.
type cancelMarkerReading struct {
	// Present is true when a cancel of the slug is under way or was interrupted.
	Present bool
	// WriterAlive is true when the marker's writer still holds the marker's lock; it is meaningful only with Present.
	WriterAlive bool
	// Marker is the decoded record, empty when the marker does not decode or is absent.
	Marker cancelMarker
}

// cancelMarkerStateLock returns the lock that guards the marker file's own reads and writes, distinct from the writer's liveness lock.
func cancelMarkerStateLock(markerPath string) string {
	return markerPath + ".lock"
}

// judgeCancelMarker decides what the cancel marker's three readings make of it.
// presentBefore is a stat of the marker, lockFree is whether the writer's lock was free to a shared try, and presentUnderLock is a second stat taken while holding that shared lock.
// An absent marker is not present; a present marker whose lock is held elsewhere has a live writer;
// a present marker whose lock was free has a dead writer, unless the writer removed it between the first stat and the lock, which reads not present.
func judgeCancelMarker(presentBefore, lockFree, presentUnderLock bool) (present, writerAlive bool) {
	if !presentBefore {
		return false, false
	}
	if !lockFree {
		return true, true
	}
	return presentUnderLock, false
}

// readCancelMarker reads the cancel marker of slug and the liveness of its writer.
// A marker that does not decode reads present with its fields empty, so a torn write never hides a cancel.
func readCancelMarker(location *lymcwd.Location, slug string) (cancelMarkerReading, error) {
	markerPath := shedrun.CancelMarker(location, slug)
	presentBefore, err := markerExists(markerPath)
	if err != nil || !presentBefore {
		return cancelMarkerReading{}, err
	}

	shared, lockFree, err := lock.TryAcquireReadLock(shedrun.CancelMarkerLock(location, slug))
	if err != nil {
		return cancelMarkerReading{}, err
	}
	var presentUnderLock bool
	if lockFree {
		presentUnderLock, err = markerExists(markerPath)
		if releaseErr := shared.Release(); err == nil {
			err = releaseErr
		}
		if err != nil {
			return cancelMarkerReading{}, err
		}
	}

	present, writerAlive := judgeCancelMarker(presentBefore, lockFree, presentUnderLock)
	reading := cancelMarkerReading{Present: present, WriterAlive: writerAlive}
	if !present {
		return reading, nil
	}
	marker, _, err := state.ReadJSONNoMkdir[cancelMarker](markerPath, cancelMarkerStateLock(markerPath))
	if err != nil && !errors.Is(err, state.ErrDecode) {
		return cancelMarkerReading{}, err
	}
	reading.Marker = marker
	return reading, nil
}

// markerExists reports whether the file at path exists.
func markerExists(path string) (bool, error) {
	_, err := os.Stat(path)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	}
	return false, err
}

// heldCancelMarker is a cancel marker its writer holds the lock of.
type heldCancelMarker struct {
	markerPath string
	held       *lock.FileLock
	// fresh is false when the marker was already there, left by a writer that died.
	fresh bool
}

// takeCancelMarker takes the marker's lock exclusively, waiting at most wait, and writes the marker with this process's pid, start time and the time now.
// A shared probe of a reader holds the lock briefly, which the wait outlasts.
// A lock not taken within the wait refuses as a live cancel.
func takeCancelMarker(location *lymcwd.Location, slug string, wait time.Duration) (heldCancelMarker, error) {
	markerPath := shedrun.CancelMarker(location, slug)
	lockPath := shedrun.CancelMarkerLock(location, slug)
	if err := os.MkdirAll(shedrun.ScratchRoot(location), 0o755); err != nil {
		return heldCancelMarker{}, fmt.Errorf("battencli: create the scratch root for the cancel marker: %w", err)
	}

	held, acquired, err := lock.AcquireWriteLockWithin(lockPath, wait)
	if err != nil {
		return heldCancelMarker{}, err
	}
	if !acquired {
		// The reading is best effort: a marker that cannot be read still refuses, with no pid to name.
		reading, _ := readCancelMarker(location, slug)
		reading.Present, reading.WriterAlive = true, true
		return heldCancelMarker{}, cancelMarkerRefusal(slug, reading)
	}

	existed, err := markerExists(markerPath)
	if err != nil {
		return heldCancelMarker{}, errors.Join(err, held.Release())
	}
	marker := cancelMarker{PID: os.Getpid(), StartedAt: time.Now().UTC().Format(time.RFC3339)}
	if start, ok := proc.StartTime(marker.PID); ok {
		marker.StartTime = start
	}
	if err := state.WriteJSON(markerPath, cancelMarkerStateLock(markerPath), marker); err != nil {
		return heldCancelMarker{}, errors.Join(fmt.Errorf("battencli: write the cancel marker: %w", err), held.Release())
	}
	return heldCancelMarker{markerPath: markerPath, held: held, fresh: !existed}, nil
}

// release removes the marker when remove is true, then releases the lock, so the marker never outlives its lock while being removed.
func (h heldCancelMarker) release(remove bool) error {
	var removeErr error
	if remove {
		if err := os.Remove(h.markerPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			removeErr = fmt.Errorf("battencli: remove the cancel marker: %w", err)
		}
	}
	return errors.Join(removeErr, h.held.Release())
}

// cancelMarkerRefusal words the refusal a driving verb or a second cancel gets for the marker reading.
// A live writer is waited for; a dead one is finished by running the cancel again.
func cancelMarkerRefusal(slug string, reading cancelMarkerReading) error {
	if reading.WriterAlive {
		pid := reading.Marker.PID
		return fmt.Errorf("a cancel of %s is under way (pid %d, since %s); way forward: wait for the cancel (pid %d) to end, seeing it with \"ps -p %d\" or in Task Manager on Windows, then run the verb again",
			slug, pid, reading.Marker.StartedAt, pid, pid)
	}
	return fmt.Errorf("a cancel of %s was interrupted; way forward: run \"lym batten cancel %s\" from the prime to finish it", slug, slug)
}
