//go:build integration

// weftgit_unborn_warp_test.go — integration coverage for the O4 fix: a warp
// repo with zero commits (a fresh `git init` -> `lyx init` -> `lyx config`
// first-run path, before the operator's first warp commit) must not fail
// every weft commit just because CommitWeft now reads warp HEAD for the
// Warp-SHA trailer. Package fabricengine_test, reusing
// export_test.go's NewFabricForTest/CurrentSHAForTest/CommitWarpForTest
// shims for the fixture helpers this file used to share directly with
// index_integration_test.go, back when both lived in package fabricengine.

package fabricengine_test

import (
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// newUnbornWarpRepo creates a minimal, isolated git repo at t.TempDir() on
// branch main with NO commits — an unborn HEAD, the state a bare `git init`
// leaves before any commit lands. Distinct from
// fabricengine.NewPlainWarpRepoForTest (which commits once), since this
// file's tests need the pre-first-commit state itself.
func newUnbornWarpRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	gitkit.MustRun(t, dir, "git", "init", "-q", "-b", "main")
	gitkit.MustRun(t, dir, "git", "config", "user.email", "test@test.com")
	gitkit.MustRun(t, dir, "git", "config", "user.name", "Test")
	return dir
}

// TestCommitWeft_UnbornWarpHEAD_CommitsWithoutTrailerOrRecord asserts that CommitWeft against an
// unborn warp HEAD still commits (unlike the pre-fix behavior, which failed with "warp CurrentSHA:
// gitrepo: repository has no commits" before ever reaching StageAndCommit), that the resulting
// commit carries no Warp-SHA trailer (there is no warp SHA yet to name), and that no correspondence
// entry was recorded for it.
// It then makes warp's first commit and calls CommitWeft again, asserting normal trailer/record
// behavior resumes — the unborn state is a one-time, self-healing condition, not a permanent mode.
func TestCommitWeft_UnbornWarpHEAD_CommitsWithoutTrailerOrRecord(t *testing.T) {
	warpPath := newUnbornWarpRepo(t)
	weftFixture := hubforge.NewHub(t, ".")
	f := fabricengine.NewFabricForTest(t, warpPath, weftFixture.PrimeWeft())

	fabricengine.WriteWeftConfigContentForTest(t, weftFixture.PrimeWeft(), "weft change, unborn warp")

	sha, committed, err := fabricengine.CommitWeftForTest(f, []string{"_lyx"}, fabricengine.DefaultCommitMessage, fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("commitWeft() against an unborn warp HEAD error = %v; want nil", err)
	}
	if !committed {
		t.Fatalf("commitWeft() committed = false; want true")
	}

	rawMessage := fabricengine.CommitMessageAtForTest(t, weftFixture.PrimeWeft(), sha)
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

	// Warp gains its first commit; a subsequent CommitWeft must resume
	// normal trailer/record behavior — the unborn condition self-heals.
	warpSHA := fabricengine.CommitWarpForTest(t, warpPath, "warp's first commit")
	fabricengine.WriteWeftConfigContentForTest(t, weftFixture.PrimeWeft(), "weft change, warp now born")

	sha2, committed2, err := fabricengine.CommitWeftForTest(f, []string{"_lyx"}, fabricengine.DefaultCommitMessage, fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("commitWeft() after warp's first commit error = %v; want nil", err)
	}
	if !committed2 {
		t.Fatalf("commitWeft() after warp's first commit committed = false; want true")
	}

	rawMessage2 := fabricengine.CommitMessageAtForTest(t, weftFixture.PrimeWeft(), sha2)
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
