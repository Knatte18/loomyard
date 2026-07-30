//go:build integration

// commit_integration_test.go — integration coverage for Fabric.Commit's
// classify-and-dispatch two-sided commit: warp-first ordering, correspondence
// recording, CommitResult field population across two-sided/warp-only/
// weft-only inputs, the warp commit's plain-git property (no trailer, no
// correspondence entry), Snapshot trailer presence/absence, message
// handling, and the spawnDetachedPushFn push-invocation seam (fired for
// anything landed, never for a genuine no-op). Package fabricengine
// (internal) because it swaps the unexported spawnDetachedPushFn seam.
// Reuses index_integration_test.go's fixture helpers (newPlainWarpRepo,
// commitWarp, newFabric, currentSHA) and syncweft_integration_test.go's
// (writeWeftConfigContent, commitMessageAt).

package fabricengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// seedFabricConfig seeds the fabric config into the weft fixture so
// WiredNames(weftPath) resolves the pathspec Fabric.Commit's classifier
// needs — every Fabric.Commit test requires this.
func seedFabricConfig(t *testing.T, weftPath string) {
	t.Helper()

	lyxtest.SeedConfig(t, weftPath, map[string]string{
		"fabric": "branch_prefix: \"\"\npathspec: _lyx _pattern\n",
	})
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
	seedFabricConfig(t, weftFixture.WeftPath)
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

// TestCommit_WarpOnly_SnapshotTagsDropped asserts that a warp-only
// Fabric.Commit with a non-empty snapshotTags returns no error, commits the
// warp side, and silently drops the tags: no Snapshot trailer and no
// Warp-SHA trailer land on the bare warp commit.
func TestCommit_WarpOnly_SnapshotTagsDropped(t *testing.T) {
	f, warpPath, _ := newCommitFixture(t)
	swapPushRecorder(t)

	writeWarpFile(t, warpPath, "README", "warp only change, tagged")

	result, err := f.Commit([]string{"README"}, "warp only commit", []string{"tag1"}, SyncOptions{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if !result.WarpCommitted {
		t.Fatalf("Commit() = %+v; want WarpCommitted=true", result)
	}

	msg := commitMessageAt(t, warpPath, result.WarpSHA)
	if strings.Contains(msg, SnapshotTrailerKey+":") {
		t.Errorf("warp-only commit message = %q; want no Snapshot trailer", msg)
	}
	if strings.Contains(msg, WarpSHATrailerKey+":") {
		t.Errorf("warp-only commit message = %q; want no Warp-SHA trailer", msg)
	}
}
