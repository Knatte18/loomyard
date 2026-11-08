// commit_integration_test.go covers Fabric.Commit's two-sided commit.
// It tests warp-first ordering and CommitResult fields;
// it also covers the spawnDetachedPushFn push seam, the empty-commit rule, and CommitWeft's inherited contract.
// It is package fabricengine_test and reaches the unexported seams through export_test.go's ForTest shims.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCommit_TwoSided_WarpFirstOrdering asserts that a two-sided Fabric.Commit's weft commit
// carries a Warp-SHA trailer naming the warp commit that Fabric.Commit just made, never the prior warp HEAD.
func TestCommit_TwoSided_WarpFirstOrdering(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: SwapPushRecorderForTest sets the package-level spawnDetachedPushFn.
	f, warpPath, weftPath := fabricengine.NewCommitFixtureForTest(t)
	fabricengine.SwapPushRecorderForTest(t)

	fabricengine.WriteWarpFileForTest(t, warpPath, "README", "warp change")
	fabricengine.WriteWeftConfigContentForTest(t, weftPath, "weft change")

	result, err := f.Commit([]string{"README", "_lym/config.yaml"}, "two-sided commit", nil, fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if !result.WarpCommitted || !result.WeftCommitted {
		t.Fatalf("Commit() = %+v; want both sides committed", result)
	}

	trailerSHA, ok := fabricengine.ParseWarpSHATrailerForTest(fabricengine.CommitMessageAtForTest(t, weftPath, result.WeftSHA))
	if !ok {
		t.Fatalf("weft commit %s carries no Warp-SHA trailer", result.WeftSHA)
	}
	if trailerSHA != result.WarpSHA {
		t.Errorf("weft commit's Warp-SHA trailer = %q; want the warp commit Fabric.Commit just created %q", trailerSHA, result.WarpSHA)
	}
}

// TestCommit_ResultFields asserts fabricengine.CommitResult fields for two-sided, warp-only and weft-only commits.
// On the warp-only path it also checks the plain-git property: no trailer and no correspondence entry.
func TestCommit_ResultFields(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: SwapPushRecorderForTest sets the package-level spawnDetachedPushFn.
	t.Run("TwoSided", func(t *testing.T) {
		f, warpPath, weftPath := fabricengine.NewCommitFixtureForTest(t)
		fabricengine.SwapPushRecorderForTest(t)

		fabricengine.WriteWarpFileForTest(t, warpPath, "README", "warp change")
		fabricengine.WriteWeftConfigContentForTest(t, weftPath, "weft change")

		result, err := f.Commit([]string{"README", "_lym/config.yaml"}, "two-sided commit", nil, fabricengine.SyncOptions{})
		if err != nil {
			t.Fatalf("Commit() error = %v", err)
		}
		if result.WarpSHA == "" || !result.WarpCommitted {
			t.Errorf("Commit() warp side = %+v; want a populated WarpSHA and WarpCommitted=true", result)
		}
		if result.WeftSHA == "" || !result.WeftCommitted {
			t.Errorf("Commit() weft side = %+v; want a populated WeftSHA and WeftCommitted=true", result)
		}
	})

	t.Run("WarpOnly", func(t *testing.T) {
		f, warpPath, _ := fabricengine.NewCommitFixtureForTest(t)
		fabricengine.SwapPushRecorderForTest(t)

		fabricengine.WriteWarpFileForTest(t, warpPath, "README", "warp only change")

		result, err := f.Commit([]string{"README"}, "warp only commit", nil, fabricengine.SyncOptions{})
		if err != nil {
			t.Fatalf("Commit() error = %v", err)
		}
		if result.WarpSHA == "" || !result.WarpCommitted {
			t.Errorf("Commit() warp side = %+v; want a populated WarpSHA and WarpCommitted=true", result)
		}
		if result.WeftSHA != "" || result.WeftCommitted {
			t.Errorf("Commit() weft side = %+v; want WeftSHA empty and WeftCommitted=false", result)
		}

		// Plain-git property: no trailer, no correspondence entry.
		if _, ok := fabricengine.ParseWarpSHATrailerForTest(fabricengine.CommitMessageAtForTest(t, warpPath, result.WarpSHA)); ok {
			t.Errorf("warp-only commit %s carries a Warp-SHA trailer; want none", result.WarpSHA)
		}
		if _, err := f.WeftSHAForWarpSHA(result.WarpSHA); err == nil {
			t.Errorf("WeftSHAForWarpSHA(%q) succeeded; want ErrNoCorrespondence (a warp-only commit records no correspondence)", result.WarpSHA)
		}
	})

	t.Run("WeftOnly", func(t *testing.T) {
		f, _, weftPath := fabricengine.NewCommitFixtureForTest(t)
		fabricengine.SwapPushRecorderForTest(t)

		fabricengine.WriteWeftConfigContentForTest(t, weftPath, "weft only change")

		result, err := f.Commit([]string{"_lym/config.yaml"}, "weft only commit", nil, fabricengine.SyncOptions{})
		if err != nil {
			t.Fatalf("Commit() error = %v", err)
		}
		if result.WarpSHA != "" || result.WarpCommitted {
			t.Errorf("Commit() warp side = %+v; want WarpSHA empty and WarpCommitted=false", result)
		}
		if result.WeftSHA == "" || !result.WeftCommitted {
			t.Errorf("Commit() weft side = %+v; want a populated WeftSHA and WeftCommitted=true", result)
		}
	})
}

// TestCommit_InvokesPushRecorder asserts that a successful two-sided Fabric.Commit calls spawnDetachedPushFn once.
// Its subtests check the push request, and that an unresolvable prime spawns no push and lands no commit.
func TestCommit_InvokesPushRecorder(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: SwapPushRecorderForTest sets the package-level spawnDetachedPushFn.
	t.Run("PrimePushesRecordsSideOnly", func(t *testing.T) {
		// The plain fixture is a single repo, so its warp worktree is the prime.
		f, warpPath, weftPath := fabricengine.NewCommitFixtureForTest(t)
		recorder := fabricengine.SwapPushRecorderForTest(t)

		fabricengine.WriteWarpFileForTest(t, warpPath, "README", "warp change")
		fabricengine.WriteWeftConfigContentForTest(t, weftPath, "weft change")

		if _, err := f.Commit([]string{"README", "_lym/config.yaml"}, "two-sided commit", nil, fabricengine.SyncOptions{}); err != nil {
			t.Fatalf("Commit() error = %v", err)
		}

		calls := recorder.Calls()
		if len(calls) != 1 {
			t.Fatalf("push recorder invocation count = %d; want 1 (calls: %+v)", len(calls), calls)
		}
		// The push request names the records worktree of the warp location, not the weft repo at weftPath.
		warpLocation, err := lymcwd.ResolveWorktree(warpPath)
		if err != nil {
			t.Fatalf("ResolveWorktree: %v", err)
		}
		wantWeft := fabricengine.RecordsWorktree(warpLocation)
		if calls[0].WarpPath != "" || calls[0].WeftPath != wantWeft {
			t.Errorf("push recorder called with (%q, %q); want (%q, %q)", calls[0].WarpPath, calls[0].WeftPath, "", wantWeft)
		}
	})

	t.Run("TaskPairPushesBothSides", func(t *testing.T) {
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{hubforge.TemplatePairSlug}})
		pairLocation, err := lymcwd.ResolveWorktree(h.PairCodeWorktree(hubforge.TemplatePairSlug))
		if err != nil {
			t.Fatalf("ResolveWorktree: %v", err)
		}
		f, err := fabricengine.Open(pairLocation)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		recorder := fabricengine.SwapPushRecorderForTest(t)

		fabricengine.WriteWarpFileForTest(t, h.PairCodeWorktree(hubforge.TemplatePairSlug), "pair-file.txt", "warp change")

		if _, err := f.Commit([]string{"pair-file.txt"}, "pair commit", nil, fabricengine.SyncOptions{}); err != nil {
			t.Fatalf("Commit() error = %v", err)
		}

		calls := recorder.Calls()
		if len(calls) != 1 {
			t.Fatalf("push recorder invocation count = %d; want 1 (calls: %+v)", len(calls), calls)
		}
		if calls[0].WarpPath != h.PairCodeWorktree(hubforge.TemplatePairSlug) || calls[0].WeftPath != h.PairRecordsSibling(hubforge.TemplatePairSlug) {
			t.Errorf("push recorder called with (%q, %q); want (%q, %q)", calls[0].WarpPath, calls[0].WeftPath, h.PairCodeWorktree(hubforge.TemplatePairSlug), h.PairRecordsSibling(hubforge.TemplatePairSlug))
		}
	})

	t.Run("UnresolvablePrimeFailsBeforeCommitting", func(t *testing.T) {
		f, warpPath, _ := fabricengine.NewCommitFixtureForTest(t)
		recorder := fabricengine.SwapPushRecorderForTest(t)
		// An anchor naming a subdirectory absent from the worktree makes the worktree list read fail in the anchor path.
		writeFabricAnchor(t, warpPath, "absent/subdir")
		headBefore := gitkit.RevParse(t, warpPath, "HEAD")
		fabricengine.WriteWarpFileForTest(t, warpPath, "README", "warp change")

		result, err := f.Commit([]string{"README"}, "unresolvable prime", nil, fabricengine.SyncOptions{})

		if err == nil || !strings.Contains(err.Error(), "resolve main worktree") {
			t.Fatalf("Commit() error = %v; want one carrying %q", err, "resolve main worktree")
		}
		if result.Committed() || gitkit.RevParse(t, warpPath, "HEAD") != headBefore {
			t.Errorf("Commit() landed a commit despite the error: %+v", result)
		}
		if len(recorder.Calls()) != 0 {
			t.Errorf("push recorder invoked %d times; want none", len(recorder.Calls()))
		}
	})
}

