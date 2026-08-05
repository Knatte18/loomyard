//go:build integration

// commit_integration_test.go — integration coverage for Fabric.Commit's
// classify-and-dispatch two-sided commit: warp-first ordering, correspondence
// recording, CommitResult field population across two-sided/warp-only/
// weft-only inputs, the warp commit's plain-git property (no trailer, no
// correspondence entry), Snapshot trailer presence/absence, message
// handling, and the spawnDetachedPushFn push-invocation seam (fired for
// anything landed, never for a genuine no-op). It also carries this batch's
// empty-commit-rule coverage: the inverted warp-only-tagged case, the
// unchanged-content correctness hole, every remaining triggering case (a
// tags-only call, a pathspec filtered to nothing, an unborn weft HEAD), both
// exceptions (unborn warp HEAD, SkipGit), the no-tags-no-op guard against
// over-firing, the CommitResult/push-seam shape on a warp-only tagged
// commit, the invalid-tag-on-an-otherwise-empty-commit guard, the dirty-
// weft-index-becomes-an-error transition, and CommitWeft's own inherited
// contract. Package fabricengine (internal) because it swaps the unexported
// spawnDetachedPushFn seam and reaches unexported helpers (weftWriteLockPath
// from commit_lock_integration_test.go). Reuses index_integration_test.go's
// fixture helpers (newPlainWarpRepo, commitWarp, newFabric, currentSHA),
// weftgit_unborn_warp_test.go's newUnbornWarpRepo, and
// syncweft_integration_test.go's (writeWeftConfigContent, commitMessageAt).

package fabricengine

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// seedFabricConfig materializes the repo-wide fabric.yaml Fabric.Commit's
// classify step now reads via RepoWiredNames — the `weft:main` base at
// hubgeometry.BoardDir(Hub) — so its resolved pathspec is what Fabric.Commit's
// classifier needs. warpPath is a bare t.TempDir() plain-git checkout (this
// file's newPlainWarpRepo/newUnbornWarpRepo fixtures, not a real hub tree),
// so hubgeometry.ResolveWorktree(warpPath)'s Hub — warpPath's parent
// directory — stands in for the hub; _board is not itself a git repository,
// so the file is written directly with no git add/commit step. Mirrors
// reconcile_stale_registration_test.go's seedRepoWideFabricConfig, which
// this file cannot call directly (package fabricengine here, vs.
// fabricengine_test there). Every Fabric.Commit test requires this.
func seedFabricConfig(t *testing.T, warpPath string) {
	t.Helper()

	boardDir := hubgeometry.BoardDir(filepath.Dir(warpPath))
	if err := os.MkdirAll(configengine.ConfigDir(boardDir), 0o755); err != nil {
		t.Fatalf("mkdir repo-wide config dir: %v", err)
	}
	configPath := configengine.ConfigFile(boardDir, "fabric")
	if err := os.WriteFile(configPath, []byte("branch_prefix: \"\"\npathspec: _lyx _pattern\n"), 0o644); err != nil {
		t.Fatalf("write repo-wide fabric config: %v", err)
	}
}

