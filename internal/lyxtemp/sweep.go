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
			if err := os.RemoveAll(path); err != nil {
				report.Failures = append(report.Failures, SweepFailure{Path: path, Err: err})
				continue
			}
		}
		report.Candidates = append(report.Candidates, path)
	}
	return report
}

// ownerDead reports whether path is a run directory past the creation grace whose owner holds no lock.
// A live owner's lock is only probed, never created, and the probe's own hold is released before ownerDead returns.
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