// writeFabricAnchor writes anchor as the .lym-anchor marker in warpPath's hub board directory.
// A later lymcwd.ResolveWorktree call then resolves the anchor instead of falling back to a cwd-derived ".".
// The writeAnchor helper in lymcwd_test.go cannot be imported across packages, so this copy duplicates it.
func writeFabricAnchor(t *testing.T, warpPath, anchor string) {
	t.Helper()

	boardDir := fabricengine.BoardDir(filepath.Dir(warpPath))
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatalf("mkdir board dir: %v", err)
	}
	anchorPath := filepath.Join(boardDir, lymcwd.AnchorFileName)
	if err := os.WriteFile(anchorPath, []byte(anchor), 0o644); err != nil {
		t.Fatalf("write %s: %v", anchorPath, err)
	}
}

// newUnbornWeftRepo creates an isolated git repo on branch main with no commits, so its HEAD is unborn.
// A plain weft fixture and a real hub's weft worktree both carry commits, so neither can reach this state.
func newUnbornWeftRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	gitkit.MustRun(t, dir, "git", "init", "-q", "-b", "main")
	gitkit.MustRun(t, dir, "git", "config", "user.email", "test@test.com")
	gitkit.MustRun(t, dir, "git", "config", "user.name", "Test")
	return dir
}

