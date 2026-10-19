// run_test.go — tests for run directories (run.go) and the process API (lyxtemp.go).

package lyxtemp

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
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
	if !regexp.MustCompile(`^r[0-9a-f]{6}$`).MatchString(filepath.Base(created.dir)) {
		t.Errorf("run directory name %q does not match r plus six hex digits", filepath.Base(created.dir))
	}
	info, err := os.Stat(created.dir)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Errorf("mode = %04o, want 0700", info.Mode().Perm())
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

func TestNewRun_RetriesATakenNameUpToTheBound(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		// script yields the drawn names in order; the last repeats when the draws outrun it.
		script []string
		// breakRoot removes the root directory after openRoot, so creation fails with fs.ErrNotExist.
		breakRoot bool
		wantDraws int
		wantErr   func(t *testing.T, err error, rootPath string)
		wantDir   string
	}{
		{
			name:      "a taken name is retried",
			script:    []string{"rtaken", "rfresh"},
			wantDraws: 2,
			wantDir:   "rfresh",
		},
		{
			name:      "the bound spent fails naming the root",
			script:    []string{"rtaken"},
			wantDraws: runNameAttempts,
			wantErr: func(t *testing.T, err error, rootPath string) {
				if !strings.Contains(err.Error(), rootPath) {
					t.Errorf("error %q does not name the root %q", err, rootPath)
				}
			},
		},
		{
			name:      "another mkdir error returns at once",
			script:    []string{"rfresh"},
			breakRoot: true,
			wantDraws: 1,
			wantErr: func(t *testing.T, err error, rootPath string) {
				if !errors.Is(err, fs.ErrNotExist) {
					t.Errorf("error %v is not fs.ErrNotExist", err)
				}
				if strings.Contains(err.Error(), "names taken") {
					t.Errorf("error %q is the bound error", err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := newTestRoot(t)
			if err := os.Mkdir(filepath.Join(root.Path(), "rtaken"), 0o700); err != nil {
				t.Fatal(err)
			}
			draws := 0
			root.runName = func() string {
				name := tt.script[min(draws, len(tt.script)-1)]
				draws++
				return name
			}
			if tt.breakRoot {
				if err := os.RemoveAll(root.Path()); err != nil {
					t.Fatal(err)
				}
			}

			created, err := root.newRun()
			if draws != tt.wantDraws {
				t.Errorf("draws = %d, want %d", draws, tt.wantDraws)
			}
			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("newRun succeeded, want an error")
				}
				tt.wantErr(t, err, root.Path())
				if !tt.breakRoot {
					entries, readErr := os.ReadDir(root.Path())
					if readErr != nil {
						t.Fatal(readErr)
					}
					if len(entries) != 1 {
						t.Errorf("root holds %d entries, want only the pre-created one", len(entries))
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("newRun: %v", err)
			}
			t.Cleanup(func() { created.remove() })
			if want := filepath.Join(root.Path(), tt.wantDir); created.dir != want {
				t.Errorf("run directory = %q, want %q", created.dir, want)
			}
			probe, acquired, lockErr := lock.TryAcquireWriteLock(filepath.Join(created.dir, lockFileName))
			if lockErr != nil {
				t.Fatal(lockErr)
			}
			if acquired {
				probe.Release()
				t.Error("the fresh run directory was not locked")
			}
		})
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
