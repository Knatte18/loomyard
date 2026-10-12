// run_test.go — tests for run directories (run.go) and the process API (lyxtemp.go).

package lyxtemp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
)

func TestNewRun_HoldsItsLockUntilRemoved(t *testing.T) {
	t.Parallel()
	root := newTestRoot(t)

	created, err := root.newRun()
	if err != nil {
		t.Fatalf("newRun: %v", err)
	}
	if filepath.Dir(created.dir) != root.Path() || !strings.HasPrefix(filepath.Base(created.dir), runPrefix) {
		t.Fatalf("run directory = %q, want a %q-prefixed child of %q", created.dir, runPrefix, root.Path())
	}
	probe, acquired, err := lock.TryAcquireWriteLock(filepath.Join(created.dir, lockFileName))
	if err != nil {
		t.Fatal(err)
	}
	if acquired {
		probe.Release()
		t.Fatal("the run directory's lock was free while its owner lived")
	}

	if err := created.remove(); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Lstat(created.dir); err == nil {
		t.Error("run directory survived remove")
	}
}

func TestNewRun_InheritedRunDirectoryLandsBesideTheParents(t *testing.T) {
	t.Parallel()
	uid := os.Getuid()
	later := func() time.Time { return fixedClock().Add(24 * time.Hour) }
	parentRoot, err := openRoot(filepath.Join(t.TempDir(), rootName(uid)), uid, later)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := parentRoot.newRun()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { parent.remove() })
	marker := filepath.Join(parent.dir, "marker")
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	childPath := resolveRootPath(parent.dir, uid)
	if childPath != parentRoot.Path() {
		t.Fatalf("resolveRootPath(parent run) = %q, want the parent's root %q", childPath, parentRoot.Path())
	}
	childRoot, err := openRoot(childPath, uid, later)
	if err != nil {
		t.Fatal(err)
	}
	child, err := childRoot.newRun()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { child.remove() })

	if filepath.Dir(child.dir) != filepath.Dir(parent.dir) || child.dir == parent.dir {
		t.Errorf("child run directory %q is not a sibling of %q", child.dir, parent.dir)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("the child's sweep touched the parent's run directory: %v", err)
	}
}

// TestProcessDirectory_StaysOneDirectoryUnderTheRoot leaves the run directory in place: the test binary's own run directory is removed by its TestMain.
func TestProcessDirectory_StaysOneDirectoryUnderTheRoot(t *testing.T) {
	t.Parallel()
	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	again, err := Dir()
	if err != nil || again != dir {
		t.Fatalf("second Dir = %q, %v, want %q", again, err, dir)
	}

	made, err := MkdirTemp("made-")
	if err != nil {
		t.Fatal(err)
	}
	file, err := CreateTemp("made-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	for _, path := range []string{made, file.Name()} {
		if filepath.Dir(path) != dir {
			t.Errorf("%q is not directly under the run directory %q", path, dir)
		}
	}
}
