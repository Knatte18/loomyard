// syncweft_integration_test.go covers the weft-git paths that need git spawned.
// It checks that the Warp-SHA trailer lands in a real commit.
// It checks that RebuildIndex reconstructs the index an incremental build produces.
// It checks the detached-commit path's self-healing lookup, and that staleness survives a rebuild.
// The package is fabricengine_test; the unexported index plumbing is reached via export_test.go's ForTest shims.

package fabricengine_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRebuildIndex_EqualsIncrementallyBuiltIndex checks that a full RebuildIndex matches the incremental index.
// The incremental index comes from several Commit rounds, each calling RecordCorrespondence.
// The rebuilt entries must be identical to the incremental ones.
func TestRebuildIndex_EqualsIncrementallyBuiltIndex(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	weftFixture := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	fabricengine.SeedFabricConfigForTest(t, warpPath)
	f := fabricengine.NewFabricForTest(t, warpPath, weftFixture.PrimeRecords())

	for i := 0; i < 3; i++ {
		fabricengine.CommitWarpForTest(t, warpPath, fmt.Sprintf("warp round %d", i))
		fabricengine.WriteWeftConfigContentForTest(t, weftFixture.PrimeRecords(), fmt.Sprintf("weft round %d", i))
		res, err := f.Commit([]string{"_lym"}, fabricengine.DefaultCommitMessage, nil, fabricengine.SyncOptions{})
		if err != nil {
			t.Fatalf("Commit() round %d error = %v", i, err)
		}
		if !res.WeftCommitted {
			t.Fatalf("Commit() round %d WeftCommitted = false; want true", i)
		}
	}

	path, err := fabricengine.CorrIndexPathForTest(f)
	if err != nil {
		t.Fatalf("corrIndexPath() error = %v", err)
	}
	incremental, err := fabricengine.LoadCorrIndexForTest(path)
	if err != nil {
		t.Fatalf("loadCorrIndex() error = %v", err)
	}
	wantEntries := fabricengine.CorrIndexEntriesForTest(incremental)
	if len(wantEntries) != 3 {
		t.Fatalf("incrementally-built index has %d entries; want 3", len(wantEntries))
	}

	if err := f.RebuildIndex(); err != nil {
		t.Fatalf("RebuildIndex() error = %v", err)
	}
	rebuilt, err := fabricengine.LoadCorrIndexForTest(path)
	if err != nil {
		t.Fatalf("loadCorrIndex() (post-rebuild) error = %v", err)
	}
	gotEntries := fabricengine.CorrIndexEntriesForTest(rebuilt)

	if len(gotEntries) != len(wantEntries) {
		t.Fatalf("RebuildIndex() entries = %d; want %d", len(gotEntries), len(wantEntries))
	}
	for i := range wantEntries {
		if gotEntries[i] != wantEntries[i] {
			t.Errorf("entries[%d]: RebuildIndex()=%+v incremental=%+v; want equal", i, gotEntries[i], wantEntries[i])
		}
	}
}

// expireAndPruneUnreachable makes git forget every commit object no longer reachable from a ref in repoPath.
// An amend or reset alone leaves the superseded object resolvable through the reflog.
// SHAExists may still report it as present until this runs.
// Tests that need SHAExists to report false for a rewritten-away SHA call this first.
func expireAndPruneUnreachable(t *testing.T, repoPath string) {
	t.Helper()

	gitkit.MustRun(t, repoPath, "git", "reflog", "expire", "--expire=now", "--all")
	gitkit.MustRun(t, repoPath, "git", "gc", "--prune=now", "-q")
}

