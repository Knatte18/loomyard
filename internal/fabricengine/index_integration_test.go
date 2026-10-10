//go:build integration

// index_integration_test.go — integration tests for the fabric layer's git
// wiring around the correspondence index: gitdir resolution, the
// RecordCorrespondence/WeftSHAForWarpSHA round trip, and RebuildIndex's
// trailer scan. Package fabricengine_test, driving weftGitDir through
// export_test.go's WeftGitDirForTest shim. Uses hubforge.NewHub for the weft
// side and a minimal, locally-built plain git repo for the warp side —
// fabric's warp is just an ordinary warp repo, so these tests need none of
// the real hub's junction/portal wiring on the warp side.

package fabricengine_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// commitWeftWithTrailer commits content into weftPath's tracked _lyx config
// file with a Warp-SHA trailer naming warpSHA — a hand-crafted stand-in for
// what CommitWeft (a later batch) produces — returning the new weft HEAD SHA.
func commitWeftWithTrailer(t *testing.T, weftPath, content, warpSHA string) string {
	t.Helper()

	msg := fabricengine.AppendWarpSHATrailerForTest("weft sync", warpSHA)
	return gitkit.CommitFile(t, weftPath, filepath.Join("_lyx", "config.yaml"), content, msg)
}

// TestCorrespondenceIndex runs the correspondence index's git wiring over one plain warp repo and one
// hub weft: gitdir resolution, the miss path, the RecordCorrespondence/WeftSHAForWarpSHA round trip
// and RebuildIndex's trailer scan.
// The steps share the fixture and run in order.
// Each commits its own warp and weft commits, so none reads an earlier step's entries.
// The rebuild step's lookups succeed only through the rebuild, never through an earlier step's RecordCorrespondence.
func TestCorrespondenceIndex(t *testing.T) {
	t.Parallel()

	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	weftFixture := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	f := fabricengine.NewFabricForTest(t, warpPath, weftFixture.PrimeRecords())

	// weftGitDir returns a path genuinely inside the weft worktree's own .git directory — the
	// per-worktree gitdir the correspondence index is deliberately scoped to.
	t.Run("WeftGitDirResolvesInsideWeftGitdir", func(t *testing.T) {
		gitDir, err := fabricengine.WeftGitDirForTest(f)
		if err != nil {
			t.Fatalf("weftGitDir() error = %v", err)
		}
		wantPrefix := filepath.Join(weftFixture.PrimeRecords(), ".git")
		if !strings.HasPrefix(gitDir, wantPrefix) {
			t.Errorf("weftGitDir() = %q; want it under %q", gitDir, wantPrefix)
		}
	})

	// The miss path: a warp SHA with no recorded correspondence at all.
	t.Run("NoEntryReturnsErrNoCorrespondence", func(t *testing.T) {
		warpSHA := fabricengine.CommitWarpForTest(t, warpPath, "warp change, never synced")

		if _, err := f.WeftSHAForWarpSHA(warpSHA); !errors.Is(err, fabricengine.ErrNoCorrespondence) {
			t.Errorf("WeftSHAForWarpSHA() error = %v; want errors.Is(err, fabricengine.ErrNoCorrespondence)", err)
		}
	})

	// A RecordCorrespondence call is visible to a subsequent WeftSHAForWarpSHA lookup, with WarpSeq
	// computed from the warp repo's first-parent commit count.
	t.Run("RecordAndLookupRoundTrip", func(t *testing.T) {
		warpSHA := fabricengine.CommitWarpForTest(t, warpPath, "warp change 1")
		weftSHA := commitWeftWithTrailer(t, weftFixture.PrimeRecords(), "weft change 1", warpSHA)

		if err := f.RecordCorrespondence(warpSHA, weftSHA); err != nil {
			t.Fatalf("RecordCorrespondence() error = %v", err)
		}

		got, err := f.WeftSHAForWarpSHA(warpSHA)
		if err != nil {
			t.Fatalf("WeftSHAForWarpSHA() error = %v", err)
		}
		if got != weftSHA {
			t.Errorf("WeftSHAForWarpSHA(%q) = %q; want %q", warpSHA, got, weftSHA)
		}
	})

	// RebuildIndex, run against a weft branch carrying several hand-crafted Warp-SHA trailer commits,
	// reconstructs an index whose lookups match what recording each correspondence incrementally would
	// have produced — never having called RecordCorrespondence for these commits itself.
	t.Run("RebuildIndexReproducesTrailerHistory", func(t *testing.T) {
		warpSHA1 := fabricengine.CommitWarpForTest(t, warpPath, "warp change 2")
		weftSHA1 := commitWeftWithTrailer(t, weftFixture.PrimeRecords(), "weft change 2", warpSHA1)
		warpSHA2 := fabricengine.CommitWarpForTest(t, warpPath, "warp change 3")
		weftSHA2 := commitWeftWithTrailer(t, weftFixture.PrimeRecords(), "weft change 3", warpSHA2)

		if err := f.RebuildIndex(); err != nil {
			t.Fatalf("RebuildIndex() error = %v", err)
		}

		wantByWarpSHA := map[string]string{warpSHA1: weftSHA1, warpSHA2: weftSHA2}
		for warpSHA, wantWeftSHA := range wantByWarpSHA {
			got, err := f.WeftSHAForWarpSHA(warpSHA)
			if err != nil {
				t.Fatalf("WeftSHAForWarpSHA(%q) error = %v", warpSHA, err)
			}
			if got != wantWeftSHA {
				t.Errorf("WeftSHAForWarpSHA(%q) = %q; want %q", warpSHA, got, wantWeftSHA)
			}
		}
	})
}
