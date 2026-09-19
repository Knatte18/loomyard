package battencli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// locationFixtures returns two synthetic *lyxcwd.Location fixtures: one anchored at the worktree
// root ("."), and one anchored at a subpath, so every path constructor is exercised against both
// anchoring shapes.
func locationFixtures(t *testing.T) map[string]*lyxcwd.Location {
	t.Helper()
	hub := t.TempDir()
	return map[string]*lyxcwd.Location{
		"RootAnchored": {
			RepoName:     "example",
			HubPath:      hub,
			WorktreeName: "task-slug",
			AnchorRel:    ".",
		},
		"SubpathAnchored": {
			RepoName:     "example",
			HubPath:      hub,
			WorktreeName: "task-slug",
			AnchorRel:    "backend",
		},
	}
}

func TestBattenDir(t *testing.T) {
	for name, l := range locationFixtures(t) {
		t.Run(name, func(t *testing.T) {
			got := BattenDir(l, "some-slug")
			want := filepath.Join(l.AnchorPath(), ".lyx", "lifecycle", "some-slug")
			if got != want {
				t.Errorf("BattenDir() = %q; want %q", got, want)
			}
		})
	}
}

func TestPerSlugPathsAreDistinctAndUnderBattenDir(t *testing.T) {
	for name, l := range locationFixtures(t) {
		t.Run(name, func(t *testing.T) {
			dir := BattenDir(l, "some-slug")
			statusFile := StatusFile(l, "some-slug")
			runLock := RunLock(l, "some-slug")
			statusLock := StatusLock(l, "some-slug")

			for _, p := range []struct {
				name string
				path string
			}{
				{"StatusFile", statusFile},
				{"RunLock", runLock},
				{"StatusLock", statusLock},
			} {
				if !strings.HasPrefix(p.path, dir+string(filepath.Separator)) {
					t.Errorf("%s() = %q; want it under BattenDir %q", p.name, p.path, dir)
				}
			}

			// RunLock differing from StatusLock in particular is what shedengine.Shed's own
			// validation rejects outright, and it must not first surface at runtime.
			if runLock == statusLock {
				t.Errorf("RunLock() = StatusLock() = %q; want distinct paths", runLock)
			}
			if statusFile == runLock {
				t.Errorf("StatusFile() = RunLock() = %q; want distinct paths", statusFile)
			}
			if statusFile == statusLock {
				t.Errorf("StatusFile() = StatusLock() = %q; want distinct paths", statusFile)
			}
		})
	}
}

func TestPrimeRunLock(t *testing.T) {
	for name, l := range locationFixtures(t) {
		t.Run(name, func(t *testing.T) {
			got := PrimeRunLock(l)
			want := filepath.Join(l.AnchorPath(), ".lyx", "lifecycle", "run.lock")
			if got != want {
				t.Errorf("PrimeRunLock() = %q; want %q", got, want)
			}
		})
	}
}

// TestPathsStayUnderAnchorAndNeverNameTheManagedSlugWorktree asserts every returned path is under
// the given Location's own anchor, and that none of them contains a managed task worktree's own
// path -- this package's half of the Batten Bookend Invariant's mechanical proxy: every seam this
// package builds resolves against the prime Location's own anchored tree, never against the managed
// slug's worktree path, which does not exist at wiring time.
func TestPathsStayUnderAnchorAndNeverNameTheManagedSlugWorktree(t *testing.T) {
	for name, l := range locationFixtures(t) {
		t.Run(name, func(t *testing.T) {
			const managedSlug = "managed-task-slug"
			managedWorktreePath := filepath.Join(l.HubPath, managedSlug)

			paths := map[string]string{
				"BattenDir":    BattenDir(l, managedSlug),
				"StatusFile":   StatusFile(l, managedSlug),
				"RunLock":      RunLock(l, managedSlug),
				"StatusLock":   StatusLock(l, managedSlug),
				"PrimeRunLock": PrimeRunLock(l),
			}

			for name, p := range paths {
				if !strings.HasPrefix(p, l.AnchorPath()+string(filepath.Separator)) && p != l.AnchorPath() {
					t.Errorf("%s() = %q; want it under Location's own anchor %q", name, p, l.AnchorPath())
				}
				if strings.Contains(p, managedWorktreePath) {
					t.Errorf("%s() = %q; want it to never contain the managed task worktree's own path %q", name, p, managedWorktreePath)
				}
			}
		})
	}
}
