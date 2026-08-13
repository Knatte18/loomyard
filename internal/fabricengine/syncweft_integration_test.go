//go:build integration

// syncweft_integration_test.go — integration coverage for the parts of the
// weft-git surface that genuinely need git spawned: the Warp-SHA trailer
// landing in a real commit, RebuildIndex reconstructing what an incremental
// build produces, the detached-commit path's self-healing lookup, and
// staleness surviving a rebuild. Package fabricengine (internal) because it
// needs the unexported index plumbing (corrIndexPath, loadCorrIndex,
// weftGitDir) alongside the exported Fabric surface. Reuses
// index_integration_test.go's fixture helpers (newPlainWarpRepo, commitWarp,
// commitWeftWithTrailer, currentSHA, newFabric) and
// commit_integration_test.go's seedFabricConfig rather than redefining them,
// since all these files share this package.

package fabricengine

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
)

// TestRebuildIndex_EqualsIncrementallyBuiltIndex asserts that after several Commit rounds (each
// incrementally updating the index via RecordCorrespondence), a full RebuildIndex reconstructs an
// index whose entries are identical to the incrementally-built one.
func TestRebuildIndex_EqualsIncrementallyBuiltIndex(t *testing.T) {
	warpPath := newPlainWarpRepo(t)
	weftFixture := gitkit.CopyWeft(t)
	seedFabricConfig(t, warpPath)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	for i := 0; i < 3; i++ {
		commitWarp(t, warpPath, fmt.Sprintf("warp round %d", i))
		writeWeftConfigContent(t, weftFixture.WeftPath, fmt.Sprintf("weft round %d", i))
		res, err := f.Commit([]string{"_lyx"}, DefaultCommitMessage, nil, SyncOptions{})
		if err != nil {
			t.Fatalf("Commit() round %d error = %v", i, err)
		}
		if !res.WeftCommitted {
			t.Fatalf("Commit() round %d WeftCommitted = false; want true", i)
		}
	}

	path, err := f.corrIndexPath()
	if err != nil {
		t.Fatalf("corrIndexPath() error = %v", err)
	}
	incremental, err := loadCorrIndex(path)
	if err != nil {
		t.Fatalf("loadCorrIndex() error = %v", err)
	}
	wantEntries := incremental.entries()
	if len(wantEntries) != 3 {
		t.Fatalf("incrementally-built index has %d entries; want 3", len(wantEntries))
	}

	if err := f.RebuildIndex(); err != nil {
		t.Fatalf("RebuildIndex() error = %v", err)
	}
	rebuilt, err := loadCorrIndex(path)
	if err != nil {
		t.Fatalf("loadCorrIndex() (post-rebuild) error = %v", err)
	}
	gotEntries := rebuilt.entries()

	if len(gotEntries) != len(wantEntries) {
		t.Fatalf("RebuildIndex() entries = %d; want %d", len(gotEntries), len(wantEntries))
	}
	for i := range wantEntries {
		if gotEntries[i] != wantEntries[i] {
			t.Errorf("entries[%d]: RebuildIndex()=%+v incremental=%+v; want equal", i, gotEntries[i], wantEntries[i])
		}
	}
}

// expireAndPruneUnreachable forces git to actually forget any commit
// objects that are no longer reachable from a ref, in repoPath. A plain
// amend/reset alone leaves the superseded object resolvable via the reflog
// for a while — gitrepo/doc.go's own documented SHAExists caveat — so tests
// that need SHAExists to genuinely report false on a rewritten-away SHA
// must call this first.
func expireAndPruneUnreachable(t *testing.T, repoPath string) {
	t.Helper()

	gitkit.MustRun(t, repoPath, "git", "reflog", "expire", "--expire=now", "--all")
	gitkit.MustRun(t, repoPath, "git", "gc", "--prune=now", "-q")
}

