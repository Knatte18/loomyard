package battencli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// locationFixtures returns two synthetic *lymcwd.Location fixtures:
// one anchored at the worktree root ("."), and one anchored at a subpath.
// Every path constructor is exercised against both anchoring shapes.
func locationFixtures(t *testing.T) map[string]*lymcwd.Location {
	t.Helper()
	hub := t.TempDir()
	return map[string]*lymcwd.Location{
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

// TestPaths_ResolveUnderTheAnchorBySegment asserts, over a root-anchored and a subpath-anchored Location,
// the path constructors' whole contract:
//   - BattenDir, PrimeRunLock and LandLock sit at their fixed spots under the ephemeral segment;
//   - StatusFile lives under the fabric-synced _lym segment;
//   - the four other locks stay under the never-tracked .lym segment, per the Durable-vs-Ephemeral State Invariant;
//   - the two per-slug locks sit under BattenDir;
//   - StatusFile, RunLock and StatusLock are pairwise distinct;
//   - shedengine.Shed rejects RunLock equal to StatusLock outright, so the clash must not first surface at runtime;
//   - every path stays under the Location's own anchor, and none contains a managed task worktree's own path;
//     this is the package's half of the Batten Bookend Invariant's mechanical proxy.
//     Every seam resolves against the prime Location's anchored tree.
//     The managed slug's worktree does not exist at wiring time, so no seam resolves against it.
//
//testtiming:keep pins every path constructor's segment, distinctness and containment, which the integration steps only use as inputs
func TestPaths_ResolveUnderTheAnchorBySegment(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	for name, l := range locationFixtures(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			const managedSlug = "managed-task-slug"
			separator := string(filepath.Separator)
			durablePrefix := filepath.Join(l.AnchorPath(), lymdirs.LymDirName) + separator
			ephemeralPrefix := filepath.Join(l.AnchorPath(), lymdirs.DotLymDirName) + separator

			dir := BattenDir(l, managedSlug)
			statusFile := StatusFile(l, managedSlug)
			runLock := RunLock(l, managedSlug)
			statusLock := StatusLock(l, managedSlug)
			primeRunLock := PrimeRunLock(l)
			landLock := LandLock(l)

			if want := filepath.Join(l.AnchorPath(), ".lym", "shed", managedSlug); dir != want {
				t.Errorf("BattenDir() = %q; want %q", dir, want)
			}
			if want := filepath.Join(l.AnchorPath(), ".lym", "shed", "run.lock"); primeRunLock != want {
				t.Errorf("PrimeRunLock() = %q; want %q", primeRunLock, want)
			}
			if want := filepath.Join(l.AnchorPath(), ".lym", "shed", "land.lock"); landLock != want {
				t.Errorf("LandLock() = %q; want %q", landLock, want)
			}

			if !strings.HasPrefix(statusFile, durablePrefix) {
				t.Errorf("StatusFile() = %q; want it under the durable segment %q", statusFile, durablePrefix)
			}
			for pathName, got := range map[string]string{"RunLock": runLock, "StatusLock": statusLock, "PrimeRunLock": primeRunLock, "LandLock": landLock} {
				if !strings.HasPrefix(got, ephemeralPrefix) {
					t.Errorf("%s() = %q; want it under the ephemeral segment %q", pathName, got, ephemeralPrefix)
				}
			}

			// Only the two ephemeral locks live under BattenDir: StatusFile is durable, under the mirrored _lym segment instead.
			for pathName, got := range map[string]string{"RunLock": runLock, "StatusLock": statusLock} {
				if !strings.HasPrefix(got, dir+separator) {
					t.Errorf("%s() = %q; want it under BattenDir %q", pathName, got, dir)
				}
			}
			if runLock == statusLock || statusFile == runLock || statusFile == statusLock {
				t.Errorf("StatusFile() = %q, RunLock() = %q, StatusLock() = %q; want pairwise distinct paths", statusFile, runLock, statusLock)
			}

			managedWorktreePath := filepath.Join(l.HubPath, managedSlug)
			for pathName, got := range map[string]string{
				"BattenDir": dir, "StatusFile": statusFile, "RunLock": runLock, "StatusLock": statusLock, "PrimeRunLock": primeRunLock, "LandLock": landLock,
			} {
				if !strings.HasPrefix(got, l.AnchorPath()+separator) && got != l.AnchorPath() {
					t.Errorf("%s() = %q; want it under Location's own anchor %q", pathName, got, l.AnchorPath())
				}
				if strings.Contains(got, managedWorktreePath) {
					t.Errorf("%s() = %q; want it to never contain the managed task worktree's own path %q", pathName, got, managedWorktreePath)
				}
			}
		})
	}
}
