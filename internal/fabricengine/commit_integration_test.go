// commit_integration_test.go covers Fabric.Commit's two-sided commit.
// It tests warp-first ordering, correspondence recording, CommitResult fields, Snapshot trailers, and message handling;
// it also covers the spawnDetachedPushFn push seam, the empty-commit rule, and CommitWeft's inherited contract.
// It is package fabricengine_test and reaches the unexported seams through export_test.go's ForTest shims.

package fabricengine_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
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

// TestCommit_TwoSided_RecordsCorrespondence asserts that a two-sided Fabric.Commit records its weft commit.
// The correspondence index holds that weft commit against the warp SHA the call just created.
func TestCommit_TwoSided_RecordsCorrespondence(t *testing.T) {
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

	got, err := f.WeftSHAForWarpSHA(result.WarpSHA)
	if err != nil {
		t.Fatalf("WeftSHAForWarpSHA() error = %v", err)
	}
	if got != result.WeftSHA {
		t.Errorf("WeftSHAForWarpSHA(%q) = %q; want %q", result.WarpSHA, got, result.WeftSHA)
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

// TestCommit_SnapshotTrailers asserts that a Snapshot: trailer is present on the weft commit for each
// snapshotTags entry, and absent entirely when snapshotTags is empty.
func TestCommit_SnapshotTrailers(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: SwapPushRecorderForTest sets the package-level spawnDetachedPushFn.
	t.Run("Present", func(t *testing.T) {
		f, warpPath, weftPath := fabricengine.NewCommitFixtureForTest(t)
		fabricengine.SwapPushRecorderForTest(t)

		fabricengine.WriteWarpFileForTest(t, warpPath, "README", "warp change")
		fabricengine.WriteWeftConfigContentForTest(t, weftPath, "weft change")

		result, err := f.Commit([]string{"README", "_lym/config.yaml"}, "tagged commit", []string{"tag1", "tag2"}, fabricengine.SyncOptions{})
		if err != nil {
			t.Fatalf("Commit() error = %v", err)
		}

		msg := fabricengine.CommitMessageAtForTest(t, weftPath, result.WeftSHA)
		for _, tag := range []string{"tag1", "tag2"} {
			want := fabricengine.SnapshotTrailerKey + ": " + tag
			if !strings.Contains(msg, want) {
				t.Errorf("weft commit message = %q; want it to contain %q", msg, want)
			}
		}
	})

	t.Run("Absent", func(t *testing.T) {
		f, warpPath, weftPath := fabricengine.NewCommitFixtureForTest(t)
		fabricengine.SwapPushRecorderForTest(t)

		fabricengine.WriteWarpFileForTest(t, warpPath, "README", "warp change")
		fabricengine.WriteWeftConfigContentForTest(t, weftPath, "weft change")

		result, err := f.Commit([]string{"README", "_lym/config.yaml"}, "untagged commit", nil, fabricengine.SyncOptions{})
		if err != nil {
			t.Fatalf("Commit() error = %v", err)
		}

		msg := fabricengine.CommitMessageAtForTest(t, weftPath, result.WeftSHA)
		if strings.Contains(msg, fabricengine.SnapshotTrailerKey+":") {
			t.Errorf("weft commit message = %q; want no Snapshot trailer", msg)
		}
	})
}

// TestCommit_MessageHandling asserts the warp commit message is the bare msg,
// and the weft commit message carries msg plus its Warp-SHA and Snapshot trailers.
func TestCommit_MessageHandling(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: SwapPushRecorderForTest sets the package-level spawnDetachedPushFn.
	f, warpPath, weftPath := fabricengine.NewCommitFixtureForTest(t)
	fabricengine.SwapPushRecorderForTest(t)

	fabricengine.WriteWarpFileForTest(t, warpPath, "README", "warp change")
	fabricengine.WriteWeftConfigContentForTest(t, weftPath, "weft change")

	result, err := f.Commit([]string{"README", "_lym/config.yaml"}, "custom commit message", []string{"tag1"}, fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	warpMsg := strings.TrimSpace(fabricengine.CommitMessageAtForTest(t, warpPath, result.WarpSHA))
	if warpMsg != "custom commit message" {
		t.Errorf("warp commit message = %q; want bare %q", warpMsg, "custom commit message")
	}

	weftMsg := fabricengine.CommitMessageAtForTest(t, weftPath, result.WeftSHA)
	if !strings.HasPrefix(weftMsg, "custom commit message") {
		t.Errorf("weft commit message = %q; want it to start with %q", weftMsg, "custom commit message")
	}
	if !strings.Contains(weftMsg, fabricengine.WarpSHATrailerKey+": "+result.WarpSHA) {
		t.Errorf("weft commit message = %q; want it to contain the Warp-SHA trailer for %q", weftMsg, result.WarpSHA)
	}
	if !strings.Contains(weftMsg, fabricengine.SnapshotTrailerKey+": tag1") {
		t.Errorf("weft commit message = %q; want it to contain the Snapshot trailer for %q", weftMsg, "tag1")
	}
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

// TestCommit_NoOp_DoesNotInvokePushRecorder asserts that a no-op Fabric.Commit never invokes spawnDetachedPushFn.
// A no-op call lands nothing on either side: an empty files list, or a warp-only input with unchanged content.
func TestCommit_NoOp_DoesNotInvokePushRecorder(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: SwapPushRecorderForTest sets the package-level spawnDetachedPushFn.
	t.Run("EmptyFiles", func(t *testing.T) {
		f, _, _ := fabricengine.NewCommitFixtureForTest(t)
		recorder := fabricengine.SwapPushRecorderForTest(t)

		result, err := f.Commit(nil, "no-op commit", nil, fabricengine.SyncOptions{})
		if err != nil {
			t.Fatalf("Commit() error = %v", err)
		}
		if result.WarpCommitted || result.WeftCommitted {
			t.Fatalf("Commit() = %+v; want a full no-op", result)
		}
		if len(recorder.Calls()) != 0 {
			t.Errorf("push recorder invocation count = %d; want 0 (calls: %+v)", len(recorder.Calls()), recorder.Calls())
		}
	})

	t.Run("WarpOnlyUnchanged", func(t *testing.T) {
		f, warpPath, _ := fabricengine.NewCommitFixtureForTest(t)
		recorder := fabricengine.SwapPushRecorderForTest(t)

		// newPlainWarpRepo already committed README with content "warp";
		// writing the identical content again leaves nothing staged.
		fabricengine.WriteWarpFileForTest(t, warpPath, "README", "warp")

		result, err := f.Commit([]string{"README"}, "no-op commit", nil, fabricengine.SyncOptions{})
		if err != nil {
			t.Fatalf("Commit() error = %v", err)
		}
		if result.WarpCommitted || result.WeftCommitted {
			t.Fatalf("Commit() = %+v; want a full no-op (unchanged content)", result)
		}
		if len(recorder.Calls()) != 0 {
			t.Errorf("push recorder invocation count = %d; want 0 (calls: %+v)", len(recorder.Calls()), recorder.Calls())
		}
	})
}

// TestCommit_WarpOnly_SnapshotTagsForceEmptyWeftCommit asserts a warp-only call with snapshot tags.
// It commits the warp side and lands an empty weft commit with a Warp-SHA trailer and a Snapshot trailer per tag.
func TestCommit_WarpOnly_SnapshotTagsForceEmptyWeftCommit(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: SwapPushRecorderForTest sets the package-level spawnDetachedPushFn.
	f, warpPath, weftPath := fabricengine.NewCommitFixtureForTest(t)
	fabricengine.SwapPushRecorderForTest(t)

	fabricengine.WriteWarpFileForTest(t, warpPath, "README", "warp only change, tagged")

	result, err := f.Commit([]string{"README"}, "warp only commit", []string{"tag1"}, fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if !result.WarpCommitted {
		t.Fatalf("Commit() = %+v; want WarpCommitted=true", result)
	}
	if !result.WeftCommitted || result.WeftSHA == "" {
		t.Fatalf("Commit() = %+v; want an empty weft commit to have landed (WeftCommitted=true, WeftSHA populated)", result)
	}

	weftMsg := fabricengine.CommitMessageAtForTest(t, weftPath, result.WeftSHA)
	wantWarpTrailer := fabricengine.WarpSHATrailerKey + ": " + result.WarpSHA
	if !strings.Contains(weftMsg, wantWarpTrailer) {
		t.Errorf("empty weft commit message = %q; want it to contain %q", weftMsg, wantWarpTrailer)
	}
	if !strings.Contains(weftMsg, fabricengine.SnapshotTrailerKey+": tag1") {
		t.Errorf("empty weft commit message = %q; want it to contain the Snapshot trailer for %q", weftMsg, "tag1")
	}
}

// TestCommit_UnchangedWeftContent_TagsStillAdvanceSnapshotBaseline guards against a stale snapshot baseline.
// Two Fabric.Commit calls land identical weft content under the same snapshot tag at different warp SHAs.
// Without an empty weft commit, the second call leaves snapshotWarpSHA answering with the first call's warp SHA.
func TestCommit_UnchangedWeftContent_TagsStillAdvanceSnapshotBaseline(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: SwapPushRecorderForTest sets the package-level spawnDetachedPushFn.
	f, warpPath, weftPath := fabricengine.NewCommitFixtureForTest(t)
	fabricengine.SwapPushRecorderForTest(t)

	fabricengine.WriteWarpFileForTest(t, warpPath, "README", "warp change 1")
	fabricengine.WriteWeftConfigContentForTest(t, weftPath, "identical content")
	result1, err := f.Commit([]string{"README", "_lym/config.yaml"}, "commit 1", []string{"raddle"}, fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("Commit() round 1 error = %v", err)
	}
	if !result1.WarpCommitted || !result1.WeftCommitted {
		t.Fatalf("Commit() round 1 = %+v; want both sides committed", result1)
	}

	got1, err := fabricengine.SnapshotWarpSHAForTest(f, "raddle")
	if err != nil {
		t.Fatalf("snapshotWarpSHA() after round 1 error = %v", err)
	}
	if got1 != result1.WarpSHA {
		t.Fatalf("snapshotWarpSHA() after round 1 = %q; want %q", got1, result1.WarpSHA)
	}

	// Advance warp again, but write identical weft content.
	// The weft-side pathspec matches tracked content, yet StageAndCommit finds nothing to change against HEAD.
	fabricengine.WriteWarpFileForTest(t, warpPath, "README", "warp change 2")
	fabricengine.WriteWeftConfigContentForTest(t, weftPath, "identical content")
	result2, err := f.Commit([]string{"README", "_lym/config.yaml"}, "commit 2", []string{"raddle"}, fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("Commit() round 2 error = %v", err)
	}
	if !result2.WarpCommitted {
		t.Fatalf("Commit() round 2 = %+v; want WarpCommitted=true", result2)
	}
	if !result2.WeftCommitted || result2.WeftSHA == "" {
		t.Fatalf("Commit() round 2 = %+v; want an empty weft commit to have landed despite unchanged content", result2)
	}
	if result2.WeftSHA == result1.WeftSHA {
		t.Errorf("Commit() round 2 WeftSHA = %q; want a NEW commit distinct from round 1's %q", result2.WeftSHA, result1.WeftSHA)
	}

	got2, err := fabricengine.SnapshotWarpSHAForTest(f, "raddle")
	if err != nil {
		t.Fatalf("snapshotWarpSHA() after round 2 error = %v", err)
	}
	if got2 != result2.WarpSHA {
		t.Errorf("snapshotWarpSHA() after round 2 = %q; want the ADVANCED baseline %q, not the stale round-1 baseline %q", got2, result2.WarpSHA, result1.WarpSHA)
	}
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

// TestCommit_NestedRelPath_ClassifiesWeftFileUnderRelPath asserts weft files classify under the worktree's RelPath.
// A two-segment anchor makes RelPath differ from ".",
// so a weft file nested under it must still land in a real commit.
func TestCommit_NestedRelPath_ClassifiesWeftFileUnderRelPath(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: SwapPushRecorderForTest sets the package-level spawnDetachedPushFn.
	f, warpPath, weftPath := fabricengine.NewCommitFixtureForTest(t)
	fabricengine.SwapPushRecorderForTest(t)

	const anchor = "wts/some-task"
	writeFabricAnchor(t, warpPath, anchor)
	// Resolving the prime runs git in the anchor directory, which must exist in the code worktree.
	if err := os.MkdirAll(filepath.Join(warpPath, filepath.FromSlash(anchor)), 0o755); err != nil {
		t.Fatalf("mkdir anchor in code worktree: %v", err)
	}

	nestedDir := filepath.Join(weftPath, filepath.FromSlash(anchor), "_lym")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("mkdir nested weft dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "nested.yaml"), []byte("nested weft content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	nestedRel := anchor + "/_lym/nested.yaml"

	result, err := f.Commit([]string{nestedRel}, "nested relpath commit", nil, fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if result.WarpCommitted {
		t.Errorf("Commit() = %+v; want no warp commit -- the nested path must route to weft, not warp", result)
	}
	if !result.WeftCommitted || result.WeftSHA == "" {
		t.Fatalf("Commit() = %+v; want a populated WeftSHA and WeftCommitted=true", result)
	}

	cmd := exec.Command("git", "show", result.WeftSHA+":"+nestedRel)
	cmd.Dir = weftPath
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git show %s:%s in %s: %v", result.WeftSHA, nestedRel, weftPath, err)
	}
	if string(out) != "nested weft content" {
		t.Errorf("committed content = %q; want %q", string(out), "nested weft content")
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

// TestCommit_TagsOnly_LandsEmptyWeftCommit covers the tags-only call shape, Commit(nil, msg, tags, opts).
// A nil files list with non-empty snapshotTags still takes the combined write lock.
// The test holds that lock externally and checks that the call blocks until it is released.
// The call then lands an empty weft commit, and snapshotWarpSHA resolves the tag to warp's current HEAD.
func TestCommit_TagsOnly_LandsEmptyWeftCommit(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: SwapPushRecorderForTest sets the package-level spawnDetachedPushFn.
	f, warpPath, _ := fabricengine.NewCommitFixtureForTest(t)
	fabricengine.SwapPushRecorderForTest(t)

	warpHEAD := fabricengine.CurrentSHAForTest(t, warpPath)

	lockPath := fabricengine.WeftWriteLockPathForTest(t, f)
	externalLock, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		t.Fatalf("AcquireWriteLock(%q) error = %v", lockPath, err)
	}

	type outcome struct {
		result fabricengine.CommitResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := f.Commit(nil, "tags only commit", []string{"raddle"}, fabricengine.SyncOptions{})
		done <- outcome{result: result, err: err}
	}()

	select {
	case <-done:
		_ = externalLock.Release()
		t.Fatal("Commit() completed while the combined write lock was externally held; want it to block (a tags-only call must take the lock)")
	case <-time.After(150 * time.Millisecond):
		// Still blocked, as expected.
	}

	if err := externalLock.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}

	var got outcome
	select {
	case got = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Commit() did not complete after the external lock was released")
	}
	if got.err != nil {
		t.Fatalf("Commit() error = %v", got.err)
	}

	result := got.result
	if result.WarpCommitted || result.WarpSHA != "" {
		t.Errorf("Commit() = %+v; want no warp commit (nil files)", result)
	}
	if !result.WeftCommitted || result.WeftSHA == "" {
		t.Fatalf("Commit() = %+v; want an empty weft commit to have landed", result)
	}

	gotWarpSHA, err := fabricengine.SnapshotWarpSHAForTest(f, "raddle")
	if err != nil {
		t.Fatalf("snapshotWarpSHA() error = %v", err)
	}
	if gotWarpSHA != warpHEAD {
		t.Errorf("snapshotWarpSHA() = %q; want warp's current HEAD %q", gotWarpSHA, warpHEAD)
	}
}

// TestCommit_PathspecFilteredToNothing_WithTags_LandsEmptyWeftCommit covers a weft-side pathspec matching nothing.
// weftPathspecFilter drops such an entry,
// and the call still lands an empty weft commit when snapshotTags is non-empty.
func TestCommit_PathspecFilteredToNothing_WithTags_LandsEmptyWeftCommit(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: SwapPushRecorderForTest sets the package-level spawnDetachedPushFn.
	f, warpPath, _ := fabricengine.NewCommitFixtureForTest(t)
	fabricengine.SwapPushRecorderForTest(t)

	fabricengine.WriteWarpFileForTest(t, warpPath, "README", "warp change")

	result, err := f.Commit([]string{"README", "_lym/doesnotexist"}, "commit", []string{"raddle"}, fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if !result.WeftCommitted || result.WeftSHA == "" {
		t.Fatalf("Commit() = %+v; want an empty weft commit despite the weft-side pathspec matching nothing", result)
	}
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

// TestCommit_UnbornWarpHEAD_WithTags_DropsTagsNoErrorNoCommit covers an unborn warp HEAD with snapshot tags.
// The call drops the tags with no commit and no error.
// A snapshot's whole content is a warp SHA, and an unborn warp HEAD has none to record.
func TestCommit_UnbornWarpHEAD_WithTags_DropsTagsNoErrorNoCommit(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: SwapPushRecorderForTest sets the package-level spawnDetachedPushFn.
	warpPath := newUnbornWarpRepo(t)
	weftFixture := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	fabricengine.SeedFabricConfigForTest(t, warpPath)
	f := fabricengine.NewFabricForTest(t, warpPath, weftFixture.PrimeRecords())
	fabricengine.SwapPushRecorderForTest(t)

	preWeftSHA := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeRecords())

	result, err := f.Commit(nil, "unborn warp, tags only", []string{"raddle"}, fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if result.WarpCommitted || result.WeftCommitted {
		t.Errorf("Commit() = %+v; want a full no-op (unborn warp drops tags exactly as before)", result)
	}

	postWeftSHA := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeRecords())
	if postWeftSHA != preWeftSHA {
		t.Errorf("weft HEAD changed from %q to %q; want unchanged (no commit)", preWeftSHA, postWeftSHA)
	}

	got, err := fabricengine.SnapshotWarpSHAForTest(f, "raddle")
	if err != nil {
		t.Fatalf("snapshotWarpSHA() error = %v", err)
	}
	if got != "" {
		t.Errorf("snapshotWarpSHA() = %q; want \"\" (nothing was recorded)", got)
	}
}

// TestCommit_SkipGit_WithTags_NoWeftCommitNoError covers opts.SkipGit with snapshot tags.
// SkipGit opts the weft side out of git entirely, so no weft commit or trailer lands whatever the tags.
// The warp commit still proceeds, because SkipGit is weft-scoped in Fabric.Commit.
func TestCommit_SkipGit_WithTags_NoWeftCommitNoError(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: it swaps the package-level `spawnDetachedPushFn` hook, so it must not run in parallel.
	f, warpPath, weftPath := fabricengine.NewCommitFixtureForTest(t)
	fabricengine.SwapPushRecorderForTest(t)

	fabricengine.WriteWarpFileForTest(t, warpPath, "README", "warp change")
	fabricengine.WriteWeftConfigContentForTest(t, weftPath, "weft change")

	result, err := f.Commit([]string{"README", "_lym/config.yaml"}, "skip git", []string{"raddle"}, fabricengine.SyncOptions{SkipGit: true})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if result.WeftCommitted {
		t.Errorf("Commit() = %+v; want no weft commit under SkipGit even with tags", result)
	}
	if !result.WarpCommitted {
		t.Errorf("Commit() = %+v; want the warp commit to still land (SkipGit is weft-scoped)", result)
	}

	got, err := fabricengine.SnapshotWarpSHAForTest(f, "raddle")
	if err != nil {
		t.Fatalf("snapshotWarpSHA() error = %v", err)
	}
	if got != "" {
		t.Errorf("snapshotWarpSHA() = %q; want \"\" (SkipGit skips the weft side entirely)", got)
	}
}

// TestCommit_NoTagsNothingToCommit_RuleDoesNotOverFire guards that the empty-commit rule does not fire without tags.
// Unchanged content and zero tags must still be a clean no-op, with the weft HEAD left untouched.
func TestCommit_NoTagsNothingToCommit_RuleDoesNotOverFire(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: it swaps the package-level `spawnDetachedPushFn` hook, so it must not run in parallel.
	f, warpPath, _ := fabricengine.NewCommitFixtureForTest(t)
	fabricengine.SwapPushRecorderForTest(t)

	preWeftSHA, err := fabricengine.WeftForTest(f).CurrentSHA()
	if err != nil {
		t.Fatalf("Weft.CurrentSHA() error = %v", err)
	}

	// `newPlainWarpRepo` already committed README with content "warp".
	// Writing the identical content again leaves nothing staged, and no tags are supplied.
	fabricengine.WriteWarpFileForTest(t, warpPath, "README", "warp")

	result, err := f.Commit([]string{"README"}, "no-op, no tags", nil, fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if result.WarpCommitted || result.WeftCommitted {
		t.Fatalf("Commit() = %+v; want a full no-op (unchanged content, no tags)", result)
	}

	postWeftSHA, err := fabricengine.WeftForTest(f).CurrentSHA()
	if err != nil {
		t.Fatalf("Weft.CurrentSHA() error = %v", err)
	}
	if postWeftSHA != preWeftSHA {
		t.Errorf("weft HEAD changed from %q to %q; want unchanged (the empty-commit rule must not fire with no tags)", preWeftSHA, postWeftSHA)
	}
}

// TestCommit_WarpOnlyTagged_InvokesPushRecorderOnce pins the `fabricengine.CommitResult` shape.
// For a warp-only tagged commit it expects WeftCommitted=true with a populated WeftSHA (the empty commit).
// It also expects exactly one push-seam invocation.
// The async-push gate is WarpCommitted || WeftCommitted.
// The snapshot trailer must reach the remote for cross-clone sharing.
func TestCommit_WarpOnlyTagged_InvokesPushRecorderOnce(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: it swaps the package-level `spawnDetachedPushFn` hook, so it must not run in parallel.
	f, warpPath, _ := fabricengine.NewCommitFixtureForTest(t)
	recorder := fabricengine.SwapPushRecorderForTest(t)

	fabricengine.WriteWarpFileForTest(t, warpPath, "README", "warp only tagged change")

	result, err := f.Commit([]string{"README"}, "warp only tagged commit", []string{"raddle"}, fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if !result.WeftCommitted || result.WeftSHA == "" {
		t.Fatalf("Commit() = %+v; want WeftCommitted=true and a populated WeftSHA", result)
	}
	if len(recorder.Calls()) != 1 {
		t.Fatalf("push recorder invocation count = %d; want 1", len(recorder.Calls()))
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
