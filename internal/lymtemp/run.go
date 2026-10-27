// run.go — one locked run directory under a root.

package lymtemp

import (
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lock"
)

const (
	// runPrefix starts every run directory's name; the sweep considers no other entry.
	runPrefix = "r"

	// runNameAttempts bounds how many names newRun draws before it gives up on a root full of taken ones.
	runNameAttempts = 16

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
// A taken name is retried with a fresh draw up to runNameAttempts times; any other creation error returns at once.
// The lock is a blocking one, so a sweeper's momentary hold delays the owner rather than failing it.
func (r *Root) newRun() (*run, error) {
	r.Sweep(true)
	var dir string
	for range runNameAttempts {
		candidate := filepath.Join(r.path, r.runName())
		err := os.Mkdir(candidate, 0o700)
		if err == nil {
			dir = candidate
			break
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("create run directory: %w", err)
		}
	}
	if dir == "" {
		return nil, fmt.Errorf("create run directory under %q: %d names taken", r.path, runNameAttempts)
	}
	held, err := lock.AcquireWriteLock(filepath.Join(dir, lockFileName))
	if err != nil {
		return nil, errors.Join(fmt.Errorf("lock run directory %q: %w", dir, err), os.RemoveAll(dir))
	}
	return &run{dir: dir, lock: held}, nil
}

// randomRunName returns runPrefix followed by six random lower-case hex digits.
// The short name keeps a tmux socket path below a run directory under the sun_path limit.
func randomRunName() string {
	return fmt.Sprintf("%s%06x", runPrefix, rand.Uint32N(1<<24))
}

// remove releases the lock, then deletes the directory, since Windows cannot delete a locked file.
func (u *run) remove() error {
	releaseErr := u.lock.Release()
	return errors.Join(releaseErr, os.RemoveAll(u.dir))
}