// writeWarpFile overwrites (without staging or committing) name inside the
// warp repo at warpPath — the standard way this file's tests dirty a
// warp-side file before calling Fabric.Commit.
func writeWarpFile(t *testing.T, warpPath, name, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(warpPath, name), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// pushCall records one spawnDetachedPushFn invocation's arguments.
type pushCall struct {
	warpPath string
	weftPath string
}

// swapPushRecorder replaces spawnDetachedPushFn with a no-op recorder for
// the duration of the test, restoring the original on cleanup — the
// push-invocation-seam-for-tests Shared Decision. Callers of this helper
// must not use t.Parallel().
func swapPushRecorder(t *testing.T) *[]pushCall {
	t.Helper()

	calls := &[]pushCall{}
	original := spawnDetachedPushFn
	spawnDetachedPushFn = func(warpPath, weftPath string) error {
		*calls = append(*calls, pushCall{warpPath: warpPath, weftPath: weftPath})
		return nil
	}
	t.Cleanup(func() { spawnDetachedPushFn = original })
	return calls
}

// newCommitFixture builds a fresh warp/weft pair with the fabric config
// seeded, returning the Fabric handle and both repo paths.
func newCommitFixture(t *testing.T) (f *Fabric, warpPath, weftPath string) {
	t.Helper()

	warpPath = newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	seedFabricConfig(t, warpPath)
	f = newFabric(t, warpPath, weftFixture.WeftPath)
	return f, warpPath, weftFixture.WeftPath
}

// TestCommit_TwoSided_WarpFirstOrdering asserts that a two-sided
// Fabric.Commit's weft commit carries a Warp-SHA trailer naming the warp
// commit Fabric.Commit just made — never the prior warp HEAD.
func TestCommit_TwoSided_WarpFirstOrdering(t *testing.T) {
	f, warpPath, weftPath := newCommitFixture(t)
	swapPushRecorder(t)

	writeWarpFile(t, warpPath, "README", "warp change")
	writeWeftConfigContent(t, weftPath, "weft change")

	result, err := f.Commit([]string{"README", "_lyx/config.yaml"}, "two-sided commit", nil, SyncOptions{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if !result.WarpCommitted || !result.WeftCommitted {
		t.Fatalf("Commit() = %+v; want both sides committed", result)
	}

	trailerSHA, ok := parseWarpSHATrailer(commitMessageAt(t, weftPath, result.WeftSHA))
	if !ok {
		t.Fatalf("weft commit %s carries no Warp-SHA trailer", result.WeftSHA)
	}
	if trailerSHA != result.WarpSHA {
		t.Errorf("weft commit's Warp-SHA trailer = %q; want the warp commit Fabric.Commit just created %q", trailerSHA, result.WarpSHA)
	}
}

// TestCommit_TwoSided_RecordsCorrespondence asserts that a two-sided
// Fabric.Commit's weft commit is recorded in the correspondence index
// against the warp SHA it just created.
func TestCommit_TwoSided_RecordsCorrespondence(t *testing.T) {
	f, warpPath, weftPath := newCommitFixture(t)
	swapPushRecorder(t)

	writeWarpFile(t, warpPath, "README", "warp change")
	writeWeftConfigContent(t, weftPath, "weft change")

	result, err := f.Commit([]string{"README", "_lyx/config.yaml"}, "two-sided commit", nil, SyncOptions{})
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

// TestCommit_ResultFields asserts CommitResult field population across the
// two-sided, warp-only, and weft-only shapes — including, on the warp-only
// path, the plain-git property (no trailer, no correspondence entry) a bare
// warp commit must have.
func TestCommit_ResultFields(t *testing.T) {
	t.Run("TwoSided", func(t *testing.T) {
		f, warpPath, weftPath := newCommitFixture(t)
		swapPushRecorder(t)

		writeWarpFile(t, warpPath, "README", "warp change")
		writeWeftConfigContent(t, weftPath, "weft change")

		result, err := f.Commit([]string{"README", "_lyx/config.yaml"}, "two-sided commit", nil, SyncOptions{})
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
		f, warpPath, _ := newCommitFixture(t)
		swapPushRecorder(t)

		writeWarpFile(t, warpPath, "README", "warp only change")

		result, err := f.Commit([]string{"README"}, "warp only commit", nil, SyncOptions{})
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
		if _, ok := parseWarpSHATrailer(commitMessageAt(t, warpPath, result.WarpSHA)); ok {
			t.Errorf("warp-only commit %s carries a Warp-SHA trailer; want none", result.WarpSHA)
		}
		if _, err := f.WeftSHAForWarpSHA(result.WarpSHA); err == nil {
			t.Errorf("WeftSHAForWarpSHA(%q) succeeded; want ErrNoCorrespondence (a warp-only commit records no correspondence)", result.WarpSHA)
		}
	})

	t.Run("WeftOnly", func(t *testing.T) {
		f, _, weftPath := newCommitFixture(t)
		swapPushRecorder(t)

		writeWeftConfigContent(t, weftPath, "weft only change")

		result, err := f.Commit([]string{"_lyx/config.yaml"}, "weft only commit", nil, SyncOptions{})
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

// TestCommit_SnapshotTrailers asserts a Snapshot: trailer is present on the
// weft commit for each snapshotTags entry, and absent entirely when
// snapshotTags is empty.
func TestCommit_SnapshotTrailers(t *testing.T) {
	t.Run("Present", func(t *testing.T) {
		f, warpPath, weftPath := newCommitFixture(t)
		swapPushRecorder(t)

		writeWarpFile(t, warpPath, "README", "warp change")
		writeWeftConfigContent(t, weftPath, "weft change")

		result, err := f.Commit([]string{"README", "_lyx/config.yaml"}, "tagged commit", []string{"tag1", "tag2"}, SyncOptions{})
		if err != nil {
			t.Fatalf("Commit() error = %v", err)
		}

		msg := commitMessageAt(t, weftPath, result.WeftSHA)
		for _, tag := range []string{"tag1", "tag2"} {
			want := SnapshotTrailerKey + ": " + tag
			if !strings.Contains(msg, want) {
				t.Errorf("weft commit message = %q; want it to contain %q", msg, want)
			}
		}
	})

	t.Run("Absent", func(t *testing.T) {
		f, warpPath, weftPath := newCommitFixture(t)
		swapPushRecorder(t)

		writeWarpFile(t, warpPath, "README", "warp change")
		writeWeftConfigContent(t, weftPath, "weft change")

		result, err := f.Commit([]string{"README", "_lyx/config.yaml"}, "untagged commit", nil, SyncOptions{})
		if err != nil {
			t.Fatalf("Commit() error = %v", err)
		}

		msg := commitMessageAt(t, weftPath, result.WeftSHA)
		if strings.Contains(msg, SnapshotTrailerKey+":") {
			t.Errorf("weft commit message = %q; want no Snapshot trailer", msg)
		}
	})
}

// TestCommit_MessageHandling asserts the warp commit message is the bare
// msg, and the weft commit message carries msg plus its Warp-SHA and
// Snapshot trailers.
func TestCommit_MessageHandling(t *testing.T) {
	f, warpPath, weftPath := newCommitFixture(t)
	swapPushRecorder(t)

	writeWarpFile(t, warpPath, "README", "warp change")
	writeWeftConfigContent(t, weftPath, "weft change")

	result, err := f.Commit([]string{"README", "_lyx/config.yaml"}, "custom commit message", []string{"tag1"}, SyncOptions{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	warpMsg := strings.TrimSpace(commitMessageAt(t, warpPath, result.WarpSHA))
	if warpMsg != "custom commit message" {
		t.Errorf("warp commit message = %q; want bare %q", warpMsg, "custom commit message")
	}

	weftMsg := commitMessageAt(t, weftPath, result.WeftSHA)
	if !strings.HasPrefix(weftMsg, "custom commit message") {
		t.Errorf("weft commit message = %q; want it to start with %q", weftMsg, "custom commit message")
	}
	if !strings.Contains(weftMsg, WarpSHATrailerKey+": "+result.WarpSHA) {
		t.Errorf("weft commit message = %q; want it to contain the Warp-SHA trailer for %q", weftMsg, result.WarpSHA)
	}
	if !strings.Contains(weftMsg, SnapshotTrailerKey+": tag1") {
		t.Errorf("weft commit message = %q; want it to contain the Snapshot trailer for %q", weftMsg, "tag1")
	}
}

// TestCommit_InvokesPushRecorder asserts a successful two-sided
// Fabric.Commit invokes spawnDetachedPushFn exactly once with (warpPath,
// weftPath).
func TestCommit_InvokesPushRecorder(t *testing.T) {
	f, warpPath, weftPath := newCommitFixture(t)
	calls := swapPushRecorder(t)

	writeWarpFile(t, warpPath, "README", "warp change")
	writeWeftConfigContent(t, weftPath, "weft change")

	if _, err := f.Commit([]string{"README", "_lyx/config.yaml"}, "two-sided commit", nil, SyncOptions{}); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	if len(*calls) != 1 {
		t.Fatalf("push recorder invocation count = %d; want 1 (calls: %+v)", len(*calls), *calls)
	}
	if (*calls)[0].warpPath != warpPath || (*calls)[0].weftPath != weftPath {
		t.Errorf("push recorder called with (%q, %q); want (%q, %q)", (*calls)[0].warpPath, (*calls)[0].weftPath, warpPath, weftPath)
	}
}

// TestCommit_NoOp_DoesNotInvokePushRecorder asserts a Fabric.Commit call
// that lands nothing on either side — an empty files list, or a warp-only
// input whose content is unchanged — never invokes spawnDetachedPushFn.
func TestCommit_NoOp_DoesNotInvokePushRecorder(t *testing.T) {
	t.Run("EmptyFiles", func(t *testing.T) {
		f, _, _ := newCommitFixture(t)
		calls := swapPushRecorder(t)

		result, err := f.Commit(nil, "no-op commit", nil, SyncOptions{})
		if err != nil {
			t.Fatalf("Commit() error = %v", err)
		}
		if result.WarpCommitted || result.WeftCommitted {
			t.Fatalf("Commit() = %+v; want a full no-op", result)
		}
		if len(*calls) != 0 {
			t.Errorf("push recorder invocation count = %d; want 0 (calls: %+v)", len(*calls), *calls)
		}
	})

	t.Run("WarpOnlyUnchanged", func(t *testing.T) {
		f, warpPath, _ := newCommitFixture(t)
		calls := swapPushRecorder(t)

		// newPlainWarpRepo already committed README with content "warp";
		// writing the identical content again leaves nothing staged.
		writeWarpFile(t, warpPath, "README", "warp")

		result, err := f.Commit([]string{"README"}, "no-op commit", nil, SyncOptions{})
		if err != nil {
			t.Fatalf("Commit() error = %v", err)
		}
		if result.WarpCommitted || result.WeftCommitted {
			t.Fatalf("Commit() = %+v; want a full no-op (unchanged content)", result)
		}
		if len(*calls) != 0 {
			t.Errorf("push recorder invocation count = %d; want 0 (calls: %+v)", len(*calls), *calls)
		}
	})
}

// TestCommit_WarpOnly_SnapshotTagsForceEmptyWeftCommit is the inverted form
// of the old TestCommit_WarpOnly_SnapshotTagsDropped: since the
// tags-force-a-weft-commit Shared Decision, a warp-only Fabric.Commit call
// carrying a non-empty snapshotTags no longer drops the tags — it commits
// the warp side as before AND lands an empty weft commit carrying both a
// Warp-SHA trailer (naming the warp commit that just landed) and a
// Snapshot: trailer per tag. The old name asserted the opposite of what this
// batch makes true, so it is renamed rather than merely re-bodied: the name
// is the clearest single statement of the behaviour this batch reverses.
func TestCommit_WarpOnly_SnapshotTagsForceEmptyWeftCommit(t *testing.T) {
	f, warpPath, weftPath := newCommitFixture(t)
	swapPushRecorder(t)

	writeWarpFile(t, warpPath, "README", "warp only change, tagged")

	result, err := f.Commit([]string{"README"}, "warp only commit", []string{"tag1"}, SyncOptions{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if !result.WarpCommitted {
		t.Fatalf("Commit() = %+v; want WarpCommitted=true", result)
	}
	if !result.WeftCommitted || result.WeftSHA == "" {
		t.Fatalf("Commit() = %+v; want an empty weft commit to have landed (WeftCommitted=true, WeftSHA populated)", result)
	}

	weftMsg := commitMessageAt(t, weftPath, result.WeftSHA)
	wantWarpTrailer := WarpSHATrailerKey + ": " + result.WarpSHA
	if !strings.Contains(weftMsg, wantWarpTrailer) {
		t.Errorf("empty weft commit message = %q; want it to contain %q", weftMsg, wantWarpTrailer)
	}
	if !strings.Contains(weftMsg, SnapshotTrailerKey+": tag1") {
		t.Errorf("empty weft commit message = %q; want it to contain the Snapshot trailer for %q", weftMsg, "tag1")
	}
}

// TestCommit_UnchangedWeftContent_TagsStillAdvanceSnapshotBaseline is the
// correctness-hole regression this whole batch exists to close: two
// Fabric.Commit calls land the IDENTICAL weft content under the same
// snapshot tag, at two different warp SHAs. Before the empty-commit rule,
// the second call's weft-side StageAndCommit would report committed=false
// (nothing changed against HEAD), no weft commit would land, and
// snapshotWarpSHA would keep answering with the first call's now-stale warp
// SHA forever — despite the second regeneration having just confirmed
// itself current against a newer baseline. This test must fail before the
// implementation and pass after; a pass before the implementation means the
// fixture itself is wrong.
func TestCommit_UnchangedWeftContent_TagsStillAdvanceSnapshotBaseline(t *testing.T) {
	f, warpPath, weftPath := newCommitFixture(t)
	swapPushRecorder(t)

	writeWarpFile(t, warpPath, "README", "warp change 1")
	writeWeftConfigContent(t, weftPath, "identical content")
	result1, err := f.Commit([]string{"README", "_lyx/config.yaml"}, "commit 1", []string{"raddle"}, SyncOptions{})
	if err != nil {
		t.Fatalf("Commit() round 1 error = %v", err)
	}
	if !result1.WarpCommitted || !result1.WeftCommitted {
		t.Fatalf("Commit() round 1 = %+v; want both sides committed", result1)
	}

	got1, err := f.snapshotWarpSHA("raddle")
	if err != nil {
		t.Fatalf("snapshotWarpSHA() after round 1 error = %v", err)
	}
	if got1 != result1.WarpSHA {
		t.Fatalf("snapshotWarpSHA() after round 1 = %q; want %q", got1, result1.WarpSHA)
	}

	// Advance warp again, but write the IDENTICAL weft content: the
	// weft-side pathspec matches real, tracked content, but StageAndCommit's
	// diff against HEAD finds nothing changed — the unchanged-content case.
	writeWarpFile(t, warpPath, "README", "warp change 2")
	writeWeftConfigContent(t, weftPath, "identical content")
	result2, err := f.Commit([]string{"README", "_lyx/config.yaml"}, "commit 2", []string{"raddle"}, SyncOptions{})
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

	got2, err := f.snapshotWarpSHA("raddle")
	if err != nil {
		t.Fatalf("snapshotWarpSHA() after round 2 error = %v", err)
	}
	if got2 != result2.WarpSHA {
		t.Errorf("snapshotWarpSHA() after round 2 = %q; want the ADVANCED baseline %q, not the stale round-1 baseline %q", got2, result2.WarpSHA, result1.WarpSHA)
	}
}

// writeFabricAnchor records anchor as the .fabric-anchor marker under
// warpPath's hub board directory, so a subsequent hubgeometry.ResolveWorktree
// call resolves l.RelPath to anchor rather than falling back to a
// cwd-derived ".". Mirrors hubgeometry's own anchor_test.go writeAnchor,
// duplicated here rather than imported since that helper lives in the
// hubgeometry_test package.
func writeFabricAnchor(t *testing.T, warpPath, anchor string) {
	t.Helper()

	boardDir := hubgeometry.BoardDir(filepath.Dir(warpPath))
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatalf("mkdir board dir: %v", err)
	}
	anchorPath := filepath.Join(boardDir, hubgeometry.FabricAnchorName)
	if err := os.WriteFile(anchorPath, []byte(anchor), 0o644); err != nil {
		t.Fatalf("write %s: %v", anchorPath, err)
	}
}

// TestCommit_NestedRelPath_ClassifiesWeftFileUnderRelPath is the regression
// guard for the card-6 fix: Fabric.Commit must classify against the
// resolved worktree's l.RelPath, not a hardcoded ".". With a recorded
// two-segment anchor ("wts/some-task"), a file physically nested at
// <RelPath>/_lyx/... in the weft checkout must still route to the weft side
// and land in a real commit — proving the fix beyond the RelPath=="."
// coverage every other test in this file exercises.
func TestCommit_NestedRelPath_ClassifiesWeftFileUnderRelPath(t *testing.T) {
	f, warpPath, weftPath := newCommitFixture(t)
	swapPushRecorder(t)

	const anchor = "wts/some-task"
	writeFabricAnchor(t, warpPath, anchor)

	nestedDir := filepath.Join(weftPath, filepath.FromSlash(anchor), "_lyx")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("mkdir nested weft dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "nested.yaml"), []byte("nested weft content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	nestedRel := anchor + "/_lyx/nested.yaml"

	result, err := f.Commit([]string{nestedRel}, "nested relpath commit", nil, SyncOptions{})
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

// newUnbornWeftRepo creates a minimal, isolated git repo at t.TempDir() on
// branch main with NO commits — an unborn weft HEAD — mirroring
// newUnbornWarpRepo (weftgit_unborn_warp_test.go) so the warp and weft
// unborn fixtures read as the pair they are. lyxtest.CopyWeft's fixture
// already carries commits, so it cannot reach this state.
func newUnbornWeftRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	lyxtest.MustRun(t, dir, "git", "init", "-q", "-b", "main")
	lyxtest.MustRun(t, dir, "git", "config", "user.email", "test@test.com")
	lyxtest.MustRun(t, dir, "git", "config", "user.name", "Test")
	return dir
}

// TestCommit_TagsOnly_LandsEmptyWeftCommit pins the tags-only supported call
// shape (Commit(nil, msg, tags, opts)): a nil files list with a non-empty
// snapshotTags still takes the combined write lock (proven here by
// externally holding it and observing the call block until released),
// lands an empty weft commit, and snapshotWarpSHA resolves the tag to
// warp's current HEAD.
func TestCommit_TagsOnly_LandsEmptyWeftCommit(t *testing.T) {
	f, warpPath, _ := newCommitFixture(t)
	swapPushRecorder(t)

	warpHEAD := currentSHA(t, warpPath)

	lockPath := weftWriteLockPath(t, f)
	externalLock, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		t.Fatalf("AcquireWriteLock(%q) error = %v", lockPath, err)
	}

	type outcome struct {
		result CommitResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := f.Commit(nil, "tags only commit", []string{"raddle"}, SyncOptions{})
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

	gotWarpSHA, err := f.snapshotWarpSHA("raddle")
	if err != nil {
		t.Fatalf("snapshotWarpSHA() error = %v", err)
	}
	if gotWarpSHA != warpHEAD {
		t.Errorf("snapshotWarpSHA() = %q; want warp's current HEAD %q", gotWarpSHA, warpHEAD)
	}
}

// TestCommit_PathspecFilteredToNothing_WithTags_LandsEmptyWeftCommit covers
// the `!positive` fall-through: a weft-side pathspec entry that
// weftPathspecFilter drops (it matches nothing in the worktree or index)
// still lands an empty weft commit when snapshotTags is non-empty.
func TestCommit_PathspecFilteredToNothing_WithTags_LandsEmptyWeftCommit(t *testing.T) {
	f, warpPath, _ := newCommitFixture(t)
	swapPushRecorder(t)

	writeWarpFile(t, warpPath, "README", "warp change")

	result, err := f.Commit([]string{"README", "_lyx/doesnotexist"}, "commit", []string{"raddle"}, SyncOptions{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if !result.WeftCommitted || result.WeftSHA == "" {
		t.Fatalf("Commit() = %+v; want an empty weft commit despite the weft-side pathspec matching nothing", result)
	}
}

// The fourth triggering case — StageAndCommit's "did not match any files"
// tolerance, plus tags — is deliberately NOT covered by a test in this file.
// weftPathspecFilter's own pre-check (entryMatchesWeft, via `git ls-files
// --cached --others`) uses the identical anchor and pathspec semantics
// StageAndCommit's own `git add` uses, so the two can never disagree within
// one commitWeftLocked call: whatever the pre-check says matches is what
// `git add` sees too, in the same synchronous call, with no window for the
// underlying state to change in between. Every construction attempted here —
// exclude-magic combinations, case-only differences under Windows'
// core.ignorecase, deleting the matched path between the two checks — either
// reproduced the SAME "did not match any files" failure the pre-check exists
// to prevent (so weftPathspecFilter itself would have to be bypassed to
// reach commitWeftLocked's fallback at all) or resolved identically in both
// commands and never triggered the fallback. Per this card's own
// instruction, that is recorded here explicitly rather than shipping a
// contrived, potentially-flaky construction: the fallback remains
// defense-in-depth for a StageAndCommit caller other than commitWeftLocked's
// own pre-checked path, not something this package's own tests can force.

// TestCommit_UnbornWeftHEAD_WithTags_LandsAsRootCommit covers the case that
// is NOT an exception to the empty-commit rule: an unborn weft HEAD still
// gets an empty commit as its own root commit, carrying its Warp-SHA and
// Snapshot trailers like any other — contrast with the unborn-WARP case
// below, which drops the tags entirely.
func TestCommit_UnbornWeftHEAD_WithTags_LandsAsRootCommit(t *testing.T) {
	warpPath := newPlainWarpRepo(t)
	weftPath := newUnbornWeftRepo(t)
	seedFabricConfig(t, warpPath)
	f := newFabric(t, warpPath, weftPath)
	swapPushRecorder(t)

	warpHEAD := currentSHA(t, warpPath)

	result, err := f.Commit(nil, "unborn weft, tags only", []string{"raddle"}, SyncOptions{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if !result.WeftCommitted || result.WeftSHA == "" {
		t.Fatalf("Commit() = %+v; want an empty weft commit to have landed as weft's root commit", result)
	}

	msg := commitMessageAt(t, weftPath, result.WeftSHA)
	wantTrailer := WarpSHATrailerKey + ": " + warpHEAD
	if !strings.Contains(msg, wantTrailer) {
		t.Errorf("root weft commit message = %q; want it to contain %q", msg, wantTrailer)
	}
	if !strings.Contains(msg, SnapshotTrailerKey+": raddle") {
		t.Errorf("root weft commit message = %q; want it to contain the Snapshot trailer", msg)
	}

	got, err := f.snapshotWarpSHA("raddle")
	if err != nil {
		t.Fatalf("snapshotWarpSHA() error = %v", err)
	}
	if got != warpHEAD {
		t.Errorf("snapshotWarpSHA() = %q; want %q", got, warpHEAD)
	}
}

// TestCommit_UnbornWarpHEAD_WithTags_DropsTagsNoErrorNoCommit covers the
// empty-commit rule's one genuine exception: an unborn warp HEAD drops the
// tags exactly as before this batch — no commit, no trailer, no error —
// because a snapshot's whole content is a warp SHA and there is none to
// record yet.
func TestCommit_UnbornWarpHEAD_WithTags_DropsTagsNoErrorNoCommit(t *testing.T) {
	warpPath := newUnbornWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	seedFabricConfig(t, warpPath)
	f := newFabric(t, warpPath, weftFixture.WeftPath)
	swapPushRecorder(t)

	preWeftSHA := currentSHA(t, weftFixture.WeftPath)

	result, err := f.Commit(nil, "unborn warp, tags only", []string{"raddle"}, SyncOptions{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if result.WarpCommitted || result.WeftCommitted {
		t.Errorf("Commit() = %+v; want a full no-op (unborn warp drops tags exactly as before)", result)
	}

	postWeftSHA := currentSHA(t, weftFixture.WeftPath)
	if postWeftSHA != preWeftSHA {
		t.Errorf("weft HEAD changed from %q to %q; want unchanged (no commit)", preWeftSHA, postWeftSHA)
	}

	got, err := f.snapshotWarpSHA("raddle")
	if err != nil {
		t.Fatalf("snapshotWarpSHA() error = %v", err)
	}
	if got != "" {
		t.Errorf("snapshotWarpSHA() = %q; want \"\" (nothing was recorded)", got)
	}
}

// TestCommit_SkipGit_WithTags_NoWeftCommitNoError covers the second
// exception: opts.SkipGit still produces no weft commit and no trailer
// regardless of tags — it is an explicit "touch no git at all" opt-out for
// the weft side — while the warp commit proceeds regardless, since SkipGit
// is weft-scoped for Fabric.Commit specifically.
func TestCommit_SkipGit_WithTags_NoWeftCommitNoError(t *testing.T) {
	f, warpPath, weftPath := newCommitFixture(t)
	swapPushRecorder(t)

	writeWarpFile(t, warpPath, "README", "warp change")
	writeWeftConfigContent(t, weftPath, "weft change")

	result, err := f.Commit([]string{"README", "_lyx/config.yaml"}, "skip git", []string{"raddle"}, SyncOptions{SkipGit: true})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if result.WeftCommitted {
		t.Errorf("Commit() = %+v; want no weft commit under SkipGit even with tags", result)
	}
	if !result.WarpCommitted {
		t.Errorf("Commit() = %+v; want the warp commit to still land (SkipGit is weft-scoped)", result)
	}

	got, err := f.snapshotWarpSHA("raddle")
	if err != nil {
		t.Fatalf("snapshotWarpSHA() error = %v", err)
	}
	if got != "" {
		t.Errorf("snapshotWarpSHA() = %q; want \"\" (SkipGit skips the weft side entirely)", got)
	}
}

// TestCommit_NoTagsNothingToCommit_RuleDoesNotOverFire guards against the
// uniform empty-commit rule firing when it should not: unchanged content and
// zero tags must still be a clean no-op, with the weft HEAD left untouched.
func TestCommit_NoTagsNothingToCommit_RuleDoesNotOverFire(t *testing.T) {
	f, warpPath, _ := newCommitFixture(t)
	swapPushRecorder(t)

	preWeftSHA, err := f.Weft.CurrentSHA()
	if err != nil {
		t.Fatalf("Weft.CurrentSHA() error = %v", err)
	}

	// newPlainWarpRepo already committed README with content "warp"; writing
	// the identical content again leaves nothing staged, and no tags are
	// supplied.
	writeWarpFile(t, warpPath, "README", "warp")

	result, err := f.Commit([]string{"README"}, "no-op, no tags", nil, SyncOptions{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if result.WarpCommitted || result.WeftCommitted {
		t.Fatalf("Commit() = %+v; want a full no-op (unchanged content, no tags)", result)
	}

	postWeftSHA, err := f.Weft.CurrentSHA()
	if err != nil {
		t.Fatalf("Weft.CurrentSHA() error = %v", err)
	}
	if postWeftSHA != preWeftSHA {
		t.Errorf("weft HEAD changed from %q to %q; want unchanged (the empty-commit rule must not fire with no tags)", preWeftSHA, postWeftSHA)
	}
}

// TestCommit_WarpOnlyTagged_InvokesPushRecorderOnce pins the CommitResult
// and push-seam shape on a warp-only tagged commit: WeftCommitted=true with
// a populated WeftSHA (the empty commit), and exactly one push-seam
// invocation — the async-push gate is WarpCommitted || WeftCommitted, and
// the snapshot trailer must reach the remote for cross-clone sharing.
func TestCommit_WarpOnlyTagged_InvokesPushRecorderOnce(t *testing.T) {
	f, warpPath, _ := newCommitFixture(t)
	calls := swapPushRecorder(t)

	writeWarpFile(t, warpPath, "README", "warp only tagged change")

	result, err := f.Commit([]string{"README"}, "warp only tagged commit", []string{"raddle"}, SyncOptions{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if !result.WeftCommitted || result.WeftSHA == "" {
		t.Fatalf("Commit() = %+v; want WeftCommitted=true and a populated WeftSHA", result)
	}
	if len(*calls) != 1 {
		t.Fatalf("push recorder invocation count = %d; want 1", len(*calls))
	}
}

// TestCommit_InvalidTag_OtherwiseEmpty_NothingCommitted pins that
// appendSnapshotTrailers' validate-all-before-appending-any property
// survives the empty-commit path: an invalid tag on an otherwise-fully-empty
// call fails before anything is staged or committed, on either side.
func TestCommit_InvalidTag_OtherwiseEmpty_NothingCommitted(t *testing.T) {
	f, warpPath, _ := newCommitFixture(t)
	swapPushRecorder(t)

	preWarpSHA := currentSHA(t, warpPath)
	preWeftSHA, err := f.Weft.CurrentSHA()
	if err != nil {
		t.Fatalf("Weft.CurrentSHA() error = %v", err)
	}

	_, err = f.Commit(nil, "invalid tag commit", []string{"bad tag with spaces"}, SyncOptions{})
	var invalidTagErr *ErrInvalidSnapshotTag
	if !errors.As(err, &invalidTagErr) {
		t.Fatalf("Commit() error = %v; want *ErrInvalidSnapshotTag via errors.As", err)
	}

	postWarpSHA := currentSHA(t, warpPath)
	postWeftSHA, err := f.Weft.CurrentSHA()
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

// TestCommit_DirtyWeftIndex_UnchangedContentWithTags_SurfacesPartialCommitError
// covers the accepted no-op-becomes-error transition: an aborted earlier run
// leaves unrelated content staged in the weft index (something the combined
// write lock serializes fabric's own callers against, but does not itself
// clean up); a tagged commit whose own pathspec content is unchanged reaches
// the `!committed` fall-through, calls CommitEmpty, and CommitEmpty's own
// dirty-index pre-check refuses with gitrepo.ErrIndexNotEmpty. That refusal
// must surface as a *PartialCommitError with WeftCommitted=false — the same
// unlanded-weft outcome the mapping already models — while the warp commit
// that already landed stands and is still pushed.
func TestCommit_DirtyWeftIndex_UnchangedContentWithTags_SurfacesPartialCommitError(t *testing.T) {
	f, warpPath, weftPath := newCommitFixture(t)
	calls := swapPushRecorder(t)

	writeWarpFile(t, warpPath, "README", "warp change")

	// Simulate an aborted earlier run's leftover staged content: unrelated to
	// this call's own pathspec, so StageAndCommit's own pathspec-scoped diff
	// never sees it, but CommitEmpty's unscoped diff --cached --quiet does.
	if err := os.WriteFile(filepath.Join(weftPath, "leftover.txt"), []byte("staged by an aborted earlier run"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	lyxtest.MustRun(t, weftPath, "git", "add", "leftover.txt")

	// _lyx/config.yaml is left at its fixture-default content, so the
	// weft-side pathspec's own StageAndCommit reports committed=false and
	// the empty-commit rule's !committed fall-through fires.
	result, err := f.Commit([]string{"README", "_lyx/config.yaml"}, "dirty index, tagged", []string{"raddle"}, SyncOptions{})
	if err == nil {
		t.Fatal("Commit() error = nil; want an error (CommitEmpty should refuse on the dirty index)")
	}

	var partialErr *PartialCommitError
	if !errors.As(err, &partialErr) {
		t.Fatalf("Commit() error = %v; want *PartialCommitError via errors.As", err)
	}
	if partialErr.WeftCommitted {
		t.Errorf("PartialCommitError.WeftCommitted = true; want false (CommitEmpty refused, nothing landed)")
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

	if len(*calls) != 1 {
		t.Errorf("push recorder invocation count = %d; want 1 (the durable warp commit should still be pushed)", len(*calls))
	}
}

// TestCommitWeft_PathspecMatchesNothing_WithTags_LandsEmptyCommit pins that
// CommitWeft — an exported entry point, not just an internal dispatch
// target — inherits commitWeftLocked's empty-commit rule as its own
// contract: called directly (not via Fabric.Commit) with a pathspec matching
// nothing and one snapshot tag, it lands the empty commit.
func TestCommitWeft_PathspecMatchesNothing_WithTags_LandsEmptyCommit(t *testing.T) {
	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	sha, committed, err := f.commitWeft([]string{"doesnotexist"}, DefaultCommitMessage, SyncOptions{}, "raddle")
	if err != nil {
		t.Fatalf("commitWeft() error = %v", err)
	}
	if !committed || sha == "" {
		t.Fatalf("commitWeft() = (%q, %v); want an empty commit to have landed", sha, committed)
	}

	warpSHA := currentSHA(t, warpPath)
	msg := commitMessageAt(t, weftFixture.WeftPath, sha)
	wantWarpTrailer := WarpSHATrailerKey + ": " + warpSHA
	if !strings.Contains(msg, wantWarpTrailer) {
		t.Errorf("commit message = %q; want it to contain %q", msg, wantWarpTrailer)
	}
	if !strings.Contains(msg, SnapshotTrailerKey+": raddle") {
		t.Errorf("commit message = %q; want it to contain the Snapshot trailer", msg)
	}
}
