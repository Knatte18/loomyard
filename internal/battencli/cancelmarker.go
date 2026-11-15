// cancelmarker.go reads and takes the cancel of "lym batten cancel" that is under way.
// The cancel is its intent in batten's record, which every machine reads, and a lock only this machine's cancel holds.
//
// A driving verb and a second cancel read both to refuse while a cancel runs.
// The same read also points at the cancel that finishes an interrupted one.
// The lock is held exclusively from before the intent's write to the cancel's end,
// so an unfinished intent whose lock is free belongs to a writer that died, or to another machine.

package battencli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
)

// cancelMarkerReading is what a read of a slug's cancel found.
type cancelMarkerReading struct {
	// Present is true when a cancel of the slug is under way or was interrupted.
	Present bool
	// WriterAlive is true when a cancel holds the cancel's lock; it is meaningful only with Present.
	WriterAlive bool
	// Intent is the unfinished cancel intent of the record, empty when the record holds none or does not decode.
	Intent shedengine.CancelIntent
}

// judgeCancelMarker decides what the cancel's three readings make of it.
// intentBefore is whether the record held an unfinished cancel intent.
// lockFree is whether the cancel's lock was free to a shared try.
// intentUnderLock is a second read of the intent taken while holding that shared lock.
// A lock held elsewhere is a cancel under way, whether or not its intent is written yet.
// With the lock free, a cancel is present only while the intent stood both before and under the lock,
// and its writer is then gone.
// An intent finished between the two reads reads not present.
func judgeCancelMarker(intentBefore, lockFree, intentUnderLock bool) (present, writerAlive bool) {
	if !lockFree {
		return true, true
	}
	return intentBefore && intentUnderLock, false
}

// readUnfinishedCancelIntent reads batten's record of slug under its status lock and creates nothing.
// unfinished is true for a cancel intent with no finish.
// A record that does not decode reads unfinished with an empty intent, so a torn write never hides a cancel.
func readUnfinishedCancelIntent(location *lymcwd.Location, slug string) (unfinished bool, intent shedengine.CancelIntent, err error) {
	record, found, err := readBattenStatus(location, slug)
	switch {
	case errors.Is(err, state.ErrDecode):
		return true, shedengine.CancelIntent{}, nil
	case err != nil:
		return false, shedengine.CancelIntent{}, err
	case !found || record.Cancel == nil || record.Cancel.Finish != nil:
		return false, shedengine.CancelIntent{}, nil
	}
	return true, *record.Cancel, nil
}

// trySharedCancelLock takes the cancel's lock of slug shared, without waiting, and creates nothing.
// free is true when no cancel holds the lock.
// A lock file that does not exist reads free with a nil shared lock, which an absent scratch root does too.
func trySharedCancelLock(location *lymcwd.Location, slug string) (shared *lock.FileLock, free bool, err error) {
	lockPath := shedrun.CancelMarkerLock(location, slug)
	exists, err := markerExists(lockPath)
	if err != nil || !exists {
		return nil, true, err
	}
	return lock.TryAcquireReadLock(lockPath)
}

// readCancelLock reads only the cancel's lock of slug, which a cancel under way holds.
// It reads no record, so it is safe under batten's status lock, where the record is known absent.
func readCancelLock(location *lymcwd.Location, slug string) (cancelMarkerReading, error) {
	shared, free, err := trySharedCancelLock(location, slug)
	if err != nil {
		return cancelMarkerReading{}, err
	}
	if shared != nil {
		if err := shared.Release(); err != nil {
			return cancelMarkerReading{}, err
		}
	}
	return cancelMarkerReading{Present: !free, WriterAlive: !free}, nil
}

// readCancelMarker reads the cancel of slug: the intent in batten's record and the liveness of its writer.
// A record that does not decode reads present with an empty intent.
// A cancel lock held elsewhere reads present with a live writer whether or not the record holds an intent yet.
// A cancel writes its intent only after it takes the lock.
func readCancelMarker(location *lymcwd.Location, slug string) (cancelMarkerReading, error) {
	intentBefore, intent, err := readUnfinishedCancelIntent(location, slug)
	if err != nil {
		return cancelMarkerReading{}, err
	}

	shared, lockFree, err := trySharedCancelLock(location, slug)
	if err != nil {
		return cancelMarkerReading{}, err
	}
	intentUnderLock := intentBefore
	if shared != nil {
		intentUnderLock, _, err = readUnfinishedCancelIntent(location, slug)
		if releaseErr := shared.Release(); err == nil {
			err = releaseErr
		}
		if err != nil {
			return cancelMarkerReading{}, err
		}
	}

	present, writerAlive := judgeCancelMarker(intentBefore, lockFree, intentUnderLock)
	return cancelMarkerReading{Present: present, WriterAlive: writerAlive, Intent: intent}, nil
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

// heldCancelMarker is the cancel's lock, held by the cancel that took it.
type heldCancelMarker struct {
	held *lock.FileLock
}

// takeCancelMarker takes the cancel's lock exclusively, waiting at most wait, and writes no file.
// A shared probe of a reader holds the lock briefly, which the wait outlasts.
// A lock not taken within the wait refuses as a live cancel.
func takeCancelMarker(location *lymcwd.Location, slug string, wait time.Duration) (heldCancelMarker, error) {
	lockPath := shedrun.CancelMarkerLock(location, slug)
	if err := os.MkdirAll(shedrun.ScratchRoot(location), 0o755); err != nil {
		return heldCancelMarker{}, fmt.Errorf("battencli: create the scratch root for the cancel's lock: %w", err)
	}

	held, acquired, err := lock.AcquireWriteLockWithin(lockPath, wait)
	if err != nil {
		return heldCancelMarker{}, err
	}
	if !acquired {
		// The reading is best effort: an intent that cannot be read still refuses, with no start time to name.
		reading, _ := readCancelMarker(location, slug)
		reading.Present, reading.WriterAlive = true, true
		return heldCancelMarker{}, cancelMarkerRefusal(slug, reading)
	}
	return heldCancelMarker{held: held}, nil
}

// release drops the cancel's lock.
func (h heldCancelMarker) release() error {
	return h.held.Release()
}

// cancelMarkerRefusal words the refusal a driving verb or a second cancel gets for the cancel reading.
// A live writer is waited for, and named with the time its intent says it started.
// An interrupted cancel is finished by running the cancel again from the machine that started it.
// The rerun carries --remove when the intent asked for it.
// An interrupted cancel with no intent to read means the record does not decode, so the refusal names its repair by hand.
func cancelMarkerRefusal(slug string, reading cancelMarkerReading) error {
	if reading.WriterAlive {
		since := ""
		if reading.Intent.StartedAt != "" {
			since = " since " + reading.Intent.StartedAt
		}
		return fmt.Errorf("a cancel of %s is under way on this machine%s; way forward: wait for it to end, then run the verb again", slug, since)
	}
	if reading.Intent == (shedengine.CancelIntent{}) {
		return fmt.Errorf("a cancel of %s may have been interrupted, but its batten record does not decode, so a cancel cannot read it; "+
			"way forward: repair the record by hand, then run the verb again", slug)
	}
	command := fmt.Sprintf("lym batten cancel %s", slug)
	if reading.Intent.Remove {
		command += " --remove"
	}
	from := "the prime"
	if reading.Intent.Machine != "" {
		from = "the prime of machine " + reading.Intent.Machine
	}
	return fmt.Errorf("a cancel of %s was interrupted; way forward: run %q from %s to finish it", slug, command, from)
}
