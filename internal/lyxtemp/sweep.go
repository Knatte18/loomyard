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

// removeRetryBudget is how long one sweep keeps retrying removals that hit the not-empty symptom, shared by all the directories it removes.
const removeRetryBudget = time.Second

// removeRetryDelay is the pause between symptom retries, long enough for a peer's lock probe to release a recreated lock file.
const removeRetryDelay = 5 * time.Millisecond

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
	var deadline time.Time
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
			if err := removeRunDirectory(r, path, &deadline); err != nil {
				report.Failures = append(report.Failures, SweepFailure{Path: path, Err: err})
				continue
			}
		}
		report.Candidates = append(report.Candidates, path)
	}
	return report
}

// removeRunDirectory removes path through r.remove, retrying while a concurrent sweeper's lock probe keeps recreating the lock file mid-removal.
// deadline is the sweep's shared retry deadline, zero until the first symptom sets it to r.retryNow() plus removeRetryBudget.
// A path that no longer exists counts as removed, and a failure that is not the symptom is returned after its one attempt.
func removeRunDirectory(r *Root, path string, deadline *time.Time) error {
	for {
		err := r.remove(path)
		if err == nil {
			return nil
		}
		if _, statErr := os.Lstat(path); errors.Is(statErr, fs.ErrNotExist) {
			return nil
		}
		if !isRemovalSymptom(err, path) {
			return err
		}
		if deadline.IsZero() {
			*deadline = r.retryNow().Add(removeRetryBudget)
		}
		if !r.retryNow().Before(*deadline) {
			return err
		}
		r.sleep(removeRetryDelay)
	}
}

// isRemovalSymptom reports whether err is the directory-not-empty failure of removing path itself, which a peer's lock probe recreating the lock file causes.
// Both errors POSIX allows rmdir of a non-empty directory, and Windows' directory-not-empty error, satisfy fs.ErrExist.
// A not-empty error on a path inside path is no symptom.
func isRemovalSymptom(err error, path string) bool {
	var pathErr *fs.PathError
	return errors.As(err, &pathErr) &&
		filepath.Clean(pathErr.Path) == filepath.Clean(path) &&
		errors.Is(pathErr.Err, fs.ErrExist)
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
