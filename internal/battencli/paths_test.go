package battencli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
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
			want := filepath.Join(l.AnchorPath(), ".lyx", "shed", "some-slug")
			if got != want {
				t.Errorf("BattenDir() = %q; want %q", got, want)
			}
		})
	}
}

// TestStatusFileIsDurableWhileLocksStayEphemeral asserts the durable/ephemeral split this batch
// introduces: StatusFile now lives under the fabric-synced _lyx segment, while RunLock, StatusLock
// and PrimeRunLock stay under the never-tracked .lyx segment, per the Durable-vs-Ephemeral State
// Invariant.
func TestStatusFileIsDurableWhileLocksStayEphemeral(t *testing.T) {
	for name, l := range locationFixtures(t) {
		t.Run(name, func(t *testing.T) {
			durablePrefix := filepath.Join(l.AnchorPath(), lyxdirs.LyxDirName) + string(filepath.Separator)
			ephemeralPrefix := filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName) + string(filepath.Separator)

			if got := StatusFile(l, "some-slug"); !strings.HasPrefix(got, durablePrefix) {
				t.Errorf("StatusFile() = %q; want it under the durable segment %q", got, durablePrefix)
			}
			for name, got := range map[string]string{
				"RunLock":      RunLock(l, "some-slug"),
				"StatusLock":   StatusLock(l, "some-slug"),
				"PrimeRunLock": PrimeRunLock(l),
			} {
				if !strings.HasPrefix(got, ephemeralPrefix) {
					t.Errorf("%s() = %q; want it under the ephemeral segment %q", name, got, ephemeralPrefix)
				}
			}
		})
	}
}

func TestPerSlugPathsAreDistinctAndTheTwoLocksAreUnderBattenDir(t *testing.T) {
	for name, l := range locationFixtures(t) {
		t.Run(name, func(t *testing.T) {
			dir := BattenDir(l, "some-slug")
			statusFile := StatusFile(l, "some-slug")
			runLock := RunLock(l, "some-slug")
			statusLock := StatusLock(l, "some-slug")

			// Only the two ephemeral locks live under BattenDir now: StatusFile is durable, under
			// the mirrored _lyx segment instead, per the durable/ephemeral split this batch
			// introduces.
			for _, p := range []struct {
				name string
				path string
			}{
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
			want := filepath.Join(l.AnchorPath(), ".lyx", "shed", "run.lock")
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
