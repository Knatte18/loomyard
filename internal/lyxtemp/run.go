// run.go — one locked run directory under a root.

package lyxtemp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lock"
)

const (
	// runPrefix starts every run directory's name; the sweep considers no other entry.
	runPrefix = "r"

	// lockFileName is the file in a run directory whose lock its owner holds.
	lockFileName = "run.lock"
)

// run is a run directory and the lock its owner holds on it.
type run struct {
	dir  string
	lock *lock.FileLock
}

// newRun sweeps the root of dead owners' run directories, then creates a run directory and locks it.
// The sweep's failures are dropped, since the next start retries them.
// The lock is a blocking one, so a sweeper's momentary hold delays the owner rather than failing it.
func (r *Root) newRun() (*run, error) {
	r.Sweep(true)
	dir, err := os.MkdirTemp(r.path, runPrefix)
	if err != nil {
		return nil, fmt.Errorf("create run directory: %w", err)
	}
	held, err := lock.AcquireWriteLock(filepath.Join(dir, lockFileName))
	if err != nil {
		return nil, errors.Join(fmt.Errorf("lock run directory %q: %w", dir, err), os.RemoveAll(dir))
	}
	return &run{dir: dir, lock: held}, nil
}

// remove releases the lock, then deletes the directory, since Windows cannot delete a locked file.
func (u *run) remove() error {
	releaseErr := u.lock.Release()
	return errors.Join(releaseErr, os.RemoveAll(u.dir))
}
