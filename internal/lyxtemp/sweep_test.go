// sweep_test.go — tests for the dead-owner sweep (sweep.go).
//
// A dead owner is a run directory whose lock nobody holds, which is what a killed test binary leaves behind.

package lyxtemp

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
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
}
