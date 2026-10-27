// dobbywire.go exports the two batten wirings the beam daemon reuses: the release of a run's parent lock record, and a shuttle runner over a worktree's reed geometry.

package battencli

import (
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ReleaseRunParentLock releases the parent lock record that names the run of slug's task pair, as the run's own owner pause does.
// A pair absent on this machine, a run with no record and a Finalize past its parent-side merge, which keeps the record, release nothing.
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
	store := &parentlock.Store{Dir: parentlock.Dir(fabricengine.BoardDir(prime.HubPath))}
	record, found, err := runLockRecord(store, slug, taskLocation.WorktreePath())
	if err != nil || !found {
		return err
	}
	_, err = store.Release(record.Branch, record.Owner)
	return err
}

// ShuttleRunner builds a shuttle runner over the reed geometry of the worktree location names, so a strand it starts opens a window in that worktree's session.
func ShuttleRunner(location *lymcwd.Location) (*shuttleengine.Runner, error) {
	return primeShuttleRunner(location)
}
