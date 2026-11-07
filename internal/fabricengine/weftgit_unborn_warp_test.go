// weftgit_unborn_warp_test.go covers CommitWeft against a warp repo with zero commits.
// That is the state of a fresh `git init`, `lym init` and `lym config`, before the operator's first warp commit.
// CommitWeft must still commit there, and leave out the Warp-SHA trailer.

package fabricengine_test

import (
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// newUnbornWarpRepo creates an isolated git repo at t.TempDir() on branch main with no commits.
// Its HEAD is unborn, the state a bare `git init` leaves before any commit lands.
func newUnbornWarpRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	gitkit.MustRun(t, dir, "git", "init", "-q", "-b", "main")
	gitkit.MustRun(t, dir, "git", "config", "user.email", "test@test.com")
	gitkit.MustRun(t, dir, "git", "config", "user.name", "Test")
	return dir
}

// TestCommitWeft_UnbornWarpHEAD_CommitsWithoutTrailerOrRecord checks CommitWeft on an unborn warp HEAD.
// The commit still happens, with no Warp-SHA trailer, since there is no warp SHA yet.
// It records no correspondence entry.
// Once warp makes its first commit, CommitWeft writes the trailer and the record again.
func TestCommitWeft_UnbornWarpHEAD_CommitsWithoutTrailerOrRecord(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	warpPath := newUnbornWarpRepo(t)
	weftFixture := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	f := fabricengine.NewFabricForTest(t, warpPath, weftFixture.PrimeRecords())

	fabricengine.WriteWeftConfigContentForTest(t, weftFixture.PrimeRecords(), "weft change, unborn warp")

	sha, committed, err := fabricengine.CommitWeftForTest(f, []string{"_lym"}, fabricengine.DefaultCommitMessage, fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("commitWeft() against an unborn warp HEAD error = %v; want nil", err)
	}
	if !committed {
		t.Fatalf("commitWeft() committed = false; want true")
	}

	rawMessage := fabricengine.CommitMessageAtForTest(t, weftFixture.PrimeRecords(), sha)
	if strings.Contains(rawMessage, fabricengine.WarpSHATrailerKey+":") {
		t.Errorf("commit message = %q; want no %s trailer (warp has no HEAD yet)", rawMessage, fabricengine.WarpSHATrailerKey)
	}

	path, err := fabricengine.CorrIndexPathForTest(f)
	if err != nil {
		t.Fatalf("corrIndexPath() error = %v", err)
	}
	if _, err := os.Stat(path); err == nil {
		ix, err := fabricengine.LoadCorrIndexForTest(path)
		if err != nil {
			t.Fatalf("loadCorrIndex() error = %v", err)
		}
		if len(fabricengine.CorrIndexEntriesForTest(ix)) != 0 {
			t.Errorf("correspondence index has %d entries after an unborn-warp commit; want 0", len(fabricengine.CorrIndexEntriesForTest(ix)))
		}
	}

	// Warp gains its first commit, so CommitWeft writes the trailer and the record again.
	warpSHA := fabricengine.CommitWarpForTest(t, warpPath, "warp's first commit")
	fabricengine.WriteWeftConfigContentForTest(t, weftFixture.PrimeRecords(), "weft change, warp now born")

	sha2, committed2, err := fabricengine.CommitWeftForTest(f, []string{"_lym"}, fabricengine.DefaultCommitMessage, fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("commitWeft() after warp's first commit error = %v; want nil", err)
	}
	if !committed2 {
		t.Fatalf("commitWeft() after warp's first commit committed = false; want true")
	}

	rawMessage2 := fabricengine.CommitMessageAtForTest(t, weftFixture.PrimeRecords(), sha2)
	wantTrailer := fabricengine.WarpSHATrailerKey + ": " + warpSHA
	if !strings.Contains(rawMessage2, wantTrailer) {
		t.Errorf("commit message after warp's first commit = %q; want it to contain %q", rawMessage2, wantTrailer)
	}

	ix, err := fabricengine.LoadCorrIndexForTest(path)
	if err != nil {
		t.Fatalf("loadCorrIndex() (post-heal) error = %v", err)
	}
	if _, ok := fabricengine.CorrIndexExactForTest(ix, warpSHA); !ok {
		t.Errorf("correspondence index has no entry for %q after the healed CommitWeft; want one", warpSHA)
	}
}
