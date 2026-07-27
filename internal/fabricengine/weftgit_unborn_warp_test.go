//go:build integration

// weftgit_unborn_warp_test.go — integration coverage for the O4 fix: a warp
// repo with zero commits (a fresh `git init` -> `lyx init` -> `lyx config`
// first-run path, before the operator's first host commit) must not fail
// every weft commit just because CommitWeft now reads warp HEAD for the
// Warp-SHA trailer. Package fabricengine (internal), reusing
// index_integration_test.go's newFabric/currentSHA/commitWarp helpers, which
// this file shares the package with.

package fabricengine

import (
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// newUnbornWarpRepo creates a minimal, isolated git repo at t.TempDir() on
// branch main with NO commits — an unborn HEAD, the state a bare `git init`
// leaves before any commit lands. Distinct from newPlainWarpRepo (which
// commits once), since these tests need the pre-first-commit state itself.
func newUnbornWarpRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	lyxtest.MustRun(t, dir, "git", "init", "-q", "-b", "main")
	lyxtest.MustRun(t, dir, "git", "config", "user.email", "test@test.com")
	lyxtest.MustRun(t, dir, "git", "config", "user.name", "Test")
	return dir
}

// TestCommitWeft_UnbornWarpHEAD_CommitsWithoutTrailerOrRecord asserts that
// CommitWeft against an unborn warp HEAD still commits (unlike the pre-fix
// behavior, which failed with "warp CurrentSHA: gitrepo: repository has no
// commits" before ever reaching StageAndCommit), that the resulting commit
// carries no Warp-SHA trailer (there is no warp SHA yet to name), and that
// no correspondence entry was recorded for it. It then makes warp's first
// commit and calls CommitWeft again, asserting normal trailer/record
// behavior resumes — the unborn state is a one-time, self-healing condition,
// not a permanent mode.
func TestCommitWeft_UnbornWarpHEAD_CommitsWithoutTrailerOrRecord(t *testing.T) {
	warpPath := newUnbornWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	writeWeftConfigContent(t, weftFixture.WeftPath, "weft change, unborn warp")

	sha, committed, err := f.CommitWeft([]string{"_lyx"}, DefaultCommitMessage, SyncOptions{})
	if err != nil {
		t.Fatalf("CommitWeft() against an unborn warp HEAD error = %v; want nil", err)
	}
	if !committed {
		t.Fatalf("CommitWeft() committed = false; want true")
	}

	rawMessage := commitMessageAt(t, weftFixture.WeftPath, sha)
	if strings.Contains(rawMessage, WarpSHATrailerKey+":") {
		t.Errorf("commit message = %q; want no %s trailer (warp has no HEAD yet)", rawMessage, WarpSHATrailerKey)
	}

	path, err := f.corrIndexPath()
	if err != nil {
		t.Fatalf("corrIndexPath() error = %v", err)
	}
	if _, err := os.Stat(path); err == nil {
		ix, err := loadCorrIndex(path)
		if err != nil {
			t.Fatalf("loadCorrIndex() error = %v", err)
		}
		if len(ix.entries()) != 0 {
			t.Errorf("correspondence index has %d entries after an unborn-warp commit; want 0", len(ix.entries()))
		}
	}

	// Warp gains its first commit; a subsequent CommitWeft must resume
	// normal trailer/record behavior — the unborn condition self-heals.
	warpSHA := commitWarp(t, warpPath, "warp's first commit")
	writeWeftConfigContent(t, weftFixture.WeftPath, "weft change, warp now born")

	sha2, committed2, err := f.CommitWeft([]string{"_lyx"}, DefaultCommitMessage, SyncOptions{})
	if err != nil {
		t.Fatalf("CommitWeft() after warp's first commit error = %v; want nil", err)
	}
	if !committed2 {
		t.Fatalf("CommitWeft() after warp's first commit committed = false; want true")
	}

	rawMessage2 := commitMessageAt(t, weftFixture.WeftPath, sha2)
	wantTrailer := WarpSHATrailerKey + ": " + warpSHA
	if !strings.Contains(rawMessage2, wantTrailer) {
		t.Errorf("commit message after warp's first commit = %q; want it to contain %q", rawMessage2, wantTrailer)
	}

	ix, err := loadCorrIndex(path)
	if err != nil {
		t.Fatalf("loadCorrIndex() (post-heal) error = %v", err)
	}
	if _, ok := ix.exact(warpSHA); !ok {
		t.Errorf("correspondence index has no entry for %q after the healed CommitWeft; want one", warpSHA)
	}
}

// TestSyncWeft_UnbornWarpHEAD_SkipPushAndPush covers SyncWeft's two branches
// against an unborn warp HEAD: the SkipPush (or SkipGit) early return and the
// real-push path. Both must report Committed=true with WarpSHA left empty
// (there is no warp SHA to report), never error.
func TestSyncWeft_UnbornWarpHEAD_SkipPushAndPush(t *testing.T) {
	t.Run("SkipPush", func(t *testing.T) {
		warpPath := newUnbornWarpRepo(t)
		weftFixture := lyxtest.CopyWeft(t)
		f := newFabric(t, warpPath, weftFixture.WeftPath)

		writeWeftConfigContent(t, weftFixture.WeftPath, "weft change, unborn warp, skip push")

		res, err := f.SyncWeft(DefaultCommitMessage, []string{"_lyx"}, SyncOptions{SkipPush: true})
		if err != nil {
			t.Fatalf("SyncWeft(SkipPush) against an unborn warp HEAD error = %v; want nil", err)
		}
		if !res.Committed || res.Pushed {
			t.Fatalf("SyncWeft(SkipPush) = %+v; want Committed=true Pushed=false", res)
		}
		if res.WarpSHA != "" {
			t.Errorf("SyncWeft(SkipPush) WarpSHA = %q; want empty (warp has no HEAD yet)", res.WarpSHA)
		}
	})

	t.Run("RealPush", func(t *testing.T) {
		warpPath := newUnbornWarpRepo(t)
		weftFixture := lyxtest.CopyWeft(t)
		f := newFabric(t, warpPath, weftFixture.WeftPath)

		writeWeftConfigContent(t, weftFixture.WeftPath, "weft change, unborn warp, real push")

		res, err := f.SyncWeft(DefaultCommitMessage, []string{"_lyx"}, SyncOptions{})
		if err != nil {
			t.Fatalf("SyncWeft() against an unborn warp HEAD error = %v; want nil", err)
		}
		if !res.Committed || !res.Pushed {
			t.Fatalf("SyncWeft() = %+v; want Committed=true Pushed=true", res)
		}
		if res.WarpSHA != "" {
			t.Errorf("SyncWeft() WarpSHA = %q; want empty (warp has no HEAD yet)", res.WarpSHA)
		}
		if !f.Weft.SHAExists(res.WeftSHA) {
			t.Errorf("SHAExists(%q) = false; want true (it is HEAD)", res.WeftSHA)
		}
	})
}
