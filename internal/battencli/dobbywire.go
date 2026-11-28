// dobbywire.go exports the two battencli wirings the beam daemon reuses.
// One releases a run's parent lock record; the other builds a shuttle runner over a worktree's reed geometry.

package battencli

import (
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ReleaseRunParentLock releases the parent lock record that names the run of slug's task pair.
// It releases nothing when the pair is absent on this machine or the run has no record.
// It also releases nothing when a Finalize past its parent-side merge keeps the record.
func ReleaseRunParentLock(prime *lymcwd.Location, slug string) error {
	present, err := taskWorktreePresent(prime, slug)
	if err != nil || !present {
		return err
	}
	taskLocation, err := taskWorktreeLocation(prime, slug)
	if err != nil {
		return err
	}
	keeps, err := landingInFlight(taskLocation)
	if err != nil || keeps {
		return err
	}
	store := &parentlock.Store{Dir: parentlock.Dir(fabricengine.BoltDir(prime.HubPath))}
	record, found, err := runLockRecord(store, slug, taskLocation.WorktreePath())
	if err != nil || !found {
		return err
	}
	_, err = store.Release(record.Branch, record.Owner)
	return err
}

// ShuttleRunner builds a shuttle runner over the reed geometry of the worktree location names,
// so a strand it starts opens a window in that worktree's session.
func ShuttleRunner(location *lymcwd.Location) (*shuttleengine.Runner, error) {
	return primeShuttleRunner(location)
}