// The "did not match any files" fallback in commitWeftLocked, combined with tags, has no test in this file.
// weftPathspecFilter pre-checks the pathspec with the same git semantics StageAndCommit's git add uses,
// so the fallback is defense-in-depth for other StageAndCommit callers and cannot be forced from these tests.

// TestCommit_UnbornWeftHEAD_WithTags_LandsAsRootCommit covers an unborn weft HEAD with snapshot tags.
// The call lands an empty commit as weft's root commit, carrying its Warp-SHA and Snapshot trailers.
func TestCommit_UnbornWeftHEAD_WithTags_LandsAsRootCommit(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: SwapPushRecorderForTest sets the package-level spawnDetachedPushFn.
	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	weftPath := newUnbornWeftRepo(t)
	fabricengine.SeedFabricConfigForTest(t, warpPath)
	f := fabricengine.NewFabricForTest(t, warpPath, weftPath)
	fabricengine.SwapPushRecorderForTest(t)

	warpHEAD := fabricengine.CurrentSHAForTest(t, warpPath)

	result, err := f.Commit(nil, "unborn weft, tags only", []string{"raddle"}, fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if !result.WeftCommitted || result.WeftSHA == "" {
		t.Fatalf("Commit() = %+v; want an empty weft commit to have landed as weft's root commit", result)
	}

	msg := fabricengine.CommitMessageAtForTest(t, weftPath, result.WeftSHA)
	wantTrailer := fabricengine.WarpSHATrailerKey + ": " + warpHEAD
	if !strings.Contains(msg, wantTrailer) {
		t.Errorf("root weft commit message = %q; want it to contain %q", msg, wantTrailer)
	}
	if !strings.Contains(msg, fabricengine.SnapshotTrailerKey+": raddle") {
		t.Errorf("root weft commit message = %q; want it to contain the Snapshot trailer", msg)
	}

	got, err := fabricengine.SnapshotWarpSHAForTest(f, "raddle")
	if err != nil {
		t.Fatalf("snapshotWarpSHA() error = %v", err)
	}
	if got != warpHEAD {
		t.Errorf("snapshotWarpSHA() = %q; want %q", got, warpHEAD)
	}
}

// TestCommit_InvalidTag_OtherwiseEmpty_NothingCommitted pins that validation survives the empty-commit path.
// `appendSnapshotTrailers` validates every tag before it appends any.
// An invalid tag on an otherwise empty call fails before anything is staged or committed, on either side.
func TestCommit_InvalidTag_OtherwiseEmpty_NothingCommitted(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: it swaps the package-level `spawnDetachedPushFn` hook, so it must not run in parallel.
	f, warpPath, _ := fabricengine.NewCommitFixtureForTest(t)
	fabricengine.SwapPushRecorderForTest(t)

	preWarpSHA := fabricengine.CurrentSHAForTest(t, warpPath)
	preWeftSHA, err := fabricengine.WeftForTest(f).CurrentSHA()
	if err != nil {
		t.Fatalf("Weft.CurrentSHA() error = %v", err)
	}

	_, err = f.Commit(nil, "invalid tag commit", []string{"bad tag with spaces"}, fabricengine.SyncOptions{})
	var invalidTagErr *fabricengine.ErrInvalidSnapshotTag
	if !errors.As(err, &invalidTagErr) {
		t.Fatalf("Commit() error = %v; want *fabricengine.ErrInvalidSnapshotTag via errors.As", err)
	}

	postWarpSHA := fabricengine.CurrentSHAForTest(t, warpPath)
	postWeftSHA, err := fabricengine.WeftForTest(f).CurrentSHA()
	if err != nil {
		t.Fatalf("Weft.CurrentSHA() error = %v", err)
	}
	if postWarpSHA != preWarpSHA {
		t.Errorf("warp HEAD changed from %q to %q; want unchanged", preWarpSHA, postWarpSHA)
	}
	if postWeftSHA != preWeftSHA {
		t.Errorf("weft HEAD changed from %q to %q; want unchanged", preWeftSHA, postWeftSHA)
	}
}

// TestCommit_DirtyWeftIndex_UnchangedContentWithTags_SurfacesPartialCommitError pins the no-op-becomes-error case.
// An aborted earlier run leaves unrelated content staged in the weft index.
// The combined write lock serializes callers against that content but does not clean it up.
// A tagged commit whose pathspec content is unchanged reaches the `!committed` fall-through and calls `CommitEmpty`.
// The dirty-index pre-check in `CommitEmpty` refuses with `gitrepo.ErrIndexNotEmpty`.
// That refusal must surface as a `*fabricengine.PartialCommitError` with WeftCommitted=false.
// The warp commit that already landed stands and is still pushed.
func TestCommit_DirtyWeftIndex_UnchangedContentWithTags_SurfacesPartialCommitError(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: it swaps the package-level `spawnDetachedPushFn` hook, so it must not run in parallel.
	f, warpPath, weftPath := fabricengine.NewCommitFixtureForTest(t)
	recorder := fabricengine.SwapPushRecorderForTest(t)

	fabricengine.WriteWarpFileForTest(t, warpPath, "README", "warp change")

	// The staged file is unrelated to this call's pathspec, so the pathspec-scoped diff in StageAndCommit never sees it.
	// CommitEmpty's unscoped `diff --cached --quiet` does.
	if err := os.WriteFile(filepath.Join(weftPath, "leftover.txt"), []byte("staged by an aborted earlier run"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	gitkit.MustRun(t, weftPath, "git", "add", "leftover.txt")

	// `_lym/config.yaml` stays at its fixture-default content, so the weft-side StageAndCommit reports committed=false.
	// The empty-commit rule's `!committed` fall-through then fires.
	result, err := f.Commit([]string{"README", "_lym/config.yaml"}, "dirty index, tagged", []string{"raddle"}, fabricengine.SyncOptions{})
	if err == nil {
		t.Fatal("Commit() error = nil; want an error (CommitEmpty should refuse on the dirty index)")
	}

	var partialErr *fabricengine.PartialCommitError
	if !errors.As(err, &partialErr) {
		t.Fatalf("Commit() error = %v; want *fabricengine.PartialCommitError via errors.As", err)
	}
	if fabricengine.PartialCommitErrorWeftCommittedForTest(partialErr) {
		t.Errorf("fabricengine.PartialCommitError.weftCommitted = true; want false (CommitEmpty refused, nothing landed)")
	}
	if !errors.Is(err, gitrepo.ErrIndexNotEmpty) {
		t.Errorf("Commit() error = %v; want errors.Is(err, gitrepo.ErrIndexNotEmpty)", err)
	}

	if !result.WarpCommitted || result.WarpSHA == "" {
		t.Errorf("Commit() = %+v; want the warp commit to stand", result)
	}
	if result.WeftCommitted {
		t.Errorf("Commit() = %+v; want WeftCommitted=false", result)
	}

	if len(recorder.Calls()) != 1 {
		t.Errorf("push recorder invocation count = %d; want 1 (the durable warp commit should still be pushed)", len(recorder.Calls()))
	}
}

// TestCommitWeft_PathspecMatchesNothing_WithTags_LandsEmptyCommit pins that CommitWeft lands the empty commit.
// The empty-commit rule is part of CommitWeft's own contract.
// It is called with a pathspec that matches nothing and one snapshot tag.
//
//testtiming:keep CommitWeft called directly with a pathspec matching nothing and one tag landing the empty commit as its own contract; coverage of its blocks by other tests does not show an assertion of this
func TestCommitWeft_PathspecMatchesNothing_WithTags_LandsEmptyCommit(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	weftFixture := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	f := fabricengine.NewFabricForTest(t, warpPath, weftFixture.PrimeRecords())

	sha, committed, err := fabricengine.CommitWeftForTest(f, []string{"doesnotexist"}, fabricengine.DefaultCommitMessage, fabricengine.SyncOptions{}, "raddle")
	if err != nil {
		t.Fatalf("commitWeft() error = %v", err)
	}
	if !committed || sha == "" {
		t.Fatalf("commitWeft() = (%q, %v); want an empty commit to have landed", sha, committed)
	}

	warpSHA := fabricengine.CurrentSHAForTest(t, warpPath)
	msg := fabricengine.CommitMessageAtForTest(t, weftFixture.PrimeRecords(), sha)
	wantWarpTrailer := fabricengine.WarpSHATrailerKey + ": " + warpSHA
	if !strings.Contains(msg, wantWarpTrailer) {
		t.Errorf("commit message = %q; want it to contain %q", msg, wantWarpTrailer)
	}
	if !strings.Contains(msg, fabricengine.SnapshotTrailerKey+": raddle") {
		t.Errorf("commit message = %q; want it to contain the Snapshot trailer", msg)
	}
}

// TestCommit_DotLymPath_HardErrorsAndCommitsNothing asserts that a Fabric.Commit call naming a path under `.lym` fails.
// The error names that path, and nothing lands on either side.
// `.lym` is the never-committed structural directory.
// The never-committed check runs before any lock is taken or any commit attempted,
// so a rejected call leaves both warp and weft HEAD unmoved.
func TestCommit_DotLymPath_HardErrorsAndCommitsNothing(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: it swaps the package-level `spawnDetachedPushFn` hook, so it must not run in parallel.
	f, warpPath, weftPath := fabricengine.NewCommitFixtureForTest(t)
	fabricengine.SwapPushRecorderForTest(t)

	warpSHABefore := fabricengine.CurrentSHAForTest(t, warpPath)
	weftSHABefore := fabricengine.CurrentSHAForTest(t, weftPath)

	offendingPath := ".lym/webster/state.json.lock"
	result, err := f.Commit([]string{offendingPath}, "should never land", nil, fabricengine.SyncOptions{})
	if err == nil {
		t.Fatalf("Commit(%q) error = nil; want a hard error naming the offending path", offendingPath)
	}
	if !strings.Contains(err.Error(), offendingPath) {
		t.Errorf("Commit(%q) error = %q; want it to name the offending path", offendingPath, err.Error())
	}
	if result.WarpCommitted || result.WeftCommitted {
		t.Errorf("Commit(%q) = %+v; want nothing committed on either side", offendingPath, result)
	}

	if got := fabricengine.CurrentSHAForTest(t, warpPath); got != warpSHABefore {
		t.Errorf("warp HEAD moved from %s to %s; want it unchanged by a rejected call", warpSHABefore, got)
	}
	if got := fabricengine.CurrentSHAForTest(t, weftPath); got != weftSHABefore {
		t.Errorf("weft HEAD moved from %s to %s; want it unchanged by a rejected call", weftSHABefore, got)
	}
}
