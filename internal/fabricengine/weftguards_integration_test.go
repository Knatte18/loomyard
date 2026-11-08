// weftguards_integration_test.go pins that MergeAbort resets only the warp side.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestWeftGuards_AbortLeavesWeftCommitsDuringAttemptWindowIntact covers MergeAbort leaving the weft side alone:
// resetMergeSides resets only the warp side.
// Here the weft gains commits during the attempt window, independent of any merge in progress on the warp side.
// A per-transition status push produces that shape.
// The warp side is reset to WarpStart,
// and the weft HEAD with the commits it carries stays where MergeAbort found it.
//
//testtiming:keep weft commits made during the attempt window surviving MergeAbort at the weft HEAD it found; coverage of its blocks by other tests does not show an assertion of this
func TestWeftGuards_AbortLeavesWeftCommitsDuringAttemptWindowIntact(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, f, _, _, _, _ := newMergeLayerFixture(t, &conflictLayer)

	warpStartSHA := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())

	if _, err := f.MergeIn("feature"); err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}

	// The weft gains its own commit during the attempt window, independent of the merge in progress on the warp side.
	gitkit.CommitFile(t, h.PrimeRecords(), "weft-progress.txt", "progress\n", "weft: progress during attempt")
	weftDuringAttempt := fabricengine.CurrentSHAForTest(t, h.PrimeRecords())

	if _, err := f.MergeAbort(); err != nil {
		t.Fatalf("MergeAbort() error = %v", err)
	}

	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != warpStartSHA {
		t.Errorf("warp HEAD after MergeAbort = %q; want restored pre-merge SHA %q", got, warpStartSHA)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeRecords()); got != weftDuringAttempt {
		t.Errorf("weft HEAD after MergeAbort = %q; want unchanged %q — the weft is not a reset target", got, weftDuringAttempt)
	}
	if _, err := os.Stat(filepath.Join(h.PrimeRecords(), "weft-progress.txt")); err != nil {
		t.Errorf("Stat(weft-progress.txt) after MergeAbort = %v; want present — the weft commit made during the attempt window must survive intact", err)
	}
}