// TestWeftSHAForWarpSHA_DetachedPathSelfCorrection covers the CLI detached path's self-correction.
// CommitWeft records a pre-push SHA, then the commit is amended in place: same trailer, new SHA.
// expireAndPruneUnreachable then makes the old SHA stop resolving.
// WeftSHAForWarpSHA must heal with one RebuildIndex retry to the surviving amended trailer commit.
func TestWeftSHAForWarpSHA_DetachedPathSelfCorrection(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	weftFixture := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	f := fabricengine.NewFabricForTest(t, warpPath, weftFixture.PrimeRecords())

	warpSHA := fabricengine.CommitWarpForTest(t, warpPath, "warp change")
	fabricengine.WriteWeftConfigContentForTest(t, weftFixture.PrimeRecords(), "weft change")

	preAmendSHA, committed, err := fabricengine.CommitWeftForTest(f, []string{"_lym"}, fabricengine.DefaultCommitMessage, fabricengine.SyncOptions{})
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

	// Change the tree before amending.
	// Git's SHA covers the tree, parent, author, committer, and message,
	// so an amend within the same second with an unchanged tree can land on the identical SHA.
	// Changing the tree guarantees a new commit object, and --no-edit keeps the trailer.
	fabricengine.WriteWeftConfigContentForTest(t, weftFixture.PrimeRecords(), "weft change, amended")
	gitkit.MustRun(t, weftFixture.PrimeRecords(), "git", "add", "-A")
	gitkit.MustRun(t, weftFixture.PrimeRecords(), "git", "commit", "--amend", "--no-edit")
	postAmendSHA := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeRecords())
	if postAmendSHA == preAmendSHA {
		t.Fatalf("amend did not change the weft SHA")
	}
	expireAndPruneUnreachable(t, weftFixture.PrimeRecords())

	got, err = f.WeftSHAForWarpSHA(warpSHA)
	if err != nil {
		t.Fatalf("WeftSHAForWarpSHA() (post-amend) error = %v", err)
	}
	if got != postAmendSHA {
		t.Errorf("WeftSHAForWarpSHA() (post-amend) = %q; want the surviving trailer commit %q", got, postAmendSHA)
	}
}

// staleCorrespondenceFixture builds a warp/weft pair with one recorded correspondence, then makes it stale.
// The history is rewritten so that RebuildIndex cannot recover the correspondence.
// The trailer commit is reset away and the orphaned object pruned, so no trailer names warpSHA anywhere.
// It returns the Fabric and the now-stale warpSHA.
func staleCorrespondenceFixture(t *testing.T) (f *fabricengine.Fabric, warpSHA string) {
	t.Helper()

	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	weftFixture := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	f = fabricengine.NewFabricForTest(t, warpPath, weftFixture.PrimeRecords())

	baseWeftSHA := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeRecords())

	warpSHA = fabricengine.CommitWarpForTest(t, warpPath, "warp change")
	fabricengine.WriteWeftConfigContentForTest(t, weftFixture.PrimeRecords(), "weft change")
	weftSHA, committed, err := fabricengine.CommitWeftForTest(f, []string{"_lym"}, fabricengine.DefaultCommitMessage, fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("commitWeft() error = %v", err)
	}
	if !committed {
		t.Fatalf("commitWeft() committed = false; want true")
	}
	if got, err := f.WeftSHAForWarpSHA(warpSHA); err != nil || got != weftSHA {
		t.Fatalf("WeftSHAForWarpSHA() (pre-rewrite) = %q, %v; want %q, nil", got, err, weftSHA)
	}

	gitkit.MustRun(t, weftFixture.PrimeRecords(), "git", "reset", "--hard", baseWeftSHA)
	expireAndPruneUnreachable(t, weftFixture.PrimeRecords())

	return f, warpSHA
}

// TestWeftSHAForWarpSHA_StalenessSurvivesRebuild covers a history rewrite that RebuildIndex cannot recover from.
// WeftSHAForWarpSHA must return an error wrapping ErrStaleSHA, so that errors.Is matches it.
func TestWeftSHAForWarpSHA_StalenessSurvivesRebuild(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	f, warpSHA := staleCorrespondenceFixture(t)

	if _, err := f.WeftSHAForWarpSHA(warpSHA); !errors.Is(err, fabricengine.ErrStaleSHA) {
		t.Errorf("WeftSHAForWarpSHA() error = %v; want errors.Is(err, ErrStaleSHA)", err)
	}
}
