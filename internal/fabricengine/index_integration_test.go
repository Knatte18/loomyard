// index_integration_test.go — integration tests for the fabric layer's git wiring around the correspondence index.
// They cover gitdir resolution and the RecordCorrespondence/WeftSHAForWarpSHA round trip.
// They also cover the RebuildIndex trailer scan.
// The tests are in package fabricengine_test and reach weftGitDir through export_test.go's WeftGitDirForTest shim.
// The hub weft comes from hubforge.NewHub; the warp side is a minimal plain git repo built locally.
// So the warp side needs none of the real hub's junction and portal wiring.

package fabricengine_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// commitWeftWithTrailer commits content into weftPath's tracked _lym config file with a Warp-SHA trailer naming warpSHA.
// It writes the commit directly through gitkit, not through the production commit path.
// It returns the new weft HEAD SHA.
func commitWeftWithTrailer(t *testing.T, weftPath, content, warpSHA string) string {
	t.Helper()

	msg := fabricengine.AppendWarpSHATrailerForTest("weft sync", warpSHA)
	return gitkit.CommitFile(t, weftPath, filepath.Join("_lym", "config.yaml"), content, msg)
}

// TestCorrespondenceIndex runs the correspondence index's git wiring over one plain warp repo and one hub weft.
// It covers gitdir resolution, the miss path and the RecordCorrespondence/WeftSHAForWarpSHA round trip.
// It also covers the RebuildIndex trailer scan.
// The steps share the fixture and run in order.
// Each commits its own warp and weft commits, so none reads an earlier step's entries.
// The rebuild step's lookups succeed only through the rebuild, never through an earlier step's RecordCorrespondence.
func TestCorrespondenceIndex(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	weftFixture := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	f := fabricengine.NewFabricForTest(t, warpPath, weftFixture.PrimeRecords())

	// weftGitDir resolves to a path inside the weft worktree's own .git directory:
	// the per-worktree gitdir the correspondence index is scoped to.
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

	// A RecordCorrespondence call is visible to a later WeftSHAForWarpSHA lookup.
	// WarpSeq is computed from the warp repo's first-parent commit count.
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

	// RebuildIndex, run against a weft branch with several hand-crafted Warp-SHA trailer commits, reconstructs an index.
	// Its lookups match what recording each correspondence incrementally would have produced,
	// and the test never calls RecordCorrespondence for these commits itself.
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
