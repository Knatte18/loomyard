// tracked_test.go covers the lock paths of tracked files (tracked.go).
// It also covers the refusal of a lock inside a tracked lym or Bolt directory.

package lock_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// resolvedTempDir returns a fresh temp dir with its symlinks followed, the form the lock paths are built from.
func resolvedTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	return dir
}

func TestTrackedFileLockPath(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	root := resolvedTempDir(t)
	holder := filepath.Join(root, "holder")
	linker := filepath.Join(root, "linker")
	reviews := filepath.Join(holder, lymdirs.LymDirName, "reviews", "code")
	if err := os.MkdirAll(reviews, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.MkdirAll(linker, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := fslink.CreateDirLink(filepath.Join(linker, lymdirs.LymDirName), filepath.Join(holder, lymdirs.LymDirName)); err != nil {
		t.Fatalf("CreateDirLink: %v", err)
	}
	wantMirrored := filepath.Join(holder, lymdirs.DotLymDirName, lymdirs.LocksDirName, lymdirs.LymDirName, "reviews", "code", "record.json.lock")
	outside := filepath.Join(root, "plain", "record.json")

	tests := []struct {
		name string
		path string
		want string
	}{
		{"UnderLymDir_MirroredUnderLocksDir", filepath.Join(reviews, "record.json"), wantMirrored},
		{"ThroughLinkedLymDir_SameLockAsTheDirItself", filepath.Join(linker, lymdirs.LymDirName, "reviews", "code", "record.json"), wantMirrored},
		{"UnderAbsentLymDir_MirroredUnderItsParent", filepath.Join(root, "fresh", lymdirs.LymDirName, "record.json"), filepath.Join(root, "fresh", lymdirs.DotLymDirName, lymdirs.LocksDirName, lymdirs.LymDirName, "record.json.lock")},
		{"RelativePathUnderLymDir_MirroredUnderItsParent", filepath.Join("relative", lymdirs.LymDirName, "record.json"), filepath.Join("relative", lymdirs.DotLymDirName, lymdirs.LocksDirName, lymdirs.LymDirName, "record.json.lock")},
		{"OutsideAnyLymDir_Beside", outside, outside + ".lock"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := lock.TrackedFileLockPath(tt.path, ".lock"); got != tt.want {
				t.Errorf("TrackedFileLockPath(%q, %q) = %q; want %q", tt.path, ".lock", got, tt.want)
			}
		})
	}
}

func TestTrackedLockPath(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	root := resolvedTempDir(t)
	real := filepath.Join(root, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	link := filepath.Join(root, "link")
	if err := fslink.CreateDirLink(link, real); err != nil {
		t.Fatalf("CreateDirLink: %v", err)
	}
	wantUnder := func(checkout string) string {
		return filepath.Join(checkout, lymdirs.DotLymDirName, lymdirs.LocksDirName, "board.lock")
	}

	tests := []struct {
		name string
		root string
		want string
	}{
		{"ResolvedRoot", real, wantUnder(real)},
		{"LinkedRoot_SameLockAsTheCheckoutItself", link, wantUnder(real)},
		{"AbsentRoot_UsedAsGiven", filepath.Join(root, "absent"), wantUnder(filepath.Join(root, "absent"))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := lock.TrackedLockPath(tt.root, "board.lock"); got != tt.want {
				t.Errorf("TrackedLockPath(%q, %q) = %q; want %q", tt.root, "board.lock", got, tt.want)
			}
		})
	}
}

func TestAcquire_LockInTrackedTree(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	acquirers := []struct {
		name    string
		acquire func(path string) (*lock.FileLock, error)
	}{
		{"AcquireWriteLock", lock.AcquireWriteLock},
		{"TryAcquireWriteLock", func(path string) (*lock.FileLock, error) {
			l, _, err := lock.TryAcquireWriteLock(path)
			return l, err
		}},
		{"AcquireWriteLockWithin", func(path string) (*lock.FileLock, error) {
			l, _, err := lock.AcquireWriteLockWithin(path, time.Second)
			return l, err
		}},
		{"AcquireReadLock", lock.AcquireReadLock},
		{"TryAcquireReadLock", func(path string) (*lock.FileLock, error) {
			l, _, err := lock.TryAcquireReadLock(path)
			return l, err
		}},
	}
	for _, tt := range acquirers {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := resolvedTempDir(t)
			tracked := filepath.Join(root, lymdirs.LymDirName, "reviews")
			bolt := filepath.Join(root, lymdirs.BoltDirName)
			if err := os.MkdirAll(bolt, 0o755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			if err := os.WriteFile(filepath.Join(bolt, ".git"), []byte("gitdir: elsewhere\n"), 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			refused := []string{
				filepath.Join(tracked, "record.json.lock"),
				filepath.Join(bolt, "board.lock"),
				filepath.Join(bolt, "board.json.swaplock"),
				filepath.Join(bolt, "sub", "board.lock"),
				filepath.Join(root, lymdirs.DotLymDirName, "checkout", lymdirs.LymDirName, "record.json.lock"),
			}
			for _, path := range refused {
				if l, err := tt.acquire(path); err == nil {
					_ = l.Release()
					t.Errorf("%s(%q) error = nil; want a refusal of a lock inside a tracked tree", tt.name, path)
				}
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Errorf("Stat(%q) error = %v; want the refused lock file absent", path, err)
				}
			}

			accepted := []string{
				lock.TrackedFileLockPath(filepath.Join(tracked, "record.json"), ".lock"),
				lock.TrackedLockPath(bolt, "board.lock"),
				filepath.Join(bolt, lymdirs.DotLymDirName, lymdirs.PauseDirName, "pause.lock"),
				filepath.Join(root, "worktrees", lymdirs.BoltDirName, "lym-push.lock"),
			}
			for _, path := range accepted {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				l, err := tt.acquire(path)
				if err != nil {
					t.Fatalf("%s(%q) error = %v; want a lock outside every tracked tree taken", tt.name, path, err)
				}
				if err := l.Release(); err != nil {
					t.Errorf("Release(%q) error = %v", path, err)
				}
			}
		})
	}
}