// TestWeftSHAForWarpSHA_DetachedPathSelfCorrection covers the CLI detached path's self-correction:
// CommitWeft records a pre-push SHA;
// the commit is then amended in place (same trailer, new SHA) and the old SHA is forced to
// genuinely stop resolving via expireAndPruneUnreachable.
// WeftSHAForWarpSHA must heal via one RebuildIndex retry to the surviving (amended) trailer commit.
func TestWeftSHAForWarpSHA_DetachedPathSelfCorrection(t *testing.T) {
	warpPath := newPlainWarpRepo(t)
	weftFixture := gitkit.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	warpSHA := commitWarp(t, warpPath, "warp change")
	writeWeftConfigContent(t, weftFixture.WeftPath, "weft change")

	preAmendSHA, committed, err := f.commitWeft([]string{"_lyx"}, DefaultCommitMessage, SyncOptions{})
	if err != nil {
		t.Fatalf("commitWeft() error = %v", err)
	}
	if !committed {
		t.Fatalf("commitWeft() committed = false; want true")
	}

	got, err := f.WeftSHAForWarpSHA(warpSHA)
	if err != nil {
		t.Fatalf("WeftSHAForWarpSHA() (pre-amend) error = %v", err)
	}
	if got != preAmendSHA {
		t.Fatalf("WeftSHAForWarpSHA() (pre-amend) = %q; want %q", got, preAmendSHA)
	}

	// Change the tree before amending — a same-second, same-tree amend can
	// legitimately land on the identical SHA (git's SHA covers tree,
	// parent, author, committer, and message; two amends within the same
	// second can otherwise tie on every field), so this guarantees a
	// genuinely new commit object while --no-edit preserves the trailer.
	writeWeftConfigContent(t, weftFixture.WeftPath, "weft change, amended")
	gitkit.MustRun(t, weftFixture.WeftPath, "git", "add", "-A")
	gitkit.MustRun(t, weftFixture.WeftPath, "git", "commit", "--amend", "--no-edit")
	postAmendSHA := currentSHA(t, weftFixture.WeftPath)
	if postAmendSHA == preAmendSHA {
		t.Fatalf("amend did not change the weft SHA")
	}
	expireAndPruneUnreachable(t, weftFixture.WeftPath)

	got, err = f.WeftSHAForWarpSHA(warpSHA)
	if err != nil {
		t.Fatalf("WeftSHAForWarpSHA() (post-amend) error = %v", err)
	}
	if got != postAmendSHA {
		t.Errorf("WeftSHAForWarpSHA() (post-amend) = %q; want the surviving trailer commit %q", got, postAmendSHA)
	}
}

// staleCorrespondenceFixture builds a fresh warp/weft pair with exactly one
// recorded correspondence, then rewrites weft's history so that
// correspondence is stale beyond what RebuildIndex can recover: the trailer
// commit is reset away and the orphaned object is genuinely pruned (not
// just amended in place), so a full trailer rescan finds nothing naming
// warpSHA anywhere. Returns the Fabric and the now-stale warpSHA.
func staleCorrespondenceFixture(t *testing.T) (f *Fabric, warpSHA string) {
	t.Helper()

	warpPath := newPlainWarpRepo(t)
	weftFixture := gitkit.CopyWeft(t)
	f = newFabric(t, warpPath, weftFixture.WeftPath)

	baseWeftSHA := currentSHA(t, weftFixture.WeftPath)

	warpSHA = commitWarp(t, warpPath, "warp change")
	writeWeftConfigContent(t, weftFixture.WeftPath, "weft change")
	weftSHA, committed, err := f.commitWeft([]string{"_lyx"}, DefaultCommitMessage, SyncOptions{})
	if err != nil {
		t.Fatalf("commitWeft() error = %v", err)
	}
	if !committed {
		t.Fatalf("commitWeft() committed = false; want true")
	}
	if got, err := f.WeftSHAForWarpSHA(warpSHA); err != nil || got != weftSHA {
		t.Fatalf("WeftSHAForWarpSHA() (pre-rewrite) = %q, %v; want %q, nil", got, err, weftSHA)
	}

	// Discard the trailer commit from history entirely, then force git to
	// genuinely forget the orphaned object — RebuildIndex's scan will find
	// no trailer naming warpSHA anywhere afterwards.
	gitkit.MustRun(t, weftFixture.WeftPath, "git", "reset", "--hard", baseWeftSHA)
	expireAndPruneUnreachable(t, weftFixture.WeftPath)

	return f, warpSHA
}

// TestWeftSHAForWarpSHA_StalenessSurvivesRebuild covers a history rewrite RebuildIndex cannot
// recover from: WeftSHAForWarpSHA must surface wrapped ErrStaleSHA (errors.Is).
func TestWeftSHAForWarpSHA_StalenessSurvivesRebuild(t *testing.T) {
	f, warpSHA := staleCorrespondenceFixture(t)

	if _, err := f.WeftSHAForWarpSHA(warpSHA); !errors.Is(err, ErrStaleSHA) {
		t.Errorf("WeftSHAForWarpSHA() error = %v; want errors.Is(err, ErrStaleSHA)", err)
	}
}
