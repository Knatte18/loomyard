// sweep_test.go — tests for the dead-owner sweep (sweep.go).
//
// A dead owner is a run directory whose lock nobody holds, which is what a killed test binary leaves behind.

package lyxtemp

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/lock"
)

// newTestRoot opens a root over its own temp directory with the fixed clock.
func newTestRoot(t *testing.T) *Root {
	t.Helper()
	uid := os.Getuid()
	root, err := openRoot(filepath.Join(t.TempDir(), rootName(uid)), uid, fixedClock)
	if err != nil {
		t.Fatalf("openRoot: %v", err)
	}
	return root
}

// makeRunDirectory creates a directory at path with a lock file when withLock, aged by age against the fixed clock.
func makeRunDirectory(t *testing.T, path string, withLock bool, age time.Duration) {
	t.Helper()
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if withLock {
		if err := os.WriteFile(filepath.Join(path, lockFileName), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	modified := fixedClock().Add(-age)
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
}

func TestSweep(t *testing.T) {
	t.Parallel()
	past := creationGrace + time.Minute

	rows := []struct {
		name        string
		entry       string
		kind        string
		withLock    bool
		held        bool
		age         time.Duration
		wantRemoved bool
	}{
		{name: "dead owner past the grace", entry: "rdead", kind: "dir", withLock: true, age: past, wantRemoved: true},
		{name: "lock-less directory past the grace", entry: "rbare", kind: "dir", age: past, wantRemoved: true},
		{name: "live owner at any age", entry: "rlive", kind: "dir", withLock: true, held: true, age: 1000 * time.Hour},
		{name: "dead owner within the grace", entry: "rfresh", kind: "dir", withLock: true, age: creationGrace - time.Minute},
		{name: "unprefixed entry", entry: "other", kind: "dir", withLock: true, age: past},
		{name: "prefixed file", entry: "rfile", kind: "file", age: past},
		{name: "prefixed link", entry: "rlink", kind: "link", age: past},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			root := newTestRoot(t)
			path := filepath.Join(root.Path(), row.entry)
			var linkTargetMarker string
			switch row.kind {
			case "dir":
				makeRunDirectory(t, path, row.withLock, row.age)
				if row.held {
					held, err := lock.AcquireWriteLock(filepath.Join(path, lockFileName))
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { held.Release() })
				}
			case "file":
				if err := os.WriteFile(path, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			case "link":
				target := filepath.Join(t.TempDir(), "target")
				makeRunDirectory(t, target, true, row.age)
				linkTargetMarker = filepath.Join(target, lockFileName)
				if err := fslink.CreateDirLink(path, target); err != nil {
					t.Fatalf("create link: %v", err)
				}
			}

			var want []string
			if row.wantRemoved {
				want = []string{path}
			}
			dry := root.Sweep(false)
			if fmt.Sprint(dry.Candidates) != fmt.Sprint(want) || len(dry.Failures) != 0 {
				t.Fatalf("dry run = %+v, want candidates %v and no failures", dry, want)
			}
			if _, err := os.Lstat(path); err != nil {
				t.Fatalf("dry run removed %s: %v", path, err)
			}

			applied := root.Sweep(true)
			if fmt.Sprint(applied.Candidates) != fmt.Sprint(want) || len(applied.Failures) != 0 {
				t.Fatalf("apply = %+v, want candidates %v and no failures", applied, want)
			}
			_, err := os.Lstat(path)
			if gone := err != nil; gone != row.wantRemoved {
				t.Errorf("entry removed = %v, want %v", gone, row.wantRemoved)
			}
			if linkTargetMarker != "" {
				if _, err := os.Stat(linkTargetMarker); err != nil {
					t.Errorf("link target was touched: %v", err)
				}
			}
		})
	}

	t.Run("two concurrent sweeps both succeed", func(t *testing.T) {
		t.Parallel()
		root := newTestRoot(t)
		for i := range 20 {
			makeRunDirectory(t, filepath.Join(root.Path(), fmt.Sprintf("rdead%02d", i)), true, past)
		}
		reports := make([]SweepReport, 2)
		var wg sync.WaitGroup
		for i := range reports {
			wg.Add(1)
			go func() {
				defer wg.Done()
				reports[i] = root.Sweep(true)
			}()
		}
		wg.Wait()
		for i, report := range reports {
			if len(report.Failures) != 0 {
				t.Errorf("sweep %d failures = %+v, want none", i, report.Failures)
			}
		}
		if left := entryNames(t, root.Path()); len(left) != 0 {
			t.Errorf("root holds %v, want every dead run directory removed", left)
		}
	})

	// newRetryRoot returns a root whose retry clock is a retryClock, so no row sleeps, with a dead run directory per name.
	newRetryRoot := func(t *testing.T, names ...string) (*Root, []string) {
		t.Helper()
		root := newTestRoot(t)
		clock := &retryClock{current: fixedClock()}
		root.retryNow, root.sleep = clock.now, clock.sleep
		var paths []string
		for _, name := range names {
			path := filepath.Join(root.Path(), name)
			makeRunDirectory(t, path, true, past)
			paths = append(paths, path)
		}
		return root, paths
	}
	notEmpty := func(path string) error {
		return &fs.PathError{Op: "unlinkat", Path: path, Err: syscall.ENOTEMPTY}
	}

	t.Run("symptom past three attempts then removed", func(t *testing.T) {
		t.Parallel()
		root, paths := newRetryRoot(t, "rdead")
		attempts := 0
		root.remove = func(path string) error {
			attempts++
			if attempts <= 5 {
				return notEmpty(path)
			}
			return os.RemoveAll(path)
		}
		report := root.Sweep(true)
		if len(report.Failures) != 0 || fmt.Sprint(report.Candidates) != fmt.Sprint(paths) {
			t.Fatalf("report = %+v, want candidates %v and no failures", report, paths)
		}
		if _, err := os.Lstat(paths[0]); err == nil {
			t.Error("run directory still in place")
		}
		if attempts != 6 {
			t.Errorf("attempts = %d, want 6", attempts)
		}
	})

	t.Run("symptom past the budget fails and shares it", func(t *testing.T) {
		t.Parallel()
		root, paths := newRetryRoot(t, "rdead1", "rdead2")
		attempts := map[string]int{}
		root.remove = func(path string) error {
			attempts[path]++
			return notEmpty(path)
		}
		report := root.Sweep(true)
		if len(report.Candidates) != 0 || len(report.Failures) != 2 {
			t.Fatalf("report = %+v, want two failures and no candidates", report)
		}
		for i, failure := range report.Failures {
			if failure.Path != paths[i] || !errors.Is(failure.Err, syscall.ENOTEMPTY) {
				t.Errorf("failure %d = %+v, want %s with ENOTEMPTY", i, failure, paths[i])
			}
			if _, err := os.Lstat(paths[i]); err != nil {
				t.Errorf("run directory %s gone: %v", paths[i], err)
			}
		}
		if want := int(removeRetryBudget/removeRetryDelay) + 1; attempts[paths[0]] != want {
			t.Errorf("first directory attempts = %d, want %d", attempts[paths[0]], want)
		}
		if attempts[paths[1]] != 1 {
			t.Errorf("second directory attempts = %d, want 1", attempts[paths[1]])
		}
	})

	for _, row := range []struct {
		name string
		err  func(path string) error
	}{
		{"another error is reported after one attempt", func(path string) error {
			return &fs.PathError{Op: "unlinkat", Path: path, Err: syscall.EACCES}
		}},
		{"not-empty on a path inside is another error", func(path string) error {
			return &fs.PathError{Op: "unlinkat", Path: filepath.Join(path, lockFileName), Err: syscall.ENOTEMPTY}
		}},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			root, paths := newRetryRoot(t, "rdead")
			attempts := 0
			root.remove = func(path string) error {
				attempts++
				return row.err(path)
			}
			report := root.Sweep(true)
			if len(report.Failures) != 1 || len(report.Candidates) != 0 {
				t.Fatalf("report = %+v, want one failure and no candidates", report)
			}
			if attempts != 1 {
				t.Errorf("attempts = %d, want 1", attempts)
			}
			if _, err := os.Lstat(paths[0]); err != nil {
				t.Errorf("run directory gone: %v", err)
			}
		})
	}

	t.Run("a path gone between attempts counts as removed", func(t *testing.T) {
		t.Parallel()
		root, paths := newRetryRoot(t, "rdead")
		root.remove = func(path string) error {
			if err := os.RemoveAll(path); err != nil {
				return err
			}
			return notEmpty(path)
		}
		report := root.Sweep(true)
		if len(report.Failures) != 0 || fmt.Sprint(report.Candidates) != fmt.Sprint(paths) {
			t.Fatalf("report = %+v, want candidates %v and no failures", report, paths)
		}
	})
}

// retryClock is a retry clock that advances only by sleeping, so a sweep's whole retry budget passes without waiting.
type retryClock struct {
	current time.Time
}

func (c *retryClock) now() time.Time { return c.current }

func (c *retryClock) sleep(d time.Duration) { c.current = c.current.Add(d) }
