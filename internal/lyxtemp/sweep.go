// sweep.go — the lock-guarded removal of dead owners' run directories.

package lyxtemp

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
)

// creationGrace is how long a run directory is left alone after its modification time, covering the window between its creation and its owner's lock.
const creationGrace = 10 * time.Minute

// removeAttempts is how many times a run directory's removal is tried before it is reported as a failure.
const removeAttempts = 3

// SweepReport is the outcome of one sweep.
type SweepReport struct {
	// Candidates are the run directories a dead owner left: removed when the sweep applied, otherwise the ones it would remove.
	Candidates []string
	// Failures are the entries the sweep could not lock or remove; each is left in place.
	Failures []SweepFailure
}

// SweepFailure is an entry a sweep could not handle.
type SweepFailure struct {
	Path string
	Err  error
}

// Sweep finds the run directories under the root whose owner is dead and past the creation grace, and removes them when apply is true.
// A dry run computes the same candidates and removes nothing.
// An entry that vanishes under a concurrent sweeper is no failure.
func (r *Root) Sweep(apply bool) SweepReport {
	var report SweepReport
	entries, err := os.ReadDir(r.path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			report.Failures = append(report.Failures, SweepFailure{Path: r.path, Err: err})
		}
		return report
	}
	now := r.now()
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), runPrefix) {
			continue
		}
		path := filepath.Join(r.path, entry.Name())
		dead, err := ownerDead(path, now)
		if err != nil {
			report.Failures = append(report.Failures, SweepFailure{Path: path, Err: err})
			continue
		}
		if !dead {
			continue
		}
		if apply {
			if err := removeRunDirectory(path); err != nil {
				report.Failures = append(report.Failures, SweepFailure{Path: path, Err: err})
				continue
			}
		}
		report.Candidates = append(report.Candidates, path)
	}
	return report
}

// removeRunDirectory removes path, retrying when a concurrent sweeper's lock probe recreates the lock file mid-removal.
// A path that no longer exists counts as removed.
func removeRunDirectory(path string) error {
	var err error
	for attempt := 0; attempt < removeAttempts; attempt++ {
		if err = os.RemoveAll(path); err == nil {
			return nil
		}
		if _, statErr := os.Lstat(path); errors.Is(statErr, fs.ErrNotExist) {
			return nil
		}
	}
	return err
}

// ownerDead reports whether path is a run directory past the creation grace whose owner holds no lock.
// The probe may briefly recreate a lock file a concurrent sweeper has just removed, and its own hold is released before ownerDead returns.
// A path that is no directory, is a link, or vanishes mid-check is not dead and no error.
func ownerDead(path string, now time.Time) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return false, ignoreNotExist(err)
	}
	if !info.Mode().IsDir() || now.Sub(info.ModTime()) <= creationGrace {
		return false, nil
	}
	lockPath := filepath.Join(path, lockFileName)
	if _, err := os.Lstat(lockPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return true, nil
		}
		return false, err
	}
	held, acquired, err := lock.TryAcquireWriteLock(lockPath)
	if err != nil {
		return false, ignoreNotExist(err)
	}
	if !acquired {
		return false, nil
	}
	return true, held.Release()
}

// ignoreNotExist turns a not-exist error into nil and returns any other error unchanged.
func ignoreNotExist(err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
